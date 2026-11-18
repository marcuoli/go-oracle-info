package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
)

// Job preserves exact source text and SQL NULL; no flags or limits are inferred.
type Job struct {
	InputType   session.Value
	Status      session.Value
	StartTime   session.Value
	EndTime     session.Value
	OutputBytes session.Value
}

// JobsOptions admits the numeric release and the exact source selection.
// New projections admit19/21/23;26ai must carry its observed numeric23.x version.
type JobsOptions struct {
	OracleMajor int
	Limit       int
}

// JobsRequest supplies caller admission and a stable caller-owned SQL reference.
type JobsRequest struct {
	Options   JobsOptions
	Reference reference.Reference
}

// JobsResult retains order/duplicates and source provenance. A completed selection remains a bounded sample, not an inventory.
// Pure Decode does not establish transport completion; Read/JSON do.
type JobsResult struct {
	Provenance Provenance
	Rows       []Job
}

// JobsStatement selects the fixed reviewed source with bound caller values.
func JobsStatement(options JobsOptions) (string, []any, error) {
	p, e := jobsPlan(options)
	return p.sql, p.args, e
}
func jobsPlan(options JobsOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	if options.Limit < 1 || options.Limit > 10000 {
		return selection{}, ErrInvalidRequest
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query, err := JobsSQL(options.Limit)
	if err != nil {
		return selection{}, ErrInvalidRequest
	}
	query = strings.NewReplacer("SELECT INPUT_TYPE, STATUS,", "SELECT INPUT_TYPE AS INPUT_TYPE, STATUS AS STATUS,", `TO_CHAR(START_TIME,'YYYY-MM-DD"T"HH24:MI:SS')`, datetime.GregorianDate("START_TIME"), `TO_CHAR(END_TIME,'YYYY-MM-DD"T"HH24:MI:SS')`, datetime.GregorianDate("END_TIME")).Replace(query)
	return selection{sql: query, args: nil, schema: []rowset.Column{{Name: "INPUT_TYPE", Kind: rowset.Text}, {Name: "STATUS", Kind: rowset.Text}, {Name: "START_TIME", Kind: rowset.Text}, {Name: "END_TIME", Kind: rowset.Text}, {Name: "OUTPUT_BYTES", Kind: rowset.Text}}, provenance: Provenance{Source: "V$RMAN_BACKUP_JOB_DETAILS", OracleMajor: options.OracleMajor, Scope: "connected_controlfile_recent_job_sample", Limit: options.Limit}}, nil
}

// DecodeJobs validates all rows before returning typed facts, preserving NULLs.
func DecodeJobs(result session.Result, options JobsOptions) (JobsResult, error) {
	p, e := jobsPlan(options)
	if e != nil {
		return JobsResult{}, e
	}
	index, e := columns(result, p, "OUTPUT_BYTES")
	if e != nil {
		return JobsResult{}, e
	}
	if len(result.Rows) > options.Limit {
		return JobsResult{}, ErrInvalidEvidence
	}
	rows := make([]Job, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = Job{InputType: r[index["INPUT_TYPE"]], Status: r[index["STATUS"]], StartTime: r[index["START_TIME"]], EndTime: r[index["END_TIME"]], OutputBytes: r[index["OUTPUT_BYTES"]]}
	}
	return JobsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadJobs executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadJobs(ctx context.Context, reader Reader, request JobsRequest) (JobsResult, error) {
	p, e := jobsPlan(request.Options)
	if e != nil {
		return JobsResult{}, e
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return JobsResult{}, ErrInvalidRequest
	}
	if e = readError(ctx, p, nil); e != nil {
		return JobsResult{}, e
	}
	result, e := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if e = readError(ctx, p, e); e != nil {
		return JobsResult{}, e
	}
	facts, e := DecodeJobs(result, request.Options)
	if e = readError(ctx, p, e); e != nil {
		return JobsResult{}, e
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// JobsRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func JobsRows(ctx context.Context, q rowset.Queryer, options JobsOptions) (*sql.Rows, error) {
	p, e := jobsPlan(options)
	if e != nil {
		return nil, e
	}
	r, e := rowset.Query(ctx, q, p.sql, p.args...)
	if e != nil {
		return nil, &Error{p.provenance.Source, e}
	}
	return r, nil
}

// JobsJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func JobsJSON(ctx context.Context, q rowset.Queryer, options JobsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, e := jobsPlan(options)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, e := DecodeJobs(r, options); return e })
}
