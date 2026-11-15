package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Tablespace preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Undo and temporary tablespaces remain in the same unfiltered inventory. BlockSize is bytes per Oracle block.
type Tablespace struct {
	ReportingInstanceID    session.Value
	Name                   session.Value
	BlockSize              session.Value
	Status                 session.Value
	Contents               session.Value
	Retention              session.Value
	Bigfile                session.Value
	Logging                session.Value
	ForceLogging           session.Value
	ExtentManagement       session.Value
	AllocationType         session.Value
	SegmentSpaceManagement session.Value
	Encrypted              session.Value
	ContainerID            *session.Value
}

// TablespacesResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type TablespacesResult struct {
	Provenance Provenance
	Rows       []Tablespace
}

// TablespacesStatement selects the reviewed source without SQL row truncation.
func TablespacesStatement(major int) (string, error) {
	plan, err := tablespacesPlan(major)
	return plan.sql, err
}
func tablespacesPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "TABLESPACE_NAME", Kind: rowset.Text},
		{Name: "BLOCK_SIZE", Kind: rowset.Integer},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "CONTENTS", Kind: rowset.Text},
		{Name: "RETENTION", Kind: rowset.Text},
		{Name: "BIGFILE", Kind: rowset.Text},
		{Name: "LOGGING", Kind: rowset.Text},
		{Name: "FORCE_LOGGING", Kind: rowset.Text},
		{Name: "EXTENT_MANAGEMENT", Kind: rowset.Text},
		{Name: "ALLOCATION_TYPE", Kind: rowset.Text},
		{Name: "SEGMENT_SPACE_MANAGEMENT", Kind: rowset.Text},
		{Name: "ENCRYPTED", Kind: rowset.Text},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.TABLESPACE_NAME AS TABLESPACE_NAME" + ", " +
		"v.BLOCK_SIZE AS BLOCK_SIZE" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.CONTENTS AS CONTENTS" + ", " +
		"v.RETENTION AS RETENTION" + ", " +
		"v.BIGFILE AS BIGFILE" + ", " +
		"v.LOGGING AS LOGGING" + ", " +
		"v.FORCE_LOGGING AS FORCE_LOGGING" + ", " +
		"v.EXTENT_MANAGEMENT AS EXTENT_MANAGEMENT" + ", " +
		"v.ALLOCATION_TYPE AS ALLOCATION_TYPE" + ", " +
		"v.SEGMENT_SPACE_MANAGEMENT AS SEGMENT_SPACE_MANAGEMENT" + ", " +
		"v.ENCRYPTED AS ENCRYPTED"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_TABLESPACES", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	order := "v.TABLESPACE_NAME"
	plan.sql = "SELECT " + projection + " FROM DBA_TABLESPACES v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeTablespaces validates every supplied row before publishing any typed slice.
func DecodeTablespaces(result session.Result, major int) (TablespacesResult, error) {
	plan, err := tablespacesPlan(major)
	if err != nil {
		return TablespacesResult{}, err
	}
	return decodeTablespaces(result, plan)
}
func decodeTablespaces(result session.Result, plan selection) (TablespacesResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return TablespacesResult{}, err
	}
	rows := make([]Tablespace, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Tablespace{
			ReportingInstanceID:    row[index["SOURCE_INST_ID"]],
			Name:                   row[index["TABLESPACE_NAME"]],
			BlockSize:              row[index["BLOCK_SIZE"]],
			Status:                 row[index["STATUS"]],
			Contents:               row[index["CONTENTS"]],
			Retention:              row[index["RETENTION"]],
			Bigfile:                row[index["BIGFILE"]],
			Logging:                row[index["LOGGING"]],
			ForceLogging:           row[index["FORCE_LOGGING"]],
			ExtentManagement:       row[index["EXTENT_MANAGEMENT"]],
			AllocationType:         row[index["ALLOCATION_TYPE"]],
			SegmentSpaceManagement: row[index["SEGMENT_SPACE_MANAGEMENT"]],
			Encrypted:              row[index["ENCRYPTED"]],
			ContainerID:            optional(row, index, "CON_ID"),
		}
	}
	return TablespacesResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadTablespaces executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadTablespaces(ctx context.Context, reader Reader, request Request) (TablespacesResult, error) {
	plan, err := tablespacesPlan(request.OracleMajor)
	if err != nil {
		return TablespacesResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return TablespacesResult{}, err
	}
	facts, decodeErr := decodeTablespaces(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return TablespacesResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TablespacesRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func TablespacesRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := tablespacesPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// TablespacesJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func TablespacesJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := tablespacesPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeTablespaces(result, plan); return err })
}
