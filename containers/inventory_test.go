package containers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestContainerReleaseSelection(t *testing.T) {
	tests := []struct {
		version   string
		inventory bool
		total     bool
		modern    bool
	}{
		{"9.2.0.8.0", false, false, false},
		{"10.1.0.5.0", false, false, false},
		{"10.2.0.5.0", false, false, false},
		{"11.1.0.7.0", false, false, false},
		{"11.2.0.4.0", false, false, false},
		{"12.1", false, false, false},
		{"12.1.0.1.0", true, false, false},
		{"12.1.0.2.0", true, true, false},
		{"12.2.0.1.0", true, true, true},
		{"18.20.0.0.0", true, true, true},
		{"19.32.0.0.0", true, true, true},
		{"21.21.0.0.0", true, true, true},
		{"23.4.0.0.0", true, true, true},
		{"23.9.0.0.0", true, true, true},
		{"23.26.1.0.0", true, true, true},
		{"23.26.4.0.0", true, true, true},
		{"26.0.0.0.0", false, false, false},
		{"23ai", false, false, false},
		{"23.26.-1.0.0", false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			p, err := inventoryPlan(InventoryOptions{OracleVersion: tc.version, Scope: Root})
			if !tc.inventory {
				if !errors.Is(err, ErrUnsupportedVersion) || p.sql != "" {
					t.Fatalf("inventory SQL = %q, err = %v", p.sql, err)
				}
				db, state := testsql.New(t)
				payload, err := InventoryJSON(t.Context(), db, InventoryOptions{tc.version, Root}, rowset.Limits{})
				if payload != nil || !errors.Is(err, ErrUnsupportedVersion) || len(state.Calls()) != 0 {
					t.Fatalf("unsupported inventory queried = %q, %v, %v", payload, err, state.Calls())
				}
			} else {
				if err != nil || !strings.Contains(p.sql, "FROM GV$CONTAINERS v") ||
					strings.Contains(p.sql, " AS TOTAL_SIZE") != tc.total ||
					strings.Contains(p.sql, " AS MAX_SIZE") != tc.modern ||
					strings.Contains(p.sql, " AS APPLICATION_ROOT_CON_ID") != tc.modern ||
					strings.Contains(p.sql, "PRIORITY") {
					t.Fatalf("inventory SQL = %q, err = %v", p.sql, err)
				}
			}
			sql, err := SavedStatesStatement(SavedStatesOptions{tc.version, Root})
			if tc.total {
				if err != nil || !strings.Contains(sql, "FROM DBA_PDB_SAVED_STATES v") ||
					!strings.Contains(sql, "ORDER BY v.CON_ID, v.INSTANCE_NAME") || strings.Contains(sql, "JOIN") {
					t.Fatalf("saved-state SQL = %q, err = %v", sql, err)
				}
			} else if !errors.Is(err, ErrUnsupportedVersion) || sql != "" {
				t.Fatalf("saved-state SQL = %q, err = %v", sql, err)
			}
		})
	}
	if _, err := InventoryStatement(InventoryOptions{"19.32", NonCDB}); !errors.Is(err, ErrUnsupportedScope) {
		t.Fatal(err)
	}
	if _, err := SavedStatesStatement(SavedStatesOptions{"23.26.4", CurrentContainer}); !errors.Is(err, ErrUnsupportedScope) {
		t.Fatal(err)
	}
}

func sessionValue(text string) session.Value { return session.Value{Text: text} }

func scopeFixture(p selection, values ...session.Value) session.Result {
	columns := make([]string, len(p.schema))
	for i, col := range p.schema {
		columns[i] = col.Name
	}
	return session.Result{Columns: columns, Rows: [][]session.Value{values}}
}

func TestInventoryPreservesAbsenceAndRejectsWrongScope(t *testing.T) {
	options := InventoryOptions{"12.1.0.1.0", CurrentContainer}
	p, err := inventoryPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	guidText := "F0000000000000000000000000000005"
	row := scopeFixture(p, sessionValue("2"), sessionValue("3"), sessionValue("3"), sessionValue("MixedCase"),
		sessionValue("MOUNTED"), session.Value{Null: true}, sessionValue("4294967295"),
		sessionValue("9007199254740993"), sessionValue(guidText))
	row.Rows = append(row.Rows, slices.Clone(row.Rows[0]))
	facts, err := DecodeInventory(row, options)
	if err != nil || len(facts.Rows) != 2 || facts.Provenance.Complete || facts.Rows[0].GUID.Text != guidText ||
		!facts.Rows[0].Restricted.Null || facts.Rows[0].TotalSizeBytes != nil || facts.Rows[0].MaxSizeBytes != nil {
		t.Fatalf("inventory evidence = %#v, err = %v", facts, err)
	}
	row.Rows[1][2] = sessionValue("1")
	facts, err = DecodeInventory(row, options)
	if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("partial inventory = %#v, err = %v", facts, err)
	}
}

