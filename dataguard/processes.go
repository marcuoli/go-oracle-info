package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Process preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type Process struct {
	InstanceID     session.Value
	Name           session.Value
	PID            session.Value
	Type           session.Value
	Role           session.Value
	Action         session.Value
	ClientPID      session.Value
	ClientRole     session.Value
	GroupNumber    session.Value
	ResetlogID     session.Value
	ThreadNumber   session.Value
	SequenceNumber session.Value
	BlockNumber    session.Value
	BlockCount     session.Value
	DestinationID  session.Value
	DBID           session.Value
	DGID           session.Value
	ContainerID    session.Value
}

// ProcessesResult contains all admitted rows; Decode alone cannot prove transport completion.
type ProcessesResult struct {
	Provenance Provenance
	Rows       []Process
}

// ProcessesStatement selects a reviewed source and bound reporter filters, without a row cap.
func ProcessesStatement(version string, filter Filter) (string, []any, error) {
	p, e := processesPlan(version, filter)
	return p.sql, p.args, e
}

func processesPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$DATAGUARD_PROCESS", []field{
		integer("INST_ID", "v.INST_ID"),
		text("NAME", "v.NAME"),
		text("PID", "v.PID"),
		text("TYPE", "v.TYPE"),
		text("ROLE", "v.ROLE"),
		text("ACTION", "v.ACTION"),
		integer("CLIENT_PID", "v.CLIENT_PID"),
		text("CLIENT_ROLE", "v.CLIENT_ROLE"),
		integer("GROUP_NUMBER", "v.GROUP#"),
		exact("RESETLOG_ID", "v.RESETLOG_ID"),
		integer("THREAD_NUMBER", "v.THREAD#"),
		integer("SEQUENCE_NUMBER", "v.SEQUENCE#"),
		integer("BLOCK_NUMBER", "v.BLOCK#"),
		integer("BLOCK_COUNT", "v.BLOCK_COUNT"),
		integer("DEST_ID", "v.DEST_ID"),
		exact("DBID", "v.DBID"),
		exact("DGID", "v.DGID"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "v.INST_ID, v.CON_ID, v.NAME, v.PID")
}

// DecodeProcesses validates the entire projection before publishing any typed rows.
func DecodeProcesses(result session.Result, version string, filter Filter) (ProcessesResult, error) {
	p, e := processesPlan(version, filter)
	if e != nil {
		return ProcessesResult{}, e
	}
	return decodeProcesses(result, p)
}

func decodeProcesses(result session.Result, p selection) (ProcessesResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return ProcessesResult{}, e
	}
	rows := make([]Process, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Process{
			InstanceID:     row[index["INST_ID"]],
			Name:           row[index["NAME"]],
			PID:            row[index["PID"]],
			Type:           row[index["TYPE"]],
			Role:           row[index["ROLE"]],
			Action:         row[index["ACTION"]],
			ClientPID:      row[index["CLIENT_PID"]],
			ClientRole:     row[index["CLIENT_ROLE"]],
			GroupNumber:    row[index["GROUP_NUMBER"]],
			ResetlogID:     row[index["RESETLOG_ID"]],
			ThreadNumber:   row[index["THREAD_NUMBER"]],
			SequenceNumber: row[index["SEQUENCE_NUMBER"]],
			BlockNumber:    row[index["BLOCK_NUMBER"]],
			BlockCount:     row[index["BLOCK_COUNT"]],
			DestinationID:  row[index["DEST_ID"]],
			DBID:           row[index["DBID"]],
			DGID:           row[index["DGID"]],
			ContainerID:    row[index["CON_ID"]],
		}
	}
	return ProcessesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadProcesses executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadProcesses(ctx context.Context, reader Reader, request Request) (ProcessesResult, error) {
	p, e := processesPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return ProcessesResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return ProcessesResult{}, e
	}
	facts, e := decodeProcesses(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return ProcessesResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ProcessesRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func ProcessesRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := processesPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// ProcessesJSON owns its separate cursor and returns nothing on any incomplete read.
func ProcessesJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := processesPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}

// ProcessesStatementWithOptions selects the same source as ProcessesStatement using one options value.
func ProcessesStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ProcessesStatement(options.OracleVersion, options.Filter)
}

// DecodeProcessesWithOptions selects the same source as DecodeProcesses using one options value.
func DecodeProcessesWithOptions(result session.Result, options ScopeOptions) (ProcessesResult, error) {
	return DecodeProcesses(result, options.OracleVersion, options.Filter)
}

// ProcessesRowsWithOptions selects the same source as ProcessesRows using one options value.
func ProcessesRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ProcessesRows(ctx, q, options.OracleVersion, options.Filter)
}

// ProcessesJSONWithOptions selects the same source as ProcessesJSON using one options value.
func ProcessesJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ProcessesJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}
