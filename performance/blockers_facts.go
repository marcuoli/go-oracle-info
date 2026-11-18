package performance

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Blocker preserves exact source text and SQL NULL; no flags or limits are inferred.
type Blocker struct {
	SID              session.Value
	SerialNumber     session.Value
	BlockingInstance session.Value
	BlockingSession  session.Value
	Event            session.Value
}

// BlockersOptions admits the numeric release and the exact source selection.
// New projections admit numeric majors 19, 21 and 23; Oracle 26ai uses its
// observed numeric version, not a synthetic major 26.
type BlockersOptions struct{ OracleMajor int }

// BlockersRequest supplies caller admission and a stable caller-owned SQL reference.
type BlockersRequest struct {
	Options   BlockersOptions
	Reference reference.Reference
}

// BlockersResult retains order/duplicates and source provenance.
// Pure Decode does not establish transport completion; Read/JSON do.
type BlockersResult struct {
	Provenance Provenance
	Rows       []Blocker
}

// BlockersStatement selects the fixed reviewed source with bound caller values.
func BlockersStatement(options BlockersOptions) (string, []any, error) {
	p, err := blockersPlan(options)
	return p.sql, p.args, err
}
func blockersPlan(options BlockersOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query := strings.Replace(BlockersSQL, ", EVENT FROM", ", EVENT AS EVENT FROM", 1)
	return selection{sql: query, args: nil, schema: []rowset.Column{{Name: "SID", Kind: rowset.Text}, {Name: "SERIAL_NUMBER", Kind: rowset.Text}, {Name: "BLOCKING_INSTANCE", Kind: rowset.Text}, {Name: "BLOCKING_SESSION", Kind: rowset.Text}, {Name: "EVENT", Kind: rowset.Text}}, provenance: Provenance{Source: "V$SESSION", OracleMajor: options.OracleMajor, Scope: "connected_instance_current_valid_blockers"}}, nil
}

// DecodeBlockers validates all rows before returning typed facts, preserving NULLs.
func DecodeBlockers(result session.Result, options BlockersOptions) (BlockersResult, error) {
	p, err := blockersPlan(options)
	if err != nil {
		return BlockersResult{}, err
	}
	index, err := columns(result, p, "SID", "SERIAL_NUMBER", "BLOCKING_INSTANCE", "BLOCKING_SESSION")
	if err != nil {
		return BlockersResult{}, err
	}
	rows := make([]Blocker, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = Blocker{SID: r[index["SID"]], SerialNumber: r[index["SERIAL_NUMBER"]], BlockingInstance: r[index["BLOCKING_INSTANCE"]], BlockingSession: r[index["BLOCKING_SESSION"]], Event: r[index["EVENT"]]}
	}
	return BlockersResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadBlockers executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadBlockers(ctx context.Context, reader Reader, request BlockersRequest) (BlockersResult, error) {
	p, err := blockersPlan(request.Options)
	if err != nil {
		return BlockersResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return BlockersResult{}, ErrInvalidRequest
	}
	if err = readError(ctx, p, nil); err != nil {
		return BlockersResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = readError(ctx, p, err); err != nil {
		return BlockersResult{}, err
	}
	facts, err := DecodeBlockers(result, request.Options)
	if err = readError(ctx, p, err); err != nil {
		return BlockersResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// BlockersRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func BlockersRows(ctx context.Context, q rowset.Queryer, options BlockersOptions) (*sql.Rows, error) {
	p, err := blockersPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return r, nil
}

// BlockersJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func BlockersJSON(ctx context.Context, q rowset.Queryer, options BlockersOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := blockersPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeBlockers(r, options); return err })
}
