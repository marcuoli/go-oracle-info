package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Parameter retains each effective list-element row without normalizing flags.
// Numeric fields retain exact integer text in session.Value. ContainerID and
// IsPDBModifiable are nil only when their columns were not selected on 11g;
// projected SQL NULL is a nonnil value with Null=true. Value/DisplayValue are text.
type Parameter struct {
	InstanceID           session.Value
	ContainerID          *session.Value
	Number               session.Value
	Name                 session.Value
	Type                 session.Value
	Ordinal              session.Value
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

// EffectiveResult preserves complete selected row order, duplicates and provenance.
// A successful empty Rows slice is not evidence of unobserved parameter defaults.
type EffectiveResult struct {
	Provenance Provenance
	Rows       []Parameter
}

// DecodeEffective validates the entire admitted projection before publishing rows.
func DecodeEffective(result session.Result, major int, filter Filter) (EffectiveResult, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	return decodeEffective(result, plan)
}

// ReadEffective queries once through the admitted reader, with no connection or
// retry ownership, then decodes only a complete successful result.
func ReadEffective(ctx context.Context, reader Reader, request Request) (EffectiveResult, error) {
	plan, err := effectivePlan(request.OracleMajor, request.Filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	facts, decodeErr := decodeEffective(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return EffectiveResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
}

func decodeEffective(result session.Result, plan selection) (EffectiveResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	rows := make([]Parameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Parameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			Number: row[index["NUM"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Ordinal: row[index["ORDINAL"]], Value: row[index["VALUE"]], DisplayValue: row[index["DISPLAY_VALUE"]],
			IsDefault: row[index["ISDEFAULT"]], IsSessionModifiable: row[index["ISSES_MODIFIABLE"]],
			IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]], IsPDBModifiable: optional(row, index, "ISPDB_MODIFIABLE"),
			IsInstanceModifiable: row[index["ISINSTANCE_MODIFIABLE"]], IsModified: row[index["ISMODIFIED"]],
			IsAdjusted: row[index["ISADJUSTED"]], IsDeprecated: row[index["ISDEPRECATED"]], IsBasic: row[index["ISBASIC"]],
			Description: row[index["DESCRIPTION"]], UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return EffectiveResult{Provenance: plan.provenance, Rows: rows}, nil
}

// EffectiveRows returns the caller's actual native cursor without materializing,
// wrapping or capping its SQL. The caller owns Next/Scan/Err/Close and budgets.
func EffectiveRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := effectivePlan(major, filter)
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

// EffectiveStatement selects instance-effective SYSTEM_PARAMETER2 values, not
// session PARAMETER values or stored definitions. CON_ID12+ is explicitly scoped
// to whole-database rows(0) plus the current session container, never other PDBs.
func EffectiveStatement(major int, filter Filter) (string, []any, error) {
	plan, err := effectivePlan(major, filter)
	return plan.sql, plan.args, err
}

func effectivePlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	if err := validate(filter.Names, filter.Instances, nil); err != nil {
		return selection{}, err
	}
	provenance := source("GV$SYSTEM_PARAMETER2", major, filter.Names, filter.Instances)
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	for _, name := range []string{"NUM", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISDEFAULT", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE"} {
		kind := rowset.Text
		if name == "NUM" || name == "TYPE" || name == "ORDINAL" {
			kind = rowset.Integer
		}
		columns = append(columns, rowset.Column{Name: name, Kind: kind})
	}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "ISPDB_MODIFIABLE", Kind: rowset.Text})
	}
	for _, name := range []string{"ISINSTANCE_MODIFIABLE", "ISMODIFIED", "ISADJUSTED", "ISDEPRECATED", "ISBASIC", "DESCRIPTION", "UPDATE_COMMENT"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	return build(major, provenance, columns, nil), nil
}
