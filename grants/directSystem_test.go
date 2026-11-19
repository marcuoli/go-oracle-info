package grants

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func systemFixture() (Options, session.Result) {
	return Options{OracleMajor: 19, Names: []string{"NoQuotaUser"}}, session.Result{Columns: []string{"GRANTEE", "PRIVILEGE", "ADMIN_OPTION", "COMMON", "INHERITED"}, Rows: [][]session.Value{
		{{Text: "NoQuotaUser"}, {Text: "UNLIMITED TABLESPACE"}, {Text: "NO"}, {Text: "YES"}, {Text: "NO"}},
		{{Text: "NoQuotaUser"}, {Text: "CREATE SESSION"}, {Null: true}, {Null: true}, {Null: true}},
	}}
}
func TestDirectSystemIndependentSourceAndReleaseFlags(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26, 99} {
		o, input := systemFixture()
		o.OracleMajor = major
		if major == 11 {
			input.Columns = input.Columns[:3]
			for i := range input.Rows {
				input.Rows[i] = input.Rows[i][:3]
			}
		}
		if major == 12 {
			input.Columns = input.Columns[:4]
			for i := range input.Rows {
				input.Rows[i] = input.Rows[i][:4]
			}
		}
		query, args, err := DirectSystemStatement(o)
		if major == 9 || major == 10 || major == 26 || major == 99 {
			if !errors.Is(err, ErrUnsupportedVersion) || query != "" {
				t.Fatal(major, query, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(args, []any{"NoQuotaUser"}) || !strings.Contains(query, "FROM DBA_SYS_PRIVS t") || strings.Contains(query, "JOIN") || strings.Contains(query, "QUOTAS") || strings.Contains(query, "UNLIMITED TABLESPACE") || !strings.Contains(query, "NLSSORT(t.GRANTEE, 'NLS_SORT=BINARY')") {
			t.Fatal(query, args, err)
		}
		got, err := DecodeDirectSystem(input, o)
		if err != nil || len(got.Records) != 2 || got.Provenance.Source != "DBA_SYS_PRIVS" || got.Records[0].Grantee.Text != "NoQuotaUser" || got.Records[0].Privilege.Text != "UNLIMITED TABLESPACE" || got.Records[0].AdminOption.Text != "NO" || !got.Records[1].AdminOption.Null {
			t.Fatal(got, err)
		}
		if (got.Records[0].Common != nil) != (major >= 12) || (got.Records[0].Inherited != nil) != (major >= 18) {
			t.Fatal(got)
		}
		if major >= 18 && (got.Records[0].Common.Text != "YES" || got.Records[0].Inherited.Text != "NO" || !got.Records[1].Common.Null || !got.Records[1].Inherited.Null) {
			t.Fatal(got)
		}
		b, _ := json.Marshal(struct {
			Package, Fact string
			Major         int
			SQL           string
			Args          []any
		}{"grants", "DirectSystem", major, query, args})
		t.Log(string(b))
	}
	if _, _, e := DirectSystemStatement(Options{OracleMajor: 19, Owners: []string{"OWNER"}}); !errors.Is(e, ErrInvalidRequest) {
		t.Fatal(e)
	}
}
func TestDirectSystemReadCompletionAndScope(t *testing.T) {
	o, input := systemFixture()
	ref, _ := reference.NewReference("unit", "grants", "system", 1)
	calls := 0
	reader := fixtureReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref || !reflect.DeepEqual(s.Args, []any{"NoQuotaUser"}) {
			t.Fatal(s)
		}
		o.Names[0] = "Changed"
		return input, nil
	})
	got, err := ReadDirectSystem(t.Context(), reader, Request{Options: o, Reference: ref})
	if err != nil || len(got.Records) != 2 || calls != 1 || got.Provenance.Names[0] != "NoQuotaUser" {
		t.Fatal(got, err, calls)
	}
	o, _ = systemFixture()
	bad := input
	bad.Rows = append(append([][]session.Value(nil), input.Rows...), []session.Value{{Text: "noquotauser"}, {Text: "UNLIMITED TABLESPACE"}, {Text: "NO"}, {Text: "NO"}, {Text: "NO"}})
	reader = func(context.Context, session.Statement) (session.Result, error) { return bad, nil }
	got, err = ReadDirectSystem(t.Context(), reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, ErrInvalidEvidence) || got.Records != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private cause")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadDirectSystem(t.Context(), reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, cause) || got.Records != nil || strings.Contains(err.Error(), cause.Error()) {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadDirectSystem(ctx, reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Records != nil {
		t.Fatal(got, err)
	}
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, nil }
	for _, lim := range []rowset.Limits{{MaxRows: 1}, {MaxBytes: 1}} {
		got, err = ReadDirectSystem(t.Context(), reader, Request{Options: o, Reference: ref, Limits: lim})
		if !errors.Is(err, rowset.ErrCapacity) || got.Records != nil {
			t.Fatal(got, err)
		}
	}
	input.Rows = nil
	got, err = ReadDirectSystem(t.Context(), reader, Request{Options: o, Reference: ref})
	if err != nil || got.Records == nil || len(got.Records) != 0 {
		t.Fatal(got, err)
	}
}

