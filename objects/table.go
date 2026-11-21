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

// TableSQL checks the bound owner (:1) and table (:2) before an explicit count.
const TableSQL = `SELECT TABLE_NAME FROM DBA_TABLES WHERE OWNER=:1 AND TABLE_NAME=:2 AND TEMPORARY='N'`

// TableRow preserves source values and SQL NULL without application policy.
type TableRow struct{ Name session.Value }

// TableOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type TableOptions struct {
	OracleMajor int
	Owner       string
	Table       string
}

// TableRequest supplies admitted options and a caller-owned SQL reference.
type TableRequest struct {
	Options   TableOptions
	Reference reference.Reference
}

// TableResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type TableResult struct {
	Provenance DatasetProvenance
	Rows       []TableRow
}

// TableStatement selects reviewed SQL with bound values, owning no connection.
func TableStatement(options TableOptions) (string, []any, error) {
	p, err := tableDatasetPlan(options)
	return p.sql, p.args, err
}

func tableDatasetPlan(options TableOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	if _, err := QuoteIdentifier(options.Table); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(TableSQL, "SELECT TABLE_NAME FROM", "SELECT TABLE_NAME AS TABLE_NAME FROM", 1)
	query = strings.NewReplacer("OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')", "TABLE_NAME=:2", "NLSSORT(TABLE_NAME, 'NLS_SORT=BINARY')=NLSSORT(:2, 'NLS_SORT=BINARY')").Replace(query)
	return datasetSelection{sql: query, args: []any{options.Owner, options.Table}, schema: []rowset.Column{{Name: "TABLE_NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_TABLES", OracleMajor: options.OracleMajor, Scope: "exact_owner_name_table_lookup_excluding_temporary", Owner: options.Owner, Table: options.Table}}, nil
}

// DecodeTable validates the entire shape and every row before returning facts.
func DecodeTable(result session.Result, options TableOptions) (TableResult, error) {
	p, err := tableDatasetPlan(options)
	if err != nil {
		return TableResult{}, err
	}
	return decodeDatasetTable(result, p)
}

func decodeDatasetTable(result session.Result, p datasetSelection) (TableResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return TableResult{}, err
	}
	if len(result.Rows) > 1 {
		return TableResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["TABLE_NAME"]]
		if v.Null || v.Text != p.provenance.Table {
			return TableResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]TableRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = TableRow{Name: r[index["TABLE_NAME"]]}
	}
	return TableResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTable executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadTable(ctx context.Context, reader Reader, request TableRequest) (TableResult, error) {
	p, err := tableDatasetPlan(request.Options)
	if err != nil {
		return TableResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TableResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TableResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableResult{}, err
	}
	facts, err := decodeDatasetTable(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TableRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func TableRows(ctx context.Context, q rowset.Queryer, options TableOptions) (*sql.Rows, error) {
	p, err := tableDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// TableJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func TableJSON(ctx context.Context, q rowset.Queryer, options TableOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tableDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetTable(r, p); return err })
}
