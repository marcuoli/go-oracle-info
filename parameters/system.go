package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/marcuoli/go-oracle-info/internal/legacyversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-sqltemplate/reference"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// SystemStatementWithOptions selects the same source as SystemStatement using one options value.
func SystemStatementWithOptions(options SourceOptions) (string, []any, error) {
	return SystemStatement(options.OracleMajor, options.Filter)
}

// DecodeSystemWithOptions selects the same source as DecodeSystem using one options value.
func DecodeSystemWithOptions(result session.Result, options SourceOptions) (SystemResult, error) {
	return DecodeSystem(result, options.OracleMajor, options.Filter)
}

// SystemRowsWithOptions selects the same source as SystemRows using one options value.
func SystemRowsWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions) (*sql.Rows, error) {
	return SystemRows(ctx, q, options.OracleMajor, options.Filter)
}

// SystemJSONWithOptions selects the same source as SystemJSON using one options value.
func SystemJSONWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	return SystemJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// SystemParameter is one GV$SYSTEM_PARAMETER row. VALUE retains the source's
// complete scalar-row text, even for list parameters; no Ordinal is fabricated.
// Numeric columns keep exact integer text. Optional nil fields mean unprojected
// 11g columns, distinct from projected SQL NULL. Flags are not normalized.
type SystemParameter struct {
	InstanceID           session.Value
	ContainerID          *session.Value
	Number               session.Value
	Name                 session.Value
	Type                 session.Value
	Value                session.Value
	DisplayValue         session.Value
	IsDefault            session.Value
	IsSessionModifiable  session.Value
	IsSystemModifiable   session.Value
	IsPDBModifiable      *session.Value
	IsInstanceModifiable session.Value
	IsModified           session.Value
	IsAdjusted           session.Value
	IsDeprecated         session.Value
	IsBasic              session.Value
	Description          session.Value
	UpdateComment        session.Value
}

// SystemResult preserves complete selected instance/container rows and source
// provenance. A nil Rows slice is not converted into a successful empty result.
type SystemResult struct {
	Provenance Provenance
	Rows       []SystemParameter
}

// SystemStatement selects instance-wide scalar-row values, not session values or
// SYSTEM_PARAMETER2 list-element rows. Empty Instances selects all qualified
// visible IDs; no implicit connected-instance restriction is introduced.
func SystemStatement(major int, filter Filter) (string, []any, error) {
	plan, err := systemPlan(major, filter)
	return plan.sql, plan.args, err
}

func systemPlan(major int, filter Filter) (selection, error) {
	base, err := effectivePlan(major, filter)
	if err != nil {
		return selection{}, err
	}
	columns := make([]rowset.Column, 0, len(base.schema)-1)
	for _, column := range base.schema {
		if column.Name != "ORDINAL" {
			columns = append(columns, column)
		}
	}
	base.provenance.Source = "GV$SYSTEM_PARAMETER"
	return build(major, base.provenance, columns, nil), nil
}

// DecodeSystem validates the full projection and every row before publishing
// selected scalar-row facts. Raw comma-containing/list VALUE text stays intact.
func DecodeSystem(result session.Result, major int, filter Filter) (SystemResult, error) {
	plan, err := systemPlan(major, filter)
	if err != nil {
		return SystemResult{}, err
	}
	return decodeSystem(result, plan)
}

// ReadSystem queries once through the existing admitted complete reader. Callers
// can enumerate visibility then read each instance with finite per-source budgets;
// scheduling, independent failures and aggregate coverage remain caller-owned.
func ReadSystem(ctx context.Context, reader Reader, request Request) (SystemResult, error) {
	plan, err := systemPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return SystemResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return SystemResult{}, err
	}
	facts, decodeErr := decodeSystem(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return SystemResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
}

// SystemRows returns the actual borrowed native cursor for SystemStatement.
// The caller owns iteration, terminal checks, Close, deadlines and stream budgets.
func SystemRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := systemPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// SystemJSON independently executes the same selection and delegates complete
// bounded consumption/finalization to the session rowset package, with no prefix.
func SystemJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := systemPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// ModifiedSystem selects exact non-NULL ISMODIFIED=MODIFIED from supplied facts.
// It does not prove SCOPE=MEMORY, non-persistence or SPFILE difference. Unknown,
// NULL and other flags remain in the original result. Selection copies mutable
// containers so it cannot mutate source evidence; unavailable nil Rows stays nil.
func ModifiedSystem(source SystemResult) SystemResult {
	provenance := source.Provenance
	provenance.Names = append([]string(nil), provenance.Names...)
	provenance.Instances = append([]int64(nil), provenance.Instances...)
	provenance.SIDs = append([]string(nil), provenance.SIDs...)
	selected := SystemResult{Provenance: provenance}
	if source.Rows == nil {
		return selected
	}
	selected.Rows = make([]SystemParameter, 0)
	for _, row := range source.Rows {
		if row.IsModified.Null || row.IsModified.Text != "MODIFIED" {
			continue
		}
		row.ContainerID = copySystemValue(row.ContainerID)
		row.IsPDBModifiable = copySystemValue(row.IsPDBModifiable)
		selected.Rows = append(selected.Rows, row)
	}
	return selected
}

