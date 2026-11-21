package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// StorageSQL classifies visible data/temp file names; missing file names remain unknown.
const StorageSQL = `SELECT CASE WHEN COUNT(*)=0 OR COUNT(FILE_NAME)<>COUNT(*) THEN NULL WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=COUNT(*) THEN 'ASM' WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=0 THEN 'FileSystem' ELSE 'Mixed (ASM+FileSystem)' END AS VALUE FROM (SELECT FILE_NAME FROM DBA_DATA_FILES UNION ALL SELECT FILE_NAME FROM DBA_TEMP_FILES)`

// StorageRow preserves source values and SQL NULL without application policy.
type StorageRow struct {
	FileName session.Value
	Kind     session.Value
}

// StorageOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type StorageOptions struct{ OracleMajor int }

// StorageRequest supplies admitted options and a caller-owned SQL reference.
type StorageRequest struct {
	Options   StorageOptions
	Reference reference.Reference
}

// StorageResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type StorageResult struct {
	Provenance DatasetProvenance
	Rows       []StorageRow
}

// StorageStatement selects reviewed SQL with bound values, owning no connection.
func StorageStatement(options StorageOptions) (string, []any, error) {
	p, err := storageDatasetPlan(options)
	return p.sql, p.args, err
}

func storageDatasetPlan(options StorageOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT FILE_NAME AS FILE_NAME, 'DATAFILE' AS FILE_KIND FROM DBA_DATA_FILES UNION ALL SELECT FILE_NAME AS FILE_NAME, 'TEMPFILE' AS FILE_KIND FROM DBA_TEMP_FILES ORDER BY FILE_KIND, FILE_NAME"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "FILE_NAME", Kind: rowset.Text}, {Name: "FILE_KIND", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_DATA_FILES+DBA_TEMP_FILES", OracleMajor: options.OracleMajor, Scope: "visible_file_names_not_physical_placement"}}, nil
}

// DecodeStorage validates the entire shape and every row before returning facts.
func DecodeStorage(result session.Result, options StorageOptions) (StorageResult, error) {
	p, err := storageDatasetPlan(options)
	if err != nil {
		return StorageResult{}, err
	}
	return decodeDatasetStorage(result, p)
}

func decodeDatasetStorage(result session.Result, p datasetSelection) (StorageResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return StorageResult{}, err
	}
	rows := make([]StorageRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = StorageRow{FileName: r[index["FILE_NAME"]], Kind: r[index["FILE_KIND"]]}
	}
	return StorageResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadStorage executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadStorage(ctx context.Context, reader Reader, request StorageRequest) (StorageResult, error) {
	p, err := storageDatasetPlan(request.Options)
	if err != nil {
		return StorageResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return StorageResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return StorageResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return StorageResult{}, err
	}
	facts, err := decodeDatasetStorage(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return StorageResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// StorageRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func StorageRows(ctx context.Context, q rowset.Queryer, options StorageOptions) (*sql.Rows, error) {
	p, err := storageDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// StorageJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func StorageJSON(ctx context.Context, q rowset.Queryer, options StorageOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := storageDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetStorage(r, p); return err })
}
