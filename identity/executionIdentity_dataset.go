package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ExecutionIdentityRow preserves source values and SQL NULL without application policy.
type ExecutionIdentityRow struct {
	Database    session.Value
	DBID        session.Value
	Version     session.Value
	InstanceID  session.Value
	ContainerID session.Value
	Service     session.Value
	StartupKey  session.Value
}

// ExecutionIdentityOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ExecutionIdentityOptions struct{ OracleMajor int }

// ExecutionIdentityRequest supplies admitted options and a caller-owned SQL reference.
type ExecutionIdentityRequest struct {
	Options   ExecutionIdentityOptions
	Reference reference.Reference
}

// ExecutionIdentityResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ExecutionIdentityResult struct {
	Provenance DatasetProvenance
	Rows       []ExecutionIdentityRow
}

// ExecutionIdentityStatement selects reviewed SQL with bound values, owning no connection.
func ExecutionIdentityStatement(options ExecutionIdentityOptions) (string, []any, error) {
	p, err := executionIdentityDatasetPlan(options)
	return p.sql, p.args, err
}
func executionIdentityDatasetPlan(options ExecutionIdentityOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := ExecutionStatement()
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "DATABASE_NAME", Kind: rowset.Text}, {Name: "DBID", Kind: rowset.Text}, {Name: "ORACLE_VERSION", Kind: rowset.Text}, {Name: "INSTANCE_ID", Kind: rowset.Text}, {Name: "CONTAINER_ID", Kind: rowset.Text}, {Name: "SERVICE_NAME", Kind: rowset.Text}, {Name: "STARTUP_KEY", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE+V$INSTANCE+SYS_CONTEXT(USERENV)", OracleMajor: options.OracleMajor, Scope: "connected_execution_database_instance_session_identity"}}, nil
}

// DecodeExecutionIdentity validates the entire shape and every row before returning facts.
func DecodeExecutionIdentity(result session.Result, options ExecutionIdentityOptions) (ExecutionIdentityResult, error) {
	p, err := executionIdentityDatasetPlan(options)
	if err != nil {
		return ExecutionIdentityResult{}, err
	}
	return decodeDatasetExecutionIdentity(result, p)
}
func decodeDatasetExecutionIdentity(result session.Result, p datasetSelection) (ExecutionIdentityResult, error) {
	index, err := datasetColumns(result, p, "DBID", "INSTANCE_ID", "CONTAINER_ID")
	if err != nil {
		return ExecutionIdentityResult{}, err
	}
	if len(result.Rows) > 1 {
		return ExecutionIdentityResult{}, ErrInvalidEvidence
	}
	if _, err := DecodeExecution(result); err != nil {
		return ExecutionIdentityResult{}, err
	}
	rows := make([]ExecutionIdentityRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ExecutionIdentityRow{Database: r[index["DATABASE_NAME"]], DBID: r[index["DBID"]], Version: r[index["ORACLE_VERSION"]], InstanceID: r[index["INSTANCE_ID"]], ContainerID: r[index["CONTAINER_ID"]], Service: r[index["SERVICE_NAME"]], StartupKey: r[index["STARTUP_KEY"]]}
	}
	return ExecutionIdentityResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadExecutionIdentity executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadExecutionIdentity(ctx context.Context, reader Reader, request ExecutionIdentityRequest) (ExecutionIdentityResult, error) {
	p, err := executionIdentityDatasetPlan(request.Options)
	if err != nil {
		return ExecutionIdentityResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ExecutionIdentityResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ExecutionIdentityResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ExecutionIdentityResult{}, err
	}
	facts, err := decodeDatasetExecutionIdentity(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ExecutionIdentityResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ExecutionIdentityRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ExecutionIdentityRows(ctx context.Context, q rowset.Queryer, options ExecutionIdentityOptions) (*sql.Rows, error) {
	p, err := executionIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ExecutionIdentityJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ExecutionIdentityJSON(ctx context.Context, q rowset.Queryer, options ExecutionIdentityOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := executionIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetExecutionIdentity(r, p); return err })
}
