package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ArchivedLog retains archive copies by reporting instance, destination,
// thread, sequence and resetlogs identity. Blocks times BlockSize is archived
// copy allocation, not exact generated redo. Keep deleted/unavailable copies
// and deduplicate identities before totals; no copy is silently selected here.
type ArchivedLog struct {
	InstanceID                session.Value
	RecordID                  session.Value
	Stamp                     session.Value
	DestinationID             session.Value
	Name                      session.Value
	ThreadNumber              session.Value
	SequenceNumber            session.Value
	ResetlogsChange           session.Value
	ResetlogsTime             session.Value
	FirstChange               session.Value
	FirstTime                 session.Value
	NextChange                session.Value
	NextTime                  session.Value
	CompletionTime            session.Value
	Blocks                    session.Value
	BlockSize                 session.Value
	Archived                  session.Value
	Applied                   session.Value
	Deleted                   session.Value
	Status                    session.Value
	Creator                   session.Value
	Registrar                 session.Value
	StandbyDestination        session.Value
	ResetlogsID               session.Value
	Activation                session.Value
	ArchivalThread            session.Value
	IsRecoveryDestinationFile session.Value
	ContainerID               *session.Value
}

// ArchivedLogsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type ArchivedLogsResult struct {
	Provenance Provenance
	Rows       []ArchivedLog
}

// ArchivedLogsStatement selects the reviewed source without SQL row truncation.
func ArchivedLogsStatement(major int, filter HistoryFilter) (string, []any, error) {
	plan, err := archivedLogsPlan(major, filter)
	return plan.sql, plan.args, err
}
func archivedLogsPlan(major int, filter HistoryFilter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "RESETLOGS_ID", Kind: rowset.Integer},
		{Name: "ACTIVATION", Kind: rowset.Integer},
		{Name: "ARCHIVAL_THREAD", Kind: rowset.Integer},
		{Name: "IS_RECOVERY_DEST_FILE", Kind: rowset.Text},
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "RECID", Kind: rowset.Integer},
		{Name: "STAMP", Kind: rowset.Integer},
		{Name: "DEST_ID", Kind: rowset.Integer},
		{Name: "NAME", Kind: rowset.Text},
		{Name: "THREAD_NUMBER", Kind: rowset.Integer},
		{Name: "SEQUENCE_NUMBER", Kind: rowset.Integer},
		{Name: "RESETLOGS_CHANGE", Kind: rowset.Text},
		{Name: "RESETLOGS_TIME", Kind: rowset.Text},
		{Name: "FIRST_CHANGE", Kind: rowset.Text},
		{Name: "FIRST_TIME", Kind: rowset.Text},
		{Name: "NEXT_CHANGE", Kind: rowset.Text},
		{Name: "NEXT_TIME", Kind: rowset.Text},
		{Name: "COMPLETION_TIME", Kind: rowset.Text},
		{Name: "BLOCKS", Kind: rowset.Integer},
		{Name: "BLOCK_SIZE", Kind: rowset.Integer},
		{Name: "ARCHIVED", Kind: rowset.Text},
		{Name: "APPLIED", Kind: rowset.Text},
		{Name: "DELETED", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "CREATOR", Kind: rowset.Text},
		{Name: "REGISTRAR", Kind: rowset.Text},
		{Name: "STANDBY_DEST", Kind: rowset.Text},
	}
	projection := "v.RESETLOGS_ID AS RESETLOGS_ID, v.ACTIVATION# AS ACTIVATION, v.ARCHIVAL_THREAD# AS ARCHIVAL_THREAD, v.IS_RECOVERY_DEST_FILE AS IS_RECOVERY_DEST_FILE, " + "v.INST_ID AS INST_ID" + ", " +
		"v.RECID AS RECID" + ", " +
		"v.STAMP AS STAMP" + ", " +
		"v.DEST_ID AS DEST_ID" + ", " +
		"v.NAME AS NAME" + ", " +
		"v.THREAD# AS THREAD_NUMBER" + ", " +
		"v.SEQUENCE# AS SEQUENCE_NUMBER" + ", " +
		exactNumber("v.RESETLOGS_CHANGE#") + " AS RESETLOGS_CHANGE" + ", " +
		localDate("v.RESETLOGS_TIME") + " AS RESETLOGS_TIME" + ", " +
		exactNumber("v.FIRST_CHANGE#") + " AS FIRST_CHANGE" + ", " +
		localDate("v.FIRST_TIME") + " AS FIRST_TIME" + ", " +
		exactNumber("v.NEXT_CHANGE#") + " AS NEXT_CHANGE" + ", " +
		localDate("v.NEXT_TIME") + " AS NEXT_TIME" + ", " +
		localDate("v.COMPLETION_TIME") + " AS COMPLETION_TIME" + ", " +
		"v.BLOCKS AS BLOCKS" + ", " +
		"v.BLOCK_SIZE AS BLOCK_SIZE" + ", " +
		"v.ARCHIVED AS ARCHIVED" + ", " +
		"v.APPLIED AS APPLIED" + ", " +
		"v.DELETED AS DELETED" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.CREATOR AS CREATOR" + ", " +
		"v.REGISTRAR AS REGISTRAR" + ", " +
		"v.STANDBY_DEST AS STANDBY_DEST"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$ARCHIVED_LOG", OracleMajor: major, Scope: "qualified_visible_instances"}}
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
	order := "v.INST_ID, v.FIRST_TIME, v.THREAD#, v.SEQUENCE#, v.DEST_ID, v.RECID"
	plan.sql = "SELECT " + projection + " FROM GV$ARCHIVED_LOG v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeArchivedLogs validates every supplied row before publishing any typed slice.
