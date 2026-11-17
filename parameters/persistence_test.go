package parameters_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func pv(text string) session.Value { return session.Value{Text: text} }
func persistenceFixture() parameters.PersistenceRequest {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	container := pv("1")
	current := parameters.Parameter{InstanceID: pv("7"), ContainerID: &container, Number: pv("1"), Name: pv("pga_aggregate_target"), Type: pv("6"), Ordinal: pv("1"), Value: pv("1073741824"), DisplayValue: pv("1G"), IsDefault: pv("FALSE"), IsAdjusted: pv("FALSE")}
	stored := parameters.SPFileParameter{InstanceID: pv("7"), ContainerID: &container, SID: pv("*"), Name: current.Name, Type: pv("native-type"), Ordinal: pv("1"), Value: pv("1G"), DisplayValue: pv("1G"), IsSpecified: pv("TRUE")}
	return parameters.PersistenceRequest{At: now, MaxAge: time.Minute, Members: []parameters.PersistenceMember{{InstanceID: 7, SID: "ORCL7", SIDVerified: true, Scope: parameters.PersistenceCDBRoot, Startup: parameters.PersistenceStartup{Kind: parameters.PersistenceSPFile, SPFilePath: pv("+DATA/spfile.ora"), Verified: true, ObservedAt: now}, Effective: parameters.EffectiveResult{Provenance: parameters.Provenance{Source: "GV$SYSTEM_PARAMETER2", OracleMajor: 19, Scope: "current_container_and_whole_database_rows", Instances: []int64{7}}, Rows: []parameters.Parameter{current}}, Stored: parameters.SPFileResult{Provenance: parameters.Provenance{Source: "GV$SPPARAMETER", OracleMajor: 19, Scope: "current_container_and_whole_database_rows", Instances: []int64{7}, SIDPolicy: "all_stored_clauses"}, Rows: []parameters.SPFileParameter{stored}}, EffectiveEvidence: parameters.PersistenceEvidence{Complete: true, ObservedAt: now}, StoredEvidence: parameters.PersistenceEvidence{Complete: true, ObservedAt: now}}}}
}
func evaluateOne(t *testing.T, request parameters.PersistenceRequest) parameters.PersistenceFinding {
	t.Helper()
	report, err := parameters.EvaluatePersistence(t.Context(), request)
	if err != nil || len(report.Members) != 1 || len(report.Members[0].Findings) != 1 {
		t.Fatal("missing member/finding", report, err)
	}
	return report.Members[0].Findings[0]
}
func TestEvaluatePersistenceSIDAndExactValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*parameters.PersistenceMember)
		want   parameters.PersistenceState
		sid    string
	}{
		{"wildcard", nil, parameters.PersistenceMatch, "*"},
		{"override", func(m *parameters.PersistenceMember) {
			r := m.Stored.Rows[0]
			r.SID = pv("ORCL7")
			r.Value = pv("2G")
			m.Stored.Rows = append(m.Stored.Rows, r)
		}, parameters.PersistenceDifferent, "ORCL7"},
		{"dormant", func(m *parameters.PersistenceMember) {
			r := m.Stored.Rows[0]
			r.SID = pv("OTHER")
			r.Value = pv("2G")
			m.Stored.Rows = append(m.Stored.Rows, r)
		}, parameters.PersistenceMatch, "*"},
		{"false_override", func(m *parameters.PersistenceMember) {
			r := m.Stored.Rows[0]
			r.SID = pv("ORCL7")
			r.IsSpecified = pv("FALSE")
			r.Value = session.Value{Null: true}
			m.Stored.Rows = append(m.Stored.Rows, r)
		}, parameters.PersistenceMatch, "*"},
		{"missing_explicit_default", func(m *parameters.PersistenceMember) {
			m.Stored.Rows[0].IsSpecified = pv("FALSE")
			m.Stored.Rows[0].Value = session.Value{Null: true}
		}, parameters.PersistenceNotStored, ""},
		{"integer_above_float", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("6")
			m.Effective.Rows[0].Value = pv("9007199254740993")
			m.Stored.Rows[0].Value = pv("9007199254740992")
		}, parameters.PersistenceDifferent, "*"},
		{"boolean", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("1")
			m.Effective.Rows[0].Value = pv("TRUE")
			m.Stored.Rows[0].Value = pv("true")
		}, parameters.PersistenceMatch, "*"},
		{"string_exact", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("2")
			m.Effective.Rows[0].Value = pv("a,b")
			m.Stored.Rows[0].Value = pv("a,b")
		}, parameters.PersistenceMatch, "*"},
		{"string_case", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("2")
			m.Effective.Rows[0].Value = pv("Case")
			m.Stored.Rows[0].Value = pv("case")
		}, parameters.PersistenceUnknown, "*"},
		{"numeric_expression", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].Value = pv("2*512M") }, parameters.PersistenceUnknown, "*"},
		{"unsupported_unit", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].Value = pv("1TB") }, parameters.PersistenceUnknown, "*"},
		{"null", func(m *parameters.PersistenceMember) { m.Effective.Rows[0].Value = session.Value{Null: true} }, parameters.PersistenceUnknown, ""},
		{"adjusted", func(m *parameters.PersistenceMember) { m.Effective.Rows[0].IsAdjusted = pv("TRUE") }, parameters.PersistenceUnknown, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := persistenceFixture()
			if tc.change != nil {
				tc.change(&request.Members[0])
			}
			got := evaluateOne(t, request)
			if got.State != tc.want || got.ApplicableSID != tc.sid {
				t.Fatal(got)
			}
		})
	}
}
func TestEvaluatePersistenceListsAreWholeOrderedAndImmutable(t *testing.T) {
	r := persistenceFixture()
	m := &r.Members[0]
	m.Effective.Rows[0].Type = pv("2")
	m.Effective.Rows[0].Value = pv("a,b")
	m.Stored.Rows[0].Value = pv("a,b")
	e := m.Effective.Rows[0]
	e.Ordinal = pv("2")
	e.Value = pv("second")
	m.Effective.Rows = append([]parameters.Parameter{e}, m.Effective.Rows...)
	s := m.Stored.Rows[0]
	s.Ordinal = pv("2")
	s.Value = pv("second")
	m.Stored.Rows = append(m.Stored.Rows, s)
	before, _ := json.Marshal(r)
	got := evaluateOne(t, r)
	after, _ := json.Marshal(r)
	if got.State != parameters.PersistenceMatch || len(got.Effective) != 2 || got.Effective[0].Ordinal.Text != "1" || got.Stored[1].Value.Text != "second" || string(before) != string(after) {
		t.Fatal("list or input changed", got)
	}
	got.Effective[0].ContainerID.Text = "99"
	got.Stored[0].ContainerID.Text = "99"
	got.Effective[0].Value.Text = "mutated"
	if r.Members[0].Effective.Rows[1].ContainerID.Text != "1" || r.Members[0].Stored.Rows[0].ContainerID.Text != "1" || r.Members[0].Effective.Rows[1].Value.Text != "a,b" {
		t.Fatal("output aliases input")
	}
	m.Stored.Rows[1].Ordinal = pv("3")
	if evaluateOne(t, r).State != parameters.PersistenceUnknown {
		t.Fatal("unmatched ordinal guessed")
	}
}
func TestEvaluatePersistenceAdmissionNeverPassesMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*parameters.PersistenceRequest)
	}{
		{"pdb", func(r *parameters.PersistenceRequest) { r.Members[0].Scope = parameters.PersistencePDB }},
		{"pfile", func(r *parameters.PersistenceRequest) { r.Members[0].Startup.Kind = parameters.PersistencePFile }},
		{"startup_unverified", func(r *parameters.PersistenceRequest) { r.Members[0].Startup.Verified = false }},
		{"sid_unverified", func(r *parameters.PersistenceRequest) {
			r.Members[0].SIDVerified = false
			named := r.Members[0].Stored.Rows[0]
			named.SID = pv("OTHER")
			r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, named)
		}},
		{"failed", func(r *parameters.PersistenceRequest) { r.Members[0].StoredEvidence.Complete = false }},
		{"empty", func(r *parameters.PersistenceRequest) { r.Members[0].Stored.Rows = nil }},
		{"stale", func(r *parameters.PersistenceRequest) {
			r.Members[0].StoredEvidence.ObservedAt = r.At.Add(-2 * time.Minute)
		}},
		{"future", func(r *parameters.PersistenceRequest) {
			r.Members[0].EffectiveEvidence.ObservedAt = r.At.Add(time.Second)
		}},
		{"filtered", func(r *parameters.PersistenceRequest) {
			r.Members[0].Stored.Provenance.Names = []string{"pga_aggregate_target"}
		}},
		{"version_mismatch", func(r *parameters.PersistenceRequest) { r.Members[0].Stored.Provenance.OracleMajor = 11 }},
		{"null_container", func(r *parameters.PersistenceRequest) {
			*r.Members[0].Effective.Rows[0].ContainerID = session.Value{Null: true}
		}},
		{"wrong_container", func(r *parameters.PersistenceRequest) { *r.Members[0].Effective.Rows[0].ContainerID = pv("3") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := persistenceFixture()
			tc.change(&r)
			report, err := parameters.EvaluatePersistence(t.Context(), r)
			if err != nil || len(report.Members) != 1 {
				t.Fatal(report, err)
			}
			member := report.Members[0]
			if len(member.Findings) == 0 {
				if member.Reason == "" {
					t.Fatal("missing explicit unknown member reason", member)
				}
				return
			}
			for _, got := range member.Findings {
				if got.State != parameters.PersistenceUnknown || got.Reason == "" {
					t.Fatal("unadmitted source passed", got)
				}
			}
		})
	}
}
func TestEvaluatePersistenceRejectsConflictingOrdinals(t *testing.T) {
	for _, stored := range []bool{false, true} {
		r := persistenceFixture()
		if stored {
			copy := r.Members[0].Stored.Rows[0]
			copy.Ordinal = pv("01")
			r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, copy)
		} else {
			copy := r.Members[0].Effective.Rows[0]
			copy.Ordinal = pv("01")
			r.Members[0].Effective.Rows = append(r.Members[0].Effective.Rows, copy)
		}
		if evaluateOne(t, r).State != parameters.PersistenceUnknown {
			t.Fatal("duplicate identity passed")
		}
	}
}
func TestEvaluatePersistenceRetainsIndependentMembersAndProvenance(t *testing.T) {
	r := persistenceFixture()
	failed := r.Members[0]
	failed.InstanceID = 8
	failed.SID = "ORCL8"
	failed.Effective = parameters.EffectiveResult{}
	failed.Stored = parameters.SPFileResult{}
	failed.EffectiveEvidence.Complete = false
	r.Members = append(r.Members, failed)
	report, err := parameters.EvaluatePersistence(t.Context(), r)
	if err != nil || len(report.Members) != 2 || report.Members[0].Findings[0].State != parameters.PersistenceMatch || report.Members[1].Reason == "" {
		t.Fatal(report, err)
	}
	if !reflect.DeepEqual(report.Members[0].EffectiveProvenance, r.Members[0].Effective.Provenance) {
		t.Fatal("provenance changed")
	}
	report.Members[0].EffectiveProvenance.Instances[0] = 999
	if r.Members[0].Effective.Provenance.Instances[0] != 7 {
		t.Fatal("provenance aliases input")
	}
}
func TestPersistenceJSONAndCancellationAreAtomic(t *testing.T) {
	r := persistenceFixture()
	payload, err := parameters.PersistenceJSON(t.Context(), r, rowset.Limits{MaxRows: 100, MaxBytes: 32768})
	if err != nil || !json.Valid(payload) || !strings.Contains(string(payload), "matches") {
		t.Fatal(string(payload), err)
	}
	payload, err = parameters.PersistenceJSON(t.Context(), r, rowset.Limits{MaxRows: 1, MaxBytes: 32768})
	if !errors.Is(err, rowset.ErrCapacity) || payload != nil {
		t.Fatal("row budget", err)
	}
	payload, err = parameters.PersistenceJSON(t.Context(), r, rowset.Limits{MaxRows: 100, MaxBytes: 10})
	if !errors.Is(err, rowset.ErrCapacity) || payload != nil {
		t.Fatal("byte budget", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	report, err := parameters.EvaluatePersistence(ctx, r)
	if !errors.Is(err, context.Canceled) || report.Members != nil {
		t.Fatal(report, err)
	}
	payload, err = parameters.PersistenceJSON(ctx, r, rowset.Limits{})
	if !errors.Is(err, context.Canceled) || payload != nil {
		t.Fatal(err)
	}
	r.MaxAge = 0
	if _, err = parameters.EvaluatePersistence(t.Context(), r); err == nil {
		t.Fatal("invalid freshness budget accepted")
	}
}

func TestEvaluatePersistenceWildcardWithoutVerifiedSID(t *testing.T) {
	r := persistenceFixture()
	r.Members[0].SID = ""
	r.Members[0].SIDVerified = false
	if f := evaluateOne(t, r); f.State != parameters.PersistenceMatch || f.ApplicableSID != "*" {
		t.Fatal("wildcard unnecessarily depends on SID proof", f)
	}
	named := r.Members[0].Stored.Rows[0]
	named.SID = pv("OTHER")
	named.Value = pv("2G")
	r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, named)
	if f := evaluateOne(t, r); f.State != parameters.PersistenceUnknown || f.Reason != parameters.PersistenceSIDUnverified {
		t.Fatal("unverified named applicability guessed", f)
	}
	r.Members[0].Stored.Rows[1].IsSpecified = pv("FALSE")
	if f := evaluateOne(t, r); f.State != parameters.PersistenceMatch {
		t.Fatal("unspecified named clause overrode wildcard", f)
	}
}
func TestEvaluatePersistenceConflictsAndAdjustmentsPrecedeMissingDefaults(t *testing.T) {
	for _, mode := range []string{"adjusted", "effective_duplicate", "stored_duplicate"} {
		r := persistenceFixture()
		r.Members[0].Stored.Rows[0].IsSpecified = pv("FALSE")
		switch mode {
		case "adjusted":
			r.Members[0].Effective.Rows[0].IsAdjusted = pv("TRUE")
		case "effective_duplicate":
			r.Members[0].Effective.Rows = append(r.Members[0].Effective.Rows, r.Members[0].Effective.Rows[0])
		case "stored_duplicate":
			r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, r.Members[0].Stored.Rows[0])
		}
		if f := evaluateOne(t, r); f.State != parameters.PersistenceUnknown {
			t.Fatal(mode, f)
		}
	}
}

