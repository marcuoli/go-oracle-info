package objects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Reader returns only complete bounded results and retains terminal/close failures.
// The caller owns admission, connections, cancellation and retry policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance records the selected source and admitted visibility, not physical coverage.
type Provenance struct {
	Source      string   `json:"source"`
	Scope       string   `json:"scope"`
	OracleMajor int      `json:"oracle_major"`
	Owners      []string `json:"owners,omitempty"`
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid execution reference.
	ErrInvalidRequest = errors.New("invalid Oracle objects request")
	// ErrInvalidEvidence rejects malformed metadata, values or late rows.
	ErrInvalidEvidence = errors.New("invalid Oracle objects evidence")
	// ErrUnsupportedVersion rejects a release below the reviewed source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle objects version")
)

// ReadError retains the original cause for private errors.Is/As inspection.
// Its public message never formats driver values, SQL or bind data.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle objects fact read failed" }

// Unwrap retains original causes for explicit private inspection.
func (e *ReadError) Unwrap() error { return e.cause }

func validateFactRequest(reader Reader, ref reference.Reference) error {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return ErrInvalidRequest
	}
	return nil
}

func factColumns(result session.Result, schema []rowset.Column) (map[string]int, error) {
	names := make([]string, len(schema))
	for i, column := range schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, column := range schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return nil, ErrInvalidEvidence
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return nil, ErrInvalidEvidence
				}
			}
		}
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

const statisticsColumns = "SELECT TABLE_NAME, " + statisticsValues

const statisticsValues = "TO_CHAR(NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT, TO_CHAR(LAST_ANALYZED, 'YYYY-MM-DD\"T\"HH24:MI:SS', 'NLS_CALENDAR=GREGORIAN') AS LAST_ANALYZED"

// DatasetProvenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage. An explicit Limit remains a bounded sample.
type DatasetProvenance struct {
	OracleMaintainedFilter bool     `json:"oracle_maintained_filter,omitempty"`
	Owners                 []string `json:"owners,omitempty"`
	Table                  string   `json:"table,omitempty"`
	Source                 string   `json:"source"`
	OracleMajor            int      `json:"oracle_major"`
	Scope                  string   `json:"scope"`
	Limit                  int      `json:"sample_limit,omitempty"`
	Owner                  string   `json:"owner,omitempty"`
	Complete               bool     `json:"complete"`
}

// DatasetError retains private causes without echoing SQL, bindings or driver diagnostics.
type DatasetError struct {
	Source string
	cause  error
}

func (e *DatasetError) Error() string { return "Oracle objects fact read failed (" + e.Source + ")" }

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

func ownerFilter(owners []string) (string, []any) {
	if len(owners) == 0 {
		return "", []any{}
	}
	placeholders, args := make([]string, len(owners)), make([]any, len(owners))
	for i, owner := range owners {
		placeholders[i], args[i] = fmt.Sprintf(":%d", i+1), owner
	}
	return " WHERE OWNER IN (" + strings.Join(placeholders, ",") + ")", args
}

// QuoteIdentifier quotes one dictionary identifier without changing its case.
// It rejects empty, invalid UTF-8, NUL and names exceeding 128 bytes. This is SQL
// escaping, not authorization to access the named object.
func QuoteIdentifier(value string) (string, error) {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", errors.New("invalid catalog identifier")
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`, nil
}
