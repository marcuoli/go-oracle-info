package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
)

// ConnectedCells selects a bounded V$CELL projection for the admitted connected
// instance, without metadata discovery or switching to GV$ views. Callers may
// omit CELL_TYPE only after establishing that the source lacks that column.
// Reserve a sentinel row in maxRows when truncation must be detected; a full
// returned limit must not be assumed to describe a complete inventory.
func ConnectedCells(maxRows int, includeType bool) (Command, error) {
	if maxRows < 1 || maxRows > 10000 {
		return Command{}, errors.New("row limit must be between 1 and 10000")
	}
	original := connectedCellsSQL
	if !includeType {
		original = connectedCellsLegacySQL
	}
	text, err := sqltemplate.Render("connected-cells", original,
		struct{ Limit sqltemplate.Fragment }{sqltemplate.Fragment(strconv.Itoa(maxRows))})
	return Command{ID: "connected-cells", Text: text, View: "V$CELL", Original: original}, err
}

// Keep aliases on inner and outer projections to preserve named-column contracts.
const connectedCellsSQL = `SELECT TO_CHAR(CELL_HASHVAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CELL_HASHVAL, CELL_PATH AS CELL_PATH, CELL_TYPE AS CELL_TYPE FROM (SELECT CELL_HASHVAL AS CELL_HASHVAL, CELL_PATH AS CELL_PATH, CELL_TYPE AS CELL_TYPE FROM V$CELL ORDER BY CELL_HASHVAL,CELL_PATH) WHERE ROWNUM<={{.Limit}}`

const connectedCellsLegacySQL = `SELECT TO_CHAR(CELL_HASHVAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CELL_HASHVAL, CELL_PATH AS CELL_PATH FROM (SELECT CELL_HASHVAL AS CELL_HASHVAL, CELL_PATH AS CELL_PATH FROM V$CELL ORDER BY CELL_HASHVAL,CELL_PATH) WHERE ROWNUM<={{.Limit}}`

// ConnectedCellRow retains exact raw facts. A nil optional pointer means its source
// column/layout was not admitted; a nonnil Null value is selected SQL NULL.
// XML remains raw evidence, not acceptance, normalization or health policy.
type ConnectedCellRow struct {
	Hash session.Value
	Path session.Value
	Type *session.Value
}

// ConnectedCellsOptions supplies an observed numeric major and admitted source capability.
// The caller owns dictionary visibility, state, container and reporter coverage.
// No metadata query or recovery is performed by the selector.
type ConnectedCellsOptions struct {
	OracleMajor         int
	Limit               int
	IncludeType         bool
	CellTypeKnownAbsent bool
}

// ConnectedCellsRequest supplies caller admission and a stable caller-owned reference.
type ConnectedCellsRequest struct {
	Options   ConnectedCellsOptions
	Reference reference.Reference
}

// ConnectedCellsResult preserves row order/duplicates and selected-transport provenance.
// A completed bounded sample is not a complete source or physical inventory.
type ConnectedCellsResult struct {
	Provenance DatasetProvenance
	Rows       []ConnectedCellRow
}

// ConnectedCellsStatement delegates to the reviewed fixed capability-selected catalog.
func ConnectedCellsStatement(options ConnectedCellsOptions) (string, []any, error) {
	p, err := connectedCellsDatasetPlan(options)
	return p.sql, p.args, err
}

func connectedCellsDatasetPlan(options ConnectedCellsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if options.IncludeType == options.CellTypeKnownAbsent {
		return datasetSelection{}, ErrInvalidRequest
	}
	command, err := ConnectedCells(options.Limit, options.IncludeType)
	if err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	schema := []rowset.Column{{Name: "CELL_HASHVAL", Kind: rowset.Text}, {Name: "CELL_PATH", Kind: rowset.Text}}
	var optional []string
	if options.IncludeType {
		schema = append(schema, rowset.Column{Name: "CELL_TYPE", Kind: rowset.Text})
		optional = []string{"CELL_TYPE"}
	}
	return datasetSelection{sql: command.Text, schema: schema, provenance: DatasetProvenance{Source: command.View, OracleMajor: options.OracleMajor, Scope: "connected_instance_bounded_cell_sample", Limit: options.Limit, OptionalColumns: optional}}, nil
}

// DecodeConnectedCells validates all selected rows before returning any source facts.
func DecodeConnectedCells(result session.Result, options ConnectedCellsOptions) (ConnectedCellsResult, error) {
	p, err := connectedCellsDatasetPlan(options)
	if err != nil {
		return ConnectedCellsResult{}, err
	}
	return decodeDatasetConnectedCells(result, p)
}

func decodeDatasetConnectedCells(result session.Result, p datasetSelection) (ConnectedCellsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return ConnectedCellsResult{}, err
	}
	if len(result.Rows) > p.provenance.Limit {
		return ConnectedCellsResult{}, ErrInvalidEvidence
	}
	rows := make([]ConnectedCellRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ConnectedCellRow{Hash: r[index["CELL_HASHVAL"]], Path: r[index["CELL_PATH"]], Type: optionalDatasetValue(r, index, "CELL_TYPE", true)}
	}
	return ConnectedCellsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadConnectedCells reads once through a complete finite-budget reader. Its frozen
// selection is shared by decoding even if caller capability maps later change.
func ReadConnectedCells(ctx context.Context, reader Reader, request ConnectedCellsRequest) (ConnectedCellsResult, error) {
	p, err := connectedCellsDatasetPlan(request.Options)
	if err != nil {
		return ConnectedCellsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ConnectedCellsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ConnectedCellsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedCellsResult{}, err
	}
	facts, err := decodeDatasetConnectedCells(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedCellsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ConnectedCellsRows returns the actual native cursor. Caller owns Next/Scan/Err/Close,
// affinity, scope, complete LOB/length checks, cancellation and finite budgets.
func ConnectedCellsRows(ctx context.Context, q rowset.Queryer, options ConnectedCellsOptions) (*sql.Rows, error) {
	p, err := connectedCellsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// ConnectedCellsJSON validates its independent complete cursor before encoding any bytes.
// Source/layout absence, SQL NULL, native numeric kinds and ordering are retained.
func ConnectedCellsJSON(ctx context.Context, q rowset.Queryer, options ConnectedCellsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := connectedCellsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetConnectedCells(r, p); return err })
}
