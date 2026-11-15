package objects

import (
	"database/sql/driver"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"strings"
	"testing"
)

func TestInvalidDetailsJSONRejectsMalformedLateOwnerAndStatus(t *testing.T) {
	for _, kind := range []string{"owner", "status"} {
		t.Run(kind, func(t *testing.T) {
			source := invalidFixture()
			first := make([]driver.Value, len(source.Columns))
			for i, v := range source.Rows[0] {
				if !v.Null {
					first[i] = v.Text
				}
			}
			second := append([]driver.Value(nil), first...)
			if kind == "owner" {
				second[0] = "case'owner"
			} else {
				second[3] = "VALID"
			}
			db, state := testsql.New(t, testsql.Step{Columns: source.Columns, Values: [][]driver.Value{first, second}})
			payload, err := InvalidDetailsJSON(t.Context(), db, 11, []string{"Case'Owner"}, rowset.Limits{})
			if payload != nil || !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("malformed prefix published: %s %v", payload, err)
			}
			if state.Closed() != 1 {
				t.Fatal("cursor not finalized", state.Closed())
			}
		})
	}
}
func TestInvalidDetailsOwnerBindsAreBinaryWithoutSessionMutation(t *testing.T) {
	query, args, err := InvalidDetailsStatement(11, "APP", "app")
	if err != nil || len(args) != 2 || args[0] != "APP" || args[1] != "app" {
		t.Fatal(query, args, err)
	}
	if !strings.Contains(query, "NLSSORT(OWNER, 'NLS_SORT=BINARY')") || !strings.Contains(query, "NLSSORT(:1, 'NLS_SORT=BINARY')") || !strings.Contains(query, "NLSSORT(:2, 'NLS_SORT=BINARY')") || strings.Contains(query, "ALTER SESSION") {
		t.Fatal("owner matching depends on caller collation", query)
	}
}
