package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RunningJobsOptions admits the observed numeric version of the connected handle.
type RunningJobsOptions struct {
	OracleVersion string
}

// RunningJobsRequest supplies the caller reference and buffered bounds.
type RunningJobsRequest struct {
	Options   RunningJobsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// RunningJob preserves the reported run, including its session and instance.
// Subname is absent on 10.1; a projected SQL NULL is a nonnil value.
// SourceContainerID is absent before 12.1 and describes the connected container.
type RunningJob struct {
	Owner             session.Value
	Name              session.Value
	SessionID         session.Value
	RunningInstance   session.Value
	Subname           *session.Value
	SourceContainerID *session.Value
}

// RunningJobsResult contains the selected rows and their connected-dictionary provenance.
type RunningJobsResult struct {
	Provenance Provenance
	Rows       []RunningJob
}

// RunningJobsStatement selects the reviewed DBA_SCHEDULER_RUNNING_JOBS projection.
func RunningJobsStatement(options RunningJobsOptions) (string, error) {
	p, err := runningJobsPlan(options)
	return p.sql, err
}

func runningJobsPlan(options RunningJobsOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 10 {
		return selection{}, ErrUnsupportedVersion
	}
	p := selection{
		sql: "SELECT v.OWNER AS OWNER, v.JOB_NAME AS JOB_NAME",
		schema: []rowset.Column{
			{Name: "OWNER", Kind: rowset.Text},
			{Name: "JOB_NAME", Kind: rowset.Text},
		},
		provenance: Provenance{Source: "DBA_SCHEDULER_RUNNING_JOBS", OracleMajor: major, Scope: "connected_dictionary"},
	}
	// 10.1 documents the run identifiers but has no JOB_SUBNAME column.
	numbers, _ := oracleversion.Parse(options.OracleVersion)
	if major > 10 || numbers[1] == 2 {
		p.sql += ", v.JOB_SUBNAME AS JOB_SUBNAME"
		p.schema = append(p.schema, rowset.Column{Name: "JOB_SUBNAME", Kind: rowset.Text})
	}
	p.sql += ", TO_CHAR(v.SESSION_ID, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SESSION_ID, TO_CHAR(v.RUNNING_INSTANCE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS RUNNING_INSTANCE"
	p.schema = append(p.schema, rowset.Column{Name: "SESSION_ID", Kind: rowset.Text}, rowset.Column{Name: "RUNNING_INSTANCE", Kind: rowset.Text})
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DBA_SCHEDULER_RUNNING_JOBS v"
	return p, nil
}

// DecodeRunningJobs validates every row and preserves source text and SQL NULL.
func DecodeRunningJobs(result session.Result, options RunningJobsOptions) (RunningJobsResult, error) {
	p, err := runningJobsPlan(options)
	if err != nil {
		return RunningJobsResult{}, err
	}
	return decodeRunningJobs(result, p)
}

func decodeRunningJobs(result session.Result, p selection) (RunningJobsResult, error) {
	index, err := columns(result, p, "SESSION_ID", "RUNNING_INSTANCE")
	if err != nil {
		return RunningJobsResult{}, err
	}
	rows := make([]RunningJob, len(result.Rows))
	for i, row := range result.Rows {
		v := RunningJob{
			Owner:           row[index["OWNER"]],
			Name:            row[index["JOB_NAME"]],
			SessionID:       row[index["SESSION_ID"]],
			RunningInstance: row[index["RUNNING_INSTANCE"]],
		}
		if j, ok := index["JOB_SUBNAME"]; ok {
			subname := row[j]
			v.Subname = &subname
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			source := row[j]
			if source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return RunningJobsResult{}, ErrInvalidEvidence
			}
			v.SourceContainerID = &source
		}
		rows[i] = v
	}
	return RunningJobsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRunningJobs executes once over an admitted complete reader.
func ReadRunningJobs(ctx context.Context, reader Reader, request RunningJobsRequest) (RunningJobsResult, error) {
	p, err := runningJobsPlan(request.Options)
	if err != nil {
		return RunningJobsResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return RunningJobsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return RunningJobsResult{}, err
	}
	if err := readError(ctx, p, nil); err != nil {
		return RunningJobsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := readError(ctx, p, err); err != nil {
		return RunningJobsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return RunningJobsResult{}, readError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeRunningJobs(result, p)
	if err := readError(ctx, p, err); err != nil {
		return RunningJobsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RunningJobsRows transfers the caller-native cursor.
func RunningJobsRows(ctx context.Context, q rowset.Queryer, options RunningJobsOptions) (*sql.Rows, error) {
	p, err := runningJobsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

// RunningJobsJSON consumes a separate complete bounded cursor.
func RunningJobsJSON(ctx context.Context, q rowset.Queryer, options RunningJobsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := runningJobsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeRunningJobs(r, p)
		return err
	})
}
