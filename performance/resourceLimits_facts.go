package performance

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
)

// ResourceLimit preserves exact source text and SQL NULL; no flags or limits are inferred.
type ResourceLimit struct {
	Name               session.Value
	CurrentUtilization session.Value
	MaxUtilization     session.Value
	Limit              session.Value
}

// ResourceLimitsOptions admits the numeric release and the exact source selection.
// New projections admit19/21/23;26ai must carry its observed numeric23.x version.
type ResourceLimitsOptions struct{ OracleMajor int }

// ResourceLimitsRequest supplies caller admission and a stable caller-owned SQL reference.
type ResourceLimitsRequest struct {
	Options   ResourceLimitsOptions
	Reference reference.Reference
}

// ResourceLimitsResult retains order/duplicates and source provenance.
// Pure Decode does not establish transport completion; Read/JSON do.
type ResourceLimitsResult struct {
	Provenance Provenance
	Rows       []ResourceLimit
}

// ResourceLimitsStatement selects the fixed reviewed source with bound caller values.
func ResourceLimitsStatement(options ResourceLimitsOptions) (string, []any, error) {
	p, e := resourceLimitsPlan(options)
	return p.sql, p.args, e
}
func resourceLimitsPlan(options ResourceLimitsOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query := strings.NewReplacer("SELECT RESOURCE_NAME,", "SELECT RESOURCE_NAME AS RESOURCE_NAME,", ", LIMIT_VALUE FROM", ", LIMIT_VALUE AS LIMIT_VALUE FROM").Replace(ResourceLimitsSQL)
	return selection{sql: query, args: nil, schema: []rowset.Column{{Name: "RESOURCE_NAME", Kind: rowset.Text}, {Name: "CURRENT_UTILIZATION", Kind: rowset.Text}, {Name: "MAX_UTILIZATION", Kind: rowset.Text}, {Name: "LIMIT_VALUE", Kind: rowset.Text}}, provenance: Provenance{Source: "V$RESOURCE_LIMIT", OracleMajor: options.OracleMajor, Scope: "connected_instance_visible_container_rows"}}, nil
}

// DecodeResourceLimits validates all rows before returning typed facts, preserving NULLs.
func DecodeResourceLimits(result session.Result, options ResourceLimitsOptions) (ResourceLimitsResult, error) {
	p, e := resourceLimitsPlan(options)
	if e != nil {
		return ResourceLimitsResult{}, e
	}
	index, e := columns(result, p, "CURRENT_UTILIZATION", "MAX_UTILIZATION")
	if e != nil {
		return ResourceLimitsResult{}, e
	}
	rows := make([]ResourceLimit, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ResourceLimit{Name: r[index["RESOURCE_NAME"]], CurrentUtilization: r[index["CURRENT_UTILIZATION"]], MaxUtilization: r[index["MAX_UTILIZATION"]], Limit: r[index["LIMIT_VALUE"]]}
	}
	return ResourceLimitsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadResourceLimits executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadResourceLimits(ctx context.Context, reader Reader, request ResourceLimitsRequest) (ResourceLimitsResult, error) {
	p, e := resourceLimitsPlan(request.Options)
	if e != nil {
		return ResourceLimitsResult{}, e
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return ResourceLimitsResult{}, ErrInvalidRequest
	}
	if e = readError(ctx, p, nil); e != nil {
		return ResourceLimitsResult{}, e
	}
	result, e := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if e = readError(ctx, p, e); e != nil {
		return ResourceLimitsResult{}, e
	}
	facts, e := DecodeResourceLimits(result, request.Options)
	if e = readError(ctx, p, e); e != nil {
		return ResourceLimitsResult{}, e
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ResourceLimitsRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func ResourceLimitsRows(ctx context.Context, q rowset.Queryer, options ResourceLimitsOptions) (*sql.Rows, error) {
	p, e := resourceLimitsPlan(options)
	if e != nil {
		return nil, e
	}
	r, e := rowset.Query(ctx, q, p.sql, p.args...)
	if e != nil {
		return nil, &Error{p.provenance.Source, e}
	}
	return r, nil
}

// ResourceLimitsJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func ResourceLimitsJSON(ctx context.Context, q rowset.Queryer, options ResourceLimitsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, e := resourceLimitsPlan(options)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, e := DecodeResourceLimits(r, options); return e })
}
