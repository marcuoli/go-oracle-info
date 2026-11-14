package parameters_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/session"
)

func hiddenCatalogResult() session.Result {
	return session.Result{Columns: []string{"NAME", "SESSION_VALUE", "INSTANCE_VALUE", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE", "DESCRIPTION"}, Rows: [][]session.Value{
		{{Text: "_waiters_test"}, {Text: "9007199254740993"}, {Text: " 1,2 "}, {Text: "TRUE"}, {Text: "DEFERRED"}, {Null: true}},
		{{Text: "_waiters_null"}, {Null: true}, {Text: "NULL"}, {Text: "FALSE"}, {Text: "IMMEDIATE"}, {Text: "description"}},
	}}
}

func TestHiddenCatalogSelection(t *testing.T) {
	t.Parallel()
	filter := parameters.HiddenCatalogFilter{Names: []string{"_candidate", "_x') OR 1=1 --"}, NameContains: "WAITERS%'_"}
	sql, args, err := parameters.HiddenCatalogStatement(19, filter)
	if err != nil || !reflect.DeepEqual(args, []any{filter.Names[0], filter.Names[1], "waiters%'_"}) {
		t.Fatal(sql, args, err)
	}
	for _, clause := range []string{"FROM SYS.X$KSPPI a", "JOIN SYS.X$KSPPCV b ON a.INDX = b.INDX", "JOIN SYS.X$KSPPSV c ON a.INDX = c.INDX", "b.KSPPSTVL AS SESSION_VALUE", "c.KSPPSTVL AS INSTANCE_VALUE", "BITAND(a.KSPPIFLG / 256, 1)", "BITAND(a.KSPPIFLG / 65536, 3)", "2, 'DEFERRED', 3, 'IMMEDIATE'", "INSTR(LOWER(a.KSPPINM), :3) > 0", "ORDER BY NLSSORT(a.KSPPINM, 'NLS_SORT=BINARY')"} {
		if !strings.Contains(sql, clause) {
			t.Fatalf("missing %q", clause)
		}
	}
	if strings.Contains(sql, filter.Names[1]) || strings.Contains(sql, "waiters") || strings.Contains(sql, "ROWNUM") || strings.Contains(sql, "FETCH") {
		t.Fatal("filter interpolation or truncated cursor")
	}
	all, binds, err := parameters.HiddenCatalogStatement(11, parameters.HiddenCatalogFilter{})
	if err != nil || len(binds) != 0 || strings.Contains(all, "INSTR(") || strings.Contains(all, " LIKE ") {
		t.Fatal(all, binds, err)
	}
	for _, bad := range []struct {
		name   string
		major  int
		filter parameters.HiddenCatalogFilter
	}{
		{"version", 10, parameters.HiddenCatalogFilter{}},
		{"public name", 19, parameters.HiddenCatalogFilter{Names: []string{"processes"}}},
		{"control", 19, parameters.HiddenCatalogFilter{NameContains: "a\nb"}},
		{"non ASCII substring", 19, parameters.HiddenCatalogFilter{NameContains: "é"}},
		{"too many", 19, parameters.HiddenCatalogFilter{Names: make([]string, 1001)}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, _, err := parameters.HiddenCatalogStatement(bad.major, bad.filter); err == nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
}

func TestDecodeHiddenCatalogPreservesSessionInstanceAndNull(t *testing.T) {
	t.Parallel()
	result := hiddenCatalogResult()
	got, err := parameters.DecodeHiddenCatalog(result, 19, parameters.HiddenCatalogFilter{NameContains: "WAITERS"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 || got.Rows[0].SessionValue.Text != "9007199254740993" || got.Rows[0].InstanceValue.Text != " 1,2 " || !got.Rows[0].Description.Null || !got.Rows[1].SessionValue.Null || got.Rows[1].InstanceValue.Null || got.Rows[1].InstanceValue.Text != "NULL" {
		t.Fatalf("raw values lost: %+v", got)
	}
	if got.Provenance.Scope != "connected_session_and_instance" || len(got.Provenance.Instances) != 0 || got.Provenance.Source != "SYS.X$KSPPI+SYS.X$KSPPCV+SYS.X$KSPPSV" {
		t.Fatal(got.Provenance)
	}
	for _, mode := range []string{"FALSE", "IMMEDIATE", "DEFERRED"} {
		result.Rows[0][4] = session.Value{Text: mode}
		decoded, err := parameters.DecodeHiddenCatalog(result, 19, parameters.HiddenCatalogFilter{})
		if err != nil || decoded.Rows[0].IsSystemModifiable.Text != mode {
			t.Fatal(mode, decoded, err)
		}
	}
}

func TestHiddenCatalogRejectsMalformedLateRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
		filter parameters.HiddenCatalogFilter
	}{
		{"wrong column", func(r *session.Result) { r.Columns[5] = "OTHER" }, parameters.HiddenCatalogFilter{}},
		{"short late row", func(r *session.Result) { r.Rows[1] = r.Rows[1][:5] }, parameters.HiddenCatalogFilter{}},
		{"non-hidden late name", func(r *session.Result) { r.Rows[1][0].Text = "processes" }, parameters.HiddenCatalogFilter{}},
		{"contradictory NULL", func(r *session.Result) { r.Rows[1][1] = session.Value{Text: "NULL", Null: true} }, parameters.HiddenCatalogFilter{}},
		{"exact name mismatch", func(r *session.Result) {}, parameters.HiddenCatalogFilter{Names: []string{"_waiters_test"}}},
		{"substring mismatch", func(r *session.Result) {}, parameters.HiddenCatalogFilter{NameContains: "missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := hiddenCatalogResult()
			tc.change(&result)
			got, err := parameters.DecodeHiddenCatalog(result, 19, tc.filter)
			if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
				t.Fatalf("prefix published: %+v %v", got, err)
			}
		})
	}
}

func TestReadHiddenCatalogCancellationAndDeniedSource(t *testing.T) {
	t.Parallel()
	denied := errors.New("private connection details")
	for _, tc := range []struct {
		name     string
		cancel   bool
		queryErr error
	}{
		{"denied", false, denied}, {"cancel after query", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := parameterReader(func(context.Context, session.Statement) (session.Result, error) {
				if tc.cancel {
					cancel()
				}
				return hiddenCatalogResult(), tc.queryErr
			})
			got, err := parameters.ReadHiddenCatalog(ctx, reader, parameters.HiddenCatalogRequest{OracleMajor: 19, Reference: testReference(t)})
			want := denied
			if tc.cancel {
				want = context.Canceled
			}
			if !errors.Is(err, want) || got.Rows != nil || strings.Contains(err.Error(), "private connection") {
				t.Fatal(got, err)
			}
		})
	}
}

