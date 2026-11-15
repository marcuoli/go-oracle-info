package sqlplan

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestProfileAndBaselineMetadataPreserveStatusAndTime(t *testing.T) {
	p := session.Result{Columns: []string{"NAME", "CATEGORY", "SIGNATURE", "CREATED", "LAST_MODIFIED", "DESCRIPTION", "TYPE", "STATUS", "FORCE_MATCHING"}, Rows: [][]session.Value{{{Text: "PROF"}, {Text: "CUSTOM"}, {Text: "18446744073709551615"}, {Text: "2001-02-03T04:05:06.123456000"}, {Null: true}, {Null: true}, {Text: "MANUAL"}, {Text: "future"}, {Text: "YES"}}}}
	got, err := DecodeProfile(p, Options{OracleMajor: 11})
	if err != nil || got.Records[0].Signature.Text != "18446744073709551615" || got.Records[0].Status.Text != "future" || !got.Records[0].LastModified.Null || got.Records[0].Created.Text != "2001-02-03T04:05:06.123456000" {
		t.Fatal(got, err)
	}
	b := session.Result{Columns: []string{"SIGNATURE", "SQL_HANDLE", "PLAN_NAME", "CREATOR", "ORIGIN", "PARSING_SCHEMA_NAME", "DESCRIPTION", "VERSION", "CREATED", "LAST_MODIFIED", "LAST_EXECUTED", "LAST_VERIFIED", "ENABLED", "ACCEPTED", "FIXED", "REPRODUCED", "AUTOPURGE"}, Rows: [][]session.Value{{{Text: "18446744073709551615"}, {Text: "SQL_1"}, {Text: "PLAN_1"}, {Text: "SYS"}, {Text: "AUTO-CAPTURE"}, {Text: "APP"}, {Null: true}, {Text: "11.2.0.4"}, {Text: "2001-02-03T04:05:06.123456000"}, {Null: true}, {Null: true}, {Null: true}, {Text: "YES"}, {Text: "NO"}, {Text: "NO"}, {Text: "YES"}, {Null: true}}}}
	base, err := DecodeBaseline(b, Options{OracleMajor: 19, Owners: []string{"APP"}})
	if err != nil || base.Records[0].Origin.Text != "AUTO-CAPTURE" || base.Records[0].Accepted.Text != "NO" || !base.Records[0].LastExecuted.Null || !base.Records[0].Autopurge.Null {
		t.Fatal(base, err)
	}
	for _, stmt := range []func(Options) (string, []any, error){ProfileStatement, BaselineStatement} {
		sql, _, err := stmt(Options{OracleMajor: 19})
		if err != nil || strings.Contains(sql, "SQL_TEXT") || strings.Contains(sql, "DBMS_SQL") || strings.Contains(sql, "ALTER") {
			t.Fatal(sql, err)
		}
	}
}
