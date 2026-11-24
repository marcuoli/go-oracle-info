package asm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func instanceClause(ids []int64) (string, []any, error) {
	if len(ids) > 1000 {
		return "", nil, ErrInvalidRequest
	}
	seen := map[int64]bool{}
	args := make([]any, 0, len(ids))
	binds := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return "", nil, ErrInvalidRequest
		}
		seen[id] = true
		args = append(args, id)
		binds = append(binds, ":"+strconv.Itoa(len(args)))
	}
	if len(ids) == 0 {
		return "", args, nil
	}
	return "v.INST_ID IN (" + strings.Join(binds, ",") + ")", args, nil
}

// Filter selects reporting IDs and requires explicit source-role admission.
type Filter struct {
	Instances  []int64
	SourceRole SourceRole
}

// Request supplies admitted source version, role, instances and SQL reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

// Client preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// SourceRole is caller admission, not discovered. Database-instance rows describe ASM connections for open ASM files; ASM-instance rows identify clients. Version text stays raw; lexical min/max is not a numeric version comparison.
type Client struct {
	InstanceID        session.Value
	GroupNumber       session.Value
	InstanceName      session.Value
	DatabaseName      session.Value
	Status            session.Value
	SoftwareVersion   session.Value
	CompatibleVersion session.Value
	ContainerID       *session.Value
}

// ClientsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type ClientsResult struct {
	Provenance Provenance
	Rows       []Client
}

// ClientsStatement selects the reviewed source without SQL row truncation.
func ClientsStatement(major int, filter Filter) (string, []any, error) {
	plan, err := clientsPlan(major, filter)
	return plan.sql, plan.args, err
}

func clientsPlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "GROUP_NUMBER", Kind: rowset.Integer},
		{Name: "INSTANCE_NAME", Kind: rowset.Text},
		{Name: "DB_NAME", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "SOFTWARE_VERSION", Kind: rowset.Text},
		{Name: "COMPATIBLE_VERSION", Kind: rowset.Text},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.GROUP_NUMBER AS GROUP_NUMBER" + ", " +
		"v.INSTANCE_NAME AS INSTANCE_NAME" + ", " +
		"v.DB_NAME AS DB_NAME" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.SOFTWARE_VERSION AS SOFTWARE_VERSION" + ", " +
		"v.COMPATIBLE_VERSION AS COMPATIBLE_VERSION"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$ASM_CLIENT", OracleMajor: major, Scope: "qualified_visible_instances"}}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := clause
	if filter.SourceRole != DatabaseInstance && filter.SourceRole != ASMInstance {
		return selection{}, ErrInvalidRequest
	}
	plan.provenance.SourceRole = string(filter.SourceRole)
	order := "v.INST_ID, v.GROUP_NUMBER, v.INSTANCE_NAME, v.DB_NAME"
	plan.sql = "SELECT " + projection + " FROM GV$ASM_CLIENT v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeClients validates every supplied row before publishing any typed slice.
func DecodeClients(result session.Result, major int, filter Filter) (ClientsResult, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return ClientsResult{}, err
	}
	return decodeClients(result, plan)
}

func decodeClients(result session.Result, plan selection) (ClientsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ClientsResult{}, err
	}
	rows := make([]Client, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Client{
			InstanceID:        row[index["INST_ID"]],
			GroupNumber:       row[index["GROUP_NUMBER"]],
			InstanceName:      row[index["INSTANCE_NAME"]],
			DatabaseName:      row[index["DB_NAME"]],
			Status:            row[index["STATUS"]],
			SoftwareVersion:   row[index["SOFTWARE_VERSION"]],
			CompatibleVersion: row[index["COMPATIBLE_VERSION"]],
			ContainerID:       optional(row, index, "CON_ID"),
		}
	}
	return ClientsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadClients executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadClients(ctx context.Context, reader Reader, request Request) (ClientsResult, error) {
	plan, err := clientsPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return ClientsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return ClientsResult{}, err
	}
	facts, decodeErr := decodeClients(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ClientsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ClientsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func ClientsRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ClientsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func ClientsJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeClients(result, plan); return err })
}

// ClientsOptions carries the source selectors for the options-based fact APIs.
type ClientsOptions struct {
	OracleMajor int
	Filter      Filter
}

// ClientsStatementWithOptions selects the same source as ClientsStatement using one options value.
func ClientsStatementWithOptions(options ClientsOptions) (string, []any, error) {
	return ClientsStatement(options.OracleMajor, options.Filter)
}

// DecodeClientsWithOptions selects the same source as DecodeClients using one options value.
func DecodeClientsWithOptions(result session.Result, options ClientsOptions) (ClientsResult, error) {
	return DecodeClients(result, options.OracleMajor, options.Filter)
}

// ClientsRowsWithOptions selects the same source as ClientsRows using one options value.
func ClientsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ClientsOptions) (*sql.Rows, error) {
	return ClientsRows(ctx, q, options.OracleMajor, options.Filter)
}

// ClientsJSONWithOptions selects the same source as ClientsJSON using one options value.
func ClientsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ClientsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ClientsJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}
