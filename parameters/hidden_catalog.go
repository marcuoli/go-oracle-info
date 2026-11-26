package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// HiddenCatalogFilter selects fixed-table underscore parameters. Empty Names and
// NameContains select all exposed rows, including defaults. Names are exact;
// NameContains is a case-insensitive literal ASCII substring, not a LIKE pattern.
// This connected-session source cannot select another reporting instance.
type HiddenCatalogFilter struct {
	Names        []string
	NameContains string
}

// HiddenCatalogRequest supplies the observed source version and caller reference.
// The reader must already have permission to read all three SYS fixed tables.
type HiddenCatalogRequest struct {
	OracleMajor int
	Filter      HiddenCatalogFilter
	Reference   reference.Reference
}

// HiddenParameter keeps session and instance values distinct. Values remain raw
// text, including numeric-looking text, or SQL NULL; modifiability is the query's
// decoded flag, not proof that changing a parameter is safe or supported.
type HiddenParameter struct {
	Name                session.Value
	SessionValue        session.Value
	InstanceValue       session.Value
	IsSessionModifiable session.Value
	IsSystemModifiable  session.Value
	Description         session.Value
}

// HiddenCatalogResult is a complete selected observation of the connected
// session/instance. It does not establish RAC or cross-container completeness.
type HiddenCatalogResult struct {
	Provenance Provenance
	Rows       []HiddenParameter
}

// HiddenParameterWithSessionDefault includes the raw KSPPSTDF flag belonging
// to the session value. Preserve SQL NULL and unrecognized flag text.
type HiddenParameterWithSessionDefault struct {
	HiddenParameter
	SessionIsDefault session.Value
}

// HiddenCatalogWithSessionDefaultResult is the session and instance catalog
// with the additional session-side default flag.
type HiddenCatalogWithSessionDefaultResult struct {
	Provenance Provenance
	Rows       []HiddenParameterWithSessionDefault
}

const hiddenCatalogSource = "SYS.X$KSPPI+SYS.X$KSPPCV+SYS.X$KSPPSV"

// HiddenCatalogStatement selects the explicit internal fixed-table catalog.
// This is independent of HiddenStatement's visible SYSTEM_PARAMETER2 dataset;
// there is no fallback or privilege escalation when the source is unavailable.
func HiddenCatalogStatement(major int, filter HiddenCatalogFilter) (string, []any, error) {
	plan, err := hiddenCatalogPlan(major, filter, false)
	return plan.sql, plan.args, err
}

