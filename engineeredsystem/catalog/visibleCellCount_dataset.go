package catalog

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// VisibleCellCountRow preserves source values and SQL NULL without application policy.
type VisibleCellCountRow struct{ Count session.Value }

// VisibleCellCountOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type VisibleCellCountOptions struct{ OracleMajor int }

// VisibleCellCountRequest supplies admitted options and a caller-owned SQL reference.
type VisibleCellCountRequest struct {
	Options   VisibleCellCountOptions
	Reference reference.Reference
}

// VisibleCellCountResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type VisibleCellCountResult struct {
	Provenance DatasetProvenance
	Rows       []VisibleCellCountRow
}

// VisibleCellCountStatement selects reviewed SQL with bound values, owning no connection.
func VisibleCellCountStatement(options VisibleCellCountOptions) (string, []any, error) {
	p, err := visibleCellCountDatasetPlan(options)
	return p.sql, p.args, err
}
func visibleCellCountDatasetPlan(options VisibleCellCountOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := VisibleCellCountSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Integer}}, provenance: DatasetProvenance{Source: "GV$CELL", OracleMajor: options.OracleMajor, Scope: "visible_reporter_cell_row_count_not_physical_hardware_or_rac_proof"}}, nil
}

// DecodeVisibleCellCount validates the entire shape and every row before returning facts.
func DecodeVisibleCellCount(result session.Result, options VisibleCellCountOptions) (VisibleCellCountResult, error) {
	p, err := visibleCellCountDatasetPlan(options)
	if err != nil {
		return VisibleCellCountResult{}, err
	}
	return decodeDatasetVisibleCellCount(result, p)
}
func decodeDatasetVisibleCellCount(result session.Result, p datasetSelection) (VisibleCellCountResult, error) {
	index, err := datasetColumns(result, p, "VALUE")
	if err != nil {
		return VisibleCellCountResult{}, err
	}
	if len(result.Rows) != 1 {
		return VisibleCellCountResult{}, ErrInvalidEvidence
	}
	if len(result.Rows) == 1 && result.Rows[0][index["VALUE"]].Null {
		return VisibleCellCountResult{}, ErrInvalidEvidence
	}
	rows := make([]VisibleCellCountRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = VisibleCellCountRow{Count: r[index["VALUE"]]}
	}
	return VisibleCellCountResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadVisibleCellCount executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadVisibleCellCount(ctx context.Context, reader Reader, request VisibleCellCountRequest) (VisibleCellCountResult, error) {
	p, err := visibleCellCountDatasetPlan(request.Options)
	if err != nil {
		return VisibleCellCountResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return VisibleCellCountResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return VisibleCellCountResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return VisibleCellCountResult{}, err
	}
	facts, err := decodeDatasetVisibleCellCount(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return VisibleCellCountResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// VisibleCellCountRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func VisibleCellCountRows(ctx context.Context, q rowset.Queryer, options VisibleCellCountOptions) (*sql.Rows, error) {
	p, err := visibleCellCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// VisibleCellCountJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func VisibleCellCountJSON(ctx context.Context, q rowset.Queryer, options VisibleCellCountOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := visibleCellCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetVisibleCellCount(r, p); return err })
}
