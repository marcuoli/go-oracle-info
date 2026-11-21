package parameters

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// JSONEnvelope keeps source/filter provenance separate from ordered native data.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Error avoids formatting driver errors, SQL or bind data. Unwrap retains the
// cause for errors.Is/As and deliberately private diagnostics.
type Error struct {
	Source string
	cause  error
}

// Error returns only the static source, never SQL, bindings or driver text.
func (e *Error) Error() string { return "parameter read failed (" + e.Source + ")" }

// Unwrap retains reported causes for explicit private inspection.
func (e *Error) Unwrap() error { return e.cause }

// EffectiveRows returns the caller's actual native cursor without materializing,
// wrapping or capping its SQL. The caller owns Next/Scan/Err/Close and budgets.
func EffectiveRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HiddenRows returns the actual caller-owned cursor for visible underscore
// instance parameters. It does not scan X$ or filter out default-state facts.
func HiddenRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HiddenJSON is the bounded independent JSON counterpart of HiddenRows. Empty
// selected names remain absent; no unobserved internal defaults are invented.
func HiddenJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// SPFileRows preserves all selected stored clauses, wildcard/defaults and raw
// ISSPECIFIED. Its native cursor has the same ownership as EffectiveRows.
func SPFileRows(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter) (*sql.Rows, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// EffectiveJSON executes EffectiveRows' exact statement/filter on its own
// cursor. It publishes nothing until terminal/close/context/budget checks pass.
func EffectiveJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// SPFileJSON is the independent JSON counterpart of SPFileRows. No value
// precedence, SPFILE-use or physical-file-completeness inference is performed.
func SPFileJSON(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	return rows, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, plan)
	}, plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(&Error{Source: plan.provenance.Source, cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := parameterColumns(result, plan)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{Source: plan.provenance.Source, cause: cause}
	}, "limits", "query", "validate")
}

// SourceOptions carries the source selectors for the options-based fact APIs.
type SourceOptions struct {
	OracleMajor int
	Filter      Filter
}

// EffectiveStatementWithOptions selects the same source as EffectiveStatement using one options value.
func EffectiveStatementWithOptions(options SourceOptions) (string, []any, error) {
	return EffectiveStatement(options.OracleMajor, options.Filter)
}

// DecodeEffectiveWithOptions selects the same source as DecodeEffective using one options value.
func DecodeEffectiveWithOptions(result session.Result, options SourceOptions) (EffectiveResult, error) {
	return DecodeEffective(result, options.OracleMajor, options.Filter)
}

// EffectiveRowsWithOptions selects the same source as EffectiveRows using one options value.
func EffectiveRowsWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions) (*sql.Rows, error) {
	return EffectiveRows(ctx, q, options.OracleMajor, options.Filter)
}

// EffectiveJSONWithOptions selects the same source as EffectiveJSON using one options value.
func EffectiveJSONWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	return EffectiveJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// HiddenStatementWithOptions selects the same source as HiddenStatement using one options value.
func HiddenStatementWithOptions(options SourceOptions) (string, []any, error) {
	return HiddenStatement(options.OracleMajor, options.Filter)
}

// DecodeHiddenWithOptions selects the same source as DecodeHidden using one options value.
func DecodeHiddenWithOptions(result session.Result, options SourceOptions) (EffectiveResult, error) {
	return DecodeHidden(result, options.OracleMajor, options.Filter)
}

// HiddenRowsWithOptions selects the same source as HiddenRows using one options value.
func HiddenRowsWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions) (*sql.Rows, error) {
	return HiddenRows(ctx, q, options.OracleMajor, options.Filter)
}

// HiddenJSONWithOptions selects the same source as HiddenJSON using one options value.
func HiddenJSONWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	return HiddenJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

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

// HiddenCatalogOptions carries the source selectors for the options-based fact APIs.
type HiddenCatalogOptions struct {
	OracleMajor int
	Filter      HiddenCatalogFilter
}

