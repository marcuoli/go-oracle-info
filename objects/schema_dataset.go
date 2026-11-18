package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SchemaRow preserves source values and SQL NULL without application policy.
type SchemaRow struct{ Name session.Value }

// SchemaOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SchemaOptions struct {
	OracleMajor int
	Owner       string
}

// SchemaRequest supplies admitted options and a caller-owned SQL reference.
type SchemaRequest struct {
	Options   SchemaOptions
	Reference reference.Reference
}

// SchemaResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SchemaResult struct {
	Provenance DatasetProvenance
	Rows       []SchemaRow
}

// SchemaStatement selects reviewed SQL with bound values, owning no connection.
func SchemaStatement(options SchemaOptions) (string, []any, error) {
	p, err := schemaDatasetPlan(options)
	return p.sql, p.args, err
}
func schemaDatasetPlan(options SchemaOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(SchemaSQL, "SELECT USERNAME FROM", "SELECT USERNAME AS USERNAME FROM", 1)
	query = strings.Replace(query, "USERNAME=:1", "NLSSORT(USERNAME, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')", 1)
	return datasetSelection{sql: query, args: []any{options.Owner}, schema: []rowset.Column{{Name: "USERNAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_USERS", OracleMajor: options.OracleMajor, Scope: "current_container_exact_schema_lookup", Owner: options.Owner}}, nil
}

// DecodeSchema validates the entire shape and every row before returning facts.
func DecodeSchema(result session.Result, options SchemaOptions) (SchemaResult, error) {
	p, err := schemaDatasetPlan(options)
	if err != nil {
		return SchemaResult{}, err
	}
	return decodeDatasetSchema(result, p)
}
func decodeDatasetSchema(result session.Result, p datasetSelection) (SchemaResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SchemaResult{}, err
	}
	if len(result.Rows) > 1 {
		return SchemaResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["USERNAME"]]
		if v.Null || v.Text != p.provenance.Owner {
			return SchemaResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]SchemaRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = SchemaRow{Name: r[index["USERNAME"]]}
	}
	return SchemaResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSchema executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadSchema(ctx context.Context, reader Reader, request SchemaRequest) (SchemaResult, error) {
	p, err := schemaDatasetPlan(request.Options)
	if err != nil {
		return SchemaResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SchemaResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SchemaResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SchemaResult{}, err
	}
	facts, err := decodeDatasetSchema(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SchemaResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SchemaRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func SchemaRows(ctx context.Context, q rowset.Queryer, options SchemaOptions) (*sql.Rows, error) {
	p, err := schemaDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// SchemaJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func SchemaJSON(ctx context.Context, q rowset.Queryer, options SchemaOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := schemaDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetSchema(r, p); return err })
}
