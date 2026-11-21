package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// HiddenContextFilter selects exact parameter names without requiring a leading
// underscore. NonDefaultOnly applies literal raw KSPFTCTXDF <> 'TRUE', excluding
// SQL NULL; it does not infer effective instance/session or stored SPFILE values.
// A zero filter retains all visible context rows, including defaults.
type HiddenContextFilter struct {
	Names          []string
	NonDefaultOnly bool
}

// HiddenContextRequest supplies an observed release, stable execution reference
// and finite materialization budgets. Its admitted reader must also enforce
// transport budgets and preserve terminal/close failures through complete EOF.
type HiddenContextRequest struct {
	OracleMajor int
	Filter      HiddenContextFilter
	Reference   reference.Reference
	Limits      rowset.Limits
}

// HiddenContextValue retains one X$KSPPCV2 context row. ContextRowIndex,
// ParameterNumber and ValueNumber preserve raw INDX/KSPFTCTXPN/KSPFTCTXVN identity;
// ValueNumber is not asserted to be a SYSTEM_PARAMETER2 list ordinal. Repeated
// names stay separate. IsDefault is raw KSPFTCTXDF, including NULL/unknown text.
// ContainerID nil means unprojected on 11g; nonnil Null is projected SQL NULL.
type HiddenContextValue struct {
	InstanceID      session.Value
	ContainerID     *session.Value
	ContextRowIndex session.Value
	ParameterNumber session.Value
	ValueNumber     session.Value
	Name            session.Value
	Value           session.Value
	IsDefault       session.Value
	Description     session.Value
}

// HiddenContextProvenance describes only the connected instance's internal
// context rows. The caller must enumerate every qualified reachable reporting
// member, admit a separate member-affine handle and record missing/failed members.
// Visible membership and a successful local read never establish complete RAC.
type HiddenContextProvenance struct {
	Source         string   `json:"source"`
	OracleMajor    int      `json:"oracle_major"`
	Scope          string   `json:"scope"`
	Names          []string `json:"names,omitempty"`
	NonDefaultOnly bool     `json:"nondefault_only"`
}

// HiddenContextResult is one complete admitted local observation, not an all-RAC
// aggregate. An empty nonnil Rows slice means successful empty selection only.
type HiddenContextResult struct {
	Provenance HiddenContextProvenance
	Rows       []HiddenContextValue
}

type hiddenContextSelection struct {
	selection
	provenance HiddenContextProvenance
}

const hiddenContextSource = "SYS.X$KSPPI+SYS.X$KSPPCV2"

