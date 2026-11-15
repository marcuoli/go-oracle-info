package triggers

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestTriggerAndObjectStatusStayDistinctWithDateAndMissingJoin(t *testing.T) {
	r := session.Result{Columns: []string{"OWNER", "TRIGGER_NAME", "TRIGGER_TYPE", "TRIGGERING_EVENT", "TABLE_OWNER", "TABLE_NAME", "TRIGGER_STATUS", "OBJECT_STATUS", "LAST_DDL_TIME", "BASE_OBJECT_TYPE", "ACTION_TYPE"}, Rows: [][]session.Value{{{Text: "SYS"}, {Text: "T"}, {Text: "AFTER EVENT"}, {Text: "LOGON"}, {Null: true}, {Null: true}, {Text: "ENABLED"}, {Text: "INVALID"}, {Text: "1999-12-31T23:59:59"}, {Text: "DATABASE"}, {Text: "PL/SQL"}}}}
	o := Options{OracleMajor: 11, Owners: []string{"SYS", "SYSTEM"}}
	got, err := Decode(r, o)
	if err != nil || got.Records[0].TriggerStatus.Text != "ENABLED" || got.Records[0].ObjectStatus.Text != "INVALID" || got.Records[0].LastDDLTime.Text != "1999-12-31T23:59:59" {
		t.Fatal(got, err)
	}
	r.Rows[0][7] = session.Value{Null: true}
	r.Rows[0][8] = session.Value{Null: true}
	got, err = Decode(r, o)
	if err != nil || !got.Records[0].ObjectStatus.Null || !got.Records[0].LastDDLTime.Null {
		t.Fatal(got, err)
	}
	sql, args, err := Statement(o)
	if err != nil || len(args) != 2 || !strings.Contains(sql, "LEFT JOIN DBA_OBJECTS") || !strings.Contains(sql, "o.OBJECT_TYPE='TRIGGER'") || strings.Contains(sql, "GET_DDL") || strings.Contains(sql, "TRIGGER_BODY") {
		t.Fatal(sql, args, err)
	}
}
