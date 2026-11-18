package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PDBCountRow preserves source values and SQL NULL without application policy.
type PDBCountRow struct{ Count session.Value }

// PDBCountOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type PDBCountOptions struct {
	OracleMajor int
	Root        bool
}

// PDBCountRequest supplies admitted options and a caller-owned SQL reference.
type PDBCountRequest struct {
	Options   PDBCountOptions
	Reference reference.Reference
}

// PDBCountResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type PDBCountResult struct {
	Provenance DatasetProvenance
	Rows       []PDBCountRow
}

// PDBCountStatement selects reviewed SQL with bound values, owning no connection.
func PDBCountStatement(options PDBCountOptions) (string, []any, error) {
	p, err := pDBCountDatasetPlan(options)
	return p.sql, p.args, err
}
func pDBCountDatasetPlan(options PDBCountOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if !options.Root {
		return datasetSelection{}, ErrInvalidRequest
	}
	query := PDBCountSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PDBS", OracleMajor: options.OracleMajor, Scope: "root_visible_nonseed_pdb_count_not_complete_membership"}}, nil
}

// DecodePDBCount validates the entire shape and every row before returning facts.
func DecodePDBCount(result session.Result, options PDBCountOptions) (PDBCountResult, error) {
	p, err := pDBCountDatasetPlan(options)
	if err != nil {
		return PDBCountResult{}, err
	}
	return decodeDatasetPDBCount(result, p)
}
func decodeDatasetPDBCount(result session.Result, p datasetSelection) (PDBCountResult, error) {
	index, err := datasetColumns(result, p, "VALUE")
	if err != nil {
		return PDBCountResult{}, err
	}
	if len(result.Rows) != 1 {
		return PDBCountResult{}, ErrInvalidEvidence
	}
	rows := make([]PDBCountRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = PDBCountRow{Count: r[index["VALUE"]]}
	}
	return PDBCountResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadPDBCount executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadPDBCount(ctx context.Context, reader Reader, request PDBCountRequest) (PDBCountResult, error) {
	p, err := pDBCountDatasetPlan(request.Options)
	if err != nil {
		return PDBCountResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return PDBCountResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return PDBCountResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return PDBCountResult{}, err
	}
	facts, err := decodeDatasetPDBCount(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return PDBCountResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PDBCountRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func PDBCountRows(ctx context.Context, q rowset.Queryer, options PDBCountOptions) (*sql.Rows, error) {
	p, err := pDBCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// PDBCountJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func PDBCountJSON(ctx context.Context, q rowset.Queryer, options PDBCountOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := pDBCountDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetPDBCount(r, p); return err })
}
