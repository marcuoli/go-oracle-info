package dataguard

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestRecoveryIdentityNumericReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		version            string
		incarnationColumns int
		promotion          bool
	}{
		{"9.2.0.8.0", 4, false}, {"10.1.0.5.0", 8, true}, {"10.2.0.5.0", 9, true},
		{"11.1.0.7.0", 9, true}, {"11.2.0.4.0", 9, true}, {"12.1.0.1.0", 11, true}, {"12.1.0.2.0", 11, true},
		{"12.2.0.1.0", 11, true}, {"18.1.0.0.0", 11, true}, {"19.32.0.0.0", 11, true}, {"21.21.0.0.0", 11, true},
		{"23.9.0.0.0", 11, true}, {"23.26.4.0.0", 11, true}, {"26ai", 0, false},
	} {
		p, err := databaseIncarnationsPlan(tc.version)
		switch {
		case tc.incarnationColumns == 0:
			if !errors.Is(err, ErrUnsupportedVersion) || p.sql != "" {
				t.Fatal(tc, p, err)
			}
		case err != nil || len(p.schema) != tc.incarnationColumns ||
			strings.Contains(p.sql, "v.PRIOR_INCARNATION#") != (tc.incarnationColumns >= 8) ||
			strings.Contains(p.sql, "v.FLASHBACK_DATABASE_ALLOWED") != (tc.incarnationColumns >= 9) ||
			strings.Contains(p.sql, "v.CON_ID AS CON_ID") != (tc.incarnationColumns >= 11):
			t.Fatal(tc, p, err)
		}
		sql, err := StandbyPromotionSCNStatement(tc.version)
		if tc.promotion {
			if err != nil || !strings.Contains(sql, "TO_CHAR(v.STANDBY_BECAME_PRIMARY_SCN") ||
				strings.Contains(sql, "v.CON_ID AS CON_ID") != (tc.incarnationColumns >= 11) {
				t.Fatal(tc, sql, err)
			}
		} else if sql != "" || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatal(tc, sql, err)
		}
	}
}

func TestIncarnationLegacyAndPromotionExactStates(t *testing.T) {
	v := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	legacy := session.Result{
		Columns: []string{"RESETLOGS_CHANGE", "RESETLOGS_TIME", "PRIOR_RESETLOGS_CHANGE", "PRIOR_RESETLOGS_TIME"},
		Rows:    [][]session.Value{{v("9007199254740993"), v("2026-01-01T01:01:01"), v("0"), null}},
	}
	facts, err := DecodeDatabaseIncarnations(legacy, "9.2.0.8.0")
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Incarnation != nil || facts.Rows[0].Status != nil ||
		facts.Rows[0].ResetlogsChange.Text != "9007199254740993" || facts.Rows[0].PriorResetlogsChange.Text != "0" ||
		!facts.Rows[0].PriorResetlogsTime.Null || facts.Rows[0].ContainerID != nil {
		t.Fatal(facts, err)
	}
	p, err := standbyPromotionSCNPlan("23.26.1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	r := session.Result{
		Columns: []string{"STANDBY_BECAME_PRIMARY_SCN", "CON_ID", "SOURCE_CON_ID"},
		Rows:    [][]session.Value{{null, v("0"), v("1")}},
	}
	promotion, err := DecodeStandbyPromotionSCN(r, "23.26.1.0.0")
	if err != nil || len(promotion.Rows) != 1 || !promotion.Rows[0].SCN.Null ||
		promotion.Rows[0].ContainerID.Text != "0" || promotion.Rows[0].SourceContainerID.Text != "1" {
		t.Fatal(promotion, err)
	}
	r.Rows[0][0] = v("18446744073709551615")
	db, state := testsql.New(t, nativeStep(p, r))
	payload, err := StandbyPromotionSCNJSON(t.Context(), db, "23.26.1.0.0", rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `"18446744073709551615",0,1`) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
	r.Rows = append(r.Rows, r.Rows[0])
	db, state = testsql.New(t, nativeStep(p, r))
	if data, err := StandbyPromotionSCNJSON(t.Context(), db, "23.26.1.0.0", rowset.Limits{}); data != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(data), err, state.Closed())
	}
}
