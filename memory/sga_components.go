package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SGAComponent preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Buffer-cache rows and shared-pool allocations remain separate by reporting instance/container. These are observed component allocations, not configured target settings or a proven RAC total.
type SGAComponent struct {
	InstanceID  session.Value
	Pool        session.Value
	Name        session.Value
	Bytes       session.Value
	ContainerID *session.Value
}

// SGAComponentsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type SGAComponentsResult struct {
	Provenance SGAComponentsProvenance
	Rows       []SGAComponent
}

// SGAComponentsProvenance records admitted reporting instances and source
// completion. Selected rows do not establish complete physical RAC membership.
type SGAComponentsProvenance struct {
	Source      string  `json:"source"`
	OracleMajor int     `json:"oracle_major"`
	Scope       string  `json:"scope"`
	Instances   []int64 `json:"instances,omitempty"`
	Complete    bool    `json:"complete"`
}

type componentSelection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance SGAComponentsProvenance
}

// InstanceFilter selects reporting IDs; empty means all visible admitted rows.
type InstanceFilter struct{ Instances []int64 }

// SGARequest admits per-instance observed component facts, not configured targets.
type SGARequest struct {
	OracleMajor int
	Filter      InstanceFilter
	Reference   reference.Reference
}

// SGAComponentsStatement selects the reviewed source without SQL row truncation.
func SGAComponentsStatement(major int, filter InstanceFilter) (string, []any, error) {
	plan, err := sgaComponentsPlan(major, filter)
	return plan.sql, plan.args, err
}

