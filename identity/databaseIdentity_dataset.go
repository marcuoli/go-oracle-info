package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// DatabaseIdentityRow preserves source values and SQL NULL without application policy.
type DatabaseIdentityRow struct {
	Name     session.Value
	Role     session.Value
	Platform session.Value
	LogMode  session.Value
}

// DatabaseIdentityOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type DatabaseIdentityOptions struct{ OracleMajor int }

// DatabaseIdentityRequest supplies admitted options and a caller-owned SQL reference.
type DatabaseIdentityRequest struct {
	Options   DatabaseIdentityOptions
	Reference reference.Reference
}

// DatabaseIdentityResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type DatabaseIdentityResult struct {
	Provenance DatasetProvenance
	Rows       []DatabaseIdentityRow
}

// DatabaseIdentityStatement selects reviewed SQL with bound values, owning no connection.
func DatabaseIdentityStatement(options DatabaseIdentityOptions) (string, []any, error) {
	p, err := databaseIdentityDatasetPlan(options)
	return p.sql, p.args, err
}
func databaseIdentityDatasetPlan(options DatabaseIdentityOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query, err := DatabaseStatement(options.OracleMajor)
	if err != nil {
		return datasetSelection{}, err
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "DATABASE_NAME", Kind: rowset.Text}, {Name: "DATABASE_ROLE", Kind: rowset.Text}, {Name: "PLATFORM_NAME", Kind: rowset.Text}, {Name: "LOG_MODE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE", OracleMajor: options.OracleMajor, Scope: "connected_database_independent_raw_facts"}}, nil
}

// DecodeDatabaseIdentity validates the entire shape and every row before returning facts.
func DecodeDatabaseIdentity(result session.Result, options DatabaseIdentityOptions) (DatabaseIdentityResult, error) {
	p, err := databaseIdentityDatasetPlan(options)
	if err != nil {
		return DatabaseIdentityResult{}, err
	}
	return decodeDatasetDatabaseIdentity(result, p)
}
func decodeDatasetDatabaseIdentity(result session.Result, p datasetSelection) (DatabaseIdentityResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return DatabaseIdentityResult{}, err
	}
	if len(result.Rows) > 1 {
		return DatabaseIdentityResult{}, ErrInvalidEvidence
	}
	rows := make([]DatabaseIdentityRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = DatabaseIdentityRow{Name: r[index["DATABASE_NAME"]], Role: r[index["DATABASE_ROLE"]], Platform: r[index["PLATFORM_NAME"]], LogMode: r[index["LOG_MODE"]]}
	}
	return DatabaseIdentityResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadDatabaseIdentity executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadDatabaseIdentity(ctx context.Context, reader Reader, request DatabaseIdentityRequest) (DatabaseIdentityResult, error) {
	p, err := databaseIdentityDatasetPlan(request.Options)
	if err != nil {
		return DatabaseIdentityResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return DatabaseIdentityResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return DatabaseIdentityResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return DatabaseIdentityResult{}, err
	}
	facts, err := decodeDatasetDatabaseIdentity(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return DatabaseIdentityResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatabaseIdentityRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func DatabaseIdentityRows(ctx context.Context, q rowset.Queryer, options DatabaseIdentityOptions) (*sql.Rows, error) {
	p, err := databaseIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// DatabaseIdentityJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func DatabaseIdentityJSON(ctx context.Context, q rowset.Queryer, options DatabaseIdentityOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := databaseIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetDatabaseIdentity(r, p); return err })
}
