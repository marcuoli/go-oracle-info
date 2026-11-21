package services

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader returns only complete bounded results and retains terminal/close failures.
// The caller owns admission, connections, cancellation and retry policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance records the selected source and admitted visibility, not physical coverage.
type Provenance struct {
	Source          string   `json:"source"`
	Scope           string   `json:"scope"`
	OracleMajor     int      `json:"oracle_major"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid execution reference.
	ErrInvalidRequest = errors.New("invalid Oracle services request")
	// ErrInvalidEvidence rejects malformed metadata, values or late rows.
	ErrInvalidEvidence = errors.New("invalid Oracle services evidence")
	// ErrUnsupportedVersion rejects a release below the reviewed source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle services version")
)

// ReadError retains the original cause for private errors.Is/As inspection.
// Its public message never formats driver values, SQL or bind data.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle services fact read failed" }

// Unwrap retains original causes for explicit private inspection.
func (e *ReadError) Unwrap() error { return e.cause }

func validateFactRequest(reader Reader, ref reference.Reference) error {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return ErrInvalidRequest
	}
	return nil
}

func factColumns(result session.Result, schema []rowset.Column) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	return index, nil
}

// Request supplies a known release and a caller-owned execution reference.
// IncludeEdition admits the 11.2.0.2+ column on an established 11g source;
// 12+ always selects it. An unknown 11g patch set leaves it unselected.
type Request struct {
	OracleMajor    int
	IncludeEdition bool
	Reference      reference.Reference
}

// Cancellation is checked after decoding too: a completed payload cannot override it.
func factReadError(ctx context.Context, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &ReadError{cause: err}
	}
	return nil
}

// DatasetProvenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage.
type DatasetProvenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
	Complete    bool   `json:"complete"`
}

// DatasetError retains private causes without echoing SQL, bindings or driver diagnostics.
type DatasetError struct {
	Source string
	cause  error
}

func (e *DatasetError) Error() string { return "Oracle services fact read failed (" + e.Source + ")" }

func (e *DatasetError) Unwrap() error { return e.cause }

type datasetSelection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance DatasetProvenance
}

// New fact projections admit the independently reviewed numeric release families.
// Older compatibility SQL remains available; branding does not admit numeric26.
func datasetMajor(major int) error {
	switch major {
	case 19, 21, 23:
		return nil
	}
	return ErrUnsupportedVersion
}

func validDatasetReader(reader Reader) bool {
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

func datasetReadError(ctx context.Context, p datasetSelection, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &DatasetError{p.provenance.Source, err}
	}
	return nil
}

func datasetColumns(result session.Result, p datasetSelection, integerFields ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range integerFields {
			v := row[index[name]]
			if !v.Null && !numeric.IsUnsignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readDatasetJSON(ctx context.Context, q rowset.Queryer, p datasetSelection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &DatasetError{p.provenance.Source, cause}
	}, "limits", "header")
}
