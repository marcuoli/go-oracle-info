package catalog

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ConfigurationRow preserves source values and SQL NULL without application policy.
type ConfigurationRow struct {
	InstanceID session.Value
	CellHash   session.Value
	CellName   session.Value
	SourceType session.Value
	Payload    session.Value
	CodeUnits  session.Value
}

// ConfigurationOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ConfigurationOptions struct {
	OracleMajor int
	Schema      Schema
	Info        bool
}

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
	if err := ValidateSchema(options.Schema); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	command, ok, err := Configuration(options.Schema, options.Info)
	if err != nil {
		return datasetSelection{}, err
	}
	if !ok {
		return datasetSelection{}, ErrUnsupportedSource
	}
	query := command.Text
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CELL_HASHVAL", Kind: rowset.Text}, {Name: "CELL_NAME", Kind: rowset.Text}, {Name: "SOURCE_TYPE", Kind: rowset.Text}, {Name: "XML_PAYLOAD", Kind: rowset.Text}, {Name: "XML_LENGTH", Kind: rowset.Integer}}, provenance: DatasetProvenance{Source: command.View, OracleMajor: options.OracleMajor, Scope: "admitted_reporter_raw_xml_configuration"}}, nil
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
	for _, r := range result.Rows {
		if !validDatasetInstance(r[index["INST_ID"]]) || !completeDatasetPayload(r[index["XML_PAYLOAD"]], r[index["XML_LENGTH"]]) {
			return ConfigurationResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]ConfigurationRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ConfigurationRow{InstanceID: r[index["INST_ID"]], CellHash: r[index["CELL_HASHVAL"]], CellName: r[index["CELL_NAME"]], SourceType: r[index["SOURCE_TYPE"]], Payload: r[index["XML_PAYLOAD"]], CodeUnits: r[index["XML_LENGTH"]]}
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
