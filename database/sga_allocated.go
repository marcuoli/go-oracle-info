package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SGAAllocatedRow preserves source values and SQL NULL without application policy.
type SGAAllocatedRow struct{ GiB session.Value }

// SGAAllocatedOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SGAAllocatedOptions struct{ OracleMajor int }

// SGAAllocatedRequest supplies admitted options and a caller-owned SQL reference.
type SGAAllocatedRequest struct {
	Options   SGAAllocatedOptions
	Reference reference.Reference
}

// SGAAllocatedResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SGAAllocatedResult struct {
	Provenance DatasetProvenance
	Rows       []SGAAllocatedRow
}

// SGAAllocatedStatement selects reviewed SQL with bound values, owning no connection.
func SGAAllocatedStatement(options SGAAllocatedOptions) (string, []any, error) {
	p, err := sGAAllocatedDatasetPlan(options)
	return p.sql, p.args, err
}

func sGAAllocatedDatasetPlan(options SGAAllocatedOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := SGAAllocatedSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$SGA", OracleMajor: options.OracleMajor, Scope: "observed_instance_sga_allocation_gib_rounded_three_decimals"}}, nil
}

// DecodeSGAAllocated validates the entire shape and every row before returning facts.
func DecodeSGAAllocated(result session.Result, options SGAAllocatedOptions) (SGAAllocatedResult, error) {
	p, err := sGAAllocatedDatasetPlan(options)
	if err != nil {
		return SGAAllocatedResult{}, err
	}
	return decodeDatasetSGAAllocated(result, p)
}

func decodeDatasetSGAAllocated(result session.Result, p datasetSelection) (SGAAllocatedResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SGAAllocatedResult{}, err
	}
	if len(result.Rows) != 1 {
		return SGAAllocatedResult{}, ErrInvalidEvidence
	}
	rows := make([]SGAAllocatedRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = SGAAllocatedRow{GiB: r[index["VALUE"]]}
	}
	return SGAAllocatedResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSGAAllocated executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadSGAAllocated(ctx context.Context, reader Reader, request SGAAllocatedRequest) (SGAAllocatedResult, error) {
	p, err := sGAAllocatedDatasetPlan(request.Options)
	if err != nil {
		return SGAAllocatedResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SGAAllocatedResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SGAAllocatedResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SGAAllocatedResult{}, err
	}
	facts, err := decodeDatasetSGAAllocated(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SGAAllocatedResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SGAAllocatedRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func SGAAllocatedRows(ctx context.Context, q rowset.Queryer, options SGAAllocatedOptions) (*sql.Rows, error) {
	p, err := sGAAllocatedDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// SGAAllocatedJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func SGAAllocatedJSON(ctx context.Context, q rowset.Queryer, options SGAAllocatedOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sGAAllocatedDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetSGAAllocated(r, p); return err })
}

// SGAAllocatedSQL reads observed instance SGA allocation in rounded GiB, not configured memory.
const SGAAllocatedSQL = `SELECT TO_CHAR(ROUND(SUM(VALUE)/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$SGA`
