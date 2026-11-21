package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// LongColumn preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// This identifies columns, not LONG payload bytes. Joining to segment owner/name attributes whole containing segments and may miss clustered/nested storage. Deduplicate tables before summing; never count a segment once per LONG column.
type LongColumn struct {
	ReportingInstanceID session.Value
	Owner               session.Value
	TableName           session.Value
	ColumnName          session.Value
	DataType            session.Value
	ColumnID            session.Value
	ContainerID         *session.Value
}

// LongColumnsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type LongColumnsResult struct {
	Provenance Provenance
	Rows       []LongColumn
}

// LongColumnsStatement selects the reviewed source without SQL row truncation.
func LongColumnsStatement(major int) (string, error) {
	plan, err := longColumnsPlan(major)
	return plan.sql, err
}

func longColumnsPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "OWNER", Kind: rowset.Text},
		{Name: "TABLE_NAME", Kind: rowset.Text},
		{Name: "COLUMN_NAME", Kind: rowset.Text},
		{Name: "DATA_TYPE", Kind: rowset.Text},
		{Name: "COLUMN_ID", Kind: rowset.Integer},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.OWNER AS OWNER" + ", " +
		"v.TABLE_NAME AS TABLE_NAME" + ", " +
		"v.COLUMN_NAME AS COLUMN_NAME" + ", " +
		"v.DATA_TYPE AS DATA_TYPE" + ", " +
		"v.COLUMN_ID AS COLUMN_ID"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_TAB_COLUMNS", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := "v.DATA_TYPE IN ('LONG','LONG RAW')"
	order := "v.OWNER, v.TABLE_NAME, v.COLUMN_ID, v.COLUMN_NAME"
	plan.sql = "SELECT " + projection + " FROM DBA_TAB_COLUMNS v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeLongColumns validates every supplied row before publishing any typed slice.
func DecodeLongColumns(result session.Result, major int) (LongColumnsResult, error) {
	plan, err := longColumnsPlan(major)
	if err != nil {
		return LongColumnsResult{}, err
	}
	return decodeLongColumns(result, plan)
}

func decodeLongColumns(result session.Result, plan selection) (LongColumnsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return LongColumnsResult{}, err
	}
	for _, row := range result.Rows {
		v := row[index["DATA_TYPE"]]
		if v.Null || (v.Text != "LONG" && v.Text != "LONG RAW") {
			return LongColumnsResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
		}
	}
	rows := make([]LongColumn, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = LongColumn{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			Owner:               row[index["OWNER"]],
			TableName:           row[index["TABLE_NAME"]],
			ColumnName:          row[index["COLUMN_NAME"]],
			DataType:            row[index["DATA_TYPE"]],
			ColumnID:            row[index["COLUMN_ID"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return LongColumnsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadLongColumns executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadLongColumns(ctx context.Context, reader Reader, request Request) (LongColumnsResult, error) {
	plan, err := longColumnsPlan(request.OracleMajor)
	if err != nil {
		return LongColumnsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return LongColumnsResult{}, err
	}
	facts, decodeErr := decodeLongColumns(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return LongColumnsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// LongColumnsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func LongColumnsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := longColumnsPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// LongColumnsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func LongColumnsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := longColumnsPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeLongColumns(result, plan); return err })
}
