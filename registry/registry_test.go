package registry_test

import (
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/marcuoli/go-oracle-info/registry"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestStatementReleaseColumns(t *testing.T) {
	t.Parallel()
	for _, major := range []int{0, 10, 11, 12, 17, 18, 19, 23, 26} {
		t.Run(strconv.Itoa(major), func(t *testing.T) {
			got, err := registry.Statement(major)
			if major < 11 {
				if got != "" || !errors.Is(err, registry.ErrUnsupportedVersion) {
					t.Fatalf("%q %v", got, err)
				}
				return
			}
			columns := "COMP_ID AS COMP_ID, COMP_NAME AS COMP_NAME, VERSION AS VERSION"
			if major >= 18 {
				columns += ", VERSION_FULL AS VERSION_FULL"
			}
			want := "SELECT " + columns + ", STATUS AS STATUS, MODIFIED AS MODIFIED FROM DBA_REGISTRY ORDER BY COMP_ID"
			if err != nil || got != want {
				t.Fatalf("%q %v, want %q", got, err, want)
			}
		})
	}
}

func registryResult() session.Result {
	return session.Result{Columns: []string{"COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"}, Rows: [][]session.Value{{{Text: "CATALOG"}, {Text: "Dictionary α"}, {Text: "19.0.0.0.0"}, {Text: "19.30.0.0.0"}, {Text: "OPTION OFF"}, {Text: " 14-NOV-2026 22:41:00 "}}}}
}

func TestDecodePreservesRawFactsAndNulls(t *testing.T) {
	t.Parallel()
	r := registryResult()
	r.Rows = append(r.Rows, []session.Value{{Text: "CATALOG"}, {Null: true}, {Text: ""}, {Null: true}, {Text: "future status"}, {Null: true}})
	full := sql.NullString{String: "19.30.0.0.0", Valid: true}
	null := sql.NullString{}
	want := []registry.Component{
		{ID: sql.NullString{String: "CATALOG", Valid: true}, Name: sql.NullString{String: "Dictionary α", Valid: true}, Version: sql.NullString{String: "19.0.0.0.0", Valid: true}, VersionFull: &full, Status: sql.NullString{String: "OPTION OFF", Valid: true}, Modified: sql.NullString{String: " 14-NOV-2026 22:41:00 ", Valid: true}},
		{ID: sql.NullString{String: "CATALOG", Valid: true}, Name: null, Version: sql.NullString{Valid: true}, VersionFull: &null, Status: sql.NullString{String: "future status", Valid: true}, Modified: null},
	}
	got, err := registry.Decode(r, 19)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v %v, want %+v", got, err, want)
	}
	got[0].VersionFull.String = "changed"
	if got[1].VersionFull.Valid || r.Rows[0][3].Text != "19.30.0.0.0" {
		t.Fatal("rows share mutable full-version storage")
	}
	for i := range r.Columns {
		r.Rows[0][i] = session.Value{Null: true, Text: "ignored"}
	}
	allNull, err := registry.Decode(session.Result{Columns: r.Columns, Rows: r.Rows[:1]}, 19)
	if err != nil || !reflect.DeepEqual(allNull, []registry.Component{{VersionFull: &null}}) {
		t.Fatalf("NULLs: %+v %v", allNull, err)
	}
}

func TestDecodeLegacyAndEmptyInventory(t *testing.T) {
	t.Parallel()
	r := session.Result{Columns: []string{"modified", "status", "version", "comp_name", "comp_id"}, Rows: [][]session.Value{{{Text: "raw local label"}, {Text: "VALID"}, {Text: "11.2.0.4.0"}, {Text: "Dictionary"}, {Text: "CATALOG"}}}}
	got, err := registry.Decode(r, 11)
	if err != nil || len(got) != 1 || got[0].VersionFull != nil || got[0].Version.String != "11.2.0.4.0" || got[0].ID.String != "CATALOG" || got[0].Modified.String != "raw local label" {
		t.Fatalf("legacy: %+v %v", got, err)
	}
	r.Rows = nil
	empty, err := registry.Decode(r, 11)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
}

func TestDecodeRejectsInvalidCompleteResultContracts(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		major  int
		change func(*session.Result)
		want   error
	}{
		{"unknown", 0, func(r *session.Result) {}, registry.ErrUnsupportedVersion},
		{"modern columns on legacy", 11, func(r *session.Result) {}, registry.ErrInvalidEvidence},
		{"missing column", 19, func(r *session.Result) { r.Columns = r.Columns[:5] }, registry.ErrInvalidEvidence},
		{"wrong column", 19, func(r *session.Result) { r.Columns[0] = "ID" }, registry.ErrInvalidEvidence},
		{"duplicate column", 19, func(r *session.Result) { r.Columns[1] = "comp_id" }, registry.ErrInvalidEvidence},
		{"short row", 19, func(r *session.Result) { r.Rows[0] = r.Rows[0][:5] }, registry.ErrInvalidEvidence},
		{"extra value", 19, func(r *session.Result) { r.Rows[0] = append(r.Rows[0], session.Value{}) }, registry.ErrInvalidEvidence},
		{"late malformed row", 19, func(r *session.Result) { r.Rows = append(r.Rows, nil) }, registry.ErrInvalidEvidence},
		{"empty wrong projection", 19, func(r *session.Result) { r.Rows = nil; r.Columns[0] = "ID" }, registry.ErrInvalidEvidence},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := registryResult()
			tt.change(&r)
			got, err := registry.Decode(r, tt.major)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("got %+v %v, want nil %v", got, err, tt.want)
			}
		})
	}
}
