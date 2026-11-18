package memory

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// DatasetProvenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage.
type DatasetProvenance struct {
	Parameter         Parameter `json:"parameter,omitempty"`
	TargetContainerID int       `json:"admitted_target_container_id,omitempty"`
	AllowInherited    bool      `json:"admitted_inheritance,omitempty"`
	EffectiveADB      bool      `json:"admitted_effective_adb,omitempty"`
	Source            string    `json:"source"`
	OracleMajor       int       `json:"oracle_major"`
	Scope             string    `json:"scope"`
	Complete          bool      `json:"complete"`
}

var ErrUnsupportedVersion = errors.New("unsupported Oracle memory source version")
var ErrInvalidRequest = errors.New("invalid Oracle memory request")

// DatasetError retains private causes without echoing SQL, bindings or driver diagnostics.
type DatasetError struct {
	Source string
	cause  error
}

func (e *DatasetError) Error() string { return "Oracle memory fact read failed (" + e.Source + ")" }
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
	case 9, 10, 11, 12, 18, 19, 21, 23:
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
func datasetColumns(result session.Result, p datasetSelection, numeric ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range numeric {
			v := row[index[name]]
			if !v.Null && (v.Text == "" || strings.IndexFunc(v.Text, func(r rune) bool { return r < '0' || r > '9' }) >= 0) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readDatasetJSON(ctx context.Context, q rowset.Queryer, p datasetSelection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	p.provenance.Complete = true
	envelope := struct {
		Provenance DatasetProvenance `json:"provenance"`
		Data       json.RawMessage   `json:"data"`
	}{Provenance: p.provenance}
	overhead, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(overhead) - 4
	if budget.MaxBytes < 1 {
		return nil, &DatasetError{p.provenance.Source, rowset.ErrCapacity}
	}
	cursor, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	table, err := rowset.Read(ctx, cursor, p.schema, budget)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &DatasetError{p.provenance.Source, ErrInvalidEvidence}
	}
	if err = validate(result); err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	envelope.Data, err = json.Marshal(table)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = datasetReadError(ctx, p, err); err != nil {
		return nil, err
	}
	return payload, nil
}
