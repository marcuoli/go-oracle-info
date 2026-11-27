package patches

import (
	"context"
	"slices"
	"time"

	"github.com/marcuoli/go-oracle-info/containers"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/session"
)

// PDBConsistencyRequest admits one root capture and independently qualified PDBs.
// The caller must verify each ContainerID on the same endpoint, including the
// open mode and visibility on the qualified reporting instances. This API uses
// whole-database GV$INSTANCE rows from that root, retaining their actual scope.
type PDBConsistencyRequest struct {
	At                time.Time
	MaxAge            time.Duration
	Versions          instances.VersionResult
	VersionsEvidence  ConsistencyEvidence
	CoverageVerified  bool
	SelfManaged       bool
	Inventory         containers.SQLPatchReleaseResult
	InventoryEvidence ConsistencyEvidence
	ContainerIDs      []session.Value
}

// EvaluatePDBConsistency evaluates only the requested PDBs from a CDB root
// capture. It preserves PDB-local order, SQL NULLs and truthful provenance.
// A missing effective release remains unknown, even on a complete query.
func EvaluatePDBConsistency(ctx context.Context, r PDBConsistencyRequest) (ConsistencyReport, error) {
	provenance := r.Inventory.Provenance
	major, _ := oracleversion.LegacyFactMajor(provenance.OracleVersion)
	if err := ctx.Err(); err != nil {
		return ConsistencyReport{}, err
	}
	if provenance.Source != "CDB_REGISTRY_SQLPATCH" || provenance.Scope != containers.Root ||
		!slices.Equal(provenance.OptionalColumns, []string{"PATCH_TYPE", "TARGET_VERSION", "FLAGS"}) {
		return ConsistencyReport{}, ErrInvalidConsistencyRequest
	}

	scopeIndex := make(map[int64]int, len(r.ContainerIDs))
	scopes := make([]SQLPatchScope, 0, len(r.ContainerIDs))
	observed := r.InventoryEvidence
	observed.Complete = observed.Complete && provenance.Complete
	for _, container := range r.ContainerIDs {
		id, ok := consistencyID(container, 2)
		if _, duplicate := scopeIndex[id]; !ok || duplicate {
			return ConsistencyReport{}, ErrInvalidConsistencyRequest
		}
		scopeIndex[id] = len(scopes)
		scopes = append(scopes, SQLPatchScope{ContainerID: container, Verified: true, Evidence: observed,
			Patches: SQLPatchResult{Provenance: Provenance{Source: provenance.Source, Scope: string(provenance.Scope), OracleMajor: major}}})
	}
	for _, row := range r.Inventory.Rows {
		if err := ctx.Err(); err != nil {
			return ConsistencyReport{}, err
		}
		id, valid := consistencyID(row.ContainerID, 0)
		if !valid {
			return ConsistencyReport{}, ErrInvalidConsistencyRequest
		}
		at, admitted := scopeIndex[id]
		if !admitted {
			continue
		}
		patch := SQLPatch{PatchID: row.PatchID, PatchUID: row.PatchUID, Action: row.Action,
			Status: row.Status, ActionTime: row.ActionTime, Description: row.Description,
			PatchType: row.PatchType, TargetVersion: row.TargetVersion, Flags: session.Value{Null: true}}
		if row.Flags != nil {
			patch.Flags = *row.Flags
		}
		scopes[at].Patches.Rows = append(scopes[at].Patches.Rows, patch)
	}
	return evaluateConsistency(ctx, ConsistencyRequest{
		At: r.At, MaxAge: r.MaxAge, Versions: r.Versions, VersionsEvidence: r.VersionsEvidence,
		CoverageVerified: r.CoverageVerified, SelfManaged: r.SelfManaged, Scopes: scopes,
	}, true)
}
