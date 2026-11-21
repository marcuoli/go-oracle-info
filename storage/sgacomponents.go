package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
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
	Provenance Provenance
	Rows       []SGAComponent
}

// SGAComponentsStatement selects the reviewed source without SQL row truncation.
func SGAComponentsStatement(major int, filter InstanceFilter) (string, []any, error) {
	plan, err := sgaComponentsPlan(major, filter)
	return plan.sql, plan.args, err
}
func sgaComponentsPlan(major int, filter InstanceFilter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
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
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$SGASTAT", OracleMajor: major, Scope: "qualified_visible_instances"}}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
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
func decodeSGAComponents(result session.Result, plan selection) (SGAComponentsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return SGAComponentsResult{}, err
	}
	for _, row := range result.Rows {
		name, pool := row[index["NAME"]], row[index["POOL"]]
		if (name.Null || name.Text != "buffer_cache") && (pool.Null || pool.Text != "shared pool") {
			return SGAComponentsResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
		}
	}
	rows := make([]SGAComponent, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = SGAComponent{
			InstanceID:  row[index["INST_ID"]],
			Pool:        row[index["POOL"]],
			Name:        row[index["NAME"]],
			Bytes:       row[index["BYTES"]],
			ContainerID: optional(row, index, "CON_ID"),
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
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return SGAComponentsResult{}, err
	}
	facts, decodeErr := decodeSGAComponents(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return SGAComponentsResult{}, &Error{plan.provenance.Source, err}
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
	return open(ctx, q, plan)
}

// SGAComponentsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func SGAComponentsJSON(ctx context.Context, q rowset.Queryer, major int, filter InstanceFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := sgaComponentsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeSGAComponents(result, plan); return err })
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
