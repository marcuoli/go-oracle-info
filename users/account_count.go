package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// AccountCount is the total DBA_USERS account count in the admitted dictionary scope. It is independent of selected usernames and object ownership.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type AccountCount struct {
	Count session.Value
}

// AccountCountResult is one complete selected dictionary observation, in source order.
type AccountCountResult struct {
	Provenance Provenance
	Records    []AccountCount
}

// AccountCountStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func AccountCountStatement(options Options) (string, []any, error) {
	p, e := accountCountPlan(options)
	return p.sql, p.args, e
}

func accountCountPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	if len(options.Owners) > 0 {
		return selection{}, ErrInvalidRequest
	}
	if len(options.Names) > 0 {
		return selection{}, ErrInvalidRequest
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ACCOUNT_COUNT")
	schema = append(schema, rowset.Column{Name: "ACCOUNT_COUNT", Kind: rowset.Text})
	integers = append(integers, "ACCOUNT_COUNT")
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_USERS t"
	args := []any{}
	return selection{accountCount: true, sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_USERS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeAccountCount validates every row before publishing any records. It cannot prove
// transport completion; use ReadAccountCount with an admitted complete Reader.
func DecodeAccountCount(result session.Result, options Options) (AccountCountResult, error) {
	plan, err := accountCountPlan(options)
	if err != nil {
		return AccountCountResult{}, err
	}
	return decodeAccountCount(result, plan)
}

func decodeAccountCount(result session.Result, plan selection) (AccountCountResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return AccountCountResult{}, &Error{Operation: "decode", cause: err}
	}
	if len(result.Rows) != 1 {
		return AccountCountResult{}, ErrInvalidEvidence
	}
	value := result.Rows[0][index["ACCOUNT_COUNT"]]
	if value.Null || strings.HasPrefix(value.Text, "-") {
		return AccountCountResult{}, ErrInvalidEvidence
	}
	records := make([]AccountCount, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = AccountCount{Count: row[index["ACCOUNT_COUNT"]]}
	}
	return AccountCountResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadAccountCount executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadAccountCount(ctx context.Context, reader Reader, request Request) (AccountCountResult, error) {
	plan, err := accountCountPlan(request.Options)
	if err != nil {
		return AccountCountResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return AccountCountResult{}, err
	}
	facts, de := decodeAccountCount(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return AccountCountResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// AccountCountRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func AccountCountRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := accountCountPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// AccountCountJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func AccountCountJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := accountCountPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
