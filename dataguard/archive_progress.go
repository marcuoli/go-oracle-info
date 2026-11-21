package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ArchiveProgress preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type ArchiveProgress struct {
	InstanceID      session.Value
	Kind            session.Value
	RecordID        session.Value
	Stamp           session.Value
	DestinationID   session.Value
	Name            session.Value
	ThreadNumber    session.Value
	SequenceNumber  session.Value
	ResetlogsChange session.Value
	ResetlogsTime   session.Value
	ResetlogsID     session.Value
	FirstChange     session.Value
	FirstTime       session.Value
	NextChange      session.Value
	NextTime        session.Value
	CompletionTime  session.Value
	Registrar       session.Value
	Applied         session.Value
	Archived        session.Value
	Deleted         session.Value
	Status          session.Value
	ContainerID     session.Value
}

// ArchiveProgressResult contains all admitted rows; Decode alone cannot prove transport completion.
type ArchiveProgressResult struct {
	Provenance Provenance
	Rows       []ArchiveProgress
}

// ArchiveProgressStatement selects a reviewed source and bound reporter filters, without a row cap.
func ArchiveProgressStatement(version string, filter ArchiveProgressOptions) (string, []any, error) {
	p, e := archiveProgressPlan(version, filter)
	return p.sql, p.args, e
}
func archiveProgressPlan(version string, filter ArchiveProgressOptions) (selection, error) {
	if !numtext.IsUnsignedInteger(filter.ResetlogsChange) || len(filter.ResetlogsChange) > 38 {
		return selection{}, ErrInvalidRequest
	}
	p, err := selectView(version, Filter{Instances: filter.Instances}, "GV$ARCHIVED_LOG", []field{
		integer("INST_ID", "v.INST_ID"),
		text("KIND", "v.KIND"),
		integer("RECID", "v.RECID"),
		integer("STAMP", "v.STAMP"),
		integer("DEST_ID", "v.DEST_ID"),
		text("NAME", "v.NAME"),
		integer("THREAD_NUMBER", "v.THREAD#"),
		integer("SEQUENCE_NUMBER", "v.SEQUENCE#"),
		exact("RESETLOGS_CHANGE", "v.RESETLOGS_CHANGE#"),
		date("RESETLOGS_TIME", "v.RESETLOGS_TIME"),
		exact("RESETLOGS_ID", "v.RESETLOGS_ID"),
		exact("FIRST_CHANGE", "v.FIRST_CHANGE#"),
		date("FIRST_TIME", "v.FIRST_TIME"),
		exact("NEXT_CHANGE", "v.NEXT_CHANGE#"),
		date("NEXT_TIME", "v.NEXT_TIME"),
		date("COMPLETION_TIME", "v.COMPLETION_TIME"),
		text("REGISTRAR", "v.REGISTRAR"),
		text("APPLIED", "v.APPLIED"),
		text("ARCHIVED", "v.ARCHIVED"),
		text("DELETED", "v.DELETED"),
		text("STATUS", "v.STATUS"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "v.INST_ID, v.CON_ID, v.THREAD#, v.KIND, v.SEQUENCE#, v.DEST_ID, v.RECID")
	if err != nil {
		return selection{}, err
	}
	if _, e := civilTime(filter.ResetlogsTime); e != nil {
		return selection{}, ErrInvalidRequest
	}
	p.provenance.ResetlogsChange = filter.ResetlogsChange
	p.provenance.ResetlogsTime = filter.ResetlogsTime
	return archiveSnapshot(p, filter)
}

// DecodeArchiveProgress validates the entire projection before publishing any typed rows.
func DecodeArchiveProgress(result session.Result, version string, filter ArchiveProgressOptions) (ArchiveProgressResult, error) {
	p, e := archiveProgressPlan(version, filter)
	if e != nil {
		return ArchiveProgressResult{}, e
	}
	return decodeArchiveProgress(result, p)
}
func decodeArchiveProgress(result session.Result, p selection) (ArchiveProgressResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return ArchiveProgressResult{}, e
	}
	rows := make([]ArchiveProgress, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ArchiveProgress{
			InstanceID:      row[index["INST_ID"]],
			Kind:            row[index["KIND"]],
			RecordID:        row[index["RECID"]],
			Stamp:           row[index["STAMP"]],
			DestinationID:   row[index["DEST_ID"]],
			Name:            row[index["NAME"]],
			ThreadNumber:    row[index["THREAD_NUMBER"]],
			SequenceNumber:  row[index["SEQUENCE_NUMBER"]],
			ResetlogsChange: row[index["RESETLOGS_CHANGE"]],
			ResetlogsTime:   row[index["RESETLOGS_TIME"]],
			ResetlogsID:     row[index["RESETLOGS_ID"]],
			FirstChange:     row[index["FIRST_CHANGE"]],
			FirstTime:       row[index["FIRST_TIME"]],
			NextChange:      row[index["NEXT_CHANGE"]],
			NextTime:        row[index["NEXT_TIME"]],
			CompletionTime:  row[index["COMPLETION_TIME"]],
			Registrar:       row[index["REGISTRAR"]],
			Applied:         row[index["APPLIED"]],
			Archived:        row[index["ARCHIVED"]],
			Deleted:         row[index["DELETED"]],
			Status:          row[index["STATUS"]],
			ContainerID:     row[index["CON_ID"]],
		}
	}
	return ArchiveProgressResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadArchiveProgress executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadArchiveProgress(ctx context.Context, reader Reader, request ArchiveProgressRequest) (ArchiveProgressResult, error) {
	p, e := archiveProgressPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return ArchiveProgressResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return ArchiveProgressResult{}, e
	}
	facts, e := decodeArchiveProgress(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return ArchiveProgressResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ArchiveProgressRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func ArchiveProgressRows(ctx context.Context, q rowset.Queryer, version string, filter ArchiveProgressOptions) (*sql.Rows, error) {
	p, e := archiveProgressPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// ArchiveProgressJSON owns its separate cursor and returns nothing on any incomplete read.
func ArchiveProgressJSON(ctx context.Context, q rowset.Queryer, version string, filter ArchiveProgressOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, e := archiveProgressPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}
