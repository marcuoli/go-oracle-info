package memory

import (
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// State separates a known value, SQL NULL, no row and incomplete evidence.
type State string

const (
	Available   State = "available"
	NoRows      State = "no_rows"
	Null        State = "null"
	Unavailable State = "unavailable"
)

// Origin identifies configured parameter scope, never observed consumption.
type Origin string

const (
	ContainerParameter Origin = "pdb_parameter"
	InheritedInstance  Origin = "inherited_instance_parameter"
	EffectiveADB       Origin = "adb_container_parameter"
	InstanceParameter  Origin = "instance_parameter"
)

// Observation owns source-rounded GiB text and source/target/execution identity.
// An inherited value is not a PDB-specific allocation or a cluster total.
type Observation struct {
	State             State
	ValueGiB          string
	Parameter         string
	Origin            Origin
	SourceContainerID int
	// CurrentContainerID is the actual execution container. A root-discovered
	// PDB read keeps root (1) here and the admitted PDB in TargetContainerID.
	CurrentContainerID int
	TargetContainerID  int
	InstanceID         int
}

// ErrInvalidEvidence rejects malformed, ambiguous or out-of-scope source results.
var ErrInvalidEvidence = errors.New("invalid configured-memory evidence")

var columns = []string{"value", "source_container_id", "current_container_id", "parameter_name", "source_rows", "instance_id", "target_container_id"}

func resultRow(result session.Result) (map[string]session.Value, bool, error) {
	if len(result.Columns) != len(columns) || len(result.Rows) > 1 {
		return nil, false, ErrInvalidEvidence
	}
	index := make(map[string]int, len(columns))
	for i, name := range result.Columns {
		name = strings.ToLower(name)
		if _, exists := index[name]; name == "" || exists {
			return nil, false, ErrInvalidEvidence
		}
		index[name] = i
	}
	for _, name := range columns {
		if _, ok := index[name]; !ok {
			return nil, false, ErrInvalidEvidence
		}
	}
	if len(result.Rows) == 0 {
		return nil, true, nil
	}
	if len(result.Rows[0]) != len(columns) {
		return nil, false, ErrInvalidEvidence
	}
	row := make(map[string]session.Value, len(columns))
	for name, i := range index {
		row[name] = result.Rows[0][i]
	}
	return row, false, nil
}

func integerCell(value session.Value) (int, bool) {
	number, err := strconv.Atoi(value.Text)
	return number, err == nil && !value.Null
}

func validDecimal(value string) bool {
	if len(value) > 128 || value == "" || value == "." {
		return false
	}
	digits := strings.Replace(value, ".", "", 1)
	return strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func zeroDecimal(value string) bool { return strings.Trim(value, "0.") == "" }

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
