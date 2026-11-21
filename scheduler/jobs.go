// Package scheduler selects scheduler metadata without executing or changing jobs.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete finite-budget results, including terminal/close errors.
// Its caller owns connections, admission, deadlines and recovery.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage.
type Provenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
	Owner       string `json:"owner,omitempty"`
	Complete    bool   `json:"complete"`
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle scheduler source version")
	ErrInvalidRequest     = errors.New("invalid Oracle scheduler request")
	ErrInvalidEvidence    = errors.New("invalid Oracle scheduler evidence")
)

// Error retains private causes without echoing SQL, bindings or driver diagnostics.
type Error struct {
	Source string
	cause  error
}

func (err *Error) Error() string { return "Oracle scheduler fact read failed (" + err.Source + ")" }

func (err *Error) Unwrap() error { return err.cause }

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance Provenance
}

// New fact projections admit the independently reviewed numeric release families.
// Older compatibility SQL remains available; branding does not admit numeric26.
func admittedMajor(major int) error {
	switch major {
	case 19, 21, 23:
		return nil
	}
	return ErrUnsupportedVersion
}

func validReader(reader Reader) bool {
	if reader == nil {
		return false
	}
	v := reflect.ValueOf(reader)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

func readError(ctx context.Context, p selection, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &Error{p.provenance.Source, err}
	}
	return nil
}

func columns(result session.Result, p selection, integerFields ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range integerFields {
			v := row[index[name]]
			if !v.Null && !numeric.IsUnsignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{p.provenance.Source, cause}
	}, "limits", "header")
}

// Job preserves exact source text and SQL NULL; no flags or limits are inferred.
type Job struct {
	Owner        session.Value
	Name         session.Value
	Enabled      session.Value
	State        session.Value
	FailureCount session.Value
	NextRunDate  session.Value
}

// JobsOptions admits the numeric release and the exact source selection.
// New projections admit numeric majors 19, 21 and 23; Oracle 26ai uses its
// observed numeric version, not a synthetic major 26.
type JobsOptions struct {
	OracleMajor int
	Owner       string
}

// JobsRequest supplies caller admission and a stable caller-owned SQL reference.
type JobsRequest struct {
	Options   JobsOptions
	Reference reference.Reference
}

// JobsResult retains order/duplicates and source provenance.
// Pure Decode does not establish transport completion; Read/JSON do.
type JobsResult struct {
	Provenance Provenance
	Rows       []Job
}

// JobsStatement selects the fixed reviewed source with bound caller values.
func JobsStatement(options JobsOptions) (string, []any, error) {
	p, err := jobsPlan(options)
	return p.sql, p.args, err
}

func jobsPlan(options JobsOptions) (selection, error) {
	if err := admittedMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	if options.Owner == "" || len(options.Owner) > 128 || !utf8.ValidString(options.Owner) || strings.ContainsRune(options.Owner, 0) {
		return selection{}, ErrInvalidRequest
	}
	// Preserve the public compatibility SQL; only fixed alias/calendar adaptations
	// belong to this typed selection. Caller values are never SQL fragments.
	query := strings.NewReplacer("SELECT OWNER, JOB_NAME, ENABLED, STATE,", "SELECT OWNER AS OWNER, JOB_NAME AS JOB_NAME, ENABLED AS ENABLED, STATE AS STATE,", "OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')").Replace(JobsSQL)
	return selection{sql: query, args: []any{options.Owner}, schema: []rowset.Column{{Name: "OWNER", Kind: rowset.Text}, {Name: "JOB_NAME", Kind: rowset.Text}, {Name: "ENABLED", Kind: rowset.Text}, {Name: "STATE", Kind: rowset.Text}, {Name: "FAILURE_COUNT", Kind: rowset.Text}, {Name: "NEXT_RUN_DATE", Kind: rowset.Text}}, provenance: Provenance{Source: "DBA_SCHEDULER_JOBS", OracleMajor: options.OracleMajor, Scope: "current_container_exact_owner", Owner: options.Owner}}, nil
}

// DecodeJobs validates all rows before returning typed facts, preserving NULLs.
func DecodeJobs(result session.Result, options JobsOptions) (JobsResult, error) {
	p, err := jobsPlan(options)
	if err != nil {
		return JobsResult{}, err
	}
	index, err := columns(result, p, "FAILURE_COUNT")
	if err != nil {
		return JobsResult{}, err
	}
	for _, r := range result.Rows {
		v := r[index["OWNER"]]
		if v.Null || v.Text != options.Owner {
			return JobsResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]Job, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = Job{Owner: r[index["OWNER"]], Name: r[index["JOB_NAME"]], Enabled: r[index["ENABLED"]], State: r[index["STATE"]], FailureCount: r[index["FAILURE_COUNT"]], NextRunDate: r[index["NEXT_RUN_DATE"]]}
	}
	return JobsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadJobs executes once on an admitted complete bounded reader; failures
// and cancellation publish no successful prefix. It owns no scope or retry.
func ReadJobs(ctx context.Context, reader Reader, request JobsRequest) (JobsResult, error) {
	p, err := jobsPlan(request.Options)
	if err != nil {
		return JobsResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return JobsResult{}, ErrInvalidRequest
	}
	if err = readError(ctx, p, nil); err != nil {
		return JobsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = readError(ctx, p, err); err != nil {
		return JobsResult{}, err
	}
	facts, err := DecodeJobs(result, request.Options)
	if err = readError(ctx, p, err); err != nil {
		return JobsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// JobsRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, affinity, cancellation and finite stream budgets.
func JobsRows(ctx context.Context, q rowset.Queryer, options JobsOptions) (*sql.Rows, error) {
	p, err := jobsPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return r, nil
}

// JobsJSON consumes its own complete cursor, validates domain facts, then
// publishes bounded standard JSON. Any scan/terminal/close/budget error returns nil.
func JobsJSON(ctx context.Context, q rowset.Queryer, options JobsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := jobsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeJobs(r, options); return err })
}

// JobsSQL reads jobs for the exact bound owner (:1).
// Callers supply the intended dictionary spelling and bound execution scope.
const JobsSQL = `SELECT OWNER, JOB_NAME, ENABLED, STATE, TO_CHAR(FAILURE_COUNT, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS FAILURE_COUNT, TO_CHAR(NEXT_RUN_DATE,'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM') AS NEXT_RUN_DATE FROM DBA_SCHEDULER_JOBS WHERE OWNER=:1 ORDER BY JOB_NAME`
