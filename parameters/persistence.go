package parameters

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-session/session"
)

// EvaluatePersistence compares complete current and stored observations without
// reading a database/file or changing any input. Different is conditional on the
// same admitted startup configuration; missing stored values never mean loss.
// Invalid request shape/cancellation returns no report; unavailable/malformed
// member evidence stays unknown without erasing independently valid members.
func EvaluatePersistence(ctx context.Context, request PersistenceRequest) (PersistenceReport, error) {
	if err := ctx.Err(); err != nil {
		return PersistenceReport{}, err
	}
	if request.At.IsZero() || request.MaxAge <= 0 || len(request.Members) == 0 {
		return PersistenceReport{}, ErrInvalidPersistenceRequest
	}
	seen := map[int64]bool{}
	for _, m := range request.Members {
		if m.InstanceID <= 0 || seen[m.InstanceID] || !utf8.ValidString(m.SID) || !utf8.ValidString(string(m.Scope)) || !validPersistenceProvenance(m.Effective.Provenance) || !validPersistenceProvenance(m.Stored.Provenance) {
			return PersistenceReport{}, ErrInvalidPersistenceRequest
		}
		seen[m.InstanceID] = true
	}
	report := PersistenceReport{At: request.At, Members: make([]PersistenceMemberResult, 0, len(request.Members))}
	for _, m := range request.Members {
		if err := ctx.Err(); err != nil {
			return PersistenceReport{}, err
		}
		result, err := evaluatePersistenceMember(ctx, request, m)
		if err != nil {
			return PersistenceReport{}, err
		}
		report.Members = append(report.Members, result)
	}
	if err := ctx.Err(); err != nil {
		return PersistenceReport{}, err
	}
	return report, nil
}

type persistenceKey struct{ name, container string }
type persistenceGroup struct {
	container *session.Value
	effective []Parameter
	stored    []SPFileParameter
}

