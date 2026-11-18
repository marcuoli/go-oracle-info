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

// TablesStatisticsRow preserves source values and SQL NULL without application policy.
type TablesStatisticsRow struct {
	Name          session.Value
	EstimatedRows session.Value
	LastAnalyzed  session.Value
}

// TablesStatisticsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type TablesStatisticsOptions struct {
	OracleMajor int
	Owner       string
}

// TablesStatisticsRequest supplies admitted options and a caller-owned SQL reference.
type TablesStatisticsRequest struct {
	Options   TablesStatisticsOptions
	Reference reference.Reference
}

// TablesStatisticsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type TablesStatisticsResult struct {
	Provenance DatasetProvenance
	Rows       []TablesStatisticsRow
}

// TablesStatisticsStatement selects reviewed SQL with bound values, owning no connection.
func TablesStatisticsStatement(options TablesStatisticsOptions) (string, []any, error) {
	p, err := tablesStatisticsDatasetPlan(options)
	return p.sql, p.args, err
}
func tablesStatisticsDatasetPlan(options TablesStatisticsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(TablesStatisticsSQL, "SELECT TABLE_NAME,", "SELECT TABLE_NAME AS TABLE_NAME,", 1)
	query = strings.NewReplacer("OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')").Replace(query)
	return datasetSelection{sql: query, args: []any{options.Owner}, schema: []rowset.Column{{Name: "TABLE_NAME", Kind: rowset.Text}, {Name: "ROW_COUNT", Kind: rowset.Text}, {Name: "LAST_ANALYZED", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_TABLES", OracleMajor: options.OracleMajor, Scope: "exact_owner_recorded_optimizer_estimates_not_current_counts", Owner: options.Owner}}, nil
}

// DecodeTablesStatistics validates the entire shape and every row before returning facts.
func DecodeTablesStatistics(result session.Result, options TablesStatisticsOptions) (TablesStatisticsResult, error) {
	p, err := tablesStatisticsDatasetPlan(options)
	if err != nil {
		return TablesStatisticsResult{}, err
	}
	return decodeDatasetTablesStatistics(result, p)
}
func decodeDatasetTablesStatistics(result session.Result, p datasetSelection) (TablesStatisticsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return TablesStatisticsResult{}, err
	}
	rows := make([]TablesStatisticsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = TablesStatisticsRow{Name: r[index["TABLE_NAME"]], EstimatedRows: r[index["ROW_COUNT"]], LastAnalyzed: r[index["LAST_ANALYZED"]]}
	}
	return TablesStatisticsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTablesStatistics executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadTablesStatistics(ctx context.Context, reader Reader, request TablesStatisticsRequest) (TablesStatisticsResult, error) {
	p, err := tablesStatisticsDatasetPlan(request.Options)
	if err != nil {
		return TablesStatisticsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TablesStatisticsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TablesStatisticsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TablesStatisticsResult{}, err
	}
	facts, err := decodeDatasetTablesStatistics(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return TablesStatisticsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TablesStatisticsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func TablesStatisticsRows(ctx context.Context, q rowset.Queryer, options TablesStatisticsOptions) (*sql.Rows, error) {
	p, err := tablesStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// TablesStatisticsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func TablesStatisticsJSON(ctx context.Context, q rowset.Queryer, options TablesStatisticsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tablesStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetTablesStatistics(r, p); return err })
}
