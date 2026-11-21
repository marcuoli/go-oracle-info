package identity

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestLogicalDatabaseReleaseProjection(t *testing.T) {
	const base = "SELECT TO_CHAR(d.DBID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS DBID, d.NAME AS NAME"
	for _, group := range []struct {
		versions []string
		wantSQL  string
		columns  []string
	}{
		{[]string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0"}, base + " FROM V$DATABASE d", []string{"DBID", "NAME"}},
		{[]string{"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.0.0.0.0", "19.32.0.0.0", "21.3.0.0.0", "23.2.0.0.0", "23.9.0.0.0", "23.26.1.0.0", "23.26.4.0.0"},
			base + ", d.CDB AS CDB FROM V$DATABASE d", []string{"DBID", "NAME", "CDB"}},
	} {
		for _, version := range group.versions {
			t.Run(version, func(t *testing.T) {
				options := LogicalDatabaseOptions{OracleVersion: version}
				statement, args, err := LogicalDatabaseStatement(options)
				if err != nil || statement != group.wantSQL || len(args) != 0 {
					t.Fatalf("statement=%q args=%v error=%v", statement, args, err)
				}
				row := []session.Value{{Text: "900719925474099312345"}, {Text: "MixedDB"}}
				if len(group.columns) == 3 {
					row = append(row, session.Value{Null: true})
				}
				facts, err := DecodeLogicalDatabase(session.Result{Columns: group.columns, Rows: [][]session.Value{row}}, options)
				if err != nil || facts.Provenance.Complete || facts.Provenance.Scope != "controlfile_database_identity" || len(facts.Rows) != 1 {
					t.Fatal(facts, err)
				}
				value := facts.Rows[0]
				if value.DBID != row[0] || value.Name != row[1] ||
					(len(row) == 2 && value.CDB != nil) || (len(row) == 3 && (value.CDB == nil || !value.CDB.Null)) {
					t.Fatal(value)
				}
			})
		}
	}
	for _, version := range []string{"", "9", "9.1.0.0.0", "10.3", "12.3", "26.0.0.0.0", "23ai", "23.+26"} {
		sql, args, err := LogicalDatabaseStatement(LogicalDatabaseOptions{OracleVersion: version})
		if sql != "" || args != nil || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatal(version, sql, args, err)
		}
	}
}

func TestLogicalDatabaseReadAndErrorStates(t *testing.T) {
	option := LogicalDatabaseOptions{OracleVersion: "10.2.0.5.0"}
	query, _, err := LogicalDatabaseStatement(option)
	if err != nil {
		t.Fatal(err)
	}
	request := LogicalDatabaseRequest{Options: option, Reference: coreReference(t)}
	denied := errors.New("ORA-1031 error occurred")
	mount := &session.Error{Code: "ORA-01507"}
	input := session.Result{Columns: []string{"DBID", "NAME"}}
	for _, tc := range []struct {
		name  string
		cause error
		class error
	}{
		{"complete empty", nil, nil},
		{"privilege denied", denied, ErrLogicalDatabaseInaccessible},
		{"unmounted database", mount, ErrLogicalDatabaseUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			reader := datasetTestReader(func(ctx context.Context, statement session.Statement) (session.Result, error) {
				calls++
				if ctx != t.Context() || statement.Text != query || statement.Reference != request.Reference || len(statement.Args) != 0 {
					t.Fatal(statement)
				}
				return input, tc.cause
			})
			facts, err := ReadLogicalDatabase(t.Context(), reader, request)
			if calls != 1 || !errors.Is(err, tc.cause) || (tc.class != nil && !errors.Is(err, tc.class)) {
				t.Fatal(facts, calls, err)
			}
			if tc.cause == nil && (!facts.Provenance.Complete || facts.Rows == nil || len(facts.Rows) != 0) {
				t.Fatal(facts)
			}
			if tc.cause != nil && (facts.Rows != nil || facts.Provenance.Complete) {
				t.Fatal(facts)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return input, denied
	})
	facts, err := ReadLogicalDatabase(ctx, reader, request)
	if facts.Rows != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, denied) || errors.Is(err, ErrLogicalDatabaseInaccessible) {
		t.Fatal(facts, err)
	}
	request.Options.OracleVersion = "8.1"
	reader = func(context.Context, session.Statement) (session.Result, error) {
		t.Fatal("unsupported selector reached the reader")
		return input, nil
	}
	if _, err := ReadLogicalDatabase(t.Context(), reader, request); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}

func TestLogicalDatabaseNativeCursorAndAtomicJSON(t *testing.T) {
	options := LogicalDatabaseOptions{OracleVersion: "23.26.1.0.0"}
	statement, _, err := LogicalDatabaseStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	row := []driver.Value{"4294967295", "MIXED DB", nil}
	step := testsql.Step{SQL: statement, Columns: []string{"DBID", "NAME", "CDB"}, Values: [][]driver.Value{row}}
	db, state := testsql.New(t, step)
	spy := &datasetSpy{db: db}
	cursor, err := LogicalDatabaseRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err := cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}

	db, state = testsql.New(t, step)
	data, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance DatasetProvenance
		Data       rowset.Table
	}
	if err != nil || json.Unmarshal(data, &envelope) != nil || !envelope.Provenance.Complete || state.Closed() != 1 ||
		!reflect.DeepEqual(envelope.Data.Rows, [][]any{{"4294967295", "MIXED DB", nil}}) {
		t.Fatal(string(data), err, state.Closed())
	}

	cases := []struct {
		name   string
		values [][]driver.Value
		want   error
	}{
		{"bad integer", [][]driver.Value{{"42.5", "A", "YES"}}, ErrInvalidEvidence},
		{"unexpected second row", [][]driver.Value{row, row}, ErrInvalidEvidence},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failed := step
			failed.Values = tc.values
			db, state := testsql.New(t, failed)
			data, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{})
			if data != nil || !errors.Is(err, tc.want) || state.Closed() != 1 {
				t.Fatal(string(data), err, state.Closed())
			}
		})
	}
	nulls := step
	nulls.Values = [][]driver.Value{{nil, nil, nil}}
	db, state = testsql.New(t, nulls)
	if payload, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{}); err != nil ||
		json.Unmarshal(payload, &envelope) != nil || !reflect.DeepEqual(envelope.Data.Rows, [][]any{{nil, nil, nil}}) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}

	terminal := step
	terminal.NextErr = errors.New("failure after the first row")
	db, state = testsql.New(t, terminal)
	if payload, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{}); payload != nil ||
		!errors.Is(err, terminal.NextErr) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}

	unavailable := step
	unavailable.QueryErr = errors.New("ORA-1507 database not mounted")
	db, _ = testsql.New(t, unavailable)
	if payload, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{}); payload != nil ||
		!errors.Is(err, unavailable.QueryErr) || !errors.Is(err, ErrLogicalDatabaseUnavailable) {
		t.Fatal(string(payload), err)
	}

	db, state = testsql.New(t, step)
	if data, err := LogicalDatabaseJSON(t.Context(), db, options, rowset.Limits{MaxBytes: len(data) - 1}); data != nil || !errors.Is(err, rowset.ErrCapacity) || state.Closed() != 1 {
		t.Fatal(string(data), err, state.Closed())
	}
}
