package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

var resourceNumber = regexp.MustCompile("^[+-]?([0-9]+(\\.[0-9]*)?|\\.[0-9]+)([eE][+-]?[0-9]+)?$")

func resourceDate(name string) field {
	return projected(name, datetime.GregorianDate("v."+name), rowset.Text)
}

func resourceCivil(value session.Value) bool {
	if value.Null {
		return true
	}
	_, err := time.Parse("2006-01-02T15:04:05", value.Text)
	return len(value.Text) == 19 && err == nil
}

// ResourceSamplesRequest adds the caller-owned reference for GV$RSRCPDBMETRIC.
type ResourceSamplesRequest struct {
	Options   ResourceOptions
	Reference reference.Reference
}

// ResourceSample preserves one observed Resource Manager sample.
// Times are source-local civil DATE values. Numeric fields retain the source units.
type ResourceSample struct {
	InstanceID                      session.Value
	ContainerID                     session.Value
	SourceContainerID               session.Value
	BeginTime                       session.Value
	EndTime                         session.Value
	IntervalCentiseconds            session.Value
	AverageCPUUtilization           session.Value
	AverageRunningSessions          session.Value
	AverageWaitingSessions          session.Value
	IOPS                            session.Value
	IOMBPS                          session.Value
	PlanName                        session.Value
	AverageIOThrottleMilliseconds   session.Value
	AverageQueuedParallelStatements session.Value
}

// ResourceSamplesResult retains source order and completeness evidence.
type ResourceSamplesResult struct {
	Provenance Provenance
	Rows       []ResourceSample
}

// ResourceSamplesStatement selects admitted member observations.
func ResourceSamplesStatement(options ResourceOptions) (string, error) {
	p, err := resourceSamplesPlan(options)
	return p.sql, err
}

func resourceSamplesPlan(options ResourceOptions) (selection, error) {
	fields := []field{
		resourceDate("BEGIN_TIME"),
		resourceDate("END_TIME"),
		exact("INTSIZE_CSEC"),
		exact("AVG_CPU_UTILIZATION"),
		exact("AVG_RUNNING_SESSIONS"),
		exact("AVG_WAITING_SESSIONS"),
		exact("IOPS"),
		exact("IOMBPS"),
		raw("PLAN_NAME", rowset.Text),
		exact("AVG_IO_THROTTLE"),
		exact("AVG_QUEUED_PARALLEL_STMTS"),
	}
	return resourceSelection(options, "GV$RSRCPDBMETRIC", fields, "v.CON_ID, v.INST_ID, v.END_TIME, v.BEGIN_TIME")
}

// DecodeResourceSamples validates the complete projection before returning any rows.
func DecodeResourceSamples(result session.Result, options ResourceOptions) (ResourceSamplesResult, error) {
	p, err := resourceSamplesPlan(options)
	if err != nil {
		return ResourceSamplesResult{}, err
	}
	return decodeResourceSamples(result, p)
}

func decodeResourceSamples(result session.Result, p selection) (ResourceSamplesResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return ResourceSamplesResult{}, err
	}
	rows := make([]ResourceSample, len(result.Rows))
	for i, row := range result.Rows {
		v := ResourceSample{
			InstanceID:                      row[index["INST_ID"]],
			ContainerID:                     row[index["CON_ID"]],
			SourceContainerID:               row[index["SOURCE_CON_ID"]],
			BeginTime:                       row[index["BEGIN_TIME"]],
			EndTime:                         row[index["END_TIME"]],
			IntervalCentiseconds:            row[index["INTSIZE_CSEC"]],
			AverageCPUUtilization:           row[index["AVG_CPU_UTILIZATION"]],
			AverageRunningSessions:          row[index["AVG_RUNNING_SESSIONS"]],
			AverageWaitingSessions:          row[index["AVG_WAITING_SESSIONS"]],
			IOPS:                            row[index["IOPS"]],
			IOMBPS:                          row[index["IOMBPS"]],
			PlanName:                        row[index["PLAN_NAME"]],
			AverageIOThrottleMilliseconds:   row[index["AVG_IO_THROTTLE"]],
			AverageQueuedParallelStatements: row[index["AVG_QUEUED_PARALLEL_STMTS"]],
		}
		if !validMember(v.InstanceID, v.ContainerID, v.SourceContainerID, p.provenance.Scope) ||
			!resourceCivil(v.BeginTime) ||
			!resourceCivil(v.EndTime) ||
			!unsigned(v.IntervalCentiseconds) ||
			(!v.AverageCPUUtilization.Null && !resourceNumber.MatchString(v.AverageCPUUtilization.Text)) ||
			(!v.AverageRunningSessions.Null && !resourceNumber.MatchString(v.AverageRunningSessions.Text)) ||
			(!v.AverageWaitingSessions.Null && !resourceNumber.MatchString(v.AverageWaitingSessions.Text)) ||
			(!v.IOPS.Null && !resourceNumber.MatchString(v.IOPS.Text)) ||
			(!v.IOMBPS.Null && !resourceNumber.MatchString(v.IOMBPS.Text)) ||
			(!v.AverageIOThrottleMilliseconds.Null && !resourceNumber.MatchString(v.AverageIOThrottleMilliseconds.Text)) ||
			(!v.AverageQueuedParallelStatements.Null && !resourceNumber.MatchString(v.AverageQueuedParallelStatements.Text)) {
			return ResourceSamplesResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return ResourceSamplesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadResourceSamples reads an independent complete buffered observation.
func ReadResourceSamples(ctx context.Context, reader Reader, request ResourceSamplesRequest) (ResourceSamplesResult, error) {
	p, err := resourceSamplesPlan(request.Options)
	if err != nil {
		return ResourceSamplesResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return ResourceSamplesResult{}, err
	}
	facts, err := decodeResourceSamples(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return ResourceSamplesResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ResourceSamplesRows returns the actual cursor, with its lifetime owned by the caller.
func ResourceSamplesRows(ctx context.Context, q rowset.Queryer, options ResourceOptions) (*sql.Rows, error) {
	p, err := resourceSamplesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// ResourceSamplesJSON returns one complete bounded native observation.
func ResourceSamplesJSON(ctx context.Context, q rowset.Queryer, options ResourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := resourceSamplesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeResourceSamples(result, p)
		return err
	})
}
