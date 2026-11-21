// Package registry selects and decodes exact current-container dictionary
// component facts. It owns no connections, grants, container changes or acceptance
// policy. Failed/incomplete reads must not be presented as an empty inventory.
package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

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

var (
	// ErrUnsupportedVersion rejects a source below the reviewed Oracle 11+ floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle registry version")
	// ErrInvalidEvidence rejects a projection or row shape that cannot be decoded.
	ErrInvalidEvidence = errors.New("invalid Oracle registry evidence")
)

// Component preserves source strings and SQL NULL without interpreting status.
// Modified is the raw VARCHAR2 value from DBA_REGISTRY, not a parsed timestamp or
// UTC time. VersionFull is nil when its column was not selected on a legacy source;
// a non-nil value with Valid false is a selected SQL NULL. Versions are not merged
// or padded into an inferred release update, and registration is not licensing.
type Component struct {
	ID          sql.NullString
	Name        sql.NullString
	Version     sql.NullString
	VersionFull *sql.NullString
	Status      sql.NullString
	Modified    sql.NullString
}

// Statement selects dictionary components visible in the admitted current
// container. VERSION_FULL is documented from 18 onward; 11 through 17 select only
// the legacy version. The caller admits privileges/open-state availability and
// bounds the complete read; this is not a cross-container or cluster inventory.
func Statement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	columns := "COMP_ID AS COMP_ID, COMP_NAME AS COMP_NAME, VERSION AS VERSION"
	if major >= 18 {
		columns += ", VERSION_FULL AS VERSION_FULL"
	}
	return "SELECT " + columns + ", STATUS AS STATUS, MODIFIED AS MODIFIED FROM DBA_REGISTRY ORDER BY COMP_ID", nil
}

// Decode validates the complete Statement projection before retaining any rows.
// All strings, NULLs, repeated IDs and source row order survive unchanged. A nil
// error and zero rows describe only a successful empty source; query, privilege,
// MOUNTED-state and cursor/capacity errors remain with the caller. A malformed late
// row returns no partial inventory. Decoding neither scores nor filters statuses.
func Decode(result session.Result, major int) ([]Component, error) {
	if _, err := Statement(major); err != nil {
		return nil, err
	}
	names := []string{"COMP_ID", "COMP_NAME", "VERSION", "STATUS", "MODIFIED"}
	if major >= 18 {
		names = append(names, "VERSION_FULL")
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	facts := make([]Component, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := Component{
			ID: nullable(row[index["COMP_ID"]]), Name: nullable(row[index["COMP_NAME"]]),
			Version: nullable(row[index["VERSION"]]), Status: nullable(row[index["STATUS"]]),
			Modified: nullable(row[index["MODIFIED"]]),
		}
		if major >= 18 {
			full := nullable(row[index["VERSION_FULL"]])
			fact.VersionFull = &full
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func nullable(value session.Value) sql.NullString {
	if value.Null {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.Clone(value.Text), Valid: true}
}
