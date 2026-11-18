package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ContainerSourceRow preserves source values and SQL NULL without application policy.
type ContainerSourceRow struct {
	Name session.Value
	CDB  session.Value
}

// ContainerSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ContainerSourceOptions struct{ OracleMajor int }

// ContainerSourceRequest supplies admitted options and a caller-owned SQL reference.
type ContainerSourceRequest struct {
	Options   ContainerSourceOptions
	Reference reference.Reference
}

// ContainerSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ContainerSourceResult struct {
	Provenance DatasetProvenance
	Rows       []ContainerSourceRow
}

// ContainerSourceStatement selects reviewed SQL with bound values, owning no connection.
func ContainerSourceStatement(options ContainerSourceOptions) (string, []any, error) {
	p, err := containerSourceDatasetPlan(options)
	return p.sql, p.args, err
}
func containerSourceDatasetPlan(options ContainerSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT SYS_CONTEXT('USERENV','CON_NAME') AS CONTAINER, CDB AS CDB FROM V$DATABASE"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "CONTAINER", Kind: rowset.Text}, {Name: "CDB", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE+SYS_CONTEXT(USERENV.CON_NAME)", OracleMajor: options.OracleMajor, Scope: "current_session_name_and_parent_cdb_flag"}}, nil
}

// DecodeContainerSource validates the entire shape and every row before returning facts.
func DecodeContainerSource(result session.Result, options ContainerSourceOptions) (ContainerSourceResult, error) {
	p, err := containerSourceDatasetPlan(options)
	if err != nil {
		return ContainerSourceResult{}, err
	}
	return decodeDatasetContainerSource(result, p)
}
func decodeDatasetContainerSource(result session.Result, p datasetSelection) (ContainerSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return ContainerSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return ContainerSourceResult{}, ErrInvalidEvidence
	}
	rows := make([]ContainerSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ContainerSourceRow{Name: r[index["CONTAINER"]], CDB: r[index["CDB"]]}
	}
	return ContainerSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadContainerSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadContainerSource(ctx context.Context, reader Reader, request ContainerSourceRequest) (ContainerSourceResult, error) {
	p, err := containerSourceDatasetPlan(request.Options)
	if err != nil {
		return ContainerSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ContainerSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ContainerSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ContainerSourceResult{}, err
	}
	facts, err := decodeDatasetContainerSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ContainerSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ContainerSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ContainerSourceRows(ctx context.Context, q rowset.Queryer, options ContainerSourceOptions) (*sql.Rows, error) {
	p, err := containerSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ContainerSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ContainerSourceJSON(ctx context.Context, q rowset.Queryer, options ContainerSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := containerSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetContainerSource(r, p); return err })
}
