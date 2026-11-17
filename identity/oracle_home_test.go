package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func homeFixture(home session.Value) session.Result {
	return coreResult("INSTANCE_ID,ORACLE_HOME,LIBRARY_FILE_SPEC", coreText("2"), home, session.Value{Null: true})
}

func TestOracleHomeSourceAndInstanceSemantics(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 19, 23, 26} {
		sql, err := OracleHomeStatement(major)
		if err != nil {
			t.Fatal(err)
		}
		if major < 12 {
			if !strings.Contains(sql, "DBA_LIBRARIES") || !strings.Contains(sql, "OWNER = 'SYS'") || !strings.Contains(sql, "'DBMS_SUMADV_LIB'") {
				t.Fatal(sql)
			}
		} else if !strings.Contains(sql, "SYS_CONTEXT('USERENV','ORACLE_HOME')") || strings.Contains(sql, "DBA_LIBRARIES") {
			t.Fatal(sql)
		}
		input := homeFixture(coreText("/opt/oracle/product/db home"))
		if major < 12 {
			input.Rows[0][2] = coreText("/opt/oracle/product/db home/lib/libqsmashr.so")
		}
		fact, err := DecodeOracleHome(input, major)
		if err != nil || fact.InstanceID != 2 || fact.Home.State != Available || fact.Home.Value != "/opt/oracle/product/db home" || fact.Inferred != (major < 12) {
			t.Fatal(fact, err)
		}
	}
	if _, err := OracleHomeStatement(8); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}

func TestOracleHomeRejectsMalformedAndPreservesMissingStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input session.Result
		state State
		bad   bool
	}{
		{"null", homeFixture(session.Value{Null: true}), Null, false},
		{"empty", homeFixture(coreText("")), Available, false},
		{"no-rows", session.Result{Columns: []string{"INSTANCE_ID", "ORACLE_HOME", "LIBRARY_FILE_SPEC"}}, NoRows, false},
		{"shape", coreResult("ORACLE_HOME", coreText("home")), "", true},
		{"late-row", func() session.Result {
			v := homeFixture(coreText("home"))
			v.Rows = append(v.Rows, v.Rows[0])
			return v
		}(), "", true},
		{"bad-member", func() session.Result { v := homeFixture(coreText("home")); v.Rows[0][0] = coreText("0"); return v }(), "", true},
		{"false-null", homeFixture(session.Value{Null: true, Text: "hidden"}), "", true},
		{"nul", homeFixture(coreText("/bad\x00path")), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact, err := DecodeOracleHome(tc.input, 19)
			if tc.bad {
				if !errors.Is(err, ErrInvalidEvidence) || fact.Home.State != "" {
					t.Fatal(fact, err)
				}
				return
			}
			if err != nil || fact.Home.State != tc.state {
				t.Fatal(fact, err)
			}
		})
	}
}

func TestOracleHomeReadNeverPublishesFailedOrCancelledPayload(t *testing.T) {
	failure := errors.New("private driver details")
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		reader := &coreFixtureReader{query: func(context.Context, session.Statement) (session.Result, error) {
			if cancelled {
				cancel()
				return homeFixture(coreText("unusable")), nil
			}
			return homeFixture(coreText("unusable")), failure
		}}
		got, err := ReadOracleHome(ctx, reader, OracleHomeRequest{OracleMajor: 19, Reference: coreReference(t)})
		cancel()
		if err == nil || got.Home.State != "" || reader.calls != 1 {
			t.Fatal(got, err, reader.calls)
		}
		if !cancelled && !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if cancelled && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("driver details leaked")
		}
	}
}

func TestOracleHomeLegacyRefusesUnqualifiedLibraryPaths(t *testing.T) {
	for _, raw := range []string{"$ORACLE_HOME/lib/libqsmashr.so", "C:\\oracle\\bin\\oraqsmashr.dll", "/home/lib/", "relative/lib/lib.so", "/home/lib/subdir/lib.so"} {
		input := homeFixture(session.Value{Null: true})
		input.Rows[0][2] = coreText(raw)
		got, err := DecodeOracleHome(input, 11)
		if err != nil || got.Home.State != Null || got.LibraryFileSpec.Value != raw || !got.Inferred {
			t.Fatal(raw, got, err)
		}
		input.Rows[0][1] = coreText("/invented")
		if _, err := DecodeOracleHome(input, 11); !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal("unqualified candidate accepted", raw, err)
		}
	}
}
