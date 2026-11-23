package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"

	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader is an already-owned, bounded query interface. Its Result must be complete
// after terminal iteration/close checks; capacity errors must not return a prefix.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies admitted scope and distinct caller-namespaced SQL references.
// Known Oracle 18+ connected PDBs require a second reference for the guarded
// effective query; other contexts do not evaluate that SQL.
type Request struct {
	Parameter          Parameter
	Options            Options
	PrimaryReference   reference.Reference
	EffectiveReference reference.Reference
}

// ReadError retains a programmatic cause without exposing driver/bind text.
type ReadError struct {
	Parameter Parameter
	Effective bool
	cause     error
}

func (e *ReadError) Error() string {
	if e.Effective {
		return "configured-memory effective-source query failed"
	}
	return "configured-memory primary-source query failed"
}

func (e *ReadError) Unwrap() error { return e.cause }

// ErrNoRows can coexist with a terminal failure in an error tree. Normalize it
// only when every cause is a no-row outcome, including ordinary wrapped adapters.
func onlyNoRows(err error) bool {
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		causes := cause.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, child := range causes {
			if !onlyNoRows(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return onlyNoRows(cause.Unwrap())
	default:
		return errors.Is(err, sql.ErrNoRows)
	}
}

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

// ErrInvalidObservedEvidence rejects invalid allocation or component facts.
var ErrInvalidObservedEvidence = errors.New("invalid observed Oracle memory evidence")

// The connected allocation projections retain their reviewed version families.
func supportsAllocation(major int) bool {
	return major == 19 || major == 21 || major == 23
}

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

func datasetColumns(result session.Result, p datasetSelection, integerFields ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range integerFields {
			v := row[index[name]]
			if !v.Null && !numtext.IsUnsignedInteger(v.Text) {
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

func readAllocationJSON(ctx context.Context, q rowset.Queryer, p datasetSelection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidObservedEvidence, validate),
	})
	return payload, projection.WrapMovedEnvelopeError(err, func(cause error) error {
		return &DatasetError{p.provenance.Source, cause}
	}, "limits", "header")
}

// State separates a known value, SQL NULL, no row and incomplete evidence.
type State string

// Origin identifies configured parameter scope, never observed consumption.
type Origin string

// Observation owns source-rounded GiB text and source/target/execution identity.
// An inherited value is not a PDB-specific allocation or a cluster total.
type Observation struct {
	State             State
	ValueGiB          string
	Parameter         string
	Origin            Origin
	SourceContainerID int
	// CurrentContainerID is the actual execution container. A root-discovered
	// PDB read keeps root (1) here and the admitted PDB in TargetContainerID.
	CurrentContainerID int
	TargetContainerID  int
	InstanceID         int
}

func resultRow(result session.Result) (map[string]session.Value, bool, error) {
	if len(result.Columns) != len(columns) || len(result.Rows) > 1 {
		return nil, false, ErrInvalidEvidence
	}
	index := make(map[string]int, len(columns))
	for i, name := range result.Columns {
		name = strings.ToLower(name)
		if _, exists := index[name]; name == "" || exists {
			return nil, false, ErrInvalidEvidence
		}
		index[name] = i
	}
	for _, name := range columns {
		if _, ok := index[name]; !ok {
			return nil, false, ErrInvalidEvidence
		}
	}
	if len(result.Rows) == 0 {
		return nil, true, nil
	}
	if len(result.Rows[0]) != len(columns) {
		return nil, false, ErrInvalidEvidence
	}
	row := make(map[string]session.Value, len(columns))
	for name, i := range index {
		row[name] = result.Rows[0][i]
	}
	return row, false, nil
}

func integerCell(value session.Value) (int, bool) {
	number, err := strconv.Atoi(value.Text)
	return number, err == nil && !value.Null
}

func validDecimal(value string) bool {
	if len(value) > 128 || value == "" || value == "." {
		return false
	}
	digits := strings.Replace(value, ".", "", 1)
	return strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func zeroDecimal(value string) bool { return strings.Trim(value, "0.") == "" }

// Parameter selects a fixed configured-memory family, never arbitrary SQL.
type Parameter string

// ErrUnsupportedParameter rejects names outside the two configured families.
var ErrUnsupportedParameter = errors.New("unsupported configured-memory parameter")

func numeric(expression string) string {
	return "TO_CHAR(" + expression + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}

// Scope describes the admitted database context, not permission to acquire one.
type Scope string

// Options selects fixed release/container variants. TargetContainerID is required
// for root-discovered PDBs and optionally pins a connected PDB's identity.
type Options struct {
	Scope             Scope
	OracleMajor       int
	TargetContainerID int
	PDB               PDBOptions
}

func validateOptions(o Options) error {
	if o.OracleMajor < 9 {
		return ErrUnsupportedScope
	}
	switch o.Scope {
	case NonCDB:
		if o.TargetContainerID != 0 || o.PDB.EffectiveADB {
			return ErrUnsupportedScope
		}
	case CDBRoot:
		if o.OracleMajor < 12 || o.TargetContainerID != 0 || o.PDB.EffectiveADB {
			return ErrUnsupportedScope
		}
	case ConnectedPDB, DiscoveredPDB:
		if o.OracleMajor < 12 || (o.TargetContainerID != 0 && o.TargetContainerID <= 2) {
			return ErrUnsupportedScope
		}
		if o.Scope == DiscoveredPDB && (o.TargetContainerID <= 2 || o.PDB.EffectiveADB) {
			return ErrUnsupportedScope
		}
		if o.PDB.EffectiveADB && o.OracleMajor < 18 {
			return ErrUnsupportedScope
		}
	default:
		return ErrUnsupportedScope
	}
	return nil
}

// SourceRequest admits one configured-memory source, not the Collect fallback
// sequence. EffectiveADB requires the caller-established primary NULL/no-row
// precondition and a known18+ connected PDB, as with PDBStatement.
type SourceRequest struct {
	Parameter Parameter
	Options   Options
	Reference reference.Reference
}

func memorySelection(text, source string, parameter Parameter, options Options, names []string) datasetSelection {
	schema := make([]rowset.Column, len(names))
	for i, name := range names {
		schema[i] = rowset.Column{Name: name, Kind: rowset.Text}
	}
	var args []any
	if options.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", options.TargetContainerID)}
	}
	return datasetSelection{sql: text, args: args, schema: schema, provenance: DatasetProvenance{Source: source, OracleMajor: options.OracleMajor, Scope: string(options.Scope), Parameter: parameter, TargetContainerID: options.TargetContainerID, AllowInherited: options.PDB.AllowInherited, EffectiveADB: options.PDB.EffectiveADB}}
}

// ParameterOptions admits a configured-memory setting and its source scope.
type ParameterOptions struct {
	Parameter Parameter
	Options   Options
}
