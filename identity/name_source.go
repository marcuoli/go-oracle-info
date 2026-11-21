package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// NameSourceRow preserves source values and SQL NULL without application policy.
type NameSourceRow struct{ Value session.Value }

// NameSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type NameSourceOptions struct{ OracleMajor int }

// NameSourceRequest supplies admitted options and a caller-owned SQL reference.
type NameSourceRequest struct {
	Options   NameSourceOptions
	Reference reference.Reference
}

// NameSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type NameSourceResult struct {
	Provenance DatasetProvenance
	Rows       []NameSourceRow
}

// NameSourceStatement selects reviewed SQL with bound values, owning no connection.
func NameSourceStatement(options NameSourceOptions) (string, []any, error) {
	p, err := nameSourceDatasetPlan(options)
	return p.sql, p.args, err
}

func nameSourceDatasetPlan(options NameSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT NAME AS NAME FROM V$DATABASE"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE.NAME", OracleMajor: options.OracleMajor, Scope: "connected_database_name"}}, nil
}

// DecodeNameSource validates the entire shape and every row before returning facts.
func DecodeNameSource(result session.Result, options NameSourceOptions) (NameSourceResult, error) {
	p, err := nameSourceDatasetPlan(options)
	if err != nil {
		return NameSourceResult{}, err
	}
	return decodeDatasetNameSource(result, p)
}

func decodeDatasetNameSource(result session.Result, p datasetSelection) (NameSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return NameSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return NameSourceResult{}, ErrInvalidEvidence
	}
	rows := make([]NameSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = NameSourceRow{Value: r[index["NAME"]]}
	}
	return NameSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadNameSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadNameSource(ctx context.Context, reader Reader, request NameSourceRequest) (NameSourceResult, error) {
	p, err := nameSourceDatasetPlan(request.Options)
	if err != nil {
		return NameSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return NameSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return NameSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return NameSourceResult{}, err
	}
	facts, err := decodeDatasetNameSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return NameSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// NameSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func NameSourceRows(ctx context.Context, q rowset.Queryer, options NameSourceOptions) (*sql.Rows, error) {
	p, err := nameSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// NameSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func NameSourceJSON(ctx context.Context, q rowset.Queryer, options NameSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := nameSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetNameSource(r, p); return err })
}

// NameStatement reads only the database name. The established projection is
// deliberately unaliased so a scalar read does not require other identity facts.
func NameStatement() string {
	return `SELECT NAME FROM V$DATABASE`
}

// DecodeName validates a complete scalar NAME result and retains its exact text,
// SQL NULL or authoritative no-row state. Display normalization belongs to callers.
func DecodeName(result session.Result) (Text, error) {
	return decodeScalar(result, "NAME")
}
