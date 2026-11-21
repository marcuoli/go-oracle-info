package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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
