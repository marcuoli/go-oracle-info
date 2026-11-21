// Package identity selects and decodes exact Oracle identity facts.
package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SnapshotSQL reads connected database/instance identity using Oracle 11g+ columns.
// It does not admit an expected destination or prove a PDB's open mode.
const SnapshotSQL = `SELECT d.DB_UNIQUE_NAME, d.NAME AS DATABASE_NAME, i.VERSION, i.HOST_NAME, d.PLATFORM_NAME, d.OPEN_MODE, d.DATABASE_ROLE, d.LOG_MODE, i.STATUS AS INSTANCE_STATUS FROM V$DATABASE d CROSS JOIN V$INSTANCE i`

// SnapshotRow preserves source values and SQL NULL without application policy.
type SnapshotRow struct {
	UniqueName     session.Value
	Name           session.Value
	Version        session.Value
	Host           session.Value
	Platform       session.Value
	OpenMode       session.Value
	Role           session.Value
	LogMode        session.Value
	InstanceStatus session.Value
}

// SnapshotOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SnapshotOptions struct{ OracleMajor int }

// SnapshotRequest supplies admitted options and a caller-owned SQL reference.
type SnapshotRequest struct {
	Options   SnapshotOptions
	Reference reference.Reference
}

// SnapshotResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SnapshotResult struct {
	Provenance DatasetProvenance
	Rows       []SnapshotRow
}

// SnapshotStatement selects reviewed SQL with bound values, owning no connection.
func SnapshotStatement(options SnapshotOptions) (string, []any, error) {
	p, err := snapshotDatasetPlan(options)
	return p.sql, p.args, err
}

func snapshotDatasetPlan(options SnapshotOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT d.DB_UNIQUE_NAME AS DB_UNIQUE_NAME, d.NAME AS DATABASE_NAME, i.VERSION AS VERSION, i.HOST_NAME AS HOST_NAME, d.PLATFORM_NAME AS PLATFORM_NAME, d.OPEN_MODE AS OPEN_MODE, d.DATABASE_ROLE AS DATABASE_ROLE, d.LOG_MODE AS LOG_MODE, i.STATUS AS INSTANCE_STATUS FROM V$DATABASE d CROSS JOIN V$INSTANCE i"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "DB_UNIQUE_NAME", Kind: rowset.Text}, {Name: "DATABASE_NAME", Kind: rowset.Text}, {Name: "VERSION", Kind: rowset.Text}, {Name: "HOST_NAME", Kind: rowset.Text}, {Name: "PLATFORM_NAME", Kind: rowset.Text}, {Name: "OPEN_MODE", Kind: rowset.Text}, {Name: "DATABASE_ROLE", Kind: rowset.Text}, {Name: "LOG_MODE", Kind: rowset.Text}, {Name: "INSTANCE_STATUS", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE+V$INSTANCE", OracleMajor: options.OracleMajor, Scope: "connected_snapshot_not_cross_endpoint_or_rac_atomicity"}}, nil
}

// DecodeSnapshot validates the entire shape and every row before returning facts.
func DecodeSnapshot(result session.Result, options SnapshotOptions) (SnapshotResult, error) {
	p, err := snapshotDatasetPlan(options)
	if err != nil {
		return SnapshotResult{}, err
	}
	return decodeDatasetSnapshot(result, p)
}

func decodeDatasetSnapshot(result session.Result, p datasetSelection) (SnapshotResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SnapshotResult{}, err
	}
	if len(result.Rows) > 1 {
		return SnapshotResult{}, ErrInvalidEvidence
	}
	rows := make([]SnapshotRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = SnapshotRow{UniqueName: r[index["DB_UNIQUE_NAME"]], Name: r[index["DATABASE_NAME"]], Version: r[index["VERSION"]], Host: r[index["HOST_NAME"]], Platform: r[index["PLATFORM_NAME"]], OpenMode: r[index["OPEN_MODE"]], Role: r[index["DATABASE_ROLE"]], LogMode: r[index["LOG_MODE"]], InstanceStatus: r[index["INSTANCE_STATUS"]]}
	}
	return SnapshotResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSnapshot executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadSnapshot(ctx context.Context, reader Reader, request SnapshotRequest) (SnapshotResult, error) {
	p, err := snapshotDatasetPlan(request.Options)
	if err != nil {
		return SnapshotResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SnapshotResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SnapshotResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SnapshotResult{}, err
	}
	facts, err := decodeDatasetSnapshot(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SnapshotResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SnapshotRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func SnapshotRows(ctx context.Context, q rowset.Queryer, options SnapshotOptions) (*sql.Rows, error) {
	p, err := snapshotDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// SnapshotJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func SnapshotJSON(ctx context.Context, q rowset.Queryer, options SnapshotOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := snapshotDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetSnapshot(r, p); return err })
}
