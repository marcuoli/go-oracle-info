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

// NLSInstanceRow preserves source values and SQL NULL without application policy.
type NLSInstanceRow struct {
	Name  session.Value
	Value session.Value
}

// NLSInstanceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type NLSInstanceOptions struct{ OracleMajor int }

// NLSInstanceRequest supplies admitted options and a caller-owned SQL reference.
type NLSInstanceRequest struct {
	Options   NLSInstanceOptions
	Reference reference.Reference
}

// NLSInstanceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type NLSInstanceResult struct {
	Provenance DatasetProvenance
	Rows       []NLSInstanceRow
}

// NLSInstanceStatement selects reviewed SQL with bound values, owning no connection.
func NLSInstanceStatement(options NLSInstanceOptions) (string, []any, error) {
	p, err := nLSInstanceDatasetPlan(options)
	return p.sql, p.args, err
}
func nLSInstanceDatasetPlan(options NLSInstanceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := strings.Replace(NLSInstanceSQL, ", VALUE FROM", ", VALUE AS VALUE FROM", 1)
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "NLS_INSTANCE_PARAMETERS", OracleMajor: options.OracleMajor, Scope: "instance_nls_values"}}, nil
}

// DecodeNLSInstance validates the entire shape and every row before returning facts.
func DecodeNLSInstance(result session.Result, options NLSInstanceOptions) (NLSInstanceResult, error) {
	p, err := nLSInstanceDatasetPlan(options)
	if err != nil {
		return NLSInstanceResult{}, err
	}
	return decodeDatasetNLSInstance(result, p)
}
func decodeDatasetNLSInstance(result session.Result, p datasetSelection) (NLSInstanceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return NLSInstanceResult{}, err
	}
	rows := make([]NLSInstanceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = NLSInstanceRow{Name: r[index["NAME"]], Value: r[index["VALUE"]]}
	}
	return NLSInstanceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadNLSInstance executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadNLSInstance(ctx context.Context, reader Reader, request NLSInstanceRequest) (NLSInstanceResult, error) {
	p, err := nLSInstanceDatasetPlan(request.Options)
	if err != nil {
		return NLSInstanceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return NLSInstanceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return NLSInstanceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return NLSInstanceResult{}, err
	}
	facts, err := decodeDatasetNLSInstance(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return NLSInstanceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// NLSInstanceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func NLSInstanceRows(ctx context.Context, q rowset.Queryer, options NLSInstanceOptions) (*sql.Rows, error) {
	p, err := nLSInstanceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// NLSInstanceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func NLSInstanceJSON(ctx context.Context, q rowset.Queryer, options NLSInstanceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := nLSInstanceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetNLSInstance(r, p); return err })
}
