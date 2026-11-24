package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MaxParallelDegree is the API's bounded hint degree, not a database capacity
// limit or a promise that Oracle will honor the hint or finish the count sooner.
const MaxParallelDegree = 1024

// CountOptions controls an optional statement hint. Zero ParallelDegree preserves
// the legacy statement exactly; positive values request PARALLEL(n). The caller
// owns workload admission, query concurrency, timeouts and database qualification.
type CountOptions struct {
	ParallelDegree int
}

// CountStatementWithOptions returns an exact decimal COUNT(*) for safely quoted
// identifiers and a validated integer degree. It never changes session settings.
// Oracle may ignore the hint; it cannot establish cutoff consistency or access.
func CountStatementWithOptions(owner, table string, options CountOptions) (string, error) {
	if options.ParallelDegree < 0 || options.ParallelDegree > MaxParallelDegree {
		return "", errors.New("parallel degree outside 0..1024")
	}
	o, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	t, err := QuoteIdentifier(table)
	if err != nil {
		return "", err
	}
	selectClause := "SELECT "
	if options.ParallelDegree > 0 {
		selectClause += "/*+ PARALLEL(" + strconv.Itoa(options.ParallelDegree) + ") */ "
	}
	return selectClause + "TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT FROM " + o + "." + t, nil
}

// CountSubobjectStatement returns an exact decimal COUNT(*) for a named
// partition or subpartition of a caller-admitted ordinary table. A subpartition
// requires its admitted parent identity. Oracle addresses a subpartition by its
// table-unique name, so the caller must confirm the parent-child relationship.
// The caller also owns workload, connection and cutoff admission.
func CountSubobjectStatement(owner, table, partition, subpartition string, options CountOptions) (string, error) {
	name, err := QuoteIdentifier(partition)
	if err != nil {
		return "", err
	}
	clause := " PARTITION"
	if subpartition != "" {
		name, err = QuoteIdentifier(subpartition)
		if err != nil {
			return "", err
		}
		clause = " SUBPARTITION"
	}
	statement, err := CountStatementWithOptions(owner, table, options)
	if err != nil {
		return "", err
	}
	return statement + clause + " (" + name + ")", nil
}

// CountStatement returns an exact decimal COUNT(*) for one safely quoted table.
// Callers must first admit the object as an ordinary table in the intended scope;
// this function cannot establish cost, cutoff consistency or access permission.
func CountStatement(owner, table string) (string, error) {
	return CountStatementWithOptions(owner, table, CountOptions{})
}

// TableCountRow preserves source values and SQL NULL without application policy.
type TableCountRow struct{ Count session.Value }

// TableCountOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type TableCountOptions struct {
	OracleMajor    int
	Owner          string
	Table          string
	ParallelDegree int
}

// TableCountRequest supplies admitted options and a caller-owned SQL reference.
type TableCountRequest struct {
	Options   TableCountOptions
	Reference reference.Reference
}

// TableCountResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type TableCountResult struct {
	Provenance DatasetProvenance
	Rows       []TableCountRow
}

// TableCountStatement selects reviewed SQL with bound values, owning no connection.
func TableCountStatement(options TableCountOptions) (string, []any, error) {
	p, err := tableCountDatasetPlan(options)
	return p.sql, p.args, err
}

func tableCountDatasetPlan(options TableCountOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	if _, err := QuoteIdentifier(options.Table); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	query, err := CountStatementWithOptions(options.Owner, options.Table, CountOptions{ParallelDegree: options.ParallelDegree})
	if err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "ROW_COUNT", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "explicit_quoted_ordinary_table", OracleMajor: options.OracleMajor, Scope: "caller_admitted_exact_table_count_not_cutoff_or_cost_approval", Owner: options.Owner, Table: options.Table}}, nil
}

// DecodeTableCount validates the entire shape and every row before returning facts.
func DecodeTableCount(result session.Result, options TableCountOptions) (TableCountResult, error) {
	p, err := tableCountDatasetPlan(options)
	if err != nil {
		return TableCountResult{}, err
	}
	return decodeDatasetTableCount(result, p)
}

func decodeDatasetTableCount(result session.Result, p datasetSelection) (TableCountResult, error) {
	index, err := datasetColumns(result, p, "ROW_COUNT")
	if err != nil {
		return TableCountResult{}, err
	}
	if len(result.Rows) != 1 {
		return TableCountResult{}, ErrInvalidEvidence
	}
	rows := make([]TableCountRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = TableCountRow{Count: r[index["ROW_COUNT"]]}
	}
	return TableCountResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTableCount executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadTableCount(ctx context.Context, reader Reader, request TableCountRequest) (TableCountResult, error) {
	p, err := tableCountDatasetPlan(request.Options)
	if err != nil {
		return TableCountResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TableCountResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TableCountResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableCountResult{}, err
	}
	facts, err := decodeDatasetTableCount(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return TableCountResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TableCountRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func TableCountRows(ctx context.Context, q rowset.Queryer, options TableCountOptions) (*sql.Rows, error) {
	p, err := tableCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// TableCountJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func TableCountJSON(ctx context.Context, q rowset.Queryer, options TableCountOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tableCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetTableCount(r, p); return err })
}