func TestEvaluatePersistenceScopeAndRequestContradictions(t *testing.T) {
	for _, which := range []string{"effective", "stored"} {
		r := persistenceFixture()
		if which == "effective" {
			row := r.Members[0].Effective.Rows[0]
			row.InstanceID = pv("8")
			r.Members[0].Effective.Rows = append(r.Members[0].Effective.Rows, row)
		} else {
			row := r.Members[0].Stored.Rows[0]
			row.InstanceID = pv("8")
			r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, row)
		}
		report, err := parameters.EvaluatePersistence(t.Context(), r)
		if err != nil || len(report.Members) != 1 || report.Members[0].Reason != parameters.PersistenceMalformedEvidence || len(report.Members[0].Findings) != 0 {
			t.Fatal("out-of-filter member row passed", which, report, err)
		}
	}
	r := persistenceFixture()
	r.Members[0].SID = string([]byte{255})
	if report, err := parameters.EvaluatePersistence(t.Context(), r); err == nil || report.Members != nil {
		t.Fatal("invalid UTF8 request can be normalized by JSON", report, err)
	}
	r = persistenceFixture()
	r.Members = append(r.Members, r.Members[0])
	if report, err := parameters.EvaluatePersistence(t.Context(), r); err == nil || report.Members != nil {
		t.Fatal("duplicate admitted member accepted", report, err)
	}
}
func TestEvaluatePersistenceScalarOrdinalsAndLegacyScope(t *testing.T) {
	r := persistenceFixture()
	r.Members[0].Stored.Rows[0].Ordinal = pv("0")
	if f := evaluateOne(t, r); f.State != parameters.PersistenceMatch {
		t.Fatal("scalar ordinal was treated as list identity", f)
	}
	r = persistenceFixture()
	m := &r.Members[0]
	m.Scope = parameters.PersistenceNonCDB
	m.Effective.Provenance.OracleMajor = 11
	m.Stored.Provenance.OracleMajor = 11
	m.Effective.Provenance.Scope = "non_cdb_visible_instance_rows"
	m.Stored.Provenance.Scope = "non_cdb_visible_instance_rows"
	m.Effective.Rows[0].ContainerID = nil
	m.Stored.Rows[0].ContainerID = nil
	if f := evaluateOne(t, r); f.State != parameters.PersistenceMatch || f.ContainerID != nil {
		t.Fatal("legacy absence fabricated", f)
	}
}

