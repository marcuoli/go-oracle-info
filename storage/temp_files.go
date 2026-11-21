package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Tempfile preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// IncludeLocal explicitly admits SHARED and owning INST_ID on Oracle 18+. A NULL owning instance describes a shared tempfile; it is independent of the reporting instance.
type Tempfile struct {
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
	ContainerID         *session.Value
	Shared              *session.Value
	OwningInstanceID    *session.Value
}

// TempfilesResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type TempfilesResult struct {
	Provenance Provenance
	Rows       []Tempfile
}

// TempfilesStatement selects the reviewed source without SQL row truncation.
func TempfilesStatement(options TempfileOptions) (string, error) {
	plan, err := tempfilesPlan(options)
	return plan.sql, err
}

func tempfilesPlan(options TempfileOptions) (selection, error) {
	major := options.OracleMajor
	if options.IncludeLocal && major < 18 {
		return selection{}, ErrUnsupportedVersion
	}
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
		"v.USER_BLOCKS AS USER_BLOCKS"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	if options.IncludeLocal {
		schema = append(schema, rowset.Column{Name: "SHARED", Kind: rowset.Text}, rowset.Column{Name: "INST_ID", Kind: rowset.Integer})
		projection += ", v.SHARED AS SHARED, v.INST_ID AS INST_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_TEMP_FILES", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	if options.IncludeLocal {
		plan.provenance.OptionalColumns = []string{"SHARED", "INST_ID"}
	}
	order := "v.TABLESPACE_NAME, v.FILE_ID"
	plan.sql = "SELECT " + projection + " FROM DBA_TEMP_FILES v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeTempfiles validates every supplied row before publishing any typed slice.
func DecodeTempfiles(result session.Result, options TempfileOptions) (TempfilesResult, error) {
	plan, err := tempfilesPlan(options)
	if err != nil {
		return TempfilesResult{}, err
	}
	return decodeTempfiles(result, plan)
}

func decodeTempfiles(result session.Result, plan selection) (TempfilesResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return TempfilesResult{}, err
	}
	rows := make([]Tempfile, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Tempfile{
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
			ContainerID:         optional(row, index, "CON_ID"),
			Shared:              optional(row, index, "SHARED"),
			OwningInstanceID:    optional(row, index, "INST_ID"),
		}
	}
	return TempfilesResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadTempfiles executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadTempfiles(ctx context.Context, reader Reader, request TempfileRequest) (TempfilesResult, error) {
	plan, err := tempfilesPlan(request.Options)
	if err != nil {
		return TempfilesResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return TempfilesResult{}, err
	}
	facts, decodeErr := decodeTempfiles(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return TempfilesResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TempfilesRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func TempfilesRows(ctx context.Context, q rowset.Queryer, options TempfileOptions) (*sql.Rows, error) {
	plan, err := tempfilesPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// TempfilesJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func TempfilesJSON(ctx context.Context, q rowset.Queryer, options TempfileOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := tempfilesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeTempfiles(result, plan); return err })
}
