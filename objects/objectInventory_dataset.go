package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ObjectInventoryRow preserves source values and SQL NULL without application policy.
type ObjectInventoryRow struct {
	Owner  session.Value
	Type   session.Value
	Status session.Value
	Count  session.Value
}

// ObjectInventoryOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ObjectInventoryOptions struct {
	OracleMajor int
	Owners      []string
}

// ObjectInventoryRequest supplies admitted options and a caller-owned SQL reference.
type ObjectInventoryRequest struct {
	Options   ObjectInventoryOptions
	Reference reference.Reference
}

// ObjectInventoryResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ObjectInventoryResult struct {
	Provenance DatasetProvenance
	Rows       []ObjectInventoryRow
}

// ObjectInventoryStatement selects reviewed SQL with bound values, owning no connection.
func ObjectInventoryStatement(options ObjectInventoryOptions) (string, []any, error) {
	p, err := objectInventoryDatasetPlan(options)
	return p.sql, p.args, err
}
func objectInventoryDatasetPlan(options ObjectInventoryOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if len(options.Owners) > 1000 {
		return datasetSelection{}, ErrInvalidRequest
	}
	for _, o := range options.Owners {
		if _, err := QuoteIdentifier(o); err != nil {
			return datasetSelection{}, ErrInvalidRequest
		}
	}
	args := make([]any, len(options.Owners))
	where := ""
	if len(options.Owners) > 0 {
		binds := make([]string, len(options.Owners))
		for i, o := range options.Owners {
			args[i] = o
			binds[i] = "NLSSORT(:" + strconv.Itoa(i+1) + ", 'NLS_SORT=BINARY')"
		}
		where = " WHERE NLSSORT(OWNER, 'NLS_SORT=BINARY') IN (" + strings.Join(binds, ",") + ")"
	}
	query := "SELECT OWNER AS OWNER, OBJECT_TYPE AS OBJECT_TYPE, STATUS AS STATUS, TO_CHAR(COUNT(*),'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS OBJECT_COUNT FROM DBA_OBJECTS" + where + " GROUP BY OWNER,OBJECT_TYPE,STATUS ORDER BY OWNER,OBJECT_TYPE,STATUS"
	return datasetSelection{sql: query, args: args, schema: []rowset.Column{{Name: "OWNER", Kind: rowset.Text}, {Name: "OBJECT_TYPE", Kind: rowset.Text}, {Name: "STATUS", Kind: rowset.Text}, {Name: "OBJECT_COUNT", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_OBJECTS", OracleMajor: options.OracleMajor, Scope: "dictionary_visible_owner_type_status_groups", Owners: append([]string(nil), options.Owners...)}}, nil
}

// DecodeObjectInventory validates the entire shape and every row before returning facts.
func DecodeObjectInventory(result session.Result, options ObjectInventoryOptions) (ObjectInventoryResult, error) {
	p, err := objectInventoryDatasetPlan(options)
	if err != nil {
		return ObjectInventoryResult{}, err
	}
	return decodeDatasetObjectInventory(result, p)
}
func decodeDatasetObjectInventory(result session.Result, p datasetSelection) (ObjectInventoryResult, error) {
	index, err := datasetColumns(result, p, "OBJECT_COUNT")
	if err != nil {
		return ObjectInventoryResult{}, err
	}
	for _, r := range result.Rows {
		if len(p.provenance.Owners) > 0 {
			v := r[index["OWNER"]]
			ok := false
			for _, o := range p.provenance.Owners {
				ok = ok || (!v.Null && v.Text == o)
			}
			if !ok {
				return ObjectInventoryResult{}, ErrInvalidEvidence
			}
		}
	}
	rows := make([]ObjectInventoryRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ObjectInventoryRow{Owner: r[index["OWNER"]], Type: r[index["OBJECT_TYPE"]], Status: r[index["STATUS"]], Count: r[index["OBJECT_COUNT"]]}
	}
	return ObjectInventoryResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadObjectInventory executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadObjectInventory(ctx context.Context, reader Reader, request ObjectInventoryRequest) (ObjectInventoryResult, error) {
	p, err := objectInventoryDatasetPlan(request.Options)
	if err != nil {
		return ObjectInventoryResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ObjectInventoryResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ObjectInventoryResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ObjectInventoryResult{}, err
	}
	facts, err := decodeDatasetObjectInventory(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ObjectInventoryResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ObjectInventoryRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ObjectInventoryRows(ctx context.Context, q rowset.Queryer, options ObjectInventoryOptions) (*sql.Rows, error) {
	p, err := objectInventoryDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ObjectInventoryJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ObjectInventoryJSON(ctx context.Context, q rowset.Queryer, options ObjectInventoryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := objectInventoryDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetObjectInventory(r, p); return err })
}
