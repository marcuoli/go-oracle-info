package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// LagRow preserves source values and SQL NULL without application policy.
type LagRow struct {
	Name         session.Value
	Value        session.Value
	Unit         session.Value
	TimeComputed session.Value
	DatumTime    session.Value
}

// LagOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type LagOptions struct{ OracleMajor int }

// LagRequest supplies admitted options and a caller-owned SQL reference.
type LagRequest struct {
	Options   LagOptions
	Reference reference.Reference
}

// LagResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type LagResult struct {
	Provenance DatasetProvenance
	Rows       []LagRow
}

// LagStatement selects reviewed SQL with bound values, owning no connection.
func LagStatement(options LagOptions) (string, []any, error) {
	p, err := lagDatasetPlan(options)
	return p.sql, p.args, err
}
func lagDatasetPlan(options LagOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT NAME AS NAME, VALUE AS VALUE, UNIT AS UNIT, TIME_COMPUTED AS TIME_COMPUTED, DATUM_TIME AS DATUM_TIME FROM V$DATAGUARD_STATS WHERE NAME IN ('transport lag','apply lag','apply finish time') ORDER BY NAME"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}, {Name: "UNIT", Kind: rowset.Text}, {Name: "TIME_COMPUTED", Kind: rowset.Text}, {Name: "DATUM_TIME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATAGUARD_STATS", OracleMajor: options.OracleMajor, Scope: "connected_database_available_lag_observations"}}, nil
}

// DecodeLag validates the entire shape and every row before returning facts.
func DecodeLag(result session.Result, options LagOptions) (LagResult, error) {
	p, err := lagDatasetPlan(options)
	if err != nil {
		return LagResult{}, err
	}
	return decodeDatasetLag(result, p)
}
func decodeDatasetLag(result session.Result, p datasetSelection) (LagResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return LagResult{}, err
	}
	rows := make([]LagRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = LagRow{Name: r[index["NAME"]], Value: r[index["VALUE"]], Unit: r[index["UNIT"]], TimeComputed: r[index["TIME_COMPUTED"]], DatumTime: r[index["DATUM_TIME"]]}
	}
	return LagResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadLag executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadLag(ctx context.Context, reader Reader, request LagRequest) (LagResult, error) {
	p, err := lagDatasetPlan(request.Options)
	if err != nil {
		return LagResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return LagResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return LagResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return LagResult{}, err
	}
	facts, err := decodeDatasetLag(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return LagResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// LagRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func LagRows(ctx context.Context, q rowset.Queryer, options LagOptions) (*sql.Rows, error) {
	p, err := lagDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// LagJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func LagJSON(ctx context.Context, q rowset.Queryer, options LagOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := lagDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetLag(r, p); return err })
}
