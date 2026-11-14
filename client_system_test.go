package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func systemStep(modern bool) testsql.Step {
	list := parameterStep(modern, false)
	result := testsql.Step{Values: [][]driver.Value{{}}}
	for i, name := range list.Columns {
		if name != "ORDINAL" {
			result.Columns = append(result.Columns, name)
			value := list.Values[0][i]
			if name == "ISMODIFIED" {
				value = "MODIFIED"
			}
			result.Values[0] = append(result.Values[0], value)
		}
	}
	return result
}
func visibilityStep(modern bool) testsql.Step {
	columns := []string{"INST_ID"}
	values := []driver.Value{int64(7)}
	if modern {
		columns = append(columns, "CON_ID")
		values = append(values, int64(0))
	}
	columns = append(columns, "INSTANCE_NUMBER", "INSTANCE_NAME", "HOST_NAME", "STATUS", "DATABASE_STATUS", "PARALLEL", "BLOCKED")
	values = append(values, int64(7), "node7", nil, "MOUNTED", "ACTIVE", "YES", "YES")
	return testsql.Step{Columns: columns, Values: [][]driver.Value{values}}
}
func TestSystemAndVisibilityBorrowExactCursor(t *testing.T) {
	for _, visibility := range []bool{false, true} {
		t.Run(map[bool]string{false: "system", true: "visibility"}[visibility], func(t *testing.T) {
			step := systemStep(true)
			if visibility {
				step = visibilityStep(true)
			}
			db, state := testsql.New(t, append(versionSteps("23"), step)...)
			spy := &nativeSpy{db: db}
			client := info.New(spy)
			var rows *sql.Rows
			var err error
			if visibility {
				rows, err = client.InstanceVisibility(t.Context())
			} else {
				rows, err = client.SystemParameters(t.Context(), info.ParameterFilter{Instances: []int64{1}})
			}
			if err != nil || rows == nil || rows != spy.last || !rows.Next() {
				t.Fatal("borrowed cursor identity/lifetime", err)
			}
			if err = rows.Close(); err != nil {
				t.Fatal(err)
			}
			calls := state.Calls()
			last := calls[len(calls)-1]
			if state.Closed() != 3 {
				t.Fatal("cursor not finalized")
			}
			if visibility {
				want, _ := instances.Statement(23)
				if last.SQL != want || len(last.Args) != 0 {
					t.Fatal(last)
				}
			} else {
				want, _, _ := parameters.SystemStatement(23, parameters.Filter{Instances: []int64{1}})
				if last.SQL != want || len(last.Args) != 1 || last.Args[0].Value != int64(1) {
					t.Fatal(last)
				}
			}
		})
	}
}
func TestSystemAndVisibilityJSONVersionTypesAndNull(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, visibility := range []bool{false, true} {
			step := systemStep(modern)
			source := "GV$SYSTEM_PARAMETER"
			if visibility {
				step = visibilityStep(modern)
				source = "GV$INSTANCE"
			}
			major := "11"
			if modern {
				major = "23"
			}
			db, state := testsql.New(t, append(versionSteps(major), step)...)
			client := info.New(db)
			var data json.RawMessage
			var err error
			if visibility {
				data, err = client.InstanceVisibilityJSON(t.Context())
			} else {
				data, err = client.SystemParametersJSON(t.Context(), info.ParameterFilter{})
			}
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Provenance struct{ Source string }
				Data       struct {
					Columns []rowset.Column
					Rows    [][]any
				}
			}
			decoder := json.NewDecoder(strings.NewReader(string(data)))
			decoder.UseNumber()
			if err = decoder.Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Provenance.Source != source || len(envelope.Data.Rows) != 1 || state.Closed() != len(versionSteps(major))+1 {
				t.Fatal("source/completeness/lifecycle", string(data))
			}
			foundContainer := false
			for i, col := range envelope.Data.Columns {
				value := envelope.Data.Rows[0][i]
				switch col.Name {
				case "INST_ID":
					want := json.Number("1")
					if visibility {
						want = "7"
					}
					if value != want {
						t.Fatal(value)
					}
				case "CON_ID":
					foundContainer = true
				case "VALUE", "HOST_NAME":
					if value != nil {
						t.Fatal("NULL fabricated", value)
					}
				case "ISMODIFIED":
					if value != "MODIFIED" {
						t.Fatal("flag normalized", value)
					}
				case "STATUS":
					if value != "MOUNTED" {
						t.Fatal(value)
					}
				case "ORDINAL":
					t.Fatal("scalar or visibility invented an ordinal")
				}
			}
			if foundContainer != modern {
				t.Fatal("legacy absent column fabricated")
			}
		}
	}
}
func TestSystemAndVisibilityJSONNeverReturnFailedPrefix(t *testing.T) {
	for _, visibility := range []bool{false, true} {
		for _, mode := range []string{"next", "close", "row-budget", "byte-budget", "canceled"} {
			t.Run(strings.Join([]string{map[bool]string{false: "system", true: "visibility"}[visibility], mode}, "/"), func(t *testing.T) {
				step := systemStep(true)
				if visibility {
					step = visibilityStep(true)
				}
				failure := errors.New("private cursor diagnostic")
				limits := info.JSONLimits{}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch mode {
				case "next":
					step.NextErr = failure
				case "close":
					step.CloseErr = failure
				case "row-budget":
					step.Values = append(step.Values, step.Values[0])
					limits.MaxRows = 1
				case "byte-budget":
					limits.MaxBytes = 140
				case "canceled":
					step.OnNext = func(int) { cancel() }
				}
				db, _ := testsql.New(t, append(versionSteps("23"), step)...)
				client := info.New(db)
				var data json.RawMessage
				var err error
				if visibility {
					data, err = client.InstanceVisibilityJSON(ctx, limits)
				} else {
					data, err = client.SystemParametersJSON(ctx, info.ParameterFilter{}, limits)
				}
				if err == nil || data != nil {
					t.Fatal("failed prefix escaped", string(data), err)
				}
				if (mode == "next" || mode == "close") && (!errors.Is(err, failure) || strings.Contains(err.Error(), failure.Error())) {
					t.Fatal("unsafe/lost error", err)
				}
				if (mode == "row-budget" || mode == "byte-budget") && !errors.Is(err, rowset.ErrCapacity) {
					t.Fatal("wrong capacity error", err)
				}
			})
		}
	}
}