func evaluatePersistenceMember(ctx context.Context, request PersistenceRequest, m PersistenceMember) (PersistenceMemberResult, error) {
	result := PersistenceMemberResult{InstanceID: m.InstanceID, SID: m.SID, Scope: m.Scope, EffectiveProvenance: clonePersistenceProvenance(m.Effective.Provenance), StoredProvenance: clonePersistenceProvenance(m.Stored.Provenance), Findings: make([]PersistenceFinding, 0)}
	result.Reason = admitPersistenceMember(request, m)
	groups, reason, err := persistenceGroups(ctx, m)
	if err != nil {
		return result, err
	}
	if reason != "" {
		result.Reason = reason
		return result, nil
	}
	keys := make([]persistenceKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name == keys[j].name {
			return keys[i].container < keys[j].container
		}
		return keys[i].name < keys[j].name
	})
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		g := groups[key]
		finding := PersistenceFinding{Name: key.name, ContainerID: copySystemValue(g.container), State: PersistenceUnknown, Reason: result.Reason, Effective: clonePersistenceEffective(g.effective), Stored: clonePersistenceStored(g.stored)}
		if finding.Reason == "" {
			finding = evaluatePersistenceGroup(m, key, g, groups, finding)
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, nil
}
func admitPersistenceMember(request PersistenceRequest, m PersistenceMember) PersistenceReason {
	switch m.Scope {
	case PersistencePDB:
		return PersistencePDBUnsupported
	case PersistenceNonCDB, PersistenceCDBRoot:
	default:
		return PersistenceUnsupportedScope
	}
	if m.SIDVerified && (m.SID == "" || m.SID == "*" || validate(nil, nil, []string{m.SID}) != nil) {
		return PersistenceSIDUnverified
	}
	if m.Startup.Kind == PersistencePFile {
		return PersistencePFileUnsupported
	}
	if m.Startup.Kind != PersistenceSPFile || !m.Startup.Verified || !validPersistenceValue(m.Startup.SPFilePath) || m.Startup.SPFilePath.Null || strings.TrimSpace(m.Startup.SPFilePath.Text) == "" {
		return PersistenceStartupUnverified
	}
	if !m.EffectiveEvidence.Complete || !m.StoredEvidence.Complete {
		return PersistenceIncompleteSource
	}
	for _, at := range []time.Time{m.Startup.ObservedAt, m.EffectiveEvidence.ObservedAt, m.StoredEvidence.ObservedAt} {
		if at.IsZero() {
			return PersistenceIncompleteSource
		}
		if at.After(request.At) {
			return PersistenceFutureEvidence
		}
		if request.At.Sub(at) > request.MaxAge {
			return PersistenceStaleEvidence
		}
	}
	ep, sp := m.Effective.Provenance, m.Stored.Provenance
	if ep.Source != "GV$SYSTEM_PARAMETER2" || sp.Source != "GV$SPPARAMETER" || ep.OracleMajor < 11 || ep.OracleMajor != sp.OracleMajor || (m.Scope == PersistenceCDBRoot && ep.OracleMajor < 12) {
		return PersistenceMalformedEvidence
	}
	expected := "non_cdb_visible_instance_rows"
	if ep.OracleMajor >= 12 {
		expected = "current_container_and_whole_database_rows"
	}
	if ep.Scope != expected || sp.Scope != expected {
		return PersistenceUnsupportedScope
	}
	for _, p := range []Provenance{ep, sp} {
		ids := map[int64]bool{}
		for _, id := range p.Instances {
			if id <= 0 || ids[id] {
				return PersistenceMalformedEvidence
			}
			ids[id] = true
		}
		if len(p.Names) > 0 || len(p.SIDs) > 0 || p.NameClass != "" || p.NameContains != "" {
			return PersistenceFilteredSource
		}
		if len(p.Instances) > 0 && !slices.Contains(p.Instances, m.InstanceID) {
			return PersistenceFilteredSource
		}
	}
	if sp.SIDPolicy != "all_stored_clauses" {
		return PersistenceFilteredSource
	}
	if len(m.Effective.Rows) == 0 || len(m.Stored.Rows) == 0 {
		return PersistenceEmptySource
	}
	return ""
}
func persistenceGroups(ctx context.Context, m PersistenceMember) (map[persistenceKey]*persistenceGroup, PersistenceReason, error) {
	groups := map[persistenceKey]*persistenceGroup{}
	group := func(name string, container *session.Value, major int) (*persistenceGroup, bool) {
		if validate([]string{name}, nil, nil) != nil {
			return nil, false
		}
		key, ok := persistenceScopeKey(container, major, m.Scope)
		if !ok {
			return nil, false
		}
		k := persistenceKey{name, key}
		g := groups[k]
		if g == nil {
			g = &persistenceGroup{container: container}
			groups[k] = g
		}
		return g, true
	}
	currentCount, storedCount := 0, 0
	for _, row := range m.Effective.Rows {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		id, ok := persistenceReportingID(row.InstanceID)
		if !ok || !validPersistenceParameter(row) {
			return nil, PersistenceMalformedEvidence, nil
		}
		if len(m.Effective.Provenance.Instances) > 0 && !slices.Contains(m.Effective.Provenance.Instances, id) {
			return nil, PersistenceMalformedEvidence, nil
		}
		if id != m.InstanceID {
			continue
		}
		currentCount++
		g, ok := group(row.Name.Text, row.ContainerID, m.Effective.Provenance.OracleMajor)
		if row.Name.Null || !ok {
			return nil, PersistenceMalformedEvidence, nil
		}
		g.effective = append(g.effective, row)
	}
	for _, row := range m.Stored.Rows {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		id, ok := persistenceReportingID(row.InstanceID)
		if !ok || !validPersistenceSPFile(row) {
			return nil, PersistenceMalformedEvidence, nil
		}
		if len(m.Stored.Provenance.Instances) > 0 && !slices.Contains(m.Stored.Provenance.Instances, id) {
			return nil, PersistenceMalformedEvidence, nil
		}
		if id != m.InstanceID {
			continue
		}
		storedCount++
		// Dormant SIDs remain in the caller's raw dataset. A stored-only name is a
		// candidate here only when the clause could apply to the admitted member.
		key, ok := persistenceScopeKey(row.ContainerID, m.Stored.Provenance.OracleMajor, m.Scope)
		if !ok || row.Name.Null || validate([]string{row.Name.Text}, nil, nil) != nil {
			return nil, PersistenceMalformedEvidence, nil
		}
		k := persistenceKey{row.Name.Text, key}
		if m.SIDVerified && groups[k] == nil && !row.SID.Null && row.SID.Text != "*" && row.SID.Text != m.SID {
			continue
		}
		g, ok := group(row.Name.Text, row.ContainerID, m.Stored.Provenance.OracleMajor)
		if !ok {
			return nil, PersistenceMalformedEvidence, nil
		}
		g.stored = append(g.stored, row)
	}
	if (currentCount == 0 || storedCount == 0) && len(m.Effective.Rows) > 0 && len(m.Stored.Rows) > 0 {
		return groups, PersistenceEmptySource, nil
	}
	return groups, "", nil
}
func persistenceContainerKey(value *session.Value, major int) (string, bool) {
	if major < 12 {
		return "legacy", value == nil
	}
	if value == nil || !validPersistenceValue(*value) {
		return "", false
	}
	if value.Null {
		return "null", true
	}
	n, ok := persistenceOrdinal(value.Text)
	if !ok {
		return "", false
	}
	return strconv.FormatInt(n, 10), true
}
func evaluatePersistenceGroup(m PersistenceMember, key persistenceKey, g *persistenceGroup, groups map[persistenceKey]*persistenceGroup, f PersistenceFinding) PersistenceFinding {
	unknown := func(reason PersistenceReason) PersistenceFinding {
		f.State = PersistenceUnknown
		f.Reason = reason
		return f
	}
	if (m.Scope == PersistenceNonCDB && key.container != "legacy" && key.container != "0") || (m.Scope == PersistenceCDBRoot && key.container != string(PersistenceCDBRoot)) {
		return unknown(PersistenceContainerMismatch)
	}
	if m.Scope == PersistenceCDBRoot && persistenceRootAmbiguous(m, g) {
		return unknown(PersistenceConflictingEvidence)
	}
	if len(g.effective) == 0 {
		return unknown(PersistenceMissingEffective)
	}
	if !orderPersistenceEffective(f.Effective) {
		return unknown(PersistenceConflictingEvidence)
	}
	kind := int64(-1)
	for _, row := range f.Effective {
		if row.Type.Null {
			return unknown(PersistenceUnknownType)
		}
		t, err := strconv.ParseInt(row.Type.Text, 10, 64)
		if err != nil || t < 1 || t > 6 || t == 5 {
			return unknown(PersistenceUnknownType)
		}
		if kind != -1 && kind != t {
			return unknown(PersistenceConflictingEvidence)
		}
		kind = t
		if row.IsAdjusted.Null || row.IsAdjusted.Text != "FALSE" {
			return unknown(PersistenceAdjustedValue)
		}
	}
	// An absent stored definition does not make malformed/ambiguous current
	// evidence usable. Admit scalar cardinality and current values first (#50).
	if kind != 2 && kind != 4 && len(f.Effective) != 1 {
		return unknown(PersistenceAmbiguousValue)
	}
	for _, row := range f.Effective {
		if row.Value.Null {
			return unknown(PersistenceAmbiguousValue)
		}
		if _, known := comparePersistenceValue(kind, row.Value.Text, row.Value.Text); !known {
			return unknown(PersistenceAmbiguousValue)
		}
	}
	if len(g.stored) == 0 {
		for other, peer := range groups {
			if other.name == key.name && other.container != key.container && len(peer.stored) > 0 {
				return unknown(PersistenceContainerMismatch)
			}
			if other.name != key.name && strings.EqualFold(other.name, key.name) {
				return unknown(PersistenceAmbiguousValue)
			}
		}
	}
	selected, sid, reason := selectPersistenceStored(g.stored, m.SID, m.SIDVerified)
	if reason != "" {
		return unknown(reason)
	}
	f.ApplicableSID = sid
	if len(selected) == 0 {
		f.State = PersistenceNotStored
		f.Reason = PersistenceDefaultUnresolved
		return f
	}
	f.Stored = clonePersistenceStored(selected)
	if !orderPersistenceStored(f.Stored) {
		return unknown(PersistenceConflictingEvidence)
	}
	storedType := ""
	for _, row := range f.Stored {
		if row.Type.Null || row.Type.Text == "" {
			return unknown(PersistenceUnknownType)
		}
		if storedType != "" && storedType != row.Type.Text {
			return unknown(PersistenceConflictingEvidence)
		}
		storedType = row.Type.Text
	}
	if len(f.Effective) != len(f.Stored) || (kind != 2 && kind != 4 && len(f.Effective) != 1) {
		return unknown(PersistenceAmbiguousValue)
	}
	different := false
	for i, row := range f.Effective {
		currentOrdinal, _ := persistenceOrdinal(row.Ordinal.Text)
		storedOrdinal, _ := persistenceOrdinal(f.Stored[i].Ordinal.Text)
		if ((kind == 2 || kind == 4) && currentOrdinal != storedOrdinal) || row.Value.Null || f.Stored[i].Value.Null {
			return unknown(PersistenceAmbiguousValue)
		}
		equal, known := comparePersistenceValue(kind, row.Value.Text, f.Stored[i].Value.Text)
		if !known {
			return unknown(PersistenceAmbiguousValue)
		}
		different = different || !equal
	}
	f.State, f.Reason = PersistenceMatch, PersistenceEqualValues
	if different {
		f.State, f.Reason = PersistenceDifferent, PersistenceDifferentValues
	}
	return f
}
func selectPersistenceStored(rows []SPFileParameter, sid string, verified bool) ([]SPFileParameter, string, PersistenceReason) {
	for _, row := range rows {
		if row.SID.Null || row.SID.Text == "" {
			return nil, "", PersistenceSIDUnverified
		}
		if !verified && row.SID.Text != "*" && (row.IsSpecified.Null || row.IsSpecified.Text != "FALSE") {
			return nil, "", PersistenceSIDUnverified
		}
		if verified && row.SID.Text != sid && strings.EqualFold(row.SID.Text, sid) {
			return nil, "", PersistenceSIDUnverified
		}
	}
	candidates := []string{"*"}
	if verified {
		candidates = []string{sid, "*"}
	}
	for _, candidate := range candidates {
		selected := make([]SPFileParameter, 0)
		unspecified := false
		seen := map[int64]bool{}
		for _, row := range rows {
			if row.SID.Text != candidate {
				continue
			}
			ordinal, ok := persistenceOrdinal(row.Ordinal.Text)
			if row.Ordinal.Null || !ok || seen[ordinal] {
				return nil, "", PersistenceConflictingEvidence
			}
			seen[ordinal] = true
			if row.IsSpecified.Null {
				return nil, "", PersistenceConflictingEvidence
			}
			switch row.IsSpecified.Text {
			case "TRUE":
				selected = append(selected, row)
			case "FALSE":
				unspecified = true
			default:
				return nil, "", PersistenceConflictingEvidence
			}
		}
		if len(selected) > 0 {
			if unspecified {
				return nil, "", PersistenceConflictingEvidence
			}
			return selected, candidate, ""
		}
	}
	return nil, "", ""
}
func orderPersistenceEffective(rows []Parameter) bool {
	seen := map[int64]bool{}
	for _, r := range rows {
		n, ok := persistenceOrdinal(r.Ordinal.Text)
		if r.Ordinal.Null || !ok || seen[n] {
			return false
		}
		seen[n] = true
	}
	sort.Slice(rows, func(i, j int) bool {
		a, _ := persistenceOrdinal(rows[i].Ordinal.Text)
		b, _ := persistenceOrdinal(rows[j].Ordinal.Text)
		return a < b
	})
	return true
}
func orderPersistenceStored(rows []SPFileParameter) bool {
	seen := map[int64]bool{}
	for _, r := range rows {
		n, ok := persistenceOrdinal(r.Ordinal.Text)
		if r.Ordinal.Null || !ok || seen[n] {
			return false
		}
		seen[n] = true
	}
	sort.Slice(rows, func(i, j int) bool {
		a, _ := persistenceOrdinal(rows[i].Ordinal.Text)
		b, _ := persistenceOrdinal(rows[j].Ordinal.Text)
		return a < b
	})
	return true
}
func persistenceReportingID(value session.Value) (int64, bool) {
	if !validPersistenceValue(value) || value.Null {
		return 0, false
	}
	n, ok := persistenceOrdinal(value.Text)
	return n, ok && n > 0
}
func validPersistenceValue(v session.Value) bool {
	return utf8.ValidString(v.Text) && (!v.Null || v.Text == "")
}
func validPersistenceParameter(r Parameter) bool {
	for _, v := range []session.Value{r.InstanceID, r.Number, r.Name, r.Type, r.Ordinal, r.Value, r.DisplayValue, r.IsDefault, r.IsSessionModifiable, r.IsSystemModifiable, r.IsInstanceModifiable, r.IsModified, r.IsAdjusted, r.IsDeprecated, r.IsBasic, r.Description, r.UpdateComment} {
		if !validPersistenceValue(v) {
			return false
		}
	}
	return (r.ContainerID == nil || validPersistenceValue(*r.ContainerID)) && (r.IsPDBModifiable == nil || validPersistenceValue(*r.IsPDBModifiable))
}
func validPersistenceSPFile(r SPFileParameter) bool {
	for _, v := range []session.Value{r.InstanceID, r.SID, r.Name, r.Type, r.Ordinal, r.Value, r.DisplayValue, r.IsSpecified, r.UpdateComment} {
		if !validPersistenceValue(v) {
			return false
		}
	}
	return r.ContainerID == nil || validPersistenceValue(*r.ContainerID)
}
func clonePersistenceProvenance(p Provenance) Provenance {
	p.Names = slices.Clone(p.Names)
	p.Instances = slices.Clone(p.Instances)
	p.SIDs = slices.Clone(p.SIDs)
	return p
}
func clonePersistenceEffective(rows []Parameter) []Parameter {
	result := slices.Clone(rows)
	for i := range result {
		result[i].ContainerID = copySystemValue(result[i].ContainerID)
		result[i].IsPDBModifiable = copySystemValue(result[i].IsPDBModifiable)
	}
	return result
}
func clonePersistenceStored(rows []SPFileParameter) []SPFileParameter {
	result := slices.Clone(rows)
	for i := range result {
		result[i].ContainerID = copySystemValue(result[i].ContainerID)
	}
	return result
}

