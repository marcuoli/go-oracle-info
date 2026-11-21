// Package sqlplan selects and decodes read-only Oracle sqlplan facts.
package sqlplan

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Options selects an already admitted Oracle release and exact dictionary scope.
// Empty name/owner lists retain every dictionary-visible row in the connected
// container. It does not establish complete CDB/RAC visibility or authorization.
type Options struct {
	OracleMajor int
	Owners      []string
	Names       []string
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
	Source      string   `json:"source"`
	OracleMajor int      `json:"oracle_major"`
	Scope       string   `json:"scope"`
	Owners      []string `json:"owners,omitempty"`
	Names       []string `json:"names,omitempty"`
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
	ownerColumn string
	nameColumn  string

	sql        string
	args       []any
	schema     []rowset.Column
	integers   []string
	provenance Provenance
}

func validate(options Options) error {
	if options.OracleMajor < 11 {
		return ErrUnsupportedVersion
	}
	for _, values := range [][]string{options.Owners, options.Names} {
		if len(values) > 1000 {
			return ErrInvalidRequest
		}
		for _, value := range values {
			if value == "" || len(value) > 128 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				return ErrInvalidRequest
			}
		}
	}
	return nil
}
func containsScope(values []string, value session.Value) bool {
	if len(values) == 0 {
		return true
	}
	if value.Null {
		return false
	}
	for _, v := range values {
		if v == value.Text {
			return true
		}
	}
	return false
}
func scopeFilter(statement string, args []any, column string, values []string) (string, []any) {
	if len(values) == 0 {
		return statement, args
	}
	binds := make([]string, len(values))
	for i, value := range values {
		args = append(args, value)
		binds[i] = "NLSSORT(:" + strconv.Itoa(len(args)) + ", 'NLS_SORT=BINARY')"
	}
	conjunction := " WHERE "
	if strings.Contains(statement, " WHERE ") {
		conjunction = " AND "
	}
	return statement + conjunction + "NLSSORT(" + column + ", 'NLS_SORT=BINARY') IN (" + strings.Join(binds, ",") + ")", args
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
		if plan.ownerColumn != "" && !containsScope(plan.provenance.Owners, row[index[plan.ownerColumn]]) {
			return nil, ErrInvalidEvidence
		}
		if plan.nameColumn != "" && !containsScope(plan.provenance.Names, row[index[plan.nameColumn]]) {
			return nil, ErrInvalidEvidence
		}

		for _, v := range row {
			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, ErrInvalidEvidence
			}
		}
		for _, name := range plan.integers {
			v := row[index[name]]
			if !v.Null && !numeric.IsSignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
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
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &Error{Operation: "decode", cause: ErrInvalidEvidence}
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
