package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ConnectedArchitectureRow preserves source values and SQL NULL without application policy.
type ConnectedArchitectureRow struct {
	CDB         session.Value
	ContainerID session.Value
	Name        session.Value
}

// ConnectedArchitectureOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type ConnectedArchitectureOptions struct{ OracleMajor int }

// ConnectedArchitectureRequest supplies admitted options and a caller-owned SQL reference.
type ConnectedArchitectureRequest struct {
	Options   ConnectedArchitectureOptions
	Reference reference.Reference
}

// ConnectedArchitectureResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type ConnectedArchitectureResult struct {
	Provenance DatasetProvenance
	Rows       []ConnectedArchitectureRow
}

// ConnectedArchitectureStatement selects reviewed SQL with bound values, owning no connection.
func ConnectedArchitectureStatement(options ConnectedArchitectureOptions) (string, []any, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	return p.sql, p.args, err
}

func connectedArchitectureDatasetPlan(options ConnectedArchitectureOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := ConnectedArchitectureSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "CDB", Kind: rowset.Text}, {Name: "CONTAINER_ID", Kind: rowset.Text}, {Name: "CONTAINER_NAME", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$DATABASE+SYS_CONTEXT(USERENV)", OracleMajor: options.OracleMajor, Scope: "parent_cdb_flag_and_actual_current_session_container"}}, nil
}

// DecodeConnectedArchitecture validates the entire shape and every row before returning facts.
func DecodeConnectedArchitecture(result session.Result, options ConnectedArchitectureOptions) (ConnectedArchitectureResult, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	return decodeDatasetConnectedArchitecture(result, p)
}

func decodeDatasetConnectedArchitecture(result session.Result, p datasetSelection) (ConnectedArchitectureResult, error) {
	index, err := datasetColumns(result, p, "CONTAINER_ID")
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	if len(result.Rows) > 1 {
		return ConnectedArchitectureResult{}, ErrInvalidEvidence
	}
	rows := make([]ConnectedArchitectureRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = ConnectedArchitectureRow{CDB: r[index["CDB"]], ContainerID: r[index["CONTAINER_ID"]], Name: r[index["CONTAINER_NAME"]]}
	}
	return ConnectedArchitectureResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadConnectedArchitecture executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadConnectedArchitecture(ctx context.Context, reader Reader, request ConnectedArchitectureRequest) (ConnectedArchitectureResult, error) {
	p, err := connectedArchitectureDatasetPlan(request.Options)
	if err != nil {
		return ConnectedArchitectureResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ConnectedArchitectureResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	facts, err := decodeDatasetConnectedArchitecture(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return ConnectedArchitectureResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ConnectedArchitectureRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func ConnectedArchitectureRows(ctx context.Context, q rowset.Queryer, options ConnectedArchitectureOptions) (*sql.Rows, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// ConnectedArchitectureJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func ConnectedArchitectureJSON(ctx context.Context, q rowset.Queryer, options ConnectedArchitectureOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := connectedArchitectureDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetConnectedArchitecture(r, p); return err })
}

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