func validPersistenceProvenance(p Provenance) bool {
	for _, v := range []string{p.Source, p.Scope, p.SIDPolicy, p.NameClass, p.NameContains} {
		if !utf8.ValidString(v) {
			return false
		}
	}
	for _, list := range [][]string{p.Names, p.SIDs} {
		for _, v := range list {
			if !utf8.ValidString(v) {
				return false
			}
		}
	}
	return true
}

// Oracle CON_ID 0 covers the entire CDB, including its root; CON_ID 1 denotes
// only the root. Align these observations solely for an admitted Oracle 12c-or-later CDB-root
// comparison, never by rewriting raw IDs or broadening PDB/non-CDB evidence.
// Each source must still have an unambiguous applicable root definition (#50).
func persistenceScopeKey(value *session.Value, major int, scope PersistenceScope) (string, bool) {
	key, ok := persistenceContainerKey(value, major)
	if ok && major >= 12 && scope == PersistenceCDBRoot && (key == "0" || key == "1") {
		return string(PersistenceCDBRoot), true
	}
	return key, ok
}
func persistenceRootAmbiguous(m PersistenceMember, g *persistenceGroup) bool {
	scopes := map[string]bool{}
	for _, row := range g.effective {
		key, ok := persistenceContainerKey(row.ContainerID, m.Effective.Provenance.OracleMajor)
		if !ok || (key != "0" && key != "1") {
			return true
		}
		scopes[key] = true
	}
	if len(scopes) > 1 {
		return true
	}
	clear(scopes)
	for _, row := range g.stored {
		// A verified unrelated SID is dormant for this member. Every wildcard,
		// matching or unverified clause remains potentially applicable, so competing
		// 0/1 observations cannot silently gain container or SID precedence.
		if m.SIDVerified && !row.SID.Null && row.SID.Text != "*" && row.SID.Text != m.SID && !strings.EqualFold(row.SID.Text, m.SID) {
			continue
		}
		key, ok := persistenceContainerKey(row.ContainerID, m.Stored.Provenance.OracleMajor)
		if !ok || (key != "0" && key != "1") {
			return true
		}
		scopes[key] = true
	}
	return len(scopes) > 1
}
