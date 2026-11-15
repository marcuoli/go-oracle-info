package directories

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestDirectoryPathAndOriginVisibility(t *testing.T) {
	for _, major := range []int{11, 12, 19, 26} {
		r := session.Result{Columns: []string{"OWNER", "DIRECTORY_NAME", "DIRECTORY_PATH"}, Rows: [][]session.Value{{{Text: "SYS"}, {Text: "IMPORT_DIR"}, {Text: "/data/Raw path/\n\u96ea"}}}}
		if major >= 18 {
			r.Columns = append(r.Columns, "ORIGIN_CON_ID")
			r.Rows[0] = append(r.Rows[0], session.Value{Null: true})
		}
		o := Options{OracleMajor: major, Names: []string{"IMPORT_DIR"}}
		got, err := Decode(r, o)
		if err != nil || got.Records[0].Path.Text != "/data/Raw path/\n\u96ea" || (got.Records[0].OriginContainerID != nil) != (major >= 18) || (got.Records[0].OriginContainerID != nil && !got.Records[0].OriginContainerID.Null) {
			t.Fatal(got, err)
		}
		sql, args, err := Statement(o)
		if err != nil || len(args) != 1 || strings.Contains(sql, "GET_DDL") || strings.Contains(sql, "UTL_FILE") {
			t.Fatal(sql, args, err)
		}
	}
}
