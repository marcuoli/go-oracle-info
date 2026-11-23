package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/memory"
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

// PGAAllocatedStatement preserves the database API for memory.PGAAllocatedStatement.
func PGAAllocatedStatement(options PGAAllocatedOptions) (string, []any, error) {
	statement, args, err := memory.PGAAllocatedStatement(memory.PGAAllocatedOptions(options))
	return statement, args, legacyMemoryDatasetError(err)
}

// DecodePGAAllocated preserves the database API for memory.DecodePGAAllocated.
func DecodePGAAllocated(result session.Result, options PGAAllocatedOptions) (PGAAllocatedResult, error) {
	return legacyPGAAllocatedResult(memory.DecodePGAAllocated(result, memory.PGAAllocatedOptions(options)))
}

// ReadPGAAllocated preserves the database API for memory.ReadPGAAllocated.
func ReadPGAAllocated(ctx context.Context, reader Reader, request PGAAllocatedRequest) (PGAAllocatedResult, error) {
	return legacyPGAAllocatedResult(memory.ReadPGAAllocated(ctx, reader, memory.PGAAllocatedRequest{
		Options: memory.PGAAllocatedOptions(request.Options), Reference: request.Reference,
	}))
}

// PGAAllocatedRows returns the native cursor from memory.PGAAllocatedRows.
func PGAAllocatedRows(ctx context.Context, q rowset.Queryer, options PGAAllocatedOptions) (*sql.Rows, error) {
	rows, err := memory.PGAAllocatedRows(ctx, q, memory.PGAAllocatedOptions(options))
	return rows, legacyMemoryDatasetError(err)
}

// PGAAllocatedJSON preserves the database API for memory.PGAAllocatedJSON.
func PGAAllocatedJSON(ctx context.Context, q rowset.Queryer, options PGAAllocatedOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := memory.PGAAllocatedJSON(ctx, q, memory.PGAAllocatedOptions(options), limits)
	return payload, legacyMemoryDatasetError(err)
}

func legacyPGAAllocatedResult(facts memory.PGAAllocatedResult, err error) (PGAAllocatedResult, error) {
	if err != nil {
		return PGAAllocatedResult{}, legacyMemoryDatasetError(err)
	}
	rows := make([]PGAAllocatedRow, len(facts.Rows))
	for i, row := range facts.Rows {
		rows[i] = PGAAllocatedRow(row)
	}
	p := facts.Provenance
	return PGAAllocatedResult{Provenance: DatasetProvenance{
		Source: p.Source, OracleMajor: p.OracleMajor, Scope: p.Scope, Complete: p.Complete,
	}, Rows: rows}, nil
}

// PGAAllocatedSQL reads observed instance PGA allocation in rounded GiB, not its target.
const PGAAllocatedSQL = memory.PGAAllocatedSQL