func TestEvaluatePersistenceNullFlagsTypesAndExactBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*parameters.PersistenceMember)
		want   parameters.PersistenceState
	}{
		{"uint64_exact", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Value = pv("18446744073709551615")
			m.Stored.Rows[0].Value = pv("18446744073709551614")
		}, parameters.PersistenceDifferent},
		{"integer_literal", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("3")
			m.Effective.Rows[0].Value = pv("1")
			m.Stored.Rows[0].Value = pv("01")
		}, parameters.PersistenceMatch},
		{"byte_units", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Value = pv("1048576")
			m.Stored.Rows[0].Value = pv("1024 KB")
		}, parameters.PersistenceMatch},
		{"unknown_flag", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].IsSpecified = pv("FUTURE") }, parameters.PersistenceUnknown},
		{"null_flag", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].IsSpecified = session.Value{Null: true} }, parameters.PersistenceUnknown},
		{"stored_null", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].Value = session.Value{Null: true} }, parameters.PersistenceUnknown},
		{"reserved_type", func(m *parameters.PersistenceMember) { m.Effective.Rows[0].Type = pv("5") }, parameters.PersistenceUnknown},
		{"unicode_boolean", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("1")
			m.Effective.Rows[0].Value = pv("FALSE")
			m.Stored.Rows[0].Value = pv("falſe")
		}, parameters.PersistenceUnknown},
		{"mixed_specified", func(m *parameters.PersistenceMember) {
			row := m.Stored.Rows[0]
			row.Ordinal = pv("2")
			row.IsSpecified = pv("FALSE")
			m.Stored.Rows = append(m.Stored.Rows, row)
		}, parameters.PersistenceUnknown},
		{"empty_not_default", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Type = pv("2")
			m.Effective.Rows[0].Value = pv("")
			m.Stored.Rows[0].Value = session.Value{Null: true}
		}, parameters.PersistenceUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := persistenceFixture()
			tc.change(&r.Members[0])
			if f := evaluateOne(t, r); f.State != tc.want {
				t.Fatal(f)
			}
		})
	}
	r := persistenceFixture()
	r.Members[0].EffectiveEvidence.ObservedAt = r.At.Add(-r.MaxAge)
	if f := evaluateOne(t, r); f.State != parameters.PersistenceMatch {
		t.Fatal("inclusive age boundary rejected", f)
	}
}

