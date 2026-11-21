package triggers

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

// Trigger retains enabled/disabled trigger status separately from object validity and LAST_DDL_TIME. Missing joined object metadata remains SQL NULL; no trigger body or DDL is executed.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Trigger struct {
	Owner           session.Value
	Name            session.Value
	TriggerType     session.Value
	TriggeringEvent session.Value
	TableOwner      session.Value
	TableName       session.Value
	TriggerStatus   session.Value
	ObjectStatus    session.Value
	LastDDLTime     session.Value
	BaseObjectType  session.Value
	ActionType      session.Value
}

// Result is one complete selected dictionary observation, in source order.
type Result struct {
	Provenance Provenance
	Records    []Trigger
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
	expressions = append(expressions, "t.TRIGGER_NAME AS TRIGGER_NAME")
	schema = append(schema, rowset.Column{Name: "TRIGGER_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.TRIGGER_TYPE AS TRIGGER_TYPE")
	schema = append(schema, rowset.Column{Name: "TRIGGER_TYPE", Kind: rowset.Text})
	expressions = append(expressions, "t.TRIGGERING_EVENT AS TRIGGERING_EVENT")
	schema = append(schema, rowset.Column{Name: "TRIGGERING_EVENT", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLE_OWNER AS TABLE_OWNER")
	schema = append(schema, rowset.Column{Name: "TABLE_OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLE_NAME AS TABLE_NAME")
	schema = append(schema, rowset.Column{Name: "TABLE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.STATUS AS TRIGGER_STATUS")
	schema = append(schema, rowset.Column{Name: "TRIGGER_STATUS", Kind: rowset.Text})
	expressions = append(expressions, "o.STATUS AS OBJECT_STATUS")
	schema = append(schema, rowset.Column{Name: "OBJECT_STATUS", Kind: rowset.Text})
	expressions = append(expressions, datetime.GregorianDate("o.LAST_DDL_TIME")+" AS LAST_DDL_TIME")
	schema = append(schema, rowset.Column{Name: "LAST_DDL_TIME", Kind: rowset.Text})
	expressions = append(expressions, "t.BASE_OBJECT_TYPE AS BASE_OBJECT_TYPE")
	schema = append(schema, rowset.Column{Name: "BASE_OBJECT_TYPE", Kind: rowset.Text})
	expressions = append(expressions, "t.ACTION_TYPE AS ACTION_TYPE")
	schema = append(schema, rowset.Column{Name: "ACTION_TYPE", Kind: rowset.Text})
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_TRIGGERS t LEFT JOIN DBA_OBJECTS o ON o.OWNER=t.OWNER AND o.OBJECT_NAME=t.TRIGGER_NAME AND o.OBJECT_TYPE='TRIGGER' AND o.SUBOBJECT_NAME IS NULL"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.TRIGGER_NAME", options.Names)
	statement += " ORDER BY t.OWNER,t.TRIGGER_NAME,o.OBJECT_ID"
	return selection{nameColumn: "TRIGGER_NAME", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_TRIGGERS+DBA_OBJECTS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
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
	records := make([]Trigger, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Trigger{Owner: row[index["OWNER"]], Name: row[index["TRIGGER_NAME"]], TriggerType: row[index["TRIGGER_TYPE"]], TriggeringEvent: row[index["TRIGGERING_EVENT"]], TableOwner: row[index["TABLE_OWNER"]], TableName: row[index["TABLE_NAME"]], TriggerStatus: row[index["TRIGGER_STATUS"]], ObjectStatus: row[index["OBJECT_STATUS"]], LastDDLTime: row[index["LAST_DDL_TIME"]], BaseObjectType: row[index["BASE_OBJECT_TYPE"]], ActionType: row[index["ACTION_TYPE"]]}
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
