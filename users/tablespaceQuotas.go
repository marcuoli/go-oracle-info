package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// TablespaceQuota retains one raw quota row. Bytes is observed consumption;
// MaxBytes is the assigned limit, with -1 meaning unlimited. Neither is rounded
// or bounded by int64. NULL is unknown, not zero or unlimited. A reduced limit
// can be below current consumption. Dropped is preserved without filtering rows.
// The reviewed 11g and later projections select DROPPED, so its pointer is nonnil
// even for source NULL; nil remains distinct from a selected NULL value.
type TablespaceQuota struct {
	Username       session.Value
	TablespaceName session.Value
	Bytes          session.Value
	MaxBytes       session.Value
	Dropped        *session.Value
}

// TablespaceQuotasResult is a complete admitted selection, in source order.
// A successful empty selection does not establish user existence, privileges or
// effective storage entitlement. Direct system privileges are separate facts.
type TablespaceQuotasResult struct {
	Provenance Provenance
	Records    []TablespaceQuota
}

// TablespaceQuotasStatement selects raw assigned quotas in the connected
// dictionary. Names contains exact usernames; Owners has no meaning here.
func TablespaceQuotasStatement(options Options) (string, []any, error) {
	p, err := tablespaceQuotasPlan(options)
	return p.sql, p.args, err
}
func tablespaceQuotasPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	// Explicitly reviewed families preserve the current 11g floor without
	// treating future numeric releases or 26ai branding as source capability.
	switch options.OracleMajor {
	case 11, 12, 18, 19, 21, 23:
	default:
		return selection{}, ErrUnsupportedVersion
	}
	if len(options.Owners) != 0 {
		return selection{}, ErrInvalidRequest
	}
	statement := "SELECT t.USERNAME AS USERNAME, t.TABLESPACE_NAME AS TABLESPACE_NAME, TO_CHAR(t.BYTES, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS BYTES, TO_CHAR(t.MAX_BYTES, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MAX_BYTES, t.DROPPED AS DROPPED FROM DBA_TS_QUOTAS t"
	statement, args := scopeFilter(statement, nil, "t.USERNAME", options.Names)
	statement += " ORDER BY t.USERNAME,t.TABLESPACE_NAME"
	schema := []rowset.Column{{Name: "USERNAME", Kind: rowset.Text}, {Name: "TABLESPACE_NAME", Kind: rowset.Text}, {Name: "BYTES", Kind: rowset.Text}, {Name: "MAX_BYTES", Kind: rowset.Text}, {Name: "DROPPED", Kind: rowset.Text}}
	return selection{sql: statement, args: args, schema: schema, integers: []string{"BYTES", "MAX_BYTES"}, nameColumn: "USERNAME", provenance: Provenance{Source: "DBA_TS_QUOTAS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Names: append([]string(nil), options.Names...)}}, nil
}

// DecodeTablespaceQuotas validates all rows without claiming cursor completion.
// It preserves raw NULLs, integer text, dropped flags, ordering and duplicates.
func DecodeTablespaceQuotas(result session.Result, options Options) (TablespaceQuotasResult, error) {
	p, err := tablespaceQuotasPlan(options)
	if err != nil {
		return TablespaceQuotasResult{}, err
	}
	return decodeTablespaceQuotas(result, p)
}
func decodeTablespaceQuotas(result session.Result, p selection) (TablespaceQuotasResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return TablespaceQuotasResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]TablespaceQuota, len(result.Rows))
	for i, row := range result.Rows {
		dropped := row[index["DROPPED"]]
		records[i] = TablespaceQuota{Username: row[index["USERNAME"]], TablespaceName: row[index["TABLESPACE_NAME"]], Bytes: row[index["BYTES"]], MaxBytes: row[index["MAX_BYTES"]], Dropped: &dropped}
	}
	return TablespaceQuotasResult{Provenance: p.provenance, Records: records}, nil
}

// ReadTablespaceQuotas executes once through an admitted complete Reader.
// Any transport, decoding, cancellation or finite-budget failure returns no
// successful prefix. It does not combine quota and privilege observations.
func ReadTablespaceQuotas(ctx context.Context, reader Reader, request Request) (TablespaceQuotasResult, error) {
	p, err := tablespaceQuotasPlan(request.Options)
	if err != nil {
		return TablespaceQuotasResult{}, err
	}
	result, err := query(ctx, reader, request, p)
	if err != nil {
		return TablespaceQuotasResult{}, err
	}
	facts, de := decodeTablespaceQuotas(result, p)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return TablespaceQuotasResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// TablespaceQuotasRows returns the actual caller-owned cursor. The caller owns
// Next/Scan/Err/Close, deadlines, budgets and admitted handle/container affinity.
func TablespaceQuotasRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, err := tablespaceQuotasPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// TablespaceQuotasJSON independently consumes and finalizes its cursor using the
// same selection/schema/provenance. It returns no bytes on any failure, including
// late invalid evidence and a final envelope exceeding the encoded-byte budget.
func TablespaceQuotasJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tablespaceQuotasPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits)
}
