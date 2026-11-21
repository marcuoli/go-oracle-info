package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// TemporaryGroup preserves a temporary tablespace group member in the current
// container. Group names are not physical tablespaces or additive allocations.
type TemporaryGroup struct {
	ReportingInstanceID session.Value
	GroupName           session.Value
	TablespaceName      session.Value
	ContainerID         *session.Value
}

// TemporaryGroupsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type TemporaryGroupsResult struct {
	Provenance Provenance
	Rows       []TemporaryGroup
}

// TemporaryGroupsStatement selects the reviewed source without SQL row truncation.
func TemporaryGroupsStatement(major int) (string, error) {
	plan, err := temporaryGroupsPlan(major)
	return plan.sql, err
}

func temporaryGroupsPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{{Name: "SOURCE_INST_ID", Kind: rowset.Integer}, {Name: "GROUP_NAME", Kind: rowset.Text}, {Name: "TABLESPACE_NAME", Kind: rowset.Text}}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID, v.GROUP_NAME AS GROUP_NAME, v.TABLESPACE_NAME AS TABLESPACE_NAME"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "DBA_TABLESPACE_GROUPS", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := ""
	order := "v.GROUP_NAME, v.TABLESPACE_NAME"
	plan.sql = "SELECT " + projection + " FROM DBA_TABLESPACE_GROUPS v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeTemporaryGroups validates every supplied row before publishing any typed slice.
func DecodeTemporaryGroups(result session.Result, major int) (TemporaryGroupsResult, error) {
	plan, err := temporaryGroupsPlan(major)
	if err != nil {
		return TemporaryGroupsResult{}, err
	}
	return decodeTemporaryGroups(result, plan)
}

func decodeTemporaryGroups(result session.Result, plan selection) (TemporaryGroupsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return TemporaryGroupsResult{}, err
	}
	rows := make([]TemporaryGroup, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = TemporaryGroup{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			GroupName:           row[index["GROUP_NAME"]],
			TablespaceName:      row[index["TABLESPACE_NAME"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return TemporaryGroupsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadTemporaryGroups executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadTemporaryGroups(ctx context.Context, reader Reader, request Request) (TemporaryGroupsResult, error) {
	plan, err := temporaryGroupsPlan(request.OracleMajor)
	if err != nil {
		return TemporaryGroupsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return TemporaryGroupsResult{}, err
	}
	facts, decodeErr := decodeTemporaryGroups(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return TemporaryGroupsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TemporaryGroupsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func TemporaryGroupsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := temporaryGroupsPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// TemporaryGroupsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func TemporaryGroupsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := temporaryGroupsPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeTemporaryGroups(result, plan); return err })
}
