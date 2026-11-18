package dataguard

import (
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"testing"
)

func v(s string) session.Value { return session.Value{Text: s} }
func TestIncarnationRequiresPairedTime(t *testing.T) {
	options := ArchiveProgressOptions{ResetlogsChange: "100"}
	if _, _, err := ArchiveProgressStatement("23.26.1.0.0", options); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("SCN alone admitted another incarnation: %v", err)
	}
	options.ResetlogsTime = "2026-11-18T01:02:03"
	p, err := archiveProgressPlan("23.26.1.0.0", options)
	if err != nil {
		t.Fatal(err)
	}
	r := validSource(p)
	for i, c := range r.Columns {
		if c == "RESETLOGS_CHANGE" {
			r.Rows[0][i] = v("100")
		}
		if c == "RESETLOGS_TIME" {
			r.Rows[0][i] = v("2026-11-17T01:02:03")
		}
	}
	if _, err = DecodeArchiveProgress(r, "23.26.1.0.0", options); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("accepted other resetlogs time: %v", err)
	}
}
func TestLatestLastAppliedRedoPreservesActualTiesAndUndated(t *testing.T) {
	row := RecoveryProgress{InstanceID: v("2"), ContainerID: v("0"), Item: v("Last Applied Redo"), Timestamp: v("2026-11-18T01:00:00"), Comments: v("SCN 18446744073709551616")}
	earlier := row
	earlier.Timestamp = v("2026-11-18T00:59:59")
	earlier.Comments = v("SCN 99999999999999999999")
	tie := row
	tie.Comments = v("SCN 18446744073709551617")
	unknown := row
	unknown.Timestamp = session.Value{Null: true}
	unknown.Comments = v("unknown time")
	result := RecoveryProgressResult{Provenance: Provenance{Complete: true}, Rows: []RecoveryProgress{earlier, row, tie, unknown}}
	latest, err := LatestLastAppliedRedo(result)
	if err != nil || len(latest) != 1 || len(latest[0].Rows) != 2 || len(latest[0].Undated) != 1 || latest[0].Rows[0].Comments != row.Comments || latest[0].Rows[1].Comments != tie.Comments {
		t.Fatalf("paired latest rows/ties lost: %+v %v", latest, err)
	}
	result.Provenance.Complete = false
	if _, err = LatestLastAppliedRedo(result); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("incomplete source accepted: %v", err)
	}
}
func TestArchiveIdentityGroupsCopiesWithoutLosingConflicts(t *testing.T) {
	row := ArchiveProgress{InstanceID: v("7"), ContainerID: v("0"), Kind: v("RECEIVED"), ThreadNumber: v("2"), SequenceNumber: v("1844674407370955"), ResetlogsChange: v("18446744073709551616"), ResetlogsTime: v("2026-11-18T00:00:00"), DestinationID: v("1"), FirstTime: v("2026-11-18T01:00:00")}
	copy2 := row
	copy2.DestinationID = v("2")
	conflict := copy2
	conflict.FirstTime = v("2026-11-18T01:00:01")
	thread := row
	thread.ThreadNumber = v("3")
	result := ArchiveProgressResult{Provenance: Provenance{Complete: true}, Rows: []ArchiveProgress{row, copy2, conflict, thread}}
	ids, err := ArchiveProgressIdentities(result)
	if err != nil || len(ids) != 2 || len(ids[0].Copies) != 3 || !ids[0].Conflicting || ids[1].Conflicting {
		t.Fatalf("lost copies/conflict/thread: %+v %v", ids, err)
	}
	result.Rows = []ArchiveProgress{row, copy2}
	ids, err = ArchiveProgressIdentities(result)
	if err != nil || len(ids) != 1 || ids[0].Conflicting {
		t.Fatalf("consistent copies conflicted: %+v %v", ids, err)
	}
}
