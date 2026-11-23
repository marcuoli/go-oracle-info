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

// SGAAllocatedRow preserves source values and SQL NULL without application policy.
type SGAAllocatedRow struct{ GiB session.Value }

// SGAAllocatedOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SGAAllocatedOptions struct{ OracleMajor int }

// SGAAllocatedRequest supplies admitted options and a caller-owned SQL reference.
type SGAAllocatedRequest struct {
	Options   SGAAllocatedOptions
	Reference reference.Reference
}

// SGAAllocatedResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SGAAllocatedResult struct {
	Provenance DatasetProvenance
	Rows       []SGAAllocatedRow
}

// SGAAllocatedStatement preserves the database API for memory.SGAAllocatedStatement.
func SGAAllocatedStatement(options SGAAllocatedOptions) (string, []any, error) {
	statement, args, err := memory.SGAAllocatedStatement(memory.SGAAllocatedOptions(options))
	return statement, args, legacyMemoryDatasetError(err)
}

// DecodeSGAAllocated preserves the database API for memory.DecodeSGAAllocated.
func DecodeSGAAllocated(result session.Result, options SGAAllocatedOptions) (SGAAllocatedResult, error) {
	return legacySGAAllocatedResult(memory.DecodeSGAAllocated(result, memory.SGAAllocatedOptions(options)))
}

// ReadSGAAllocated preserves the database API for memory.ReadSGAAllocated.
func ReadSGAAllocated(ctx context.Context, reader Reader, request SGAAllocatedRequest) (SGAAllocatedResult, error) {
	return legacySGAAllocatedResult(memory.ReadSGAAllocated(ctx, reader, memory.SGAAllocatedRequest{
		Options: memory.SGAAllocatedOptions(request.Options), Reference: request.Reference,
	}))
}

// SGAAllocatedRows returns the native cursor from memory.SGAAllocatedRows.
func SGAAllocatedRows(ctx context.Context, q rowset.Queryer, options SGAAllocatedOptions) (*sql.Rows, error) {
	rows, err := memory.SGAAllocatedRows(ctx, q, memory.SGAAllocatedOptions(options))
	return rows, legacyMemoryDatasetError(err)
}

// SGAAllocatedJSON preserves the database API for memory.SGAAllocatedJSON.
func SGAAllocatedJSON(ctx context.Context, q rowset.Queryer, options SGAAllocatedOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := memory.SGAAllocatedJSON(ctx, q, memory.SGAAllocatedOptions(options), limits)
	return payload, legacyMemoryDatasetError(err)
}

func legacySGAAllocatedResult(facts memory.SGAAllocatedResult, err error) (SGAAllocatedResult, error) {
	if err != nil {
		return SGAAllocatedResult{}, legacyMemoryDatasetError(err)
	}
	rows := make([]SGAAllocatedRow, len(facts.Rows))
	for i, row := range facts.Rows {
		rows[i] = SGAAllocatedRow(row)
	}
	p := facts.Provenance
	return SGAAllocatedResult{Provenance: DatasetProvenance{
		Source: p.Source, OracleMajor: p.OracleMajor, Scope: p.Scope, Complete: p.Complete,
	}, Rows: rows}, nil
}

// SGAAllocatedSQL reads observed instance SGA allocation in rounded GiB, not configured memory.
const SGAAllocatedSQL = memory.SGAAllocatedSQL
