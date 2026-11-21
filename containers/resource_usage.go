package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ResourceUsageRequest adds the caller-owned reference for GV$RSRC_PDB.
type ResourceUsageRequest struct {
	Options   ResourceOptions
	Reference reference.Reference
}

// ResourceUsageRow preserves one observed member memory gauge.
// The SGA and PGA byte counts are exact decimal text or SQL NULL.
type ResourceUsageRow struct {
	InstanceID        session.Value
	ContainerID       session.Value
	SourceContainerID session.Value
	SGABytes          session.Value
	PGABytes          session.Value
}

// ResourceUsageResult retains source order and completeness evidence.
type ResourceUsageResult struct {
	Provenance Provenance
	Rows       []ResourceUsageRow
}

// ResourceUsageStatement selects admitted member observations.
func ResourceUsageStatement(options ResourceOptions) (string, error) {
	p, err := resourceUsagePlan(options)
	return p.sql, err
}

func resourceUsagePlan(options ResourceOptions) (selection, error) {
	fields := []field{
		exact("SGA_BYTES"),
		exact("PGA_BYTES"),
	}
	return resourceSelection(options, "GV$RSRC_PDB", fields, "v.CON_ID, v.INST_ID")
}

// DecodeResourceUsage validates the complete projection before returning any rows.
func DecodeResourceUsage(result session.Result, options ResourceOptions) (ResourceUsageResult, error) {
	p, err := resourceUsagePlan(options)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	return decodeResourceUsage(result, p)
}

func decodeResourceUsage(result session.Result, p selection) (ResourceUsageResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	rows := make([]ResourceUsageRow, len(result.Rows))
	for i, row := range result.Rows {
		v := ResourceUsageRow{
			InstanceID:        row[index["INST_ID"]],
			ContainerID:       row[index["CON_ID"]],
			SourceContainerID: row[index["SOURCE_CON_ID"]],
			SGABytes:          row[index["SGA_BYTES"]],
			PGABytes:          row[index["PGA_BYTES"]],
		}
		if !validMember(v.InstanceID, v.ContainerID, v.SourceContainerID, p.provenance.Scope) ||
			!unsigned(v.SGABytes) ||
			!unsigned(v.PGABytes) {
			return ResourceUsageResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return ResourceUsageResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadResourceUsage reads an independent complete buffered observation.
func ReadResourceUsage(ctx context.Context, reader Reader, request ResourceUsageRequest) (ResourceUsageResult, error) {
	p, err := resourceUsagePlan(request.Options)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	facts, err := decodeResourceUsage(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return ResourceUsageResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ResourceUsageRows returns the actual cursor, with its lifetime owned by the caller.
func ResourceUsageRows(ctx context.Context, q rowset.Queryer, options ResourceOptions) (*sql.Rows, error) {
	p, err := resourceUsagePlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// ResourceUsageJSON returns one complete bounded native observation.
func ResourceUsageJSON(ctx context.Context, q rowset.Queryer, options ResourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := resourceUsagePlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeResourceUsage(result, p)
		return err
	})
}
