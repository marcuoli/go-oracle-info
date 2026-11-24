package containers

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestPlugInViolationsReleaseAndScope(t *testing.T) {
	cases := []struct {
		version string
		scope   Scope
		cols    int
		want    error
	}{
		{"9.2.0.8.0", Root, 0, ErrUnsupportedVersion},
		{"10.1.0.5.0", Root, 0, ErrUnsupportedVersion},
		{"10.2.0.5.0", Root, 0, ErrUnsupportedVersion},
		{"11.1.0.7.0", Root, 0, ErrUnsupportedVersion},
		{"11.2.0.4.0", Root, 0, ErrUnsupportedVersion},
		{"12.1", Root, 0, ErrUnsupportedVersion},
		{"12.1.0.1.0", Root, 9, nil},
		{"12.1.0.2.0", Root, 9, nil},
		{"12.1.0.2.0", CurrentContainer, 0, ErrUnsupportedScope},
		{"12.2.0.1.0", Root, 10, nil},
		{"18.1.0.0.0", Root, 10, nil},
		{"19.32.0.0.0", Root, 10, nil},
		{"21.21.0.0.0", Root, 10, nil},
		{"23.9.0.0.0", Root, 10, nil},
		{"23.26.1.0.0", CurrentContainer, 10, nil},
		{"23.26.4.0.0", Root, 10, nil},
		{"23.26.4.0.0", NonCDB, 0, ErrUnsupportedScope},
		{"26ai", Root, 0, ErrUnsupportedVersion},
	}
	for _, tc := range cases {
		t.Run(tc.version+"/"+string(tc.scope), func(t *testing.T) {
			options := PlugInViolationsOptions{tc.version, tc.scope}
			p, err := plugInViolationsPlan(options)
			if !errors.Is(err, tc.want) || len(p.schema) != tc.cols {
				t.Fatal(p, err)
			}
			if tc.want != nil {
				db, state := testsql.New(t)
				got, err := PlugInViolationsJSON(t.Context(), db, options, rowset.Limits{})
				if got != nil || !errors.Is(err, tc.want) || len(state.Calls()) != 0 {
					t.Fatal(got, err, state.Calls())
				}
				return
			}
			predicate := connectedContainer + " = 1"
			if tc.scope == CurrentContainer {
				predicate = "v.CON_ID >= 1 AND v.CON_ID = " + connectedContainer
			}
			if !strings.Contains(p.sql, "FROM PDB_PLUG_IN_VIOLATIONS v WHERE "+predicate+" ORDER BY") ||
				strings.Contains(p.sql, " AS CON_ID") != (tc.cols == 10) ||
				!strings.Contains(p.sql, "'FM00D000000'") || strings.Contains(p.sql, "PENDING") {
				t.Fatal(p.sql)
			}
		})
	}
}