func hiddenCatalogPlan(major int, filter HiddenCatalogFilter, includeSessionDefault bool) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	if err := validate(filter.Names, nil, nil); err != nil {
		return selection{}, err
	}
	for _, name := range filter.Names {
		if !strings.HasPrefix(name, "_") {
			return selection{}, ErrInvalidFilter
		}
	}
	if len(filter.NameContains) > 80 {
		return selection{}, ErrInvalidFilter
	}
	// Parameter names use ASCII. Limit only the substring selector so its case
	// folding has the same meaning on the legacy and modern Oracle sources.
	for _, r := range filter.NameContains {
		if r < 32 || r > 126 {
			return selection{}, ErrInvalidFilter
		}
	}
	columns := []rowset.Column{}
	for _, name := range []string{"NAME", "SESSION_VALUE", "INSTANCE_VALUE", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE", "DESCRIPTION"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	sql := `SELECT a.KSPPINM AS NAME, b.KSPPSTVL AS SESSION_VALUE, c.KSPPSTVL AS INSTANCE_VALUE,
 DECODE(BITAND(a.KSPPIFLG / 256, 1), 1, 'TRUE', 'FALSE') AS ISSES_MODIFIABLE,
 DECODE(BITAND(a.KSPPIFLG / 65536, 3), 1, 'IMMEDIATE', 2, 'DEFERRED', 3, 'IMMEDIATE', 'FALSE') AS ISSYS_MODIFIABLE,
 a.KSPPDESC AS DESCRIPTION`
	if includeSessionDefault {
		sql += ", b.KSPPSTDF AS SESSION_IS_DEFAULT"
		columns = append(columns, rowset.Column{Name: "SESSION_IS_DEFAULT", Kind: rowset.Text})
	}
	sql += `
 FROM SYS.X$KSPPI a
 JOIN SYS.X$KSPPCV b ON a.INDX = b.INDX
 JOIN SYS.X$KSPPSV c ON a.INDX = c.INDX
 WHERE NLSSORT(SUBSTR(a.KSPPINM,1,1), 'NLS_SORT=BINARY') = NLSSORT('_', 'NLS_SORT=BINARY')`
	args := []any{}
	if len(filter.Names) > 0 {
		binds := make([]string, len(filter.Names))
		for i, name := range filter.Names {
			args = append(args, name)
			binds[i] = "NLSSORT(:" + strconv.Itoa(len(args)) + ", 'NLS_SORT=BINARY')"
		}
		sql += " AND NLSSORT(a.KSPPINM, 'NLS_SORT=BINARY') IN (" + strings.Join(binds, ",") + ")"
	}
	if filter.NameContains != "" {
		args = append(args, strings.ToLower(filter.NameContains))
		sql += " AND INSTR(LOWER(a.KSPPINM), :" + strconv.Itoa(len(args)) + ") > 0"
	}
	sql += " ORDER BY NLSSORT(a.KSPPINM, 'NLS_SORT=BINARY')"
	provenance := Provenance{Source: hiddenCatalogSource, OracleMajor: major, Scope: "connected_session_and_instance", Names: append([]string(nil), filter.Names...), NameClass: "leading_underscore_only", NameContains: filter.NameContains}
	return selection{sql: sql, args: args, schema: columns, provenance: provenance}, nil
}

// DecodeHiddenCatalog validates every row before returning any typed facts.
// SQL NULL, literal NULL text and different session/instance values stay distinct.
func DecodeHiddenCatalog(result session.Result, major int, filter HiddenCatalogFilter) (HiddenCatalogResult, error) {
	plan, err := hiddenCatalogPlan(major, filter, false)
	if err != nil {
		return HiddenCatalogResult{}, err
	}
	return decodeHiddenCatalog(result, plan)
}

func decodeHiddenCatalog(result session.Result, plan selection) (HiddenCatalogResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return HiddenCatalogResult{}, err
	}
	rows := make([]HiddenParameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = hiddenParameter(row, index)
	}
	return HiddenCatalogResult{Provenance: plan.provenance, Rows: rows}, nil
}

func hiddenParameter(row []session.Value, index map[string]int) HiddenParameter {
	return HiddenParameter{
		Name: row[index["NAME"]], SessionValue: row[index["SESSION_VALUE"]],
		InstanceValue: row[index["INSTANCE_VALUE"]], Description: row[index["DESCRIPTION"]],
		IsSessionModifiable: row[index["ISSES_MODIFIABLE"]], IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]],
	}
}

// ReadHiddenCatalog executes once through an admitted complete buffered reader.
// The caller owns deadlines, finite transport budgets and session lifetime.
// Query/terminal/cancellation failures publish no successful prefix or fallback.
func ReadHiddenCatalog(ctx context.Context, reader Reader, request HiddenCatalogRequest) (HiddenCatalogResult, error) {
	plan, err := hiddenCatalogPlan(request.OracleMajor, request.Filter, false)
	if err != nil {
		return HiddenCatalogResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return HiddenCatalogResult{}, err
	}
	facts, decodeErr := decodeHiddenCatalog(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return HiddenCatalogResult{}, &Error{Source: hiddenCatalogSource, cause: err}
	}
	return facts, nil
}

// HiddenCatalogRows returns the actual caller-native cursor. The caller owns
// Next/Scan/Err/Close, budgets and session affinity; the SQL does not truncate.
func HiddenCatalogRows(ctx context.Context, q rowset.Queryer, major int, filter HiddenCatalogFilter) (*sql.Rows, error) {
	plan, err := hiddenCatalogPlan(major, filter, false)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HiddenCatalogJSON reads the same selection on its own cursor using shared
// rowset lifecycle/budgets. It publishes no bytes on iteration/close/cancel/error.
func HiddenCatalogJSON(ctx context.Context, q rowset.Queryer, major int, filter HiddenCatalogFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := hiddenCatalogPlan(major, filter, false)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// HiddenCatalogWithSessionDefaultStatement selects the legacy catalog columns
// followed by the raw SESSION_IS_DEFAULT column.
func HiddenCatalogWithSessionDefaultStatement(major int, filter HiddenCatalogFilter) (string, []any, error) {
	plan, err := hiddenCatalogPlan(major, filter, true)
	return plan.sql, plan.args, err
}

// DecodeHiddenCatalogWithSessionDefault validates the full extended projection.
func DecodeHiddenCatalogWithSessionDefault(result session.Result, major int, filter HiddenCatalogFilter) (HiddenCatalogWithSessionDefaultResult, error) {
	plan, err := hiddenCatalogPlan(major, filter, true)
	if err != nil {
		return HiddenCatalogWithSessionDefaultResult{}, err
	}
	return decodeHiddenCatalogWithSessionDefault(result, plan)
}

func decodeHiddenCatalogWithSessionDefault(result session.Result, plan selection) (HiddenCatalogWithSessionDefaultResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return HiddenCatalogWithSessionDefaultResult{}, err
	}
	rows := make([]HiddenParameterWithSessionDefault, len(result.Rows))
	flag := index["SESSION_IS_DEFAULT"]
	for i, row := range result.Rows {
		rows[i] = HiddenParameterWithSessionDefault{
			HiddenParameter:  hiddenParameter(row, index),
			SessionIsDefault: row[flag],
		}
	}
	return HiddenCatalogWithSessionDefaultResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadHiddenCatalogWithSessionDefault reads the extended catalog once on the
// admitted session. The caller provides complete results under finite budgets.
func ReadHiddenCatalogWithSessionDefault(ctx context.Context, reader Reader, request HiddenCatalogRequest) (HiddenCatalogWithSessionDefaultResult, error) {
	plan, err := hiddenCatalogPlan(request.OracleMajor, request.Filter, true)
	if err != nil {
		return HiddenCatalogWithSessionDefaultResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return HiddenCatalogWithSessionDefaultResult{}, err
	}
	facts, decodeErr := decodeHiddenCatalogWithSessionDefault(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return HiddenCatalogWithSessionDefaultResult{}, &Error{Source: hiddenCatalogSource, cause: err}
	}
	return facts, nil
}

// HiddenCatalogWithSessionDefaultRows returns the extended caller-native cursor.
func HiddenCatalogWithSessionDefaultRows(ctx context.Context, q rowset.Queryer, major int, filter HiddenCatalogFilter) (*sql.Rows, error) {
	plan, err := hiddenCatalogPlan(major, filter, true)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HiddenCatalogWithSessionDefaultJSON returns the complete bounded extended catalog.
func HiddenCatalogWithSessionDefaultJSON(ctx context.Context, q rowset.Queryer, major int, filter HiddenCatalogFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := hiddenCatalogPlan(major, filter, true)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
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
