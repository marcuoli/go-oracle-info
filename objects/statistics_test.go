package objects_test

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestOldestStatisticsStatement(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		major     int
		automatic bool
		owners    []string
	}{
		{"legacy-admitted", 11, false, []string{"MiXeD", "X') OR 1=1 --"}},
		{"12-admitted", 12, false, []string{"App"}},
		{"12-capability", 12, true, nil},
		{"19-all-non-maintained", 19, true, nil},
		{"26-restricted", 26, true, []string{"雪"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := objects.StatisticsOptions{OracleMajor: tc.major, Limit: 10, OracleMaintainedAvailable: tc.automatic}
			text, args, err := objects.OldestStatisticsStatement(options, tc.owners...)
			want := make([]any, 0, len(tc.owners)+1)
			for _, owner := range tc.owners {
				want = append(want, owner)
				if strings.Contains(text, owner) {
					t.Fatalf("owner escaped bound argument: %q", text)
				}
			}
			want = append(want, 10)
			if err != nil || !reflect.DeepEqual(args, want) {
				t.Fatalf("args=%v want=%v err=%v", args, want, err)
			}
			if strings.Contains(text, "ORACLE_MAINTAINED") != tc.automatic || strings.Contains(text, "s.SCOPE AS SCOPE") != (tc.major >= 12) {
				t.Fatalf("version/capability selection lost: %s", text)
			}
			for _, clause := range []string{
				"FROM DBA_TAB_STATISTICS s",
				"ROW_NUMBER() OVER (ORDER BY s.LAST_ANALYZED ASC NULLS FIRST, NLSSORT(s.OWNER, 'NLS_SORT=BINARY')",
				"NLSSORT(s.TABLE_NAME, 'NLS_SORT=BINARY') ASC NULLS FIRST, NLSSORT(s.PARTITION_NAME, 'NLS_SORT=BINARY')",
				"NLSSORT(s.SUBPARTITION_NAME, 'NLS_SORT=BINARY') ASC NULLS FIRST",
				"TO_CHAR(s.NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS NUM_ROWS",
				"TO_CHAR(s.LAST_ANALYZED, 'YYYY-MM-DD\"T\"HH24:MI:SS', 'NLS_CALENDAR=GREGORIAN') AS LAST_ANALYZED",
				" ORDER BY STATISTICS_RANK",
			} {
				if !strings.Contains(text, clause) {
					t.Fatalf("missing bounded chronological contract %q in %s", clause, text)
				}
			}
			if strings.Contains(text, "PARTITION BY") || strings.Contains(text, "FETCH FIRST") || strings.Contains(text, "COUNT(") {
				t.Fatalf("query changed global 11g-compatible selection: %s", text)
			}
			if tc.automatic && !strings.Contains(text, "u.USERNAME=s.OWNER AND u.ORACLE_MAINTAINED='N'") {
				t.Fatal("automatic selection lost exact maintained-owner filter")
			}
			if len(tc.owners) == 2 && !strings.Contains(text, "OWNER IN (:1,:2)") {
				t.Fatal("legacy owner admission lost")
			}
			if len(args) > 1 {
				args[0] = "mutated"
				if tc.owners[0] == "mutated" {
					t.Fatal("arguments alias caller owners")
				}
			}
		})
	}
}

func TestOldestStatisticsRejectsUnsafeSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		options objects.StatisticsOptions
		owners  []string
		want    error
	}{
		{"old", objects.StatisticsOptions{OracleMajor: 10, Limit: 10}, []string{"APP"}, objects.ErrUnsupportedStatisticsVersion},
		{"unbounded", objects.StatisticsOptions{OracleMajor: 19, Limit: 0, OracleMaintainedAvailable: true}, nil, objects.ErrInvalidStatisticsOptions},
		{"negative", objects.StatisticsOptions{OracleMajor: 19, Limit: -1}, []string{"APP"}, objects.ErrInvalidStatisticsOptions},
		{"over-budget", objects.StatisticsOptions{OracleMajor: 19, Limit: 1001}, []string{"APP"}, objects.ErrInvalidStatisticsOptions},
		{"legacy-unclassified", objects.StatisticsOptions{OracleMajor: 11, Limit: 10}, nil, objects.ErrStatisticsOwnersRequired},
		{"12-unclassified", objects.StatisticsOptions{OracleMajor: 12, Limit: 10}, nil, objects.ErrStatisticsOwnersRequired},
		{"legacy-false-capability", objects.StatisticsOptions{OracleMajor: 11, Limit: 10, OracleMaintainedAvailable: true}, nil, objects.ErrInvalidStatisticsOptions},
		{"empty-owner", objects.StatisticsOptions{OracleMajor: 11, Limit: 10}, []string{""}, objects.ErrInvalidStatisticsOptions},
		{"invalid-owner", objects.StatisticsOptions{OracleMajor: 11, Limit: 10}, []string{"APP\x00"}, objects.ErrInvalidStatisticsOptions},
		{"too-many-owners", objects.StatisticsOptions{OracleMajor: 19, Limit: 10}, make([]string, 1001), objects.ErrInvalidStatisticsOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, args, err := objects.OldestStatisticsStatement(tc.options, tc.owners...)
			if !errors.Is(err, tc.want) || text != "" || args != nil {
				t.Fatalf("unsafe statement emitted: %s %v %v", text, args, err)
			}
		})
	}
}