func TestPlugInViolationsTypedEvidence(t *testing.T) {
	options := PlugInViolationsOptions{"23.26.1.0.0", Root}
	p, err := plugInViolationsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	null := session.Value{Null: true}
	result := scopeFixture(p, sessionValue("2026-11-24T12:23:45.000037"), sessionValue("Prospective"),
		sessionValue("SQL Patch"), sessionValue("WARNING"), sessionValue("-9007199254740993"),
		sessionValue("0"), sessionValue("line\nmessage"), sessionValue("PENDING"), null, sessionValue("0"))
	for _, status := range []session.Value{sessionValue("RESOLVED"), sessionValue("IGNORE"), null} {
		row := append([]session.Value(nil), result.Rows[0]...)
		row[7], row[9] = status, null
		result.Rows = append(result.Rows, row)
	}
	ref, err := reference.NewReference("unit", "containers", "plug-in-violations", 1)
	if err != nil {
		t.Fatal(err)
	}
	reader := readerFunc(func(_ context.Context, stmt session.Statement) (session.Result, error) {
		if stmt.Reference != ref || stmt.Text != p.sql || len(stmt.Args) != 0 {
			t.Fatal(stmt)
		}
		return result, nil
	})
	request := PlugInViolationsRequest{Options: options, Reference: ref}
	want, err := ReadPlugInViolations(t.Context(), reader, request)
	if err != nil || len(want.Rows) != 4 || !want.Provenance.Complete {
		t.Fatal(want, err)
	}
	for i, status := range []session.Value{sessionValue("PENDING"), sessionValue("RESOLVED"), sessionValue("IGNORE"), null} {
		row := want.Rows[i]
		if row.Status != status || row.Time.Text != "2026-11-24T12:23:45.000037" || row.ErrorNumber.Text != "-9007199254740993" ||
			row.Name.Text != "Prospective" || !row.Action.Null || row.Message.Text != "line\nmessage" ||
			row.ContainerID == nil || row.ContainerID.Null != (i != 0) || (i == 0 && row.ContainerID.Text != "0") {
			t.Fatal(row)
		}
	}
	request.Limits.MaxRows = 3
	if got, err := ReadPlugInViolations(t.Context(), reader, request); got.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatal(got, err)
	}
	request.Limits = rowset.Limits{}
	result.Rows[len(result.Rows)-1][0] = sessionValue("2026-11-24T12:23:45,000037")
	if got, err := ReadPlugInViolations(t.Context(), reader, request); got.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal(got, err)
	}
	result.Rows = nil
	if got, err := ReadPlugInViolations(t.Context(), reader, request); err != nil || got.Rows == nil || len(got.Rows) != 0 || !got.Provenance.Complete {
		t.Fatal(got, err)
	}
	old := PlugInViolationsOptions{"12.1.0.2.0", Root}
	legacyPlan, err := plugInViolationsPlan(old)
	if err != nil {
		t.Fatal(err)
	}
	legacy := scopeFixture(legacyPlan, sessionValue("2026-11-24T12:23:45.000000"), sessionValue("OLDPDB"),
		sessionValue("CHECK"), sessionValue("ERROR"), null, sessionValue("1"), sessionValue("msg"), null, null)
	decoded, err := DecodePlugInViolations(legacy, old)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || decoded.Rows[0].ContainerID != nil ||
		!decoded.Rows[0].ErrorNumber.Null || !decoded.Rows[0].Status.Null {
		t.Fatal(decoded, err)
	}
}

func TestPlugInViolationsNativeRowsAndAtomicJSON(t *testing.T) {
	options := PlugInViolationsOptions{"23.26.1.0.0", CurrentContainer}
	p, err := plugInViolationsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	columns := make([]string, len(p.schema))
	for i, c := range p.schema {
		columns[i] = c.Name
	}
	step := testsql.Step{SQL: p.sql, Columns: columns, Values: [][]driver.Value{
		{"2026-11-24T05:06:07.900003", "LABPDB", "CHECK", "ERROR", nil, "7", "note", nil, nil, "3"},
	}}
	failedClose := step
	closeErr := errors.New("terminal cursor close failure")
	failedClose.CloseErr = closeErr
	db, state := testsql.New(t, step, step, failedClose)
	spy := &spyQueryer{db: db}
	rows, err := PlugInViolationsRows(t.Context(), spy, options)
	if err != nil || rows != spy.cursor || state.Closed() != 0 || !rows.Next() {
		t.Fatal(rows, err, state.Closed())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := PlugInViolationsJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `"2026-11-24T05:06:07.900003","LABPDB","CHECK","ERROR",null,"7","note",null,null,"3"`) ||
		!strings.Contains(string(payload), `"complete":true`) {
		t.Fatal(string(payload), err)
	}
	if got, err := PlugInViolationsJSON(t.Context(), db, options, rowset.Limits{}); got != nil || !errors.Is(err, closeErr) || state.Closed() != 3 {
		t.Fatal(string(got), err, state.Closed())
	}
	step.Values[0][9] = "0"
	db, state = testsql.New(t, step)
	if got, err := PlugInViolationsJSON(t.Context(), db, options, rowset.Limits{}); got != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(got), err, state.Closed())
	}
}
