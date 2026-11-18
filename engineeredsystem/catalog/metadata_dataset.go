package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MetadataColumnRow preserves source values and SQL NULL without application policy.
type MetadataColumnRow struct {
	Source   session.Value
	Column   session.Value
	DataType session.Value
}

// MetadataOptions admits the independently reviewed source selection.
// Capability-selected projections admit numeric majors 12, 18, 19, 21 and 23.
// The live schema must independently admit every selected source/column. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type MetadataOptions struct{ OracleMajor int }

// MetadataRequest supplies admitted options and a caller-owned SQL reference.
type MetadataRequest struct {
	Options   MetadataOptions
	Reference reference.Reference
}

// MetadataResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type MetadataResult struct {
	Provenance DatasetProvenance
	Rows       []MetadataColumnRow
}

// MetadataStatement selects reviewed SQL with bound values, owning no connection.
func MetadataStatement(options MetadataOptions) (string, []any, error) {
	p, err := metadataDatasetPlan(options)
	return p.sql, p.args, err
}
func metadataDatasetPlan(options MetadataOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	command, err := Metadata()
	if err != nil {
		return datasetSelection{}, err
	}
	query := command.Text
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "SOURCE_NAME", Kind: rowset.Text}, {Name: "COLUMN_NAME", Kind: rowset.Text}, {Name: "DATA_TYPE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "SYS.ALL_TAB_COLUMNS", OracleMajor: options.OracleMajor, Scope: "admitted_engineered_source_column_metadata"}}, nil
}

// DecodeMetadata validates the entire shape and every row before returning facts.
func DecodeMetadata(result session.Result, options MetadataOptions) (MetadataResult, error) {
	p, err := metadataDatasetPlan(options)
	if err != nil {
		return MetadataResult{}, err
	}
	return decodeDatasetMetadata(result, p)
}
func decodeDatasetMetadata(result session.Result, p datasetSelection) (MetadataResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return MetadataResult{}, err
	}
	rows := make([]MetadataColumnRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = MetadataColumnRow{Source: r[index["SOURCE_NAME"]], Column: r[index["COLUMN_NAME"]], DataType: r[index["DATA_TYPE"]]}
	}
	facts := MetadataResult{Provenance: p.provenance, Rows: rows}
	if _, err := facts.CapabilitySchema(); err != nil {
		return MetadataResult{}, errors.Join(ErrInvalidEvidence, err)
	}
	return facts, nil
}

// ReadMetadata executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadMetadata(ctx context.Context, reader Reader, request MetadataRequest) (MetadataResult, error) {
	p, err := metadataDatasetPlan(request.Options)
	if err != nil {
		return MetadataResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return MetadataResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return MetadataResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return MetadataResult{}, err
	}
	facts, err := decodeDatasetMetadata(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return MetadataResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// MetadataRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func MetadataRows(ctx context.Context, q rowset.Queryer, options MetadataOptions) (*sql.Rows, error) {
	p, err := metadataDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// MetadataJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func MetadataJSON(ctx context.Context, q rowset.Queryer, options MetadataOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := metadataDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetMetadata(r, p); return err })
}
