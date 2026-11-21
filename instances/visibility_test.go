package instances_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func visibilityFixture(blocked, container bool) session.Result {
	source := fixture(container)
	if blocked {
		change(&source, "BLOCKED", session.Value{Null: true})
	} else {
		source.Columns = source.Columns[:len(source.Columns)-1]
		source.Rows[0] = source.Rows[0][:len(source.Rows[0])-1]
	}
	return source
}

func TestVersionedVisibilityReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		version            string
		major              int
		blocked, container bool
	}{
		{"9.2.0.8.0", 9, false, false},
		{"10.1.0.5.0", 10, false, false},
		{"10.2.0.5.0", 10, false, false},
		{"11.1.0.7.0", 11, true, false},
		{"11.2.0.4.0", 11, true, false},
		{"12.1.0.2.0", 12, true, true},
		{"12.2.0.1.0", 12, true, true},
		{"18.0.0.0.0", 18, true, true},
		{"19.32.0.0.0", 19, true, true},
		{"21.3.0.0.0", 21, true, true},
		{"23.2.0.0.0", 23, true, true},
		{"23.9.0.0.0", 23, true, true},
		{"23.26.1.0.0", 23, true, true},
		{"23.26.4.0.0", 23, true, true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := instances.VisibilityOptions{OracleVersion: tc.version}
			source := visibilityFixture(tc.blocked, tc.container)
			got, err := instances.VisibilityStatement(options)
			expressions := make([]string, len(source.Columns))
			for i, name := range source.Columns {
				expressions[i] = "i." + name + " AS " + name
			}
			want := "SELECT " + strings.Join(expressions, ", ") + " FROM GV$INSTANCE i"
			order := " ORDER BY i.INST_ID"
			if tc.container {
				want += " WHERE i.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))"
				order += ", i.CON_ID"
			}
			want += order + ", i.INSTANCE_NUMBER"
			if err != nil || got != want {
				t.Fatalf("visibility query = %q, %v; want %q", got, err, want)
			}
			if tc.major >= 11 {
				legacy, err := instances.Statement(tc.major)
				if err != nil || legacy != got {
					t.Fatalf("existing selection differs: %q, %v", legacy, err)
				}
			}
			source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
			change(&source, "INSTANCE_NAME", session.Value{Text: "second observation"})
			facts, err := instances.DecodeVisibility(source, options)
			if err != nil || facts.State != instances.Available || facts.Provenance.OracleVersion != tc.version ||
				facts.Provenance.Complete || facts.Provenance.OracleMajor != tc.major || facts.Provenance.Source != "GV$INSTANCE" || len(facts.Rows) != 2 {
				t.Fatalf("facts = %+v, %v", facts, err)
			}
			row := facts.Rows[0]
			if row.InstanceID.Text != "7" || row.Name.Text != " Node7 " || row.Status.Text != "MOUNTED" || !row.HostName.Null ||
				facts.Rows[1].InstanceID.Text != "7" || facts.Rows[1].Name.Text != "second observation" ||
				(row.Blocked != nil) != tc.blocked || tc.blocked && !row.Blocked.Null ||
				(row.ContainerID != nil) != tc.container || tc.container && row.ContainerID.Text != "0" {
				t.Fatalf("incorrect instance observations: %+v", facts.Rows)
			}
			source.Rows = nil
			empty, err := instances.DecodeVisibility(source, options)
			if err != nil || empty.State != instances.NoRows || empty.Rows == nil || len(empty.Rows) != 0 {
				t.Fatalf("empty = %+v, %v", empty, err)
			}
		})
	}
}

type visibilityQueryer func(context.Context, string, ...any) (*sql.Rows, error)

func (f visibilityQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return f(ctx, query, args...)
}

