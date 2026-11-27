package parameters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marcuoli/go-oracle-session/rowset"
)

// PDBOverrideState describes current evidence, independently of the last
// ALTER SYSTEM scope and future reopen behavior.
type PDBOverrideState string

const (
	PDBOverridePersistent PDBOverrideState = "persistent_override"
	PDBOverrideMemoryOnly PDBOverrideState = "memory_only_observed"
	PDBOverrideUnknown    PDBOverrideState = "unknown"
)

// ErrInvalidPDBOverridesRequest rejects a missing PDB/member, invalid age or
// unsupported connected scope.
var ErrInvalidPDBOverridesRequest = errors.New("invalid PDB parameter overrides request")

// PDBOverridesRequest supplies fresh observations from the same admitted direct
// PDB and reporting instance. Member.Scope must be PersistencePDB. The caller
// establishes that both observations belong to the same database and PDB.
type PDBOverridesRequest struct {
	OracleVersion string
	ContainerID   int64
	At            time.Time
	MaxAge        time.Duration
	// RuntimePairVerified admits a stable pairing across both source reads.
	RuntimePairVerified bool
	Member              PersistenceMember
}

// PDBOverrideFinding retains effective and stored source rows. Persistent means
// at least one explicit PDB clause was visible, including a clause for a dormant
// SID. It does not mean the stored value is currently in use.
type PDBOverrideFinding struct {
	Name      string            `json:"name"`
	State     PDBOverrideState  `json:"state"`
	Effective []Parameter       `json:"effective"`
	Stored    []SPFileParameter `json:"stored"`
}

// PDBOverridesReport is an observation for one PDB and reporting instance.
type PDBOverridesReport struct {
	OracleVersion       string               `json:"oracle_version"`
	ContainerID         int64                `json:"container_id"`
	InstanceID          int64                `json:"instance_id"`
	At                  time.Time            `json:"at"`
	EffectiveObserved   bool                 `json:"effective_observed"`
	StoredObserved      bool                 `json:"stored_observed"`
	EffectiveProvenance Provenance           `json:"effective_provenance"`
	StoredProvenance    Provenance           `json:"stored_provenance"`
	Findings            []PDBOverrideFinding `json:"findings"`
}

