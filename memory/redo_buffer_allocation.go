package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RedoBufferAllocationOptions selects the observed numeric release and
// qualified reporting instances. An empty filter selects all visible members.
// Supply a non-CDB or CDB root connection for instance-wide observations.
type RedoBufferAllocationOptions struct {
	OracleMajor int
	Filter      InstanceFilter
}

// RedoBufferAllocationRequest supplies a caller-owned statement reference.
type RedoBufferAllocationRequest struct {
	Options   RedoBufferAllocationOptions
	Reference reference.Reference
}

// RedoBufferAllocationRow reports exact allocated bytes, preserving SQL NULL.
// The allocation is separate from the effective LOG_BUFFER parameter.
type RedoBufferAllocationRow struct {
	InstanceID session.Value
	Bytes      session.Value
}

// RedoBufferAllocationResult records reporting instances in source order.
// Complete means the selected query completed. It does not prove RAC coverage.
type RedoBufferAllocationResult struct {
	Provenance DatasetProvenance
	Rows       []RedoBufferAllocationRow
}

func redoBufferAllocationPlan(options RedoBufferAllocationOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	clause, args, err := sgaInstanceClause(options.Filter.Instances)
	if err != nil {
		return datasetSelection{}, err
	}
	text := "SELECT v.INST_ID AS INST_ID, v.VALUE AS BYTES FROM GV$SGA v WHERE v.NAME = 'Redo Buffers'"
	if options.OracleMajor >= 12 {
		// Redo buffer allocation is instance-wide, independent of a PDB.
		text += " AND v.CON_ID = 0"
	}
	if clause != "" {
		text += " AND " + clause
	}
	return datasetSelection{
		sql: text + " ORDER BY v.INST_ID", args: args,
		schema: []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "BYTES", Kind: rowset.Integer}},
		provenance: DatasetProvenance{
			Source: "GV$SGA", OracleMajor: options.OracleMajor, Scope: "instance_redo_buffer_allocation",
		},
	}, nil
}

// RedoBufferAllocationStatement returns the reviewed SQL and native bind arguments.
func RedoBufferAllocationStatement(options RedoBufferAllocationOptions) (string, []any, error) {
	plan, err := redoBufferAllocationPlan(options)
	return plan.sql, plan.args, err
}

// DecodeRedoBufferAllocation validates the whole rowset without asserting
// transport completion. It retains source duplicates for callers to interpret.
func DecodeRedoBufferAllocation(result session.Result, options RedoBufferAllocationOptions) (RedoBufferAllocationResult, error) {
	plan, err := redoBufferAllocationPlan(options)
	if err != nil {
		return RedoBufferAllocationResult{}, err
	}
	return decodeRedoBufferAllocation(result, plan, options.Filter.Instances)
}

func decodeRedoBufferAllocation(result session.Result, plan datasetSelection, admitted []int64) (RedoBufferAllocationResult, error) {
	columns, err := datasetColumns(result, plan, "INST_ID", "BYTES")
	if err != nil {
		return RedoBufferAllocationResult{}, ErrInvalidObservedEvidence
	}
	rows := make([]RedoBufferAllocationRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		member := row[columns["INST_ID"]]
		id, err := strconv.ParseInt(member.Text, 10, 64)
		if member.Null || err != nil || id <= 0 || len(admitted) != 0 && !slices.Contains(admitted, id) {
			return RedoBufferAllocationResult{}, ErrInvalidObservedEvidence
		}
		rows = append(rows, RedoBufferAllocationRow{InstanceID: member, Bytes: row[columns["BYTES"]]})
	}
	return RedoBufferAllocationResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadRedoBufferAllocation reads once through the caller's admitted reader and
// publishes observations only after a successful, uncancelled decode.
func ReadRedoBufferAllocation(ctx context.Context, reader Reader, request RedoBufferAllocationRequest) (RedoBufferAllocationResult, error) {
	plan, err := redoBufferAllocationPlan(request.Options)
	if err != nil {
		return RedoBufferAllocationResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return RedoBufferAllocationResult{}, ErrInvalidRequest
	}
	if err := datasetReadError(ctx, plan, nil); err != nil {
		return RedoBufferAllocationResult{}, err
	}
	raw, err := reader.Query(ctx, session.Statement{Text: plan.sql, Args: plan.args, Reference: request.Reference})
	if err := datasetReadError(ctx, plan, err); err != nil {
		return RedoBufferAllocationResult{}, err
	}
	result, decodeErr := decodeRedoBufferAllocation(raw, plan, request.Options.Filter.Instances)
	if err := datasetReadError(ctx, plan, decodeErr); err != nil {
		return RedoBufferAllocationResult{}, projection.MarkDecodeError(err, decodeErr)
	}
	result.Provenance.Complete = true
	return result, nil
}

// RedoBufferAllocationRows returns the actual native cursor. The caller owns
// iteration, Err, Close, affinity and finite budgets.
func RedoBufferAllocationRows(ctx context.Context, q rowset.Queryer, options RedoBufferAllocationOptions) (*sql.Rows, error) {
	plan, err := redoBufferAllocationPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &DatasetError{plan.provenance.Source, err}
	}
	return rows, nil
}

// RedoBufferAllocationJSON consumes its own cursor under the supplied limits.
// Query, validation, terminal or cancellation failures return no JSON prefix.
func RedoBufferAllocationJSON(ctx context.Context, q rowset.Queryer, options RedoBufferAllocationOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := redoBufferAllocationPlan(options)
	if err != nil {
		return nil, err
	}
	return readAllocationJSON(ctx, q, plan, limits, func(result session.Result) error {
		_, err := decodeRedoBufferAllocation(result, plan, options.Filter.Instances)
		return err
	})
}
