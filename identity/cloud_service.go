package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// CloudServiceRow preserves the executing session's raw CLOUD_SERVICE value and SQL NULL.
type CloudServiceRow struct {
	Value session.Value
}

// CloudServiceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type CloudServiceOptions struct{ OracleMajor int }

// CloudServiceRequest supplies admitted options and a caller-owned SQL reference.
type CloudServiceRequest struct {
	Options   CloudServiceOptions
	Reference reference.Reference
}

// CloudServiceResult retains one session value and source provenance.
// Decode sets no transport completion flag. Read and JSON require completion.
type CloudServiceResult struct {
	Provenance DatasetProvenance
	Rows       []CloudServiceRow
}

// CloudServiceStatement selects reviewed SQL with bound values, owning no connection.
func CloudServiceStatement(options CloudServiceOptions) (string, []any, error) {
	p, err := cloudServiceDatasetPlan(options)
	return p.sql, p.args, err
}

func cloudServiceDatasetPlan(options CloudServiceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT SYS_CONTEXT('USERENV','CLOUD_SERVICE') AS CLOUD_SERVICE FROM DUAL"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "CLOUD_SERVICE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "SYS_CONTEXT(USERENV.CLOUD_SERVICE)", OracleMajor: options.OracleMajor, Scope: "executing_session_cloud_service"}}, nil
}

// DecodeCloudService validates the entire shape and every row before returning facts.
func DecodeCloudService(result session.Result, options CloudServiceOptions) (CloudServiceResult, error) {
	p, err := cloudServiceDatasetPlan(options)
	if err != nil {
		return CloudServiceResult{}, err
	}
	return decodeDatasetCloudService(result, p)
}

func decodeDatasetCloudService(result session.Result, p datasetSelection) (CloudServiceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return CloudServiceResult{}, err
	}
	if len(result.Rows) != 1 {
		return CloudServiceResult{}, ErrInvalidEvidence
	}
	rows := []CloudServiceRow{{Value: result.Rows[0][index["CLOUD_SERVICE"]]}}
	return CloudServiceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCloudService executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadCloudService(ctx context.Context, reader Reader, request CloudServiceRequest) (CloudServiceResult, error) {
	p, err := cloudServiceDatasetPlan(request.Options)
	if err != nil {
		return CloudServiceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return CloudServiceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return CloudServiceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return CloudServiceResult{}, err
	}
	facts, err := decodeDatasetCloudService(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return CloudServiceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// CloudServiceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func CloudServiceRows(ctx context.Context, q rowset.Queryer, options CloudServiceOptions) (*sql.Rows, error) {
	p, err := cloudServiceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// CloudServiceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func CloudServiceJSON(ctx context.Context, q rowset.Queryer, options CloudServiceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := cloudServiceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetCloudService(r, p); return err })
}
