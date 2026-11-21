// Package catalog owns capability-selected engineered-system SQL. No discovered
// identifier is interpolated: every relation, column and expression is fixed.
package catalog

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
	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
)

type Schema map[string]map[string]string

type Command struct {
	ID, Text, View string
	Original       string
}

type fragments struct{ Instance, View, CellType, CellHash, CellName, SourceType, Payload, NativeLength sqltemplate.Fragment }

func xmlCommand(id, view string, f fragments) (Command, bool, error) {
	text, err := sqltemplate.Render(id, XMLTemplate, f)
	return Command{ID: id, Text: text, View: "SYS." + view, Original: XMLTemplate}, true, err
}

func choose(s Schema, suffix string) (string, map[string]string) {
	for _, prefix := range []string{"GV_$", "V_$"} {
		view := prefix + suffix
		if columns := s[view]; len(columns) > 0 {
			return view, columns
		}
	}
	return "", nil
}

func instanceValid(view string, columns map[string]string) bool {
	return !strings.HasPrefix(view, "GV_$") || columns["INST_ID"] == "NUMBER"
}

func localFallback(schema Schema, global string) Schema {
	if !strings.HasPrefix(global, "GV_$") || len(schema[strings.TrimPrefix(global, "G")]) == 0 {
		return nil
	}
	next := make(Schema, len(schema))
	for view, columns := range schema {
		if view != global {
			next[view] = columns
		}
	}
	return next
}

func instance(view string) sqltemplate.Fragment {
	if strings.HasPrefix(view, "GV_$") {
		return "C.INST_ID"
	}
	return "TO_NUMBER(SYS_CONTEXT('USERENV', 'INSTANCE'))"
}

func has(columns map[string]string, names ...string) bool {
	for _, name := range names {
		if columns[name] == "" {
			return false
		}
	}
	return true
}

func textType(t string) bool {
	return t == "VARCHAR2" || t == "NVARCHAR2" || t == "CHAR" || t == "NCHAR"
}

func payloadLength(t, expression string) (sqltemplate.Fragment, bool) {
	if t == "CLOB" || t == "NCLOB" {
		return sqltemplate.Fragment("DBMS_LOB.GETLENGTH(" + expression + ")"), true
	}
	if textType(t) {
		return sqltemplate.Fragment("LENGTH2(" + expression + ")"), true
	}
	return "", false
}

// Visible separates an inaccessible/absent view from a visible unsupported
// schema. It never promises which grant or installation caused invisibility.
func Visible(schema Schema, suffix string) bool { view, _ := choose(schema, suffix); return view != "" }

func ValidateSchema(s Schema) error {
	for view := range s {
		switch view {
		case "GV_$CELL", "V_$CELL", "GV_$CELL_CONFIG", "V_$CELL_CONFIG", "GV_$CELL_CONFIG_INFO", "V_$CELL_CONFIG_INFO", "GV_$CELL_OPEN_ALERTS", "V_$CELL_OPEN_ALERTS":
		default:
			return errors.New("unexpected engineered-system dictionary relation")
		}
	}
	return nil
}

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

// ErrUnsupportedSource means the caller-admitted capability snapshot cannot
// select the requested fixed source. No discovery query or fallback is executed.
var ErrUnsupportedSource = errors.New("unsupported engineered-system source capability")

func validDatasetInstance(value session.Value) bool {
	if value.Null {
		return false
	}
	n, err := strconv.ParseInt(value.Text, 10, 64)
	return err == nil && n > 0
}

// Native LENGTH2/GETLENGTH uses UCS2 units. Preserve raw XML, including malformed
// XML evidence; this check establishes complete transport, not XML acceptance.
func completeDatasetPayload(payload, length session.Value) bool {
	if payload.Null {
		return length.Null
	}
	if length.Null {
		return false
	}
	want, err := strconv.ParseInt(length.Text, 10, 64)
	if err != nil || want < 0 {
		return false
	}
	actual := int64(0)
	for _, r := range payload.Text {
		actual++
		if r > 0xffff {
			actual++
		}
	}
	return actual == want
}

func optionalDatasetValue(row []session.Value, index map[string]int, name string, admitted bool) *session.Value {
	pos, ok := index[name]
	if !admitted || !ok {
		return nil
	}
	v := row[pos]
	return &v
}

const XMLTemplate = `SELECT {{.Instance}} AS INST_ID,
       {{.CellHash}} AS CELL_HASHVAL,
       {{.CellName}} AS CELL_NAME,
       {{.SourceType}} AS SOURCE_TYPE,
       {{.Payload}} AS XML_PAYLOAD,
       {{.NativeLength}} AS XML_LENGTH
FROM {{.View}} C`
