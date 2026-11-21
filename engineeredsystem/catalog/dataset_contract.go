package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Reader supplies complete finite-budget results, including terminal/close errors.
// Its caller owns connections, admission, deadlines and recovery.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// DatasetProvenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage. An explicit Limit remains a bounded sample.
type DatasetProvenance struct {
	OptionalColumns []string `json:"optional_columns,omitempty"`
	Layout          string   `json:"layout,omitempty"`
	Source          string   `json:"source"`
	OracleMajor     int      `json:"oracle_major"`
	Scope           string   `json:"scope"`
	Limit           int      `json:"sample_limit,omitempty"`
	Complete        bool     `json:"complete"`
}

var ErrUnsupportedVersion = errors.New("unsupported Oracle engineeredsystem/catalog source version")
var ErrInvalidRequest = errors.New("invalid Oracle engineeredsystem/catalog request")
var ErrInvalidEvidence = errors.New("invalid Oracle engineeredsystem/catalog evidence")

// DatasetError retains private causes without echoing SQL, bindings or driver diagnostics.
type DatasetError struct {
	Source string
	cause  error
}

func (e *DatasetError) Error() string {
	return "Oracle engineeredsystem/catalog fact read failed (" + e.Source + ")"
}
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
	case 12, 18, 19, 21, 23:
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
