package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Member preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// A NULL status is retained; duplicate GV observations are not distinct physical members.
type Member struct {
	InstanceID                session.Value
	GroupNumber               session.Value
	Status                    session.Value
	Type                      session.Value
	Member                    session.Value
	IsRecoveryDestinationFile session.Value
	ContainerID               *session.Value
}

// MembersResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type MembersResult struct {
	Provenance Provenance
	Rows       []Member
}

// MembersStatement selects the reviewed source without SQL row truncation.
func MembersStatement(major int, filter Filter) (string, []any, error) {
	plan, err := membersPlan(major, filter)
	return plan.sql, plan.args, err
}
func membersPlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "GROUP_NUMBER", Kind: rowset.Integer},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "TYPE", Kind: rowset.Text},
		{Name: "MEMBER", Kind: rowset.Text},
		{Name: "IS_RECOVERY_DEST_FILE", Kind: rowset.Text},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.GROUP# AS GROUP_NUMBER" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.TYPE AS TYPE" + ", " +
		"v.MEMBER AS MEMBER" + ", " +
		"v.IS_RECOVERY_DEST_FILE AS IS_RECOVERY_DEST_FILE"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$LOGFILE", OracleMajor: major, Scope: "qualified_visible_instances"}}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := clause
	order := "v.INST_ID, v.GROUP#, v.MEMBER"
	plan.sql = "SELECT " + projection + " FROM GV$LOGFILE v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeMembers validates every supplied row before publishing any typed slice.
func DecodeMembers(result session.Result, major int, filter Filter) (MembersResult, error) {
	plan, err := membersPlan(major, filter)
	if err != nil {
		return MembersResult{}, err
	}
	return decodeMembers(result, plan)
}
func decodeMembers(result session.Result, plan selection) (MembersResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return MembersResult{}, err
	}
	rows := make([]Member, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Member{
			InstanceID:                row[index["INST_ID"]],
			GroupNumber:               row[index["GROUP_NUMBER"]],
			Status:                    row[index["STATUS"]],
			Type:                      row[index["TYPE"]],
			Member:                    row[index["MEMBER"]],
			IsRecoveryDestinationFile: row[index["IS_RECOVERY_DEST_FILE"]],
			ContainerID:               optional(row, index, "CON_ID"),
		}
	}
	return MembersResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadMembers executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadMembers(ctx context.Context, reader Reader, request Request) (MembersResult, error) {
	plan, err := membersPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return MembersResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return MembersResult{}, err
	}
	facts, decodeErr := decodeMembers(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return MembersResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// MembersRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func MembersRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := membersPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// MembersJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func MembersJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := membersPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeMembers(result, plan); return err })
}
