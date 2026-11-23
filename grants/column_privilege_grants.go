package grants

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ColumnPrivilegeGrant records a direct column grant to a named principal or PUBLIC.
// ObjectType is SQL NULL if DBA_OBJECTS provides no unique candidate type.
// A nil flag pointer means the release projection omitted that column.
// A nonnil flag with Null set retains the source's SQL NULL.
type ColumnPrivilegeGrant struct {
	Owner       session.Value
	Grantor     session.Value
	Grantee     session.Value
	GranteeKind session.Value
	ObjectType  session.Value
	TableName   session.Value
	ColumnName  session.Value
	Privilege   session.Value
	Grantable   session.Value
	Common      *session.Value
	Inherited   *session.Value
}

// ColumnPrivilegeGrantsResult holds one selected dictionary observation in source order.
type ColumnPrivilegeGrantsResult struct {
	Provenance Provenance
	Records    []ColumnPrivilegeGrant
}

// ColumnPrivilegeGrantsStatement selects direct dictionary facts with exact bound filters.
func ColumnPrivilegeGrantsStatement(options ClassifiedOptions) (string, []any, error) {
	plan, err := columnPrivilegeGrantsPlan(options)
	return plan.sql, plan.args, err
}

// Same-statement lookup retains grant multiplicity and does not guess between
// objects of different types sharing an owner/name (e.g. a materialized view and
// its backing table). Partitions and unrelated object namespaces are excluded.
const columnGrantObjectType = `(
	SELECT CASE WHEN COUNT(DISTINCT o.OBJECT_TYPE) = 1 THEN MIN(o.OBJECT_TYPE) END
	FROM DBA_OBJECTS o
	WHERE NLSSORT(o.OWNER, 'NLS_SORT=BINARY') = NLSSORT(t.OWNER, 'NLS_SORT=BINARY')
	  AND NLSSORT(o.OBJECT_NAME, 'NLS_SORT=BINARY') = NLSSORT(t.TABLE_NAME, 'NLS_SORT=BINARY')
	  AND o.SUBOBJECT_NAME IS NULL
	  AND o.OBJECT_TYPE IN ('TABLE', 'VIEW', 'MATERIALIZED VIEW')
)`

func columnPrivilegeGrantsPlan(options ClassifiedOptions) (selection, error) {
	release, err := classifiedRelease(options)
	if err != nil {
		return selection{}, err
	}
	fields := rawGrantColumns("OWNER", "GRANTOR", "GRANTEE")
	fields = append(fields, grantColumn{"p.GRANTEE_KIND", "GRANTEE_KIND"}, grantColumn{columnGrantObjectType, "OBJECT_TYPE"})
	fields = append(fields, rawGrantColumns("TABLE_NAME", "COLUMN_NAME", "PRIVILEGE", "GRANTABLE")...)
	return selectClassifiedGrants(options, release, "DBA_COL_PRIVS+"+granteeSources+"+DBA_OBJECTS", "DBA_COL_PRIVS", "OWNER",
		"t.OWNER, t.TABLE_NAME, t.COLUMN_NAME, t.GRANTEE, t.PRIVILEGE, t.GRANTOR", release.appendContainerColumns(fields))
}

// DecodeColumnPrivilegeGrants validates every row and retains NULL values and duplicates.
// Decode alone cannot establish transport completeness.
func DecodeColumnPrivilegeGrants(result session.Result, options ClassifiedOptions) (ColumnPrivilegeGrantsResult, error) {
	plan, err := columnPrivilegeGrantsPlan(options)
	if err != nil {
		return ColumnPrivilegeGrantsResult{}, err
	}
	return decodeColumnPrivilegeGrants(result, plan)
}

func decodeColumnPrivilegeGrants(result session.Result, plan selection) (ColumnPrivilegeGrantsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ColumnPrivilegeGrantsResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]ColumnPrivilegeGrant, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = ColumnPrivilegeGrant{
			Owner:       row[index["OWNER"]],
			Grantor:     row[index["GRANTOR"]],
			Grantee:     row[index["GRANTEE"]],
			GranteeKind: row[index["GRANTEE_KIND"]],
			ObjectType:  row[index["OBJECT_TYPE"]],
			TableName:   row[index["TABLE_NAME"]],
			ColumnName:  row[index["COLUMN_NAME"]],
			Privilege:   row[index["PRIVILEGE"]],
			Grantable:   row[index["GRANTABLE"]],
			Common:      optionalGrantValue(row, index, "COMMON"),
			Inherited:   optionalGrantValue(row, index, "INHERITED"),
		}
	}
	return ColumnPrivilegeGrantsResult{Provenance: plan.provenance, Records: records}, nil
}

// ReadColumnPrivilegeGrants reads once through the caller's admitted reader. It returns no
// successful prefix on query, capacity, cancellation or validation failure.
func ReadColumnPrivilegeGrants(ctx context.Context, reader Reader, request Request) (ColumnPrivilegeGrantsResult, error) {
	return ReadColumnPrivilegeGrantsForRelease(ctx, reader, request, "")
}

// ReadColumnPrivilegeGrantsForRelease additionally accepts the observed numeric Oracle release
// so the Oracle 12 selector can include only the available flags.
func ReadColumnPrivilegeGrantsForRelease(ctx context.Context, reader Reader, request Request, oracleRelease string) (ColumnPrivilegeGrantsResult, error) {
	return readClassifiedGrants(ctx, reader, ClassifiedRequest{
		Options:   ClassifiedOptions{Options: request.Options, OracleRelease: oracleRelease},
		Reference: request.Reference, Limits: request.Limits,
	}, columnPrivilegeGrantsPlan, decodeColumnPrivilegeGrants)
}

// ReadColumnPrivilegeGrantsWithOptions reads a complete result with the selected classification scope.
func ReadColumnPrivilegeGrantsWithOptions(ctx context.Context, reader Reader, request ClassifiedRequest) (ColumnPrivilegeGrantsResult, error) {
	return readClassifiedGrants(ctx, reader, request, columnPrivilegeGrantsPlan, decodeColumnPrivilegeGrants)
}

// ColumnPrivilegeGrantsRows returns the actual caller-owned native cursor. The caller owns
// iteration, error and close checks, handle affinity and finite budgets.
func ColumnPrivilegeGrantsRows(ctx context.Context, q rowset.Queryer, options ClassifiedOptions) (*sql.Rows, error) {
	plan, err := columnPrivilegeGrantsPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// ColumnPrivilegeGrantsJSON consumes the selected native rows within the supplied limits.
// It returns no bytes after a query, close, capacity or validation failure.
func ColumnPrivilegeGrantsJSON(ctx context.Context, q rowset.Queryer, options ClassifiedOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := columnPrivilegeGrantsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
