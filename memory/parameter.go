package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Collect performs the selected read and, only when eligible, its effective ADB
// source fallback. It neither reconnects nor retries a failed statement. A primary
// observation remains returned if a later fallback fails, with that failure intact.
func Collect(ctx context.Context, reader Reader, request Request) (Observation, error) {
	if reader == nil {
		return Observation{}, errors.New("configured-memory reader is required")
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	o := request.Options
	if o.PDB.EffectiveADB {
		return Observation{}, ErrUnsupportedScope
	}
	statement, err := Statement(request.Parameter, o)
	if err != nil {
		return Observation{}, err
	}
	eligible := o.Scope == ConnectedPDB && o.OracleMajor >= 18
	if request.PrimaryReference.Namespace() == "" || (eligible && (request.EffectiveReference.Namespace() == "" || request.PrimaryReference == request.EffectiveReference)) {
		return Observation{}, errors.New("configured-memory execution references are missing or not distinct")
	}
	var args []any
	if o.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", o.TargetContainerID)}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.PrimaryReference, Text: statement, Args: args})
	if err != nil && !onlyNoRows(err) {
		return Observation{}, &ReadError{Parameter: request.Parameter, cause: err}
	}
	observation := Observation{State: NoRows}
	if err == nil {
		observation, err = Decode(result, request.Parameter, o)
	}
	if onlyNoRows(err) {
		err = nil
	}
	if err != nil {
		return Observation{}, err
	}
	// A complete read can race cancellation. Keep validated primary evidence,
	// but never report it as a successful collection after cancellation.
	if err := ctx.Err(); err != nil {
		return observation, err
	}
	if !eligible || (observation.State != Null && observation.State != NoRows) {
		return observation, nil
	}
	o.PDB.EffectiveADB = true
	statement, err = Statement(request.Parameter, o)
	if err != nil {
		return observation, err
	}
	result, err = reader.Query(ctx, session.Statement{Reference: request.EffectiveReference, Text: statement})
	missing := onlyNoRows(err)
	if missing {
		err = nil
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return observation, &ReadError{Parameter: request.Parameter, Effective: true, cause: err}
	}
	if missing {
		return observation, nil
	}
	effective, err := Decode(result, request.Parameter, o)
	if cancelled := ctx.Err(); cancelled != nil {
		return observation, errors.Join(err, cancelled)
	}
	if err != nil {
		return observation, err
	}
	// An ineligible/empty effective source must not erase a primary NULL's
	// validated provenance. A NULL effective row can enrich a primary no-row read.
	if effective.State == Available || (observation.State == NoRows && effective.State == Null) {
		return effective, nil
	}
	return observation, nil
}

const (
	NonCDB        Scope = "non_cdb"
	CDBRoot       Scope = "cdb_root"
	ConnectedPDB  Scope = "connected_pdb"
	DiscoveredPDB Scope = "discovered_pdb"
)

// ErrUnsupportedScope rejects unknown releases and unadmitted container shapes.
var ErrUnsupportedScope = errors.New("unsupported configured-memory scope")

// Statement returns unannotated SQL for an admitted configured-memory scope.
// Root-discovered statements bind target_container_id; data is never interpolated.
func Statement(parameter Parameter, o Options) (string, error) {
	if err := validateOptions(o); err != nil {
		return "", err
	}
	if o.Scope == ConnectedPDB {
		return PDBStatement(parameter, o.PDB)
	}
	if o.Scope == DiscoveredPDB {
		return pdbStatement(parameter, o.PDB, ":target_container_id", "SYS.GV_$SYSTEM_PARAMETER")
	}
	name := "pga_aggregate_target"
	switch parameter {
	case SGA:
		name = "sga_max_size"
	case PGA:
	default:
		return "", ErrUnsupportedParameter
	}
	current := "0"
	source := "0 AS SOURCE_CONTAINER_ID"
	partition, priority := "S.NAME", "S.INST_ID"
	predicate := "S.INST_ID=TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AND S.NAME='" + name + "'"
	if o.OracleMajor >= 12 {
		current = "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))"
		source = "S.CON_ID AS SOURCE_CONTAINER_ID"
		partition, priority = "S.NAME,S.CON_ID", "S.CON_ID"
		predicate += " AND S.CON_ID IN (0,1)"
	}
	inner := "SELECT S.VALUE AS VALUE, S.NAME AS PARAMETER_NAME, " + source + ",\n" +
		"COUNT(*) OVER (PARTITION BY " + partition + ") AS SOURCE_ROWS,\n" +
		"DENSE_RANK() OVER (ORDER BY " + priority + ") AS SOURCE_PRIORITY\n" +
		"FROM GV$SYSTEM_PARAMETER S WHERE " + predicate
	return "SELECT " + numeric("ROUND(TO_NUMBER(VALUE)/POWER(1024,3),3)") + " AS VALUE, " + numeric("SOURCE_CONTAINER_ID") + " AS SOURCE_CONTAINER_ID, " +
		numeric(current) + " AS CURRENT_CONTAINER_ID, PARAMETER_NAME AS PARAMETER_NAME, " + numeric("SOURCE_ROWS") + " AS SOURCE_ROWS, " +
		numeric("TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE'))") + " AS INSTANCE_ID, " + numeric(current) + " AS TARGET_CONTAINER_ID FROM (" + inner + ") WHERE SOURCE_PRIORITY=1", nil
}

// Decode validates Statement results, preserving NULL/no-row and provenance.
func Decode(result session.Result, parameter Parameter, o Options) (Observation, error) {
	if err := validateOptions(o); err != nil {
		return Observation{}, err
	}
	if o.Scope == ConnectedPDB || o.Scope == DiscoveredPDB {
		value, err := decodePDB(result, parameter, o.PDB, o.Scope == DiscoveredPDB)
		if err == nil && value.State != NoRows && o.TargetContainerID != 0 && value.TargetContainerID != o.TargetContainerID {
			return Observation{}, ErrInvalidEvidence
		}
		return value, err
	}
	if parameter != SGA && parameter != PGA {
		return Observation{}, ErrUnsupportedParameter
	}
	// Reuse the single schema/cardinality admission path; root/non-CDB have their
	// own source identity checks and do not acquire PDB inheritance permissions.
	row, absent, err := resultRow(result)
	if err != nil {
		return Observation{}, err
	}
	if absent {
		return Observation{State: NoRows}, nil
	}
	current, ok := integerCell(row["current_container_id"])
	if !ok || (o.Scope == NonCDB && current != 0) || (o.Scope == CDBRoot && current != 1) {
		return Observation{}, ErrInvalidEvidence
	}
	target, ok := integerCell(row["target_container_id"])
	if !ok || target != current {
		return Observation{}, ErrInvalidEvidence
	}
	source, ok := integerCell(row["source_container_id"])
	if !ok || source < 0 || source > 1 {
		return Observation{}, ErrInvalidEvidence
	}
	if o.Scope == NonCDB && source != 0 {
		return Observation{}, ErrInvalidEvidence
	}
	instance, ok := integerCell(row["instance_id"])
	if !ok || instance <= 0 || row["source_rows"].Null || row["source_rows"].Text != "1" {
		return Observation{}, ErrInvalidEvidence
	}
	name := "pga_aggregate_target"
	if parameter == SGA {
		name = "sga_max_size"
	}
	if row["parameter_name"].Null || row["parameter_name"].Text != name {
		return Observation{}, ErrInvalidEvidence
	}
	value := Observation{State: Available, Parameter: name, Origin: InstanceParameter, SourceContainerID: source, CurrentContainerID: current, TargetContainerID: target, InstanceID: instance}
	if row["value"].Null {
		value.State = Null
		return value, nil
	}
	if !validDecimal(row["value"].Text) {
		return Observation{}, ErrInvalidEvidence
	}
	value.ValueGiB = strings.Clone(row["value"].Text)
	return value, nil
}

func parameterPlan(parameter Parameter, options Options) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	text, err := Statement(parameter, options)
	if err != nil {
		return datasetSelection{}, err
	}
	source := "GV$SYSTEM_PARAMETER"
	if options.Scope == DiscoveredPDB {
		source = "SYS.GV_$SYSTEM_PARAMETER"
	}
	if options.PDB.EffectiveADB {
		source = "V$PARAMETER"
	}
	names := []string{"VALUE", "SOURCE_CONTAINER_ID", "CURRENT_CONTAINER_ID", "PARAMETER_NAME", "SOURCE_ROWS", "INSTANCE_ID", "TARGET_CONTAINER_ID"}
	return memorySelection(text, source, parameter, options, names), nil
}

