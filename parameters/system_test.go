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

func systemResult(modern bool) session.Result {
	source := parameterResult(modern, false)
	result := session.Result{Rows: [][]session.Value{{}}}
	for i, name := range source.Columns {
		if name != "ORDINAL" {
			result.Columns = append(result.Columns, name)
			result.Rows[0] = append(result.Rows[0], source.Rows[0][i])
		}
	}
	return result
}

func TestSystemStatementIsGlobalScalarAndBound(t *testing.T) {
	for _, major := range []int{11, 12, 19, 23, 26} {
		sql, args, err := parameters.SystemStatement(major, parameters.Filter{})
		if err != nil || len(args) != 0 || !strings.Contains(sql, "FROM GV$SYSTEM_PARAMETER p") || strings.Contains(sql, "ORDINAL") || strings.Contains(sql, "SYSTEM_PARAMETER2") || strings.Contains(sql, "p.INST_ID IN") || strings.Contains(sql, "V$PARAMETER ") {
			t.Fatal(major, sql, args, err)
		}
		if strings.Contains(sql, "CON_ID") != (major >= 12) || strings.Contains(sql, "ISPDB_MODIFIABLE") != (major >= 12) {
			t.Fatal("wrong version projection", major, sql)
		}
		list, _, err := parameters.EffectiveStatement(major, parameters.Filter{})
		if err != nil || !strings.Contains(list, "FROM GV$SYSTEM_PARAMETER2 p") || !strings.HasSuffix(list, ", p.ORDINAL") {
			t.Fatal("published list contract changed", list, err)
		}
	}
	sql, args, err := parameters.SystemStatement(19, parameters.Filter{Instances: []int64{7, 2, 7}, Names: []string{"sga_target", "a' OR 1=1"}})
	if err != nil || strings.Contains(sql, "a' OR") || !strings.Contains(sql, "p.INST_ID IN (:1,:2,:3)") || !reflect.DeepEqual(args, []any{int64(7), int64(2), int64(7), "sga_target", "a' OR 1=1"}) {
		t.Fatal("bind/filter contract", sql, args, err)
	}
	if _, _, err = parameters.SystemStatement(10, parameters.Filter{}); !errors.Is(err, parameters.ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}

func TestSystemDecodePreservesScalarFlagsAndVersionAbsence(t *testing.T) {
	for _, major := range []int{11, 19} {
		source := systemResult(major >= 12)
		source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
		setValue(&source, 0, "VALUE", session.Value{Text: " 'one,two', 9007199254740993, 256K "})
		setValue(&source, 0, "ISMODIFIED", session.Value{Text: "MODIFIED"})
		setValue(&source, 1, "ISMODIFIED", session.Value{Null: true})
		if major >= 12 {
			setValue(&source, 0, "CON_ID", session.Value{Null: true})
		}
		got, err := parameters.DecodeSystem(source, major, parameters.Filter{Instances: []int64{7}})
		if err != nil || len(got.Rows) != 2 || got.Provenance.Source != "GV$SYSTEM_PARAMETER" || got.Rows[0].Value.Text != " 'one,two', 9007199254740993, 256K " || got.Rows[0].IsModified.Text != "MODIFIED" || !got.Rows[1].IsModified.Null {
			t.Fatal(got, err)
		}
		if major == 11 {
			if got.Rows[0].ContainerID != nil || got.Rows[0].IsPDBModifiable != nil {
				t.Fatal("legacy absence fabricated")
			}
		} else if got.Rows[0].ContainerID == nil || !got.Rows[0].ContainerID.Null || got.Rows[1].ContainerID.Text != "3" {
			t.Fatal("scope/NULL lost")
		}
		source.Rows = nil
		empty, err := parameters.DecodeSystem(source, major, parameters.Filter{})
		if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
			t.Fatal("empty is not failed", empty, err)
		}
	}
}

func TestSystemRejectsLateMalformedOrUnselectedRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"width", func(r *session.Result) { r.Rows[1] = r.Rows[1][:2] }},
		{"integer", func(r *session.Result) { setValue(r, 1, "TYPE", session.Value{Text: "1.5"}) }},
		{"utf8", func(r *session.Result) { setValue(r, 1, "VALUE", session.Value{Text: string([]byte{255})}) }},
		{"null-text", func(r *session.Result) { setValue(r, 1, "VALUE", session.Value{Text: "x", Null: true}) }},
		{"unselected-instance", func(r *session.Result) { setValue(r, 1, "INST_ID", session.Value{Text: "8"}) }},
		{"unselected-name", func(r *session.Result) { setValue(r, 1, "NAME", session.Value{Text: "other"}) }},
		{"duplicate-column", func(r *session.Result) { r.Columns[0] = r.Columns[1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := systemResult(true)
			r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
			tc.change(&r)
			got, err := parameters.DecodeSystem(r, 19, parameters.Filter{Instances: []int64{7}, Names: []string{"_candidate"}})
			if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
				t.Fatal("prefix escaped", got, err)
			}
		})
	}
}