type systemCursorSpy struct {
	db     *sql.DB
	cursor *sql.Rows
}

func (s *systemCursorSpy) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(ctx, q, args...)
	s.cursor = r
	return r, e
}
func TestDirectSystemNativeAndAtomicJSON(t *testing.T) {
	o, input := systemFixture()
	values := [][]driver.Value{{"NoQuotaUser", "UNLIMITED TABLESPACE", "NO", "YES", "NO"}, {"NoQuotaUser", "CREATE SESSION", nil, nil, nil}}
	db, state := testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
	spy := &systemCursorSpy{db: db}
	cursor, err := DirectSystemRows(t.Context(), spy, o)
	if err != nil || cursor != spy.cursor || !cursor.Next() || state.Closed() != 0 {
		t.Fatal(cursor, err)
	}
	cursor.Close()
	if state.Closed() != 1 {
		t.Fatal(state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
	payload, err := DirectSystemJSON(t.Context(), db, o, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var doc struct {
		Provenance Provenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &doc); err != nil || doc.Provenance.Source != "DBA_SYS_PRIVS" || !reflect.DeepEqual(doc.Data.Rows, [][]any{{"NoQuotaUser", "UNLIMITED TABLESPACE", "NO", "YES", "NO"}, {"NoQuotaUser", "CREATE SESSION", nil, nil, nil}}) {
		t.Fatal(string(payload), err)
	}
	want := []rowset.Column{{Name: "GRANTEE", Kind: rowset.Text}, {Name: "PRIVILEGE", Kind: rowset.Text}, {Name: "ADMIN_OPTION", Kind: rowset.Text}, {Name: "COMMON", Kind: rowset.Text}, {Name: "INHERITED", Kind: rowset.Text}}
	if !reflect.DeepEqual(doc.Data.Columns, want) {
		t.Fatal(doc.Data.Columns)
	}
	for _, n := range []int{len(payload), len(payload) - 1} {
		db, _ = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
		b, e := DirectSystemJSON(t.Context(), db, o, rowset.Limits{MaxBytes: n})
		if n == len(payload) {
			if e != nil || !reflect.DeepEqual(b, payload) {
				t.Fatal(string(b), e)
			}
		} else if !errors.Is(e, rowset.ErrCapacity) || b != nil {
			t.Fatal(string(b), e)
		}
	}
	terminal := errors.New("terminal")
	closeErr := errors.New("close")
	for _, tc := range []struct {
		name string
		step testsql.Step
		lim  rowset.Limits
		want error
	}{
		{"late-scope", testsql.Step{Columns: input.Columns, Values: append(values, []driver.Value{"noquotauser", "UNLIMITED TABLESPACE", "NO", "NO", "NO"})}, rowset.Limits{}, ErrInvalidEvidence},
		{"terminal", testsql.Step{Columns: input.Columns, Values: values, NextErr: terminal}, rowset.Limits{}, terminal},
		{"close", testsql.Step{Columns: input.Columns, Values: values, CloseErr: closeErr}, rowset.Limits{}, closeErr},
		{"row-budget", testsql.Step{Columns: input.Columns, Values: values}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			b, e := DirectSystemJSON(t.Context(), db, o, tc.lim)
			if !errors.Is(e, tc.want) || b != nil || state.Closed() != 1 {
				t.Fatal(string(b), e)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	db, state = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values, OnNext: func(int) { cancel() }})
	b, e := DirectSystemJSON(ctx, db, o, rowset.Limits{})
	if !errors.Is(e, context.Canceled) || b != nil || state.Closed() != 1 {
		t.Fatal(string(b), e)
	}
}
