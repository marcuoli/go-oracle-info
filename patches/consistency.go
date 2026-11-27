package patches

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-session/session"
)

// EvaluateConsistency compares one database's independently admitted SQL release
// observations to each qualified visible member. It executes nothing. A mismatch
// is evidence to investigate incomplete patching, never proof that datapatch was
// omitted or authority to run it. Raw attempts remain intact in the report.
func EvaluateConsistency(ctx context.Context, request ConsistencyRequest) (ConsistencyReport, error) {
	return evaluateConsistency(ctx, request, false)
}

func evaluateConsistency(ctx context.Context, request ConsistencyRequest, root bool) (ConsistencyReport, error) {
	if err := ctx.Err(); err != nil {
		return ConsistencyReport{}, err
	}
	if request.At.IsZero() || request.MaxAge <= 0 || !validConsistencyText(request) {
		return ConsistencyReport{}, ErrInvalidConsistencyRequest
	}
	report := cloneConsistency(request)
	if len(request.Versions.Rows) == 0 {
		report.Reason = ConsistencyNoMembers
	}
	if len(request.Scopes) == 0 {
		report.Reason = ConsistencyNoScopes
	}
	for si, scope := range request.Scopes {
		if err := ctx.Err(); err != nil {
			return ConsistencyReport{}, err
		}
		level := releaseLevel{}
		scopeReason := scopeAdmission(request, scope, root)
		if scopeReason == "" {
			level = effectiveRelease(ctx, scope.Patches.Rows)
		}
		for mi, member := range request.Versions.Rows {
			if err := ctx.Err(); err != nil {
				return ConsistencyReport{}, err
			}
			finding := ConsistencyFinding{MemberIndex: mi, ScopeIndex: si, State: ConsistencyUnknown}
			reason := memberAdmission(request, member)
			if reason == "" {
				reason = scopeReason
			}
			if reason == "" {
				reason = memberScope(member, scope, root)
			}
			if reason == "" && ambiguousMember(request.Versions.Rows, mi) {
				reason = ConsistencyAmbiguous
			}
			if reason != "" {
				finding.Reason = reason
				report.Findings = append(report.Findings, finding)
				continue
			}
			finding.CandidateRows = append([]int(nil), level.candidates...)
			finding.FailedRows = append([]int(nil), level.failures...)
			finding.SQLTargetVersion = cloneConsistencyValue(level.target)
			finding.Reason = level.reason
			if level.reason == ConsistencyLatestErrors {
				finding.State = ConsistencySQLErrors
			} else if level.reason == "" {
				binary, _ := releaseVersion(member.VersionFull.Text)
				target, ok := releaseVersion(level.target.Text)
				if !ok || target[0] != binary[0] {
					finding.Reason = ConsistencyMalformed
				} else if target == binary {
					finding.ComparedComponents = 5
					finding.State = ConsistencyMatches
					finding.Reason = ConsistencyEqualRelease
				} else if target[1] == binary[1] && target[2] == binary[2] {
					// MRP/recut-only differences do not establish a missing SQL release transition.
					finding.Reason = ConsistencyUnsupported
				} else {
					finding.ComparedComponents = 3
					finding.State = ConsistencyMismatch
					finding.Reason = ConsistencyDifferentRelease
				}
			}
			report.Findings = append(report.Findings, finding)
		}
	}
	if err := ctx.Err(); err != nil {
		return ConsistencyReport{}, err
	}
	return report, nil
}

