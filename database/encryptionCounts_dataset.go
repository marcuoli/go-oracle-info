package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// EncryptionCountsRow preserves source values and SQL NULL without application policy.
type EncryptionCountsRow struct {
	Tablespaces session.Value
	Columns     session.Value
}

// EncryptionCountsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type EncryptionCountsOptions struct{ OracleMajor int }

// EncryptionCountsRequest supplies admitted options and a caller-owned SQL reference.
type EncryptionCountsRequest struct {
	Options   EncryptionCountsOptions
	Reference reference.Reference
}

// EncryptionCountsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type EncryptionCountsResult struct {
	Provenance DatasetProvenance
	Rows       []EncryptionCountsRow
}

// EncryptionCountsStatement selects reviewed SQL with bound values, owning no connection.
func EncryptionCountsStatement(options EncryptionCountsOptions) (string, []any, error) {
	p, err := encryptionCountsDatasetPlan(options)
	return p.sql, p.args, err
}
func encryptionCountsDatasetPlan(options EncryptionCountsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := EncryptionSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "ENCRYPTED_TABLESPACES", Kind: rowset.Text}, {Name: "ENCRYPTED_COLUMNS", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_TABLESPACES+DBA_ENCRYPTED_COLUMNS", OracleMajor: options.OracleMajor, Scope: "independent_visible_encrypted_tablespace_and_column_counts"}}, nil
}

// DecodeEncryptionCounts validates the entire shape and every row before returning facts.
func DecodeEncryptionCounts(result session.Result, options EncryptionCountsOptions) (EncryptionCountsResult, error) {
	p, err := encryptionCountsDatasetPlan(options)
	if err != nil {
		return EncryptionCountsResult{}, err
	}
	return decodeDatasetEncryptionCounts(result, p)
}
func decodeDatasetEncryptionCounts(result session.Result, p datasetSelection) (EncryptionCountsResult, error) {
	index, err := datasetColumns(result, p, "ENCRYPTED_TABLESPACES", "ENCRYPTED_COLUMNS")
	if err != nil {
		return EncryptionCountsResult{}, err
	}
	if len(result.Rows) != 1 {
		return EncryptionCountsResult{}, ErrInvalidEvidence
	}
	rows := make([]EncryptionCountsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = EncryptionCountsRow{Tablespaces: r[index["ENCRYPTED_TABLESPACES"]], Columns: r[index["ENCRYPTED_COLUMNS"]]}
	}
	return EncryptionCountsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadEncryptionCounts executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadEncryptionCounts(ctx context.Context, reader Reader, request EncryptionCountsRequest) (EncryptionCountsResult, error) {
	p, err := encryptionCountsDatasetPlan(request.Options)
	if err != nil {
		return EncryptionCountsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return EncryptionCountsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return EncryptionCountsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return EncryptionCountsResult{}, err
	}
	facts, err := decodeDatasetEncryptionCounts(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return EncryptionCountsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// EncryptionCountsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func EncryptionCountsRows(ctx context.Context, q rowset.Queryer, options EncryptionCountsOptions) (*sql.Rows, error) {
	p, err := encryptionCountsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// EncryptionCountsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func EncryptionCountsJSON(ctx context.Context, q rowset.Queryer, options EncryptionCountsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := encryptionCountsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetEncryptionCounts(r, p); return err })
}
