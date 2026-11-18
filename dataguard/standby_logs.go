package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// StandbyLog preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type StandbyLog struct {
	InstanceID     session.Value
	GroupNumber    session.Value
	DBID           session.Value
	ThreadNumber   session.Value
	SequenceNumber session.Value
	Bytes          session.Value
	BlockSize      session.Value
	Used           session.Value
	Archived       session.Value
	Status         session.Value
	FirstChange    session.Value
	FirstTime      session.Value
	NextChange     session.Value
	NextTime       session.Value
	LastChange     session.Value
	LastTime       session.Value
	ContainerID    session.Value
}

// StandbyLogsResult contains all admitted rows; Decode alone cannot prove transport completion.
type StandbyLogsResult struct {
	Provenance Provenance
	Rows       []StandbyLog
}

// StandbyLogsStatement selects a reviewed source and bound reporter filters, without a row cap.
func StandbyLogsStatement(version string, filter Filter) (string, []any, error) {
	p, e := standbyLogsPlan(version, filter)
	return p.sql, p.args, e
}
func standbyLogsPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$STANDBY_LOG", []field{
		integer("INST_ID", "v.INST_ID"),
		integer("GROUP_NUMBER", "v.GROUP#"),
		text("DBID", "v.DBID"),
		integer("THREAD_NUMBER", "v.THREAD#"),
		integer("SEQUENCE_NUMBER", "v.SEQUENCE#"),
		integer("BYTES", "v.BYTES"),
		integer("BLOCK_SIZE", "v.BLOCKSIZE"),
		integer("USED", "v.USED"),
		text("ARCHIVED", "v.ARCHIVED"),
		text("STATUS", "v.STATUS"),
		exact("FIRST_CHANGE", "v.FIRST_CHANGE#"),
		date("FIRST_TIME", "v.FIRST_TIME"),
		exact("NEXT_CHANGE", "v.NEXT_CHANGE#"),
		date("NEXT_TIME", "v.NEXT_TIME"),
		exact("LAST_CHANGE", "v.LAST_CHANGE#"),
		date("LAST_TIME", "v.LAST_TIME"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "v.INST_ID, v.CON_ID, v.THREAD#, v.GROUP#")
}

// DecodeStandbyLogs validates the entire projection before publishing any typed rows.
func DecodeStandbyLogs(result session.Result, version string, filter Filter) (StandbyLogsResult, error) {
	p, e := standbyLogsPlan(version, filter)
	if e != nil {
		return StandbyLogsResult{}, e
	}
	return decodeStandbyLogs(result, p)
}
func decodeStandbyLogs(result session.Result, p selection) (StandbyLogsResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return StandbyLogsResult{}, e
	}
	rows := make([]StandbyLog, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = StandbyLog{
			InstanceID:     row[index["INST_ID"]],
			GroupNumber:    row[index["GROUP_NUMBER"]],
			DBID:           row[index["DBID"]],
			ThreadNumber:   row[index["THREAD_NUMBER"]],
			SequenceNumber: row[index["SEQUENCE_NUMBER"]],
			Bytes:          row[index["BYTES"]],
			BlockSize:      row[index["BLOCK_SIZE"]],
			Used:           row[index["USED"]],
			Archived:       row[index["ARCHIVED"]],
			Status:         row[index["STATUS"]],
			FirstChange:    row[index["FIRST_CHANGE"]],
			FirstTime:      row[index["FIRST_TIME"]],
			NextChange:     row[index["NEXT_CHANGE"]],
			NextTime:       row[index["NEXT_TIME"]],
			LastChange:     row[index["LAST_CHANGE"]],
			LastTime:       row[index["LAST_TIME"]],
			ContainerID:    row[index["CON_ID"]],
		}
	}
	return StandbyLogsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadStandbyLogs executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadStandbyLogs(ctx context.Context, reader Reader, request Request) (StandbyLogsResult, error) {
	p, e := standbyLogsPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return StandbyLogsResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return StandbyLogsResult{}, e
	}
	facts, e := decodeStandbyLogs(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return StandbyLogsResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// StandbyLogsRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func StandbyLogsRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := standbyLogsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// StandbyLogsJSON owns its separate cursor and returns nothing on any incomplete read.
func StandbyLogsJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := standbyLogsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}
