package parameters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func pdbOverrideFixture() parameters.PDBOverridesRequest {
	at := time.Date(2026, 11, 27, 15, 30, 0, 0, time.UTC)
	evidence := parameters.PersistenceEvidence{Complete: true, ObservedAt: at}
	con := &session.Value{Text: "4"}
	scope := "current_container_and_whole_database_rows"
	return parameters.PDBOverridesRequest{OracleVersion: "23.26.3.0.0", ContainerID: 4, At: at, MaxAge: time.Minute, RuntimePairVerified: true,
		Member: parameters.PersistenceMember{
			Scope: parameters.PersistencePDB, InstanceID: 1, EffectiveEvidence: evidence, StoredEvidence: evidence,
			Startup: parameters.PersistenceStartup{Kind: parameters.PersistenceSPFile, Verified: true, ObservedAt: at, SPFilePath: session.Value{Text: "spfile.ora"}},
			Effective: parameters.EffectiveResult{
				Provenance: parameters.Provenance{Source: "GV$SYSTEM_PARAMETER2", OracleMajor: 23, Scope: scope},
				Rows: []parameters.Parameter{{
					InstanceID: session.Value{Text: "1"}, ContainerID: con, Name: session.Value{Text: "open_cursors"},
					Ordinal: session.Value{Text: "1"}, Type: session.Value{Text: "3"}, Value: session.Value{Text: "733"},
					IsDefault: session.Value{Text: "FALSE"}, IsModified: session.Value{Text: "MODIFIED"},
					IsPDBModifiable: &session.Value{Text: "TRUE"},
				}},
			},
			Stored: parameters.SPFileResult{
				Provenance: parameters.Provenance{Source: "GV$SPPARAMETER", OracleMajor: 23, Scope: scope, SIDPolicy: "all_stored_clauses"},
				Rows: []parameters.SPFileParameter{{
					InstanceID: session.Value{Text: "1"}, ContainerID: con, Name: session.Value{Text: "open_cursors"},
					SID: session.Value{Text: "*"}, Ordinal: session.Value{Text: "0"}, Value: session.Value{Null: true},
					IsSpecified: session.Value{Text: "FALSE"},
				}},
			},
		},
	}
}

