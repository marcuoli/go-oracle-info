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

// DatafileSizeStatement preserves the database API for storage.DatafileSizeStatement.
func DatafileSizeStatement(options DatafileSizeOptions) (string, []any, error) {
	statement, args, err := storage.DatafileSizeStatement(storage.DatafileSizeOptions(options))
	return statement, args, legacyStorageDatasetError(err)
}

// DecodeDatafileSize preserves the database API for storage.DecodeDatafileSize.
func DecodeDatafileSize(result session.Result, options DatafileSizeOptions) (DatafileSizeResult, error) {
	return legacyDatafileSizeResult(storage.DecodeDatafileSize(result, storage.DatafileSizeOptions(options)))
}

// ReadDatafileSize preserves the database API for storage.ReadDatafileSize.
func ReadDatafileSize(ctx context.Context, reader Reader, request DatafileSizeRequest) (DatafileSizeResult, error) {
	return legacyDatafileSizeResult(storage.ReadDatafileSize(ctx, reader, storage.DatafileSizeRequest{
		Options: storage.DatafileSizeOptions(request.Options), Reference: request.Reference,
	}))
}

// DatafileSizeRows returns the native cursor from storage.DatafileSizeRows.
func DatafileSizeRows(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions) (*sql.Rows, error) {
	rows, err := storage.DatafileSizeRows(ctx, q, storage.DatafileSizeOptions(options))
	return rows, legacyStorageDatasetError(err)
}

// DatafileSizeJSON preserves the database API for storage.DatafileSizeJSON.
func DatafileSizeJSON(ctx context.Context, q rowset.Queryer, options DatafileSizeOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := storage.DatafileSizeJSON(ctx, q, storage.DatafileSizeOptions(options), limits)
	return payload, legacyStorageDatasetError(err)
}

func legacyDatafileSizeResult(facts storage.DatafileSizeResult, err error) (DatafileSizeResult, error) {
	if err != nil {
		return DatafileSizeResult{}, legacyStorageDatasetError(err)
	}
	rows := make([]DatafileSizeRow, len(facts.Rows))
	for i, row := range facts.Rows {
		rows[i] = DatafileSizeRow(row)
	}
	p := facts.Provenance
	return DatafileSizeResult{Provenance: DatasetProvenance{
		Source: p.Source, OracleMajor: p.OracleMajor, Scope: p.Scope, Complete: p.Complete,
	}, Rows: rows}, nil
}

// DatafileSizeSQL sums visible datafile bytes in rounded GiB; an empty source remains NULL.
const DatafileSizeSQL = storage.DatafileSizeSQL
