package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// StatusRow preserves source values and SQL NULL without application policy.
type StatusRow struct {
	Role             session.Value
	ProtectionMode   session.Value
	ProtectionLevel  session.Value
	SwitchoverStatus session.Value
}

// StatusOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type StatusOptions struct{ OracleMajor int }

// StatusRequest supplies admitted options and a caller-owned SQL reference.
type StatusRequest struct {
	Options   StatusOptions
	Reference reference.Reference
}

// StatusResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type StatusResult struct {
	Provenance DatasetProvenance
	Rows       []StatusRow
}

// StatusStatement selects reviewed SQL with bound values, owning no connection.
func StatusStatement(options StatusOptions) (string, []any, error) {
	p, err := statusDatasetPlan(options)
	return p.sql, p.args, err
}
func statusDatasetPlan(options StatusOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT DATABASE_ROLE AS DATABASE_ROLE, PROTECTION_MODE AS PROTECTION_MODE, PROTECTION_LEVEL AS PROTECTION_LEVEL, SWITCHOVER_STATUS AS SWITCHOVER_STATUS FROM V$DATABASE"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "DATABASE_ROLE", Kind: rowset.Text}, {Name: "PROTECTION_MODE", Kind: rowset.Text}, {Name: "PROTECTION_LEVEL", Kind: rowset.Text}, {Name: "SWITCHOVER_STATUS", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE", OracleMajor: options.OracleMajor, Scope: "connected_database_role_protection_switchover"}}, nil
}

// DecodeStatus validates the entire shape and every row before returning facts.
func DecodeStatus(result session.Result, options StatusOptions) (StatusResult, error) {
	p, err := statusDatasetPlan(options)
	if err != nil {
		return StatusResult{}, err
	}
	return decodeDatasetStatus(result, p)
}
func decodeDatasetStatus(result session.Result, p datasetSelection) (StatusResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return StatusResult{}, err
	}
	if len(result.Rows) > 1 {
		return StatusResult{}, ErrInvalidEvidence
	}
	rows := make([]StatusRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = StatusRow{Role: r[index["DATABASE_ROLE"]], ProtectionMode: r[index["PROTECTION_MODE"]], ProtectionLevel: r[index["PROTECTION_LEVEL"]], SwitchoverStatus: r[index["SWITCHOVER_STATUS"]]}
	}
	return StatusResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadStatus executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadStatus(ctx context.Context, reader Reader, request StatusRequest) (StatusResult, error) {
	p, err := statusDatasetPlan(request.Options)
	if err != nil {
		return StatusResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return StatusResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return StatusResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return StatusResult{}, err
	}
	facts, err := decodeDatasetStatus(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return StatusResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// StatusRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func StatusRows(ctx context.Context, q rowset.Queryer, options StatusOptions) (*sql.Rows, error) {
	p, err := statusDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// StatusJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func StatusJSON(ctx context.Context, q rowset.Queryer, options StatusOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := statusDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetStatus(r, p); return err })
}
