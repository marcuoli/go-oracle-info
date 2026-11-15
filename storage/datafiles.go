package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Datafile preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// IncrementBy is Oracle blocks, not bytes. UserBytes excludes file metadata. No potential-capacity or used-space arithmetic is performed.
type Datafile struct {
	ReportingInstanceID session.Value
	FileID              session.Value
	FileName            session.Value
	TablespaceName      session.Value
	Bytes               session.Value
	Blocks              session.Value
	Status              session.Value
	RelativeFileNumber  session.Value
	Autoextensible      session.Value
	MaxBytes            session.Value
	MaxBlocks           session.Value
	IncrementBy         session.Value
	UserBytes           session.Value
	UserBlocks          session.Value
	OnlineStatus        session.Value
	ContainerID         *session.Value
}

// DatafilesResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type DatafilesResult struct {
	Provenance Provenance
	Rows       []Datafile
}

// DatafilesStatement selects the reviewed source without SQL row truncation.
func DatafilesStatement(major int) (string, error) {
	plan, err := datafilesPlan(major)
	return plan.sql, err
}
func datafilesPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "FILE_ID", Kind: rowset.Integer},
		{Name: "FILE_NAME", Kind: rowset.Text},
		{Name: "TABLESPACE_NAME", Kind: rowset.Text},
		{Name: "BYTES", Kind: rowset.Integer},
		{Name: "BLOCKS", Kind: rowset.Integer},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "RELATIVE_FNO", Kind: rowset.Integer},
		{Name: "AUTOEXTENSIBLE", Kind: rowset.Text},
		{Name: "MAXBYTES", Kind: rowset.Integer},
		{Name: "MAXBLOCKS", Kind: rowset.Integer},
		{Name: "INCREMENT_BY", Kind: rowset.Integer},
		{Name: "USER_BYTES", Kind: rowset.Integer},
		{Name: "USER_BLOCKS", Kind: rowset.Integer},
		{Name: "ONLINE_STATUS", Kind: rowset.Text},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.FILE_ID AS FILE_ID" + ", " +
		"v.FILE_NAME AS FILE_NAME" + ", " +
		"v.TABLESPACE_NAME AS TABLESPACE_NAME" + ", " +
		"v.BYTES AS BYTES" + ", " +
		"v.BLOCKS AS BLOCKS" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.RELATIVE_FNO AS RELATIVE_FNO" + ", " +
		"v.AUTOEXTENSIBLE AS AUTOEXTENSIBLE" + ", " +
		"v.MAXBYTES AS MAXBYTES" + ", " +
		"v.MAXBLOCKS AS MAXBLOCKS" + ", " +
		"v.INCREMENT_BY AS INCREMENT_BY" + ", " +
		"v.USER_BYTES AS USER_BYTES" + ", " +
		"v.USER_BLOCKS AS USER_BLOCKS" + ", " +
		"v.ONLINE_STATUS AS ONLINE_STATUS"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_DATA_FILES", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	order := "v.TABLESPACE_NAME, v.FILE_ID"
	plan.sql = "SELECT " + projection + " FROM DBA_DATA_FILES v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeDatafiles validates every supplied row before publishing any typed slice.
func DecodeDatafiles(result session.Result, major int) (DatafilesResult, error) {
	plan, err := datafilesPlan(major)
	if err != nil {
		return DatafilesResult{}, err
	}
	return decodeDatafiles(result, plan)
}
func decodeDatafiles(result session.Result, plan selection) (DatafilesResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return DatafilesResult{}, err
	}
	rows := make([]Datafile, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Datafile{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			FileID:              row[index["FILE_ID"]],
			FileName:            row[index["FILE_NAME"]],
			TablespaceName:      row[index["TABLESPACE_NAME"]],
			Bytes:               row[index["BYTES"]],
			Blocks:              row[index["BLOCKS"]],
			Status:              row[index["STATUS"]],
			RelativeFileNumber:  row[index["RELATIVE_FNO"]],
			Autoextensible:      row[index["AUTOEXTENSIBLE"]],
			MaxBytes:            row[index["MAXBYTES"]],
			MaxBlocks:           row[index["MAXBLOCKS"]],
			IncrementBy:         row[index["INCREMENT_BY"]],
			UserBytes:           row[index["USER_BYTES"]],
			UserBlocks:          row[index["USER_BLOCKS"]],
			OnlineStatus:        row[index["ONLINE_STATUS"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return DatafilesResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadDatafiles executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadDatafiles(ctx context.Context, reader Reader, request Request) (DatafilesResult, error) {
	plan, err := datafilesPlan(request.OracleMajor)
	if err != nil {
		return DatafilesResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return DatafilesResult{}, err
	}
	facts, decodeErr := decodeDatafiles(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return DatafilesResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatafilesRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func DatafilesRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := datafilesPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// DatafilesJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func DatafilesJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := datafilesPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeDatafiles(result, plan); return err })
}
