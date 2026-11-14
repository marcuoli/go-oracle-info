package instances_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func fixture(modern bool) session.Result {
	names := []string{"INST_ID"}
	values := []session.Value{{Text: "7"}}
	if modern {
		names = append(names, "CON_ID")
		values = append(values, session.Value{Text: "0"})
	}
	names = append(names, "INSTANCE_NUMBER", "INSTANCE_NAME", "HOST_NAME", "STATUS", "DATABASE_STATUS", "PARALLEL", "BLOCKED")
	values = append(values, session.Value{Text: "7"}, session.Value{Text: " Node7 "}, session.Value{Null: true}, session.Value{Text: "MOUNTED"}, session.Value{Text: "ACTIVE"}, session.Value{Text: "YES"}, session.Value{Text: "YES"})
	return session.Result{Columns: names, Rows: [][]session.Value{values}}
}
func change(r *session.Result, name string, value session.Value) {
	for i, n := range r.Columns {
		if n == name {
			r.Rows[len(r.Rows)-1][i] = value
			return
		}
	}
	panic(name)
}
func TestVisibilityScopeIncludesMountedAndNoLocalFilter(t *testing.T) {
	for _, major := range []int{11, 12, 19, 23, 26} {
		sql, err := instances.Statement(major)
		if err != nil || !strings.Contains(sql, "FROM GV$INSTANCE i") || strings.Contains(sql, "i.INST_ID =") || strings.Contains(sql, "OPEN_MODE") || strings.Contains(sql, "STATUS =") {
			t.Fatal(sql, err)
		}
		if strings.Contains(sql, "CON_ID") != (major >= 12) {
			t.Fatal(major, sql)
		}
		got, err := instances.Decode(fixture(major >= 12), major)
		if err != nil || got.State != instances.Available || got.Provenance.Source != "GV$INSTANCE" || len(got.Rows) != 1 || got.Rows[0].InstanceID.Text != "7" || got.Rows[0].Status.Text != "MOUNTED" || got.Rows[0].Name.Text != " Node7 " || !got.Rows[0].HostName.Null || got.Rows[0].Blocked.Text != "YES" {
			t.Fatal(got, err)
		}
		if (got.Rows[0].ContainerID != nil) != (major >= 12) {
			t.Fatal("absence fabricated")
		}
	}
	if _, err := instances.Statement(10); !errors.Is(err, instances.ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}
func TestDecodeVisibilityMetadataOrderDuplicatesAndEmpty(t *testing.T) {
	r := fixture(true)
	r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
	for i, j := 0, len(r.Columns)-1; i <= j; i, j = i+1, j-1 {
		r.Columns[i], r.Columns[j] = strings.ToLower(r.Columns[j]), strings.ToLower(r.Columns[i])
		for k := range r.Rows {
			r.Rows[k][i], r.Rows[k][j] = r.Rows[k][j], r.Rows[k][i]
		}
	}
	got, err := instances.Decode(r, 19)
	if err != nil || len(got.Rows) != 2 || got.Rows[1].InstanceID.Text != "7" {
		t.Fatal(got, err)
	}
	r.Rows = nil
	got, err = instances.Decode(r, 19)
	if err != nil || got.State != instances.NoRows || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatal(got, err)
	}
}
func TestDecodeVisibilityRejectsEveryMalformedPrefix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*session.Result)
	}{
		{"short", func(r *session.Result) { r.Rows[1] = r.Rows[1][:1] }},
		{"duplicate-column", func(r *session.Result) { r.Columns[0] = r.Columns[1] }},
		{"missing-column", func(r *session.Result) { r.Columns[0] = "WRONG" }},
		{"null-id", func(r *session.Result) { change(r, "INST_ID", session.Value{Null: true}) }},
		{"zero-id", func(r *session.Result) { change(r, "INST_ID", session.Value{Text: "0"}) }},
		{"negative-id", func(r *session.Result) { change(r, "INST_ID", session.Value{Text: "-7"}) }},
		{"overflow-id", func(r *session.Result) { change(r, "INST_ID", session.Value{Text: "9223372036854775808"}) }},
		{"fraction", func(r *session.Result) { change(r, "INSTANCE_NUMBER", session.Value{Text: "7.1"}) }},
		{"invalid-utf8", func(r *session.Result) { change(r, "HOST_NAME", session.Value{Text: string([]byte{255})}) }},
		{"null-text", func(r *session.Result) { change(r, "BLOCKED", session.Value{Text: "YES", Null: true}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixture(true)
			r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
			tc.modify(&r)
			got, err := instances.Decode(r, 19)
			if !errors.Is(err, instances.ErrInvalidEvidence) || got.State != instances.Unavailable || got.Rows != nil {
				t.Fatal(got, err)
			}
		})
	}
}

type readerFunc func(context.Context, session.Statement) (session.Result, error)

func (f readerFunc) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func TestReadVisibilityDoesNotFallbackOrReturnFailedPrefix(t *testing.T) {
	ref, err := reference.NewReference("unit", "instances", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := instances.Request{OracleMajor: 19, Reference: ref}
	calls := 0
	reader := readerFunc(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || s.Reference != ref || len(s.Args) != 0 || !strings.Contains(s.Text, "GV$INSTANCE") {
			t.Fatal(s)
		}
		return fixture(true), nil
	})
	got, err := instances.Read(t.Context(), reader, request)
	if err != nil || got.State != instances.Available || calls != 1 {
		t.Fatal(got, err, calls)
	}
	failure := errors.New("private driver diagnostic")
	reader = func(context.Context, session.Statement) (session.Result, error) {
		calls++
		return fixture(true), failure
	}
	got, err = instances.Read(t.Context(), reader, request)
	if !errors.Is(err, failure) || got.State != instances.Unavailable || got.Rows != nil || calls != 2 || strings.Contains(err.Error(), failure.Error()) {
		t.Fatal(got, err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return fixture(true), nil }
	got, err = instances.Read(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	_, err = instances.Read(t.Context(), nil, request)
	if !errors.Is(err, instances.ErrInvalidRequest) {
		t.Fatal(err)
	}
	_, err = instances.Read(t.Context(), reader, instances.Request{OracleMajor: 19})
	if !errors.Is(err, instances.ErrInvalidRequest) {
		t.Fatal(err)
	}
}