func TestDecodeStatisticsPreservesIndependentFacts(t *testing.T) {
	t.Parallel()
	// Metadata deliberately reordered/case-varied; a 38-digit NUMBER must not
	// cross a floating-point conversion, and GTT scope must not collapse rows.
	columns := []string{"scope", "TABLE_NAME", "owner", "OBJECT_TYPE", "PARTITION_NAME", "SUBPARTITION_NAME", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED"}
	row := []session.Value{
		{Text: "SHARED"}, {Text: "MiXeD"}, {Text: "App"}, {Text: "SUBPARTITION"},
		{Text: "Parent"}, {Text: "Child"}, {Text: "99999999999999999999999999999999999999"},
		{Text: "2026-01-02T03:04:05"}, {Text: "DATA"},
	}
	second := append([]session.Value(nil), row...)
	second[0] = session.Value{Text: "SESSION"}
	second[6] = session.Value{Null: true, Text: "do not retain NULL payload"}
	second[7] = session.Value{Null: true}
	second[8] = session.Value{Null: true}
	result := session.Result{Columns: columns, Rows: [][]session.Value{row, second, row}}
	got, err := objects.DecodeStatistics(result, 19, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("facts=%v err=%v", got, err)
	}
	n := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	shared, sessionScope := n("SHARED"), n("SESSION")
	want := []objects.Statistics{
		{Owner: n("App"), Table: n("MiXeD"), Partition: n("Parent"), Subpartition: n("Child"), ObjectType: n("SUBPARTITION"), NumRows: n("99999999999999999999999999999999999999"), LastAnalyzed: n("2026-01-02T03:04:05"), StatTypeLocked: n("DATA"), Scope: &shared},
		{Owner: n("App"), Table: n("MiXeD"), Partition: n("Parent"), Subpartition: n("Child"), ObjectType: n("SUBPARTITION"), Scope: &sessionScope},
	}
	want = append(want, want[0])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("independent raw facts changed:\ngot %#v\nwant %#v", got, want)
	}
	row[1].Text = "changed"
	if got[0].Table.String != "MiXeD" {
		t.Fatal("returned facts changed with source")
	}
	legacy := session.Result{Columns: columns[1:], Rows: [][]session.Value{second[1:]}}
	got, err = objects.DecodeStatistics(legacy, 11, 1)
	if err != nil || len(got) != 1 || got[0].Scope != nil || got[0].NumRows.Valid || got[0].StatTypeLocked.Valid {
		t.Fatalf("legacy absence/NULL lost: %#v %v", got, err)
	}
	second[0] = session.Value{Null: true}
	got, err = objects.DecodeStatistics(session.Result{Columns: columns, Rows: [][]session.Value{second}}, 19, 1)
	if err != nil || got[0].Scope == nil || got[0].Scope.Valid {
		t.Fatal("selected NULL scope confused with unavailable legacy column")
	}
}

func TestDecodeStatisticsRejectsMalformedOrOversizedResult(t *testing.T) {
	t.Parallel()
	base := []string{"OWNER", "TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED"}
	for _, tc := range []struct {
		name    string
		columns []string
		rows    [][]session.Value
	}{
		{"missing", base[:7], nil},
		{"duplicate", []string{"OWNER", "owner", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED"}, nil},
		{"unexpected", append(append([]string(nil), base...), "SCOPE"), nil},
		{"late-short-row", base, [][]session.Value{make([]session.Value, 8), make([]session.Value, 7)}},
		{"late-wide-row", base, [][]session.Value{make([]session.Value, 8), make([]session.Value, 9)}},
		{"too-many", base, make([][]session.Value, 11)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := objects.DecodeStatistics(session.Result{Columns: tc.columns, Rows: tc.rows}, 11, 10)
			if got != nil || !errors.Is(err, objects.ErrInvalidStatisticsEvidence) {
				t.Fatalf("partial evidence accepted: %#v %v", got, err)
			}
		})
	}
	got, err := objects.DecodeStatistics(session.Result{Columns: base}, 11, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("successful empty selection lost: %#v %v", got, err)
	}
	for _, tc := range []struct {
		name         string
		major, limit int
		want         error
	}{
		{"version", 10, 10, objects.ErrUnsupportedStatisticsVersion},
		{"bound", 11, 0, objects.ErrInvalidStatisticsOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := objects.DecodeStatistics(session.Result{Columns: base}, tc.major, tc.limit)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("invalid decode options accepted: %#v %v", got, err)
			}
		})
	}
}
