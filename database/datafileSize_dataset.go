package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// DatafileSizeRow preserves source values and SQL NULL without application policy.
type DatafileSizeRow struct{ GiB session.Value }

// DatafileSizeOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type DatafileSizeOptions struct{ OracleMajor int }

// DatafileSizeRequest supplies admitted options and a caller-owned SQL reference.
type DatafileSizeRequest struct {
	Options   DatafileSizeOptions
	Reference reference.Reference
}

// DatafileSizeResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type DatafileSizeResult struct {
	Provenance DatasetProvenance
	Rows       []DatafileSizeRow
}

// DatafileSizeStatement selects reviewed SQL with bound values, owning no connection.
func DatafileSizeStatement(options DatafileSizeOptions) (string, []any, error) {
	p, err := datafileSizeDatasetPlan(options)
	return p.sql, p.args, err
}
func datafileSizeDatasetPlan(options DatafileSizeOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := DatafileSizeSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_DATA_FILES", OracleMajor: options.OracleMajor, Scope: "visible_observed_allocation_gib_rounded_three_decimals"}}, nil
}

// DecodeDatafileSize validates the entire shape and every row before returning facts.
func DecodeDatafileSize(result session.Result, options DatafileSizeOptions) (DatafileSizeResult, error) {
	p, err := datafileSizeDatasetPlan(options)
	if err != nil {
		return DatafileSizeResult{}, err
	}
	return decodeDatasetDatafileSize(result, p)
}
func decodeDatasetDatafileSize(result session.Result, p datasetSelection) (DatafileSizeResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return DatafileSizeResult{}, err
	}
	if len(result.Rows) != 1 {
		return DatafileSizeResult{}, ErrInvalidEvidence
	}
	rows := make([]DatafileSizeRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = DatafileSizeRow{GiB: r[index["VALUE"]]}
	}
	return DatafileSizeResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadDatafileSize executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadDatafileSize(ctx context.Context, reader Reader, request DatafileSizeRequest) (DatafileSizeResult, error) {
	p, err := datafileSizeDatasetPlan(request.Options)
	if err != nil {
		return DatafileSizeResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return DatafileSizeResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return DatafileSizeResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return DatafileSizeResult{}, err
	}
	facts, err := decodeDatasetDatafileSize(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return DatafileSizeResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatafileSizeRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func DatafileSizeRows(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions) (*sql.Rows, error) {
	p, err := datafileSizeDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// DatafileSizeJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func DatafileSizeJSON(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := datafileSizeDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetDatafileSize(r, p); return err })
}
