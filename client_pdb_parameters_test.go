package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
)

func TestPDBParametersRootAndDirect(t *testing.T) {
	filter := info.ParameterFilter{Names: []string{"open_cursors"}, Instances: []int64{7}}
	for _, test := range []struct {
		name      string
		container string
		scope     string
		ids       []int64
		wantArgs  []any
	}{
		{"root all", "1", "root_visible_pdb_rows", nil, []any{int64(7), "open_cursors"}},
		{"root selected", "1", "root_visible_pdb_rows", []int64{3}, []any{int64(3), int64(7), "open_cursors"}},
		{"direct PDB", "3", "current_pdb_rows", nil, []any{int64(3), int64(7), "open_cursors"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := "CDB$ROOT"
			if test.container == "3" {
				name = "APP_PDB"
			}
			steps := []testsql.Step{
				{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"19.0.0.0.0"}}},
				{SQL: identity.ConnectedArchitectureSQL, Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}, Values: [][]driver.Value{{"YES", test.container, name}}},
				{Columns: []string{"INST_ID", "CON_ID", "NAME", "VALUE", "DISPLAY_VALUE"}, Values: [][]driver.Value{{int64(7), int64(3), "open_cursors", "310", nil}}},
			}
			db, trace := testsql.New(t, steps...)
			spy := &nativeSpy{db: db}
			rows, err := info.New(spy).PDBParameters(t.Context(), test.ids, filter)
			if err != nil || rows != spy.last || !rows.Next() {
				t.Fatalf("did not return the caller-native cursor: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			calls := trace.Calls()
			got := make([]any, len(calls[2].Args))
			for i, arg := range calls[2].Args {
				got[i] = arg.Value
			}
			if !reflect.DeepEqual(got, test.wantArgs) {
				t.Fatalf("selected wrong scope: %+v", calls[2])
			}
			steps[2].SQL = calls[2].SQL
			db, _ = testsql.New(t, steps...)
			data, err := info.New(db).PDBParametersJSON(t.Context(), test.ids, filter)
			var payload struct {
				Provenance parameters.ContainerSystemProvenance
				Data       struct{ Rows [][]any }
			}
			if err != nil || json.Unmarshal(data, &payload) != nil || !payload.Provenance.Complete ||
				payload.Provenance.Scope != test.scope || len(payload.Data.Rows) != 1 ||
				!reflect.DeepEqual(payload.Data.Rows[0], []any{float64(7), float64(3), "open_cursors", "310", nil}) {
				t.Fatalf("incorrect JSON evidence: %s %v", data, err)
			}
			terminal := errors.New("terminal cursor failure")
			steps[2].NextErr = terminal
			db, _ = testsql.New(t, steps...)
			data, err = info.New(db).PDBParametersJSON(t.Context(), test.ids, filter)
			if data != nil || !errors.Is(err, terminal) {
				t.Fatalf("failed cursor returned a prefix: %s %v", data, err)
			}
		})
	}
}

func TestPDBParametersRejectsInvalidFilterAndLegacy(t *testing.T) {
	db, trace := testsql.New(t)
	data, err := info.New(db).PDBParametersJSON(t.Context(), []int64{2}, info.ParameterFilter{})
	if data != nil || !errors.Is(err, parameters.ErrInvalidFilter) || len(trace.Calls()) != 0 {
		t.Fatalf("invalid PDB filter used the database: %v", err)
	}
	db, trace = testsql.New(t, testsql.Step{
		SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"11.2.0.4.0"}},
	})
	rows, err := info.New(db).PDBParameters(t.Context(), nil, info.ParameterFilter{})
	if rows != nil || !errors.Is(err, parameters.ErrUnsupportedVersion) || len(trace.Calls()) != 1 {
		t.Fatalf("legacy connection executed the PDB projection: %v", err)
	}
}

func TestPDBParametersUnknownScopeStopsBeforeParameterQuery(t *testing.T) {
	db, trace := testsql.New(t,
		testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"19.0.0.0.0"}}},
		testsql.Step{SQL: identity.ConnectedArchitectureSQL, Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}},
	)
	data, err := info.New(db).PDBParametersJSON(t.Context(), nil, info.ParameterFilter{})
	if data != nil || !errors.Is(err, info.ErrArchitectureUnavailable) || len(trace.Calls()) != 2 {
		t.Fatalf("unknown container reached the PDB query: %s %v", data, err)
	}
}
