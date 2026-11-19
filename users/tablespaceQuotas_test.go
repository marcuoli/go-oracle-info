package users

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

func quotaFixture() (Options, session.Result) {
	return Options{OracleMajor: 19, Names: []string{"Case'User"}}, session.Result{Columns: []string{"USERNAME", "TABLESPACE_NAME", "BYTES", "MAX_BYTES", "DROPPED"}, Rows: [][]session.Value{
		{{Text: "Case'User"}, {Text: "DATA"}, {Text: "18446744073709551616"}, {Text: "-1"}, {Text: "NO"}},
		{{Text: "Case'User"}, {Text: "DATA_2"}, {Text: "4096"}, {Text: "0"}, {Text: "YES"}},
		{{Text: "Case'User"}, {Text: "Nulls"}, {Null: true}, {Null: true}, {Null: true}},
	}}
}
func TestTablespaceQuotaExactSourceContract(t *testing.T) {
	o, input := quotaFixture()
	sql, args, err := TablespaceQuotasStatement(o)
	if err != nil || !reflect.DeepEqual(args, []any{"Case'User"}) || strings.Contains(sql, "Case'User") || !strings.Contains(sql, "FROM DBA_TS_QUOTAS t") || !strings.Contains(sql, "NLSSORT(t.USERNAME, 'NLS_SORT=BINARY')") || !strings.HasSuffix(sql, " ORDER BY t.USERNAME,t.TABLESPACE_NAME") {
		t.Fatal(sql, args, err)
	}
	got, err := DecodeTablespaceQuotas(input, o)
	if err != nil || len(got.Records) != 3 || got.Provenance.Source != "DBA_TS_QUOTAS" {
		t.Fatal(got, err)
	}
	if got.Records[0].Username.Text != "Case'User" || got.Records[0].TablespaceName.Text != "DATA" || got.Records[0].Bytes.Text != "18446744073709551616" || got.Records[0].MaxBytes.Text != "-1" || got.Records[0].Dropped == nil || got.Records[0].Dropped.Text != "NO" {
		t.Fatal(got)
	}
	if got.Records[1].Bytes.Text != "4096" || got.Records[1].MaxBytes.Text != "0" || got.Records[1].Dropped.Text != "YES" || !got.Records[2].Bytes.Null || !got.Records[2].MaxBytes.Null || got.Records[2].Dropped == nil || !got.Records[2].Dropped.Null {
		t.Fatal(got)
	}
	input.Rows = nil
	empty, err := DecodeTablespaceQuotas(input, o)
	if err != nil || empty.Records == nil || len(empty.Records) != 0 {
		t.Fatal(empty, err)
	}
}
func TestTablespaceQuotasVersionSelection(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26, 99} {
		o := Options{OracleMajor: major}
		sql, args, err := TablespaceQuotasStatement(o)
		if major == 9 || major == 10 || major == 26 || major == 99 {
			if !errors.Is(err, ErrUnsupportedVersion) || sql != "" {
				t.Fatal(major, sql, err)
			}
			continue
		}
		if err != nil || len(args) != 0 || !strings.Contains(sql, "t.DROPPED AS DROPPED") {
			t.Fatal(major, sql, args, err)
		}
		b, _ := json.Marshal(struct {
			Package, Fact string
			Major         int
			SQL           string
			Args          []any
		}{"users", "TablespaceQuotas", major, sql, args})
		t.Log(string(b))
	}
	if _, _, err := TablespaceQuotasStatement(Options{OracleMajor: 19, Owners: []string{"OWNER"}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}
func TestTablespaceQuotasCompleteRead(t *testing.T) {
	o, input := quotaFixture()
	ref, _ := reference.NewReference("unit", "users", "quota", 1)
	calls := 0
	reader := fixtureReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref || !reflect.DeepEqual(s.Args, []any{"Case'User"}) {
			t.Fatal(s)
		}
		o.Names[0] = "Changed"
		return input, nil
	})
	got, err := ReadTablespaceQuotas(t.Context(), reader, Request{Options: o, Reference: ref})
	if err != nil || calls != 1 || len(got.Records) != 3 || got.Provenance.Names[0] != "Case'User" {
		t.Fatal(got, err, calls)
	}
	o, _ = quotaFixture()
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, nil }
	for _, bad := range []session.Result{
		{Columns: input.Columns, Rows: append(append([][]session.Value(nil), input.Rows...), []session.Value{{Text: "Case'User"}, {Text: "X"}, {Text: "1.5"}, {Text: "0"}, {Text: "NO"}})},
		{Columns: input.Columns, Rows: append(append([][]session.Value(nil), input.Rows...), []session.Value{{Text: "Case'User"}, {Text: "X"}, {Null: true, Text: "1"}, {Text: "0"}, {Text: "NO"}})},
	} {
		r := fixtureReader(func(context.Context, session.Statement) (session.Result, error) { return bad, nil })
		v, e := ReadTablespaceQuotas(t.Context(), r, Request{Options: o, Reference: ref})
		if !errors.Is(e, ErrInvalidEvidence) || v.Records != nil {
			t.Fatal(v, e)
		}
	}
	cause := errors.New("private failure")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTablespaceQuotas(t.Context(), reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, cause) || got.Records != nil || strings.Contains(err.Error(), cause.Error()) {
		t.Fatal(got, err)
	}
	reader = func(context.Context, session.Statement) (session.Result, error) {
		return session.Result{}, sql.ErrNoRows
	}
	got, err = ReadTablespaceQuotas(t.Context(), reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, sql.ErrNoRows) || got.Records != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTablespaceQuotas(ctx, reader, Request{Options: o, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Records != nil {
		t.Fatal(got, err)
	}
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, nil }
	for _, lim := range []rowset.Limits{{MaxRows: 1}, {MaxBytes: 1}} {
		got, err = ReadTablespaceQuotas(t.Context(), reader, Request{Options: o, Reference: ref, Limits: lim})
		if !errors.Is(err, rowset.ErrCapacity) || got.Records != nil {
			t.Fatal(got, err)
		}
	}
}

