package grants

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// RoleGrant retains ROLE_TAB_PRIVS facts for roles accessible to the current user, including column identity. It does not claim complete DBA role visibility or invent a grantor absent from this source.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type RoleGrant struct {
	Common     *session.Value
	Inherited  *session.Value
	Role       session.Value
	Owner      session.Value
	TableName  session.Value
	ColumnName session.Value
	Privilege  session.Value
	Grantable  session.Value
}

// RoleResult is one complete selected dictionary observation, in source order.
type RoleResult struct {
	Provenance Provenance
	Records    []RoleGrant
}

// RoleStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func RoleStatement(options Options) (string, []any, error) {
	p, e := rolePlan(options)
	return p.sql, p.args, e
}
func rolePlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.ROLE AS ROLE")
	schema = append(schema, rowset.Column{Name: "ROLE", Kind: rowset.Text})
	expressions = append(expressions, "t.OWNER AS OWNER")
	schema = append(schema, rowset.Column{Name: "OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLE_NAME AS TABLE_NAME")
	schema = append(schema, rowset.Column{Name: "TABLE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.COLUMN_NAME AS COLUMN_NAME")
	schema = append(schema, rowset.Column{Name: "COLUMN_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.PRIVILEGE AS PRIVILEGE")
	schema = append(schema, rowset.Column{Name: "PRIVILEGE", Kind: rowset.Text})
	expressions = append(expressions, "t.GRANTABLE AS GRANTABLE")
	schema = append(schema, rowset.Column{Name: "GRANTABLE", Kind: rowset.Text})
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.COMMON AS COMMON")
		schema = append(schema, rowset.Column{Name: "COMMON", Kind: rowset.Text})
	}
	if options.OracleMajor >= 18 {
		expressions = append(expressions, "t.INHERITED AS INHERITED")
		schema = append(schema, rowset.Column{Name: "INHERITED", Kind: rowset.Text})
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "ROLE_TAB_PRIVS t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.ROLE", options.Names)
	statement += " ORDER BY t.ROLE,t.OWNER,t.TABLE_NAME,t.COLUMN_NAME,t.PRIVILEGE"
	return selection{nameColumn: "ROLE", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "ROLE_TAB_PRIVS", OracleMajor: options.OracleMajor, Scope: "connected_user_accessible_roles_only", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeRole validates every row before publishing any records. It cannot prove
// transport completion; use ReadRole with an admitted complete Reader.
func DecodeRole(result session.Result, options Options) (RoleResult, error) {
	plan, err := rolePlan(options)
	if err != nil {
		return RoleResult{}, err
	}
	return decodeRole(result, plan)
}
func decodeRole(result session.Result, plan selection) (RoleResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return RoleResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]RoleGrant, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = RoleGrant{Role: row[index["ROLE"]], Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]], ColumnName: row[index["COLUMN_NAME"]], Privilege: row[index["PRIVILEGE"]], Grantable: row[index["GRANTABLE"]]}
		if j, ok := index["COMMON"]; ok {
			v := row[j]
			records[i].Common = &v
		}
		if j, ok := index["INHERITED"]; ok {
			v := row[j]
			records[i].Inherited = &v
		}

	}
	return RoleResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadRole executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadRole(ctx context.Context, reader Reader, request Request) (RoleResult, error) {
	plan, err := rolePlan(request.Options)
	if err != nil {
		return RoleResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return RoleResult{}, err
	}
	facts, de := decodeRole(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return RoleResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// RoleRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func RoleRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := rolePlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// RoleJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func RoleJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := rolePlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
