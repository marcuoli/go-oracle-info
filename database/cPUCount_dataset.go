package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// CPUCountRow preserves source values and SQL NULL without application policy.
type CPUCountRow struct{ Value session.Value }

// CPUCountOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type CPUCountOptions struct{ OracleMajor int }

// CPUCountRequest supplies admitted options and a caller-owned SQL reference.
type CPUCountRequest struct {
	Options   CPUCountOptions
	Reference reference.Reference
}

// CPUCountResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type CPUCountResult struct {
	Provenance DatasetProvenance
	Rows       []CPUCountRow
}

// CPUCountStatement selects reviewed SQL with bound values, owning no connection.
func CPUCountStatement(options CPUCountOptions) (string, []any, error) {
	p, err := cPUCountDatasetPlan(options)
	return p.sql, p.args, err
}
func cPUCountDatasetPlan(options CPUCountOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := strings.Replace(CPUCountSQL, "SELECT VALUE FROM", "SELECT VALUE AS VALUE FROM", 1)
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PARAMETER", OracleMajor: options.OracleMajor, Scope: "current_session_configured_cpu_count_not_host_capacity"}}, nil
}

// DecodeCPUCount validates the entire shape and every row before returning facts.
func DecodeCPUCount(result session.Result, options CPUCountOptions) (CPUCountResult, error) {
	p, err := cPUCountDatasetPlan(options)
	if err != nil {
		return CPUCountResult{}, err
	}
	return decodeDatasetCPUCount(result, p)
}
func decodeDatasetCPUCount(result session.Result, p datasetSelection) (CPUCountResult, error) {
	index, err := datasetColumns(result, p, "VALUE")
	if err != nil {
		return CPUCountResult{}, err
	}
	if len(result.Rows) > 1 {
		return CPUCountResult{}, ErrInvalidEvidence
	}
	rows := make([]CPUCountRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = CPUCountRow{Value: r[index["VALUE"]]}
	}
	return CPUCountResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCPUCount executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadCPUCount(ctx context.Context, reader Reader, request CPUCountRequest) (CPUCountResult, error) {
	p, err := cPUCountDatasetPlan(request.Options)
	if err != nil {
		return CPUCountResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return CPUCountResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return CPUCountResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return CPUCountResult{}, err
	}
	facts, err := decodeDatasetCPUCount(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return CPUCountResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// CPUCountRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func CPUCountRows(ctx context.Context, q rowset.Queryer, options CPUCountOptions) (*sql.Rows, error) {
	p, err := cPUCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// CPUCountJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func CPUCountJSON(ctx context.Context, q rowset.Queryer, options CPUCountOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := cPUCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetCPUCount(r, p); return err })
}
