// Package storage exposes reviewed read-only storage facts over admitted caller handles.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
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
	Source          string   `json:"source"`
	OracleMajor     int      `json:"oracle_major"`
	Scope           string   `json:"scope"`
	Instances       []int64  `json:"instances,omitempty"`
	Complete        bool     `json:"complete"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
}

var (
	// ErrUnsupportedVersion rejects unsupported source releases or columns.
	ErrUnsupportedVersion = errors.New("unsupported Oracle storage source version")
	// ErrInvalidRequest rejects invalid filters, readers and SQL references.
	ErrInvalidRequest = errors.New("invalid Oracle storage request")
	// ErrInvalidEvidence rejects malformed shapes, exact integers or selection.
	ErrInvalidEvidence = errors.New("invalid Oracle storage evidence")
)

// Error exposes only a static source. Unwrap retains causes for errors.Is/As and
// deliberately private diagnostics; driver errors can contain sensitive inputs.
type Error struct {
	Source string
	cause  error
}

// Error returns a static source without formatting driver details.
func (e *Error) Error() string { return "storage read failed (" + e.Source + ")" }

// Unwrap preserves the original cause for deliberate inspection.
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	maxRows     int
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
			// DBA_TEMP_FILES.INST_ID is an optional owning ID, not reporting identity.
			if plan.provenance.Source == "DBA_TEMP_FILES" && name == "INST_ID" {
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
				if !numeric.IsUnsignedInteger(v.Text) {
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
	plan.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql, plan.args...), plan.schema, limits, rowset.EnvelopeOptions{
		MaxRows:    plan.maxRows,
		Provenance: plan.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{plan.provenance.Source, cause}
	}, "limits")
}

// The dictionary summaries retain their reviewed version and reader contracts,
// separately from the file inventory projections.
func fileSummaryMajor(major int) error {
	switch major {
	case 19, 21, 23:
		return nil
	default:
		return ErrUnsupportedVersion
	}
}

func validFileSummaryReader(reader Reader) bool {
	if reader == nil {
		return false
	}
	v := reflect.ValueOf(reader)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

func fileSummaryReadError(ctx context.Context, p selection, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &Error{p.provenance.Source, err}
	}
	return nil
}

func readFileSummaryJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapMovedEnvelopeError(err, func(cause error) error {
		return &Error{p.provenance.Source, cause}
	}, "limits", "header")
}

// Request admits a source version and caller-owned SQL execution reference.
type Request struct {
	OracleMajor int
	Reference   reference.Reference
}

// TempfileOptions explicitly admits optional local-tempfile columns. IncludeLocal
// requires 18+; the common 11g projection never fabricates owning-instance data.
type TempfileOptions struct {
	OracleMajor  int
	IncludeLocal bool
}

// TempfileRequest supplies the explicit column capability and caller reference.
type TempfileRequest struct {
	Options   TempfileOptions
	Reference reference.Reference
}
