package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
)

func Cells(schema Schema) (Command, bool, error) {
	view, columns := choose(schema, "CELL")
	if view == "" {
		return Command{}, false, nil
	}
	if !instanceValid(view, columns) || columns["CELL_HASHVAL"] != "NUMBER" || !textType(columns["CELL_PATH"]) {
		if local := localFallback(schema, view); local != nil {
			return Cells(local)
		}
		return Command{View: "SYS." + view}, false, nil
	}
	f := fragments{Instance: instance(view), View: sqltemplate.Fragment("SYS." + view), CellType: sqltemplate.Fragment("CAST(NULL AS VARCHAR2(32))")}
	if textType(columns["CELL_TYPE"]) {
		f.CellType = "C.CELL_TYPE"
	}
	text, err := sqltemplate.Render("engineered-cells", CellsTemplate, f)
	return Command{ID: "engineered-cells", Text: text, View: "SYS." + view, Original: CellsTemplate}, true, err
}

// CellRow retains exact raw facts. A nil optional pointer means its source
// column/layout was not admitted; a nonnil Null value is selected SQL NULL.
// XML remains raw evidence, not acceptance, normalization or health policy.
type CellRow struct {
	InstanceID session.Value
	Hash       session.Value
	Path       session.Value
	Type       *session.Value
}

// CellsOptions supplies an observed numeric major and admitted source capability.
// The caller owns dictionary visibility, state, container and reporter coverage.
// No metadata query or recovery is performed by the selector.
type CellsOptions struct {
	OracleMajor int
	Schema      Schema
}

// CellsRequest supplies caller admission and a stable caller-owned reference.
type CellsRequest struct {
	Options   CellsOptions
	Reference reference.Reference
}

// CellsResult preserves row order/duplicates and selected-transport provenance.
// A completed bounded sample is not a complete source or physical inventory.
type CellsResult struct {
	Provenance DatasetProvenance
	Rows       []CellRow
}

// CellsStatement delegates to the reviewed fixed capability-selected catalog.
func CellsStatement(options CellsOptions) (string, []any, error) {
	p, err := cellsDatasetPlan(options)
	return p.sql, p.args, err
}

func cellsDatasetPlan(options CellsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if err := ValidateSchema(options.Schema); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	command, ok, err := Cells(options.Schema)
	if err != nil {
		return datasetSelection{}, err
	}
	if !ok {
		return datasetSelection{}, ErrUnsupportedSource
	}
	schema := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CELL_HASHVAL", Kind: rowset.Text}, {Name: "CELL_PATH", Kind: rowset.Text}, {Name: "CELL_TYPE", Kind: rowset.Text}}
	var optional []string
	if textType(options.Schema[strings.TrimPrefix(command.View, "SYS.")]["CELL_TYPE"]) {
		optional = []string{"CELL_TYPE"}
	}
	return datasetSelection{sql: command.Text, schema: schema, provenance: DatasetProvenance{Source: command.View, OracleMajor: options.OracleMajor, Scope: "admitted_visible_reporter_cell_rows", OptionalColumns: optional}}, nil
}

// DecodeCells validates all selected rows before returning any source facts.
func DecodeCells(result session.Result, options CellsOptions) (CellsResult, error) {
	p, err := cellsDatasetPlan(options)
	if err != nil {
		return CellsResult{}, err
	}
	return decodeDatasetCells(result, p)
}

func decodeDatasetCells(result session.Result, p datasetSelection) (CellsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return CellsResult{}, err
	}
	for _, r := range result.Rows {
		if !validDatasetInstance(r[index["INST_ID"]]) {
			return CellsResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]CellRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = CellRow{InstanceID: r[index["INST_ID"]], Hash: r[index["CELL_HASHVAL"]], Path: r[index["CELL_PATH"]], Type: optionalDatasetValue(r, index, "CELL_TYPE", slices.Contains(p.provenance.OptionalColumns, "CELL_TYPE"))}
	}
	return CellsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCells reads once through a complete finite-budget reader. Its frozen
// selection is shared by decoding even if caller capability maps later change.
func ReadCells(ctx context.Context, reader Reader, request CellsRequest) (CellsResult, error) {
	p, err := cellsDatasetPlan(request.Options)
	if err != nil {
		return CellsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return CellsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return CellsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return CellsResult{}, err
	}
	facts, err := decodeDatasetCells(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return CellsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// CellsRows returns the actual native cursor. Caller owns Next/Scan/Err/Close,
// affinity, scope, complete LOB/length checks, cancellation and finite budgets.
func CellsRows(ctx context.Context, q rowset.Queryer, options CellsOptions) (*sql.Rows, error) {
	p, err := cellsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// CellsJSON validates its independent complete cursor before encoding any bytes.
// Source/layout absence, SQL NULL, native numeric kinds and ordering are retained.
func CellsJSON(ctx context.Context, q rowset.Queryer, options CellsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := cellsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetCells(r, p); return err })
}

const CellsTemplate = `SELECT {{.Instance}} AS INST_ID,
       TO_CHAR(C.CELL_HASHVAL, 'TM9') AS CELL_HASHVAL,
       C.CELL_PATH AS CELL_PATH,
       {{.CellType}} AS CELL_TYPE
FROM {{.View}} C`
