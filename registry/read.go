package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete bounded results, retaining terminal and close failures.
// It owns admission, lease lifetime and cancellation; this package never retries.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits a known release and caller-owned execution reference.
type Request struct {
	OracleMajor int
	Reference   reference.Reference
}

// Provenance identifies the connected dictionary source without claiming licensing.
type Provenance struct {
	Source      string `json:"source"`
	Scope       string `json:"scope"`
	OracleMajor int    `json:"oracle_major"`
}

// Result retains the published Component model and its complete ordered inventory.
type Result struct {
	Provenance Provenance
	Rows       []Component
}

// ErrInvalidRequest rejects absent readers and invalid execution references.
var ErrInvalidRequest = errors.New("invalid Oracle registry request")

// ReadError retains original errors for explicit private errors.Is/As inspection.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle registry fact read failed" }

// Unwrap retains original causes for explicit private inspection.
func (e *ReadError) Unwrap() error { return e.cause }

func source(major int) Provenance {
	return Provenance{Source: "DBA_REGISTRY", Scope: "current_container_dictionary", OracleMajor: major}
}
func schema(major int) []rowset.Column {
	columns := []rowset.Column{{Name: "COMP_ID", Kind: rowset.Text}, {Name: "COMP_NAME", Kind: rowset.Text}, {Name: "VERSION", Kind: rowset.Text}}
	if major >= 18 {
		columns = append(columns, rowset.Column{Name: "VERSION_FULL", Kind: rowset.Text})
	}
	return append(columns, rowset.Column{Name: "STATUS", Kind: rowset.Text}, rowset.Column{Name: "MODIFIED", Kind: rowset.Text})
}

// Read executes Statement once on the existing reader, then validates every row
// with Decode. Existing VERSION_FULL/NULL/MODIFIED semantics remain unchanged.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return Result{}, ErrInvalidRequest
	}
	statement, err := Statement(request.OracleMajor)
	if err != nil {
		return Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return Result{}, &ReadError{cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return Result{}, &ReadError{cause: err}
	}
	facts, err := Decode(result, request.OracleMajor)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return Result{}, &ReadError{cause: err}
	}
	return Result{Provenance: source(request.OracleMajor), Rows: facts}, nil
}

// Rows returns the actual native cursor unchanged; callers own Next/Scan/Err/Close
// and their budgets. This method does not manufacture a session stream escape.
func Rows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	statement, err := Statement(major)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// JSON independently consumes/finalizes Rows' projection into bounded JSON.
// No prefix survives terminal, close, cancellation, conversion or capacity failure.
func JSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: source(major)}
	metadata, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(metadata) - 4
	if budget.MaxBytes < 1 {
		return nil, rowset.ErrCapacity
	}
	cursor, err := Rows(ctx, q, major)
	if err != nil {
		return nil, err
	}
	envelope.Data, err = rowset.ReadJSON(ctx, cursor, schema(major), budget)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &ReadError{cause: err}
	}
	return json.RawMessage(payload), nil
}
