package memory

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PGAAllocatedRow preserves source values and SQL NULL without application policy.
type PGAAllocatedRow struct{ GiB session.Value }

// PGAAllocatedOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type PGAAllocatedOptions struct{ OracleMajor int }

// PGAAllocatedRequest supplies admitted options and a caller-owned SQL reference.
type PGAAllocatedRequest struct {
	Options   PGAAllocatedOptions
	Reference reference.Reference
}

// PGAAllocatedResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type PGAAllocatedResult struct {
	Provenance DatasetProvenance
	Rows       []PGAAllocatedRow
}

// PGAAllocatedStatement selects reviewed SQL with bound values, owning no connection.
func PGAAllocatedStatement(options PGAAllocatedOptions) (string, []any, error) {
	p, err := pgaAllocatedPlan(options)
	return p.sql, p.args, err
}

func pgaAllocatedPlan(options PGAAllocatedOptions) (datasetSelection, error) {
	if !supportsAllocation(options.OracleMajor) {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	query := PGAAllocatedSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PGASTAT", OracleMajor: options.OracleMajor, Scope: "observed_instance_pga_allocation_gib_rounded_three_decimals"}}, nil
}

// DecodePGAAllocated validates the entire shape and every row before returning facts.
func DecodePGAAllocated(result session.Result, options PGAAllocatedOptions) (PGAAllocatedResult, error) {
	p, err := pgaAllocatedPlan(options)
	if err != nil {
		return PGAAllocatedResult{}, err
	}
	return decodeDatasetPGAAllocated(result, p)
}

func decodeDatasetPGAAllocated(result session.Result, p datasetSelection) (PGAAllocatedResult, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok || len(result.Rows) > 1 {
		return PGAAllocatedResult{}, ErrInvalidObservedEvidence
	}
	rows := make([]PGAAllocatedRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = PGAAllocatedRow{GiB: r[index["VALUE"]]}
	}
	return PGAAllocatedResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadPGAAllocated executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadPGAAllocated(ctx context.Context, reader Reader, request PGAAllocatedRequest) (PGAAllocatedResult, error) {
	p, err := pgaAllocatedPlan(request.Options)
	if err != nil {
		return PGAAllocatedResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return PGAAllocatedResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return PGAAllocatedResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return PGAAllocatedResult{}, err
	}
	facts, err := decodeDatasetPGAAllocated(result, p)
	if err = datasetReadError(ctx, p, projection.MarkDecodeError(err)); err != nil {
		return PGAAllocatedResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PGAAllocatedRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func PGAAllocatedRows(ctx context.Context, q rowset.Queryer, options PGAAllocatedOptions) (*sql.Rows, error) {
	p, err := pgaAllocatedPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// PGAAllocatedJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func PGAAllocatedJSON(ctx context.Context, q rowset.Queryer, options PGAAllocatedOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := pgaAllocatedPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, projection.MarkDecodeError(ErrInvalidObservedEvidence), func(r session.Result) error {
		_, err := decodeDatasetPGAAllocated(r, p)
		return projection.MarkDecodeError(err)
	})
}

// PGAAllocatedSQL reads observed instance PGA allocation in rounded GiB, not its target.
const PGAAllocatedSQL = `SELECT TO_CHAR(ROUND(VALUE/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$PGASTAT WHERE NAME='total PGA allocated'`