type quotaCursorSpy struct {
	db     *sql.DB
	cursor *sql.Rows
}

func (s *quotaCursorSpy) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(ctx, q, args...)
	s.cursor = r
	return r, e
}
func TestTablespaceQuotasNativeJSON(t *testing.T) {
	o, input := quotaFixture()
	values := [][]driver.Value{{"Case'User", "DATA", "18446744073709551616", "-1", "NO"}, {"Case'User", "DATA_2", "4096", "0", "YES"}, {"Case'User", "Nulls", nil, nil, nil}}
	db, state := testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
	spy := &quotaCursorSpy{db: db}
	cursor, err := TablespaceQuotasRows(t.Context(), spy, o)
	if err != nil || cursor != spy.cursor || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	cursor.Close()
	if state.Closed() != 1 {
		t.Fatal(state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
	payload, err := TablespaceQuotasJSON(t.Context(), db, o, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &doc); err != nil || doc.Provenance.Source != "DBA_TS_QUOTAS" || !reflect.DeepEqual(doc.Data.Rows, [][]any{{"Case'User", "DATA", "18446744073709551616", "-1", "NO"}, {"Case'User", "DATA_2", "4096", "0", "YES"}, {"Case'User", "Nulls", nil, nil, nil}}) {
		t.Fatal(string(payload), err)
	}
	want := []rowset.Column{{Name: "USERNAME", Kind: rowset.Text}, {Name: "TABLESPACE_NAME", Kind: rowset.Text}, {Name: "BYTES", Kind: rowset.Text}, {Name: "MAX_BYTES", Kind: rowset.Text}, {Name: "DROPPED", Kind: rowset.Text}}
	if !reflect.DeepEqual(doc.Data.Columns, want) {
		t.Fatal(doc.Data.Columns)
	}
	size := len(payload)
	for _, n := range []int{size, size - 1} {
		db, _ = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values})
		b, e := TablespaceQuotasJSON(t.Context(), db, o, rowset.Limits{MaxBytes: n})
		if n == size {
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
		{"late-domain", testsql.Step{Columns: input.Columns, Values: append(values, []driver.Value{"Case'User", "BAD", "1.5", "0", "NO"})}, rowset.Limits{}, ErrInvalidEvidence},
		{"terminal", testsql.Step{Columns: input.Columns, Values: values, NextErr: terminal}, rowset.Limits{}, terminal},
		{"close", testsql.Step{Columns: input.Columns, Values: values, CloseErr: closeErr}, rowset.Limits{}, closeErr},
		{"row-budget", testsql.Step{Columns: input.Columns, Values: values}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			b, e := TablespaceQuotasJSON(t.Context(), db, o, tc.lim)
			if !errors.Is(e, tc.want) || b != nil || state.Closed() != 1 {
				t.Fatal(string(b), e, state.Closed())
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	db, state = testsql.New(t, testsql.Step{Columns: input.Columns, Values: values, OnNext: func(int) { cancel() }})
	b, e := TablespaceQuotasJSON(ctx, db, o, rowset.Limits{})
	if !errors.Is(e, context.Canceled) || b != nil || state.Closed() != 1 {
		t.Fatal(string(b), e)
	}
}