func TestPDBOverridesClassifyOnlyAdmittedEvidence(t *testing.T) {
	persistent := func(r *parameters.PDBOverridesRequest, value string) {
		row := &r.Member.Stored.Rows[0]
		row.Ordinal.Text, row.IsSpecified.Text = "1", "TRUE"
		row.Value = session.Value{Text: value}
	}
	for _, test := range []struct {
		name   string
		change func(*parameters.PDBOverridesRequest)
		want   parameters.PDBOverrideState
	}{
		{"memory-shaped snapshot", func(*parameters.PDBOverridesRequest) {}, parameters.PDBOverrideMemoryOnly},
		{"stored differs from current memory", func(r *parameters.PDBOverridesRequest) { persistent(r, "734") }, parameters.PDBOverridePersistent},
		{"both", func(r *parameters.PDBOverridesRequest) { persistent(r, "733") }, parameters.PDBOverridePersistent},
		{"inherited", func(r *parameters.PDBOverridesRequest) {
			r.Member.Effective.Rows[0].ContainerID = &session.Value{Text: "0"}
			r.Member.Effective.Rows[0].IsModified.Text = "FALSE"
		}, parameters.PDBOverrideUnknown},
		{"empty stored read", func(r *parameters.PDBOverridesRequest) { r.Member.Stored.Rows = nil }, parameters.PDBOverrideUnknown},
		{"incomplete stored read", func(r *parameters.PDBOverridesRequest) {
			persistent(r, "734")
			r.Member.StoredEvidence.Complete = false
		}, parameters.PDBOverrideUnknown},
		{"stale stored read", func(r *parameters.PDBOverridesRequest) {
			r.Member.StoredEvidence.ObservedAt = r.At.Add(-2 * r.MaxAge)
		}, parameters.PDBOverrideUnknown},
		{"wrong PDB stored clause", func(r *parameters.PDBOverridesRequest) {
			persistent(r, "734")
			r.Member.Stored.Rows[0].ContainerID = &session.Value{Text: "5"}
		}, parameters.PDBOverrideUnknown},
		{"stored root clause", func(r *parameters.PDBOverridesRequest) {
			persistent(r, "734")
			r.Member.Stored.Rows[0].ContainerID = &session.Value{Text: "1"}
		}, parameters.PDBOverrideUnknown},
		{"memory gate different RU", func(r *parameters.PDBOverridesRequest) { r.OracleVersion = "23.26.1.0.0" }, parameters.PDBOverrideUnknown},
		{"unverified startup", func(r *parameters.PDBOverridesRequest) { r.Member.Startup.Verified = false }, parameters.PDBOverrideUnknown},
		{"short numeric version", func(r *parameters.PDBOverridesRequest) { r.OracleVersion = "23.26.3" }, parameters.PDBOverrideUnknown},
		{"unverified temporal pairing", func(r *parameters.PDBOverridesRequest) { r.RuntimePairVerified = false }, parameters.PDBOverrideUnknown},
		{"malformed effective integer", func(r *parameters.PDBOverridesRequest) { r.Member.Effective.Rows[0].Value.Text = "not-an-integer" }, parameters.PDBOverrideUnknown},
		{"unqualified parameter", func(r *parameters.PDBOverridesRequest) {
			r.Member.Effective.Rows[0].Name.Text = "session_cached_cursors"
			r.Member.Stored.Rows[0].Name.Text = "session_cached_cursors"
		}, parameters.PDBOverrideUnknown},
		{"stored SID contradicts declared filter", func(r *parameters.PDBOverridesRequest) {
			persistent(r, "734")
			r.Member.Stored.Rows[0].SID.Text = "OTHER_SID"
			r.Member.Stored.Provenance.SIDs = []string{"FILTERED_SID", "*"}
			r.Member.Stored.Provenance.SIDPolicy = "selected_sids_plus_wildcard"
		}, parameters.PDBOverrideUnknown},
		{"listed SID filter", func(r *parameters.PDBOverridesRequest) { r.Member.Stored.Provenance.SIDs = []string{"*"} }, parameters.PDBOverrideUnknown},
		{"unset flag NULL", func(r *parameters.PDBOverridesRequest) {
			r.Member.Stored.Rows[0].IsSpecified = session.Value{Null: true}
		}, parameters.PDBOverrideUnknown},
		{"same SID mixed flags", func(r *parameters.PDBOverridesRequest) {
			unset := r.Member.Stored.Rows[0]
			persistent(r, "734")
			r.Member.Stored.Rows = append(r.Member.Stored.Rows, unset)
		}, parameters.PDBOverrideUnknown},
		{"persistent and runtime unavailable", func(r *parameters.PDBOverridesRequest) {
			persistent(r, "734")
			r.Member.EffectiveEvidence.Complete = false
			r.Member.Stored.Rows[0].SID.Text = "OTHER_SID"
		}, parameters.PDBOverridePersistent},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := pdbOverrideFixture()
			test.change(&r)
			report, err := parameters.EvaluatePDBOverrides(t.Context(), r)
			if err != nil || len(report.Findings) != 1 || report.Findings[0].State != test.want {
				t.Fatalf("override report = %+v, err = %v", report, err)
			}
			finding := report.Findings[0]
			if len(finding.Effective) > 0 && finding.Effective[0].ContainerID == r.Member.Effective.Rows[0].ContainerID {
				t.Fatal("effective snapshot aliases input")
			}
			if len(finding.Stored) > 0 && finding.Stored[0].ContainerID == r.Member.Stored.Rows[0].ContainerID {
				t.Fatal("stored snapshot aliases input")
			}
			if test.name == "stored differs from current memory" && (finding.Effective[0].Value.Text != "733" || finding.Stored[0].Value.Text != "734") {
				t.Fatal("independent effective and stored values were lost")
			}
		})
	}
}

func TestPDBOverridesLimitsAndCancellation(t *testing.T) {
	r := pdbOverrideFixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if data, err := parameters.PDBOverridesJSON(ctx, r, rowset.Limits{}); data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled evaluation emitted %s %v", data, err)
	}
	if data, err := parameters.PDBOverridesJSON(t.Context(), r, rowset.Limits{MaxRows: 2}); data != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("row bound emitted %s %v", data, err)
	}
	payload, err := parameters.PDBOverridesJSON(t.Context(), r, rowset.Limits{MaxRows: 3})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := parameters.PDBOverridesJSON(t.Context(), r, rowset.Limits{MaxRows: 3, MaxBytes: len(payload) - 1}); data != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("byte bound emitted %s %v", data, err)
	}
	if data, err := parameters.PDBOverridesJSON(t.Context(), r, rowset.Limits{MaxRows: 3, MaxBytes: len(payload)}); len(data) != len(payload) || err != nil {
		t.Fatalf("exact byte bound rejected %d bytes: %v", len(data), err)
	}
	r.OracleVersion = "11.2.0.4.0"
	if _, err := parameters.EvaluatePDBOverrides(t.Context(), r); !errors.Is(err, parameters.ErrUnsupportedVersion) {
		t.Fatalf("pre-PDB version admitted: %v", err)
	}
}
