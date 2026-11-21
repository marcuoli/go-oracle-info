// Package identity selects and decodes Oracle database, instance and platform facts.
// Platform text is not evidence of an operating-system distribution or release.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Provenance records the selected source and admitted visibility, not physical coverage.
type Provenance struct {
	Source          string   `json:"source"`
	Scope           string   `json:"scope"`
	OracleMajor     int      `json:"oracle_major"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid execution reference.
	ErrInvalidRequest = errors.New("invalid Oracle identity request")
)

// ReadError retains the original cause for private errors.Is/As inspection.
// Its public message never formats driver values, SQL or bind data.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle identity fact read failed" }

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

func (e *DatasetError) Error() string { return "Oracle identity fact read failed (" + e.Source + ")" }

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

// State preserves known text, SQL NULL and authoritative no-row results.
type State string

const (
	Available State = "available"
	Null      State = "null"
	NoRows    State = "no_rows"
)

// Text preserves the original source text without inferred or localized values.
type Text struct {
	State State
	Value string
}

const (
	DatabasePlatform PlatformSource = "v$database.platform_name"
	ServerTNSBanner  PlatformSource = "v$version.tns_banner"
)

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle identity version")
	ErrInvalidEvidence    = errors.New("invalid Oracle identity evidence")
)

// Major parses the leading numeric release component without rewriting the value.
func Major(version string) (int, error) {
	if len(version) > 128 {
		return 0, ErrUnsupportedVersion
	}
	component, _, _ := strings.Cut(version, ".")
	if component == "" || strings.IndexFunc(component, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, ErrUnsupportedVersion
	}
	major, err := strconv.Atoi(component)
	if err != nil || major < 9 {
		return 0, ErrUnsupportedVersion
	}
	return major, nil
}

func row(result session.Result, names ...string) (map[string]session.Value, bool, error) {
	if len(result.Columns) != len(names) || len(result.Rows) > 1 {
		return nil, false, ErrInvalidEvidence
	}
	index := make(map[string]int, len(names))
	for i, name := range result.Columns {
		name = strings.ToUpper(name)
		if _, ok := index[name]; name == "" || ok {
			return nil, false, ErrInvalidEvidence
		}
		index[name] = i
	}
	for _, name := range names {
		if _, ok := index[name]; !ok {
			return nil, false, ErrInvalidEvidence
		}
	}
	if len(result.Rows) == 0 {
		return nil, true, nil
	}
	if len(result.Rows[0]) != len(names) {
		return nil, false, ErrInvalidEvidence
	}
	values := make(map[string]session.Value, len(names))
	for name, i := range index {
		values[name] = result.Rows[0][i]
	}
	return values, false, nil
}

func text(value session.Value) Text {
	if value.Null {
		return Text{State: Null}
	}
	return Text{State: Available, Value: strings.Clone(value.Text)}
}

func decodeScalar(result session.Result, column string) (Text, error) {
	r, absent, err := row(result, column)
	if err != nil {
		return Text{}, err
	}
	if absent {
		return Text{State: NoRows}, nil
	}
	return text(r[column]), nil
}