func TestHiddenCatalogInvalidRequestsNeverQuery(t *testing.T) {
	t.Parallel()
	calls := 0
	reader := parameterReader(func(context.Context, session.Statement) (session.Result, error) {
		calls++
		return hiddenCatalogResult(), nil
	})
	ref := testReference(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := parameters.ReadHiddenCatalog(ctx, reader, parameters.HiddenCatalogRequest{OracleMajor: 19, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil || calls != 0 {
		t.Fatal(got, err, calls)
	}
	if _, err = parameters.ReadHiddenCatalog(t.Context(), reader, parameters.HiddenCatalogRequest{OracleMajor: 19}); !errors.Is(err, parameters.ErrInvalidRequest) || calls != 0 {
		t.Fatal(err, calls)
	}
	if _, err = parameters.ReadHiddenCatalog(t.Context(), nil, parameters.HiddenCatalogRequest{OracleMajor: 19, Reference: ref}); !errors.Is(err, parameters.ErrInvalidRequest) {
		t.Fatal(err)
	}
	filter := parameters.HiddenCatalogFilter{Names: []string{"_waiters_test"}}
	reader = parameterReader(func(context.Context, session.Statement) (session.Result, error) {
		filter.Names[0] = "_changed"
		result := hiddenCatalogResult()
		result.Rows = result.Rows[:1]
		return result, nil
	})
	got, err = parameters.ReadHiddenCatalog(t.Context(), reader, parameters.HiddenCatalogRequest{OracleMajor: 19, Reference: ref, Filter: filter})
	if err != nil || len(got.Rows) != 1 || got.Provenance.Names[0] != "_waiters_test" {
		t.Fatal("request filter changed during read", got, err)
	}
}
