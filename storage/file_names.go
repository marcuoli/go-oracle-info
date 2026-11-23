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

// StorageTypeSQL classifies visible data/temp file names; missing file names remain unknown.
const StorageTypeSQL = `SELECT CASE WHEN COUNT(*)=0 OR COUNT(FILE_NAME)<>COUNT(*) THEN NULL WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=COUNT(*) THEN 'ASM' WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=0 THEN 'FileSystem' ELSE 'Mixed (ASM+FileSystem)' END AS VALUE FROM (SELECT FILE_NAME FROM DBA_DATA_FILES UNION ALL SELECT FILE_NAME FROM DBA_TEMP_FILES)`

// FileNamesRow preserves source values and SQL NULL without application policy.
type FileNamesRow struct {
	FileName session.Value
	Kind     session.Value
}

// FileNamesOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type FileNamesOptions struct{ OracleMajor int }

// FileNamesRequest supplies admitted options and a caller-owned SQL reference.
type FileNamesRequest struct {
	Options   FileNamesOptions
	Reference reference.Reference
}

// FileNamesResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type FileNamesResult struct {
	Provenance Provenance
	Rows       []FileNamesRow
}

// FileNamesStatement selects reviewed SQL with bound values, owning no connection.
func FileNamesStatement(options FileNamesOptions) (string, []any, error) {
	p, err := fileNamesPlan(options)
	return p.sql, p.args, err
}

func fileNamesPlan(options FileNamesOptions) (selection, error) {
	if err := fileSummaryMajor(options.OracleMajor); err != nil {
		return selection{}, err
	}
	query := "SELECT FILE_NAME AS FILE_NAME, 'DATAFILE' AS FILE_KIND FROM DBA_DATA_FILES UNION ALL SELECT FILE_NAME AS FILE_NAME, 'TEMPFILE' AS FILE_KIND FROM DBA_TEMP_FILES ORDER BY FILE_KIND, FILE_NAME"
	return selection{sql: query, args: nil, schema: []rowset.Column{{Name: "FILE_NAME", Kind: rowset.Text}, {Name: "FILE_KIND", Kind: rowset.Text}}, provenance: Provenance{Source: "DBA_DATA_FILES+DBA_TEMP_FILES", OracleMajor: options.OracleMajor, Scope: "visible_file_names_not_physical_placement"}}, nil
}

// DecodeFileNames validates the entire shape and every row before returning facts.
func DecodeFileNames(result session.Result, options FileNamesOptions) (FileNamesResult, error) {
	p, err := fileNamesPlan(options)
	if err != nil {
		return FileNamesResult{}, err
	}
	return decodeDatasetFileNames(result, p)
}

func decodeDatasetFileNames(result session.Result, p selection) (FileNamesResult, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return FileNamesResult{}, ErrInvalidEvidence
	}
	rows := make([]FileNamesRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = FileNamesRow{FileName: r[index["FILE_NAME"]], Kind: r[index["FILE_KIND"]]}
	}
	return FileNamesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFileNames executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadFileNames(ctx context.Context, reader Reader, request FileNamesRequest) (FileNamesResult, error) {
	p, err := fileNamesPlan(request.Options)
	if err != nil {
		return FileNamesResult{}, err
	}
	if !validFileSummaryReader(reader) || request.Reference.Validate() != nil {
		return FileNamesResult{}, ErrInvalidRequest
	}
	if err = fileSummaryReadError(ctx, p, nil); err != nil {
		return FileNamesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return FileNamesResult{}, err
	}
	facts, decodeErr := decodeDatasetFileNames(result, p)
	if err = fileSummaryReadError(ctx, p, decodeErr); err != nil {
		return FileNamesResult{}, projection.MarkDecodeError(err, decodeErr)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FileNamesRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func FileNamesRows(ctx context.Context, q rowset.Queryer, options FileNamesOptions) (*sql.Rows, error) {
	p, err := fileNamesPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// FileNamesJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func FileNamesJSON(ctx context.Context, q rowset.Queryer, options FileNamesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fileNamesPlan(options)
	if err != nil {
		return nil, err
	}
	return readFileSummaryJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeDatasetFileNames(r, p)
		return err
	})
}
