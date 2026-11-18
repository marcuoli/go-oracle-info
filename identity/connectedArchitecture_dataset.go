package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ConnectedArchitectureRow preserves source values and SQL NULL without application policy.
type ConnectedArchitectureRow struct {
	CDB         session.Value
	ContainerID session.Value
	Name        session.Value
}

// ConnectedArchitectureOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ConnectedArchitectureOptions struct{ OracleMajor int }

// ConnectedArchitectureRequest supplies admitted options and a caller-owned SQL reference.
type ConnectedArchitectureRequest struct {
	Options   ConnectedArchitectureOptions
	Reference reference.Reference
}

// ConnectedArchitectureResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ConnectedArchitectureResult struct {
	Provenance DatasetProvenance
	Rows       []ConnectedArchitectureRow
}

// ConnectedArchitectureStatement selects reviewed SQL with bound values, owning no connection.
func ConnectedArchitectureStatement(options ConnectedArchitectureOptions) (string, []any, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	return p.sql, p.args, err
}
func connectedArchitectureDatasetPlan(options ConnectedArchitectureOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := ConnectedArchitectureSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "CDB", Kind: rowset.Text}, {Name: "CONTAINER_ID", Kind: rowset.Text}, {Name: "CONTAINER_NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE+SYS_CONTEXT(USERENV)", OracleMajor: options.OracleMajor, Scope: "parent_cdb_flag_and_actual_current_session_container"}}, nil
}

// DecodeConnectedArchitecture validates the entire shape and every row before returning facts.
func DecodeConnectedArchitecture(result session.Result, options ConnectedArchitectureOptions) (ConnectedArchitectureResult, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	return decodeDatasetConnectedArchitecture(result, p)
}
func decodeDatasetConnectedArchitecture(result session.Result, p datasetSelection) (ConnectedArchitectureResult, error) {
	index, err := datasetColumns(result, p, "CONTAINER_ID")
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	if len(result.Rows) > 1 {
		return ConnectedArchitectureResult{}, ErrInvalidEvidence
	}
	rows := make([]ConnectedArchitectureRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ConnectedArchitectureRow{CDB: r[index["CDB"]], ContainerID: r[index["CONTAINER_ID"]], Name: r[index["CONTAINER_NAME"]]}
	}
	return ConnectedArchitectureResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadConnectedArchitecture executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadConnectedArchitecture(ctx context.Context, reader Reader, request ConnectedArchitectureRequest) (ConnectedArchitectureResult, error) {
	p, err := connectedArchitectureDatasetPlan(request.Options)
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ConnectedArchitectureResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	facts, err := decodeDatasetConnectedArchitecture(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ConnectedArchitectureRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ConnectedArchitectureRows(ctx context.Context, q rowset.Queryer, options ConnectedArchitectureOptions) (*sql.Rows, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ConnectedArchitectureJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ConnectedArchitectureJSON(ctx context.Context, q rowset.Queryer, options ConnectedArchitectureOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetConnectedArchitecture(r, p); return err })
}
