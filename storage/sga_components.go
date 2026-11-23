package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// The storage entry points preserve the published types, provenance and errors.
// Memory owns the SQL, validation and transport orchestration for this fact.

// SGAComponent preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// Buffer-cache rows and shared-pool allocations remain separate by reporting instance/container. These are observed component allocations, not configured target settings or a proven RAC total.
type SGAComponent struct {
	InstanceID  session.Value
	Pool        session.Value
	Name        session.Value
	Bytes       session.Value
	ContainerID *session.Value
}

// SGAComponentsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type SGAComponentsResult struct {
	Provenance Provenance
	Rows       []SGAComponent
}

// InstanceFilter selects reporting IDs; empty means all visible admitted rows.
type InstanceFilter struct{ Instances []int64 }

// SGARequest admits per-instance observed component facts, not configured targets.
type SGARequest struct {
	OracleMajor int
	Filter      InstanceFilter
	Reference   reference.Reference
}

// SGAComponentsStatement is the storage compatibility selector for memory.SGAComponentsStatement.
func SGAComponentsStatement(major int, filter InstanceFilter) (string, []any, error) {
	statement, args, err := memory.SGAComponentsStatement(major, memory.InstanceFilter(filter))
	return statement, args, storageSGAError(err)
}

// DecodeSGAComponents is the storage compatibility decoder for memory.DecodeSGAComponents.
func DecodeSGAComponents(result session.Result, major int, filter InstanceFilter) (SGAComponentsResult, error) {
	return storageSGAResult(memory.DecodeSGAComponents(result, major, memory.InstanceFilter(filter)))
}

// ReadSGAComponents preserves the storage buffered API for SGA component facts.
func ReadSGAComponents(ctx context.Context, reader Reader, request SGARequest) (SGAComponentsResult, error) {
	return storageSGAResult(memory.ReadSGAComponents(ctx, reader, memory.SGARequest{
		OracleMajor: request.OracleMajor,
		Filter:      memory.InstanceFilter(request.Filter),
		Reference:   request.Reference,
	}))
}

// SGAComponentsRows returns the actual native cursor from memory.SGAComponentsRows.
func SGAComponentsRows(ctx context.Context, q rowset.Queryer, major int, filter InstanceFilter) (*sql.Rows, error) {
	rows, err := memory.SGAComponentsRows(ctx, q, major, memory.InstanceFilter(filter))
	return rows, storageSGAError(err)
}

// SGAComponentsJSON preserves the storage JSON contract for SGA component facts.
func SGAComponentsJSON(ctx context.Context, q rowset.Queryer, major int, filter InstanceFilter, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := memory.SGAComponentsJSON(ctx, q, major, memory.InstanceFilter(filter), limits)
	return payload, storageSGAError(err)
}

func storageSGAResult(facts memory.SGAComponentsResult, err error) (SGAComponentsResult, error) {
	if err != nil {
		return SGAComponentsResult{}, storageSGAError(err)
	}
	rows := make([]SGAComponent, len(facts.Rows))
	for i, row := range facts.Rows {
		rows[i] = SGAComponent(row)
	}
	p := facts.Provenance
	return SGAComponentsResult{Provenance: Provenance{
		Source: p.Source, OracleMajor: p.OracleMajor, Scope: p.Scope,
		Instances: p.Instances, Complete: p.Complete,
	}, Rows: rows}, nil
}

func storageSGAError(err error) error {
	switch err {
	case memory.ErrUnsupportedVersion:
		return ErrUnsupportedVersion
	case memory.ErrInvalidRequest:
		return ErrInvalidRequest
	case memory.ErrInvalidObservedEvidence:
		return ErrInvalidEvidence
	}
	if failure, ok := err.(*memory.DatasetError); ok {
		cause := failure.Unwrap()
		if cause == memory.ErrInvalidObservedEvidence {
			// A pure decoder returns its validation error directly.
			cause = ErrInvalidEvidence
		} else {
			cause = projection.MapDecodeError(cause, storageSGAError)
		}
		return &Error{failure.Source, cause}
	}
	return err
}

// SGAComponentsOptions carries the source selectors for the options-based fact APIs.
type SGAComponentsOptions struct {
	OracleMajor int
	Filter      InstanceFilter
}

// SGAComponentsStatementWithOptions selects the same source as SGAComponentsStatement using one options value.
func SGAComponentsStatementWithOptions(options SGAComponentsOptions) (string, []any, error) {
	return SGAComponentsStatement(options.OracleMajor, options.Filter)
}

// DecodeSGAComponentsWithOptions selects the same source as DecodeSGAComponents using one options value.
func DecodeSGAComponentsWithOptions(result session.Result, options SGAComponentsOptions) (SGAComponentsResult, error) {
	return DecodeSGAComponents(result, options.OracleMajor, options.Filter)
}

// SGAComponentsRowsWithOptions selects the same source as SGAComponentsRows using one options value.
func SGAComponentsRowsWithOptions(ctx context.Context, q rowset.Queryer, options SGAComponentsOptions) (*sql.Rows, error) {
	return SGAComponentsRows(ctx, q, options.OracleMajor, options.Filter)
}

// SGAComponentsJSONWithOptions selects the same source as SGAComponentsJSON using one options value.
func SGAComponentsJSONWithOptions(ctx context.Context, q rowset.Queryer, options SGAComponentsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return SGAComponentsJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}
