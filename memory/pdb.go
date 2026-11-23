// Package memory selects and decodes configured memory and observed allocation facts.
// It opens no sessions and leaves endpoint/container admission to callers.
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

const (
	Available   State = "available"
	NoRows      State = "no_rows"
	Null        State = "null"
	Unavailable State = "unavailable"
)

const (
	ContainerParameter Origin = "pdb_parameter"
	InheritedInstance  Origin = "inherited_instance_parameter"
	EffectiveADB       Origin = "adb_container_parameter"
	InstanceParameter  Origin = "instance_parameter"
)

// ErrInvalidEvidence rejects malformed, ambiguous or out-of-scope source results.
var ErrInvalidEvidence = errors.New("invalid configured-memory evidence")

var columns = []string{"value", "source_container_id", "current_container_id", "parameter_name", "source_rows", "instance_id", "target_container_id"}

// DecodePDB validates PDBStatement results with the same options. NoRows has no
// inferred identities; NULL retains verified source identity. Errors do not
// permit an effective-value retry. The supplied reader owns endpoint admission.
func DecodePDB(result session.Result, parameter Parameter, options PDBOptions) (Observation, error) {
	return decodePDB(result, parameter, options, false)
}

func decodePDB(result session.Result, parameter Parameter, options PDBOptions, discovered bool) (Observation, error) {
	if parameter != SGA && parameter != PGA {
		return Observation{}, ErrUnsupportedParameter
	}
	row, absent, err := resultRow(result)
	if err != nil {
		return Observation{}, err
	}
	if absent {
		return Observation{State: NoRows}, nil
	}
	current, currentOK := integerCell(row["current_container_id"])
	target, targetOK := integerCell(row["target_container_id"])
	source, sourceOK := integerCell(row["source_container_id"])
	instance, instanceOK := integerCell(row["instance_id"])
	if !currentOK || !targetOK || !sourceOK || !instanceOK || instance <= 0 || target <= 2 || row["source_rows"].Null || row["source_rows"].Text != "1" {
		return Observation{}, ErrInvalidEvidence
	}
	if (discovered && current != 1) || (!discovered && current != target) {
		return Observation{}, ErrInvalidEvidence
	}
	if source != target && (options.EffectiveADB || !options.AllowInherited || (source != 0 && source != 1)) {
		return Observation{}, ErrInvalidEvidence
	}
	name := "pga_aggregate_target"
	if parameter == SGA {
		name = "sga_target"
		if source != target {
			name = "sga_max_size"
		}
	}
	if row["parameter_name"].Null || row["parameter_name"].Text != name {
		return Observation{}, ErrInvalidEvidence
	}
	value := Observation{State: Available, Parameter: name, Origin: ContainerParameter, SourceContainerID: source, CurrentContainerID: current, TargetContainerID: target, InstanceID: instance}
	if source != target {
		value.Origin = InheritedInstance
	}
	if options.EffectiveADB {
		value.Origin = EffectiveADB
	}
	if row["value"].Null {
		value.State = Null
		return value, nil
	}
	if !validDecimal(row["value"].Text) || (parameter == SGA && source == target && zeroDecimal(row["value"].Text)) {
		return Observation{}, ErrInvalidEvidence
	}
	value.ValueGiB = strings.Clone(row["value"].Text)
	return value, nil
}

const (
	SGA Parameter = "sga"
	PGA Parameter = "pga"
)

// PDBOptions declares already-admitted inheritance and effective-value selection.
// EffectiveADB requires known Oracle 18+ and a primary NULL/no-row outcome.
// Use Collect to enforce that transition; PDBStatement alone executes nothing.
type PDBOptions struct {
	AllowInherited bool
	EffectiveADB   bool
}

// PDBStatement selects current-instance, connected-PDB configured settings.
// It returns exact three-decimal GiB text and source/current-container identity.
// An inherited setting is not a PDB allocation or a RAC total.
func PDBStatement(parameter Parameter, options PDBOptions) (string, error) {
	return pdbStatement(parameter, options, "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))", "GV$SYSTEM_PARAMETER")
}

func pdbStatement(parameter Parameter, options PDBOptions, target, source string) (string, error) {
	child, parent := "pga_aggregate_target", "pga_aggregate_target"
	childPriority := "P.SOURCE_CONTAINER_ID=" + target
	selection := ""
	switch parameter {
	case SGA:
		child, parent = "sga_target", "sga_max_size"
		// Duplicate zero rows must win child priority and reach the decoder. Dropping
		// them before selection would silently promote a valid-looking parent value.
		childPriority += " AND (P.SOURCE_ROWS<>1 OR P.VALUE IS NULL OR TO_NUMBER(P.VALUE)<>0)"
		selection = " AND (SOURCE_ROWS<>1 OR SOURCE_CONTAINER_ID<>" + target + " OR VALUE IS NULL OR TO_NUMBER(VALUE)<>0)"
	case PGA:
	default:
		return "", ErrUnsupportedParameter
	}
	projection := "SELECT " + numeric("ROUND(TO_NUMBER(VALUE)/POWER(1024,3),3)") + " AS VALUE, " +
		numeric("SOURCE_CONTAINER_ID") + " AS SOURCE_CONTAINER_ID, " + numeric("TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))") + " AS CURRENT_CONTAINER_ID, " +
		"PARAMETER_NAME AS PARAMETER_NAME, " + numeric("SOURCE_ROWS") + " AS SOURCE_ROWS, " +
		numeric("TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE'))") + " AS INSTANCE_ID, " + numeric(target) + " AS TARGET_CONTAINER_ID FROM ("
	inherited := ""
	if options.AllowInherited {
		inherited = " OR (S.CON_ID IN (0,1) AND S.NAME='" + parent + "')"
	}
	query := projection + "SELECT P.VALUE AS VALUE, P.PARAMETER_NAME AS PARAMETER_NAME, P.SOURCE_CONTAINER_ID AS SOURCE_CONTAINER_ID, P.SOURCE_ROWS AS SOURCE_ROWS,\n" +
		"DENSE_RANK() OVER (ORDER BY CASE WHEN " + childPriority + " THEN 0 WHEN P.SOURCE_CONTAINER_ID=0 THEN 1 WHEN P.SOURCE_CONTAINER_ID=1 THEN 2 ELSE 3 END) AS SOURCE_PRIORITY\n" +
		"FROM (SELECT S.VALUE AS VALUE, S.NAME AS PARAMETER_NAME, S.CON_ID AS SOURCE_CONTAINER_ID,\n" +
		"COUNT(*) OVER (PARTITION BY S.CON_ID,S.NAME) AS SOURCE_ROWS FROM " + source + " S\n" +
		"WHERE S.INST_ID=TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AND ((S.CON_ID=" + target + " AND S.NAME='" + child + "')" + inherited + ")) P)\n" +
		"WHERE SOURCE_PRIORITY=1" + selection
	if options.EffectiveADB {
		query = projection + "SELECT P.VALUE AS VALUE, P.NAME AS PARAMETER_NAME, P.CON_ID AS SOURCE_CONTAINER_ID, COUNT(*) OVER () AS SOURCE_ROWS\n" +
			"FROM V$PARAMETER P WHERE P.CON_ID=" + target + " AND P.NAME='" + child + "'\n" +
			"AND P.ISSES_MODIFIABLE='FALSE' AND SYS_CONTEXT('USERENV','CLOUD_SERVICE') IN ('DWCS','OLTP','JDCS'))"
		if parameter == SGA {
			query += " WHERE SOURCE_ROWS<>1 OR VALUE IS NULL OR TO_NUMBER(VALUE)>0"
		}
	}
	return query, nil
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
