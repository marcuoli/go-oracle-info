package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ProductsRow preserves source values and SQL NULL without application policy.
type ProductsRow struct{ ComponentID session.Value }

// ProductsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ProductsOptions struct{ OracleMajor int }

// ProductsRequest supplies admitted options and a caller-owned SQL reference.
type ProductsRequest struct {
	Options   ProductsOptions
	Reference reference.Reference
}

// ProductsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ProductsResult struct {
	Provenance DatasetProvenance
	Rows       []ProductsRow
}

// ProductsStatement selects reviewed SQL with bound values, owning no connection.
func ProductsStatement(options ProductsOptions) (string, []any, error) {
	p, err := productsDatasetPlan(options)
	return p.sql, p.args, err
}

func productsDatasetPlan(options ProductsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT COMP_ID AS VALUE FROM DBA_REGISTRY ORDER BY COMP_ID"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_REGISTRY", OracleMajor: options.OracleMajor, Scope: "all_visible_registered_components_not_licensing"}}, nil
}

// DecodeProducts validates the entire shape and every row before returning facts.
func DecodeProducts(result session.Result, options ProductsOptions) (ProductsResult, error) {
	p, err := productsDatasetPlan(options)
	if err != nil {
		return ProductsResult{}, err
	}
	return decodeDatasetProducts(result, p)
}

func decodeDatasetProducts(result session.Result, p datasetSelection) (ProductsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return ProductsResult{}, err
	}
	rows := make([]ProductsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ProductsRow{ComponentID: r[index["VALUE"]]}
	}
	return ProductsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadProducts executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadProducts(ctx context.Context, reader Reader, request ProductsRequest) (ProductsResult, error) {
	p, err := productsDatasetPlan(request.Options)
	if err != nil {
		return ProductsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ProductsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ProductsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ProductsResult{}, err
	}
	facts, err := decodeDatasetProducts(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ProductsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ProductsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ProductsRows(ctx context.Context, q rowset.Queryer, options ProductsOptions) (*sql.Rows, error) {
	p, err := productsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ProductsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ProductsJSON(ctx context.Context, q rowset.Queryer, options ProductsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := productsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetProducts(r, p); return err })
}

// ProductsSQL lists selected registered components; registration does not prove licensing.
const ProductsSQL = `SELECT COMP_ID AS VALUE FROM DBA_REGISTRY WHERE COMP_ID IN ('CONTEXT','SDO','XOQ','DV','OLS','APEX') ORDER BY COMP_ID`
