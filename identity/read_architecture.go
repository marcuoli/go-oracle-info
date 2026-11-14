package identity

import (
	"context"
	"strconv"
	"strings"

	"github.com/marcuoli/go-sqltemplate/reference"
)

// ArchitectureReferences supplies distinct bootstrap and connected-container SQL sites.
type ArchitectureReferences struct {
	Version   reference.Reference
	Container reference.Reference
}

// Architecture describes the connected container, separately from its parent's CDB flag.
type Architecture string

const (
	CDB    Architecture = "CDB"
	PDB    Architecture = "PDB"
	NonCDB Architecture = "NON-CDB"
)

// ArchitectureMethod distinguishes dictionary signals from the11g release constraint.
type ArchitectureMethod string

const (
	DictionaryArchitecture    ArchitectureMethod = "dictionary"
	LegacyReleaseArchitecture ArchitectureMethod = "legacy_release"
)

// ConnectedArchitectureSQL requires established12+ support. The context ID is
// the session's actual connected container, not V$DATABASE's dictionary row ID.
const ConnectedArchitectureSQL = "SELECT CDB AS CDB, SYS_CONTEXT('USERENV','CON_ID') AS CONTAINER_ID, SYS_CONTEXT('USERENV','CON_NAME') AS CONTAINER_NAME FROM V$DATABASE"

// ArchitectureObservation separates the parent database's CDB flag from the
// connected architecture. Legacy11g classification follows the documented
// pre-multitenant release constraint, without fabricating any dictionary signals.
type ArchitectureObservation struct {
	PropertyEvidence
	Value         Architecture
	Method        ArchitectureMethod
	Version       Text
	DatabaseCDB   Text
	ContainerID   Text
	ContainerName Text
}

// ReadArchitecture selects only current-session facts on the admitted reader.
// On12+, all required signals must be present and mutually consistent. Missing
// signals leave the property unavailable; contradictory signals additionally
// return ErrInvalidEvidence. This never discovers or changes a container.
func ReadArchitecture(ctx context.Context, reader Reader, refs ArchitectureReferences) (ArchitectureObservation, error) {
	unavailable := Text{State: Unavailable}
	observation := ArchitectureObservation{
		PropertyEvidence: PropertyEvidence{State: Unavailable, Source: InstanceVersionSource},
		Version:          unavailable, DatabaseCDB: unavailable, ContainerID: unavailable, ContainerName: unavailable,
	}
	if err := validatePropertyReader(reader, refs.Version, refs.Container); err != nil {
		return observation, err
	}
	version, major, err := bootstrapProperty(ctx, reader, refs.Version, &observation.Attempts)
	observation.Version = version
	if err != nil || version.State != Available {
		return observation, err
	}
	if major < 12 {
		observation.State, observation.Value = Available, NonCDB
		observation.Method, observation.Fallback = LegacyReleaseArchitecture, true
		return observation, nil
	}
	observation.Source, observation.Method = ConnectedContainerSource, DictionaryArchitecture
	result, err := queryProperty(ctx, reader, refs.Container, ConnectedArchitectureSQL, ConnectedContainerSource, &observation.Attempts)
	if err != nil {
		return observation, err
	}
	values, absent, err := row(result, "CDB", "CONTAINER_ID", "CONTAINER_NAME")
	if err != nil {
		return observation, propertyDecodeFailure(ConnectedContainerSource, err, &observation.Attempts)
	}
	last := &observation.Attempts[len(observation.Attempts)-1]
	if absent {
		missing := Text{State: NoRows}
		observation.DatabaseCDB, observation.ContainerID, observation.ContainerName = missing, missing, missing
		last.State = NoRows
		return observation, nil
	}
	observation.DatabaseCDB, observation.ContainerID, observation.ContainerName =
		text(values["CDB"]), text(values["CONTAINER_ID"]), text(values["CONTAINER_NAME"])
	last.State = Available
	for _, signal := range []Text{observation.DatabaseCDB, observation.ContainerID, observation.ContainerName} {
		if signal.State != Available || strings.TrimSpace(signal.Value) == "" {
			return observation, nil
		}
	}
	value, err := connectedArchitecture(observation.DatabaseCDB.Value, observation.ContainerID.Value, observation.ContainerName.Value)
	if err != nil {
		return observation, propertyDecodeFailure(ConnectedContainerSource, err, &observation.Attempts)
	}
	observation.State, observation.Value = Available, value
	return observation, nil
}

func connectedArchitecture(cdb, containerID, name string) (Architecture, error) {
	if strings.IndexFunc(containerID, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return "", ErrInvalidEvidence
	}
	id, err := strconv.ParseUint(containerID, 10, 32)
	if err != nil {
		return "", ErrInvalidEvidence
	}
	root, seed := name == "CDB$ROOT", name == "PDB$SEED"
	switch {
	case cdb == "NO" && id == 0 && !root && !seed:
		return NonCDB, nil
	case cdb == "YES" && id == 1 && root:
		return CDB, nil
	case cdb == "YES" && id == 2 && seed:
		return PDB, nil
	case cdb == "YES" && id > 2 && !root && !seed:
		return PDB, nil
	default:
		return "", ErrInvalidEvidence
	}
}
