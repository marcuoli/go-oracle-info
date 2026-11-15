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

// DirectGrant retains object grants to database users, including grantor, hierarchy and grant option. Owners/Names are explicit scope, not a built-in application-user exclusion policy.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type DirectGrant struct {
	Grantee   session.Value
	Owner     session.Value
	TableName session.Value
	Grantor   session.Value
	Privilege session.Value
	Grantable session.Value
	Hierarchy session.Value
	Common    *session.Value
	Inherited *session.Value
}

// DirectResult is one complete selected dictionary observation, in source order.
type DirectResult struct {
	Provenance Provenance
	Records    []DirectGrant
}

// DirectStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func DirectStatement(options Options) (string, []any, error) {
	p, e := directPlan(options)
	return p.sql, p.args, e
}
func directPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.GRANTEE AS GRANTEE")
	schema = append(schema, rowset.Column{Name: "GRANTEE", Kind: rowset.Text})
	expressions = append(expressions, "t.OWNER AS OWNER")
	schema = append(schema, rowset.Column{Name: "OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLE_NAME AS TABLE_NAME")
	schema = append(schema, rowset.Column{Name: "TABLE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.GRANTOR AS GRANTOR")
	schema = append(schema, rowset.Column{Name: "GRANTOR", Kind: rowset.Text})
	expressions = append(expressions, "t.PRIVILEGE AS PRIVILEGE")
	schema = append(schema, rowset.Column{Name: "PRIVILEGE", Kind: rowset.Text})
	expressions = append(expressions, "t.GRANTABLE AS GRANTABLE")
	schema = append(schema, rowset.Column{Name: "GRANTABLE", Kind: rowset.Text})
	expressions = append(expressions, "t.HIERARCHY AS HIERARCHY")
	schema = append(schema, rowset.Column{Name: "HIERARCHY", Kind: rowset.Text})
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.COMMON AS COMMON")
		schema = append(schema, rowset.Column{Name: "COMMON", Kind: rowset.Text})
	}
	if options.OracleMajor >= 18 {
		expressions = append(expressions, "t.INHERITED AS INHERITED")
		schema = append(schema, rowset.Column{Name: "INHERITED", Kind: rowset.Text})
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_TAB_PRIVS t JOIN DBA_USERS u ON u.USERNAME=t.GRANTEE"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.GRANTEE", options.Names)
	statement += " ORDER BY t.OWNER,t.TABLE_NAME,t.GRANTEE,t.PRIVILEGE,t.GRANTOR"
	return selection{nameColumn: "GRANTEE", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_TAB_PRIVS+DBA_USERS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeDirect validates every row before publishing any records. It cannot prove
// transport completion; use ReadDirect with an admitted complete Reader.
func DecodeDirect(result session.Result, options Options) (DirectResult, error) {
	plan, err := directPlan(options)
	if err != nil {
		return DirectResult{}, err
	}
	return decodeDirect(result, plan)
}
func decodeDirect(result session.Result, plan selection) (DirectResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return DirectResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]DirectGrant, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = DirectGrant{Grantee: row[index["GRANTEE"]], Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]], Grantor: row[index["GRANTOR"]], Privilege: row[index["PRIVILEGE"]], Grantable: row[index["GRANTABLE"]], Hierarchy: row[index["HIERARCHY"]]}
		if position, selected := index["COMMON"]; selected {
			value := row[position]
			records[i].Common = &value
		}
		if position, selected := index["INHERITED"]; selected {
			value := row[position]
			records[i].Inherited = &value
		}
	}
	return DirectResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadDirect executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadDirect(ctx context.Context, reader Reader, request Request) (DirectResult, error) {
	plan, err := directPlan(request.Options)
	if err != nil {
		return DirectResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return DirectResult{}, err
	}
	facts, de := decodeDirect(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return DirectResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// DirectRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func DirectRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := directPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// DirectJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func DirectJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := directPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