func freshConsistency(at time.Time, maxAge time.Duration, e ConsistencyEvidence) ConsistencyReason {
	if !e.Complete || e.ObservedAt.IsZero() {
		return ConsistencyIncomplete
	}
	if e.ObservedAt.After(at) {
		return ConsistencyFuture
	}
	if at.Sub(e.ObservedAt) > maxAge {
		return ConsistencyStale
	}
	return ""
}
func memberAdmission(r ConsistencyRequest, m instances.VersionRow) ConsistencyReason {
	if m.CloudService != nil && !m.CloudService.Null {
		return ConsistencyManaged
	}
	if !r.SelfManaged {
		return ConsistencyManaged
	}
	if !r.CoverageVerified {
		return ConsistencyUnverifiedCoverage
	}
	if reason := freshConsistency(r.At, r.MaxAge, r.VersionsEvidence); reason != "" {
		return reason
	}
	p := r.Versions.Provenance
	switch p.OracleMajor {
	case 18, 19, 21, 23:
	default:
		return ConsistencyUnsupported
	}
	if r.Versions.Precision != instances.FullPrecision {
		return ConsistencyUnsupported
	}
	if p.Source != "GV$INSTANCE" || p.Scope != "current_container_and_whole_database_qualified_instance_versions" {
		return ConsistencySourceMismatch
	}
	if r.Versions.State != instances.Available {
		return ConsistencyBinaryUnavailable
	}
	if m.Status.Null || m.Status.Text != "OPEN" {
		return ConsistencyMemberNotOpen
	}
	if _, ok := consistencyID(m.InstanceID, 1); !ok {
		return ConsistencyMalformed
	}
	if m.VersionFull == nil || m.VersionFull.Null {
		return ConsistencyBinaryUnavailable
	}
	full, ok := releaseVersion(m.VersionFull.Text)
	if !ok || full[0] != uint64(p.OracleMajor) {
		return ConsistencyMalformed
	}
	legacy, ok := releaseVersion(m.Version.Text)
	if m.Version.Null || !ok || legacy[0] != full[0] {
		return ConsistencyMalformed
	}
	return ""
}
func scopeAdmission(r ConsistencyRequest, s SQLPatchScope, root bool) ConsistencyReason {
	if !s.Verified {
		return ConsistencyUnverifiedScope
	}
	if _, ok := consistencyID(s.ContainerID, 0); !ok {
		return ConsistencyUnverifiedScope
	}
	if reason := freshConsistency(r.At, r.MaxAge, s.Evidence); reason != "" {
		return reason
	}
	p := s.Patches.Provenance
	source, scope := "DBA_REGISTRY_SQLPATCH", "current_container_datapatch_attempts"
	if root {
		source, scope = "CDB_REGISTRY_SQLPATCH", "cdb_root"
	}
	if p.Source != source || p.Scope != scope || p.OracleMajor != r.Versions.Provenance.OracleMajor || p.OracleMinor != 0 {
		return ConsistencySourceMismatch
	}
	return ""
}
func memberScope(m instances.VersionRow, s SQLPatchScope, root bool) ConsistencyReason {
	if m.CurrentContainerID == nil {
		return ConsistencyUnverifiedScope
	}
	current, ok := consistencyID(*m.CurrentContainerID, 0)
	containerID, valid := consistencyID(s.ContainerID, 0)
	if !ok || !valid {
		return ConsistencySourceMismatch
	}
	if root && (current != 1 || containerID < 2) || !root && current != containerID {
		return ConsistencySourceMismatch
	}
	if m.ContainerID == nil {
		return ConsistencySourceMismatch
	}
	id, ok := consistencyID(*m.ContainerID, 0)
	if !ok {
		return ConsistencySourceMismatch
	}
	// A PDB seen from root requires a whole-database binary observation.
	// A container-qualified member row cannot stand in for peer containers.
	if id != 0 && (root || id != containerID) {
		return ConsistencySourceMismatch
	}
	return ""
}
func ambiguousMember(rows []instances.VersionRow, index int) bool {
	id, ok := consistencyID(rows[index].InstanceID, 1)
	if !ok {
		return true
	}
	for i, row := range rows {
		if i != index {
			other, ok := consistencyID(row.InstanceID, 1)
			if ok && other == id {
				return true
			}
		}
	}
	return false
}
func consistencyID(v session.Value, min int64) (int64, bool) {
	n, err := strconv.ParseInt(v.Text, 10, 64)
	return n, !v.Null && err == nil && n >= min
}
func releaseVersion(text string) ([5]uint64, bool) {
	var version [5]uint64
	parts := strings.Split(text, ".")
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		if part == "" {
			return version, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return version, false
			}
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return version, false
		}
		version[i] = n
	}
	return version, true
}
func versionAtLeast(a, b [5]uint64) bool {
	if a[0] != b[0] {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

type releaseLevel struct {
	target               *session.Value
	candidates, failures []int
	reason               ConsistencyReason
}
type patchAttempt struct {
	index int
	tied  bool
}

func effectiveRelease(ctx context.Context, rows []SQLPatch) releaseLevel {
	latest := make(map[[2]int64]patchAttempt)
	keys := make([][2]int64, 0)
	for i, row := range rows {
		if ctx.Err() != nil {
			return releaseLevel{reason: ConsistencyIncomplete}
		}
		id, ok := consistencyID(row.PatchID, 1)
		uid, uidOK := consistencyID(row.PatchUID, 1)
		parsed, err := time.Parse("2006-01-02T15:04:05.000000", row.ActionTime.Text)
		if !ok || !uidOK || row.ActionTime.Null || err != nil || parsed.Format("2006-01-02T15:04:05.000000") != row.ActionTime.Text {
			return releaseLevel{reason: ConsistencyMalformed}
		}
		if row.Action.Null || (row.Action.Text != "APPLY" && row.Action.Text != "ROLLBACK") || row.Status.Null || (row.Status.Text != "SUCCESS" && row.Status.Text != "WITH ERRORS") {
			return releaseLevel{reason: ConsistencyMalformed}
		}
		if row.PatchType == nil || row.PatchType.Null {
			return releaseLevel{reason: ConsistencyUnsupported}
		}
		switch row.PatchType.Text {
		case "RU", "RUI", "RUR", "CU", "INTERIM":
		default:
			return releaseLevel{reason: ConsistencyUnsupported}
		}
		key := [2]int64{id, uid}
		prior, present := latest[key]
		if !present {
			keys = append(keys, key)
			latest[key] = patchAttempt{index: i}
			continue
		}
		before := rows[prior.index]
		if before.PatchType.Text != row.PatchType.Text {
			return releaseLevel{reason: ConsistencyAmbiguous}
		}
		if row.ActionTime.Text > before.ActionTime.Text {
			latest[key] = patchAttempt{index: i}
		} else if row.ActionTime.Text == before.ActionTime.Text {
			prior.tied = true
			latest[key] = prior
		}
	}
	selected := -1
	selectedTied := false
	rollback := -1
	for _, key := range keys {
		attempt := latest[key]
		if attempt.tied {
			return releaseLevel{reason: ConsistencyAmbiguous}
		}
		row := rows[attempt.index]
		if row.PatchType.Text == "INTERIM" {
			continue
		}
		if row.Flags.Null {
			return releaseLevel{reason: ConsistencyUnsupported}
		}
		if strings.Contains(row.Flags.Text, "J") {
			continue
		}
		if row.Status.Text != "SUCCESS" {
			continue
		}
		if row.Action.Text == "ROLLBACK" {
			if rollback < 0 || row.ActionTime.Text > rows[rollback].ActionTime.Text {
				rollback = attempt.index
			}
			continue
		}
		if selected < 0 || row.ActionTime.Text > rows[selected].ActionTime.Text {
			selected = attempt.index
			selectedTied = false
		} else if row.ActionTime.Text == rows[selected].ActionTime.Text {
			selectedTied = true
		}
	}
	// Only ties at the final maximum timestamp are ambiguous. A later release
	// supersedes an earlier tied pair regardless of source row order (#51).
	if selectedTied {
		return releaseLevel{reason: ConsistencyAmbiguous}
	}
	level := releaseLevel{}
	var selectedVersion [5]uint64
	if selected >= 0 {
		target := rows[selected].TargetVersion
		if target == nil || target.Null {
			return releaseLevel{reason: ConsistencyUnsupported}
		}
		var ok bool
		selectedVersion, ok = releaseVersion(target.Text)
		if !ok {
			return releaseLevel{reason: ConsistencyMalformed}
		}
		level.target = target
		level.candidates = []int{selected}
	}
	for _, key := range keys {
		i := latest[key].index
		row := rows[i]
		if row.Status.Text != "WITH ERRORS" {
			continue
		}
		// A later successful cumulative release can supersede an older failed release
		// APPLY transition. Failed rollbacks need their own successful retry; target
		// polarity cannot establish repair by an unrelated apply. No repair relation is
		// invented for an interim/JVM patch.
		repaired := false
		if selected >= 0 && row.Action.Text == "APPLY" && row.PatchType.Text != "INTERIM" && !row.Flags.Null && !strings.Contains(row.Flags.Text, "J") && row.ActionTime.Text < rows[selected].ActionTime.Text && row.TargetVersion != nil && !row.TargetVersion.Null {
			failedVersion, ok := releaseVersion(row.TargetVersion.Text)
			repaired = ok && versionAtLeast(selectedVersion, failedVersion)
		}
		if !repaired {
			level.failures = append(level.failures, i)
		}
	}
	if len(level.failures) > 0 {
		level.reason = ConsistencyLatestErrors
		return level
	}
	if rollback >= 0 && (selected < 0 || rows[rollback].ActionTime.Text >= rows[selected].ActionTime.Text) {
		level.reason = ConsistencyRolledBack
		return level
	}
	if selected < 0 {
		level.reason = ConsistencyNoRelease
	}
	return level
}

func validConsistencyText(r ConsistencyRequest) bool {
	valid := func(v session.Value) bool { return utf8.ValidString(v.Text) && (!v.Null || v.Text == "") }
	pointer := func(v *session.Value) bool { return v == nil || valid(*v) }
	p := r.Versions.Provenance
	if !utf8.ValidString(string(r.Versions.State)) || !utf8.ValidString(r.Versions.ContextSource) || !utf8.ValidString(string(r.Versions.Precision)) || !utf8.ValidString(p.Source) || !utf8.ValidString(p.Scope) {
		return false
	}
	for _, v := range r.Versions.Rows {
		for _, value := range []session.Value{v.InstanceID, v.InstanceNumber, v.Name, v.HostName, v.Status, v.Version, v.SessionUser, v.IsDBA} {
			if !valid(value) {
				return false
			}
		}
		if !pointer(v.ContainerID) || !pointer(v.VersionFull) || !pointer(v.CurrentContainerID) || !pointer(v.CloudService) {
			return false
		}
	}
	seen := make(map[int64]bool)
	for _, s := range r.Scopes {
		if !valid(s.ContainerID) || !utf8.ValidString(s.Patches.Provenance.Source) || !utf8.ValidString(s.Patches.Provenance.Scope) {
			return false
		}
		if id, ok := consistencyID(s.ContainerID, 0); s.Verified && ok {
			if seen[id] {
				return false
			}
			seen[id] = true
		}
		for _, name := range s.Patches.Provenance.OptionalColumns {
			if !utf8.ValidString(name) {
				return false
			}
		}
		for _, v := range s.Patches.Rows {
			for _, value := range []session.Value{v.PatchID, v.PatchUID, v.Action, v.Status, v.ActionTime, v.Description, v.Logfile, v.Flags} {
				if !valid(value) {
					return false
				}
			}
			for _, value := range []*session.Value{v.InstallID, v.Version, v.BundleSeries, v.BundleID, v.PatchType, v.SourceVersion, v.SourceBuildDescription, v.SourceBuildTimestamp, v.TargetVersion, v.TargetBuildDescription, v.TargetBuildTimestamp, v.RULogfile} {
				if !pointer(value) {
					return false
				}
			}
		}
	}
	return true
}
func cloneConsistencyValue(v *session.Value) *session.Value {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
func cloneConsistency(r ConsistencyRequest) ConsistencyReport {
	report := ConsistencyReport{At: r.At, Versions: r.Versions, Scopes: make([]SQLPatchScope, len(r.Scopes)), Findings: make([]ConsistencyFinding, 0)}
	report.Versions.Rows = slices.Clone(r.Versions.Rows)
	for i := range report.Versions.Rows {
		v := &report.Versions.Rows[i]
		v.ContainerID = cloneConsistencyValue(v.ContainerID)
		v.VersionFull = cloneConsistencyValue(v.VersionFull)
		v.CurrentContainerID = cloneConsistencyValue(v.CurrentContainerID)
		v.CloudService = cloneConsistencyValue(v.CloudService)
	}
	for i, s := range r.Scopes {
		report.Scopes[i] = s
		out := &report.Scopes[i].Patches
		out.Provenance.OptionalColumns = slices.Clone(s.Patches.Provenance.OptionalColumns)
		out.Rows = slices.Clone(s.Patches.Rows)
		for j := range out.Rows {
			v := &out.Rows[j]
			v.InstallID = cloneConsistencyValue(v.InstallID)
			v.Version = cloneConsistencyValue(v.Version)
			v.BundleSeries = cloneConsistencyValue(v.BundleSeries)
			v.BundleID = cloneConsistencyValue(v.BundleID)
			v.PatchType = cloneConsistencyValue(v.PatchType)
			v.SourceVersion = cloneConsistencyValue(v.SourceVersion)
			v.SourceBuildDescription = cloneConsistencyValue(v.SourceBuildDescription)
			v.SourceBuildTimestamp = cloneConsistencyValue(v.SourceBuildTimestamp)
			v.TargetVersion = cloneConsistencyValue(v.TargetVersion)
			v.TargetBuildDescription = cloneConsistencyValue(v.TargetBuildDescription)
			v.TargetBuildTimestamp = cloneConsistencyValue(v.TargetBuildTimestamp)
			v.RULogfile = cloneConsistencyValue(v.RULogfile)
		}
	}
	return report
}
