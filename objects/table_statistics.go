package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// TableStatisticsSQL reads the same statistics for bound owner (:1) and table
// (:2), retaining empty results when the declared ordinary table is not visible.
const TableStatisticsSQL = statisticsColumns + " FROM DBA_TABLES WHERE OWNER=:1 AND TABLE_NAME=:2 AND TEMPORARY='N' AND NESTED='NO' AND SECONDARY='N' AND NVL(IOT_TYPE,'HEAP')<>'IOT_OVERFLOW'"

// TableStatisticsRow preserves source values and SQL NULL without application policy.
type TableStatisticsRow struct {
	Name          session.Value
	EstimatedRows session.Value
	LastAnalyzed  session.Value
}

// TableStatisticsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type TableStatisticsOptions struct {
	OracleMajor int
	Owner       string
	Table       string
}

// TableStatisticsRequest supplies admitted options and a caller-owned SQL reference.
type TableStatisticsRequest struct {
	Options   TableStatisticsOptions
	Reference reference.Reference
}

// TableStatisticsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type TableStatisticsResult struct {
	Provenance DatasetProvenance
	Rows       []TableStatisticsRow
}

// TableStatisticsStatement selects reviewed SQL with bound values, owning no connection.
func TableStatisticsStatement(options TableStatisticsOptions) (string, []any, error) {
	p, err := tableStatisticsDatasetPlan(options)
	return p.sql, p.args, err
}

func tableStatisticsDatasetPlan(options TableStatisticsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	if _, err := QuoteIdentifier(options.Table); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(TableStatisticsSQL, "SELECT TABLE_NAME,", "SELECT TABLE_NAME AS TABLE_NAME,", 1)
	query = strings.NewReplacer("OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')", "TABLE_NAME=:2", "NLSSORT(TABLE_NAME, 'NLS_SORT=BINARY')=NLSSORT(:2, 'NLS_SORT=BINARY')").Replace(query)
	return datasetSelection{sql: query, args: []any{options.Owner, options.Table}, schema: []rowset.Column{{Name: "TABLE_NAME", Kind: rowset.Text}, {Name: "ROW_COUNT", Kind: rowset.Text}, {Name: "LAST_ANALYZED", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_TABLES", OracleMajor: options.OracleMajor, Scope: "exact_table_recorded_optimizer_estimate_not_current_count", Owner: options.Owner, Table: options.Table}}, nil
}

// DecodeTableStatistics validates the entire shape and every row before returning facts.
func DecodeTableStatistics(result session.Result, options TableStatisticsOptions) (TableStatisticsResult, error) {
	p, err := tableStatisticsDatasetPlan(options)
	if err != nil {
		return TableStatisticsResult{}, err
	}
	return decodeDatasetTableStatistics(result, p)
}

func decodeDatasetTableStatistics(result session.Result, p datasetSelection) (TableStatisticsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return TableStatisticsResult{}, err
	}
	if len(result.Rows) > 1 {
		return TableStatisticsResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["TABLE_NAME"]]
		if v.Null || v.Text != p.provenance.Table {
			return TableStatisticsResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]TableStatisticsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = TableStatisticsRow{Name: r[index["TABLE_NAME"]], EstimatedRows: r[index["ROW_COUNT"]], LastAnalyzed: r[index["LAST_ANALYZED"]]}
	}
	return TableStatisticsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTableStatistics executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadTableStatistics(ctx context.Context, reader Reader, request TableStatisticsRequest) (TableStatisticsResult, error) {
	p, err := tableStatisticsDatasetPlan(request.Options)
	if err != nil {
		return TableStatisticsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TableStatisticsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TableStatisticsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableStatisticsResult{}, err
	}
	facts, err := decodeDatasetTableStatistics(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableStatisticsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TableStatisticsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func TableStatisticsRows(ctx context.Context, q rowset.Queryer, options TableStatisticsOptions) (*sql.Rows, error) {
	p, err := tableStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// TableStatisticsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func TableStatisticsJSON(ctx context.Context, q rowset.Queryer, options TableStatisticsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tableStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetTableStatistics(r, p); return err })
}
