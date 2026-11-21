package dblinks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Link retains declared destination and account-name metadata, without retrieving credentials or contacting any remote endpoint.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Link struct {
	Owner    session.Value
	Name     session.Value
	Username session.Value
	Host     session.Value
	Created  session.Value
}

// Result is one complete selected dictionary observation, in source order.
type Result struct {
	Provenance Provenance
	Records    []Link
}

// Statement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func Statement(options Options) (string, []any, error) {
	p, e := inventoryPlan(options)
	return p.sql, p.args, e
}
func inventoryPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.OWNER AS OWNER")
	schema = append(schema, rowset.Column{Name: "OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.DB_LINK AS DB_LINK")
	schema = append(schema, rowset.Column{Name: "DB_LINK", Kind: rowset.Text})
	expressions = append(expressions, "t.USERNAME AS USERNAME")
	schema = append(schema, rowset.Column{Name: "USERNAME", Kind: rowset.Text})
	expressions = append(expressions, "t.HOST AS HOST")
	schema = append(schema, rowset.Column{Name: "HOST", Kind: rowset.Text})
	expressions = append(expressions, datetime.GregorianDate("t.CREATED")+" AS CREATED")
	schema = append(schema, rowset.Column{Name: "CREATED", Kind: rowset.Text})
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_DB_LINKS t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.DB_LINK", options.Names)
	statement += " ORDER BY t.OWNER,t.DB_LINK"
	return selection{nameColumn: "DB_LINK", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_DB_LINKS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// Decode validates every row before publishing any records. It cannot prove
// transport completion; use Read with an admitted complete Reader.
func Decode(result session.Result, options Options) (Result, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return Result{}, err
	}
	return decodeInventory(result, plan)
}
func decodeInventory(result session.Result, plan selection) (Result, error) {
	index, err := columns(result, plan)
	if err != nil {
		return Result{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]Link, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Link{Owner: row[index["OWNER"]], Name: row[index["DB_LINK"]], Username: row[index["USERNAME"]], Host: row[index["HOST"]], Created: row[index["CREATED"]]}
	}
	return Result{Provenance: plan.provenance, Records: records}, nil
}

// Read executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	plan, err := inventoryPlan(request.Options)
	if err != nil {
		return Result{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return Result{}, err
	}
	facts, de := decodeInventory(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return Result{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// JSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// InventoryStatement is the named form of Statement.
func InventoryStatement(options Options) (string, []any, error) {
	return Statement(options)
}

// DecodeInventory is the named form of Decode.
func DecodeInventory(result session.Result, options Options) (Result, error) {
	return Decode(result, options)
}

// ReadInventory is the named form of Read.
func ReadInventory(ctx context.Context, reader Reader, request Request) (Result, error) {
	return Read(ctx, reader, request)
}

// InventoryRows is the named form of Rows.
func InventoryRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	return Rows(ctx, q, options)
}

// InventoryJSON is the named form of JSON.
func InventoryJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options, limits)
}