// HiddenContextStatement selects the distinct internal context source with its
// INDX+1 join. Availability must be admitted for the actual release/container;
// no fallback to the underscore catalog or effective/session values is attempted.
func HiddenContextStatement(major int, filter HiddenContextFilter) (string, []any, error) {
	plan, err := hiddenContextPlan(major, filter)
	return plan.sql, plan.args, err
}
func hiddenContextPlan(major int, filter HiddenContextFilter) (hiddenContextSelection, error) {
	if major < 11 {
		return hiddenContextSelection{}, ErrUnsupportedVersion
	}
	if err := validate(filter.Names, nil, nil); err != nil {
		return hiddenContextSelection{}, err
	}
	expressions := []string{"b.INST_ID AS INST_ID"}
	schema := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	if major >= 12 {
		expressions = append(expressions, "b.CON_ID AS CON_ID")
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	expressions = append(expressions, "b.INDX AS CONTEXT_ROW_INDEX", "b.KSPFTCTXPN AS PARAMETER_NUMBER", "b.KSPFTCTXVN AS VALUE_NUMBER", "a.KSPPINM AS NAME", "b.KSPFTCTXVL AS VALUE", "b.KSPFTCTXDF AS ISDEFAULT", "a.KSPPDESC AS DESCRIPTION")
	for _, name := range []string{"CONTEXT_ROW_INDEX", "PARAMETER_NUMBER", "VALUE_NUMBER"} {
		schema = append(schema, rowset.Column{Name: name, Kind: rowset.Integer})
	}
	for _, name := range []string{"NAME", "VALUE", "ISDEFAULT", "DESCRIPTION"} {
		schema = append(schema, rowset.Column{Name: name, Kind: rowset.Text})
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM SYS.X$KSPPI a JOIN SYS.X$KSPPCV2 b ON a.INDX + 1 = b.KSPFTCTXPN"
	args := []any{}
	predicates := []string{}
	if filter.NonDefaultOnly {
		predicates = append(predicates, "NLSSORT(b.KSPFTCTXDF, 'NLS_SORT=BINARY') <> NLSSORT('TRUE', 'NLS_SORT=BINARY')")
	}
	if len(filter.Names) > 0 {
		binds := make([]string, len(filter.Names))
		for i, name := range filter.Names {
			args = append(args, name)
			binds[i] = "NLSSORT(:" + strconv.Itoa(len(args)) + ", 'NLS_SORT=BINARY')"
		}
		predicates = append(predicates, "NLSSORT(a.KSPPINM, 'NLS_SORT=BINARY') IN ("+strings.Join(binds, ",")+")")
	}
	if len(predicates) > 0 {
		statement += " WHERE " + strings.Join(predicates, " AND ")
	}
	statement += " ORDER BY b.INST_ID,"
	if major >= 12 {
		statement += "b.CON_ID,"
	}
	statement += "b.KSPFTCTXPN,b.KSPFTCTXVN,b.INDX"
	provenance := HiddenContextProvenance{Source: hiddenContextSource, OracleMajor: major, Scope: "connected_instance_context_rows", Names: append([]string(nil), filter.Names...), NonDefaultOnly: filter.NonDefaultOnly}
	base := selection{sql: statement, args: args, schema: schema, provenance: Provenance{Source: hiddenContextSource, OracleMajor: major, Scope: provenance.Scope, Names: append([]string(nil), filter.Names...)}}
	return hiddenContextSelection{selection: base, provenance: provenance}, nil
}

// DecodeHiddenContext validates every row and the same raw-default/name admission
// used by JSON. A pure decoder cannot certify EOF; ReadHiddenContext requires an
// admitted complete reader. Numeric identifiers retain exact source text.
func DecodeHiddenContext(result session.Result, major int, filter HiddenContextFilter) (HiddenContextResult, error) {
	plan, err := hiddenContextPlan(major, filter)
	if err != nil {
		return HiddenContextResult{}, err
	}
	return decodeHiddenContext(result, plan)
}
func decodeHiddenContext(result session.Result, plan hiddenContextSelection) (HiddenContextResult, error) {
	index, err := parameterColumns(result, plan.selection)
	if err != nil {
		return HiddenContextResult{}, err
	}
	rows := make([]HiddenContextValue, len(result.Rows))
	for i, row := range result.Rows {
		fail := func() (HiddenContextResult, error) {
			return HiddenContextResult{}, &Error{Source: hiddenContextSource, cause: ErrInvalidEvidence}
		}
		name, instance, number := row[index["NAME"]], row[index["INST_ID"]], row[index["PARAMETER_NUMBER"]]
		id, instanceErr := strconv.ParseInt(instance.Text, 10, 64)
		pn, pnErr := strconv.ParseInt(number.Text, 10, 64)
		if name.Null || name.Text == "" || instance.Null || instanceErr != nil || id < 1 || number.Null || pnErr != nil || pn < 1 {
			return fail()
		}
		flag := row[index["ISDEFAULT"]]
		if plan.provenance.NonDefaultOnly && (flag.Null || flag.Text == "TRUE") {
			return fail()
		}
		rows[i] = HiddenContextValue{InstanceID: instance, ContainerID: optional(row, index, "CON_ID"), ContextRowIndex: row[index["CONTEXT_ROW_INDEX"]], ParameterNumber: number, ValueNumber: row[index["VALUE_NUMBER"]], Name: name, Value: row[index["VALUE"]], IsDefault: flag, Description: row[index["DESCRIPTION"]]}
	}
	return HiddenContextResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadHiddenContext executes once through the admitted local reader. Query,
// terminal, cancellation, late decode and finite-budget failures publish no rows.
// Collection and explicit success/failure coverage across reachable members stay
// with the caller; no remote-member selection or connection switching exists here.
func ReadHiddenContext(ctx context.Context, reader Reader, request HiddenContextRequest) (HiddenContextResult, error) {
	plan, err := hiddenContextPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return HiddenContextResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return HiddenContextResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan.selection)
	if err != nil {
		return HiddenContextResult{}, err
	}
	if len(result.Rows) > limits.MaxRows {
		return HiddenContextResult{}, &Error{Source: hiddenContextSource, cause: rowset.ErrCapacity}
	}
	retained := 0
	for _, row := range result.Rows {
		for _, v := range row {
			if len(v.Text) > limits.MaxBytes-retained {
				return HiddenContextResult{}, &Error{Source: hiddenContextSource, cause: rowset.ErrCapacity}
			}
			retained += len(v.Text)
		}
	}
	facts, de := decodeHiddenContext(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return HiddenContextResult{}, &Error{Source: hiddenContextSource, cause: err}
	}
	return facts, nil
}

// HiddenContextRows returns the actual caller-native local cursor. The caller
// owns iteration, Err/Close, complete value consumption, budgets and member
// affinity. No return-time cancellation, preconsumption or SQL cap is introduced.
func HiddenContextRows(ctx context.Context, q rowset.Queryer, major int, filter HiddenContextFilter) (*sql.Rows, error) {
	plan, err := hiddenContextPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan.selection)
}

// HiddenContextJSON independently owns the same local selection's cursor through
// shared rowset consumption/finalization, then applies DecodeHiddenContext's
// exact admission. No JSON bytes survive terminal/close/cancel/capacity failure.
func HiddenContextJSON(ctx context.Context, q rowset.Queryer, major int, filter HiddenContextFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := hiddenContextPlan(major, filter)
	if err != nil {
		return nil, err
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, plan.selection)
	}, plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(&Error{Source: hiddenContextSource, cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := decodeHiddenContext(result, plan)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{Source: hiddenContextSource, cause: cause}
	}, "limits", "query", "validate")
}
