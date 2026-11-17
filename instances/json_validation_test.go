package instances_test

import (
	"database/sql/driver"
	"errors"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
	"testing"
)

func TestVisibilityJSONRejectsLateInvalidInstance(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		native     driver.Value
		null       bool
	}{
		{"zero", "0", int64(0), false}, {"negative", "-1", int64(-1), false}, {"NULL", "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fixture(true)
			source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
			change(&source, "INST_ID", session.Value{Text: tc.text, Null: tc.null})
			if _, err := instances.Decode(source, 19); !errors.Is(err, instances.ErrInvalidEvidence) {
				t.Fatal("typed rejection setup", err)
			}
			row := make([]driver.Value, len(source.Columns))
			for i, v := range source.Rows[0] {
				if v.Null {
					continue
				}
				row[i] = v.Text
				switch source.Columns[i] {
				case "INST_ID", "CON_ID", "INSTANCE_NUMBER":
					n, err := strconv.ParseInt(v.Text, 10, 64)
					if err != nil {
						t.Fatal(err)
					}
					row[i] = n
				}
			}
			bad := append([]driver.Value(nil), row...)
			bad[0] = tc.native
			db, state := testsql.New(t, testsql.Step{Columns: source.Columns, Values: [][]driver.Value{row, bad}})
			got, err := instances.JSON(t.Context(), db, 19, rowset.Limits{})
			if got != nil || !errors.Is(err, instances.ErrInvalidEvidence) {
				t.Fatalf("invalid visible member published: %s %v", got, err)
			}
			if state.Closed() != 1 {
				t.Fatal("cursor not finalized", state.Closed())
			}
		})
	}
}