func TestEvaluatePersistenceAmbiguousCurrentPrecedesMissingDefaults(t *testing.T) {
	for _, mode := range []string{"multiple_scalar_values", "null_current", "unsupported_current"} {
		t.Run(mode, func(t *testing.T) {
			r := persistenceFixture()
			r.Members[0].Stored.Rows[0].IsSpecified = pv("FALSE")
			switch mode {
			case "multiple_scalar_values":
				row := r.Members[0].Effective.Rows[0]
				row.Ordinal = pv("2")
				r.Members[0].Effective.Rows = append(r.Members[0].Effective.Rows, row)
			case "null_current":
				r.Members[0].Effective.Rows[0].Value = session.Value{Null: true}
			case "unsupported_current":
				r.Members[0].Effective.Rows[0].Value = pv("unknown-expression")
			}
			if f := evaluateOne(t, r); f.State != parameters.PersistenceUnknown {
				t.Fatal("ambiguous current evidence became default conclusion", mode, f)
			}
		})
	}
}

func TestEvaluatePersistenceNameContainsIsFilteredEvidence(t *testing.T) {
	baseline := persistenceFixture()
	baseline.Members[0].Stored.Rows[0].Value = pv("2G")
	if f := evaluateOne(t, baseline); f.State != parameters.PersistenceDifferent {
		t.Fatal("control does not exercise a provable explicit difference", f)
	}
	for _, source := range []string{"effective", "stored"} {
		for _, specified := range []bool{true, false} {
			t.Run(source+map[bool]string{true: "/explicit", false: "/unspecified"}[specified], func(t *testing.T) {
				r := persistenceFixture()
				r.Members[0].Stored.Rows[0].Value = pv("2G")
				if !specified {
					r.Members[0].Stored.Rows[0].IsSpecified = pv("FALSE")
				}
				if source == "effective" {
					r.Members[0].Effective.Provenance.NameContains = "pga"
				} else {
					r.Members[0].Stored.Provenance.NameContains = "pga"
				}
				before, _ := json.Marshal(r)
				report, err := parameters.EvaluatePersistence(t.Context(), r)
				after, _ := json.Marshal(r)
				if err != nil || len(report.Members) != 1 || report.Members[0].Reason != parameters.PersistenceFilteredSource || len(report.Members[0].Findings) != 1 || report.Members[0].Findings[0].State != parameters.PersistenceUnknown || string(before) != string(after) {
					t.Fatal("substring-filtered source acquired a persistence conclusion", report, err)
				}
				payload, err := parameters.PersistenceJSON(t.Context(), r, rowset.Limits{MaxRows: 100, MaxBytes: 32768})
				if err != nil {
					t.Fatal(err)
				}
				var decoded parameters.PersistenceReport
				if err = json.Unmarshal(payload, &decoded); err != nil || len(decoded.Members) != 1 || decoded.Members[0].Reason != parameters.PersistenceFilteredSource || decoded.Members[0].Findings[0].State != parameters.PersistenceUnknown {
					t.Fatal("JSON bypassed source filter admission", string(payload), err)
				}
				provenance := decoded.Members[0].EffectiveProvenance
				if source == "stored" {
					provenance = decoded.Members[0].StoredProvenance
				}
				if provenance.NameContains != "pga" {
					t.Fatal("raw filter provenance lost", provenance)
				}
			})
		}
	}
}
func TestPersistenceNameContainsRejectsInvalidEncoding(t *testing.T) {
	r := persistenceFixture()
	r.Members[0].Stored.Provenance.NameContains = string([]byte{255})
	if report, err := parameters.EvaluatePersistence(t.Context(), r); !errors.Is(err, parameters.ErrInvalidPersistenceRequest) || report.Members != nil {
		t.Fatal("invalid metadata encoding accepted", report, err)
	}
	if payload, err := parameters.PersistenceJSON(t.Context(), r, rowset.Limits{}); !errors.Is(err, parameters.ErrInvalidPersistenceRequest) || payload != nil {
		t.Fatal("JSON normalized invalid raw filter metadata", string(payload), err)
	}
}