func visibilityNativeStep(t *testing.T, source session.Result) testsql.Step {
	t.Helper()
	step := testsql.Step{Columns: source.Columns}
	for _, row := range source.Rows {
		values := make([]driver.Value, len(row))
		for i, value := range row {
			if value.Null {
				continue
			}
			switch source.Columns[i] {
			case "INST_ID", "INSTANCE_NUMBER", "CON_ID":
				n, err := strconv.ParseInt(value.Text, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				values[i] = n
			default:
				values[i] = value.Text
			}
		}
		step.Values = append(step.Values, values)
	}
	return step
}

func TestVersionedVisibilityNativeAndAtomicJSON(t *testing.T) {
	options := instances.VisibilityOptions{OracleVersion: "9.2.0.8.0"}
	source := visibilityFixture(false, false)
	step := visibilityNativeStep(t, source)
	step.SQL, _ = instances.VisibilityStatement(options)
	db, calls := testsql.New(t, step, step, step)
	var native *sql.Rows
	rows, err := instances.VisibilityRows(t.Context(), visibilityQueryer(func(ctx context.Context, text string, args ...any) (*sql.Rows, error) {
		rows, err := db.QueryContext(ctx, text, args...)
		native = rows
		return rows, err
	}), options)
	if err != nil || rows == nil || rows != native || calls.Closed() != 0 || !rows.Next() {
		t.Fatalf("caller cursor = %p, original = %p, err = %v", rows, native, err)
	}
	var id, number int64
	var name, host, status, dbStatus, parallel sql.NullString
	if err := rows.Scan(&id, &number, &name, &host, &status, &dbStatus, &parallel); err != nil ||
		id != 7 || number != 7 || name.String != " Node7 " || host.Valid || status.String != "MOUNTED" || parallel.String != "YES" {
		t.Fatalf("unexpected cursor row: %d, %d, %v, %v, %v, %v", id, number, name, host, status, err)
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected extra row or terminal error: %v", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := instances.VisibilityJSON(t.Context(), db, options, rowset.Limits{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance instances.VisibilityProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(envelope.Data.Columns))
	for i, column := range envelope.Data.Columns {
		names[i] = column.Name
	}
	want := [][]any{{float64(7), float64(7), " Node7 ", nil, "MOUNTED", "ACTIVE", "YES"}}
	if !envelope.Provenance.Complete || envelope.Provenance.OracleVersion != options.OracleVersion || !reflect.DeepEqual(names, source.Columns) ||
		!reflect.DeepEqual(envelope.Data.Rows, want) || calls.Closed() != 2 {
		t.Fatalf("unexpected visibility envelope: %s", payload)
	}
	short, err := instances.VisibilityJSON(t.Context(), db, options, rowset.Limits{MaxRows: 1, MaxBytes: len(payload) - 1})
	if short != nil || err == nil || len(calls.Calls()) != 3 || calls.Closed() != 3 {
		t.Fatalf("short budget published %s, %v; queries=%d closed=%d", short, err, len(calls.Calls()), calls.Closed())
	}

	source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
	change(&source, "INST_ID", session.Value{Text: "0"})
	badDB, state := testsql.New(t, visibilityNativeStep(t, source))
	payload, err = instances.VisibilityJSON(t.Context(), badDB, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, instances.ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatalf("malformed final row published: %s, %v", payload, err)
	}
}

func TestVersionedVisibilityReadOutcomes(t *testing.T) {
	ref, err := reference.NewReference("unit", "instances", "versioned", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := instances.VisibilityRequest{Options: instances.VisibilityOptions{OracleVersion: "10.2.0.5.0"}, Reference: ref}
	source := visibilityFixture(false, false)
	text, _ := instances.VisibilityStatement(request.Options)
	calls := 0
	ok := readerFunc(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || s.Reference != ref || s.Text != text || len(s.Args) != 0 {
			t.Fatalf("incorrect statement: %+v", s)
		}
		return source, nil
	})
	facts, err := instances.ReadVisibility(t.Context(), ok, request)
	if err != nil || facts.State != instances.Available || !facts.Provenance.Complete || len(facts.Rows) != 1 || calls != 1 {
		t.Fatalf("read visibility = %+v, %v; calls = %d", facts, err, calls)
	}
	denied := &session.Error{Code: "ORA-01031"}
	unresolved := errors.New("ORA-00942: private log text")
	canceled := errors.Join(denied, context.Canceled)
	for _, tc := range []struct {
		cause, class error
		state        instances.State
	}{
		{denied, instances.ErrSourceInaccessible, instances.Inaccessible},
		{unresolved, instances.ErrSourceUnavailable, instances.Unavailable},
		{canceled, nil, instances.Failed},
	} {
		r := readerFunc(func(context.Context, session.Statement) (session.Result, error) { return source, tc.cause })
		got, err := instances.ReadVisibility(t.Context(), r, request)
		if got.State != tc.state || got.Rows != nil || !errors.Is(err, tc.cause) ||
			tc.class != nil && !errors.Is(err, tc.class) || tc.class == nil && errors.Is(err, instances.ErrSourceInaccessible) ||
			strings.Contains(err.Error(), "private") {
			t.Fatalf("incorrect failure: %+v, %v", got, err)
		}
	}
	for _, version := range []string{"9.0.1.4.0", "10.0", "11.3", "12.3", "26.1", "Oracle Database 26ai", "19", "19..0", "19.0.x", "19.4294967296"} {
		request.Options.OracleVersion = version
		got, err := instances.ReadVisibility(t.Context(), ok, request)
		if got.State != instances.Unsupported || got.Rows != nil || !errors.Is(err, instances.ErrUnsupportedVersion) || calls != 1 {
			t.Fatalf("%s: unsupported read = %+v, %v; calls = %d", version, got, err, calls)
		}
	}
}
