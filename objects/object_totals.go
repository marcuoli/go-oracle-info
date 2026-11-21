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

// ObjectTotalsRow preserves source values and SQL NULL without application policy.
type ObjectTotalsRow struct {
	Type    session.Value
	Total   session.Value
	Valid   session.Value
	Invalid session.Value
	Other   session.Value
}

// ObjectTotalsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ObjectTotalsOptions struct {
	OracleMajor int
	Owner       string
}

// ObjectTotalsRequest supplies admitted options and a caller-owned SQL reference.
type ObjectTotalsRequest struct {
	Options   ObjectTotalsOptions
	Reference reference.Reference
}

// ObjectTotalsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ObjectTotalsResult struct {
	Provenance DatasetProvenance
	Rows       []ObjectTotalsRow
}

// ObjectTotalsStatement selects reviewed SQL with bound values, owning no connection.
func ObjectTotalsStatement(options ObjectTotalsOptions) (string, []any, error) {
	p, err := objectTotalsDatasetPlan(options)
	return p.sql, p.args, err
}

func objectTotalsDatasetPlan(options ObjectTotalsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := strings.Replace(TotalsSQL, "SELECT OBJECT_TYPE,", "SELECT OBJECT_TYPE AS OBJECT_TYPE,", 1)
	query = strings.NewReplacer("OWNER=:1", "NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')").Replace(query)
	return datasetSelection{sql: query, args: []any{options.Owner}, schema: []rowset.Column{{Name: "OBJECT_TYPE", Kind: rowset.Text}, {Name: "TOTAL", Kind: rowset.Text}, {Name: "VALID_COUNT", Kind: rowset.Text}, {Name: "INVALID_COUNT", Kind: rowset.Text}, {Name: "OTHER_COUNT", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_OBJECTS", OracleMajor: options.OracleMajor, Scope: "current_container_exact_owner_status_totals", Owner: options.Owner}}, nil
}

// DecodeObjectTotals validates the entire shape and every row before returning facts.
func DecodeObjectTotals(result session.Result, options ObjectTotalsOptions) (ObjectTotalsResult, error) {
	p, err := objectTotalsDatasetPlan(options)
	if err != nil {
		return ObjectTotalsResult{}, err
	}
	return decodeDatasetObjectTotals(result, p)
}

func decodeDatasetObjectTotals(result session.Result, p datasetSelection) (ObjectTotalsResult, error) {
	index, err := datasetColumns(result, p, "TOTAL", "VALID_COUNT", "INVALID_COUNT", "OTHER_COUNT")
	if err != nil {
		return ObjectTotalsResult{}, err
	}
	rows := make([]ObjectTotalsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ObjectTotalsRow{Type: r[index["OBJECT_TYPE"]], Total: r[index["TOTAL"]], Valid: r[index["VALID_COUNT"]], Invalid: r[index["INVALID_COUNT"]], Other: r[index["OTHER_COUNT"]]}
	}
	return ObjectTotalsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadObjectTotals executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadObjectTotals(ctx context.Context, reader Reader, request ObjectTotalsRequest) (ObjectTotalsResult, error) {
	p, err := objectTotalsDatasetPlan(request.Options)
	if err != nil {
		return ObjectTotalsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ObjectTotalsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ObjectTotalsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ObjectTotalsResult{}, err
	}
	facts, err := decodeDatasetObjectTotals(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ObjectTotalsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ObjectTotalsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ObjectTotalsRows(ctx context.Context, q rowset.Queryer, options ObjectTotalsOptions) (*sql.Rows, error) {
	p, err := objectTotalsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ObjectTotalsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ObjectTotalsJSON(ctx context.Context, q rowset.Queryer, options ObjectTotalsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := objectTotalsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetObjectTotals(r, p); return err })
}

// TotalsSQL counts each object status class for the bound owner (:1).
const TotalsSQL = `SELECT OBJECT_TYPE, TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS TOTAL, TO_CHAR(SUM(CASE WHEN STATUS='VALID' THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALID_COUNT, TO_CHAR(SUM(CASE WHEN STATUS='INVALID' THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS INVALID_COUNT, TO_CHAR(SUM(CASE WHEN STATUS NOT IN ('VALID','INVALID') OR STATUS IS NULL THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OTHER_COUNT FROM DBA_OBJECTS WHERE OWNER=:1 GROUP BY OBJECT_TYPE ORDER BY OBJECT_TYPE`
