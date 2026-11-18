package alertlog

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Count preserves exact source text and SQL NULL; no flags or limits are inferred.
type Count struct {
	MessageLevel session.Value
	MessageType  session.Value
	MessageCount session.Value
	FirstSeen    session.Value
	LastSeen     session.Value
}

// CountsOptions admits the numeric release and the exact source selection.
// New projections admit numeric majors 19, 21 and 23; Oracle 26ai uses its
// observed numeric version, not a synthetic major 26.
type CountsOptions struct {
	OracleMajor   int
	WindowSeconds int64
}

// CountsRequest supplies caller admission and a stable caller-owned SQL reference.
type CountsRequest struct {
	Options   CountsOptions
	Reference reference.Reference
}

// CountsResult retains order/duplicates and source provenance.
// Pure Decode does not establish transport completion; Read/JSON do.
type CountsResult struct {
	Provenance Provenance
	Rows       []Count
}

// CountsStatement selects the fixed reviewed source with bound caller values.
func CountsStatement(options CountsOptions) (string, []any, error) {
	p, err := countsPlan(options)
	return p.sql, p.args, err
}
func countsPlan(options CountsOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	if options.WindowSeconds < 1 {
		return selection{}, ErrInvalidRequest
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query := CountsSQL
	return selection{sql: query, args: []any{options.WindowSeconds}, schema: []rowset.Column{{Name: "MESSAGE_LEVEL", Kind: rowset.Text}, {Name: "MESSAGE_TYPE", Kind: rowset.Text}, {Name: "MESSAGE_COUNT", Kind: rowset.Text}, {Name: "FIRST_SEEN", Kind: rowset.Text}, {Name: "LAST_SEEN", Kind: rowset.Text}}, provenance: Provenance{Source: "V$DIAG_ALERT_EXT", OracleMajor: options.OracleMajor, Scope: "connected_instance_current_container_error_window_groups", WindowSeconds: options.WindowSeconds}}, nil
}

// DecodeCounts validates all rows before returning typed facts, preserving NULLs.
func DecodeCounts(result session.Result, options CountsOptions) (CountsResult, error) {
	p, err := countsPlan(options)
	if err != nil {
		return CountsResult{}, err
	}
	index, err := columns(result, p, "MESSAGE_LEVEL", "MESSAGE_TYPE", "MESSAGE_COUNT")
	if err != nil {
		return CountsResult{}, err
	}
	rows := make([]Count, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = Count{MessageLevel: r[index["MESSAGE_LEVEL"]], MessageType: r[index["MESSAGE_TYPE"]], MessageCount: r[index["MESSAGE_COUNT"]], FirstSeen: r[index["FIRST_SEEN"]], LastSeen: r[index["LAST_SEEN"]]}
	}
	return CountsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCounts executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadCounts(ctx context.Context, reader Reader, request CountsRequest) (CountsResult, error) {
	p, err := countsPlan(request.Options)
	if err != nil {
		return CountsResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return CountsResult{}, ErrInvalidRequest
	}
	if err = readError(ctx, p, nil); err != nil {
		return CountsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = readError(ctx, p, err); err != nil {
		return CountsResult{}, err
	}
	facts, err := DecodeCounts(result, request.Options)
	if err = readError(ctx, p, err); err != nil {
		return CountsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// CountsRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func CountsRows(ctx context.Context, q rowset.Queryer, options CountsOptions) (*sql.Rows, error) {
	p, err := countsPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return r, nil
}

// CountsJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func CountsJSON(ctx context.Context, q rowset.Queryer, options CountsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := countsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeCounts(r, options); return err })
}
