package database

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
	"testing"
)

type coreFixtureReader struct {
	query func(context.Context, session.Statement) (session.Result, error)
	calls int
}

func (r *coreFixtureReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	r.calls++
	return r.query(ctx, s)
}
func coreReference(t *testing.T) reference.Reference {
	t.Helper()
	ref, err := reference.NewReference("unit", "core", "facts", 1)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func coreResult(columns string, values ...session.Value) session.Result {
	return session.Result{Columns: strings.Split(columns, ","), Rows: [][]session.Value{values}}
}
func coreText(value string) session.Value { return session.Value{Text: value} }

func TestNLSAndOptionsKeepNamesNullUnknownAndLateFailures(t *testing.T) {
	nls := coreResult("VALUE,NAME", coreText(" AL16UTF16 "), coreText("NLS_NCHAR_CHARACTERSET"))
	facts, err := DecodeNLSDatabase(nls, 11)
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Name.Text != "NLS_NCHAR_CHARACTERSET" || facts.Rows[0].Value.Text != " AL16UTF16 " || facts.Provenance.Source != "NLS_DATABASE_PARAMETERS" {
		t.Fatalf("NLS: %+v %v", facts, err)
	}
	nls.Rows = append(nls.Rows, []session.Value{{Null: true}, coreText("NLS_LANGUAGE")})
	facts, err = DecodeNLSDatabase(nls, 26)
	if err != nil || !facts.Rows[1].Value.Null {
		t.Fatalf("NLS NULL: %+v %v", facts, err)
	}
	options := coreResult("PARAMETER,VALUE", coreText("future option"), coreText("UNKNOWN"))
	opt, err := DecodeOptions(options, 19)
	if err != nil || opt.Rows[0].Value.Text != "UNKNOWN" || opt.Provenance.Source != "V$OPTION" {
		t.Fatalf("options: %+v %v", opt, err)
	}
	options.Rows = append(options.Rows, []session.Value{coreText("bad")})
	opt, err = DecodeOptions(options, 19)
	if !errors.Is(err, ErrInvalidEvidence) || opt.Rows != nil {
		t.Fatalf("late prefix: %+v %v", opt, err)
	}
	nls.Rows = append(nls.Rows, []session.Value{coreText("\xff"), coreText("NLS_TERRITORY")})
	facts, err = DecodeNLSDatabase(nls, 19)
	if !errors.Is(err, ErrInvalidEvidence) || facts.Rows != nil {
		t.Fatalf("UTF8 prefix: %+v %v", facts, err)
	}
}

func TestDatabaseOwnedReadsRejectFailedAndCancelledPayloads(t *testing.T) {
	for _, operation := range []string{"nls", "options"} {
		for _, mode := range []string{"success", "terminal", "cancel", "late", "typed-nil", "invalid-ref"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				ref := coreReference(t)
				nls := coreResult("NAME,VALUE", coreText("NLS_LANGUAGE"), coreText("AMERICAN"))
				option := coreResult("PARAMETER,VALUE", coreText("future option"), session.Value{Null: true})
				failure := errors.New("private reader failure")
				reader := &coreFixtureReader{query: func(_ context.Context, statement session.Statement) (session.Result, error) {
					want, _ := NLSDatabaseStatement(19)
					result := nls
					if operation == "options" {
						want, _ = OptionsStatement(19)
						result = option
					}
					if statement.Reference != ref || statement.Text != want || len(statement.Args) != 0 {
						t.Fatal("changed reference/SQL")
					}
					switch mode {
					case "terminal":
						return result, failure
					case "cancel":
						cancel()
					case "late":
						result.Rows = append(result.Rows, nil)
					}
					return result, nil
				}}
				var admitted Reader = reader
				if mode == "typed-nil" {
					var absent *coreFixtureReader
					admitted = absent
				}
				if mode == "invalid-ref" {
					ref = reference.Reference{}
				}
				rows := 0
				var err error
				if operation == "nls" {
					facts, e := ReadNLSDatabase(ctx, admitted, NLSDatabaseRequest{OracleMajor: 19, Reference: ref})
					rows = len(facts.Rows)
					err = e
				} else {
					facts, e := ReadOptions(ctx, admitted, OptionsRequest{OracleMajor: 19, Reference: ref})
					rows = len(facts.Rows)
					err = e
				}
				if mode == "success" {
					if err != nil || rows != 1 {
						t.Fatalf("success: %d %v", rows, err)
					}
					return
				}
				if err == nil || rows != 0 {
					t.Fatalf("failed payload retained: %d %v", rows, err)
				}
				if mode == "terminal" && (!errors.Is(err, failure) || strings.Contains(err.Error(), "private")) {
					t.Fatal("lost/wrote private cause")
				}
				if mode == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if (mode == "typed-nil" || mode == "invalid-ref") && reader.calls != 0 {
					t.Fatal("invalid request queried")
				}
			})
		}
	}
}
