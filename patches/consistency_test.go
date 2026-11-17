package patches

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-session/session"
	"testing"
	"time"
)

func consistencyValue(s string) *session.Value { v := coreText(s); return &v }
func consistencyPatch(id, at, kind, action, status, target string) SQLPatch {
	return SQLPatch{PatchID: coreText(id), PatchUID: coreText(id), Action: coreText(action), Status: coreText(status), ActionTime: coreText("2026-11-15T" + at + ".000000"), Description: coreText("description must not determine level"), Flags: coreText("N"), PatchType: consistencyValue(kind), TargetVersion: consistencyValue(target)}
}
func consistencyRequest() ConsistencyRequest {
	at := time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC)
	return ConsistencyRequest{At: at, MaxAge: time.Hour, CoverageVerified: true, SelfManaged: true,
		VersionsEvidence: ConsistencyEvidence{Complete: true, ObservedAt: at},
		Versions:         instances.VersionResult{State: instances.Available, Precision: instances.FullPrecision, Provenance: instances.Provenance{Source: "GV$INSTANCE", OracleMajor: 19, Scope: "current_container_and_whole_database_qualified_instance_versions"}, Rows: []instances.VersionRow{{InstanceID: coreText("1"), ContainerID: consistencyValue("0"), CurrentContainerID: consistencyValue("1"), InstanceNumber: coreText("1"), Status: coreText("OPEN"), Version: coreText("19.0.0.0.0"), VersionFull: consistencyValue("19.32.0.0.0")}}},
		Scopes:           []SQLPatchScope{{ContainerID: coreText("1"), Verified: true, Evidence: ConsistencyEvidence{Complete: true, ObservedAt: at}, Patches: SQLPatchResult{Provenance: sqlPatchProvenance(SQLPatchOptions{OracleMajor: 19}), Rows: []SQLPatch{consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0")}}}}}
}
func TestConsistencyEffectiveAttemptsAndReleaseSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rows   []SQLPatch
		want   ConsistencyState
		reason ConsistencyReason
	}{
		{"equal", nil, ConsistencyMatches, ConsistencyEqualRelease},
		{"mismatch", []SQLPatch{consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.31.0.0.0")}, ConsistencyMismatch, ConsistencyDifferentRelease},
		{"numeric-not-lexical", []SQLPatch{consistencyPatch("100", "00:00:00", "RU", "APPLY", "SUCCESS", "19.9.0.0.0"), consistencyPatch("2", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0")}, ConsistencyMatches, ConsistencyEqualRelease},
		{"new-interim-not-release", []SQLPatch{consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0"), consistencyPatch("200", "02:00:00", "INTERIM", "APPLY", "SUCCESS", "19.0.0.0.0")}, ConsistencyMatches, ConsistencyEqualRelease},
		{"retry-repaired", []SQLPatch{consistencyPatch("100", "00:00:00", "RU", "APPLY", "WITH ERRORS", "19.32.0.0.0"), consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0")}, ConsistencyMatches, ConsistencyEqualRelease},
		{"older-ru-error-superseded", []SQLPatch{consistencyPatch("99", "00:00:00", "RU", "APPLY", "WITH ERRORS", "19.31.0.0.0"), consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0")}, ConsistencyMatches, ConsistencyEqualRelease},
		{"unrepaired", []SQLPatch{consistencyPatch("100", "00:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0"), consistencyPatch("100", "01:00:00", "RU", "APPLY", "WITH ERRORS", "19.32.0.0.0")}, ConsistencySQLErrors, ConsistencyLatestErrors},
		{"rollback-removes", []SQLPatch{consistencyPatch("100", "00:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0"), consistencyPatch("100", "01:00:00", "RU", "ROLLBACK", "SUCCESS", "19.32.0.0.0")}, ConsistencyUnknown, ConsistencyRolledBack},
		{"ties", []SQLPatch{consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.32.0.0.0"), consistencyPatch("100", "01:00:00", "RU", "ROLLBACK", "SUCCESS", "19.32.0.0.0")}, ConsistencyUnknown, ConsistencyAmbiguous},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := consistencyRequest()
			if tc.rows != nil {
				r.Scopes[0].Patches.Rows = tc.rows
			}
			got, err := EvaluateConsistency(t.Context(), r)
			if err != nil || len(got.Findings) != 1 || got.Findings[0].State != tc.want || got.Findings[0].Reason != tc.reason {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
func TestConsistencyUnknownBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ConsistencyRequest)
		reason ConsistencyReason
	}{
		{"coverage", func(r *ConsistencyRequest) { r.CoverageVerified = false }, ConsistencyUnverifiedCoverage},
		{"managed", func(r *ConsistencyRequest) { r.SelfManaged = false }, ConsistencyManaged},
		{"scope", func(r *ConsistencyRequest) { r.Scopes[0].Verified = false }, ConsistencyUnverifiedScope},
		{"mounted", func(r *ConsistencyRequest) { r.Versions.Rows[0].Status = coreText("MOUNTED") }, ConsistencyMemberNotOpen},
		{"legacy", func(r *ConsistencyRequest) { r.Versions.Precision = instances.LegacyPrecision }, ConsistencyUnsupported},
		{"partial", func(r *ConsistencyRequest) { r.Scopes[0].Evidence.Complete = false }, ConsistencyIncomplete},
		{"stale", func(r *ConsistencyRequest) { r.Scopes[0].Evidence.ObservedAt = r.At.Add(-2 * time.Hour) }, ConsistencyStale},
		{"future", func(r *ConsistencyRequest) { r.VersionsEvidence.ObservedAt = r.At.Add(time.Second) }, ConsistencyFuture},
		{"full-null", func(r *ConsistencyRequest) { r.Versions.Rows[0].VersionFull = &session.Value{Null: true} }, ConsistencyBinaryUnavailable},
		{"unsupported-major", func(r *ConsistencyRequest) { r.Versions.Provenance.OracleMajor = 26 }, ConsistencyUnsupported},
		{"source", func(r *ConsistencyRequest) { r.Scopes[0].Patches.Provenance.Source = "other" }, ConsistencySourceMismatch},
		{"no-release", func(r *ConsistencyRequest) { r.Scopes[0].Patches.Rows = nil }, ConsistencyNoRelease},
		{"jvm-not-release", func(r *ConsistencyRequest) { r.Scopes[0].Patches.Rows[0].Flags = coreText("J") }, ConsistencyNoRelease},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := consistencyRequest()
			tc.change(&r)
			got, err := EvaluateConsistency(t.Context(), r)
			if err != nil || got.Findings[0].State != ConsistencyUnknown || got.Findings[0].Reason != tc.reason {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
func TestConsistencyEveryMemberScopeDetachedRawAnd26ai(t *testing.T) {
	r := consistencyRequest()
	r.Versions.Provenance.OracleMajor = 23
	r.Scopes[0].Patches.Provenance.OracleMajor = 23
	r.Versions.Rows[0].Version = coreText("23.0.0.0.0")
	r.Versions.Rows[0].VersionFull = consistencyValue("23.26.1.0.0")
	r.Scopes[0].Patches.Rows[0].TargetVersion = consistencyValue("23.26.1.0.0")
	r.Versions.Rows = append(r.Versions.Rows, r.Versions.Rows[0])
	r.Versions.Rows[1].InstanceID = coreText("2")
	r.Versions.Rows[1].VersionFull = consistencyValue("23.26.0.0.0")
	s := r.Scopes[0]
	s.ContainerID = coreText("3")
	r.Scopes = append(r.Scopes, s)
	before, _ := json.Marshal(r)
	got, err := EvaluateConsistency(t.Context(), r)
	after, _ := json.Marshal(r)
	if err != nil || len(got.Findings) != 4 || string(before) != string(after) {
		t.Fatal(got, err)
	}
	if got.Findings[0].State != ConsistencyMatches || got.Findings[1].State != ConsistencyMismatch {
		t.Fatal(got.Findings)
	}
	got.Versions.Rows[0].VersionFull.Text = "changed"
	got.Scopes[0].Patches.Rows[0].TargetVersion.Text = "changed"
	final, _ := json.Marshal(r)
	if string(final) != string(before) {
		t.Fatal("raw inputs aliased")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out, err := EvaluateConsistency(ctx, r)
	if !errors.Is(err, context.Canceled) || out.Findings != nil || out.Scopes != nil {
		t.Fatal("canceled report published", out, err)
	}
}

func TestConsistencySublevelsAndInvalidAdmissionNeverBecomeMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ConsistencyRequest)
		want   ConsistencyReason
	}{
		{"mrp-or-recut", func(r *ConsistencyRequest) { r.Versions.Rows[0].VersionFull = consistencyValue("19.32.0.1.0") }, ConsistencyUnsupported},
		{"cloud-known", func(r *ConsistencyRequest) { r.Versions.Rows[0].CloudService = consistencyValue("OLTP") }, ConsistencyManaged},
		{"cloud-future", func(r *ConsistencyRequest) { r.Versions.Rows[0].CloudService = consistencyValue("future") }, ConsistencyManaged},
		{"no-current-container", func(r *ConsistencyRequest) { r.Versions.Rows[0].CurrentContainerID = nil }, ConsistencyUnverifiedScope},
		{"different-execution-container", func(r *ConsistencyRequest) { r.Versions.Rows[0].CurrentContainerID = consistencyValue("3") }, ConsistencySourceMismatch},
		{"duplicate-numeric-member", func(r *ConsistencyRequest) {
			r.Versions.Rows = append(r.Versions.Rows, r.Versions.Rows[0])
			r.Versions.Rows[1].InstanceID = coreText("01")
		}, ConsistencyAmbiguous},
		{"latest-global-tie", func(r *ConsistencyRequest) {
			r.Scopes[0].Patches.Rows = append(r.Scopes[0].Patches.Rows, consistencyPatch("200", "01:00:00", "RU", "APPLY", "SUCCESS", "19.31.0.0.0"))
		}, ConsistencyAmbiguous},
		{"invalid-time", func(r *ConsistencyRequest) {
			r.Scopes[0].Patches.Rows[0].ActionTime = coreText("2026-02-30T01:00:00.000000")
		}, ConsistencyMalformed},
		{"unknown-patch-type", func(r *ConsistencyRequest) { r.Scopes[0].Patches.Rows[0].PatchType = consistencyValue("future") }, ConsistencyUnsupported},
		{"rollback-after-old-active", func(r *ConsistencyRequest) {
			r.Scopes[0].Patches.Rows = append([]SQLPatch{consistencyPatch("99", "00:00:00", "RU", "APPLY", "SUCCESS", "19.31.0.0.0")}, r.Scopes[0].Patches.Rows...)
			r.Scopes[0].Patches.Rows = append(r.Scopes[0].Patches.Rows, consistencyPatch("100", "02:00:00", "RU", "ROLLBACK", "SUCCESS", "19.32.0.0.0"))
		}, ConsistencyRolledBack},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := consistencyRequest()
			tc.change(&r)
			got, err := EvaluateConsistency(t.Context(), r)
			if err != nil || got.Findings[0].State != ConsistencyUnknown || got.Findings[0].Reason != tc.want {
				t.Fatalf("%+v %v", got.Findings, err)
			}
		})
	}
}

func TestConsistencyKeepsSuccessfulEmptyRawInventory(t *testing.T) {
	r := consistencyRequest()
	r.Scopes[0].Patches.Rows = []SQLPatch{}
	got, err := EvaluateConsistency(t.Context(), r)
	if err != nil || got.Scopes[0].Patches.Rows == nil || len(got.Scopes[0].Patches.Rows) != 0 || got.Findings[0].State != ConsistencyUnknown {
		t.Fatal("successful empty source changed", got, err)
	}
	for _, kind := range []string{"RUI", "RUR", "CU"} {
		r := consistencyRequest()
		r.Scopes[0].Patches.Rows[0].PatchType = consistencyValue(kind)
		got, err := EvaluateConsistency(t.Context(), r)
		if err != nil || got.Findings[0].State != ConsistencyMatches {
			t.Fatal(kind, got, err)
		}
	}
	r = consistencyRequest()
	r.Scopes[0].Patches.Rows[0].Description = coreText(string([]byte{255}))
	got, err = EvaluateConsistency(t.Context(), r)
	if !errors.Is(err, ErrInvalidConsistencyRequest) || got.Scopes != nil {
		t.Fatal("invalid UTF8 published", got, err)
	}
}

func TestConsistencyFailedRollbackIsNotRepairedByUnrelatedLowerApply(t *testing.T) {
	r := consistencyRequest()
	r.Versions.Rows[0].VersionFull = consistencyValue("19.31.0.0.0")
	failed := consistencyPatch("99", "00:00:00", "RU", "ROLLBACK", "WITH ERRORS", "19.3.0.0.0")
	failed.SourceVersion = consistencyValue("19.32.0.0.0")
	r.Scopes[0].Patches.Rows = []SQLPatch{failed, consistencyPatch("100", "01:00:00", "RU", "APPLY", "SUCCESS", "19.31.0.0.0")}
	report, err := EvaluateConsistency(t.Context(), r)
	if err != nil || report.Findings[0].State != ConsistencySQLErrors || len(report.Findings[0].FailedRows) != 1 || report.Findings[0].FailedRows[0] != 0 {
		t.Fatal("unrelated apply hid failed rollback", report.Findings, err)
	}
}

func TestConsistencyLatestReleaseIgnoresOlderTies(t *testing.T) {
	orders := []struct {
		name    string
		indexes [3]int
	}{
		{"ABC", [3]int{0, 1, 2}}, {"ACB", [3]int{0, 2, 1}}, {"BAC", [3]int{1, 0, 2}},
		{"BCA", [3]int{1, 2, 0}}, {"CAB", [3]int{2, 0, 1}}, {"CBA", [3]int{2, 1, 0}},
	}
	for _, latestTied := range []bool{false, true} {
		name, oldTime, newTime := "unique_latest", "00:00:00", "01:00:00"
		if latestTied {
			name, oldTime, newTime = "latest_tied", "01:00:00", "00:00:00"
		}
		rows := [3]SQLPatch{
			consistencyPatch("100", oldTime, "RU", "APPLY", "SUCCESS", "19.29.0.0.0"),
			consistencyPatch("200", oldTime, "RU", "APPLY", "SUCCESS", "19.30.0.0.0"),
			consistencyPatch("300", newTime, "RU", "APPLY", "SUCCESS", "19.31.0.0.0"),
		}
		for _, order := range orders {
			t.Run(name+"/"+order.name, func(t *testing.T) {
				request := consistencyRequest()
				request.Scopes[0].Patches.Rows = []SQLPatch{rows[order.indexes[0]], rows[order.indexes[1]], rows[order.indexes[2]]}
				before, err := json.Marshal(request.Scopes[0].Patches.Rows)
				if err != nil {
					t.Fatal(err)
				}
				report, err := EvaluateConsistency(t.Context(), request)
				if err != nil || len(report.Findings) != 1 {
					t.Fatal(report, err)
				}
				finding := report.Findings[0]
				if latestTied {
					if finding.State != ConsistencyUnknown || finding.Reason != ConsistencyAmbiguous {
						t.Fatal("latest tie was resolved", finding)
					}
				} else {
					if finding.State != ConsistencyMismatch || finding.Reason != ConsistencyDifferentRelease || finding.SQLTargetVersion == nil || finding.SQLTargetVersion.Text != "19.31.0.0.0" {
						t.Fatal("older tie hid latest release", finding)
					}
					if len(finding.CandidateRows) != 1 {
						t.Fatal("missing latest source index", finding)
					}
					index := finding.CandidateRows[0]
					if index < 0 || index >= len(report.Scopes[0].Patches.Rows) || report.Scopes[0].Patches.Rows[index].PatchID.Text != "300" {
						t.Fatal("candidate source index changed", finding)
					}
				}
				after, err := json.Marshal(request.Scopes[0].Patches.Rows)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(report.Scopes[0].Patches.Rows)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || string(before) != string(raw) {
					t.Fatal("raw source order changed")
				}
			})
		}
	}
}
