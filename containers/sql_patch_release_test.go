package containers

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestSQLPatchReleaseSelectorMatrix(t *testing.T) {
	for _, tc := range []struct {
		version         string
		legacy, release bool
	}{
		{"9.2.0.8.0", false, false}, {"10.1.0.5.0", false, false}, {"10.2.0.5.0", false, false},
		{"11.1.0.7.0", false, false}, {"11.2.0.4.0", false, false}, {"12.1.0.1.0", false, false},
		{"12.1.0.2.0", true, false}, {"12.2.0.1.0", true, false},
		{"18.1.0.0.0", true, true}, {"19.3.0.0.0", true, true}, {"19.33.0.0.0", true, true},
		{"21.3.0.0.0", true, true}, {"23.1.0.0.0", true, true}, {"23.9.0.0.0", true, true},
		{"23.26.1.0.0", true, true}, {"26.0.0.0.0", false, false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := RootDictionaryOptions{OracleVersion: tc.version, Scope: Root}
			legacy, legacyErr := SQLPatchHistoryStatement(options)
			extended, err := SQLPatchReleaseStatement(options)
			if !tc.legacy {
				if !errors.Is(legacyErr, ErrUnsupportedVersion) || !errors.Is(err, ErrUnsupportedVersion) || extended != "" {
					t.Fatalf("unsupported release selected SQL: %q, %v, %v", extended, legacyErr, err)
				}
				return
			}
			if err != nil || legacyErr != nil || strings.Contains(legacy, "TARGET_VERSION") {
				t.Fatalf("legacy contract changed: %s, %v, %v", legacy, legacyErr, err)
			}
			if !tc.release && legacy != extended {
				t.Fatalf("legacy SQL changed when release columns unavailable: %s", extended)
			}
			if tc.release {
				for _, want := range []string{"v.PATCH_TYPE, v.TARGET_VERSION, v.FLAGS", "/*+ NO_MERGE(v) */", "FROM SYS.CDB_REGISTRY_SQLPATCH", ") v ORDER BY v.CON_ID, v.ACTION_TIME, TO_NUMBER(v.PATCH_ID), TO_NUMBER(v.PATCH_UID), v.ACTION"} {
					if !strings.Contains(extended, want) {
						t.Fatalf("extended SQL lost %q: %s", want, extended)
					}
				}
			}
		})
	}
	for _, scope := range []Scope{CurrentContainer, NonCDB} {
		if sql, err := SQLPatchReleaseStatement(RootDictionaryOptions{OracleVersion: "19.3.0.0.0", Scope: scope}); !errors.Is(err, ErrUnsupportedScope) || sql != "" {
			t.Fatalf("non-root source admitted: %s, %v", sql, err)
		}
	}
}

func TestSQLPatchReleaseDecodesNullAndLegacyAbsence(t *testing.T) {
	raw := session.Result{
		Columns: []string{"CON_ID", "PATCH_ID", "PATCH_UID", "ACTION", "STATUS", "ACTION_TIME", "DESCRIPTION", "PATCH_TYPE", "TARGET_VERSION", "FLAGS"},
		Rows: [][]session.Value{{{Text: "3"}, {Text: "9007199254740993"}, {Text: "2"}, {Text: "APPLY"}, {Text: "SUCCESS"},
			{Text: "2026-11-27T09:00:00.000001"}, {Null: true}, {Text: "RU"}, {Text: "19.3.0.0.0"}, {Null: true}}},
	}
	options := RootDictionaryOptions{OracleVersion: "19.3.0.0.0", Scope: Root}
	got, err := DecodeSQLPatchRelease(raw, options)
	if err != nil || len(got.Rows) != 1 {
		t.Fatalf("decoded rows=%v, error=%v", got.Rows, err)
	}
	row := got.Rows[0]
	if row.PatchID.Text != "9007199254740993" || row.PatchType == nil || row.PatchType.Text != "RU" ||
		row.TargetVersion == nil || row.TargetVersion.Text != "19.3.0.0.0" || row.Flags == nil || !row.Flags.Null ||
		got.Provenance.Complete {
		t.Fatalf("lost exact evidence or added transport certainty: %+v / %+v", row, got.Provenance)
	}
	raw.Columns, raw.Rows[0] = raw.Columns[:7], raw.Rows[0][:7]
	if _, err := DecodeSQLPatchRelease(raw, options); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("treated unselected modern release fields as full evidence: %v", err)
	}
	options.OracleVersion = "12.2.0.1.0"
	legacy, err := DecodeSQLPatchRelease(raw, options)
	if err != nil || len(legacy.Rows) != 1 || legacy.Rows[0].PatchType != nil || legacy.Rows[0].TargetVersion != nil || legacy.Rows[0].Flags != nil {
		t.Fatalf("legacy absence ignored: %+v, %v", legacy, err)
	}
}
