package memory

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SourceRequest admits one configured-memory source, not the Collect fallback
// sequence. EffectiveADB requires the caller-established primary NULL/no-row
// precondition and a known18+ connected PDB, as with PDBStatement.
type SourceRequest struct {
	Parameter Parameter
	Options   Options
	Reference reference.Reference
}

func sourcePlan(parameter Parameter, options Options, rac bool) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	var text string
	var err error
	names := []string{"VALUE", "SOURCE_CONTAINER_ID", "CURRENT_CONTAINER_ID", "PARAMETER_NAME", "SOURCE_ROWS", "INSTANCE_ID", "TARGET_CONTAINER_ID"}
	source := "GV$SYSTEM_PARAMETER"
	if options.Scope == DiscoveredPDB {
		source = "SYS.GV_$SYSTEM_PARAMETER"
	}
	if options.PDB.EffectiveADB {
		source = "V$PARAMETER"
	}
	if rac {
		text, err = RACStatement(options)
		names = []string{"PARAMETER_NAME", "PARAMETER_VALUE", "INSTANCE_ID", "SOURCE_CONTAINER_ID", "TARGET_CONTAINER_ID"}
		source = "GV$INSTANCE+GV$SYSTEM_PARAMETER"
		if options.Scope == ConnectedPDB || options.Scope == DiscoveredPDB {
			source = "GV$PDBS+GV$SYSTEM_PARAMETER"
		}
	} else {
		text, err = Statement(parameter, options)
	}
	if err != nil {
		return datasetSelection{}, err
	}
	schema := make([]rowset.Column, len(names))
	for i, name := range names {
		schema[i] = rowset.Column{Name: name, Kind: rowset.Text}
	}
	provenanceParameter := parameter
	if rac {
		provenanceParameter = ""
	}
	var args []any
	if options.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", options.TargetContainerID)}
	}
	return datasetSelection{sql: text, args: args, schema: schema, provenance: DatasetProvenance{Source: source, OracleMajor: options.OracleMajor, Scope: string(options.Scope), Parameter: provenanceParameter, TargetContainerID: options.TargetContainerID, AllowInherited: options.PDB.AllowInherited, EffectiveADB: options.PDB.EffectiveADB}}, nil
}

// Read selects and decodes one complete configured-memory source without
// fallback. Collect remains the independent-evidence/guarded-fallback property API.
func Read(ctx context.Context, reader Reader, request SourceRequest) (Observation, error) {
	p, err := sourcePlan(request.Parameter, request.Options, false)
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
	p, err := sourcePlan(parameter, options, false)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// JSON independently consumes/finalizes and validates one complete source.
// It never starts an effective-source fallback or publishes a failed prefix.
func JSON(ctx context.Context, q rowset.Queryer, parameter Parameter, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sourcePlan(parameter, options, false)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := Decode(r, parameter, options); return err })
}

// ReadPDB is the connected-PDB source form of Read, with identical admission.
func ReadPDB(ctx context.Context, reader Reader, request SourceRequest) (Observation, error) {
	if request.Options.Scope != ConnectedPDB {
		return Observation{}, ErrUnsupportedScope
	}
	return Read(ctx, reader, request)
}

// PDBRows returns the actual PDBStatement cursor with explicit full scope admission.
func PDBRows(ctx context.Context, q rowset.Queryer, parameter Parameter, options Options) (*sql.Rows, error) {
	if options.Scope != ConnectedPDB {
		return nil, ErrUnsupportedScope
	}
	return Rows(ctx, q, parameter, options)
}

// PDBJSON is the atomic JSON companion to the same admitted PDBStatement source.
func PDBJSON(ctx context.Context, q rowset.Queryer, parameter Parameter, options Options, limits rowset.Limits) (json.RawMessage, error) {
	if options.Scope != ConnectedPDB {
		return nil, ErrUnsupportedScope
	}
	return JSON(ctx, q, parameter, options, limits)
}