// Read selects and decodes one complete configured-memory source without
// fallback. Collect remains the independent-evidence/guarded-fallback property API.
func Read(ctx context.Context, reader Reader, request SourceRequest) (Observation, error) {
	p, err := parameterPlan(request.Parameter, request.Options)
	if err != nil {
		return Observation{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return Observation{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return Observation{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return Observation{}, err
	}
	if _, err = datasetColumns(result, p); err != nil {
		return Observation{}, err
	}
	facts, err := Decode(result, request.Parameter, request.Options)
	if err = datasetReadError(ctx, p, err); err != nil {
		return Observation{}, err
	}
	return facts, nil
}

// Rows returns the actual native cursor for Statement. Its caller owns every
// Next/Scan/Err/Close, deadline, scope/handle-affinity and finite stream budget.
func Rows(ctx context.Context, q rowset.Queryer, parameter Parameter, options Options) (*sql.Rows, error) {
	p, err := parameterPlan(parameter, options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// JSON independently consumes/finalizes and validates one complete source.
// It never starts an effective-source fallback or publishes a failed prefix.
func JSON(ctx context.Context, q rowset.Queryer, parameter Parameter, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := parameterPlan(parameter, options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := Decode(r, parameter, options); return err })
}

// ParameterStatement selects the same scoped projection and bind values as ParameterRows.
func ParameterStatement(options ParameterOptions) (string, []any, error) {
	plan, err := parameterPlan(options.Parameter, options.Options)
	return plan.sql, plan.args, err
}

// DecodeParameter validates the selected source before decoding its observation.
func DecodeParameter(result session.Result, options ParameterOptions) (Observation, error) {
	plan, err := parameterPlan(options.Parameter, options.Options)
	if err != nil {
		return Observation{}, err
	}
	if _, err = datasetColumns(result, plan); err != nil {
		return Observation{}, err
	}
	return Decode(result, options.Parameter, options.Options)
}

// ReadParameter is the named form of Read.
func ReadParameter(ctx context.Context, reader Reader, request SourceRequest) (Observation, error) {
	return Read(ctx, reader, request)
}

// ParameterRows is the named form of Rows.
func ParameterRows(ctx context.Context, q rowset.Queryer, options ParameterOptions) (*sql.Rows, error) {
	return Rows(ctx, q, options.Parameter, options.Options)
}

// ParameterJSON is the named form of JSON.
func ParameterJSON(ctx context.Context, q rowset.Queryer, options ParameterOptions, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options.Parameter, options.Options, limits)
}
