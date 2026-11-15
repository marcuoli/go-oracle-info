package sequences

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Sequence retains stored/cache metadata. LAST_NUMBER is the last number written to disk, may exceed the last used value and must be ignored for session sequences. No NEXTVAL is evaluated.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Sequence struct {
	Owner       session.Value
	Name        session.Value
	MinValue    session.Value
	MaxValue    session.Value
	IncrementBy session.Value
	CycleFlag   session.Value
	OrderFlag   session.Value
	CacheSize   session.Value
	LastNumber  session.Value
	SessionFlag *session.Value
	KeepValue   *session.Value
	ScaleFlag   *session.Value
	ExtendFlag  *session.Value
	ShardedFlag *session.Value
}

// Result is one complete selected dictionary observation, in source order.
type Result struct {
	Provenance Provenance
	Records    []Sequence
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
	expressions = append(expressions, "t.SEQUENCE_OWNER AS SEQUENCE_OWNER")
	schema = append(schema, rowset.Column{Name: "SEQUENCE_OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.SEQUENCE_NAME AS SEQUENCE_NAME")
	schema = append(schema, rowset.Column{Name: "SEQUENCE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.MIN_VALUE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MIN_VALUE")
	schema = append(schema, rowset.Column{Name: "MIN_VALUE", Kind: rowset.Text})
	integers = append(integers, "MIN_VALUE")
	expressions = append(expressions, "TO_CHAR(t.MAX_VALUE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MAX_VALUE")
	schema = append(schema, rowset.Column{Name: "MAX_VALUE", Kind: rowset.Text})
	integers = append(integers, "MAX_VALUE")
	expressions = append(expressions, "TO_CHAR(t.INCREMENT_BY, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS INCREMENT_BY")
	schema = append(schema, rowset.Column{Name: "INCREMENT_BY", Kind: rowset.Text})
	integers = append(integers, "INCREMENT_BY")
	expressions = append(expressions, "t.CYCLE_FLAG AS CYCLE_FLAG")
	schema = append(schema, rowset.Column{Name: "CYCLE_FLAG", Kind: rowset.Text})
	expressions = append(expressions, "t.ORDER_FLAG AS ORDER_FLAG")
	schema = append(schema, rowset.Column{Name: "ORDER_FLAG", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.CACHE_SIZE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CACHE_SIZE")
	schema = append(schema, rowset.Column{Name: "CACHE_SIZE", Kind: rowset.Text})
	integers = append(integers, "CACHE_SIZE")
	expressions = append(expressions, "TO_CHAR(t.LAST_NUMBER, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS LAST_NUMBER")
	schema = append(schema, rowset.Column{Name: "LAST_NUMBER", Kind: rowset.Text})
	integers = append(integers, "LAST_NUMBER")
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.SESSION_FLAG AS SESSION_FLAG")
		schema = append(schema, rowset.Column{Name: "SESSION_FLAG", Kind: rowset.Text})
	}
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.KEEP_VALUE AS KEEP_VALUE")
		schema = append(schema, rowset.Column{Name: "KEEP_VALUE", Kind: rowset.Text})
	}
	if options.OracleMajor >= 18 {
		expressions = append(expressions, "t.SCALE_FLAG AS SCALE_FLAG")
		schema = append(schema, rowset.Column{Name: "SCALE_FLAG", Kind: rowset.Text})
	}
	if options.OracleMajor >= 18 {
		expressions = append(expressions, "t.EXTEND_FLAG AS EXTEND_FLAG")
		schema = append(schema, rowset.Column{Name: "EXTEND_FLAG", Kind: rowset.Text})
	}
	if options.OracleMajor >= 19 {
		expressions = append(expressions, "t.SHARDED_FLAG AS SHARDED_FLAG")
		schema = append(schema, rowset.Column{Name: "SHARDED_FLAG", Kind: rowset.Text})
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_SEQUENCES t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.SEQUENCE_OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.SEQUENCE_NAME", options.Names)
	statement += " ORDER BY t.SEQUENCE_OWNER,t.SEQUENCE_NAME"
	return selection{nameColumn: "SEQUENCE_NAME", ownerColumn: "SEQUENCE_OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_SEQUENCES", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
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
	records := make([]Sequence, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Sequence{Owner: row[index["SEQUENCE_OWNER"]], Name: row[index["SEQUENCE_NAME"]], MinValue: row[index["MIN_VALUE"]], MaxValue: row[index["MAX_VALUE"]], IncrementBy: row[index["INCREMENT_BY"]], CycleFlag: row[index["CYCLE_FLAG"]], OrderFlag: row[index["ORDER_FLAG"]], CacheSize: row[index["CACHE_SIZE"]], LastNumber: row[index["LAST_NUMBER"]]}
		if position, selected := index["SESSION_FLAG"]; selected {
			value := row[position]
			records[i].SessionFlag = &value
		}
		if position, selected := index["KEEP_VALUE"]; selected {
			value := row[position]
			records[i].KeepValue = &value
		}
		if position, selected := index["SCALE_FLAG"]; selected {
			value := row[position]
			records[i].ScaleFlag = &value
		}
		if position, selected := index["EXTEND_FLAG"]; selected {
			value := row[position]
			records[i].ExtendFlag = &value
		}
		if position, selected := index["SHARDED_FLAG"]; selected {
			value := row[position]
			records[i].ShardedFlag = &value
		}
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
