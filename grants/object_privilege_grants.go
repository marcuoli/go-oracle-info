package grants

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ObjectPrivilegeGrant records direct object grants to named users and roles.
// On 11g, OBJECT_TYPE is SQL NULL because DBA_TAB_PRIVS has no TYPE column.
// A nil flag pointer means the release projection omitted that column.
// A nonnil flag with Null set retains the source's SQL NULL.
type ObjectPrivilegeGrant struct {
	Owner       session.Value
	Grantor     session.Value
	Grantee     session.Value
	GranteeKind session.Value
	ObjectType  session.Value
	TableName   session.Value
	Privilege   session.Value
	Grantable   session.Value
	Hierarchy   session.Value
	Common      *session.Value
	Inherited   *session.Value
}

// ObjectPrivilegeGrantsResult holds one selected dictionary observation in source order.
type ObjectPrivilegeGrantsResult struct {
	Provenance Provenance
	Records    []ObjectPrivilegeGrant
}

// ObjectPrivilegeGrantsStatement selects direct dictionary facts with exact bound filters.
func ObjectPrivilegeGrantsStatement(options ClassifiedOptions) (string, []any, error) {
	plan, err := objectPrivilegeGrantsPlan(options)
	return plan.sql, plan.args, err
}

func objectPrivilegeGrantsPlan(options ClassifiedOptions) (selection, error) {
	release, err := classifiedRelease(options)
	if err != nil {
		return selection{}, err
	}
	objectType := "t.TYPE"
	if options.OracleMajor == 11 {
		// The 11g DBA_TAB_PRIVS projection cannot identify the raw object type.
		objectType = "CAST(NULL AS VARCHAR2(24))"
	}
	fields := rawGrantColumns("OWNER", "GRANTOR", "GRANTEE")
	fields = append(fields, grantColumn{"p.GRANTEE_KIND", "GRANTEE_KIND"}, grantColumn{objectType, "OBJECT_TYPE"})
	fields = append(fields, rawGrantColumns("TABLE_NAME", "PRIVILEGE", "GRANTABLE", "HIERARCHY")...)
	return selectClassifiedGrants(options.Options, "DBA_TAB_PRIVS+"+granteeSources, "DBA_TAB_PRIVS", "OWNER",
		"t.OWNER, t.TABLE_NAME, t.GRANTEE, t.PRIVILEGE, t.GRANTOR", release.appendContainerColumns(fields)), nil
}

// DecodeObjectPrivilegeGrants validates every row and retains NULL values and duplicates.
// Decode alone cannot establish transport completeness.
func DecodeObjectPrivilegeGrants(result session.Result, options ClassifiedOptions) (ObjectPrivilegeGrantsResult, error) {
	plan, err := objectPrivilegeGrantsPlan(options)
	if err != nil {
		return ObjectPrivilegeGrantsResult{}, err
	}
	return decodeObjectPrivilegeGrants(result, plan)
}

func decodeObjectPrivilegeGrants(result session.Result, plan selection) (ObjectPrivilegeGrantsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ObjectPrivilegeGrantsResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]ObjectPrivilegeGrant, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = ObjectPrivilegeGrant{
			Owner:       row[index["OWNER"]],
			Grantor:     row[index["GRANTOR"]],
			Grantee:     row[index["GRANTEE"]],
			GranteeKind: row[index["GRANTEE_KIND"]],
			ObjectType:  row[index["OBJECT_TYPE"]],
			TableName:   row[index["TABLE_NAME"]],
			Privilege:   row[index["PRIVILEGE"]],
			Grantable:   row[index["GRANTABLE"]],
			Hierarchy:   row[index["HIERARCHY"]],
			Common:      optionalGrantValue(row, index, "COMMON"),
			Inherited:   optionalGrantValue(row, index, "INHERITED"),
		}
	}
	return ObjectPrivilegeGrantsResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadObjectPrivilegeGrants reads once through the caller's admitted reader. It returns no
// successful prefix on query, capacity, cancellation or validation failure.
func ReadObjectPrivilegeGrants(ctx context.Context, reader Reader, request Request) (ObjectPrivilegeGrantsResult, error) {
	return ReadObjectPrivilegeGrantsForRelease(ctx, reader, request, "")
}

// ReadObjectPrivilegeGrantsForRelease additionally accepts the observed numeric Oracle release
// so the Oracle 12 selector can include only the available flags.
func ReadObjectPrivilegeGrantsForRelease(ctx context.Context, reader Reader, request Request, oracleRelease string) (ObjectPrivilegeGrantsResult, error) {
	return readClassifiedGrants(ctx, reader, request, oracleRelease, objectPrivilegeGrantsPlan, decodeObjectPrivilegeGrants)
}

// ObjectPrivilegeGrantsRows returns the actual caller-owned native cursor. The caller owns
// iteration, error and close checks, handle affinity and finite budgets.
func ObjectPrivilegeGrantsRows(ctx context.Context, q rowset.Queryer, options ClassifiedOptions) (*sql.Rows, error) {
	plan, err := objectPrivilegeGrantsPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// ObjectPrivilegeGrantsJSON consumes the selected native rows within the supplied limits.
// It returns no bytes after a query, close, capacity or validation failure.
func ObjectPrivilegeGrantsJSON(ctx context.Context, q rowset.Queryer, options ClassifiedOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := objectPrivilegeGrantsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
