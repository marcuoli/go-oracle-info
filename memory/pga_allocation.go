package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PGAAllocationOptions carries the observed numeric Oracle release.
type PGAAllocationOptions struct{ OracleVersion string }

// PGAAllocationRequest supplies a caller-owned SQL reference and buffered bounds.
type PGAAllocationRequest struct {
	Options   PGAAllocationOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// PGAAllocationRow preserves exact byte text or SQL NULL and the raw unit.
// ContainerID is the statistic's CON_ID. SessionContainerID, InstanceID and
// InstanceName identify the executing session in the same Oracle observation.
type PGAAllocationRow struct {
	Bytes              session.Value
	Unit               session.Value
	ContainerID        session.Value
	SessionContainerID session.Value
	InstanceID         session.Value
	InstanceName       session.Value
}

// PGAAllocationResult contains a complete visible observation when Read succeeds.
// One row establishes executing-instance scope. It supplies no fleet total.
type PGAAllocationResult struct {
	Provenance DatasetProvenance
	Rows       []PGAAllocationRow
}

func pgaAllocationPlan(options PGAAllocationOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 12 {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	// Project the raw statistic scope so an unexpected container row or
	// duplicate cannot silently become instance-wide allocation.
	const query = "SELECT p.VALUE AS BYTES, p.UNIT AS UNIT, p.CON_ID AS CON_ID, " +
		"SYS_CONTEXT('USERENV','CON_ID') AS SESSION_CON_ID, SYS_CONTEXT('USERENV','INSTANCE') AS INST_ID, " +
		"SYS_CONTEXT('USERENV','INSTANCE_NAME',4000) AS INSTANCE_NAME FROM V$PGASTAT p WHERE p.NAME = 'total PGA allocated'"
	return datasetSelection{
		sql: query,
		schema: []rowset.Column{
			{Name: "BYTES", Kind: rowset.Integer},
			{Name: "UNIT", Kind: rowset.Text},
			{Name: "CON_ID", Kind: rowset.Integer},
			{Name: "SESSION_CON_ID", Kind: rowset.Integer},
			{Name: "INST_ID", Kind: rowset.Integer},
			{Name: "INSTANCE_NAME", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "V$PGASTAT", OracleMajor: major, Scope: "executing_instance_pga_allocation"},
	}, nil
}

// PGAAllocationStatement selects current allocated PGA on the executing instance.
func PGAAllocationStatement(options PGAAllocationOptions) (string, []any, error) {
	plan, err := pgaAllocationPlan(options)
	return plan.sql, plan.args, err
}

// DecodePGAAllocation validates byte units, instance scope and session identity.
func DecodePGAAllocation(result session.Result, options PGAAllocationOptions) (PGAAllocationResult, error) {
	plan, err := pgaAllocationPlan(options)
	if err != nil {
		return PGAAllocationResult{}, err
	}
	return decodePGAAllocation(result, plan)
}

func decodePGAAllocation(result session.Result, plan datasetSelection) (PGAAllocationResult, error) {
	column, err := datasetColumns(result, plan, "BYTES", "CON_ID", "SESSION_CON_ID", "INST_ID")
	if err != nil || len(result.Rows) > 1 {
		return PGAAllocationResult{}, ErrInvalidObservedEvidence
	}
	rows := make([]PGAAllocationRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := PGAAllocationRow{
			Bytes: row[column["BYTES"]], Unit: row[column["UNIT"]],
			ContainerID: row[column["CON_ID"]], SessionContainerID: row[column["SESSION_CON_ID"]],
			InstanceID: row[column["INST_ID"]], InstanceName: row[column["INSTANCE_NAME"]],
		}
		source, sourceErr := strconv.ParseUint(fact.ContainerID.Text, 10, 64)
		_, sessionErr := strconv.ParseUint(fact.SessionContainerID.Text, 10, 64)
		instance, instanceErr := strconv.ParseUint(fact.InstanceID.Text, 10, 64)
		if fact.Unit.Null || !strings.EqualFold(fact.Unit.Text, "bytes") ||
			fact.ContainerID.Null || sourceErr != nil || source != 0 ||
			fact.SessionContainerID.Null || sessionErr != nil ||
			fact.InstanceID.Null || instanceErr != nil || instance == 0 ||
			fact.InstanceName.Null || strings.TrimSpace(fact.InstanceName.Text) == "" {
			return PGAAllocationResult{}, ErrInvalidObservedEvidence
		}
		rows = append(rows, fact)
	}
	return PGAAllocationResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadPGAAllocation reads once through the caller's admitted session and
// completes validation before returning its instance and container evidence.
func ReadPGAAllocation(ctx context.Context, reader Reader, request PGAAllocationRequest) (PGAAllocationResult, error) {
	plan, err := pgaAllocationPlan(request.Options)
	if err != nil {
		return PGAAllocationResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return PGAAllocationResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return PGAAllocationResult{}, err
	}
	if err = datasetReadError(ctx, plan, nil); err != nil {
		return PGAAllocationResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: plan.sql, Args: plan.args, Reference: request.Reference})
	if err = datasetReadError(ctx, plan, err); err != nil {
		return PGAAllocationResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return PGAAllocationResult{}, datasetReadError(ctx, plan, rowset.ErrCapacity)
	}
	facts, decodeErr := decodePGAAllocation(result, plan)
	if err = datasetReadError(ctx, plan, decodeErr); err != nil {
		return PGAAllocationResult{}, projection.MarkDecodeError(err, decodeErr)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PGAAllocationRows transfers the actual caller-native cursor.
func PGAAllocationRows(ctx context.Context, q rowset.Queryer, options PGAAllocationOptions) (*sql.Rows, error) {
	plan, err := pgaAllocationPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &DatasetError{plan.provenance.Source, err}
	}
	return rows, nil
}

// PGAAllocationJSON validates and consumes a separate complete bounded cursor.
func PGAAllocationJSON(ctx context.Context, q rowset.Queryer, options PGAAllocationOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := pgaAllocationPlan(options)
	if err != nil {
		return nil, err
	}
	return readAllocationJSON(ctx, q, plan, limits, func(result session.Result) error {
		_, err := decodePGAAllocation(result, plan)
		return err
	})
}
