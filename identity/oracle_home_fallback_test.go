package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

type homeFallbackReader struct {
	query func(context.Context, session.Statement) (session.Result, error)
	exec  func(context.Context, session.Statement) error
	calls []string
}

func (r *homeFallbackReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	label := "context"
	if strings.Contains(s.Text, "SYS.DBA_LIBRARIES") {
		label = "library"
	}
	r.calls = append(r.calls, label)
	return r.query(ctx, s)
}
func (r *homeFallbackReader) Exec(ctx context.Context, s session.Statement, _ session.ExecOptions) (sql.Result, error) {
	r.calls = append(r.calls, "environment")
	if err := r.exec(ctx, s); err != nil {
		return nil, err
	}
	return driver.RowsAffected(0), nil
}
func setHomeOut(t *testing.T, s session.Statement, home string) {
	t.Helper()
	if s.Text != OracleHomeEnvironmentStatement() || len(s.Args) != 2 {
		t.Fatal("unexpected environment statement")
	}
	if len(*s.Args[0].(sql.Out).Dest.(*string)) != 4000 || len(*s.Args[1].(sql.Out).Dest.(*string)) != 32 {
		t.Fatal("missing bounded output storage")
	}
	*s.Args[0].(sql.Out).Dest.(*string) = home
	*s.Args[1].(sql.Out).Dest.(*string) = "2"
}
func TestOracleHomeFallbackOrderAndRetainedAttempts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		major    int
		want     []string
		source   OracleHomeSource
		inferred bool
	}{
		{"environment", 19, []string{"context", "environment"}, OracleHomeEnvironment, false},
		{"library", 19, []string{"context", "environment", "library"}, OracleHomeLibrary, true},
		{"legacy-environment", 11, []string{"environment"}, OracleHomeEnvironment, false},
		{"10g-environment", 10, []string{"environment"}, OracleHomeEnvironment, false},
		{"9i-library", 9, []string{"environment", "library"}, OracleHomeLibrary, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &homeFallbackReader{}
			r.query = func(_ context.Context, s session.Statement) (session.Result, error) {
				if strings.Contains(s.Text, "SYS.DBA_LIBRARIES") {
					v := homeFixture(coreText("/library"))
					v.Rows[0][2] = coreText("/library/lib/lib.so")
					return v, nil
				}
				return session.Result{}, &session.Error{Code: "ORA-02003"}
			}
			r.exec = func(_ context.Context, s session.Statement) error {
				setHomeOut(t, s, "/environment")
				if tc.name == "library" || tc.name == "9i-library" {
					return &session.Error{Code: "ORA-01031"}
				}
				return nil
			}
			got, err := ReadOracleHome(t.Context(), r, OracleHomeRequest{OracleMajor: tc.major, Reference: coreReference(t)})
			if err != nil || !reflect.DeepEqual(r.calls, tc.want) || got.Provenance.Source != string(tc.source) || got.Inferred != tc.inferred || len(got.Attempts) != len(tc.want) {
				t.Fatal(got, err, r.calls)
			}
			if tc.inferred && (got.Home.Value != "/library" || got.LibraryFileSpec.Value != "/library/lib/lib.so") {
				t.Fatal("failed OUT value survived", got)
			}
		})
	}
}
func TestOracleHomeFallbackStopsUnsafeFailures(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, driver.ErrBadConn, session.ErrUnavailable, errors.New("private transport error"), errors.Join(&session.Error{Code: "ORA-01031"}, driver.ErrBadConn), errors.Join(&session.Error{Code: "ORA-01031"}, errors.New("cursor close failed")), errors.Join(&session.Error{Code: "ORA-01031"}, ErrInvalidEvidence)} {
		r := &homeFallbackReader{query: func(context.Context, session.Statement) (session.Result, error) {
			return homeFixture(coreText("unsafe")), failure
		}, exec: func(context.Context, session.Statement) error { t.Fatal("unsafe fallback"); return nil }}
		got, err := ReadOracleHome(t.Context(), r, OracleHomeRequest{OracleMajor: 19, Reference: coreReference(t)})
		if err == nil || !errors.Is(err, failure) || got.Home.State != "" || !reflect.DeepEqual(r.calls, []string{"context"}) {
			t.Fatal(got, err, r.calls)
		}
	}
}
func TestOracleHomeNullContinuesAndContextCancellationDiscardsOUT(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &homeFallbackReader{query: func(context.Context, session.Statement) (session.Result, error) {
		return homeFixture(session.Value{Null: true}), nil
	}}
	r.exec = func(_ context.Context, s session.Statement) error {
		setHomeOut(t, s, "/must-not-publish")
		cancel()
		return nil
	}
	got, err := ReadOracleHome(ctx, r, OracleHomeRequest{OracleMajor: 19, Reference: coreReference(t)})
	if !errors.Is(err, context.Canceled) || got.Home.State != "" || len(r.calls) != 2 {
		t.Fatal(got, err, r.calls)
	}
}