// EvaluatePDBOverrides classifies already-collected evidence and owns no
// database connection or file access. Unusable source evidence does not erase
// independently verified observations from the other source.
func EvaluatePDBOverrides(ctx context.Context, request PDBOverridesRequest) (PDBOverridesReport, error) {
	if err := ctx.Err(); err != nil {
		return PDBOverridesReport{}, err
	}
	m := request.Member
	if request.ContainerID <= 2 || request.At.IsZero() || request.MaxAge <= 0 || m.InstanceID <= 0 || m.Scope != PersistencePDB {
		return PDBOverridesReport{}, ErrInvalidPDBOverridesRequest
	}
	major := containerSystemMajor(request.OracleVersion)
	if major == 0 {
		return PDBOverridesReport{}, ErrUnsupportedVersion
	}
	report := PDBOverridesReport{
		OracleVersion: request.OracleVersion, ContainerID: request.ContainerID, InstanceID: m.InstanceID, At: request.At,
		Findings: make([]PDBOverrideFinding, 0),
	}
	names := map[string]*PDBOverrideFinding{}
	target := strconv.FormatInt(request.ContainerID, 10)
	finding := func(name string) *PDBOverrideFinding {
		if names[name] == nil {
			names[name] = &PDBOverrideFinding{Name: name, State: PDBOverrideUnknown}
		}
		return names[name]
	}

	ep := m.Effective.Provenance
	if freshPDBEvidence(request, m.EffectiveEvidence) && pdbOverrideSource(ep, "GV$SYSTEM_PARAMETER2", major) {
		effective := PersistenceMember{InstanceID: m.InstanceID, Scope: PersistencePDB, Effective: m.Effective}
		groups, reason, err := persistenceGroups(ctx, effective)
		if err != nil {
			return PDBOverridesReport{}, err
		}
		if reason == "" && pdbOverrideGroupsInScope(groups, ep, target) {
			report.EffectiveObserved = true
			report.EffectiveProvenance = clonePersistenceProvenance(ep)
			for key, group := range groups {
				if key.container == "0" {
					finding(key.name).Effective = clonePersistenceEffective(group.effective)
				}
			}
			for key, group := range groups {
				if key.container == target {
					f := finding(key.name)
					f.Effective = append(f.Effective, clonePersistenceEffective(group.effective)...)
				}
			}
		}
	}

	sp := m.Stored.Provenance
	if freshPDBEvidence(request, m.StoredEvidence) && pdbOverrideSource(sp, "GV$SPPARAMETER", major) {
		// No SID precedence is applied. The report preserves stored clauses
		// for names without an effective row, including other members' SIDs.
		stored := PersistenceMember{InstanceID: m.InstanceID, Scope: PersistencePDB, Stored: m.Stored}
		groups, reason, err := persistenceGroups(ctx, stored)
		if err != nil {
			return PDBOverridesReport{}, err
		}
		if reason == "" && pdbOverrideGroupsInScope(groups, sp, target) {
			report.StoredObserved = true
			report.StoredProvenance = clonePersistenceProvenance(sp)
			for key, group := range groups {
				if key.container != target {
					continue
				}
				// Unspecified placeholders annotate observed names; by
				// themselves they are not a parameter setting to report.
				if f := names[key.name]; f != nil {
					f.Stored = clonePersistenceStored(group.stored)
				} else if slices.ContainsFunc(group.stored, func(row SPFileParameter) bool {
					return row.IsSpecified.Text == "TRUE"
				}) {
					finding(key.name).Stored = clonePersistenceStored(group.stored)
				}
			}
		}
	}

	startup := m.Startup
	memoryAdmitted := request.RuntimePairVerified &&
		startup.Kind == PersistenceSPFile && startup.Verified && !startup.SPFilePath.Null &&
		strings.TrimSpace(startup.SPFilePath.Text) != "" && validPersistenceValue(startup.SPFilePath) &&
		freshPDBEvidence(request, PersistenceEvidence{Complete: true, ObservedAt: startup.ObservedAt}) &&
		report.EffectiveObserved && report.StoredObserved && len(sp.SIDs) == 0 && sp.SIDPolicy == "all_stored_clauses"

	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	for _, name := range keys {
		if err := ctx.Err(); err != nil {
			return PDBOverridesReport{}, err
		}
		f := names[name]
		switch {
		case explicitPDBClause(f.Stored):
			f.State = PDBOverridePersistent
		case memoryAdmitted && qualifiedPDBMemoryPair(request.OracleVersion, name) && observedPDBMemoryOnly(*f, target):
			f.State = PDBOverrideMemoryOnly
		}
		report.Findings = append(report.Findings, *f)
	}
	if err := ctx.Err(); err != nil {
		return PDBOverridesReport{}, err
	}
	return report, nil
}

func freshPDBEvidence(request PDBOverridesRequest, evidence PersistenceEvidence) bool {
	return evidence.Complete && !evidence.ObservedAt.IsZero() &&
		!evidence.ObservedAt.After(request.At) && request.At.Sub(evidence.ObservedAt) <= request.MaxAge
}

func pdbOverrideSource(p Provenance, source string, major int) bool {
	if p.Source != source || p.Scope != "current_container_and_whole_database_rows" || p.OracleMajor != major ||
		p.NameClass != "" || p.NameContains != "" || !validPersistenceProvenance(p) ||
		validate(p.Names, p.Instances, p.SIDs) != nil {
		return false
	}
	if source == "GV$SPPARAMETER" {
		if len(p.SIDs) == 0 {
			return p.SIDPolicy == "all_stored_clauses"
		}
		return p.SIDPolicy == "selected_sids_plus_wildcard" && slices.Contains(p.SIDs, "*")
	}
	return len(p.SIDs) == 0 && p.SIDPolicy == ""
}

