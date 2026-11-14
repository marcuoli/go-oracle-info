package parameters_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func parameterResult(modern, stored bool) session.Result {
	names := []string{"INST_ID"}
	if modern {
		names = append(names, "CON_ID")
	}
	if stored {
		names = append(names, "SID", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISSPECIFIED", "UPDATE_COMMENT")
	} else {
		names = append(names, "NUM", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISDEFAULT", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE")
		if modern {
			names = append(names, "ISPDB_MODIFIABLE")
		}
		names = append(names, "ISINSTANCE_MODIFIABLE", "ISMODIFIED", "ISADJUSTED", "ISDEPRECATED", "ISBASIC", "DESCRIPTION", "UPDATE_COMMENT")
	}
	row := make([]session.Value, len(names))
	for i, name := range names {
		row[i] = session.Value{Text: "FALSE"}
		switch name {
		case "INST_ID":
			row[i].Text = "7"
		case "CON_ID":
			row[i].Text = "3"
		case "NUM":
			row[i].Text = "1"
		case "TYPE":
			row[i].Text = "6"
		case "ORDINAL":
			row[i].Text = "2"
		case "NAME":
			row[i].Text = "_candidate"
		case "SID":
			row[i].Text = "*"
		case "VALUE":
			row[i].Text = " 9007199254740993,raw "
		case "DISPLAY_VALUE":
			row[i].Text = " display text "
		case "ISDEFAULT":
			row[i].Text = "TRUE"
		case "UPDATE_COMMENT":
			row[i] = session.Value{Null: true}
		}
	}
	return session.Result{Columns: names, Rows: [][]session.Value{row}}
}
func setValue(result *session.Result, row int, name string, value session.Value) {
	for i, column := range result.Columns {
		if column == name {
			result.Rows[row][i] = value
			return
		}
	}
	panic("unknown test column")
}
func testReference(t *testing.T) reference.Reference {
	t.Helper()
	ref, err := reference.NewReference("unit", "parameter", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

type parameterReader func(context.Context, session.Statement) (session.Result, error)

func (f parameterReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}

func TestDecodeHiddenPreservesCompleteFacts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		major int
	}{{"legacy", 11}, {"modern", 19}} {
		t.Run(tc.name, func(t *testing.T) {
			result := parameterResult(tc.major >= 12, false)
			result.Rows = append(result.Rows, append([]session.Value(nil), result.Rows[0]...))
			setValue(&result, 1, "ORDINAL", session.Value{Text: "3"})
			setValue(&result, 1, "ISDEFAULT", session.Value{Text: "UNKNOWN"})
			if tc.major >= 12 {
				setValue(&result, 0, "CON_ID", session.Value{Null: true})
			}
			facts, err := parameters.DecodeHidden(result, tc.major, parameters.Filter{Names: []string{"_candidate"}, Instances: []int64{7}})
			if err != nil || len(facts.Rows) != 2 {
				t.Fatal(facts, err)
			}
			a, b := facts.Rows[0], facts.Rows[1]
			if facts.Provenance.Source != "GV$SYSTEM_PARAMETER2" || facts.Provenance.NameClass != "leading_underscore_only" || a.IsDefault.Text != "TRUE" || b.IsDefault.Text != "UNKNOWN" || a.Value.Text != " 9007199254740993,raw " || !a.UpdateComment.Null || a.Ordinal.Text != "2" || b.Ordinal.Text != "3" {
				t.Fatal("raw/source/list/default facts changed", facts)
			}
			if tc.major == 11 {
				if a.ContainerID != nil || a.IsPDBModifiable != nil {
					t.Fatal("legacy absence fabricated")
				}
			} else if a.ContainerID == nil || !a.ContainerID.Null || a.IsPDBModifiable == nil || b.ContainerID == nil || b.ContainerID.Text != "3" {
				t.Fatal("NULL and unselected column conflated")
			}
			result.Rows[0][0].Text = "changed"
			if a.InstanceID.Text != "7" {
				t.Fatal("input row mutation changed returned fact")
			}
		})
	}
}
func TestDecodeRejectsEveryMalformedLateRow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"late-shape", func(r *session.Result) { r.Rows[1] = r.Rows[1][:1] }},
		{"late-integer", func(r *session.Result) { setValue(r, 1, "INST_ID", session.Value{Text: "7.5"}) }},
		{"late-overflow", func(r *session.Result) { setValue(r, 1, "NUM", session.Value{Text: "9223372036854775808"}) }},
		{"late-utf8", func(r *session.Result) { setValue(r, 1, "VALUE", session.Value{Text: string([]byte{255})}) }},
		{"null-with-text", func(r *session.Result) { setValue(r, 1, "VALUE", session.Value{Text: "hidden", Null: true}) }},
		{"ordinary-name", func(r *session.Result) { setValue(r, 1, "NAME", session.Value{Text: "processes"}) }},
		{"wrong-name", func(r *session.Result) { setValue(r, 1, "NAME", session.Value{Text: "_other"}) }},
		{"wrong-instance", func(r *session.Result) { setValue(r, 1, "INST_ID", session.Value{Text: "8"}) }},
		{"duplicate-column", func(r *session.Result) { r.Columns[0] = r.Columns[1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := parameterResult(true, false)
			result.Rows = append(result.Rows, append([]session.Value(nil), result.Rows[0]...))
			tc.change(&result)
			got, err := parameters.DecodeHidden(result, 19, parameters.Filter{Names: []string{"_candidate"}, Instances: []int64{7}})
			if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
				t.Fatal("malformed prefix escaped", got, err)
			}
		})
	}
}
func TestBufferedMetadataOrderAndEmptyArePreserved(t *testing.T) {
	t.Parallel()
	result := parameterResult(true, false)
	for i, j := 0, len(result.Columns)-1; i < j; i, j = i+1, j-1 {
		result.Columns[i], result.Columns[j] = strings.ToLower(result.Columns[j]), strings.ToLower(result.Columns[i])
		result.Rows[0][i], result.Rows[0][j] = result.Rows[0][j], result.Rows[0][i]
	}
	got, err := parameters.DecodeEffective(result, 19, parameters.Filter{})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Name.Text != "_candidate" {
		t.Fatal(got, err)
	}
	result.Rows = nil
	got, err = parameters.DecodeHidden(result, 19, parameters.Filter{Names: []string{"_missing"}})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatal("missing name fabricated or empty mistaken for failed prefix", got, err)
	}
}
func TestSPFileDecodedTypeWildcardAndDormantClauses(t *testing.T) {
	t.Parallel()
	result := parameterResult(true, true)
	result.Rows = append(result.Rows, append([]session.Value(nil), result.Rows[0]...))
	setValue(&result, 1, "SID", session.Value{Text: "Dormant"})
	setValue(&result, 0, "TYPE", session.Value{Text: "future-text-type"})
	facts, err := parameters.DecodeSPFile(result, 19, parameters.SPFileFilter{SIDs: []string{"Dormant"}})
	if err != nil || len(facts.Rows) != 2 || facts.Rows[0].SID.Text != "*" || facts.Rows[1].SID.Text != "Dormant" || facts.Rows[0].Type.Text != "future-text-type" || facts.Rows[0].IsSpecified.Text != "FALSE" {
		t.Fatal(facts, err)
	}
	setValue(&result, 1, "SID", session.Value{Text: "NotSelected"})
	if facts, err = parameters.DecodeSPFile(result, 19, parameters.SPFileFilter{SIDs: []string{"Dormant"}}); !errors.Is(err, parameters.ErrInvalidEvidence) || facts.Rows != nil {
		t.Fatal("unselected clause escaped", facts, err)
	}
}
func TestReadHiddenCarriesReferenceAndNeverPublishesFailedPrefix(t *testing.T) {
	t.Parallel()
	ref := testReference(t)
	request := parameters.Request{OracleMajor: 19, Filter: parameters.Filter{Names: []string{"_candidate"}, Instances: []int64{7}}, Reference: ref}
	wantSQL, wantArgs, err := parameters.HiddenStatement(19, request.Filter)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	reader := parameterReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || s.Reference != ref || s.Text != wantSQL || len(s.Args) != len(wantArgs) || s.Args[0] != int64(7) || s.Args[1] != "_candidate" {
			t.Fatal("request transport changed", s)
		}
		return parameterResult(true, false), nil
	})
	got, err := parameters.ReadHidden(t.Context(), reader, request)
	if err != nil || len(got.Rows) != 1 || calls != 1 {
		t.Fatal(got, err, calls)
	}
	failure := errors.New("private driver/bind details")
	reader = func(context.Context, session.Statement) (session.Result, error) {
		return parameterResult(true, false), failure
	}
	got, err = parameters.ReadHidden(t.Context(), reader, request)
	if !errors.Is(err, failure) || got.Rows != nil || strings.Contains(err.Error(), failure.Error()) {
		t.Fatal("failed prefix/cause leaked", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return parameterResult(true, false), nil
	}
	got, err = parameters.ReadHidden(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal("postquery cancellation ignored", got, err)
	}
}
func TestReadRequestValidationBeforeQuery(t *testing.T) {
	t.Parallel()
	calls := 0
	reader := parameterReader(func(context.Context, session.Statement) (session.Result, error) {
		calls++
		return session.Result{}, nil
	})
	ref := testReference(t)
	for _, request := range []parameters.Request{{OracleMajor: 19}, {OracleMajor: 10, Reference: ref}, {OracleMajor: 19, Filter: parameters.Filter{Instances: []int64{0}}, Reference: ref}} {
		if _, err := parameters.ReadEffective(t.Context(), reader, request); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := parameters.ReadHidden(ctx, reader, parameters.Request{OracleMajor: 19, Reference: ref}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("invalid/cancelled request queried", calls)
	}
	filter := parameters.SPFileFilter{SIDs: []string{"Dormant"}}
	reader = func(context.Context, session.Statement) (session.Result, error) {
		filter.SIDs[0] = "changed"
		return parameterResult(true, true), nil
	}
	got, err := parameters.ReadSPFile(t.Context(), reader, parameters.SPFileRequest{OracleMajor: 19, Filter: filter, Reference: ref})
	if err != nil || len(got.Rows) != 1 || got.Provenance.SIDs[0] != "Dormant" {
		t.Fatal("request selection changed during read", got, err)
	}
}
