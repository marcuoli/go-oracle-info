package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func databaseFileFixture() session.Result {
	return session.Result{Columns: []string{"FILE_TYPE", "FILE_ID", "FILE_NAME", "BYTES", "CON_ID"}, Rows: [][]session.Value{
		{{Text: "DATAFILE"}, {Text: "1"}, {Text: "+DATA/db/system.dbf"}, {Text: "9007199254740993"}, {Text: "1"}},
		{{Text: "DATAFILE"}, {Text: "2"}, {Text: "+DATA/db/app.dbf"}, {Text: "1024"}, {Text: "3"}},
		{{Text: "TEMPFILE"}, {Text: "1"}, {Text: "/data/temp01.dbf"}, {Text: "4096"}, {Text: "3"}},
	}}
}

func TestDatabaseFilesExactAllocationAndStorage(t *testing.T) {
	cases := []struct {
		name, kind string
		change     func(*session.Result)
		wantSize   string
		missing    bool
	}{
		{"mixed", "ASM+FileSystem", nil, "9007199254742017", false},
		{"asm", "ASM", func(r *session.Result) { r.Rows[2][2].Text = "+TEMP/temp01.dbf" }, "9007199254742017", false},
		{"filesystem", "FileSystem", func(r *session.Result) {
			for i := range r.Rows {
				r.Rows[i][2].Text = "/data/file.dbf"
			}
		}, "9007199254742017", false},
		{"inaccessible", "ASM+FileSystem", func(r *session.Result) { r.Rows[0][3].Text = "0" }, "", true},
		{"null_size", "ASM+FileSystem", func(r *session.Result) { r.Rows[0][3] = session.Value{Null: true} }, "", true},
		{"missing_path", "", func(r *session.Result) { r.Rows[0][2] = session.Value{Null: true} }, "9007199254742017", false},
		{"exascale_only", "", func(r *session.Result) {
			for i := range r.Rows {
				r.Rows[i][2].Text = "@vault/db/file.dbf"
			}
		}, "9007199254742017", false},
		{"asm_exascale", "", func(r *session.Result) { r.Rows[2][2].Text = "@vault/db/temp.dbf" }, "9007199254742017", false},
		{"empty", "", func(r *session.Result) { r.Rows = nil }, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := databaseFileFixture()
			if tc.change != nil {
				tc.change(&source)
			}
			reader := &fixtureReader{result: source}
			result, err := ReadDatabaseFiles(t.Context(), reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
			if err != nil {
				t.Fatal(err)
			}
			size := result.DatafileBytes()
			if size.Null != tc.missing || size.Text != tc.wantSize {
				t.Fatal("allocation rounded, included tempfiles, or accepted unknown size", size)
			}
			kind := result.StorageType()
			if kind.Text != tc.kind || kind.Null != (tc.kind == "") {
				t.Fatal("storage classification differs from file evidence", kind)
			}
			if !result.Provenance.Complete || len(reader.calls) != 1 || !strings.Contains(reader.calls[0].Text, "V$DATAFILE") {
				t.Fatal("source or completion provenance lost")
			}
		})
	}
}

func TestDatabaseFilesRejectLateMalformedOrDuplicateRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"bad_kind", func(r *session.Result) { r.Rows[2][0].Text = "REDO" }},
		{"negative_bytes", func(r *session.Result) { r.Rows[2][3].Text = "-1" }},
		{"duplicate", func(r *session.Result) { r.Rows = append(r.Rows, r.Rows[0]) }},
		{"numeric_file_id_alias", func(r *session.Result) {
			row := append([]session.Value(nil), r.Rows[0]...)
			row[1].Text = "01"
			r.Rows = append(r.Rows, row)
		}},
		{"numeric_container_alias", func(r *session.Result) {
			row := append([]session.Value(nil), r.Rows[0]...)
			row[4].Text = "01"
			r.Rows = append(r.Rows, row)
		}},
		{"numeric_both_aliases", func(r *session.Result) {
			row := append([]session.Value(nil), r.Rows[0]...)
			row[1].Text = "01"
			row[4].Text = "01"
			r.Rows = append(r.Rows, row)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := databaseFileFixture()
			tc.change(&source)
			got, err := DecodeDatabaseFiles(source, 19)
			if err == nil || len(got.Rows) != 0 {
				t.Fatal("malformed or duplicate late row published a prefix", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader := &fixtureReader{result: databaseFileFixture(), after: cancel}
	got, err := ReadDatabaseFiles(ctx, reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if !errors.Is(err, context.Canceled) || len(got.Rows) != 0 {
		t.Fatal("post-query cancellation published allocation", got, err)
	}
}

func TestDatabaseFilesNativeAndJSONOwnership(t *testing.T) {
	source := databaseFileFixture()
	sql, _ := DatabaseFilesStatement(19)
	step := testsql.Step{SQL: sql, Columns: source.Columns}
	for _, row := range source.Rows {
		values := make([]driver.Value, len(row))
		for i, v := range row {
			values[i] = v.Text
		}
		step.Values = append(step.Values, values)
	}
	db, state := testsql.New(t, step)
	rows, err := DatabaseFilesRows(t.Context(), db, 19)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil || count != 3 {
		t.Fatal("native cursor incomplete", count, err)
	}
	_ = state
	db, _ = testsql.New(t, step)
	payload, err := DatabaseFilesJSON(t.Context(), db, 19, rowset.Limits{MaxRows: 10, MaxBytes: 32768})
	if err != nil || !strings.Contains(string(payload), "9007199254740993") {
		t.Fatal("JSON lost exact bytes", string(payload), err)
	}
	db, _ = testsql.New(t, step)
	payload, err = DatabaseFilesJSON(t.Context(), db, 19, rowset.Limits{MaxRows: 1, MaxBytes: 32768})
	if err == nil || len(payload) != 0 {
		t.Fatal("over-budget JSON published a prefix")
	}
}

func TestDatabaseFilesRetainOriginalNumericIdentityText(t *testing.T) {
	source := databaseFileFixture()
	source.Rows[0][1].Text = "01"
	source.Rows[0][4].Text = "01"
	facts, err := ReadDatabaseFiles(t.Context(), &fixtureReader{result: source}, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if err != nil || facts.Rows[0].FileID.Text != "01" || facts.Rows[0].ContainerID.Text != "01" || facts.DatafileBytes().Text != "9007199254742017" {
		t.Fatal("duplicate-key normalization changed raw identity or allocation", facts, err)
	}
}
