// Package asm exposes reviewed read-only ASM client facts over admitted caller handles.
package asm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Reader supplies complete finite-budget results, including terminal and close
// failures. Its caller retains admission, session lifetime and cancellation.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance identifies the admitted view and scope. Complete is true only after
// an owned reader or JSON cursor completes successfully. Visible instance rows
// never establish complete physical RAC membership or a common snapshot.
type Provenance struct {
	Source      string  `json:"source"`
	OracleMajor int     `json:"oracle_major"`
	Scope       string  `json:"scope"`
	Instances   []int64 `json:"instances,omitempty"`
	Complete    bool    `json:"complete"`
	SourceRole  string  `json:"source_role,omitempty"`
}

var (
	// ErrUnsupportedVersion rejects unsupported source releases or columns.
	ErrUnsupportedVersion = errors.New("unsupported Oracle ASM client source version")
	// ErrInvalidRequest rejects invalid filters, readers and SQL references.
	ErrInvalidRequest = errors.New("invalid Oracle ASM client request")
	// ErrInvalidEvidence rejects malformed shapes, exact integers or selection.
	ErrInvalidEvidence = errors.New("invalid Oracle ASM client evidence")
)

// Error exposes only a static source. Unwrap retains causes for errors.Is/As and
// deliberately private diagnostics; driver errors can contain sensitive inputs.
type Error struct {
	Source string
	cause  error
}

// Error returns a static source without formatting driver details.
func (e *Error) Error() string { return "ASM client read failed (" + e.Source + ")" }

// Unwrap preserves the original cause for deliberate inspection.
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql         string
	args        []any
	schema      []rowset.Column
	integerText []string
	provenance  Provenance
}

func columns(result session.Result, plan selection) (map[string]int, error) {
	names := make([]string, len(plan.schema))
	for i, c := range plan.schema {
		names[i] = c.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
	}
	integers := map[string]bool{}
	for _, c := range plan.schema {
		if c.Kind == rowset.Integer {
			integers[c.Name] = true
		}
	}
	for _, name := range plan.integerText {
		integers[name] = true
	}
	for _, row := range result.Rows {
		for _, name := range []string{"INST_ID", "SOURCE_INST_ID"} {
			pos, exists := index[name]
			if !exists {
				continue
			}

			v := row[pos]
			id, err := strconv.ParseInt(v.Text, 10, 64)
			if v.Null || err != nil || id <= 0 {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
		}

		for _, c := range plan.schema {
			v := row[index[c.Name]]
			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
			if !v.Null && integers[c.Name] {
				// Preserve exact nonnegative Oracle NUMBER text without float rounding.
				if v.Text == "" || strings.IndexFunc(v.Text, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
					return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
				}
				if _, ok := new(big.Int).SetString(v.Text, 10); !ok {
					return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
				}
			}
		}
		if pos, exists := index["INST_ID"]; exists && len(plan.provenance.Instances) > 0 {
			v := row[pos]
			id, err := strconv.ParseInt(v.Text, 10, 64)
			admitted := false
			for _, selected := range plan.provenance.Instances {
				admitted = admitted || selected == id
			}
			if err != nil || v.Null || !admitted {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
		}
	}
	return index, nil
}
func optional(row []session.Value, index map[string]int, name string) *session.Value {
	if pos, ok := index[name]; ok {
		v := row[pos]
		return &v
	}
	return nil
}
func query(ctx context.Context, reader Reader, ref reference.Reference, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{plan.provenance.Source, err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{plan.provenance.Source, err}
	}
	return result, nil
}
func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	return rows, nil
}
func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	plan.provenance.Complete = true
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: plan.provenance}
	metadata, err := json.Marshal(envelope)
	if err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	resolved.MaxBytes -= len(metadata) - 4
	if resolved.MaxBytes < 1 {
		return nil, &Error{plan.provenance.Source, rowset.ErrCapacity}
	}
	rows, err := open(ctx, q, plan)
	if err != nil {
		return nil, err
	}
	// Shared rowset owns the only cursor consumption and finalization. Domain
	// admission must pass after complete reading, before encoding any JSON.
	table, err := rowset.Read(ctx, rows, plan.schema, resolved)
	if err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
	}
	if err = validate(result); err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	// Encode the original native table, preserving its order, scalar kinds and NULL.
	data, err := json.Marshal(table)
	if err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	envelope.Data = data
	payload, err := json.Marshal(envelope)
	original, _ := rowset.Resolve(limits)
	if len(payload) > original.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	return payload, nil
}
func instanceClause(ids []int64) (string, []any, error) {
	if len(ids) > 1000 {
		return "", nil, ErrInvalidRequest
	}
	seen := map[int64]bool{}
	args := make([]any, 0, len(ids))
	binds := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return "", nil, ErrInvalidRequest
		}
		seen[id] = true
		args = append(args, id)
		binds = append(binds, ":"+strconv.Itoa(len(args)))
	}
	if len(ids) == 0 {
		return "", args, nil
	}
	return "v.INST_ID IN (" + strings.Join(binds, ",") + ")", args, nil
}