// ReadRAC retains CollectRAC's independently valid metric evidence alongside
// errors. This is aggregate evidence, never a failed cursor prefix or new retry.
func ReadRAC(ctx context.Context, reader Reader, options Options, ref reference.Reference) (RACObservation, error) {
	unavailable := RACObservation{SGA: Total{State: Unavailable}, PGA: Total{State: Unavailable}}
	p, err := sourcePlan(SGA, options, true)
	if err != nil {
		return unavailable, err
	}
	if !validDatasetReader(reader) || ref.Validate() != nil {
		return unavailable, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return unavailable, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: ref})
	missing := onlyNoRows(err)
	if missing {
		err = nil
	}
	if err = datasetReadError(ctx, p, err); err != nil {
		return unavailable, err
	}
	if missing {
		return RACObservation{SGA: Total{State: NoRows}, PGA: Total{State: NoRows}}, nil
	}
	// Validate the entire transport before the legacy aggregate decoder can skip
	// an absent metric name. A contradictory NULL scalar is not partial evidence.
	// Valid independently available metrics still survive domain decoding errors.
	if _, err = datasetColumns(result, p); err != nil {
		return unavailable, err
	}
	observation, err := DecodeRAC(result, options)
	if ctx.Err() != nil {
		return unavailable, datasetReadError(ctx, p, err)
	}
	return observation, err
}

// RACRows returns the actual native RACStatement cursor, not a fabricated total.
func RACRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, err := sourcePlan(SGA, options, true)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// RACJSON preserves complete source rows; it does not infer physical membership
// or certify both totals. Domain/transport/cancellation/budget failures return nil.
func RACJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sourcePlan(SGA, options, true)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeRAC(r, options); return err })
}

// ParameterOptions admits a configured-memory setting and its source scope.
type ParameterOptions struct {
	Parameter Parameter
	Options   Options
}

// ParameterStatement selects the same scoped projection and bind values as ParameterRows.
func ParameterStatement(options ParameterOptions) (string, []any, error) {
	plan, err := sourcePlan(options.Parameter, options.Options, false)
	return plan.sql, plan.args, err
}

// DecodeParameter validates the selected source before decoding its observation.
func DecodeParameter(result session.Result, options ParameterOptions) (Observation, error) {
	plan, err := sourcePlan(options.Parameter, options.Options, false)
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

// RACRequest adds the caller's SQL reference to one admitted RAC selection.
type RACRequest struct {
	Options   Options
	Reference reference.Reference
}

// ReadRACWithOptions reads RAC memory using a single caller request.
func ReadRACWithOptions(ctx context.Context, reader Reader, request RACRequest) (RACObservation, error) {
	return ReadRAC(ctx, reader, request.Options, request.Reference)
}

// PDBStatementWithOptions selects a fully admitted connected-PDB projection.
func PDBStatementWithOptions(options ParameterOptions) (string, []any, error) {
	if options.Options.Scope != ConnectedPDB {
		return "", nil, ErrUnsupportedScope
	}
	return ParameterStatement(options)
}

// DecodePDBWithOptions validates a complete connected-PDB source.
func DecodePDBWithOptions(result session.Result, options ParameterOptions) (Observation, error) {
	if options.Options.Scope != ConnectedPDB {
		return Observation{}, ErrUnsupportedScope
	}
	return DecodeParameter(result, options)
}

// PDBRowsWithOptions is the named form of PDBRows.
func PDBRowsWithOptions(ctx context.Context, q rowset.Queryer, options ParameterOptions) (*sql.Rows, error) {
	return PDBRows(ctx, q, options.Parameter, options.Options)
}

// PDBJSONWithOptions is the named form of PDBJSON.
func PDBJSONWithOptions(ctx context.Context, q rowset.Queryer, options ParameterOptions, limits rowset.Limits) (json.RawMessage, error) {
	return PDBJSON(ctx, q, options.Parameter, options.Options, limits)
}