// HiddenCatalogStatementWithOptions selects the same source as HiddenCatalogStatement using one options value.
func HiddenCatalogStatementWithOptions(options HiddenCatalogOptions) (string, []any, error) {
	return HiddenCatalogStatement(options.OracleMajor, options.Filter)
}

// DecodeHiddenCatalogWithOptions selects the same source as DecodeHiddenCatalog using one options value.
func DecodeHiddenCatalogWithOptions(result session.Result, options HiddenCatalogOptions) (HiddenCatalogResult, error) {
	return DecodeHiddenCatalog(result, options.OracleMajor, options.Filter)
}

// HiddenCatalogRowsWithOptions selects the same source as HiddenCatalogRows using one options value.
func HiddenCatalogRowsWithOptions(ctx context.Context, q rowset.Queryer, options HiddenCatalogOptions) (*sql.Rows, error) {
	return HiddenCatalogRows(ctx, q, options.OracleMajor, options.Filter)
}

// HiddenCatalogJSONWithOptions selects the same source as HiddenCatalogJSON using one options value.
func HiddenCatalogJSONWithOptions(ctx context.Context, q rowset.Queryer, options HiddenCatalogOptions, limits rowset.Limits) (json.RawMessage, error) {
	return HiddenCatalogJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// HiddenContextOptions carries the source selectors for the options-based fact APIs.
type HiddenContextOptions struct {
	OracleMajor int
	Filter      HiddenContextFilter
}

// HiddenContextStatementWithOptions selects the same source as HiddenContextStatement using one options value.
func HiddenContextStatementWithOptions(options HiddenContextOptions) (string, []any, error) {
	return HiddenContextStatement(options.OracleMajor, options.Filter)
}

// DecodeHiddenContextWithOptions selects the same source as DecodeHiddenContext using one options value.
func DecodeHiddenContextWithOptions(result session.Result, options HiddenContextOptions) (HiddenContextResult, error) {
	return DecodeHiddenContext(result, options.OracleMajor, options.Filter)
}

// HiddenContextRowsWithOptions selects the same source as HiddenContextRows using one options value.
func HiddenContextRowsWithOptions(ctx context.Context, q rowset.Queryer, options HiddenContextOptions) (*sql.Rows, error) {
	return HiddenContextRows(ctx, q, options.OracleMajor, options.Filter)
}

// HiddenContextJSONWithOptions selects the same source as HiddenContextJSON using one options value.
func HiddenContextJSONWithOptions(ctx context.Context, q rowset.Queryer, options HiddenContextOptions, limits rowset.Limits) (json.RawMessage, error) {
	return HiddenContextJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// SPFileOptions carries the source selectors for the options-based fact APIs.
type SPFileOptions struct {
	OracleMajor int
	Filter      SPFileFilter
}

// SPFileStatementWithOptions selects the same source as SPFileStatement using one options value.
func SPFileStatementWithOptions(options SPFileOptions) (string, []any, error) {
	return SPFileStatement(options.OracleMajor, options.Filter)
}

// DecodeSPFileWithOptions selects the same source as DecodeSPFile using one options value.
func DecodeSPFileWithOptions(result session.Result, options SPFileOptions) (SPFileResult, error) {
	return DecodeSPFile(result, options.OracleMajor, options.Filter)
}

// SPFileRowsWithOptions selects the same source as SPFileRows using one options value.
func SPFileRowsWithOptions(ctx context.Context, q rowset.Queryer, options SPFileOptions) (*sql.Rows, error) {
	return SPFileRows(ctx, q, options.OracleMajor, options.Filter)
}

// SPFileJSONWithOptions selects the same source as SPFileJSON using one options value.
func SPFileJSONWithOptions(ctx context.Context, q rowset.Queryer, options SPFileOptions, limits rowset.Limits) (json.RawMessage, error) {
	return SPFileJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}
