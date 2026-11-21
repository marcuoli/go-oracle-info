package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/projection"
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
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return Rows(ctx, q, major)
	}, schema(major), limits, rowset.EnvelopeOptions{
		Provenance: source(major),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}

// ComponentsOptions admits the Oracle projection for registered components.
type ComponentsOptions struct {
	OracleMajor int
}

// ComponentsRequest adds the caller's SQL reference to the source options.
type ComponentsRequest struct {
	Options   ComponentsOptions
	Reference reference.Reference
}

// ComponentsStatement is the named form of Statement.
func ComponentsStatement(options ComponentsOptions) (string, error) {
	return Statement(options.OracleMajor)
}

// DecodeComponents is the named form of Decode.
func DecodeComponents(result session.Result, options ComponentsOptions) ([]Component, error) {
	return Decode(result, options.OracleMajor)
}

// ReadComponents is the named form of Read.
func ReadComponents(ctx context.Context, reader Reader, request ComponentsRequest) (Result, error) {
	return Read(ctx, reader, Request{OracleMajor: request.Options.OracleMajor, Reference: request.Reference})
}

// ComponentsRows is the named form of Rows.
func ComponentsRows(ctx context.Context, q rowset.Queryer, options ComponentsOptions) (*sql.Rows, error) {
	return Rows(ctx, q, options.OracleMajor)
}

// ComponentsJSON is the named form of JSON.
func ComponentsJSON(ctx context.Context, q rowset.Queryer, options ComponentsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options.OracleMajor, limits)
}
