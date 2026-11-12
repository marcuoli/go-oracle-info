package memory

import (
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// Scope describes the admitted database context, not permission to acquire one.
type Scope string

const (
	NonCDB        Scope = "non_cdb"
	CDBRoot       Scope = "cdb_root"
	ConnectedPDB  Scope = "connected_pdb"
	DiscoveredPDB Scope = "discovered_pdb"
)

// Options selects fixed release/container variants. TargetContainerID is required
// for root-discovered PDBs and optionally pins a connected PDB's identity.
type Options struct {
	Scope             Scope
	OracleMajor       int
	TargetContainerID int
	PDB               PDBOptions
}

// ErrUnsupportedScope rejects unknown releases and unadmitted container shapes.
var ErrUnsupportedScope = errors.New("unsupported configured-memory scope")

func validateOptions(o Options) error {
	if o.OracleMajor < 9 {
		return ErrUnsupportedScope
	}
	switch o.Scope {
	case NonCDB:
		if o.TargetContainerID != 0 || o.PDB.EffectiveADB {
			return ErrUnsupportedScope
		}
	case CDBRoot:
		if o.OracleMajor < 12 || o.TargetContainerID != 0 || o.PDB.EffectiveADB {
			return ErrUnsupportedScope
		}
	case ConnectedPDB, DiscoveredPDB:
		if o.OracleMajor < 12 || (o.TargetContainerID != 0 && o.TargetContainerID <= 2) {
			return ErrUnsupportedScope
		}
		if o.Scope == DiscoveredPDB && (o.TargetContainerID <= 2 || o.PDB.EffectiveADB) {
			return ErrUnsupportedScope
		}
		if o.PDB.EffectiveADB && o.OracleMajor < 18 {
			return ErrUnsupportedScope
		}
	default:
		return ErrUnsupportedScope
	}
	return nil
}

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
