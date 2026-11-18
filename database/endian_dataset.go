package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// EndianRow preserves source values and SQL NULL without application policy.
type EndianRow struct{ Value session.Value }

// EndianOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type EndianOptions struct{ OracleMajor int }

// EndianRequest supplies admitted options and a caller-owned SQL reference.
type EndianRequest struct {
	Options   EndianOptions
	Reference reference.Reference
}

// EndianResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type EndianResult struct {
	Provenance DatasetProvenance
	Rows       []EndianRow
}

// EndianStatement selects reviewed SQL with bound values, owning no connection.
func EndianStatement(options EndianOptions) (string, []any, error) {
	p, err := endianDatasetPlan(options)
	return p.sql, p.args, err
}
func endianDatasetPlan(options EndianOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := EndianSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$TRANSPORTABLE_PLATFORM+V$DATABASE", OracleMajor: options.OracleMajor, Scope: "database_platform_byte_order"}}, nil
}

// DecodeEndian validates the entire shape and every row before returning facts.
func DecodeEndian(result session.Result, options EndianOptions) (EndianResult, error) {
	p, err := endianDatasetPlan(options)
	if err != nil {
		return EndianResult{}, err
	}
	return decodeDatasetEndian(result, p)
}
func decodeDatasetEndian(result session.Result, p datasetSelection) (EndianResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return EndianResult{}, err
	}
	if len(result.Rows) > 1 {
		return EndianResult{}, ErrInvalidEvidence
	}
	rows := make([]EndianRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = EndianRow{Value: r[index["VALUE"]]}
	}
	return EndianResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadEndian executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadEndian(ctx context.Context, reader Reader, request EndianRequest) (EndianResult, error) {
	p, err := endianDatasetPlan(request.Options)
	if err != nil {
		return EndianResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return EndianResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return EndianResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return EndianResult{}, err
	}
	facts, err := decodeDatasetEndian(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return EndianResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// EndianRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func EndianRows(ctx context.Context, q rowset.Queryer, options EndianOptions) (*sql.Rows, error) {
	p, err := endianDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// EndianJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func EndianJSON(ctx context.Context, q rowset.Queryer, options EndianOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := endianDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetEndian(r, p); return err })
}
