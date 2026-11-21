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

// DirectColumnGrant retains column identity and grant options for direct user grants. A column grant never becomes an object-wide GRANT statement.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type DirectColumnGrant struct {
	Grantee    session.Value
	Owner      session.Value
	TableName  session.Value
	ColumnName session.Value
	Grantor    session.Value
	Privilege  session.Value
	Grantable  session.Value
	Common     *session.Value
	Inherited  *session.Value
}

// DirectColumnResult is one complete selected dictionary observation, in source order.
type DirectColumnResult struct {
	Provenance Provenance
	Records    []DirectColumnGrant
}

// DirectColumnStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func DirectColumnStatement(options Options) (string, []any, error) {
	p, e := directColumnPlan(options)
	return p.sql, p.args, e
}

func directColumnPlan(options Options) (selection, error) {
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
	expressions = append(expressions, "t.COLUMN_NAME AS COLUMN_NAME")
	schema = append(schema, rowset.Column{Name: "COLUMN_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.GRANTOR AS GRANTOR")
	schema = append(schema, rowset.Column{Name: "GRANTOR", Kind: rowset.Text})
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
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_COL_PRIVS t JOIN DBA_USERS u ON u.USERNAME=t.GRANTEE"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.GRANTEE", options.Names)
	statement += " ORDER BY t.OWNER,t.TABLE_NAME,t.COLUMN_NAME,t.GRANTEE,t.PRIVILEGE,t.GRANTOR"
	return selection{nameColumn: "GRANTEE", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_COL_PRIVS+DBA_USERS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeDirectColumn validates every row before publishing any records. It cannot prove
// transport completion; use ReadDirectColumn with an admitted complete Reader.
func DecodeDirectColumn(result session.Result, options Options) (DirectColumnResult, error) {
	plan, err := directColumnPlan(options)
	if err != nil {
		return DirectColumnResult{}, err
	}
	return decodeDirectColumn(result, plan)
}

func decodeDirectColumn(result session.Result, plan selection) (DirectColumnResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return DirectColumnResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]DirectColumnGrant, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = DirectColumnGrant{Grantee: row[index["GRANTEE"]], Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]], ColumnName: row[index["COLUMN_NAME"]], Grantor: row[index["GRANTOR"]], Privilege: row[index["PRIVILEGE"]], Grantable: row[index["GRANTABLE"]]}
		if position, selected := index["COMMON"]; selected {
			value := row[position]
			records[i].Common = &value
		}
		if position, selected := index["INHERITED"]; selected {
			value := row[position]
			records[i].Inherited = &value
		}
	}
	return DirectColumnResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadDirectColumn executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadDirectColumn(ctx context.Context, reader Reader, request Request) (DirectColumnResult, error) {
	plan, err := directColumnPlan(request.Options)
	if err != nil {
		return DirectColumnResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return DirectColumnResult{}, err
	}
	facts, de := decodeDirectColumn(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return DirectColumnResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// DirectColumnRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func DirectColumnRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := directColumnPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// DirectColumnJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func DirectColumnJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := directColumnPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
