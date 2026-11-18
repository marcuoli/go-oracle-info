package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// FullVersionSourceRow preserves source values and SQL NULL without application policy.
type FullVersionSourceRow struct{ Value session.Value }

// FullVersionSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type FullVersionSourceOptions struct{ OracleMajor int }

// FullVersionSourceRequest supplies admitted options and a caller-owned SQL reference.
type FullVersionSourceRequest struct {
	Options   FullVersionSourceOptions
	Reference reference.Reference
}

// FullVersionSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type FullVersionSourceResult struct {
	Provenance DatasetProvenance
	Rows       []FullVersionSourceRow
}

// FullVersionSourceStatement selects reviewed SQL with bound values, owning no connection.
func FullVersionSourceStatement(options FullVersionSourceOptions) (string, []any, error) {
	p, err := fullVersionSourceDatasetPlan(options)
	return p.sql, p.args, err
}
func fullVersionSourceDatasetPlan(options FullVersionSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT VERSION_FULL AS VERSION_FULL FROM V$INSTANCE"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VERSION_FULL", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$INSTANCE.VERSION_FULL", OracleMajor: options.OracleMajor, Scope: "connected_full_numeric_version"}}, nil
}

// DecodeFullVersionSource validates the entire shape and every row before returning facts.
func DecodeFullVersionSource(result session.Result, options FullVersionSourceOptions) (FullVersionSourceResult, error) {
	p, err := fullVersionSourceDatasetPlan(options)
	if err != nil {
		return FullVersionSourceResult{}, err
	}
	return decodeDatasetFullVersionSource(result, p)
}
func decodeDatasetFullVersionSource(result session.Result, p datasetSelection) (FullVersionSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return FullVersionSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return FullVersionSourceResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["VERSION_FULL"]]
		if !v.Null {
			major, err := numericVersionMajor(v.Text)
			if err != nil || major != p.provenance.OracleMajor {
				return FullVersionSourceResult{}, ErrInvalidEvidence
			}
		}
	}
	rows := make([]FullVersionSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = FullVersionSourceRow{Value: r[index["VERSION_FULL"]]}
	}
	return FullVersionSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFullVersionSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadFullVersionSource(ctx context.Context, reader Reader, request FullVersionSourceRequest) (FullVersionSourceResult, error) {
	p, err := fullVersionSourceDatasetPlan(request.Options)
	if err != nil {
		return FullVersionSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return FullVersionSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return FullVersionSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return FullVersionSourceResult{}, err
	}
	facts, err := decodeDatasetFullVersionSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return FullVersionSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FullVersionSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func FullVersionSourceRows(ctx context.Context, q rowset.Queryer, options FullVersionSourceOptions) (*sql.Rows, error) {
	p, err := fullVersionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// FullVersionSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func FullVersionSourceJSON(ctx context.Context, q rowset.Queryer, options FullVersionSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fullVersionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetFullVersionSource(r, p); return err })
}