func copySystemValue(value *session.Value) *session.Value {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func decodeSystem(result session.Result, plan selection) (SystemResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return SystemResult{}, err
	}
	rows := make([]SystemParameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = SystemParameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			Number: row[index["NUM"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Value: row[index["VALUE"]], DisplayValue: row[index["DISPLAY_VALUE"]], IsDefault: row[index["ISDEFAULT"]],
			IsSessionModifiable: row[index["ISSES_MODIFIABLE"]], IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]],
			IsPDBModifiable: optional(row, index, "ISPDB_MODIFIABLE"), IsInstanceModifiable: row[index["ISINSTANCE_MODIFIABLE"]],
			IsModified: row[index["ISMODIFIED"]], IsAdjusted: row[index["ISADJUSTED"]], IsDeprecated: row[index["ISDEPRECATED"]],
			IsBasic: row[index["ISBASIC"]], Description: row[index["DESCRIPTION"]], UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return SystemResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ScalarSystemScope controls which admitted container rows are selected on 12c+.
// Both options refer to the visible non-CDB rows before CON_ID existed.
type ScalarSystemScope uint8

const (
	ScalarSystemCurrentAndDatabase ScalarSystemScope = iota
	ScalarSystemDatabaseOnly
)

// ScalarSystemOptions carries the observed numeric version, scope and filters.
// The supplied handle already belongs to the desired database and container.
type ScalarSystemOptions struct {
	OracleVersion string
	Scope         ScalarSystemScope
	Filter        Filter
}

// ScalarSystemRequest adds a caller-owned SQL reference for an admitted reader.
type ScalarSystemRequest struct {
	Options   ScalarSystemOptions
	Reference reference.Reference
}

// ScalarSystemProvenance retains the selected scope and the observed version.
type ScalarSystemProvenance struct {
	Provenance
	OracleVersion string `json:"oracle_version"`
	Complete      bool   `json:"complete"`
}

// ScalarSystemParameter retains raw VALUE, including comma-containing values
// and SQL NULL. Pointer fields are nil only when their source column is absent.
type ScalarSystemParameter struct {
	InstanceID           session.Value
	ContainerID          *session.Value
	Number               session.Value
	Name                 session.Value
	Type                 session.Value
	Value                session.Value
	DisplayValue         *session.Value
	IsDefault            session.Value
	IsSessionModifiable  session.Value
	IsSystemModifiable   session.Value
	IsPDBModifiable      *session.Value
	IsInstanceModifiable *session.Value
	IsModified           session.Value
	IsAdjusted           session.Value
	IsDeprecated         *session.Value
	IsBasic              *session.Value
	Description          session.Value
	UpdateComment        session.Value
}

// ScalarSystemState distinguishes observed rows, successful empty results,
// inaccessible or unavailable sources, failed reads and unsupported releases.
type ScalarSystemState string

const (
	ScalarSystemAvailable    ScalarSystemState = "available"
	ScalarSystemNoRows       ScalarSystemState = "no_rows"
	ScalarSystemInaccessible ScalarSystemState = "inaccessible"
	ScalarSystemUnavailable  ScalarSystemState = "unavailable"
	ScalarSystemFailed       ScalarSystemState = "failed"
	ScalarSystemUnsupported  ScalarSystemState = "unsupported"
)

// ScalarSystemResult preserves selected scalar rows, diagnostics through the
// returned error, and scope. A successful empty result has a nonnil Rows slice.
type ScalarSystemResult struct {
	State      ScalarSystemState
	Provenance ScalarSystemProvenance
	Rows       []ScalarSystemParameter
}

// ErrInvalidScalarSystemScope rejects unknown scope choices before querying.
var ErrInvalidScalarSystemScope = errors.New("invalid scalar system parameter scope")

// ErrSourceInaccessible identifies a query rejected for insufficient privilege.
var ErrSourceInaccessible = projection.ErrSourceInaccessible

// ErrSourceUnavailable identifies an unresolved source or unavailable database
// state. The original query diagnostic remains in the error chain.
var ErrSourceUnavailable = projection.ErrSourceUnavailable

const scalarDatabaseRows = "whole_database_rows"

// ScalarSystemStatement selects the release-aware instance-wide scalar rows.
// VALUE and DISPLAY_VALUE remain independent when both columns are present.
func ScalarSystemStatement(options ScalarSystemOptions) (string, []any, error) {
	plan, err := scalarSystemPlan(options)
	return plan.sql, plan.args, err
}

func scalarSystemPlan(options ScalarSystemOptions) (selection, error) {
	major, ok := legacyversion.Major(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	if options.Scope != ScalarSystemCurrentAndDatabase && options.Scope != ScalarSystemDatabaseOnly {
		return selection{}, ErrInvalidScalarSystemScope
	}

	// Reuse the published projection. 10g adds the display and two flag
	// columns. The basic-parameter flag is first documented in 11g.
	plan, err := systemPlan(max(major, 11), options.Filter)
	if err != nil {
		return selection{}, err
	}
	switch {
	case major < 11:
		columns := slices.DeleteFunc(plan.schema, func(column rowset.Column) bool {
			switch column.Name {
			case "ISBASIC":
				return true
			case "DISPLAY_VALUE", "ISINSTANCE_MODIFIABLE", "ISDEPRECATED":
				return major < 10
			}
			return false
		})
		plan.provenance.OracleMajor = major
		plan = build(major, plan.provenance, columns, nil)
	case major >= 12 && options.Scope == ScalarSystemDatabaseOnly:
		plan.provenance.Scope = scalarDatabaseRows
		plan = build(major, plan.provenance, plan.schema, nil)
	}
	plan.classifyAccess = true
	return plan, nil
}

func scalarSystemFailure(err error) ScalarSystemResult {
	state := ScalarSystemFailed
	if errors.Is(err, ErrUnsupportedVersion) {
		state = ScalarSystemUnsupported
	}
	return ScalarSystemResult{State: state}
}

// DecodeScalarSystem validates the full projection and every row before
// publishing any typed facts. A pure decoder cannot establish transport completion.
func DecodeScalarSystem(result session.Result, options ScalarSystemOptions) (ScalarSystemResult, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return scalarSystemFailure(err), err
	}
	return decodeScalarSystem(result, plan, options.OracleVersion)
}

func decodeScalarSystem(result session.Result, plan selection, version string) (ScalarSystemResult, error) {
	facts := ScalarSystemResult{State: ScalarSystemFailed, Provenance: ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: version}}
	index, err := parameterColumns(result, plan)
	if err != nil {
		return facts, err
	}
	facts.Rows = make([]ScalarSystemParameter, len(result.Rows))
	facts.State = ScalarSystemNoRows
	if len(facts.Rows) > 0 {
		facts.State = ScalarSystemAvailable
	}
	for i, row := range result.Rows {
		facts.Rows[i] = ScalarSystemParameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			Number: row[index["NUM"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Value: row[index["VALUE"]], DisplayValue: optional(row, index, "DISPLAY_VALUE"), IsDefault: row[index["ISDEFAULT"]],
			IsSessionModifiable: row[index["ISSES_MODIFIABLE"]], IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]],
			IsPDBModifiable: optional(row, index, "ISPDB_MODIFIABLE"), IsInstanceModifiable: optional(row, index, "ISINSTANCE_MODIFIABLE"),
			IsModified: row[index["ISMODIFIED"]], IsAdjusted: row[index["ISADJUSTED"]], IsDeprecated: optional(row, index, "ISDEPRECATED"),
			IsBasic: optional(row, index, "ISBASIC"), Description: row[index["DESCRIPTION"]], UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return facts, nil
}

// ReadScalarSystem queries the admitted complete reader once and discards all
// returned rows on query, decode or cancellation errors.
func ReadScalarSystem(ctx context.Context, reader Reader, request ScalarSystemRequest) (ScalarSystemResult, error) {
	plan, err := scalarSystemPlan(request.Options)
	if err != nil {
		return scalarSystemFailure(err), err
	}
	failure := ScalarSystemResult{State: ScalarSystemFailed, Provenance: ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: request.Options.OracleVersion}}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		switch {
		case errors.Is(err, ErrSourceInaccessible):
			failure.State = ScalarSystemInaccessible
		case errors.Is(err, ErrSourceUnavailable):
			failure.State = ScalarSystemUnavailable
		}
		return failure, err
	}
	facts, decodeErr := decodeScalarSystem(result, plan, request.Options.OracleVersion)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return failure, &Error{Source: plan.provenance.Source, cause: err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ScalarSystemRows returns the actual caller-native cursor. The caller owns
// iteration, terminal checks, Close, deadlines and stream budgets.
func ScalarSystemRows(ctx context.Context, q rowset.Queryer, options ScalarSystemOptions) (*sql.Rows, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ScalarSystemJSON independently executes the same selection and publishes only
// a complete bounded JSON envelope, with version and scope kept as provenance.
func ScalarSystemJSON(ctx context.Context, q rowset.Queryer, options ScalarSystemOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return nil, err
	}
	provenance := ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion, Complete: true}
	return readParameterJSON(ctx, q, plan, provenance, limits)
}
