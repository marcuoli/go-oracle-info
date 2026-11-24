package storage

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestRecoveryAreaReleaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		version, usage   string
		quota, container bool
	}{
		{"9.2.0.8.0", "", false, false}, {"10.1.0.5.0", "", true, false},
		{"10.2.0.5.0", "V$FLASH_RECOVERY_AREA_USAGE", true, false},
		{"11.1.0.7.0", "V$FLASH_RECOVERY_AREA_USAGE", true, false},
		{"11.2.0.4.0", "V$RECOVERY_AREA_USAGE", true, false},
		{"12.1.0.1.0", "V$RECOVERY_AREA_USAGE", true, true}, {"12.1.0.2.0", "V$RECOVERY_AREA_USAGE", true, true},
		{"12.2.0.1.0", "V$RECOVERY_AREA_USAGE", true, true}, {"18.1.0.0.0", "V$RECOVERY_AREA_USAGE", true, true},
		{"19.32.0.0.0", "V$RECOVERY_AREA_USAGE", true, true}, {"21.21.0.0.0", "V$RECOVERY_AREA_USAGE", true, true},
		{"23.9.0.0.0", "V$RECOVERY_AREA_USAGE", true, true}, {"23.26.4.0.0", "V$RECOVERY_AREA_USAGE", true, true},
		{"26ai", "", false, false},
	} {
		opts := RecoveryAreaOptions{tc.version}
		a, err := RecoveryAreaStatement(opts)
		if tc.quota {
			if err != nil || !strings.Contains(a, " FROM V$RECOVERY_FILE_DEST v") ||
				strings.Contains(a, "v.CON_ID AS CON_ID") != tc.container {
				t.Fatal(tc, a, err)
			}
		} else if a != "" || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatal(tc, a, err)
		}
		b, err := RecoveryAreaUsageStatement(opts)
		if tc.usage != "" {
			if err != nil || !strings.Contains(b, " FROM "+tc.usage+" v") ||
				strings.Contains(b, "v.CON_ID AS CON_ID") != tc.container {
				t.Fatal(tc, b, err)
			}
		} else {
			db, state := testsql.New(t)
			data, jerr := RecoveryAreaUsageJSON(t.Context(), db, opts, rowset.Limits{})
			if b != "" || !errors.Is(err, ErrUnsupportedVersion) || data != nil ||
				!errors.Is(jerr, ErrUnsupportedVersion) || len(state.Calls()) != 0 {
				t.Fatal(tc, b, err, data, jerr, state.Calls())
			}
		}
	}
}

func TestRecoveryAreaExactBytesAndPercentages(t *testing.T) {
	v := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	r := session.Result{
		Columns: []string{"INST_ID", "NAME", "SPACE_LIMIT", "SPACE_USED", "SPACE_RECLAIMABLE", "NUMBER_OF_FILES"},
		Rows:    [][]session.Value{{v("1"), null, v("18446744073709551615"), v("9007199254740993"), null, v("0")}},
	}
	opts := RecoveryAreaOptions{"10.1.0.5.0"}
	req := RecoveryAreaRequest{Options: opts, Reference: sourceRef(t)}
	got, err := ReadRecoveryArea(t.Context(), &fixtureReader{result: r}, req)
	if err != nil || !got.Provenance.Complete || len(got.Rows) != 1 ||
		got.Rows[0].ReportingInstanceID.Text != "1" || got.Rows[0].ContainerID != nil || !got.Rows[0].Name.Null ||
		got.Rows[0].SpaceLimit.Text != "18446744073709551615" || got.Rows[0].SpaceUsed.Text != "9007199254740993" ||
		!got.Rows[0].SpaceReclaimable.Null || got.Rows[0].FileCount.Text != "0" {
		t.Fatal(got, err)
	}
	req.Limits.MaxBytes = 1
	if bad, err := ReadRecoveryArea(t.Context(), &fixtureReader{result: r}, req); bad.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatal(bad, err)
	}
	modern := RecoveryAreaOptions{"23.26.1.0.0"}
	p, err := recoveryAreaUsagePlan(modern)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{
		SQL:     p.sql,
		Columns: []string{"INST_ID", "FILE_TYPE", "PERCENT_SPACE_USED", "PERCENT_SPACE_RECLAIMABLE", "NUMBER_OF_FILES", "CON_ID", "SOURCE_CON_ID"},
		Values:  [][]driver.Value{{int64(2), "REMOTE ARCHIVED LOG", ".1234567890123456789", nil, "0", int64(0), int64(1)}},
	}
	db, state := testsql.New(t, step)
	payload, err := RecoveryAreaUsageJSON(t.Context(), db, modern, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `"REMOTE ARCHIVED LOG",".1234567890123456789",null,"0",0,1`) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
	step.Values = append(step.Values, []driver.Value{int64(2), "FLASHBACK LOG", "1/2", "0", "1", int64(0), int64(1)})
	db, state = testsql.New(t, step)
	if payload, err := RecoveryAreaUsageJSON(t.Context(), db, modern, rowset.Limits{}); payload != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
}
