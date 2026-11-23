package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/storage"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

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

// StorageStatement preserves the database API for storage.FileNamesStatement.
func StorageStatement(options StorageOptions) (string, []any, error) {
	statement, args, err := storage.FileNamesStatement(storage.FileNamesOptions(options))
	return statement, args, legacyStorageDatasetError(err)
}

// DecodeStorage preserves the database API for storage.DecodeFileNames.
func DecodeStorage(result session.Result, options StorageOptions) (StorageResult, error) {
	return legacyStorageResult(storage.DecodeFileNames(result, storage.FileNamesOptions(options)))
}

// ReadStorage preserves the database API for storage.ReadFileNames.
func ReadStorage(ctx context.Context, reader Reader, request StorageRequest) (StorageResult, error) {
	return legacyStorageResult(storage.ReadFileNames(ctx, reader, storage.FileNamesRequest{
		Options: storage.FileNamesOptions(request.Options), Reference: request.Reference,
	}))
}

// StorageRows returns the native cursor from storage.FileNamesRows.
func StorageRows(ctx context.Context, q rowset.Queryer, options StorageOptions) (*sql.Rows, error) {
	rows, err := storage.FileNamesRows(ctx, q, storage.FileNamesOptions(options))
	return rows, legacyStorageDatasetError(err)
}

// StorageJSON preserves the database API for storage.FileNamesJSON.
func StorageJSON(ctx context.Context, q rowset.Queryer, options StorageOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := storage.FileNamesJSON(ctx, q, storage.FileNamesOptions(options), limits)
	return payload, legacyStorageDatasetError(err)
}

func legacyStorageResult(facts storage.FileNamesResult, err error) (StorageResult, error) {
	if err != nil {
		return StorageResult{}, legacyStorageDatasetError(err)
	}
	rows := make([]StorageRow, len(facts.Rows))
	for i, row := range facts.Rows {
		rows[i] = StorageRow(row)
	}
	p := facts.Provenance
	return StorageResult{Provenance: DatasetProvenance{
		Source: p.Source, OracleMajor: p.OracleMajor, Scope: p.Scope, Complete: p.Complete,
	}, Rows: rows}, nil
}

// StorageSQL classifies visible data/temp file names; missing file names remain unknown.
const StorageSQL = storage.StorageTypeSQL
