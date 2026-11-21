package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RoleSourceRow preserves source values and SQL NULL without application policy.
type RoleSourceRow struct{ Value session.Value }

// RoleSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type RoleSourceOptions struct{ OracleMajor int }

// RoleSourceRequest supplies admitted options and a caller-owned SQL reference.
type RoleSourceRequest struct {
	Options   RoleSourceOptions
	Reference reference.Reference
}

// RoleSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type RoleSourceResult struct {
	Provenance DatasetProvenance
	Rows       []RoleSourceRow
}

// RoleSourceStatement selects reviewed SQL with bound values, owning no connection.
func RoleSourceStatement(options RoleSourceOptions) (string, []any, error) {
	p, err := roleSourceDatasetPlan(options)
	return p.sql, p.args, err
}

func roleSourceDatasetPlan(options RoleSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT DATABASE_ROLE AS DATABASE_ROLE FROM V$DATABASE"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "DATABASE_ROLE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE.DATABASE_ROLE", OracleMajor: options.OracleMajor, Scope: "connected_database_role_raw"}}, nil
}

// DecodeRoleSource validates the entire shape and every row before returning facts.
func DecodeRoleSource(result session.Result, options RoleSourceOptions) (RoleSourceResult, error) {
	p, err := roleSourceDatasetPlan(options)
	if err != nil {
		return RoleSourceResult{}, err
	}
	return decodeDatasetRoleSource(result, p)
}

func decodeDatasetRoleSource(result session.Result, p datasetSelection) (RoleSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return RoleSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return RoleSourceResult{}, ErrInvalidEvidence
	}
	rows := make([]RoleSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = RoleSourceRow{Value: r[index["DATABASE_ROLE"]]}
	}
	return RoleSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRoleSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadRoleSource(ctx context.Context, reader Reader, request RoleSourceRequest) (RoleSourceResult, error) {
	p, err := roleSourceDatasetPlan(request.Options)
	if err != nil {
		return RoleSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return RoleSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return RoleSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return RoleSourceResult{}, err
	}
	facts, err := decodeDatasetRoleSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return RoleSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RoleSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func RoleSourceRows(ctx context.Context, q rowset.Queryer, options RoleSourceOptions) (*sql.Rows, error) {
	p, err := roleSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// RoleSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func RoleSourceJSON(ctx context.Context, q rowset.Queryer, options RoleSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := roleSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetRoleSource(r, p); return err })
}

// RoleStatement preserves the established scalar projection and its whitespace.
// It does not probe the platform or other columns selected by DatabaseStatement.
func RoleStatement() string {
	return "SELECT DATABASE_ROLE \n\t\t\t\t\tFROM V$DATABASE"
}

// DecodeRole validates a complete scalar DATABASE_ROLE result without normalizing
// or inferring a role. It preserves SQL NULL and no-row states independently.
func DecodeRole(result session.Result) (Text, error) {
	return decodeScalar(result, "DATABASE_ROLE")
}
