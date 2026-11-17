package parameters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// PersistenceState describes admitted observations, conditional on the same
// startup configuration remaining applicable. It never predicts an operator's
// next startup choice, future edits, defaults or successful startup.
type PersistenceState string

const (
	PersistenceMatch     PersistenceState = "matches"
	PersistenceDifferent PersistenceState = "explicit_stored_difference"
	PersistenceNotStored PersistenceState = "not_explicitly_stored_default_unresolved"
	PersistenceUnknown   PersistenceState = "unknown"
)

// PersistenceReason is a stable diagnostic code without raw values or errors.
type PersistenceReason string

const (
	PersistenceEqualValues         PersistenceReason = "explicit_values_match"
	PersistenceDifferentValues     PersistenceReason = "explicit_values_differ"
	PersistenceDefaultUnresolved   PersistenceReason = "restart_default_unresolved"
	PersistenceUnsupportedScope    PersistenceReason = "unsupported_scope"
	PersistencePDBUnsupported      PersistenceReason = "pdb_dictionary_persistence_unresolved"
	PersistenceSIDUnverified       PersistenceReason = "instance_sid_unverified"
	PersistenceStartupUnverified   PersistenceReason = "startup_configuration_unverified"
	PersistencePFileUnsupported    PersistenceReason = "pfile_configuration_unresolved"
	PersistenceIncompleteSource    PersistenceReason = "source_not_complete"
	PersistenceFilteredSource      PersistenceReason = "source_filtered"
	PersistenceStaleEvidence       PersistenceReason = "evidence_stale"
	PersistenceFutureEvidence      PersistenceReason = "evidence_from_future"
	PersistenceEmptySource         PersistenceReason = "source_empty"
	PersistenceMalformedEvidence   PersistenceReason = "malformed_evidence"
	PersistenceConflictingEvidence PersistenceReason = "conflicting_evidence"
	PersistenceAdjustedValue       PersistenceReason = "adjusted_value"
	PersistenceAmbiguousValue      PersistenceReason = "value_equivalence_unresolved"
	PersistenceUnknownType         PersistenceReason = "parameter_type_unresolved"
	PersistenceMissingEffective    PersistenceReason = "effective_value_missing"
	PersistenceContainerMismatch   PersistenceReason = "container_applicability_unresolved"
)

// PersistenceScope is admitted connected scope, never acquired by this package.
type PersistenceScope string

const (
	PersistenceNonCDB  PersistenceScope = "non_cdb"
	PersistenceCDBRoot PersistenceScope = "cdb_root"
	PersistencePDB     PersistenceScope = "pdb"
)

// PersistenceStartupKind identifies the caller-admitted startup mechanism;
// selecting SPFILE alone does not verify its use for these observations.
type PersistenceStartupKind string

const (
	PersistenceSPFile         PersistenceStartupKind = "spfile"
	PersistencePFile          PersistenceStartupKind = "pfile"
	PersistenceStartupUnknown PersistenceStartupKind = "unverified"
)

// PersistenceEvidence is the caller's explicit transport/coverage admission.
// Complete requires successful unfiltered reading of the selected member source;
// a decoded Result alone cannot establish EOF, privileges or freshness.
type PersistenceEvidence struct {
	Complete   bool      `json:"complete"`
	ObservedAt time.Time `json:"observed_at"`
}

// PersistenceStartup admits the observed current SPFILE configuration. A path
// alone is not verification; the caller must admit its association with these
// observations. No host file is read and no future operator choice is assumed.
type PersistenceStartup struct {
	Kind       PersistenceStartupKind `json:"kind"`
	SPFilePath session.Value          `json:"spfile_path"`
	Verified   bool                   `json:"verified"`
	ObservedAt time.Time              `json:"observed_at"`
}

