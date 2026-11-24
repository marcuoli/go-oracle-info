package services

import (
	"context"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ScanRequest identifies an admitted Oracle release and the two read references.
// Limits bound combined retained rows and registration evidence.
// The supplied reader also bounds each individual read.
type ScanRequest struct {
	OracleVersion       string
	ParametersReference reference.Reference
	ListenersReference  reference.Reference
	Limits              rowset.Limits
}

// ScanParameter retains each ordered parameter value and its reporting scope.
type ScanParameter struct {
	InstanceID  session.Value  `json:"instance_id"`
	ContainerID *session.Value `json:"container_id,omitempty"`
	Name        session.Value  `json:"name"`
	Ordinal     session.Value  `json:"ordinal"`
	Value       session.Value  `json:"value"`
}

// ScanRegistrationState describes the outcome of the registration view read.
type ScanRegistrationState string

const (
	ScanRegistrationAvailable    ScanRegistrationState = "available"
	ScanRegistrationNoRows       ScanRegistrationState = "no_rows"
	ScanRegistrationInaccessible ScanRegistrationState = "inaccessible"
	ScanRegistrationUnavailable  ScanRegistrationState = "unavailable"
)

// ScanObservation reports a configured SCAN hostname candidate and the SQL
// evidence supporting it. An empty Name means unknown. The candidate is never
// Clusterware or DNS verification. The two reads are independent observations.
type ScanObservation struct {
	Name                string                  `json:"name,omitempty"`
	ParameterProvenance parameters.Provenance   `json:"parameter_provenance"`
	Parameters          []ScanParameter         `json:"parameters"`
	RegistrationState   ScanRegistrationState   `json:"registration_state"`
	Registration        *ListenerNetworksResult `json:"registration,omitempty"`
}

// ReadScan reads effective REMOTE_LISTENER and LISTENER_NETWORKS and visible
// remote registrations. A diagnosed source-access error during cursor opening
// is reported as a registration state. Errors from later stages fail the read.
func ReadScan(ctx context.Context, reader Reader, request ScanRequest) (ScanObservation, error) {
	major, err := scanMajor(request.OracleVersion)
	if err != nil {
		return ScanObservation{}, err
	}
	if !validDatasetReader(reader) || request.ParametersReference.Validate() != nil || request.ListenersReference.Validate() != nil {
		return ScanObservation{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ScanObservation{}, err
	}
	facts, err := parameters.ReadEffective(ctx, reader, parameters.Request{
		OracleMajor: major,
		Filter:      parameters.Filter{Names: []string{"remote_listener", "listener_networks"}},
		Reference:   request.ParametersReference,
	})
	if err != nil {
		return ScanObservation{}, err
	}
	if len(facts.Rows) > limits.MaxRows {
		return ScanObservation{}, rowset.ErrCapacity
	}
	observation := ScanObservation{
		ParameterProvenance: facts.Provenance,
		Parameters:          make([]ScanParameter, len(facts.Rows)),
	}
	for i, row := range facts.Rows {
		observation.Parameters[i] = ScanParameter{
			InstanceID: row.InstanceID, ContainerID: row.ContainerID,
			Name: row.Name, Ordinal: row.Ordinal, Value: row.Value,
		}
	}
	observation.Name = configuredScanName(observation.Parameters)
	registrations, err := ReadListenerNetworks(ctx, reader, ListenerNetworksRequest{
		Options:   ListenerNetworksOptions{OracleVersion: request.OracleVersion},
		Reference: request.ListenersReference,
		Limits:    limits,
	})
	if ctx.Err() != nil {
		return ScanObservation{}, errors.Join(err, ctx.Err())
	}
	switch {
	case errors.Is(err, projection.ErrSourceInaccessible):
		observation.RegistrationState = ScanRegistrationInaccessible
	case errors.Is(err, projection.ErrSourceUnavailable):
		observation.RegistrationState = ScanRegistrationUnavailable
	case err != nil:
		return ScanObservation{}, err
	default:
		if len(registrations.Rows) > limits.MaxRows-len(observation.Parameters) {
			return ScanObservation{}, rowset.ErrCapacity
		}
		observation.Registration = &registrations
		observation.RegistrationState = ScanRegistrationNoRows
		if len(registrations.Rows) > 0 {
			observation.RegistrationState = ScanRegistrationAvailable
		}
	}
	return observation, nil
}

func configuredScanName(rows []ScanParameter) string {
	type scope struct {
		instance  string
		container session.Value
	}
	seen := make(map[scope]uint8)
	var name string
	for _, row := range rows {
		if row.InstanceID.Null || (row.ContainerID != nil && row.ContainerID.Null) {
			return ""
		}
		key := scope{instance: row.InstanceID.Text}
		if row.ContainerID != nil {
			key.container = *row.ContainerID
		}
		targets := []string{row.Value.Text}
		switch row.Name.Text {
		case "remote_listener":
			seen[key] |= 1
		case "listener_networks":
			seen[key] |= 2
			if !row.Value.Null {
				var ok bool
				targets, ok = remoteNetworkTargets(row.Value.Text)
				if !ok {
					return ""
				}
			}
		}
		if row.Value.Null {
			continue
		}
		for _, target := range targets {
			hosts := registrationHosts(target, 0)
			if len(hosts) == 0 {
				return ""
			}
			for _, host := range hosts {
				if name != "" && name != host {
					return ""
				}
				name = host
			}
		}
	}
	for _, sources := range seen {
		if sources != 3 {
			return ""
		}
	}
	return name
}
