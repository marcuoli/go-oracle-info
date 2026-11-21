package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PDBOpenModeRow preserves source values and SQL NULL without application policy.
type PDBOpenModeRow struct{ OpenMode session.Value }

// PDBOpenModeOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type PDBOpenModeOptions struct{ OracleMajor int }

// PDBOpenModeRequest supplies admitted options and a caller-owned SQL reference.
type PDBOpenModeRequest struct {
	Options   PDBOpenModeOptions
	Reference reference.Reference
}

// PDBOpenModeResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type PDBOpenModeResult struct {
	Provenance DatasetProvenance
	Rows       []PDBOpenModeRow
}

// PDBOpenModeStatement selects reviewed SQL with bound values, owning no connection.
func PDBOpenModeStatement(options PDBOpenModeOptions) (string, []any, error) {
	p, err := pDBOpenModeDatasetPlan(options)
	return p.sql, p.args, err
}

func pDBOpenModeDatasetPlan(options PDBOpenModeOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := "SELECT OPEN_MODE AS OPEN_MODE FROM V$PDBS WHERE CON_ID=TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))"
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "OPEN_MODE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PDBS+SYS_CONTEXT(USERENV.CON_ID)", OracleMajor: options.OracleMajor, Scope: "current_session_container_open_mode_lookup"}}, nil
}

// DecodePDBOpenMode validates the entire shape and every row before returning facts.
func DecodePDBOpenMode(result session.Result, options PDBOpenModeOptions) (PDBOpenModeResult, error) {
	p, err := pDBOpenModeDatasetPlan(options)
	if err != nil {
		return PDBOpenModeResult{}, err
	}
	return decodeDatasetPDBOpenMode(result, p)
}

func decodeDatasetPDBOpenMode(result session.Result, p datasetSelection) (PDBOpenModeResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return PDBOpenModeResult{}, err
	}
	if len(result.Rows) > 1 {
		return PDBOpenModeResult{}, ErrInvalidEvidence
	}
	rows := make([]PDBOpenModeRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = PDBOpenModeRow{OpenMode: r[index["OPEN_MODE"]]}
	}
	return PDBOpenModeResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadPDBOpenMode executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadPDBOpenMode(ctx context.Context, reader Reader, request PDBOpenModeRequest) (PDBOpenModeResult, error) {
	p, err := pDBOpenModeDatasetPlan(request.Options)
	if err != nil {
		return PDBOpenModeResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return PDBOpenModeResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return PDBOpenModeResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return PDBOpenModeResult{}, err
	}
	facts, err := decodeDatasetPDBOpenMode(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return PDBOpenModeResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PDBOpenModeRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func PDBOpenModeRows(ctx context.Context, q rowset.Queryer, options PDBOpenModeOptions) (*sql.Rows, error) {
	p, err := pDBOpenModeDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// PDBOpenModeJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func PDBOpenModeJSON(ctx context.Context, q rowset.Queryer, options PDBOpenModeOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := pDBOpenModeDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetPDBOpenMode(r, p); return err })
}

// PDBOpenModeSQL reads the connected PDB's open mode on Oracle 12c+.
// Empty or inaccessible results do not inherit the root's open mode.
const PDBOpenModeSQL = `SELECT OPEN_MODE FROM V$PDBS WHERE CON_ID=TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))`
