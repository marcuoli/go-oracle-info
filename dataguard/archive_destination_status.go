package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ArchiveDestinationStatus preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type ArchiveDestinationStatus struct {
	InstanceID            session.Value
	DestinationID         session.Value
	DestinationName       session.Value
	Status                session.Value
	Type                  session.Value
	DatabaseMode          session.Value
	RecoveryMode          session.Value
	ProtectionMode        session.Value
	Destination           session.Value
	StandbyLogfileCount   session.Value
	StandbyLogfileActive  session.Value
	ArchivedThread        session.Value
	ArchivedSequence      session.Value
	AppliedThread         session.Value
	AppliedSequence       session.Value
	Error                 session.Value
	SRL                   session.Value
	DBUniqueName          session.Value
	SynchronizationStatus session.Value
	Synchronized          session.Value
	GapStatus             session.Value
	ContainerID           session.Value
}

// ArchiveDestinationStatusResult contains all admitted rows; Decode alone cannot prove transport completion.
type ArchiveDestinationStatusResult struct {
	Provenance Provenance
	Rows       []ArchiveDestinationStatus
}

// ArchiveDestinationStatusStatement selects a reviewed source and bound reporter filters, without a row cap.
func ArchiveDestinationStatusStatement(version string, filter Filter) (string, []any, error) {
	p, e := archiveDestinationStatusPlan(version, filter)
	return p.sql, p.args, e
}

func archiveDestinationStatusPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$ARCHIVE_DEST_STATUS", []field{
		integer("INST_ID", "v.INST_ID"),
		integer("DEST_ID", "v.DEST_ID"),
		text("DEST_NAME", "v.DEST_NAME"),
		text("STATUS", "v.STATUS"),
		text("TYPE", "v.TYPE"),
		text("DATABASE_MODE", "v.DATABASE_MODE"),
		text("RECOVERY_MODE", "v.RECOVERY_MODE"),
		text("PROTECTION_MODE", "v.PROTECTION_MODE"),
		text("DESTINATION", "v.DESTINATION"),
		integer("STANDBY_LOGFILE_COUNT", "v.STANDBY_LOGFILE_COUNT"),
		integer("STANDBY_LOGFILE_ACTIVE", "v.STANDBY_LOGFILE_ACTIVE"),
		integer("ARCHIVED_THREAD_NUMBER", "v.ARCHIVED_THREAD#"),
		integer("ARCHIVED_SEQUENCE_NUMBER", "v.ARCHIVED_SEQ#"),
		integer("APPLIED_THREAD_NUMBER", "v.APPLIED_THREAD#"),
		integer("APPLIED_SEQUENCE_NUMBER", "v.APPLIED_SEQ#"),
		text("ERROR", "v.ERROR"),
		text("SRL", "v.SRL"),
		text("DB_UNIQUE_NAME", "v.DB_UNIQUE_NAME"),
		text("SYNCHRONIZATION_STATUS", "v.SYNCHRONIZATION_STATUS"),
		text("SYNCHRONIZED", "v.SYNCHRONIZED"),
		text("GAP_STATUS", "v.GAP_STATUS"),
		integer("CON_ID", "v.CON_ID"),
	}, "v.DESTINATION IS NOT NULL", "v.INST_ID, v.CON_ID, v.DEST_ID")
}

// DecodeArchiveDestinationStatus validates the entire projection before publishing any typed rows.
func DecodeArchiveDestinationStatus(result session.Result, version string, filter Filter) (ArchiveDestinationStatusResult, error) {
	p, e := archiveDestinationStatusPlan(version, filter)
	if e != nil {
		return ArchiveDestinationStatusResult{}, e
	}
	return decodeArchiveDestinationStatus(result, p)
}

func decodeArchiveDestinationStatus(result session.Result, p selection) (ArchiveDestinationStatusResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return ArchiveDestinationStatusResult{}, e
	}
	rows := make([]ArchiveDestinationStatus, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ArchiveDestinationStatus{
			InstanceID:            row[index["INST_ID"]],
			DestinationID:         row[index["DEST_ID"]],
			DestinationName:       row[index["DEST_NAME"]],
			Status:                row[index["STATUS"]],
			Type:                  row[index["TYPE"]],
			DatabaseMode:          row[index["DATABASE_MODE"]],
			RecoveryMode:          row[index["RECOVERY_MODE"]],
			ProtectionMode:        row[index["PROTECTION_MODE"]],
			Destination:           row[index["DESTINATION"]],
			StandbyLogfileCount:   row[index["STANDBY_LOGFILE_COUNT"]],
			StandbyLogfileActive:  row[index["STANDBY_LOGFILE_ACTIVE"]],
			ArchivedThread:        row[index["ARCHIVED_THREAD_NUMBER"]],
			ArchivedSequence:      row[index["ARCHIVED_SEQUENCE_NUMBER"]],
			AppliedThread:         row[index["APPLIED_THREAD_NUMBER"]],
			AppliedSequence:       row[index["APPLIED_SEQUENCE_NUMBER"]],
			Error:                 row[index["ERROR"]],
			SRL:                   row[index["SRL"]],
			DBUniqueName:          row[index["DB_UNIQUE_NAME"]],
			SynchronizationStatus: row[index["SYNCHRONIZATION_STATUS"]],
			Synchronized:          row[index["SYNCHRONIZED"]],
			GapStatus:             row[index["GAP_STATUS"]],
			ContainerID:           row[index["CON_ID"]],
		}
	}
	return ArchiveDestinationStatusResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadArchiveDestinationStatus executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadArchiveDestinationStatus(ctx context.Context, reader Reader, request Request) (ArchiveDestinationStatusResult, error) {
	p, e := archiveDestinationStatusPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return ArchiveDestinationStatusResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return ArchiveDestinationStatusResult{}, e
	}
	facts, e := decodeArchiveDestinationStatus(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return ArchiveDestinationStatusResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ArchiveDestinationStatusRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func ArchiveDestinationStatusRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := archiveDestinationStatusPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// ArchiveDestinationStatusJSON owns its separate cursor and returns nothing on any incomplete read.
func ArchiveDestinationStatusJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := archiveDestinationStatusPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}

// ArchiveDestinationStatusStatementWithOptions selects the same source as ArchiveDestinationStatusStatement using one options value.
func ArchiveDestinationStatusStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveDestinationStatusStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveDestinationStatusWithOptions selects the same source as DecodeArchiveDestinationStatus using one options value.
func DecodeArchiveDestinationStatusWithOptions(result session.Result, options ScopeOptions) (ArchiveDestinationStatusResult, error) {
	return DecodeArchiveDestinationStatus(result, options.OracleVersion, options.Filter)
}

// ArchiveDestinationStatusRowsWithOptions selects the same source as ArchiveDestinationStatusRows using one options value.
func ArchiveDestinationStatusRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveDestinationStatusRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveDestinationStatusJSONWithOptions selects the same source as ArchiveDestinationStatusJSON using one options value.
func ArchiveDestinationStatusJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveDestinationStatusJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}
