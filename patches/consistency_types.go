package patches

import (
	"errors"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-session/session"
	"time"
)

// ConsistencyState describes admitted SQL release-level evidence only. Even a
// match cannot certify binary inventory parity or all SQL-requiring one-offs.
type ConsistencyState string

const (
	ConsistencyMatches   ConsistencyState = "matches_release_level"
	ConsistencyMismatch  ConsistencyState = "release_level_mismatch"
	ConsistencySQLErrors ConsistencyState = "sql_patch_errors"
	ConsistencyUnknown   ConsistencyState = "unknown"
)

// ConsistencyReason records the evidence boundary behind a finding.
type ConsistencyReason string

const (
	ConsistencyEqualRelease       ConsistencyReason = "equal_release_level"
	ConsistencyDifferentRelease   ConsistencyReason = "different_release_level"
	ConsistencyLatestErrors       ConsistencyReason = "latest_attempt_errors"
	ConsistencyUnverifiedCoverage ConsistencyReason = "coverage_unverified"
	ConsistencyUnverifiedScope    ConsistencyReason = "scope_unverified"
	ConsistencyManaged            ConsistencyReason = "managed_service_unqualified"
	ConsistencyIncomplete         ConsistencyReason = "incomplete_source"
	ConsistencyStale              ConsistencyReason = "stale_evidence"
	ConsistencyFuture             ConsistencyReason = "future_evidence"
	ConsistencyUnsupported        ConsistencyReason = "unsupported_version_or_precision"
	ConsistencyBinaryUnavailable  ConsistencyReason = "binary_version_unavailable"
	ConsistencyMemberNotOpen      ConsistencyReason = "member_not_open"
	ConsistencyMalformed          ConsistencyReason = "malformed_evidence"
	ConsistencyAmbiguous          ConsistencyReason = "ambiguous_attempts_or_members"
	ConsistencyNoRelease          ConsistencyReason = "no_effective_release_apply"
	ConsistencyRolledBack         ConsistencyReason = "later_release_rollback"
	ConsistencySourceMismatch     ConsistencyReason = "source_or_scope_mismatch"
	ConsistencyNoMembers          ConsistencyReason = "no_members"
	ConsistencyNoScopes           ConsistencyReason = "no_scopes"
)

// ConsistencyEvidence admits complete collection and its observation instant.
// SQL ACTION_TIME remains a separate source-local civil timestamp.
type ConsistencyEvidence struct {
	Complete   bool
	ObservedAt time.Time
}

// SQLPatchScope identifies the actual caller-admitted dictionary container.
// Its ID is not inferred from a whole-database GV$INSTANCE CON_ID=0 row.
// Patches is the unchanged complete ReadSQLPatch/DecodeSQLPatch inventory.
type SQLPatchScope struct {
	ContainerID session.Value
	Verified    bool
	Patches     SQLPatchResult
	Evidence    ConsistencyEvidence
}

// ConsistencyRequest supplies independently admitted observations of one database.
// CoverageVerified means coverage of the caller's qualified visible/reachable set,
// never proof of physical RAC membership. SelfManaged is false for unqualified
// managed services. At and positive MaxAge make freshness explicit and testable.
type ConsistencyRequest struct {
	At               time.Time
	MaxAge           time.Duration
	Versions         instances.VersionResult
	VersionsEvidence ConsistencyEvidence
	CoverageVerified bool
	SelfManaged      bool
	Scopes           []SQLPatchScope
}

// ConsistencyFinding refers to raw report slices by index, retaining source order.
// CandidateRows identify effective release-level evidence; FailedRows identify
// latest unresolved recorded failures, not a diagnosis that datapatch was omitted.
type ConsistencyFinding struct {
	// ComparedComponents is5 for exact full-version equality,3 for a differing
	// RU/RUR component, and0 when sublevel semantics cannot be qualified.
	ComparedComponents int               `json:"compared_components"`
	MemberIndex        int               `json:"member_index"`
	ScopeIndex         int               `json:"scope_index"`
	State              ConsistencyState  `json:"state"`
	Reason             ConsistencyReason `json:"reason"`
	SQLTargetVersion   *session.Value    `json:"sql_target_version,omitempty"`
	CandidateRows      []int             `json:"candidate_rows,omitempty"`
	FailedRows         []int             `json:"failed_rows,omitempty"`
}

// ConsistencyReport owns detached raw observations and per-member/container facts.
// No aggregate success certifies an unobserved member/container or OPatch parity.
type ConsistencyReport struct {
	At       time.Time               `json:"at"`
	Reason   ConsistencyReason       `json:"reason,omitempty"`
	Versions instances.VersionResult `json:"versions"`
	Scopes   []SQLPatchScope         `json:"scopes"`
	Findings []ConsistencyFinding    `json:"findings"`
}

// ErrInvalidConsistencyRequest rejects invalid timing or malformed text/shape.
var ErrInvalidConsistencyRequest = errors.New("invalid Oracle patch consistency request")