func sgaComponentsPlan(major int, filter InstanceFilter) (componentSelection, error) {
	if major < 11 {
		return componentSelection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "POOL", Kind: rowset.Text},
		{Name: "NAME", Kind: rowset.Text},
		{Name: "BYTES", Kind: rowset.Integer},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.POOL AS POOL" + ", " +
		"v.NAME AS NAME" + ", " +
		"v.BYTES AS BYTES"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := componentSelection{schema: schema, provenance: SGAComponentsProvenance{Source: "GV$SGASTAT", OracleMajor: major, Scope: "qualified_visible_instances"}}
	clause, args, err := sgaInstanceClause(filter.Instances)
	if err != nil {
		return componentSelection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := "(v.NAME = 'buffer_cache' OR v.POOL = 'shared pool')"
	if clause != "" {
		where += " AND " + clause
	}
	order := "v.INST_ID, v.CON_ID, v.POOL, v.NAME"
	if major < 12 {
		order = "v.INST_ID, v.POOL, v.NAME"
	}
	plan.sql = "SELECT " + projection + " FROM GV$SGASTAT v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeSGAComponents validates every supplied row before publishing any typed slice.
func DecodeSGAComponents(result session.Result, major int, filter InstanceFilter) (SGAComponentsResult, error) {
	plan, err := sgaComponentsPlan(major, filter)
	if err != nil {
		return SGAComponentsResult{}, err
	}
	return decodeSGAComponents(result, plan)
}

func decodeSGAComponents(result session.Result, plan componentSelection) (SGAComponentsResult, error) {
	index, err := sgaComponentColumns(result, plan)
	if err != nil {
		return SGAComponentsResult{}, err
	}
	for _, row := range result.Rows {
		name, pool := row[index["NAME"]], row[index["POOL"]]
		if (name.Null || name.Text != "buffer_cache") && (pool.Null || pool.Text != "shared pool") {
			return SGAComponentsResult{}, &DatasetError{plan.provenance.Source, ErrInvalidObservedEvidence}
		}
	}
	rows := make([]SGAComponent, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = SGAComponent{
			InstanceID: row[index["INST_ID"]],
			Pool:       row[index["POOL"]],
			Name:       row[index["NAME"]],
			Bytes:      row[index["BYTES"]],
		}
		if pos, ok := index["CON_ID"]; ok {
			value := row[pos]
			rows[i].ContainerID = &value
		}
	}
	return SGAComponentsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadSGAComponents executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadSGAComponents(ctx context.Context, reader Reader, request SGARequest) (SGAComponentsResult, error) {
	plan, err := sgaComponentsPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return SGAComponentsResult{}, err
	}
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return SGAComponentsResult{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return SGAComponentsResult{}, &DatasetError{plan.provenance.Source, err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return SGAComponentsResult{}, &DatasetError{plan.provenance.Source, err}
	}
	facts, decodeErr := decodeSGAComponents(result, plan)
	if err = errors.Join(projection.MarkDecodeError(decodeErr), ctx.Err()); err != nil {
		return SGAComponentsResult{}, &DatasetError{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SGAComponentsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func SGAComponentsRows(ctx context.Context, q rowset.Queryer, major int, filter InstanceFilter) (*sql.Rows, error) {
	plan, err := sgaComponentsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &DatasetError{plan.provenance.Source, err}
	}
	return rows, nil
}

// SGAComponentsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func SGAComponentsJSON(ctx context.Context, q rowset.Queryer, major int, filter InstanceFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := sgaComponentsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	plan.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql, plan.args...), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(projection.MarkDecodeError(ErrInvalidObservedEvidence), func(result session.Result) error {
			_, err := decodeSGAComponents(result, plan)
			return projection.MarkDecodeError(err)
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &DatasetError{plan.provenance.Source, cause}
	}, "limits")
}

// SGAComponentsOptions carries the source selectors for the options-based fact APIs.
type SGAComponentsOptions struct {
	OracleMajor int
	Filter      InstanceFilter
}

// SGAComponentsStatementWithOptions selects the same source as SGAComponentsStatement using one options value.
func SGAComponentsStatementWithOptions(options SGAComponentsOptions) (string, []any, error) {
	return SGAComponentsStatement(options.OracleMajor, options.Filter)
}

// DecodeSGAComponentsWithOptions selects the same source as DecodeSGAComponents using one options value.
func DecodeSGAComponentsWithOptions(result session.Result, options SGAComponentsOptions) (SGAComponentsResult, error) {
	return DecodeSGAComponents(result, options.OracleMajor, options.Filter)
}

// SGAComponentsRowsWithOptions selects the same source as SGAComponentsRows using one options value.
func SGAComponentsRowsWithOptions(ctx context.Context, q rowset.Queryer, options SGAComponentsOptions) (*sql.Rows, error) {
	return SGAComponentsRows(ctx, q, options.OracleMajor, options.Filter)
}

// SGAComponentsJSONWithOptions selects the same source as SGAComponentsJSON using one options value.
func SGAComponentsJSONWithOptions(ctx context.Context, q rowset.Queryer, options SGAComponentsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return SGAComponentsJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

func sgaInstanceClause(ids []int64) (string, []any, error) {
	if len(ids) > 1000 {
		return "", nil, ErrInvalidRequest
	}
	seen := map[int64]bool{}
	args := make([]any, 0, len(ids))
	binds := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return "", nil, ErrInvalidRequest
		}
		seen[id] = true
		args = append(args, id)
		binds = append(binds, ":"+strconv.Itoa(len(args)))
	}
	if len(ids) == 0 {
		return "", args, nil
	}
	return "v.INST_ID IN (" + strings.Join(binds, ",") + ")", args, nil
}

func sgaComponentColumns(result session.Result, plan componentSelection) (map[string]int, error) {
	names := make([]string, len(plan.schema))
	for i, column := range plan.schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	invalid := &DatasetError{plan.provenance.Source, ErrInvalidObservedEvidence}
	if !ok {
		return nil, invalid
	}
	for _, row := range result.Rows {
		instance := row[index["INST_ID"]]
		id, err := strconv.ParseInt(instance.Text, 10, 64)
		if instance.Null || err != nil || id <= 0 {
			return nil, invalid
		}
		if len(plan.provenance.Instances) > 0 && !slices.Contains(plan.provenance.Instances, id) {
			return nil, invalid
		}
		for _, column := range plan.schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return nil, invalid
			}
			// Component bytes and container IDs preserve arbitrary-width NUMBER text.
			if !value.Null && column.Kind == rowset.Integer && !numtext.IsUnsignedInteger(value.Text) {
				return nil, invalid
			}
		}
	}
	return index, nil
}