func persistenceRootNativeShape() parameters.PersistenceRequest {
	r := persistenceFixture()
	zero, one := pv("0"), pv("1")
	m := &r.Members[0]
	m.Effective.Rows[0].ContainerID = &zero
	m.Stored.Rows[0].ContainerID = &one
	return r
}
func TestEvaluatePersistenceAdmittedRootWholeAndRootRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*parameters.PersistenceMember)
		want   parameters.PersistenceState
	}{
		{"numeric_match", nil, parameters.PersistenceMatch},
		{"modified_but_stored", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Name = pv("log_archive_dest_state_2")
			m.Stored.Rows[0].Name = m.Effective.Rows[0].Name
			m.Effective.Rows[0].Type = pv("2")
			m.Stored.Rows[0].Type = pv("string")
			m.Effective.Rows[0].Value = pv("ENABLE")
			m.Effective.Rows[0].IsModified = pv("MODIFIED")
			m.Stored.Rows[0].Value = pv("ENABLE")
		}, parameters.PersistenceMatch},
		{"compatible", func(m *parameters.PersistenceMember) {
			m.Effective.Rows[0].Name = pv("compatible")
			m.Stored.Rows[0].Name = m.Effective.Rows[0].Name
			m.Effective.Rows[0].Type = pv("2")
			m.Stored.Rows[0].Type = pv("string")
			m.Effective.Rows[0].Value = pv("23.6.0")
			m.Stored.Rows[0].Value = pv("23.6.0")
		}, parameters.PersistenceMatch},
		{"numeric_difference", func(m *parameters.PersistenceMember) { m.Stored.Rows[0].Value = pv("2G") }, parameters.PersistenceDifferent},
		{"sid_override", func(m *parameters.PersistenceMember) {
			row := m.Stored.Rows[0]
			row.SID = pv(m.SID)
			row.Value = pv("2G")
			m.Stored.Rows = append(m.Stored.Rows, row)
		}, parameters.PersistenceDifferent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := persistenceRootNativeShape()
			if tc.change != nil {
				tc.change(&r.Members[0])
			}
			before, _ := json.Marshal(r)
			report, err := parameters.EvaluatePersistence(t.Context(), r)
			after, _ := json.Marshal(r)
			if err != nil || len(report.Members) != 1 || len(report.Members[0].Findings) != 1 {
				t.Fatal("root applicability fragmented", report, err)
			}
			f := report.Members[0].Findings[0]
			if f.State != tc.want || f.ContainerID == nil || f.ContainerID.Text != "0" || f.Effective[0].ContainerID.Text != "0" || f.Stored[0].ContainerID.Text != "1" || string(before) != string(after) {
				t.Fatal("root applicability/raw scope changed", f, err)
			}
			payload, err := parameters.PersistenceJSON(t.Context(), r, rowset.Limits{MaxRows: 100, MaxBytes: 32768})
			if err != nil || !json.Valid(payload) {
				t.Fatal("root JSON failed", err)
			}
			var parsed parameters.PersistenceReport
			if err = json.Unmarshal(payload, &parsed); err != nil || len(parsed.Members[0].Findings) != 1 || parsed.Members[0].Findings[0].State != tc.want || parsed.Members[0].Findings[0].Stored[0].ContainerID.Text != "1" {
				t.Fatal("root JSON lost original scoped rows", err)
			}
		})
	}
}
func TestEvaluatePersistenceRootScopeAmbiguityStaysUnknown(t *testing.T) {
	for _, mode := range []string{"both_effective", "both_stored", "competing_named_and_wildcard", "split_list"} {
		t.Run(mode, func(t *testing.T) {
			r := persistenceRootNativeShape()
			zero, one := pv("0"), pv("1")
			m := &r.Members[0]
			switch mode {
			case "both_effective":
				row := m.Effective.Rows[0]
				row.ContainerID = &one
				m.Effective.Rows = append(m.Effective.Rows, row)
			case "both_stored":
				row := m.Stored.Rows[0]
				row.ContainerID = &zero
				m.Stored.Rows = append(m.Stored.Rows, row)
			case "competing_named_and_wildcard":
				row := m.Stored.Rows[0]
				row.ContainerID = &zero
				row.SID = pv(m.SID)
				m.Stored.Rows = append(m.Stored.Rows, row)
			case "split_list":
				m.Effective.Rows[0].Type = pv("2")
				m.Effective.Rows[0].Value = pv("first")
				m.Stored.Rows[0].Value = pv("first")
				e := m.Effective.Rows[0]
				e.Ordinal = pv("2")
				e.Value = pv("second")
				e.ContainerID = &one
				m.Effective.Rows = append(m.Effective.Rows, e)
				s := m.Stored.Rows[0]
				s.Ordinal = pv("2")
				s.Value = pv("second")
				m.Stored.Rows = append(m.Stored.Rows, s)
			}
			report, err := parameters.EvaluatePersistence(t.Context(), r)
			if err != nil || len(report.Members) != 1 || len(report.Members[0].Findings) != 1 || report.Members[0].Findings[0].State != parameters.PersistenceUnknown || report.Members[0].Findings[0].Reason == "" {
				t.Fatal("competing0/1 source selected a winner", mode, report, err)
			}
		})
	}
}
func TestEvaluatePersistenceRootMappingDoesNotWidenOtherScopes(t *testing.T) {
	for _, scope := range []parameters.PersistenceScope{parameters.PersistencePDB, parameters.PersistenceNonCDB, ""} {
		r := persistenceRootNativeShape()
		r.Members[0].Scope = scope
		report, err := parameters.EvaluatePersistence(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range report.Members {
			for _, f := range m.Findings {
				if f.State != parameters.PersistenceUnknown {
					t.Fatal("root applicability escaped admitted root", scope, f)
				}
			}
		}
	}
	r := persistenceRootNativeShape()
	three := pv("3")
	r.Members[0].Stored.Rows[0].ContainerID = &three
	report, err := parameters.EvaluatePersistence(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Members[0].Findings {
		if f.State != parameters.PersistenceUnknown {
			t.Fatal("root widened to PDB clause", f)
		}
	}
}
func TestEvaluatePersistenceRootDormantScopeDoesNotOverride(t *testing.T) {
	r := persistenceRootNativeShape()
	zero := pv("0")
	row := r.Members[0].Stored.Rows[0]
	row.ContainerID = &zero
	row.SID = pv("OTHER")
	row.Value = pv("4G")
	r.Members[0].Stored.Rows = append(r.Members[0].Stored.Rows, row)
	report, err := parameters.EvaluatePersistence(t.Context(), r)
	if err != nil || len(report.Members[0].Findings) != 1 || report.Members[0].Findings[0].State != parameters.PersistenceMatch {
		t.Fatal("dormant scope changed applicable root clause", report, err)
	}
	if len(r.Members[0].Stored.Rows) != 2 {
		t.Fatal("dormant raw row erased")
	}
}
