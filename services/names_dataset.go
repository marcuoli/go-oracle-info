package services

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ServiceNameRow preserves source values and SQL NULL without application policy.
type ServiceNameRow struct{ Name session.Value }

// NamesOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type NamesOptions struct{ OracleMajor int }

// NamesRequest supplies admitted options and a caller-owned SQL reference.
type NamesRequest struct {
	Options   NamesOptions
	Reference reference.Reference
}

// NamesResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type NamesResult struct {
	Provenance DatasetProvenance
	Rows       []ServiceNameRow
}

// NamesStatement selects reviewed SQL with bound values, owning no connection.
func NamesStatement(options NamesOptions) (string, []any, error) {
	p, err := namesDatasetPlan(options)
	return p.sql, p.args, err
}
func namesDatasetPlan(options NamesOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := NamesSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "SERVICE_NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$SERVICES", OracleMajor: options.OracleMajor, Scope: "connected_container_excluding_internal_session_services"}}, nil
}

// DecodeNames validates the entire shape and every row before returning facts.
func DecodeNames(result session.Result, options NamesOptions) (NamesResult, error) {
	p, err := namesDatasetPlan(options)
	if err != nil {
		return NamesResult{}, err
	}
	return decodeDatasetNames(result, p)
}
func decodeDatasetNames(result session.Result, p datasetSelection) (NamesResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return NamesResult{}, err
	}
	for _, r := range result.Rows {
		v := r[index["SERVICE_NAME"]]
		if !v.Null && (v.Text == "SYS$BACKGROUND" || v.Text == "SYS$USERS") {
			return NamesResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]ServiceNameRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ServiceNameRow{Name: r[index["SERVICE_NAME"]]}
	}
	return NamesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadNames executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadNames(ctx context.Context, reader Reader, request NamesRequest) (NamesResult, error) {
	p, err := namesDatasetPlan(request.Options)
	if err != nil {
		return NamesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return NamesResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return NamesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return NamesResult{}, err
	}
	facts, err := decodeDatasetNames(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return NamesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// NamesRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func NamesRows(ctx context.Context, q rowset.Queryer, options NamesOptions) (*sql.Rows, error) {
	p, err := namesDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// NamesJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func NamesJSON(ctx context.Context, q rowset.Queryer, options NamesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := namesDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetNames(r, p); return err })
}
