package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ConfigurationRow preserves source values and SQL NULL without application policy.
type ConfigurationRow struct{ Value session.Value }

// ConfigurationOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ConfigurationOptions struct{ OracleMajor int }

// ConfigurationRequest supplies admitted options and a caller-owned SQL reference.
type ConfigurationRequest struct {
	Options   ConfigurationOptions
	Reference reference.Reference
}

// ConfigurationResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ConfigurationResult struct {
	Provenance DatasetProvenance
	Rows       []ConfigurationRow
}

// ConfigurationStatement selects reviewed SQL with bound values, owning no connection.
func ConfigurationStatement(options ConfigurationOptions) (string, []any, error) {
	p, err := configurationDatasetPlan(options)
	return p.sql, p.args, err
}
func configurationDatasetPlan(options ConfigurationOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT VALUE AS VALUE FROM V$PARAMETER WHERE NAME='cluster_database'"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PARAMETER", OracleMajor: options.OracleMajor, Scope: "current_session_cluster_database_parameter_raw"}}, nil
}

// DecodeConfiguration validates the entire shape and every row before returning facts.
func DecodeConfiguration(result session.Result, options ConfigurationOptions) (ConfigurationResult, error) {
	p, err := configurationDatasetPlan(options)
	if err != nil {
		return ConfigurationResult{}, err
	}
	return decodeDatasetConfiguration(result, p)
}
func decodeDatasetConfiguration(result session.Result, p datasetSelection) (ConfigurationResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return ConfigurationResult{}, err
	}
	if len(result.Rows) > 1 {
		return ConfigurationResult{}, ErrInvalidEvidence
	}
	rows := make([]ConfigurationRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ConfigurationRow{Value: r[index["VALUE"]]}
	}
	return ConfigurationResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadConfiguration executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadConfiguration(ctx context.Context, reader Reader, request ConfigurationRequest) (ConfigurationResult, error) {
	p, err := configurationDatasetPlan(request.Options)
	if err != nil {
		return ConfigurationResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ConfigurationResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ConfigurationResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConfigurationResult{}, err
	}
	facts, err := decodeDatasetConfiguration(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConfigurationResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ConfigurationRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ConfigurationRows(ctx context.Context, q rowset.Queryer, options ConfigurationOptions) (*sql.Rows, error) {
	p, err := configurationDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ConfigurationJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ConfigurationJSON(ctx context.Context, q rowset.Queryer, options ConfigurationOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := configurationDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetConfiguration(r, p); return err })
}
