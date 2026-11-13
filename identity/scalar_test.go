package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestScalarStatementsPreserveExactProjections(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		got  string
		want string
	}{
		{"name", identity.NameStatement(), "SELECT NAME FROM V$DATABASE"},
		{"role", identity.RoleStatement(), "SELECT DATABASE_ROLE \n\t\t\t\t\tFROM V$DATABASE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("statement = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestScalarDecoders(t *testing.T) {
	t.Parallel()
	for _, decoder := range []struct {
		name   string
		column string
		value  string
		decode func(session.Result) (identity.Text, error)
	}{
		{"name", "NAME", "  DB_α\t\n", identity.DecodeName},
		{"role", "DATABASE_ROLE", " PHYSICAL STANDBY \t", identity.DecodeRole},
	} {
		t.Run(decoder.name, func(t *testing.T) {
			for _, tt := range []struct {
				name   string
				result session.Result
				want   identity.Text
			}{
				{"exact text", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Text: decoder.value}}}}, identity.Text{State: identity.Available, Value: decoder.value}},
				{"empty text", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Text: ""}}}}, identity.Text{State: identity.Available}},
				{"zero text", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Text: "0"}}}}, identity.Text{State: identity.Available, Value: "0"}},
				{"SQL NULL", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Null: true, Text: "ignored"}}}}, identity.Text{State: identity.Null}},
				{"no rows", session.Result{Columns: []string{decoder.column}}, identity.Text{State: identity.NoRows}},
				{"lowercase column", session.Result{Columns: []string{strings.ToLower(decoder.column)}, Rows: [][]session.Value{{{Text: decoder.value}}}}, identity.Text{State: identity.Available, Value: decoder.value}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := decoder.decode(tt.result)
					if err != nil || got != tt.want {
						t.Fatalf("decode = %+v, %v; want %+v, nil", got, err, tt.want)
					}
				})
			}
			for _, tt := range []struct {
				name   string
				result session.Result
			}{
				{"missing columns", session.Result{}},
				{"wrong column", session.Result{Columns: []string{"DATABASE_NAME"}}},
				{"empty column", session.Result{Columns: []string{""}}},
				{"column whitespace", session.Result{Columns: []string{" " + decoder.column}}},
				{"extra columns", session.Result{Columns: []string{decoder.column, "OTHER"}}},
				{"duplicate columns", session.Result{Columns: []string{decoder.column, decoder.column}}},
				{"missing row value", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{nil}}},
				{"extra row value", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Text: "first"}, {Text: "extra"}}}}},
				{"multiple rows", session.Result{Columns: []string{decoder.column}, Rows: [][]session.Value{{{Text: "same"}}, {{Text: "same"}}}}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := decoder.decode(tt.result)
					if !errors.Is(err, identity.ErrInvalidEvidence) || got != (identity.Text{}) {
						t.Fatalf("decode = %+v, %v; want zero Text, ErrInvalidEvidence", got, err)
					}
				})
			}
		})
	}
}
