package dblinks

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestLinksRetainDeclaredDestinationWithoutExecuting(t *testing.T) {
	r := session.Result{Columns: []string{"OWNER", "DB_LINK", "USERNAME", "HOST", "CREATED"}, Rows: [][]session.Value{{{Text: "PUBLIC"}, {Text: "REMOTE.EXAMPLE"}, {Null: true}, {Text: "(DESCRIPTION=(ADDRESS=(HOST= db.invalid )))"}, {Text: "2000-02-29T01:02:03"}}}}
	o := Options{OracleMajor: 11, Owners: []string{"PUBLIC"}}
	got, err := Decode(r, o)
	if err != nil || !got.Records[0].Username.Null || got.Records[0].Host.Text != "(DESCRIPTION=(ADDRESS=(HOST= db.invalid )))" || got.Records[0].Created.Text != "2000-02-29T01:02:03" {
		t.Fatal(got, err)
	}
	sql, args, err := Statement(o)
	if err != nil || len(args) != 1 || strings.Contains(sql, "@") || strings.Contains(sql, "PASSWORD") || strings.Contains(sql, "SYS.LINK$") {
		t.Fatal(sql, args, err)
	}
}
