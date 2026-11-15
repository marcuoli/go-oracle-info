package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Segment preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Bytes is allocated segment space, not measured live data. Owner exclusions, top 50 selection, partition and LOB overlaps belong to callers.
type Segment struct {
	ReportingInstanceID session.Value
	Owner               session.Value
	Name                session.Value
	PartitionName       session.Value
	Type                session.Value
	Subtype             session.Value
	TablespaceName      session.Value
	Bytes               session.Value
	Blocks              session.Value
	Extents             session.Value
	HeaderFile          session.Value
	HeaderBlock         session.Value
	RelativeFileNumber  session.Value
	ContainerID         *session.Value
}

// SegmentsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type SegmentsResult struct {
	Provenance Provenance
	Rows       []Segment
}

// SegmentsStatement selects the reviewed source without SQL row truncation.
func SegmentsStatement(major int) (string, error) {
	plan, err := segmentsPlan(major)
	return plan.sql, err
}
func segmentsPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "OWNER", Kind: rowset.Text},
		{Name: "SEGMENT_NAME", Kind: rowset.Text},
		{Name: "PARTITION_NAME", Kind: rowset.Text},
		{Name: "SEGMENT_TYPE", Kind: rowset.Text},
		{Name: "SEGMENT_SUBTYPE", Kind: rowset.Text},
		{Name: "TABLESPACE_NAME", Kind: rowset.Text},
		{Name: "BYTES", Kind: rowset.Integer},
		{Name: "BLOCKS", Kind: rowset.Integer},
		{Name: "EXTENTS", Kind: rowset.Integer},
		{Name: "HEADER_FILE", Kind: rowset.Integer},
		{Name: "HEADER_BLOCK", Kind: rowset.Integer},
		{Name: "RELATIVE_FNO", Kind: rowset.Integer},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.OWNER AS OWNER" + ", " +
		"v.SEGMENT_NAME AS SEGMENT_NAME" + ", " +
		"v.PARTITION_NAME AS PARTITION_NAME" + ", " +
		"v.SEGMENT_TYPE AS SEGMENT_TYPE" + ", " +
		"v.SEGMENT_SUBTYPE AS SEGMENT_SUBTYPE" + ", " +
		"v.TABLESPACE_NAME AS TABLESPACE_NAME" + ", " +
		"v.BYTES AS BYTES" + ", " +
		"v.BLOCKS AS BLOCKS" + ", " +
		"v.EXTENTS AS EXTENTS" + ", " +
		"v.HEADER_FILE AS HEADER_FILE" + ", " +
		"v.HEADER_BLOCK AS HEADER_BLOCK" + ", " +
		"v.RELATIVE_FNO AS RELATIVE_FNO"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_SEGMENTS", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	order := "v.OWNER, v.SEGMENT_NAME, v.PARTITION_NAME, v.SEGMENT_TYPE, v.HEADER_FILE, v.HEADER_BLOCK"
	plan.sql = "SELECT " + projection + " FROM DBA_SEGMENTS v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeSegments validates every supplied row before publishing any typed slice.
func DecodeSegments(result session.Result, major int) (SegmentsResult, error) {
	plan, err := segmentsPlan(major)
	if err != nil {
		return SegmentsResult{}, err
	}
	return decodeSegments(result, plan)
}
func decodeSegments(result session.Result, plan selection) (SegmentsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return SegmentsResult{}, err
	}
	rows := make([]Segment, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Segment{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			Owner:               row[index["OWNER"]],
			Name:                row[index["SEGMENT_NAME"]],
			PartitionName:       row[index["PARTITION_NAME"]],
			Type:                row[index["SEGMENT_TYPE"]],
			Subtype:             row[index["SEGMENT_SUBTYPE"]],
			TablespaceName:      row[index["TABLESPACE_NAME"]],
			Bytes:               row[index["BYTES"]],
			Blocks:              row[index["BLOCKS"]],
			Extents:             row[index["EXTENTS"]],
			HeaderFile:          row[index["HEADER_FILE"]],
			HeaderBlock:         row[index["HEADER_BLOCK"]],
			RelativeFileNumber:  row[index["RELATIVE_FNO"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return SegmentsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadSegments executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadSegments(ctx context.Context, reader Reader, request Request) (SegmentsResult, error) {
	plan, err := segmentsPlan(request.OracleMajor)
	if err != nil {
		return SegmentsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return SegmentsResult{}, err
	}
	facts, decodeErr := decodeSegments(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return SegmentsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SegmentsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func SegmentsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := segmentsPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// SegmentsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func SegmentsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := segmentsPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeSegments(result, plan); return err })
}
