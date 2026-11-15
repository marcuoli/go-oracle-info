package sqlplan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Baseline retains read-only plan management metadata; flags remain raw and nullable. LAST_EXECUTED may lag actual use. Counts derive from the complete selected records, never an invented empty source.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Baseline struct {
	Signature         session.Value
	SQLHandle         session.Value
	PlanName          session.Value
	Creator           session.Value
	Origin            session.Value
	ParsingSchemaName session.Value
	Description       session.Value
	Version           session.Value
	Created           session.Value
	LastModified      session.Value
	LastExecuted      session.Value
	LastVerified      session.Value
	Enabled           session.Value
	Accepted          session.Value
	Fixed             session.Value
	Reproduced        session.Value
	Autopurge         session.Value
}

// BaselineResult is one complete selected dictionary observation, in source order.
type BaselineResult struct {
	Provenance Provenance
	Records    []Baseline
}

// BaselineStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func BaselineStatement(options Options) (string, []any, error) {
	p, e := baselinePlan(options)
	return p.sql, p.args, e
}
func baselinePlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "TO_CHAR(t.SIGNATURE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SIGNATURE")
	schema = append(schema, rowset.Column{Name: "SIGNATURE", Kind: rowset.Text})
	integers = append(integers, "SIGNATURE")
	expressions = append(expressions, "t.SQL_HANDLE AS SQL_HANDLE")
	schema = append(schema, rowset.Column{Name: "SQL_HANDLE", Kind: rowset.Text})
	expressions = append(expressions, "t.PLAN_NAME AS PLAN_NAME")
	schema = append(schema, rowset.Column{Name: "PLAN_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.CREATOR AS CREATOR")
	schema = append(schema, rowset.Column{Name: "CREATOR", Kind: rowset.Text})
	expressions = append(expressions, "t.ORIGIN AS ORIGIN")
	schema = append(schema, rowset.Column{Name: "ORIGIN", Kind: rowset.Text})
	expressions = append(expressions, "t.PARSING_SCHEMA_NAME AS PARSING_SCHEMA_NAME")
	schema = append(schema, rowset.Column{Name: "PARSING_SCHEMA_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.DESCRIPTION AS DESCRIPTION")
	schema = append(schema, rowset.Column{Name: "DESCRIPTION", Kind: rowset.Text})
	expressions = append(expressions, "t.VERSION AS VERSION")
	schema = append(schema, rowset.Column{Name: "VERSION", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.CREATED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS CREATED")
	schema = append(schema, rowset.Column{Name: "CREATED", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.LAST_MODIFIED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS LAST_MODIFIED")
	schema = append(schema, rowset.Column{Name: "LAST_MODIFIED", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.LAST_EXECUTED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS LAST_EXECUTED")
	schema = append(schema, rowset.Column{Name: "LAST_EXECUTED", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.LAST_VERIFIED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS LAST_VERIFIED")
	schema = append(schema, rowset.Column{Name: "LAST_VERIFIED", Kind: rowset.Text})
	expressions = append(expressions, "t.ENABLED AS ENABLED")
	schema = append(schema, rowset.Column{Name: "ENABLED", Kind: rowset.Text})
	expressions = append(expressions, "t.ACCEPTED AS ACCEPTED")
	schema = append(schema, rowset.Column{Name: "ACCEPTED", Kind: rowset.Text})
	expressions = append(expressions, "t.FIXED AS FIXED")
	schema = append(schema, rowset.Column{Name: "FIXED", Kind: rowset.Text})
	expressions = append(expressions, "t.REPRODUCED AS REPRODUCED")
	schema = append(schema, rowset.Column{Name: "REPRODUCED", Kind: rowset.Text})
	expressions = append(expressions, "t.AUTOPURGE AS AUTOPURGE")
	schema = append(schema, rowset.Column{Name: "AUTOPURGE", Kind: rowset.Text})
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_SQL_PLAN_BASELINES t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.PARSING_SCHEMA_NAME", options.Owners)
	statement, args = scopeFilter(statement, args, "t.PLAN_NAME", options.Names)
	statement += " ORDER BY t.SQL_HANDLE,t.PLAN_NAME"
	return selection{nameColumn: "PLAN_NAME", ownerColumn: "PARSING_SCHEMA_NAME", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_SQL_PLAN_BASELINES", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeBaseline validates every row before publishing any records. It cannot prove
// transport completion; use ReadBaseline with an admitted complete Reader.
func DecodeBaseline(result session.Result, options Options) (BaselineResult, error) {
	plan, err := baselinePlan(options)
	if err != nil {
		return BaselineResult{}, err
	}
	return decodeBaseline(result, plan)
}
func decodeBaseline(result session.Result, plan selection) (BaselineResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return BaselineResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]Baseline, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Baseline{Signature: row[index["SIGNATURE"]], SQLHandle: row[index["SQL_HANDLE"]], PlanName: row[index["PLAN_NAME"]], Creator: row[index["CREATOR"]], Origin: row[index["ORIGIN"]], ParsingSchemaName: row[index["PARSING_SCHEMA_NAME"]], Description: row[index["DESCRIPTION"]], Version: row[index["VERSION"]], Created: row[index["CREATED"]], LastModified: row[index["LAST_MODIFIED"]], LastExecuted: row[index["LAST_EXECUTED"]], LastVerified: row[index["LAST_VERIFIED"]], Enabled: row[index["ENABLED"]], Accepted: row[index["ACCEPTED"]], Fixed: row[index["FIXED"]], Reproduced: row[index["REPRODUCED"]], Autopurge: row[index["AUTOPURGE"]]}
	}
	return BaselineResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadBaseline executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadBaseline(ctx context.Context, reader Reader, request Request) (BaselineResult, error) {
	plan, err := baselinePlan(request.Options)
	if err != nil {
		return BaselineResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return BaselineResult{}, err
	}
	facts, de := decodeBaseline(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return BaselineResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// BaselineRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func BaselineRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := baselinePlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// BaselineJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func BaselineJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := baselinePlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
