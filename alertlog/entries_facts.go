package alertlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Entry preserves exact source text and SQL NULL; no flags or limits are inferred.
type Entry struct {
	Timestamp    session.Value
	MessageType  session.Value
	MessageLevel session.Value
	Message      session.Value
}

// EntriesOptions admits the numeric release and the exact source selection.
// New projections admit numeric majors 19, 21 and 23; Oracle 26ai uses its
// observed numeric version, not a synthetic major 26.
type EntriesOptions struct {
	OracleMajor   int
	Limit         int
	WindowSeconds int64
}

// EntriesRequest supplies caller admission and a stable caller-owned SQL reference.
type EntriesRequest struct {
	Options   EntriesOptions
	Reference reference.Reference
}

// EntriesResult retains order/duplicates and source provenance. A completed selection remains a bounded sample, not an inventory.
// Pure Decode does not establish transport completion; Read/JSON do.
type EntriesResult struct {
	Provenance Provenance
	Rows       []Entry
}

// EntriesStatement selects the fixed reviewed source with bound caller values.
func EntriesStatement(options EntriesOptions) (string, []any, error) {
	p, err := entriesPlan(options)
	return p.sql, p.args, err
}
func entriesPlan(options EntriesOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	if options.Limit < 1 || options.Limit > 10000 {
		return selection{}, ErrInvalidRequest
	}
	if options.WindowSeconds < 1 {
		return selection{}, ErrInvalidRequest
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query, err := EntriesSQL(options.Limit)
	if err != nil {
		return selection{}, ErrInvalidRequest
	}
	query = strings.Replace(query, "MESSAGE_TEXT FROM", "MESSAGE_TEXT AS MESSAGE_TEXT FROM", 1)
	return selection{sql: query, args: []any{options.WindowSeconds}, schema: []rowset.Column{{Name: "ORIGINATING_TIMESTAMP", Kind: rowset.Text}, {Name: "MESSAGE_TYPE", Kind: rowset.Text}, {Name: "MESSAGE_LEVEL", Kind: rowset.Text}, {Name: "MESSAGE_TEXT", Kind: rowset.Text}}, provenance: Provenance{Source: "V$DIAG_ALERT_EXT", OracleMajor: options.OracleMajor, Scope: "connected_instance_current_container_recent_error_sample", Limit: options.Limit, WindowSeconds: options.WindowSeconds}}, nil
}

// DecodeEntries validates all rows before returning typed facts, preserving NULLs.
func DecodeEntries(result session.Result, options EntriesOptions) (EntriesResult, error) {
	p, err := entriesPlan(options)
	if err != nil {
		return EntriesResult{}, err
	}
	index, err := columns(result, p, "MESSAGE_TYPE", "MESSAGE_LEVEL")
	if err != nil {
		return EntriesResult{}, err
	}
	if len(result.Rows) > options.Limit {
		return EntriesResult{}, ErrInvalidEvidence
	}
	rows := make([]Entry, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = Entry{Timestamp: r[index["ORIGINATING_TIMESTAMP"]], MessageType: r[index["MESSAGE_TYPE"]], MessageLevel: r[index["MESSAGE_LEVEL"]], Message: r[index["MESSAGE_TEXT"]]}
	}
	return EntriesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadEntries executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadEntries(ctx context.Context, reader Reader, request EntriesRequest) (EntriesResult, error) {
	p, err := entriesPlan(request.Options)
	if err != nil {
		return EntriesResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return EntriesResult{}, ErrInvalidRequest
	}
	if err = readError(ctx, p, nil); err != nil {
		return EntriesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = readError(ctx, p, err); err != nil {
		return EntriesResult{}, err
	}
	facts, err := DecodeEntries(result, request.Options)
	if err = readError(ctx, p, err); err != nil {
		return EntriesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// EntriesRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func EntriesRows(ctx context.Context, q rowset.Queryer, options EntriesOptions) (*sql.Rows, error) {
	p, err := entriesPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return r, nil
}

// EntriesJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func EntriesJSON(ctx context.Context, q rowset.Queryer, options EntriesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := entriesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeEntries(r, options); return err })
}
