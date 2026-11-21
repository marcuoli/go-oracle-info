package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Table retains recorded optimizer statistics for every selected DBA_TABLES row, including temporary, nested, secondary and IOT overflow entries. NUM_ROWS is nullable/stale statistics, never a live count.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Table struct {
	Owner            session.Value
	TableName        session.Value
	TablespaceName   session.Value
	Status           session.Value
	Temporary        session.Value
	Nested           session.Value
	Secondary        session.Value
	IOTType          session.Value
	IOTName          session.Value
	Partitioned      session.Value
	Duration         session.Value
	NumRows          session.Value
	Blocks           session.Value
	EmptyBlocks      session.Value
	AverageRowLength session.Value
	SampleSize       session.Value
	LastAnalyzed     session.Value
	GlobalStats      session.Value
	UserStats        session.Value
	Degree           session.Value
	Instances        session.Value
}

// TableMetadataResult is one complete selected dictionary observation, in source order.
type TableMetadataResult struct {
	Provenance Provenance
	Records    []Table
}

// TableMetadataStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func TableMetadataStatement(options Options) (string, []any, error) {
	p, e := tableMetadataPlan(options)
	return p.sql, p.args, e
}

func tableMetadataPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.OWNER AS OWNER")
	schema = append(schema, rowset.Column{Name: "OWNER", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLE_NAME AS TABLE_NAME")
	schema = append(schema, rowset.Column{Name: "TABLE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.TABLESPACE_NAME AS TABLESPACE_NAME")
	schema = append(schema, rowset.Column{Name: "TABLESPACE_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.STATUS AS STATUS")
	schema = append(schema, rowset.Column{Name: "STATUS", Kind: rowset.Text})
	expressions = append(expressions, "t.TEMPORARY AS TEMPORARY")
	schema = append(schema, rowset.Column{Name: "TEMPORARY", Kind: rowset.Text})
	expressions = append(expressions, "t.NESTED AS NESTED")
	schema = append(schema, rowset.Column{Name: "NESTED", Kind: rowset.Text})
	expressions = append(expressions, "t.SECONDARY AS SECONDARY")
	schema = append(schema, rowset.Column{Name: "SECONDARY", Kind: rowset.Text})
	expressions = append(expressions, "t.IOT_TYPE AS IOT_TYPE")
	schema = append(schema, rowset.Column{Name: "IOT_TYPE", Kind: rowset.Text})
	expressions = append(expressions, "t.IOT_NAME AS IOT_NAME")
	schema = append(schema, rowset.Column{Name: "IOT_NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.PARTITIONED AS PARTITIONED")
	schema = append(schema, rowset.Column{Name: "PARTITIONED", Kind: rowset.Text})
	expressions = append(expressions, "t.DURATION AS DURATION")
	schema = append(schema, rowset.Column{Name: "DURATION", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS NUM_ROWS")
	schema = append(schema, rowset.Column{Name: "NUM_ROWS", Kind: rowset.Text})
	integers = append(integers, "NUM_ROWS")
	expressions = append(expressions, "TO_CHAR(t.BLOCKS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS BLOCKS")
	schema = append(schema, rowset.Column{Name: "BLOCKS", Kind: rowset.Text})
	integers = append(integers, "BLOCKS")
	expressions = append(expressions, "TO_CHAR(t.EMPTY_BLOCKS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS EMPTY_BLOCKS")
	schema = append(schema, rowset.Column{Name: "EMPTY_BLOCKS", Kind: rowset.Text})
	integers = append(integers, "EMPTY_BLOCKS")
	expressions = append(expressions, "TO_CHAR(t.AVG_ROW_LEN, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS AVG_ROW_LEN")
	schema = append(schema, rowset.Column{Name: "AVG_ROW_LEN", Kind: rowset.Text})
	integers = append(integers, "AVG_ROW_LEN")
	expressions = append(expressions, "TO_CHAR(t.SAMPLE_SIZE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SAMPLE_SIZE")
	schema = append(schema, rowset.Column{Name: "SAMPLE_SIZE", Kind: rowset.Text})
	integers = append(integers, "SAMPLE_SIZE")
	expressions = append(expressions, datetime.GregorianDate("t.LAST_ANALYZED")+" AS LAST_ANALYZED")
	schema = append(schema, rowset.Column{Name: "LAST_ANALYZED", Kind: rowset.Text})
	expressions = append(expressions, "t.GLOBAL_STATS AS GLOBAL_STATS")
	schema = append(schema, rowset.Column{Name: "GLOBAL_STATS", Kind: rowset.Text})
	expressions = append(expressions, "t.USER_STATS AS USER_STATS")
	schema = append(schema, rowset.Column{Name: "USER_STATS", Kind: rowset.Text})
	expressions = append(expressions, "t.DEGREE AS DEGREE")
	schema = append(schema, rowset.Column{Name: "DEGREE", Kind: rowset.Text})
	expressions = append(expressions, "t.INSTANCES AS INSTANCES")
	schema = append(schema, rowset.Column{Name: "INSTANCES", Kind: rowset.Text})
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_TABLES t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.OWNER", options.Owners)
	statement, args = scopeFilter(statement, args, "t.TABLE_NAME", options.Names)
	statement += " ORDER BY t.OWNER,t.TABLE_NAME"
	return selection{nameColumn: "TABLE_NAME", ownerColumn: "OWNER", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_TABLES", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeTableMetadata validates every row before publishing any records. It cannot prove
// transport completion; use ReadTableMetadata with an admitted complete Reader.
func DecodeTableMetadata(result session.Result, options Options) (TableMetadataResult, error) {
	plan, err := tableMetadataPlan(options)
	if err != nil {
		return TableMetadataResult{}, err
	}
	return decodeTableMetadata(result, plan)
}

func decodeTableMetadata(result session.Result, plan selection) (TableMetadataResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return TableMetadataResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]Table, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Table{Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]], TablespaceName: row[index["TABLESPACE_NAME"]], Status: row[index["STATUS"]], Temporary: row[index["TEMPORARY"]], Nested: row[index["NESTED"]], Secondary: row[index["SECONDARY"]], IOTType: row[index["IOT_TYPE"]], IOTName: row[index["IOT_NAME"]], Partitioned: row[index["PARTITIONED"]], Duration: row[index["DURATION"]], NumRows: row[index["NUM_ROWS"]], Blocks: row[index["BLOCKS"]], EmptyBlocks: row[index["EMPTY_BLOCKS"]], AverageRowLength: row[index["AVG_ROW_LEN"]], SampleSize: row[index["SAMPLE_SIZE"]], LastAnalyzed: row[index["LAST_ANALYZED"]], GlobalStats: row[index["GLOBAL_STATS"]], UserStats: row[index["USER_STATS"]], Degree: row[index["DEGREE"]], Instances: row[index["INSTANCES"]]}
	}
	return TableMetadataResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadTableMetadata executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadTableMetadata(ctx context.Context, reader Reader, request Request) (TableMetadataResult, error) {
	plan, err := tableMetadataPlan(request.Options)
	if err != nil {
		return TableMetadataResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return TableMetadataResult{}, err
	}
	facts, de := decodeTableMetadata(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return TableMetadataResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// TableMetadataRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func TableMetadataRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := tableMetadataPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// TableMetadataJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func TableMetadataJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := tableMetadataPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