// PersistenceMember supplies independent complete evidence for one reporting ID.
// SIDVerified admits the actual stored-clause identity; INST_ID is never its SID.
type PersistenceMember struct {
	InstanceID        int64
	SID               string
	SIDVerified       bool
	Scope             PersistenceScope
	Startup           PersistenceStartup
	Effective         EffectiveResult
	Stored            SPFileResult
	EffectiveEvidence PersistenceEvidence
	StoredEvidence    PersistenceEvidence
}

// PersistenceRequest sets the explicit comparison time and positive maximum
// evidence age for independent members. Callers retain the inputs and their
// admission; evaluation reads no source and does not invent a current time.
type PersistenceRequest struct {
	At      time.Time
	MaxAge  time.Duration
	Members []PersistenceMember
}

// PersistenceFinding owns snapshots of the ordered effective and applicable
// stored rows. Original complete raw datasets, including dormant clauses, remain
// caller-owned. No inferred default or normalized replacement value is emitted.
type PersistenceFinding struct {
	Name          string            `json:"name"`
	ContainerID   *session.Value    `json:"container_id,omitempty"`
	State         PersistenceState  `json:"state"`
	Reason        PersistenceReason `json:"reason"`
	ApplicableSID string            `json:"applicable_sid,omitempty"`
	Effective     []Parameter       `json:"effective"`
	Stored        []SPFileParameter `json:"stored"`
}

// PersistenceMemberResult owns one member's findings and copied source provenance.
// A nonempty Reason marks failed admission and prevents persistence conclusions
// for that member; independently valid members remain represented.
type PersistenceMemberResult struct {
	InstanceID          int64                `json:"instance_id"`
	SID                 string               `json:"sid"`
	Scope               PersistenceScope     `json:"scope"`
	Reason              PersistenceReason    `json:"reason,omitempty"`
	EffectiveProvenance Provenance           `json:"effective_provenance"`
	StoredProvenance    Provenance           `json:"stored_provenance"`
	Findings            []PersistenceFinding `json:"findings"`
}

// PersistenceReport owns conditional comparison snapshots at At, with no promise
// about a future startup choice or configuration change. Canceled evaluations
// return no report rather than a partial member prefix.
type PersistenceReport struct {
	At      time.Time                 `json:"at"`
	Members []PersistenceMemberResult `json:"members"`
}

// ErrInvalidPersistenceRequest rejects a missing comparison time, nonpositive age,
// invalid member identity or malformed metadata encoding. Unavailable admitted
// observations instead retain explicit member/finding reasons.
var ErrInvalidPersistenceRequest = errors.New("invalid parameter persistence request")

// PersistenceJSON evaluates the same supplied evidence as bounded standard JSON.
// MaxRows counts members, findings and retained effective/stored value rows; the
// byte limit includes the complete encoded report. Any failure returns nil JSON.
// Raw query rows/JSON remain in the existing admitted-reader APIs.
func PersistenceJSON(ctx context.Context, request PersistenceRequest, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	total := 0
	add := func(n int) bool {
		if n > resolved.MaxRows-total {
			return false
		}
		total += n
		return true
	}
	if !add(len(request.Members)) {
		return nil, rowset.ErrCapacity
	}
	// Bound retained input before evaluation can allocate snapshots. Input and
	// output accounting are independent; neither may hide a truncated prefix.
	for _, member := range request.Members {
		if !add(len(member.Effective.Rows)) || !add(len(member.Stored.Rows)) {
			return nil, rowset.ErrCapacity
		}
	}
	report, err := EvaluatePersistence(ctx, request)
	if err != nil {
		return nil, err
	}
	total = 0
	if !add(len(report.Members)) {
		return nil, rowset.ErrCapacity
	}
	for _, member := range report.Members {
		if !add(len(member.Findings)) {
			return nil, rowset.ErrCapacity
		}
		for _, finding := range member.Findings {
			if !add(len(finding.Effective)) || !add(len(finding.Stored)) {
				return nil, rowset.ErrCapacity
			}
		}
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if len(payload) > resolved.MaxBytes {
		return nil, rowset.ErrCapacity
	}
	return payload, nil
}