func pdbOverrideGroupsInScope(groups map[persistenceKey]*persistenceGroup, p Provenance, target string) bool {
	for key, group := range groups {
		if (key.container != "0" && key.container != target) ||
			(len(p.Names) > 0 && !slices.Contains(p.Names, key.name)) {
			return false
		}
		for _, row := range group.stored {
			if !admittedText(row.SID, p.SIDs) {
				return false
			}
		}
	}
	return true
}

func explicitPDBClause(rows []SPFileParameter) bool {
	type clause struct {
		sid     string
		ordinal int64
	}
	seen := make(map[clause]bool)
	flags := make(map[string]string)
	explicit := false
	for _, row := range rows {
		ordinal, ok := persistenceOrdinal(row.Ordinal.Text)
		if !ok || row.Ordinal.Null || row.SID.Null || row.IsSpecified.Null ||
			validate(nil, nil, []string{row.SID.Text}) != nil {
			return false
		}
		key := clause{sid: row.SID.Text, ordinal: ordinal}
		if seen[key] {
			return false
		}
		seen[key] = true
		flag := row.IsSpecified.Text
		if previous, ok := flags[row.SID.Text]; ok && previous != flag {
			return false
		}
		flags[row.SID.Text] = flag
		switch flag {
		case "TRUE":
			explicit = true
		case "FALSE":
		default:
			return false
		}
	}
	return explicit
}

// Admit exact native qualifications only. See the PDB override evidence matrix
// for the recorded controls, patch levels and connection scope.
func qualifiedPDBMemoryPair(version, name string) bool {
	switch version {
	case "12.2.0.1.0", "18.4.0.0.0", "19.32.0.0.0", "21.3.0.0.0", "23.9.0.25.07", "23.26.3.0.0":
		switch name {
		case "open_cursors", "optimizer_index_cost_adj", "optimizer_dynamic_sampling":
			return true
		}
	}
	return false
}

func observedPDBMemoryOnly(f PDBOverrideFinding, target string) bool {
	if len(f.Effective) != 1 || len(f.Stored) != 1 {
		return false
	}
	effective, stored := f.Effective[0], f.Stored[0]
	id, ok := persistenceContainerKey(effective.ContainerID, 12)
	_, integer := comparePersistenceValue(3, effective.Value.Text, effective.Value.Text)
	return integer && ok && id == target && effective.Type.Text == "3" && !effective.Value.Null &&
		effective.Ordinal.Text == "1" && effective.IsModified.Text == "MODIFIED" &&
		effective.IsPDBModifiable != nil && effective.IsPDBModifiable.Text == "TRUE" &&
		stored.IsSpecified.Text == "FALSE" && stored.SID.Text == "*" && stored.Ordinal.Text == "0" && stored.Value.Null
}

// PDBOverridesJSON independently evaluates the evidence and includes snapshots
// in its row and encoded-byte limits. No failure returns a result prefix.
func PDBOverridesJSON(ctx context.Context, request PDBOverridesRequest, limits rowset.Limits) (json.RawMessage, error) {
	bounds, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	effective, stored := len(request.Member.Effective.Rows), len(request.Member.Stored.Rows)
	if effective > bounds.MaxRows || stored > bounds.MaxRows-effective {
		return nil, rowset.ErrCapacity
	}
	report, err := EvaluatePDBOverrides(ctx, request)
	if err != nil {
		return nil, err
	}
	remaining := bounds.MaxRows
	for _, f := range report.Findings {
		remaining -= 1 + len(f.Effective) + len(f.Stored)
		if remaining < 0 {
			return nil, rowset.ErrCapacity
		}
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if len(payload) > bounds.MaxBytes {
		return nil, rowset.ErrCapacity
	}
	return payload, nil
}
