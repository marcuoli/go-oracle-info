package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"strconv"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// History preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Control-file history can have aged-out records. Successful selection proves neither seven-day retention nor exact generated/archived bytes. Dates are database-local wall-clock strings without a timezone.
type History struct {
	InstanceID      session.Value
	RecordID        session.Value
	Stamp           session.Value
	ThreadNumber    session.Value
	SequenceNumber  session.Value
	FirstChange     session.Value
	FirstTime       session.Value
	NextChange      session.Value
	ResetlogsChange session.Value
	ResetlogsTime   session.Value
	ContainerID     *session.Value
}

// HistoryResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type HistoryResult struct {
	Provenance Provenance
	Rows       []History
}

// HistoryStatement selects the reviewed source without SQL row truncation.
func HistoryStatement(major int, filter HistoryFilter) (string, []any, error) {
	plan, err := historyPlan(major, filter)
	return plan.sql, plan.args, err
}
func historyPlan(major int, filter HistoryFilter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "RECID", Kind: rowset.Integer},
		{Name: "STAMP", Kind: rowset.Integer},
		{Name: "THREAD_NUMBER", Kind: rowset.Integer},
		{Name: "SEQUENCE_NUMBER", Kind: rowset.Integer},
		{Name: "FIRST_CHANGE", Kind: rowset.Text},
		{Name: "FIRST_TIME", Kind: rowset.Text},
		{Name: "NEXT_CHANGE", Kind: rowset.Text},
		{Name: "RESETLOGS_CHANGE", Kind: rowset.Text},
		{Name: "RESETLOGS_TIME", Kind: rowset.Text},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.RECID AS RECID" + ", " +
		"v.STAMP AS STAMP" + ", " +
		"v.THREAD# AS THREAD_NUMBER" + ", " +
		"v.SEQUENCE# AS SEQUENCE_NUMBER" + ", " +
		exactNumber("v.FIRST_CHANGE#") + " AS FIRST_CHANGE" + ", " +
		datetime.GregorianDate("v.FIRST_TIME") + " AS FIRST_TIME" + ", " +
		exactNumber("v.NEXT_CHANGE#") + " AS NEXT_CHANGE" + ", " +
		exactNumber("v.RESETLOGS_CHANGE#") + " AS RESETLOGS_CHANGE" + ", " +
		datetime.GregorianDate("v.RESETLOGS_TIME") + " AS RESETLOGS_TIME"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$LOG_HISTORY", OracleMajor: major, Scope: "qualified_visible_instances"}}
	plan.integerText = []string{"FIRST_CHANGE", "NEXT_CHANGE", "RESETLOGS_CHANGE"}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := clause
	from, until, err := historyBounds(filter)
	if err != nil {
		return selection{}, err
	}
	plan.provenance.From = from
	plan.provenance.Until = until
	if where != "" {
		where += " AND "
	}
	base := len(plan.args)
	// Only validated Gregorian wall-clock dates enter ANSI DATE literals.
	// Subday offsets remain bound integers; NLS_CALENDAR cannot shift the window.
	fromSeconds := filter.From.Hour()*3600 + filter.From.Minute()*60 + filter.From.Second()
	untilSeconds := filter.Until.Hour()*3600 + filter.Until.Minute()*60 + filter.Until.Second()
	plan.args = append(plan.args, fromSeconds, untilSeconds)
	where += "v.FIRST_TIME >= DATE '" + from[:10] + "' + (:" + strconv.Itoa(base+1) + "/86400) AND v.FIRST_TIME < DATE '" + until[:10] + "' + (:" + strconv.Itoa(base+2) + "/86400)"
	order := "v.INST_ID, v.FIRST_TIME, v.THREAD#, v.SEQUENCE#, v.RECID"
	plan.sql = "SELECT " + projection + " FROM GV$LOG_HISTORY v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeHistory validates every supplied row before publishing any typed slice.
func DecodeHistory(result session.Result, major int, filter HistoryFilter) (HistoryResult, error) {
	plan, err := historyPlan(major, filter)
	if err != nil {
		return HistoryResult{}, err
	}
	return decodeHistory(result, plan)
}
func decodeHistory(result session.Result, plan selection) (HistoryResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return HistoryResult{}, err
	}
	for _, row := range result.Rows {
		v := row[index["FIRST_TIME"]]
		if v.Null || v.Text < plan.provenance.From || v.Text >= plan.provenance.Until {
			return HistoryResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
		}
	}
	rows := make([]History, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = History{
			InstanceID:      row[index["INST_ID"]],
			RecordID:        row[index["RECID"]],
			Stamp:           row[index["STAMP"]],
			ThreadNumber:    row[index["THREAD_NUMBER"]],
			SequenceNumber:  row[index["SEQUENCE_NUMBER"]],
			FirstChange:     row[index["FIRST_CHANGE"]],
			FirstTime:       row[index["FIRST_TIME"]],
			NextChange:      row[index["NEXT_CHANGE"]],
			ResetlogsChange: row[index["RESETLOGS_CHANGE"]],
			ResetlogsTime:   row[index["RESETLOGS_TIME"]],
			ContainerID:     optional(row, index, "CON_ID"),
		}
	}
	return HistoryResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadHistory executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadHistory(ctx context.Context, reader Reader, request HistoryRequest) (HistoryResult, error) {
	plan, err := historyPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return HistoryResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return HistoryResult{}, err
	}
	facts, decodeErr := decodeHistory(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return HistoryResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// HistoryRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func HistoryRows(ctx context.Context, q rowset.Queryer, major int, filter HistoryFilter) (*sql.Rows, error) {
	plan, err := historyPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HistoryJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func HistoryJSON(ctx context.Context, q rowset.Queryer, major int, filter HistoryFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := historyPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeHistory(result, plan); return err })
}
