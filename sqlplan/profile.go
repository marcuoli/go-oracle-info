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

// Profile retains read-only SQL profile metadata. Complete records support exact category/status counts; SQL text and tuning operations are not collected.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type Profile struct {
	Name          session.Value
	Category      session.Value
	Signature     session.Value
	Created       session.Value
	LastModified  session.Value
	Description   session.Value
	Type          session.Value
	Status        session.Value
	ForceMatching session.Value
}

// ProfileResult is one complete selected dictionary observation, in source order.
type ProfileResult struct {
	Provenance Provenance
	Records    []Profile
}

// ProfileStatement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func ProfileStatement(options Options) (string, []any, error) {
	p, e := profilePlan(options)
	return p.sql, p.args, e
}
func profilePlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	if len(options.Owners) > 0 {
		return selection{}, ErrInvalidRequest
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.NAME AS NAME")
	schema = append(schema, rowset.Column{Name: "NAME", Kind: rowset.Text})
	expressions = append(expressions, "t.CATEGORY AS CATEGORY")
	schema = append(schema, rowset.Column{Name: "CATEGORY", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.SIGNATURE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SIGNATURE")
	schema = append(schema, rowset.Column{Name: "SIGNATURE", Kind: rowset.Text})
	integers = append(integers, "SIGNATURE")
	expressions = append(expressions, "TO_CHAR(t.CREATED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS CREATED")
	schema = append(schema, rowset.Column{Name: "CREATED", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.LAST_MODIFIED, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American') AS LAST_MODIFIED")
	schema = append(schema, rowset.Column{Name: "LAST_MODIFIED", Kind: rowset.Text})
	expressions = append(expressions, "t.DESCRIPTION AS DESCRIPTION")
	schema = append(schema, rowset.Column{Name: "DESCRIPTION", Kind: rowset.Text})
	expressions = append(expressions, "t.TYPE AS TYPE")
	schema = append(schema, rowset.Column{Name: "TYPE", Kind: rowset.Text})
	expressions = append(expressions, "t.STATUS AS STATUS")
	schema = append(schema, rowset.Column{Name: "STATUS", Kind: rowset.Text})
	expressions = append(expressions, "t.FORCE_MATCHING AS FORCE_MATCHING")
	schema = append(schema, rowset.Column{Name: "FORCE_MATCHING", Kind: rowset.Text})
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_SQL_PROFILES t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.NAME", options.Names)
	statement += " ORDER BY t.CATEGORY,t.NAME"
	return selection{nameColumn: "NAME", ownerColumn: "", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_SQL_PROFILES", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeProfile validates every row before publishing any records. It cannot prove
// transport completion; use ReadProfile with an admitted complete Reader.
func DecodeProfile(result session.Result, options Options) (ProfileResult, error) {
	plan, err := profilePlan(options)
	if err != nil {
		return ProfileResult{}, err
	}
	return decodeProfile(result, plan)
}
func decodeProfile(result session.Result, plan selection) (ProfileResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ProfileResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]Profile, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = Profile{Name: row[index["NAME"]], Category: row[index["CATEGORY"]], Signature: row[index["SIGNATURE"]], Created: row[index["CREATED"]], LastModified: row[index["LAST_MODIFIED"]], Description: row[index["DESCRIPTION"]], Type: row[index["TYPE"]], Status: row[index["STATUS"]], ForceMatching: row[index["FORCE_MATCHING"]]}
	}
	return ProfileResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadProfile executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func ReadProfile(ctx context.Context, reader Reader, request Request) (ProfileResult, error) {
	plan, err := profilePlan(request.Options)
	if err != nil {
		return ProfileResult{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return ProfileResult{}, err
	}
	facts, de := decodeProfile(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return ProfileResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// ProfileRows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func ProfileRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := profilePlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// ProfileJSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func ProfileJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := profilePlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
