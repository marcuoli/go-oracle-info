package patches

import (
	"testing"

	"github.com/marcuoli/go-oracle-info/containers"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestPDBConsistencyKeepsRootAndPDBEvidenceIndependent(t *testing.T) {
	base := consistencyRequest()
	r := PDBConsistencyRequest{
		At: base.At, MaxAge: base.MaxAge, Versions: base.Versions, VersionsEvidence: base.VersionsEvidence,
		CoverageVerified: true, SelfManaged: true, InventoryEvidence: base.Scopes[0].Evidence,
		ContainerIDs: []session.Value{coreText("3"), coreText("4"), coreText("5"), coreText("6")},
		Inventory: containers.SQLPatchReleaseResult{Provenance: containers.Provenance{
			Source: "CDB_REGISTRY_SQLPATCH", Scope: containers.Root, OracleVersion: "19.32.0.0.0",
			Complete: true, OptionalColumns: []string{"PATCH_TYPE", "TARGET_VERSION", "FLAGS"},
		}},
	}
	add := func(id, patchID, timestamp, status, version string) {
		p := consistencyPatch(patchID, timestamp, "RU", "APPLY", status, version)
		r.Inventory.Rows = append(r.Inventory.Rows, containers.SQLPatchReleaseAttempt{
			SQLPatchAttempt: containers.SQLPatchAttempt{ContainerID: coreText(id), PatchID: p.PatchID, PatchUID: p.PatchUID,
				Action: p.Action, Status: p.Status, ActionTime: p.ActionTime, Description: p.Description},
			PatchType: p.PatchType, TargetVersion: p.TargetVersion, Flags: &p.Flags,
		})
	}
	add("1", "100", "01:00:00", "SUCCESS", "19.32.0.0.0")
	add("3", "101", "02:00:00", "WITH ERRORS", "19.32.0.0.0")
	add("3", "101", "03:00:00", "SUCCESS", "19.32.0.0.0")
	add("4", "102", "01:30:00", "SUCCESS", "19.31.0.0.0")
	add("5", "103", "02:30:00", "WITH ERRORS", "19.32.0.0.0")
	want := []ConsistencyState{ConsistencyMatches, ConsistencyMismatch, ConsistencySQLErrors, ConsistencyUnknown}
	got, err := EvaluatePDBConsistency(t.Context(), r)
	if err != nil || len(got.Findings) != len(want) {
		t.Fatalf("unexpected PDB results: %+v, %v", got.Findings, err)
	}
	for i, finding := range got.Findings {
		if finding.State != want[i] {
			t.Errorf("container %s got %+v, want %s", r.ContainerIDs[i].Text, finding, want[i])
		}
	}
	if got.Versions.Rows[0].CurrentContainerID.Text != "1" || got.Scopes[0].Patches.Provenance.Source != "CDB_REGISTRY_SQLPATCH" {
		t.Fatalf("provenance changed: %+v, %+v", got.Versions, got.Scopes[0])
	}
	r.Versions.Rows[0].ContainerID = consistencyValue("1")
	got, err = EvaluatePDBConsistency(t.Context(), r)
	if err != nil || got.Findings[0].State != ConsistencyUnknown || got.Findings[0].Reason != ConsistencySourceMismatch {
		t.Fatalf("a root-specific member passed for a PDB: %+v, %v", got.Findings, err)
	}
}
