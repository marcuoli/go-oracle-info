package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FreeExtent preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// An absent row may mean no free extent or unavailable offline-file extent information. It never independently establishes zero free space.
type FreeExtent struct {
	ReportingInstanceID session.Value
	TablespaceName      session.Value
	FileID              session.Value
	BlockID             session.Value
	Bytes               session.Value
	Blocks              session.Value
	RelativeFileNumber  session.Value
	ContainerID         *session.Value
}

// FreeExtentsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type FreeExtentsResult struct {
	Provenance Provenance
	Rows       []FreeExtent
}

// FreeExtentsStatement selects the reviewed source without SQL row truncation.
func FreeExtentsStatement(major int) (string, error) {
	plan, err := freeExtentsPlan(major)
	return plan.sql, err
}
func freeExtentsPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "TABLESPACE_NAME", Kind: rowset.Text},
		{Name: "FILE_ID", Kind: rowset.Integer},
		{Name: "BLOCK_ID", Kind: rowset.Integer},
		{Name: "BYTES", Kind: rowset.Integer},
		{Name: "BLOCKS", Kind: rowset.Integer},
		{Name: "RELATIVE_FNO", Kind: rowset.Integer},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.TABLESPACE_NAME AS TABLESPACE_NAME" + ", " +
		"v.FILE_ID AS FILE_ID" + ", " +
		"v.BLOCK_ID AS BLOCK_ID" + ", " +
		"v.BYTES AS BYTES" + ", " +
		"v.BLOCKS AS BLOCKS" + ", " +
		"v.RELATIVE_FNO AS RELATIVE_FNO"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_FREE_SPACE", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	order := "v.TABLESPACE_NAME, v.FILE_ID, v.BLOCK_ID"
	plan.sql = "SELECT " + projection + " FROM DBA_FREE_SPACE v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeFreeExtents validates every supplied row before publishing any typed slice.
func DecodeFreeExtents(result session.Result, major int) (FreeExtentsResult, error) {
	plan, err := freeExtentsPlan(major)
	if err != nil {
		return FreeExtentsResult{}, err
	}
	return decodeFreeExtents(result, plan)
}
func decodeFreeExtents(result session.Result, plan selection) (FreeExtentsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return FreeExtentsResult{}, err
	}
	rows := make([]FreeExtent, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = FreeExtent{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			TablespaceName:      row[index["TABLESPACE_NAME"]],
			FileID:              row[index["FILE_ID"]],
			BlockID:             row[index["BLOCK_ID"]],
			Bytes:               row[index["BYTES"]],
			Blocks:              row[index["BLOCKS"]],
			RelativeFileNumber:  row[index["RELATIVE_FNO"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return FreeExtentsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadFreeExtents executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadFreeExtents(ctx context.Context, reader Reader, request Request) (FreeExtentsResult, error) {
	plan, err := freeExtentsPlan(request.OracleMajor)
	if err != nil {
		return FreeExtentsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return FreeExtentsResult{}, err
	}
	facts, decodeErr := decodeFreeExtents(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return FreeExtentsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FreeExtentsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func FreeExtentsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := freeExtentsPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// FreeExtentsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func FreeExtentsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := freeExtentsPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeFreeExtents(result, plan); return err })
}