func DecodeArchivedLogs(result session.Result, major int, filter HistoryFilter) (ArchivedLogsResult, error) {
	plan, err := archivedLogsPlan(major, filter)
	if err != nil {
		return ArchivedLogsResult{}, err
	}
	return decodeArchivedLogs(result, plan)
}
func decodeArchivedLogs(result session.Result, plan selection) (ArchivedLogsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ArchivedLogsResult{}, err
	}
	for _, row := range result.Rows {
		v := row[index["FIRST_TIME"]]
		if v.Null || v.Text < plan.provenance.From || v.Text >= plan.provenance.Until {
			return ArchivedLogsResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
		}
	}
	rows := make([]ArchivedLog, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ArchivedLog{
			ResetlogsID:               row[index["RESETLOGS_ID"]],
			Activation:                row[index["ACTIVATION"]],
			ArchivalThread:            row[index["ARCHIVAL_THREAD"]],
			IsRecoveryDestinationFile: row[index["IS_RECOVERY_DEST_FILE"]],
			InstanceID:                row[index["INST_ID"]],
			RecordID:                  row[index["RECID"]],
			Stamp:                     row[index["STAMP"]],
			DestinationID:             row[index["DEST_ID"]],
			Name:                      row[index["NAME"]],
			ThreadNumber:              row[index["THREAD_NUMBER"]],
			SequenceNumber:            row[index["SEQUENCE_NUMBER"]],
			ResetlogsChange:           row[index["RESETLOGS_CHANGE"]],
			ResetlogsTime:             row[index["RESETLOGS_TIME"]],
			FirstChange:               row[index["FIRST_CHANGE"]],
			FirstTime:                 row[index["FIRST_TIME"]],
			NextChange:                row[index["NEXT_CHANGE"]],
			NextTime:                  row[index["NEXT_TIME"]],
			CompletionTime:            row[index["COMPLETION_TIME"]],
			Blocks:                    row[index["BLOCKS"]],
			BlockSize:                 row[index["BLOCK_SIZE"]],
			Archived:                  row[index["ARCHIVED"]],
			Applied:                   row[index["APPLIED"]],
			Deleted:                   row[index["DELETED"]],
			Status:                    row[index["STATUS"]],
			Creator:                   row[index["CREATOR"]],
			Registrar:                 row[index["REGISTRAR"]],
			StandbyDestination:        row[index["STANDBY_DEST"]],
			ContainerID:               optional(row, index, "CON_ID"),
		}
	}
	return ArchivedLogsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadArchivedLogs executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadArchivedLogs(ctx context.Context, reader Reader, request HistoryRequest) (ArchivedLogsResult, error) {
	plan, err := archivedLogsPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return ArchivedLogsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return ArchivedLogsResult{}, err
	}
	facts, decodeErr := decodeArchivedLogs(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ArchivedLogsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ArchivedLogsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func ArchivedLogsRows(ctx context.Context, q rowset.Queryer, major int, filter HistoryFilter) (*sql.Rows, error) {
	plan, err := archivedLogsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ArchivedLogsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func ArchivedLogsJSON(ctx context.Context, q rowset.Queryer, major int, filter HistoryFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := archivedLogsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeArchivedLogs(result, plan); return err })
}
