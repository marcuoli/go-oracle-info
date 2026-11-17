package parameters_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func parameterNativeStep(t *testing.T, source session.Result) testsql.Step {
	t.Helper()
	stored := false
	for _, name := range source.Columns {
		stored = stored || name == "SID"
	}
	step := testsql.Step{Columns: source.Columns}
	for _, row := range source.Rows {
		values := make([]driver.Value, len(row))
		for i, v := range row {
			if v.Null {
				continue
			}
			values[i] = v.Text
			switch source.Columns[i] {
			case "INST_ID", "CON_ID", "NUM", "ORDINAL":
				n, err := strconv.ParseInt(v.Text, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				values[i] = n
			case "TYPE":
				if !stored {
					n, err := strconv.ParseInt(v.Text, 10, 64)
					if err != nil {
						t.Fatal(err)
					}
					values[i] = n
				}
			}
		}
		step.Values = append(step.Values, values)
	}
	return step
}

func TestParameterJSONRejectsLateOutOfSelectionRows(t *testing.T) {
	filter := parameters.Filter{Names: []string{"_candidate"}, Instances: []int64{7}}
	stored := parameters.SPFileFilter{Names: []string{"_candidate"}, Instances: []int64{7}, SIDs: []string{"node7"}}
	substring := parameters.HiddenCatalogFilter{NameContains: "waiters"}
	exact := parameters.HiddenCatalogFilter{Names: []string{"_waiters_test"}}
	for _, tc := range []struct {
		name          string
		source        session.Result
		column, value string
		decode        func(session.Result) error
		read          func(context.Context, rowset.Queryer) (json.RawMessage, error)
	}{
		{"effective name", parameterResult(true, false), "NAME", "_other", func(r session.Result) error { _, e := parameters.DecodeEffective(r, 19, filter); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.EffectiveJSON(c, q, 19, filter, rowset.Limits{})
		}},
		{"effective instance", parameterResult(true, false), "INST_ID", "8", func(r session.Result) error { _, e := parameters.DecodeEffective(r, 19, filter); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.EffectiveJSON(c, q, 19, filter, rowset.Limits{})
		}},
		{"system name", systemResult(true), "NAME", "_other", func(r session.Result) error { _, e := parameters.DecodeSystem(r, 19, filter); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.SystemJSON(c, q, 19, filter, rowset.Limits{})
		}},
		{"hidden prefix", parameterResult(true, false), "NAME", "processes", func(r session.Result) error { _, e := parameters.DecodeHidden(r, 19, parameters.Filter{}); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.HiddenJSON(c, q, 19, parameters.Filter{}, rowset.Limits{})
		}},
		{"stored SID", parameterResult(true, true), "SID", "node8", func(r session.Result) error { _, e := parameters.DecodeSPFile(r, 19, stored); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.SPFileJSON(c, q, 19, stored, rowset.Limits{})
		}},
		{"catalog prefix", hiddenCatalogResult(), "NAME", "processes", func(r session.Result) error {
			_, e := parameters.DecodeHiddenCatalog(r, 19, parameters.HiddenCatalogFilter{})
			return e
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.HiddenCatalogJSON(c, q, 19, parameters.HiddenCatalogFilter{}, rowset.Limits{})
		}},
		{"catalog substring", hiddenCatalogResult(), "NAME", "_unrelated", func(r session.Result) error { _, e := parameters.DecodeHiddenCatalog(r, 19, substring); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.HiddenCatalogJSON(c, q, 19, substring, rowset.Limits{})
		}},
		{"catalog exact name", hiddenCatalogResult(), "NAME", "_waiters_other", func(r session.Result) error { _, e := parameters.DecodeHiddenCatalog(r, 19, exact); return e }, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return parameters.HiddenCatalogJSON(c, q, 19, exact, rowset.Limits{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source
			source.Rows = source.Rows[:1]
			if err := tc.decode(source); err != nil {
				t.Fatalf("valid prefix setup: %v", err)
			}
			source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
			setValue(&source, 1, tc.column, session.Value{Text: tc.value})
			if err := tc.decode(source); !errors.Is(err, parameters.ErrInvalidEvidence) {
				t.Fatalf("typed rejection setup: %v", err)
			}
			db, state := testsql.New(t, parameterNativeStep(t, source))
			got, err := tc.read(t.Context(), db)
			if got != nil || !errors.Is(err, parameters.ErrInvalidEvidence) {
				t.Fatalf("out-of-selection prefix published: %s %v", got, err)
			}
			if state.Closed() != 1 {
				t.Fatal("owned cursor not finalized", state.Closed())
			}
		})
	}
}
