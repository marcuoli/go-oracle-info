package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// RecoveryProgress preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type RecoveryProgress struct {
	InstanceID  session.Value
	StartTime   session.Value
	Type        session.Value
	Item        session.Value
	Units       session.Value
	Sofar       session.Value
	Total       session.Value
	Timestamp   session.Value
	Comments    session.Value
	ContainerID session.Value
}

// RecoveryProgressResult contains all admitted rows; Decode alone cannot prove transport completion.
type RecoveryProgressResult struct {
	Provenance Provenance
	Rows       []RecoveryProgress
}

// RecoveryProgressStatement selects a reviewed source and bound reporter filters, without a row cap.
func RecoveryProgressStatement(version string, filter Filter) (string, []any, error) {
	p, e := recoveryProgressPlan(version, filter)
	return p.sql, p.args, e
}
func recoveryProgressPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$RECOVERY_PROGRESS", []field{
		integer("INST_ID", "v.INST_ID"),
		date("START_TIME", "v.START_TIME"),
		text("TYPE", "v.TYPE"),
		text("ITEM", "v.ITEM"),
		text("UNITS", "v.UNITS"),
		numeric("SOFAR", "v.SOFAR"),
		numeric("TOTAL", "v.TOTAL"),
		date("TIMESTAMP", "v.TIMESTAMP"),
		text("COMMENTS", "v.COMMENTS"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "v.INST_ID, v.CON_ID, v.START_TIME, v.TYPE, v.ITEM")
}

// DecodeRecoveryProgress validates the entire projection before publishing any typed rows.
func DecodeRecoveryProgress(result session.Result, version string, filter Filter) (RecoveryProgressResult, error) {
	p, e := recoveryProgressPlan(version, filter)
	if e != nil {
		return RecoveryProgressResult{}, e
	}
	return decodeRecoveryProgress(result, p)
}
func decodeRecoveryProgress(result session.Result, p selection) (RecoveryProgressResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return RecoveryProgressResult{}, e
	}
	rows := make([]RecoveryProgress, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = RecoveryProgress{
			InstanceID:  row[index["INST_ID"]],
			StartTime:   row[index["START_TIME"]],
			Type:        row[index["TYPE"]],
			Item:        row[index["ITEM"]],
			Units:       row[index["UNITS"]],
			Sofar:       row[index["SOFAR"]],
			Total:       row[index["TOTAL"]],
			Timestamp:   row[index["TIMESTAMP"]],
			Comments:    row[index["COMMENTS"]],
			ContainerID: row[index["CON_ID"]],
		}
	}
	return RecoveryProgressResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRecoveryProgress executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadRecoveryProgress(ctx context.Context, reader Reader, request Request) (RecoveryProgressResult, error) {
	p, e := recoveryProgressPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return RecoveryProgressResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return RecoveryProgressResult{}, e
	}
	facts, e := decodeRecoveryProgress(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return RecoveryProgressResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RecoveryProgressRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func RecoveryProgressRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := recoveryProgressPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// RecoveryProgressJSON owns its separate cursor and returns nothing on any incomplete read.
func RecoveryProgressJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := recoveryProgressPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}
