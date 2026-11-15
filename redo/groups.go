package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Group preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Repeated reporting-instance rows are not additive physical allocations. SCNs
// remain decimal strings because they may exceed int64. BlockSize is omitted
// on 11g because the major cannot establish 11.2. NextChange/NextTime are selected
// from 18 onward because major 12 alone cannot establish the reviewed 12.2 source.
type Group struct {
	InstanceID     session.Value
	GroupNumber    session.Value
	ThreadNumber   session.Value
	SequenceNumber session.Value
	Bytes          session.Value
	BlockSize      *session.Value
	Members        session.Value
	Archived       session.Value
	Status         session.Value
	FirstChange    session.Value
	FirstTime      session.Value
	NextChange     *session.Value
	NextTime       *session.Value
	ContainerID    *session.Value
}

// GroupsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type GroupsResult struct {
	Provenance Provenance
	Rows       []Group
}

// GroupsStatement selects the reviewed source without SQL row truncation.
func GroupsStatement(major int, filter Filter) (string, []any, error) {
	plan, err := groupsPlan(major, filter)
	return plan.sql, plan.args, err
}
func groupsPlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "GROUP_NUMBER", Kind: rowset.Integer},
		{Name: "THREAD_NUMBER", Kind: rowset.Integer},
		{Name: "SEQUENCE_NUMBER", Kind: rowset.Integer},
		{Name: "BYTES", Kind: rowset.Integer},
		{Name: "MEMBERS", Kind: rowset.Integer},
		{Name: "ARCHIVED", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "FIRST_CHANGE", Kind: rowset.Text},
		{Name: "FIRST_TIME", Kind: rowset.Text},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.GROUP# AS GROUP_NUMBER" + ", " +
		"v.THREAD# AS THREAD_NUMBER" + ", " +
		"v.SEQUENCE# AS SEQUENCE_NUMBER" + ", " +
		"v.BYTES AS BYTES" + ", " +
		"v.MEMBERS AS MEMBERS" + ", " +
		"v.ARCHIVED AS ARCHIVED" + ", " +
		"v.STATUS AS STATUS" + ", " +
		exactNumber("v.FIRST_CHANGE#") + " AS FIRST_CHANGE" + ", " +
		localDate("v.FIRST_TIME") + " AS FIRST_TIME"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "BLOCK_SIZE", Kind: rowset.Integer})
		projection += ", v.BLOCKSIZE AS BLOCK_SIZE"
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$LOG", OracleMajor: major, Scope: "qualified_visible_instances"}}
	if major >= 12 {
		plan.provenance.OptionalColumns = []string{"BLOCK_SIZE"}
	}
	plan.integerText = []string{"FIRST_CHANGE"}
	if major >= 18 {
		plan.schema = append(plan.schema, rowset.Column{Name: "NEXT_CHANGE", Kind: rowset.Text}, rowset.Column{Name: "NEXT_TIME", Kind: rowset.Text})
		projection += ", " + exactNumber("v.NEXT_CHANGE#") + " AS NEXT_CHANGE, " + localDate("v.NEXT_TIME") + " AS NEXT_TIME"
		plan.integerText = append(plan.integerText, "NEXT_CHANGE")
		plan.provenance.OptionalColumns = append(plan.provenance.OptionalColumns, "NEXT_CHANGE", "NEXT_TIME")
	}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := clause
	order := "v.INST_ID, v.THREAD#, v.GROUP#"
	plan.sql = "SELECT " + projection + " FROM GV$LOG v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeGroups validates every supplied row before publishing any typed slice.
func DecodeGroups(result session.Result, major int, filter Filter) (GroupsResult, error) {
	plan, err := groupsPlan(major, filter)
	if err != nil {
		return GroupsResult{}, err
	}
	return decodeGroups(result, plan)
}
func decodeGroups(result session.Result, plan selection) (GroupsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return GroupsResult{}, err
	}
	rows := make([]Group, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Group{
			InstanceID:     row[index["INST_ID"]],
			GroupNumber:    row[index["GROUP_NUMBER"]],
			ThreadNumber:   row[index["THREAD_NUMBER"]],
			SequenceNumber: row[index["SEQUENCE_NUMBER"]],
			Bytes:          row[index["BYTES"]],
			BlockSize:      optional(row, index, "BLOCK_SIZE"),
			Members:        row[index["MEMBERS"]],
			Archived:       row[index["ARCHIVED"]],
			Status:         row[index["STATUS"]],
			FirstChange:    row[index["FIRST_CHANGE"]],
			FirstTime:      row[index["FIRST_TIME"]],
			NextChange:     optional(row, index, "NEXT_CHANGE"),
			NextTime:       optional(row, index, "NEXT_TIME"),
			ContainerID:    optional(row, index, "CON_ID"),
		}
	}
	return GroupsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadGroups executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadGroups(ctx context.Context, reader Reader, request Request) (GroupsResult, error) {
	plan, err := groupsPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return GroupsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return GroupsResult{}, err
	}
	facts, decodeErr := decodeGroups(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return GroupsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// GroupsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func GroupsRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := groupsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// GroupsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func GroupsJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := groupsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeGroups(result, plan); return err })
}
