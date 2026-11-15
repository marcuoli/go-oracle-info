package metadata

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func ddlResult(text string) session.Result {
	return session.Result{Columns: []string{"OBJECT_TYPE", "OBJECT_NAME", "OWNER", "DDL", "CLOB_CODE_UNITS"}, Rows: [][]session.Value{{{Text: "TRIGGER"}, {Text: "T_1"}, {Text: "SYS"}, {Text: text}, {Text: strconv.Itoa(len(utf16.Encode([]rune(text))))}}}}
}

var ddlOptions = Options{OracleMajor: 19, Kind: Trigger, Name: "T_1", Owner: "SYS"}

func TestMetadataBindsAllowlistAndNoTransformMutation(t *testing.T) {
	for _, kind := range []Kind{Tablespace, Directory, Trigger} {
		options := Options{OracleMajor: 11, Kind: kind, Name: "X' || attack", Owner: "SYS"}
		sql, args, err := Statement(options)
		if err != nil || len(args) != 2 || args[0] != options.Name || args[1] != "SYS" || strings.Contains(sql, options.Name) || !strings.Contains(sql, "DBMS_METADATA.GET_DDL('"+string(kind)+"'") || strings.Contains(sql, "SET_TRANSFORM") || strings.Contains(sql, "SUBSTR") || strings.Contains(sql, "EXECUTE") {
			t.Fatal(sql, args, err)
		}
	}
	for _, options := range []Options{{OracleMajor: 19, Kind: "USER", Name: "SYS"}, {OracleMajor: 19, Kind: Trigger, Name: "T"}, {OracleMajor: 19, Kind: Tablespace, Name: "T", MaxDDLBytes: -1}} {
		if _, _, err := Statement(options); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal(options, err)
		}
	}
	if _, _, err := Statement(Options{OracleMajor: 10, Kind: Directory, Name: "D"}); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}
func TestMetadataFullCLOBUnicodeBudgetAndIdentity(t *testing.T) {
	ddl := "CREATE OR REPLACE TRIGGER \"T_1\" -- \U0001f600\n" + strings.Repeat("x", 5000) + "\n/\n"
	result := ddlResult(ddl)
	got, err := Decode(result, ddlOptions)
	if err != nil || !got.Complete || len(got.Records) != 1 || got.Records[0].DDL.Text != ddl || got.Provenance.Transforms != "caller_session_inherited_unmodified" {
		t.Fatal(got, err)
	}
	for _, change := range []func(*session.Result){
		func(r *session.Result) { r.Rows[0][3].Text = r.Rows[0][3].Text[:100] },
		func(r *session.Result) { r.Rows[0][0].Text = "TABLESPACE" },
		func(r *session.Result) { r.Rows[0][1].Text = "OTHER" },
		func(r *session.Result) { r.Rows[0][2].Null = true; r.Rows[0][2].Text = "" },
		func(r *session.Result) { r.Rows[0][3] = session.Value{Null: true} },
		func(r *session.Result) { r.Rows[0][4].Text = "2.5" },
		func(r *session.Result) { r.Rows = append(r.Rows, r.Rows[0]) },
		func(r *session.Result) { r.Columns[0] = "DDL" },
	} {
		r := ddlResult(ddl)
		change(&r)
		got, err = Decode(r, ddlOptions)
		if !errors.Is(err, ErrInvalidEvidence) || got.Complete || got.Records != nil {
			t.Fatal(got, err)
		}
	}
	options := ddlOptions
	options.MaxDDLBytes = len(ddl) - 1
	got, err = Decode(result, options)
	if !errors.Is(err, rowset.ErrCapacity) || got.Complete || got.Records != nil {
		t.Fatal(got, err)
	}
	result.Rows = nil
	got, err = Decode(result, ddlOptions)
	if !errors.Is(err, ErrNoRows) || got.Complete {
		t.Fatal(got, err)
	}
	// Supplementary characters use two UCS2 units; counting runes is insufficient.
	result = ddlResult("\U0001f600")
	result.Rows[0][4].Text = "1"
	if _, err = Decode(result, ddlOptions); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal(err)
	}
}

type ddlReader func(context.Context, session.Statement) (session.Result, error)

func (f ddlReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func TestMetadataReadNoPrefixAfterFailureCancellationOrBudget(t *testing.T) {
	ref, err := reference.NewReference("unit", "ddl", "capture", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Options: ddlOptions, Reference: ref}
	r := ddlResult("CREATE TRIGGER T")
	calls := 0
	reader := ddlReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref || len(s.Args) != 2 {
			t.Fatal(s)
		}
		return r, nil
	})
	got, err := Read(t.Context(), reader, request)
	if err != nil || !got.Complete || calls != 1 {
		t.Fatal(got, err, calls)
	}
	cause := errors.New("private DB cause")
	reader = func(context.Context, session.Statement) (session.Result, error) { return r, cause }
	got, err = Read(t.Context(), reader, request)
	if !errors.Is(err, cause) || got.Complete || got.Records != nil || strings.Contains(err.Error(), cause.Error()) {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return r, nil }
	got, err = Read(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Complete || got.Records != nil {
		t.Fatal(got, err)
	}
	request.Limits = rowset.Limits{MaxBytes: 1}
	reader = func(context.Context, session.Statement) (session.Result, error) { return r, nil }
	got, err = Read(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || got.Complete {
		t.Fatal(got, err)
	}
	request.Reference = reference.Reference{}
	if _, err = Read(t.Context(), reader, request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}
func TestMetadataNativeRowsAndAtomicJSONCompleteCLOB(t *testing.T) {
	text := "BEGIN null; -- \U0001f600\nEND;"
	r := ddlResult(text)
	values := make([]driver.Value, len(r.Columns))
	for i, v := range r.Rows[0] {
		values[i] = v.Text
	}
	sql, _, err := Statement(ddlOptions)
	if err != nil {
		t.Fatal(err)
	}
	db, state := testsql.New(t, testsql.Step{SQL: sql, Columns: r.Columns, Values: [][]driver.Value{values}})
	rows, err := Rows(t.Context(), db, ddlOptions)
	if err != nil || state.Closed() != 0 || rows == nil {
		t.Fatal(err, state.Closed())
	}
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	if err = rows.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	terminal := errors.New("terminal")
	closeErr := errors.New("close")
	for _, tc := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{
		{"terminal", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, NextErr: terminal}, rowset.Limits{}, terminal},
		{"close", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, CloseErr: closeErr}, rowset.Limits{}, closeErr},
		{"row-cap", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
		{"byte-cap", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 5}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			payload, err := JSON(t.Context(), db, ddlOptions, tc.limits)
			if !errors.Is(err, tc.want) || payload != nil || state.Closed() != 1 {
				t.Fatal(string(payload), err, state.Closed())
			}
		})
	}
	truncated := append([]driver.Value(nil), values...)
	truncated[3] = "prefix"
	db, _ = testsql.New(t, testsql.Step{Columns: r.Columns, Values: [][]driver.Value{truncated}})
	payload, err := JSON(t.Context(), db, ddlOptions, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || payload != nil {
		t.Fatal(string(payload), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}})
	payload, err = JSON(t.Context(), db, ddlOptions, rowset.Limits{})
	var got Result
	if err != nil || json.Unmarshal(payload, &got) != nil || !got.Complete || got.Records[0].DDL.Text != text {
		t.Fatal(string(payload), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	db, state = testsql.New(t, testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, OnNext: func(int) { cancel() }})
	payload, err = JSON(ctx, db, ddlOptions, rowset.Limits{})
	if !errors.Is(err, context.Canceled) || payload != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
}
