package backup

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestRestorePointReleaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		ver     string
		columns int
	}{
		{"9.2.0.8.0", 0}, {"10.1.0.5.0", 0}, {"10.2.0.5.0", 6},
		{"11.1.0.7.0", 8}, {"11.2.0.4.0", 8}, {"12.1.0.1.0", 10}, {"12.1.0.2.0", 10},
		{"12.2.0.1.0", 13}, {"18.1.0.0.0", 13}, {"19.32.0.0.0", 13}, {"21.21.0.0.0", 13},
		{"23.9.0.0.0", 13}, {"23.26.4.0.0", 13}, {"26ai", 0},
	} {
		options := RestorePointsOptions{tc.ver}
		p, err := restorePointsPlan(options)
		if tc.columns == 0 {
			if !errors.Is(err, ErrUnsupportedVersion) || p.sql != "" {
				t.Fatal(tc, p, err)
			}
		} else if err != nil || len(p.schema) != tc.columns ||
			strings.Contains(p.sql, "v.PRESERVED AS PRESERVED") != (tc.columns >= 8) ||
			strings.Contains(p.sql, "v.CON_ID AS CON_ID") != (tc.columns >= 10) ||
			strings.Contains(p.sql, "v.PDB_INCARNATION#") != (tc.columns >= 13) ||
			!strings.Contains(p.sql, "CAST(v.TIME AS TIMESTAMP(9))") {
			t.Fatal(tc, p, err)
		}
	}
}

func TestRestorePointNullableIdentityAndPrecision(t *testing.T) {
	v := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	legacy := session.Result{
		Columns: []string{"NAME", "SCN", "DATABASE_INCARNATION", "GUARANTEE_FLASHBACK_DATABASE", "STORAGE_SIZE", "TIME"},
		Rows:    [][]session.Value{{v("BEFORE_UPGRADE"), v("9007199254740995"), v("4"), v("YES"), v("18446744073709551615"), v("2026-11-23T23:01:02.123456789")}},
	}
	sample, err := DecodeRestorePoints(legacy, RestorePointsOptions{"10.2.0.5.0"})
	if err != nil || len(sample.Rows) != 1 || sample.Rows[0].Preserved != nil || sample.Rows[0].RestorePointTime != nil ||
		sample.Rows[0].ContainerID != nil || sample.Rows[0].SCN.Text != "9007199254740995" || sample.Rows[0].StorageSize.Text != "18446744073709551615" {
		t.Fatal(sample, err)
	}
	modern := RestorePointsOptions{"23.26.1.0.0"}
	complete := session.Result{
		Columns: append(legacy.Columns, "RESTORE_POINT_TIME", "PRESERVED", "CON_ID", "SOURCE_CON_ID", "PDB_RESTORE_POINT", "CLEAN_PDB_RESTORE_POINT", "PDB_INCARNATION"),
		Rows:    [][]session.Value{append(legacy.Rows[0], null, v("YES"), v("3"), v("1"), v("YES"), v("NO"), null)},
	}
	ref, err := reference.NewReference("unit", "backup", "RestorePoints", 1)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := RestorePointsStatement(modern)
	if err != nil {
		t.Fatal(err)
	}
	reader := apiReader(func(_ context.Context, s session.Statement) (session.Result, error) {
		if s.Reference != ref || s.Text != stmt {
			t.Fatal(s)
		}
		return complete, nil
	})
	facts, err := ReadRestorePoints(t.Context(), reader, RestorePointsRequest{Options: modern, Reference: ref})
	if err != nil || len(facts.Rows) != 1 || !facts.Provenance.Complete || !facts.Rows[0].RestorePointTime.Null ||
		facts.Rows[0].Preserved.Text != "YES" || facts.Rows[0].ContainerID.Text != "3" ||
		facts.Rows[0].SourceContainerID.Text != "1" || facts.Rows[0].CleanPDBRestorePoint.Text != "NO" ||
		!facts.Rows[0].PDBIncarnation.Null || facts.Rows[0].Time.Text != "2026-11-23T23:01:02.123456789" {
		t.Fatal(facts, err)
	}
	step := testsql.Step{SQL: stmt, Columns: complete.Columns, Values: [][]driver.Value{
		{"BEFORE_UPGRADE", "9007199254740995", "4", "YES", "18446744073709551615", "2026-11-23T23:01:02.123456789", nil, "YES", int64(3), int64(1), "YES", "NO", nil},
	}}
	db, state := testsql.New(t, step)
	payload, err := RestorePointsJSON(t.Context(), db, modern, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `"18446744073709551615","2026-11-23T23:01:02.123456789",null,"YES",3,1`) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
	bad := append([]driver.Value(nil), step.Values[0]...)
	bad[5] = "2026-11-23T23:01:02.123457"
	step.Values = append(step.Values, bad)
	db, state = testsql.New(t, step)
	if payload, err := RestorePointsJSON(t.Context(), db, modern, rowset.Limits{}); payload != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
}
