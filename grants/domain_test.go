package grants

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestDirectAndColumnGrantsPreserveDifferentScope(t *testing.T) {
	o := Options{OracleMajor: 11, Owners: []string{"SYS"}, Names: []string{"AppUser"}}
	r := session.Result{Columns: []string{"GRANTEE", "OWNER", "TABLE_NAME", "GRANTOR", "PRIVILEGE", "GRANTABLE", "HIERARCHY"}, Rows: [][]session.Value{{{Text: "AppUser"}, {Text: "SYS"}, {Text: "OBJ"}, {Text: "SYS"}, {Text: "SELECT"}, {Text: "YES"}, {Text: "NO"}}}}
	got, err := DecodeDirect(r, o)
	if err != nil || got.Records[0].Grantee.Text != "AppUser" || got.Records[0].Grantable.Text != "YES" || got.Records[0].Hierarchy.Text != "NO" || got.Records[0].Common != nil {
		t.Fatal(got, err)
	}
	c := session.Result{Columns: []string{"GRANTEE", "OWNER", "TABLE_NAME", "COLUMN_NAME", "GRANTOR", "PRIVILEGE", "GRANTABLE"}, Rows: [][]session.Value{{{Text: "AppUser"}, {Text: "SYS"}, {Text: "OBJ"}, {Text: "CaseSensitive"}, {Text: "SYS"}, {Text: "UPDATE"}, {Text: "YES"}}}}
	column, err := DecodeDirectColumn(c, o)
	if err != nil || column.Records[0].ColumnName.Text != "CaseSensitive" || column.Records[0].Grantor.Text != "SYS" || column.Records[0].Grantable.Text != "YES" {
		t.Fatal(column, err)
	}
	for _, stmt := range []func(Options) (string, []any, error){DirectStatement, DirectColumnStatement, RoleStatement} {
		sql, args, err := stmt(o)
		if err != nil || len(args) != 2 || strings.Contains(sql, "GRANT '") || strings.Contains(sql, "NOT IN") || strings.Contains(sql, "AppUser") {
			t.Fatal(sql, args, err)
		}
	}
}
func TestRoleGrantVisibilityColumnAndScopeFlags(t *testing.T) {
	r := session.Result{Columns: []string{"ROLE", "OWNER", "TABLE_NAME", "COLUMN_NAME", "PRIVILEGE", "GRANTABLE", "COMMON", "INHERITED"}, Rows: [][]session.Value{{{Text: "APP_ROLE"}, {Text: "SYS"}, {Text: "OBJ"}, {Text: "C1"}, {Text: "UPDATE"}, {Text: "YES"}, {Text: "YES"}, {Null: true}}, {{Text: "APP_ROLE"}, {Text: "SYS"}, {Text: "OBJ"}, {Null: true}, {Text: "SELECT"}, {Text: "NO"}, {Text: "NO"}, {Text: "NO"}}}}
	got, err := DecodeRole(r, Options{OracleMajor: 19, Owners: []string{"SYS"}})
	if err != nil || len(got.Records) != 2 || got.Provenance.Scope != "connected_user_accessible_roles_only" || got.Records[0].ColumnName.Text != "C1" || !got.Records[1].ColumnName.Null || !got.Records[0].Inherited.Null || got.Records[0].Common.Text != "YES" {
		t.Fatal(got, err)
	}
}