type readerFunc func(context.Context, session.Statement) (session.Result, error)

func (r readerFunc) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	return r(ctx, statement)
}

func TestSavedStatesStatusAndInactiveInstance(t *testing.T) {
	options := SavedStatesOptions{"23.26.4.0.0", Root}
	p, err := savedStatesPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	result := scopeFixture(p, sessionValue("3"), sessionValue("MixedPDB"), sessionValue("inactive_member"),
		sessionValue("4294967295"), session.Value{Null: true}, sessionValue("OPEN"), session.Value{Null: true})
	ref, err := reference.NewReference("unit", "containers", "saved", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := SavedStatesRequest{options, ref}
	read := readerFunc(func(_ context.Context, got session.Statement) (session.Result, error) {
		if got.Text != p.sql || got.Reference != ref || len(got.Args) != 0 {
			t.Fatal(got)
		}
		return result, nil
	})
	observed, err := ReadSavedStates(t.Context(), read, request)
	if err != nil || len(observed.Rows) != 1 || !observed.Provenance.Complete ||
		observed.Rows[0].InstanceName.Text != "inactive_member" || !observed.Rows[0].GUID.Null ||
		observed.Rows[0].State.Text != "OPEN" {
		t.Fatalf("saved definitions = %#v, err = %v", observed, err)
	}
	result.Rows = nil
	empty, err := ReadSavedStates(t.Context(), read, request)
	if err != nil || !empty.Provenance.Complete || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatal(empty, err)
	}
	for _, tc := range []struct {
		message string
		outcome Outcome
	}{
		{"ORA-00942: object resolution failed in private lookup", Unavailable},
		{"ORA-01031: private source permission", Inaccessible},
		{"private interrupted transport", Failed},
	} {
		cause := errors.New(tc.message)
		got, err := ReadSavedStates(t.Context(), readerFunc(func(context.Context, session.Statement) (session.Result, error) {
			return result, cause
		}), request)
		var failed *ReadError
		if got.Rows != nil || !errors.As(err, &failed) || failed.Outcome != tc.outcome || !errors.Is(err, cause) ||
			strings.Contains(err.Error(), tc.message) {
			t.Fatalf("saved result = %#v, error = %v", got, err)
		}
	}
}

type spyQueryer struct {
	db     *sql.DB
	cursor *sql.Rows
}

func (s *spyQueryer) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, statement, args...)
	s.cursor = rows
	return rows, err
}

func TestInventoryNativeCursorAndAtomicJSON(t *testing.T) {
	options := InventoryOptions{"23.26.4.0.0", Root}
	p, err := inventoryPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	columns := make([]string, len(p.schema))
	for i := range p.schema {
		columns[i] = p.schema[i].Name
	}
	value := []driver.Value{int64(1), int64(3), int64(1), "P1", "MOUNTED", nil, "9007199254740993",
		"4294967295", nil, "18446744073709551616", nil, "NO", "NO", "NO", nil}
	step := testsql.Step{SQL: p.sql, Columns: columns, Values: [][]driver.Value{value}}
	db, state := testsql.New(t, step, step, step, step)
	spy := &spyQueryer{db: db}
	rows, err := InventoryRows(t.Context(), spy, options)
	if err != nil || rows != spy.cursor || state.Closed() != 0 || !rows.Next() {
		t.Fatal(rows, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := InventoryJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `"9007199254740993","4294967295",null,"18446744073709551616",null`) ||
		!strings.Contains(string(payload), `"complete":true`) || state.Closed() != 2 {
		t.Fatalf("inventory JSON = %s, err = %v", payload, err)
	}
	exact, err := InventoryJSON(t.Context(), db, options, rowset.Limits{MaxBytes: len(payload)})
	if err != nil || string(exact) != string(payload) {
		t.Fatal(string(exact), err)
	}
	short, err := InventoryJSON(t.Context(), db, options, rowset.Limits{MaxBytes: len(payload) - 1})
	if short != nil || !errors.Is(err, rowset.ErrCapacity) || state.Closed() != 4 {
		t.Fatal(string(short), err, state.Closed())
	}
	late := slices.Clone(value)
	late[2] = int64(7)
	step.Values = append(step.Values, late)
	db, state = testsql.New(t, step)
	payload, err = InventoryJSON(t.Context(), db, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
}
