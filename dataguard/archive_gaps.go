package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ArchiveGap preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type ArchiveGap struct {
	InstanceID   session.Value
	ThreadNumber session.Value
	LowSequence  session.Value
	HighSequence session.Value
	ContainerID  session.Value
}

// ArchiveGapsResult contains all admitted rows; Decode alone cannot prove transport completion.
type ArchiveGapsResult struct {
	Provenance Provenance
	Rows       []ArchiveGap
}

// ArchiveGapsStatement selects a reviewed source and bound reporter filters, without a row cap.
func ArchiveGapsStatement(version string, filter Filter) (string, []any, error) {
	p, e := archiveGapsPlan(version, filter)
	return p.sql, p.args, e
}

func archiveGapsPlan(version string, filter Filter) (selection, error) {
	return selectView(version, filter, "GV$ARCHIVE_GAP", []field{
		integer("INST_ID", "v.INST_ID"),
		integer("THREAD_NUMBER", "v.THREAD#"),
		integer("LOW_SEQUENCE_NUMBER", "v.LOW_SEQUENCE#"),
		integer("HIGH_SEQUENCE_NUMBER", "v.HIGH_SEQUENCE#"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "v.INST_ID, v.CON_ID, v.THREAD#, v.LOW_SEQUENCE#")
}

// DecodeArchiveGaps validates the entire projection before publishing any typed rows.
func DecodeArchiveGaps(result session.Result, version string, filter Filter) (ArchiveGapsResult, error) {
	p, e := archiveGapsPlan(version, filter)
	if e != nil {
		return ArchiveGapsResult{}, e
	}
	return decodeArchiveGaps(result, p)
}

func decodeArchiveGaps(result session.Result, p selection) (ArchiveGapsResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return ArchiveGapsResult{}, e
	}
	rows := make([]ArchiveGap, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ArchiveGap{
			InstanceID:   row[index["INST_ID"]],
			ThreadNumber: row[index["THREAD_NUMBER"]],
			LowSequence:  row[index["LOW_SEQUENCE_NUMBER"]],
			HighSequence: row[index["HIGH_SEQUENCE_NUMBER"]],
			ContainerID:  row[index["CON_ID"]],
		}
	}
	return ArchiveGapsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadArchiveGaps executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadArchiveGaps(ctx context.Context, reader Reader, request Request) (ArchiveGapsResult, error) {
	p, e := archiveGapsPlan(request.OracleVersion, request.Filter)
	if e != nil {
		return ArchiveGapsResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return ArchiveGapsResult{}, e
	}
	facts, e := decodeArchiveGaps(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return ArchiveGapsResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ArchiveGapsRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func ArchiveGapsRows(ctx context.Context, q rowset.Queryer, version string, filter Filter) (*sql.Rows, error) {
	p, e := archiveGapsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// ArchiveGapsJSON owns its separate cursor and returns nothing on any incomplete read.
func ArchiveGapsJSON(ctx context.Context, q rowset.Queryer, version string, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	p, e := archiveGapsPlan(version, filter)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}

// ArchiveGapsStatementWithOptions selects the same source as ArchiveGapsStatement using one options value.
func ArchiveGapsStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveGapsStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveGapsWithOptions selects the same source as DecodeArchiveGaps using one options value.
func DecodeArchiveGapsWithOptions(result session.Result, options ScopeOptions) (ArchiveGapsResult, error) {
	return DecodeArchiveGaps(result, options.OracleVersion, options.Filter)
}

// ArchiveGapsRowsWithOptions selects the same source as ArchiveGapsRows using one options value.
func ArchiveGapsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveGapsRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveGapsJSONWithOptions selects the same source as ArchiveGapsJSON using one options value.
func ArchiveGapsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveGapsJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}
