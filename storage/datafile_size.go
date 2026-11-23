package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
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
	Provenance Provenance
	Rows       []DatafileSizeRow
}

// DatafileSizeStatement selects reviewed SQL with bound values, owning no connection.
func DatafileSizeStatement(options DatafileSizeOptions) (string, []any, error) {
	p, err := datafileSizePlan(options)
	return p.sql, p.args, err
}

func datafileSizePlan(options DatafileSizeOptions) (selection, error) {
	if err := fileSummaryMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	query := DatafileSizeSQL
	return selection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: Provenance{Source: "DBA_DATA_FILES", OracleMajor: options.OracleMajor, Scope: "visible_observed_allocation_gib_rounded_three_decimals"}}, nil
}

// DecodeDatafileSize validates the entire shape and every row before returning facts.
func DecodeDatafileSize(result session.Result, options DatafileSizeOptions) (DatafileSizeResult, error) {
	p, err := datafileSizePlan(options)
	if err != nil {
		return DatafileSizeResult{}, err
	}
	return decodeDatasetDatafileSize(result, p)
}

func decodeDatasetDatafileSize(result session.Result, p selection) (DatafileSizeResult, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return DatafileSizeResult{}, ErrInvalidEvidence
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
	p, err := datafileSizePlan(request.Options)
	if err != nil {
		return DatafileSizeResult{}, err
	}
	if !validFileSummaryReader(reader) || request.Reference.Validate() != nil {
		return DatafileSizeResult{}, ErrInvalidRequest
	}
	if err = fileSummaryReadError(ctx, p, nil); err != nil {
		return DatafileSizeResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return DatafileSizeResult{}, err
	}
	facts, err := decodeDatasetDatafileSize(result, p)
	if err = fileSummaryReadError(ctx, p, projection.MarkDecodeError(err)); err != nil {
		return DatafileSizeResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatafileSizeRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func DatafileSizeRows(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions) (*sql.Rows, error) {
	p, err := datafileSizePlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// DatafileSizeJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func DatafileSizeJSON(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := datafileSizePlan(options)
	if err != nil {
		return nil, err
	}
	return readFileSummaryJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeDatasetDatafileSize(r, p)
		return projection.MarkDecodeError(err)
	})
}

// DatafileSizeSQL sums visible datafile bytes in rounded GiB; an empty source remains NULL.
const DatafileSizeSQL = `SELECT CASE WHEN COUNT(*)>0 THEN TO_CHAR(ROUND(SUM(BYTES)/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') END AS VALUE FROM DBA_DATA_FILES`
