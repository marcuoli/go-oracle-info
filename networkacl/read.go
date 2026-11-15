// Package networkacl selects distinct read-only ACL visibility facts.
package networkacl

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Dataset identifies a distinct documented visibility/source responsibility.
type Dataset string

const (
	// DBAHostMappings selects admitted DBA host-to-ACL mappings.
	DBAHostMappings Dataset = "dba_host_mappings"
	// DBAHostPrivileges selects DBA ACL/ACE privilege facts, not effective access.
	DBAHostPrivileges Dataset = "dba_host_privileges"
	// DBAWalletMappings selects DBA wallet assignments, including the 11g mapping.
	DBAWalletMappings Dataset = "dba_wallet_mappings"
	// DBAWalletPrivileges selects DBA wallet ACE facts from 12c onward.
	DBAWalletPrivileges Dataset = "dba_wallet_privileges"
	// UserHostPrivileges selects the current user's visible host status only.
	UserHostPrivileges Dataset = "user_host_privileges"
	// UserWalletPrivileges selects the current user's visible wallet status only.
	UserWalletPrivileges Dataset = "user_wallet_privileges"
)

// Options admits one source/release; it changes no grants or session identity.
type Options struct {
	OracleMajor int
	Dataset     Dataset
}

// Reader supplies complete, bounded results and retains terminal/close failures.
// Its owner retains admission, deadlines, session lifetime and recovery policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies a stable caller reference and finite result limits. Zero
// limits use shared safe defaults; the reader also needs transport/LOB budgets.
type Request struct {
	Options   Options
	Reference reference.Reference
	Limits    rowset.Limits
}

// Provenance retains the selected source, release and dictionary visibility.
// Successful empty records mean an empty admitted selection, never unavailable.
type Provenance struct {
	Source      string  `json:"source"`
	OracleMajor int     `json:"oracle_major"`
	Scope       string  `json:"scope"`
	Dataset     Dataset `json:"dataset"`
}

var (
	// ErrUnsupportedVersion rejects sources before Oracle 11g.
	ErrUnsupportedVersion = errors.New("Oracle dictionary facts require Oracle 11g or later")
	// ErrInvalidRequest rejects invalid scopes, limits, readers or references.
	ErrInvalidRequest = errors.New("invalid Oracle dictionary fact request")
	// ErrInvalidEvidence rejects malformed or partial dictionary observations.
	ErrInvalidEvidence = errors.New("invalid Oracle dictionary fact evidence")
)

// Error retains a private cause for errors.Is/As without formatting source data.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "Oracle dictionary " + e.Operation + " failed" }
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	integers   []string
	provenance Provenance
}

func columns(result session.Result, plan selection) (map[string]int, error) {
	names := make([]string, len(plan.schema))
	for i, c := range plan.schema {
		names[i] = c.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, v := range row {
			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, ErrInvalidEvidence
			}
		}
		for _, name := range plan.integers {
			v := row[index[name]]
			if !v.Null {
				if _, ok := new(big.Int).SetString(v.Text, 10); !ok {
					return nil, ErrInvalidEvidence
				}
			}
		}
	}
	return index, nil
}
func query(ctx context.Context, reader Reader, request Request, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return session.Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return session.Result{}, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{Operation: "query", cause: err}
	}
	if len(result.Rows) > limits.MaxRows {
		return session.Result{}, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	// Count every retained value, including late values, before publishing facts.
	retained := 0
	for _, row := range result.Rows {
		for _, v := range row {
			if len(v.Text) > limits.MaxBytes-retained {
				return session.Result{}, &Error{Operation: "budget", cause: rowset.ErrCapacity}
			}
			retained += len(v.Text)
		}
	}
	return result, nil
}
func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	type envelope struct {
		Provenance Provenance `json:"provenance"`
		Data       any        `json:"data"`
	}
	header, err := json.Marshal(envelope{Provenance: plan.provenance})
	if err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	original := resolved.MaxBytes
	resolved.MaxBytes -= len(header) - 4
	if resolved.MaxBytes < 1 {
		return nil, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{Operation: "query", cause: err}
	}
	table, err := rowset.Read(ctx, rows, plan.schema, resolved)
	if err != nil {
		return nil, &Error{Operation: "consume", cause: err}
	}
	result := session.Result{Columns: make([]string, len(table.Columns)), Rows: make([][]session.Value, len(table.Rows))}
	for i, c := range table.Columns {
		result.Columns[i] = c.Name
	}
	for i, row := range table.Rows {
		result.Rows[i] = make([]session.Value, len(row))
		for j, v := range row {
			if v == nil {
				result.Rows[i][j].Null = true
			} else {
				text, ok := v.(string)
				if !ok {
					return nil, &Error{Operation: "decode", cause: ErrInvalidEvidence}
				}
				result.Rows[i][j].Text = text
			}
		}
	}
	if _, err = columns(result, plan); err != nil {
		return nil, &Error{Operation: "decode", cause: err}
	}
	payload, err := json.Marshal(envelope{Provenance: plan.provenance, Data: table})
	if len(payload) > original {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	return json.RawMessage(payload), nil
}
