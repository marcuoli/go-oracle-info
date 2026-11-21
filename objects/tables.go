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

// TablesSQL lists ordinary visible tables for the bound owner (:1), excluding temporary, nested, secondary and IOT-overflow entries.
const TablesSQL = `SELECT TABLE_NAME FROM DBA_TABLES WHERE OWNER=:1 AND TEMPORARY='N' AND NESTED='NO' AND SECONDARY='N' AND NVL(IOT_TYPE,'HEAP')<>'IOT_OVERFLOW' ORDER BY TABLE_NAME`

// TablesRow preserves source values and SQL NULL without application policy.
type TablesRow struct{ Name session.Value }

// TablesOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type TablesOptions struct {
	OracleMajor int
	Owner       string
}

// TablesRequest supplies admitted options and a caller-owned SQL reference.
type TablesRequest struct {
	Options   TablesOptions
	Reference reference.Reference
}

// TablesResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type TablesResult struct {
	Provenance DatasetProvenance
	Rows       []TablesRow
}

// TablesStatement selects reviewed SQL with bound values, owning no connection.
func TablesStatement(options TablesOptions) (string, []any, error) {
	p, err := tablesDatasetPlan(options)
	return p.sql, p.args, err
}

func tablesDatasetPlan(options TablesOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(TablesSQL, "SELECT TABLE_NAME FROM", "SELECT TABLE_NAME AS TABLE_NAME FROM", 1)
	query = strings.NewReplacer("OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')").Replace(query)
	return datasetSelection{sql: query, args: []any{options.Owner}, schema: []rowset.Column{{Name: "TABLE_NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_TABLES", OracleMajor: options.OracleMajor, Scope: "exact_owner_ordinary_table_discovery", Owner: options.Owner}}, nil
}

// DecodeTables validates the entire shape and every row before returning facts.
func DecodeTables(result session.Result, options TablesOptions) (TablesResult, error) {
	p, err := tablesDatasetPlan(options)
	if err != nil {
		return TablesResult{}, err
	}
	return decodeDatasetTables(result, p)
}

func decodeDatasetTables(result session.Result, p datasetSelection) (TablesResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return TablesResult{}, err
	}
	rows := make([]TablesRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = TablesRow{Name: r[index["TABLE_NAME"]]}
	}
	return TablesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTables executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadTables(ctx context.Context, reader Reader, request TablesRequest) (TablesResult, error) {
	p, err := tablesDatasetPlan(request.Options)
	if err != nil {
		return TablesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TablesResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TablesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TablesResult{}, err
	}
	facts, err := decodeDatasetTables(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return TablesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TablesRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func TablesRows(ctx context.Context, q rowset.Queryer, options TablesOptions) (*sql.Rows, error) {
	p, err := tablesDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// TablesJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func TablesJSON(ctx context.Context, q rowset.Queryer, options TablesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tablesDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetTables(r, p); return err })
}
