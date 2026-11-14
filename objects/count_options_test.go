package objects

import (
	"strings"
	"testing"
)

func TestCountOptionsKeepDefaultAndQuoteIdentifiers(t *testing.T) {
	want := "SELECT TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT FROM \"a\"\"b\".\"MiXeD\""
	legacy, err := CountStatement("a\"b", "MiXeD")
	if err != nil || legacy != want {
		t.Fatalf("legacy=%q err=%v", legacy, err)
	}
	plain, err := CountStatementWithOptions("a\"b", "MiXeD", CountOptions{})
	if err != nil || plain != want {
		t.Fatalf("zero options=%q err=%v", plain, err)
	}
	for _, degree := range []int{1, 4, 1024} {
		text, err := CountStatementWithOptions("a\"b", "MiXeD", CountOptions{ParallelDegree: degree})
		if err != nil || !strings.Contains(text, "/*+ PARALLEL(") || !strings.HasSuffix(text, "FROM \"a\"\"b\".\"MiXeD\"") {
			t.Fatalf("degree %d: %q err=%v", degree, text, err)
		}
	}
	hinted, err := CountStatementWithOptions("HR", "EMPLOYEES", CountOptions{ParallelDegree: 4})
	if err != nil || !strings.HasPrefix(hinted, "SELECT /*+ PARALLEL(4) */ TO_CHAR(COUNT(*)") {
		t.Fatalf("hint placement: %q err=%v", hinted, err)
	}
}

func TestCountOptionsRejectInvalidDegreeAndNames(t *testing.T) {
	for _, degree := range []int{-1, 1025} {
		if text, err := CountStatementWithOptions("HR", "T", CountOptions{ParallelDegree: degree}); err == nil || text != "" {
			t.Fatalf("accepted degree %d: %q %v", degree, text, err)
		}
	}
	for _, name := range []string{"", "bad\x00name", strings.Repeat("x", 129), string([]byte{0xff})} {
		if text, err := CountStatementWithOptions(name, "T", CountOptions{ParallelDegree: 4}); err == nil || text != "" {
			t.Fatalf("accepted owner %q", name)
		}
		if text, err := CountStatementWithOptions("HR", name, CountOptions{ParallelDegree: 4}); err == nil || text != "" {
			t.Fatalf("accepted table %q", name)
		}
	}
}

func TestStatisticsSelectorsPreserveNullsAndBoundScope(t *testing.T) {
	for _, statement := range []string{TablesStatisticsSQL, TableStatisticsSQL} {
		for _, required := range []string{"NUM_ROWS", "AS ROW_COUNT", "LAST_ANALYZED", "NLS_CALENDAR=GREGORIAN", "OWNER=:1", "TEMPORARY='N'", "NESTED='NO'", "SECONDARY='N'", "IOT_OVERFLOW"} {
			if !strings.Contains(statement, required) {
				t.Fatalf("missing %q in %s", required, statement)
			}
		}
		if strings.Contains(statement, "NVL(NUM_ROWS") || strings.Contains(statement, "COUNT(*)") || strings.Contains(statement, "DBMS_STATS") {
			t.Fatalf("statistics query fabricated counts or gathered statistics: %s", statement)
		}
	}
	if !strings.Contains(TableStatisticsSQL, "TABLE_NAME=:2") || strings.Contains(TablesStatisticsSQL, ":2") {
		t.Fatal("single-table and owner inventory bindings differ from their contracts")
	}
}
