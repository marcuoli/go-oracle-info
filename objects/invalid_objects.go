// Package objects selects dictionary object inventories and exact table counts.
// Callers retain schema authorization, collection budgets and comparison policy.
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

// InvalidObjectsRow preserves source values and SQL NULL without application policy.
type InvalidObjectsRow struct {
	Owner  session.Value
	Type   session.Value
	Name   session.Value
	Status session.Value
}

// InvalidObjectsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type InvalidObjectsOptions struct {
	OracleMajor int
	Owners      []string
}

// InvalidObjectsRequest supplies admitted options and a caller-owned SQL reference.
type InvalidObjectsRequest struct {
	Options   InvalidObjectsOptions
	Reference reference.Reference
}

// InvalidObjectsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type InvalidObjectsResult struct {
	Provenance DatasetProvenance
	Rows       []InvalidObjectsRow
}

// InvalidObjectsStatement selects reviewed SQL with bound values, owning no connection.
func InvalidObjectsStatement(options InvalidObjectsOptions) (string, []any, error) {
	p, err := invalidObjectsDatasetPlan(options)
	return p.sql, p.args, err
}

func invalidObjectsDatasetPlan(options InvalidObjectsOptions) (datasetSelection, error) {
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
	if where == "" {
		where = " WHERE STATUS='INVALID'"
	} else {
		where += " AND STATUS='INVALID'"
	}
	query := "SELECT OWNER AS OWNER, OBJECT_TYPE AS OBJECT_TYPE, OBJECT_NAME AS OBJECT_NAME, STATUS AS STATUS FROM DBA_OBJECTS" + where + " ORDER BY OWNER,OBJECT_TYPE,OBJECT_NAME"
	return datasetSelection{sql: query, args: args, schema: []rowset.Column{{Name: "OWNER", Kind: rowset.Text}, {Name: "OBJECT_TYPE", Kind: rowset.Text}, {Name: "OBJECT_NAME", Kind: rowset.Text}, {Name: "STATUS", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "DBA_OBJECTS", OracleMajor: options.OracleMajor, Scope: "dictionary_visible_invalid_objects_admitted_owners", Owners: append([]string(nil), options.Owners...)}}, nil
}

// DecodeInvalidObjects validates the entire shape and every row before returning facts.
func DecodeInvalidObjects(result session.Result, options InvalidObjectsOptions) (InvalidObjectsResult, error) {
	p, err := invalidObjectsDatasetPlan(options)
	if err != nil {
		return InvalidObjectsResult{}, err
	}
	return decodeDatasetInvalidObjects(result, p)
}

func decodeDatasetInvalidObjects(result session.Result, p datasetSelection) (InvalidObjectsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return InvalidObjectsResult{}, err
	}
	for _, r := range result.Rows {
		if len(p.provenance.Owners) > 0 {
			v := r[index["OWNER"]]
			ok := false
			for _, o := range p.provenance.Owners {
				ok = ok || (!v.Null && v.Text == o)
			}
			if !ok {
				return InvalidObjectsResult{}, ErrInvalidEvidence
			}
		}
	}
	for _, r := range result.Rows {
		v := r[index["STATUS"]]
		if v.Null || v.Text != "INVALID" {
			return InvalidObjectsResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]InvalidObjectsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = InvalidObjectsRow{Owner: r[index["OWNER"]], Type: r[index["OBJECT_TYPE"]], Name: r[index["OBJECT_NAME"]], Status: r[index["STATUS"]]}
	}
	return InvalidObjectsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadInvalidObjects executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadInvalidObjects(ctx context.Context, reader Reader, request InvalidObjectsRequest) (InvalidObjectsResult, error) {
	p, err := invalidObjectsDatasetPlan(request.Options)
	if err != nil {
		return InvalidObjectsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return InvalidObjectsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return InvalidObjectsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return InvalidObjectsResult{}, err
	}
	facts, err := decodeDatasetInvalidObjects(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return InvalidObjectsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// InvalidObjectsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func InvalidObjectsRows(ctx context.Context, q rowset.Queryer, options InvalidObjectsOptions) (*sql.Rows, error) {
	p, err := invalidObjectsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// InvalidObjectsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func InvalidObjectsJSON(ctx context.Context, q rowset.Queryer, options InvalidObjectsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := invalidObjectsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetInvalidObjects(r, p); return err })
}

// InvalidStatement reads invalid objects in the bound owner list. An empty list
// means all dictionary-visible owners; it does not infer application schemas.
func InvalidStatement(owners ...string) (string, []any) {
	filter, args := ownerFilter(owners)
	if filter == "" {
		filter = " WHERE STATUS='INVALID'"
	} else {
		filter += " AND STATUS='INVALID'"
	}
	return "SELECT OWNER, OBJECT_TYPE, OBJECT_NAME, STATUS FROM DBA_OBJECTS" + filter + " ORDER BY OWNER,OBJECT_TYPE,OBJECT_NAME", args
}