func TestReadSystemReferenceFailuresAndCancellation(t *testing.T) {
	ref := testReference(t)
	request := parameters.Request{OracleMajor: 19, Reference: ref, Filter: parameters.Filter{Instances: []int64{7}}}
	calls := 0
	reader := parameterReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || s.Reference != ref || !strings.Contains(s.Text, "FROM GV$SYSTEM_PARAMETER p") || !reflect.DeepEqual(s.Args, []any{int64(7)}) {
			t.Fatal(s)
		}
		return systemResult(true), nil
	})
	got, err := parameters.ReadSystem(t.Context(), reader, request)
	if err != nil || len(got.Rows) != 1 || calls != 1 {
		t.Fatal(got, err, calls)
	}
	failure := errors.New("private source failure")
	reader = func(context.Context, session.Statement) (session.Result, error) { return systemResult(true), failure }
	got, err = parameters.ReadSystem(t.Context(), reader, request)
	if !errors.Is(err, failure) || got.Rows != nil || strings.Contains(err.Error(), failure.Error()) {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return systemResult(true), nil
	}
	got, err = parameters.ReadSystem(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
}

func TestModifiedSystemUsesOnlyLiteralFlagAndCopies(t *testing.T) {
	source, err := parameters.DecodeSystem(systemResult(true), 19, parameters.Filter{Instances: []int64{7}, Names: []string{"_candidate"}})
	if err != nil {
		t.Fatal(err)
	}
	first := source.Rows[0]
	source.Rows = nil
	for _, flag := range []session.Value{{Text: "MODIFIED"}, {Text: "SYSTEM_MOD"}, {Text: "FALSE"}, {Text: "modified"}, {Null: true}, {Text: "FUTURE"}} {
		r := first
		r.IsModified = flag
		source.Rows = append(source.Rows, r)
	}
	selected := parameters.ModifiedSystem(source)
	if len(selected.Rows) != 1 || selected.Rows[0].IsModified.Text != "MODIFIED" {
		t.Fatal(selected)
	}
	selected.Rows[0].ContainerID.Text = "9"
	selected.Rows[0].IsPDBModifiable.Text = "changed"
	selected.Provenance.Instances[0] = 9
	selected.Provenance.Names[0] = "changed"
	if source.Rows[0].ContainerID.Text != "3" || source.Rows[0].IsPDBModifiable.Text != "FALSE" || source.Provenance.Instances[0] != 7 || source.Provenance.Names[0] != "_candidate" {
		t.Fatal("selection mutated evidence")
	}
	if parameters.ModifiedSystem(parameters.SystemResult{}).Rows != nil {
		t.Fatal("failure became empty")
	}
	if parameters.ModifiedSystem(parameters.SystemResult{Rows: []parameters.SystemParameter{}}).Rows == nil {
		t.Fatal("complete empty became unavailable")
	}
}

func TestSystemKeepsIndependentInstancesAndContainers(t *testing.T) {
	source := systemResult(true)
	for range 2 {
		source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
	}
	setValue(&source, 0, "CON_ID", session.Value{Text: "0"})
	setValue(&source, 1, "INST_ID", session.Value{Text: "2"})
	setValue(&source, 1, "VALUE", session.Value{Text: "peer2"})
	setValue(&source, 2, "CON_ID", session.Value{Text: "4"})
	setValue(&source, 2, "VALUE", session.Value{Null: true})
	got, err := parameters.DecodeSystem(source, 19, parameters.Filter{})
	if err != nil || len(got.Rows) != 3 || got.Rows[0].InstanceID.Text != "7" || got.Rows[0].ContainerID.Text != "0" || got.Rows[1].InstanceID.Text != "2" || got.Rows[1].Value.Text != "peer2" || got.Rows[2].ContainerID.Text != "4" || !got.Rows[2].Value.Null {
		t.Fatal("instance/container identity collapsed", got, err)
	}
}
