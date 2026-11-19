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

// DirectSystemGrant retains a DBA_SYS_PRIVS row, not a computed effective grant.
// Grantees can be users or roles. Role memberships are not expanded. COMMON and
// INHERITED retain source container-grant flags, not role-derived privileges.
// Nil means the release projection omitted a flag; nonnil Null is source NULL.
type DirectSystemGrant struct {
	Grantee     session.Value
	Privilege   session.Value
	AdminOption session.Value
	Common      *session.Value
	Inherited   *session.Value
}

// DirectSystemResult is a complete selected dictionary observation, in source
// order. The source is independent of quota rows and object ownership; a grant
// remains visible even when its grantee has no explicit tablespace quotas.
type DirectSystemResult struct {
	Provenance Provenance
	Records    []DirectSystemGrant
}

// DirectSystemStatement selects generic direct system-privilege rows. Names
// contains exact grantees; Owners is rejected. No quota join or role recursion
// conceals grants, and no particular privilege becomes SDK selection policy.
func DirectSystemStatement(options Options) (string, []any, error) {
	p, err := directSystemPlan(options)
	return p.sql, p.args, err
}
func directSystemPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	switch options.OracleMajor {
	case 11, 12, 18, 19, 21, 23:
	default:
		return selection{}, ErrUnsupportedVersion
	}
	if len(options.Owners) != 0 {
		return selection{}, ErrInvalidRequest
	}
	names := []string{"GRANTEE", "PRIVILEGE", "ADMIN_OPTION"}
	if options.OracleMajor >= 12 {
		names = append(names, "COMMON")
	}
	// INHERITED starts at 12.2.0.1. Major-only 12 evidence cannot establish
	// that boundary, so this conservative profile projects it only on known 18+.
	if options.OracleMajor >= 18 {
		names = append(names, "INHERITED")
	}
	expressions := make([]string, len(names))
	schema := make([]rowset.Column, len(names))
	for i, name := range names {
		expressions[i] = "t." + name + " AS " + name
		schema[i] = rowset.Column{Name: name, Kind: rowset.Text}
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM DBA_SYS_PRIVS t"
	statement, args := scopeFilter(statement, nil, "t.GRANTEE", options.Names)
	statement += " ORDER BY t.GRANTEE,t.PRIVILEGE,t.ADMIN_OPTION"
	return selection{sql: statement, args: args, schema: schema, nameColumn: "GRANTEE", provenance: Provenance{Source: "DBA_SYS_PRIVS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeDirectSystem preserves raw privileges, flags, NULLs, order and duplicates
// after validating every row. It does not certify transport or grant freshness.
func DecodeDirectSystem(result session.Result, options Options) (DirectSystemResult, error) {
	p, err := directSystemPlan(options)
	if err != nil {
		return DirectSystemResult{}, err
	}
	return decodeDirectSystem(result, p)
}
func decodeDirectSystem(result session.Result, p selection) (DirectSystemResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return DirectSystemResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]DirectSystemGrant, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = DirectSystemGrant{Grantee: row[index["GRANTEE"]], Privilege: row[index["PRIVILEGE"]], AdminOption: row[index["ADMIN_OPTION"]]}
		if position, selected := index["COMMON"]; selected {
			value := row[position]
			records[i].Common = &value
		}
		if position, selected := index["INHERITED"]; selected {
			value := row[position]
			records[i].Inherited = &value
		}
	}
	return DirectSystemResult{Provenance: p.provenance, Records: records}, nil
}

// ReadDirectSystem executes once through the caller's admitted complete Reader.
// Failed/partial/cancelled/budget-limited observations never become empty grants
// or a successful prefix. No effective-privilege or quota comparison is inferred.
func ReadDirectSystem(ctx context.Context, reader Reader, request Request) (DirectSystemResult, error) {
	p, err := directSystemPlan(request.Options)
	if err != nil {
		return DirectSystemResult{}, err
	}
	result, err := query(ctx, reader, request, p)
	if err != nil {
		return DirectSystemResult{}, err
	}
	facts, de := decodeDirectSystem(result, p)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return DirectSystemResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// DirectSystemRows returns the actual caller-native cursor. Its caller owns
// iteration, Err/Close, finite stream budgets, deadlines and source affinity.
func DirectSystemRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, err := directSystemPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// DirectSystemJSON independently consumes/finalizes the selected cursor and
// emits the standard complete dataset envelope. Any failure returns no bytes.
func DirectSystemJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := directSystemPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits)
}
