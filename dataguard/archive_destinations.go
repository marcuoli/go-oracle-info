package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ArchiveDestination preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type ArchiveDestination struct {
	InstanceID      session.Value
	DestinationID   session.Value
	DestinationName session.Value
	Status          session.Value
	Target          session.Value
	Destination     session.Value
	TransmitMode    session.Value
	Affirm          session.Value
	ValidNow        session.Value
	ValidType       session.Value
	ValidRole       session.Value
	DBUniqueName    session.Value
	AppliedSCN      session.Value
	Error           session.Value
	ContainerID     session.Value
}

// ArchiveDestinationsResult contains all admitted rows; Decode alone cannot prove transport completion.
type ArchiveDestinationsResult struct {
	Provenance Provenance
	Rows       []ArchiveDestination
}

// ArchiveDestinationsStatement selects a reviewed source and bound reporter filters, without a row cap.
func ArchiveDestinationsStatement(version string, filter Filter) (string, []any, error) {
	p, e := archiveDestinationsPlan(version, filter)
	return p.sql, p.args, e
}

func archiveDestinationsPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$ARCHIVE_DEST", []field{
		integer("INST_ID", "v.INST_ID"),
		integer("DEST_ID", "v.DEST_ID"),
		text("DEST_NAME", "v.DEST_NAME"),
		text("STATUS", "v.STATUS"),
		text("TARGET", "v.TARGET"),
		text("DESTINATION", "v.DESTINATION"),
		text("TRANSMIT_MODE", "v.TRANSMIT_MODE"),
		text("AFFIRM", "v.AFFIRM"),
		text("VALID_NOW", "v.VALID_NOW"),
		text("VALID_TYPE", "v.VALID_TYPE"),
		text("VALID_ROLE", "v.VALID_ROLE"),
		text("DB_UNIQUE_NAME", "v.DB_UNIQUE_NAME"),
		exact("APPLIED_SCN", "v.APPLIED_SCN"),
		text("ERROR", "v.ERROR"),
		integer("CON_ID", "v.CON_ID"),
	}, "v.DESTINATION IS NOT NULL", "v.INST_ID, v.CON_ID, v.DEST_ID")
}

// DecodeArchiveDestinations validates the entire projection before publishing any typed rows.
func DecodeArchiveDestinations(result session.Result, version string, filter Filter) (ArchiveDestinationsResult, error) {
	p, e := archiveDestinationsPlan(version, filter)
	if e != nil {
		return ArchiveDestinationsResult{}, e
	}
	return decodeArchiveDestinations(result, p)
}

func decodeArchiveDestinations(result session.Result, p selection) (ArchiveDestinationsResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return ArchiveDestinationsResult{}, e
	}
	rows := make([]ArchiveDestination, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ArchiveDestination{
			InstanceID:      row[index["INST_ID"]],
			DestinationID:   row[index["DEST_ID"]],
			DestinationName: row[index["DEST_NAME"]],
			Status:          row[index["STATUS"]],
			Target:          row[index["TARGET"]],
			Destination:     row[index["DESTINATION"]],
			TransmitMode:    row[index["TRANSMIT_MODE"]],
			Affirm:          row[index["AFFIRM"]],
			ValidNow:        row[index["VALID_NOW"]],
			ValidType:       row[index["VALID_TYPE"]],
			ValidRole:       row[index["VALID_ROLE"]],
			DBUniqueName:    row[index["DB_UNIQUE_NAME"]],
			AppliedSCN:      row[index["APPLIED_SCN"]],
			Error:           row[index["ERROR"]],
			ContainerID:     row[index["CON_ID"]],
		}
	}
	return ArchiveDestinationsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadArchiveDestinations executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadArchiveDestinations(ctx context.Context, reader Reader, request Request) (ArchiveDestinationsResult, error) {
	p, e := archiveDestinationsPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return ArchiveDestinationsResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return ArchiveDestinationsResult{}, e
	}
	facts, e := decodeArchiveDestinations(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return ArchiveDestinationsResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ArchiveDestinationsRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func ArchiveDestinationsRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := archiveDestinationsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// ArchiveDestinationsJSON owns its separate cursor and returns nothing on any incomplete read.
func ArchiveDestinationsJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := archiveDestinationsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}

// ArchiveDestinationsStatementWithOptions selects the same source as ArchiveDestinationsStatement using one options value.
func ArchiveDestinationsStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveDestinationsStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveDestinationsWithOptions selects the same source as DecodeArchiveDestinations using one options value.
func DecodeArchiveDestinationsWithOptions(result session.Result, options ScopeOptions) (ArchiveDestinationsResult, error) {
	return DecodeArchiveDestinations(result, options.OracleVersion, options.Filter)
}

// ArchiveDestinationsRowsWithOptions selects the same source as ArchiveDestinationsRows using one options value.
func ArchiveDestinationsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveDestinationsRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveDestinationsJSONWithOptions selects the same source as ArchiveDestinationsJSON using one options value.
func ArchiveDestinationsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveDestinationsJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}
