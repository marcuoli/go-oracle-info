package grants

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// RoleMembership records a direct role grant to a named principal or PUBLIC.
// Role-to-role edges retain the grantee role as recorded by Oracle.
// A nil flag pointer means the release projection omitted that column.
// A nonnil flag with Null set retains the source's SQL NULL.
type RoleMembership struct {
	Grantee        session.Value
	GranteeKind    session.Value
	GrantedRole    session.Value
	AdminOption    session.Value
	DefaultRole    session.Value
	DelegateOption *session.Value
	Common         *session.Value
	Inherited      *session.Value
	// GrantedRoleOracleMaintained is the granted role's dictionary flag.
	// The selector projects it when exact PUBLIC scope is requested on a
	// qualified release. It does not identify the grant's author.
	GrantedRoleOracleMaintained *session.Value `json:",omitempty"`
}

// RoleMembershipsResult holds one selected dictionary observation in source order.
type RoleMembershipsResult struct {
	Provenance Provenance
	Records    []RoleMembership
}

// RoleMembershipsStatement selects direct dictionary facts with exact bound filters.
func RoleMembershipsStatement(options ClassifiedOptions) (string, []any, error) {
	plan, err := roleMembershipsPlan(options)
	return plan.sql, plan.args, err
}

const grantedRoleMaintenance = `(SELECT r.ORACLE_MAINTAINED FROM DBA_ROLES r
 WHERE NLSSORT(r.ROLE, 'NLS_SORT=BINARY') = NLSSORT(t.GRANTED_ROLE, 'NLS_SORT=BINARY'))`

func roleMembershipsPlan(options ClassifiedOptions) (selection, error) {
	release, err := classifiedRelease(options)
	if err != nil {
		return selection{}, err
	}
	if len(options.Owners) != 0 {
		return selection{}, ErrInvalidRequest
	}
	fields := []grantColumn{{"t.GRANTEE", "GRANTEE"}, {"p.GRANTEE_KIND", "GRANTEE_KIND"}}
	fields = append(fields, rawGrantColumns("GRANTED_ROLE", "ADMIN_OPTION", "DEFAULT_ROLE")...)
	if release.delegate {
		fields = append(fields, grantColumn{"t.DELEGATE_OPTION", "DELEGATE_OPTION"})
	}
	if release.roleMaintained && slices.Contains(options.Names, "PUBLIC") {
		fields = append(fields, grantColumn{grantedRoleMaintenance, "GRANTED_ROLE_ORACLE_MAINTAINED"})
	}
	return selectClassifiedGrants(options, release, "DBA_ROLE_PRIVS+"+granteeSources, "DBA_ROLE_PRIVS", "",
		"t.GRANTEE, t.GRANTED_ROLE", release.appendContainerColumns(fields))
}

// DecodeRoleMemberships validates every row and retains NULL values and duplicates.
// Decode alone cannot establish transport completeness.
func DecodeRoleMemberships(result session.Result, options ClassifiedOptions) (RoleMembershipsResult, error) {
	plan, err := roleMembershipsPlan(options)
	if err != nil {
		return RoleMembershipsResult{}, err
	}
	return decodeRoleMemberships(result, plan)
}

func decodeRoleMemberships(result session.Result, plan selection) (RoleMembershipsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return RoleMembershipsResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]RoleMembership, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = RoleMembership{
			Grantee:                     row[index["GRANTEE"]],
			GranteeKind:                 row[index["GRANTEE_KIND"]],
			GrantedRole:                 row[index["GRANTED_ROLE"]],
			AdminOption:                 row[index["ADMIN_OPTION"]],
			DefaultRole:                 row[index["DEFAULT_ROLE"]],
			DelegateOption:              optionalGrantValue(row, index, "DELEGATE_OPTION"),
			Common:                      optionalGrantValue(row, index, "COMMON"),
			Inherited:                   optionalGrantValue(row, index, "INHERITED"),
			GrantedRoleOracleMaintained: optionalGrantValue(row, index, "GRANTED_ROLE_ORACLE_MAINTAINED"),
		}
	}
	return RoleMembershipsResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadRoleMemberships reads once through the caller's admitted reader. It returns no
// successful prefix on query, capacity, cancellation or validation failure.
func ReadRoleMemberships(ctx context.Context, reader Reader, request Request) (RoleMembershipsResult, error) {
	return ReadRoleMembershipsForRelease(ctx, reader, request, "")
}

// ReadRoleMembershipsForRelease additionally accepts the observed numeric Oracle release
// so the Oracle 12 selector can include only the available flags.
func ReadRoleMembershipsForRelease(ctx context.Context, reader Reader, request Request, oracleRelease string) (RoleMembershipsResult, error) {
	return readClassifiedGrants(ctx, reader, ClassifiedRequest{
		Options:   ClassifiedOptions{Options: request.Options, OracleRelease: oracleRelease},
		Reference: request.Reference, Limits: request.Limits,
	}, roleMembershipsPlan, decodeRoleMemberships)
}

// ReadRoleMembershipsWithOptions reads a complete result with the selected classification scope.
func ReadRoleMembershipsWithOptions(ctx context.Context, reader Reader, request ClassifiedRequest) (RoleMembershipsResult, error) {
	return readClassifiedGrants(ctx, reader, request, roleMembershipsPlan, decodeRoleMemberships)
}

// RoleMembershipsRows returns the actual caller-owned native cursor. The caller owns
// iteration, error and close checks, handle affinity and finite budgets.
func RoleMembershipsRows(ctx context.Context, q rowset.Queryer, options ClassifiedOptions) (*sql.Rows, error) {
	plan, err := roleMembershipsPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// RoleMembershipsJSON consumes the selected native rows within the supplied limits.
// It returns no bytes after a query, close, capacity or validation failure.
func RoleMembershipsJSON(ctx context.Context, q rowset.Queryer, options ClassifiedOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := roleMembershipsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
