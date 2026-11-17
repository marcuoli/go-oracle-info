package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func homeStep() testsql.Step {
	return testsql.Step{Columns: []string{"INSTANCE_ID", "ORACLE_HOME", "LIBRARY_FILE_SPEC"}, Values: [][]driver.Value{{"2", "/db/home", nil}}}
}
func TestOracleHomeFacadeScalarNativeAndJSON(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		db, state := testsql.New(t, append(versionSteps("19"), homeStep())...)
		fact, err := info.New(db).OracleHome(t.Context())
		if err != nil || fact.Home.State != identity.Available || fact.Home.Value != "/db/home" || fact.InstanceID != 2 || fact.Inferred || state.Closed() != 3 {
			t.Fatal(fact, err)
		}
	})
	t.Run("native", func(t *testing.T) {
		db, state := testsql.New(t, append(versionSteps("19"), homeStep())...)
		spy := &nativeSpy{db: db}
		rows, err := info.New(spy).OracleHomeRows(t.Context())
		if err != nil || rows == nil || rows != spy.last || !rows.Next() {
			t.Fatal("native cursor lost", err)
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
		if state.Closed() != 3 {
			t.Fatal("native cursor not finalized")
		}
	})
	t.Run("json", func(t *testing.T) {
		db, state := testsql.New(t, append(versionSteps("19"), homeStep())...)
		payload, err := info.New(db).OracleHomeRowsJSON(t.Context())
		var fact struct {
			Provenance identity.Provenance
			Inferred   bool
			Data       json.RawMessage
		}
		if err != nil || json.Unmarshal(payload, &fact) != nil || fact.Inferred || fact.Provenance.Scope != "executing_instance" || !strings.Contains(string(fact.Data), `"/db/home"`) || !strings.Contains(string(fact.Data), "null") || state.Closed() != 3 {
			t.Fatal(string(payload), err)
		}
	})
}
func TestOracleHomeJSONPublishesNothingOnFailures(t *testing.T) {
	sentinel := errors.New("source failed")
	for _, mode := range []string{"query", "terminal", "close", "cancel", "rows", "bytes", "domain"} {
		t.Run(mode, func(t *testing.T) {
			step := homeStep()
			limits := info.JSONLimits{MaxRows: 10, MaxBytes: 10000}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := sentinel
			switch mode {
			case "query":
				step.QueryErr = sentinel
			case "terminal":
				step.NextErr = sentinel
			case "close":
				step.CloseErr = sentinel
			case "cancel":
				step.OnNext = func(int) { cancel() }
				want = context.Canceled
			case "rows":
				step.Values = append(step.Values, step.Values[0])
				want = rowset.ErrCapacity
			case "bytes":
				limits.MaxBytes = 1
				want = rowset.ErrCapacity
			case "domain":
				step.Values[0][0] = "0"
				want = identity.ErrInvalidEvidence
			}
			db, _ := testsql.New(t, append(versionSteps("19"), step)...)
			payload, err := info.New(db).OracleHomeRowsJSON(ctx, limits)
			if payload != nil || !errors.Is(err, want) {
				t.Fatal(mode, string(payload), err, want)
			}
		})
	}
}
func TestOracleHomeLegacyJSONMarksInferenceAndRawLibrary(t *testing.T) {
	for _, version := range []string{"9", "10", "11"} {
		t.Run(version, func(t *testing.T) {
			step := homeStep()
			step.Values = [][]driver.Value{{"1", "/old/home", "/old/home/lib/libqsmashr.so"}}
			db, _ := testsql.New(t, append(versionSteps(version)[:1], step)...)
			payload, err := info.New(db).OracleHomeRowsJSON(t.Context())
			if err != nil || !strings.Contains(string(payload), `"inferred":true`) || !strings.Contains(string(payload), "/old/home/lib/libqsmashr.so") {
				t.Fatal(string(payload), err)
			}
		})
	}
}
func TestOracleHomeFacadeFallbackClassifiesWrappedNativeError(t *testing.T) {
	first := homeStep()
	first.QueryErr = errors.New("ORA-02003: invalid USERENV parameter; private")
	last := homeStep()
	last.Values[0] = []driver.Value{"2", "/legacy/home", "/legacy/home/lib/lib.so"}
	db, _ := testsql.New(t, append(versionSteps("19"), first, last)...)
	client := info.New(&nativeSpy{db: db}) // Deliberately admits no ExecContext.
	got, err := client.OracleHome(t.Context())
	if err != nil || got.Home.Value != "/legacy/home" || !got.Inferred || len(got.Attempts) != 3 || got.Attempts[0].Code != "ORA-02003" || got.Attempts[1].Code != "executor_unavailable" {
		t.Fatal(got, err)
	}
}

func TestOracleHomeFallbackJSONPreservesSourceAndAttempts(t *testing.T) {
	first := homeStep()
	first.QueryErr = errors.New("ORA-02003: private-source-message")
	last := homeStep()
	last.Values[0] = []driver.Value{"2", "/legacy/home", "/legacy/home/lib/lib.so"}
	db, _ := testsql.New(t, append(versionSteps("19"), first, last)...)
	payload, err := info.New(&nativeSpy{db: db}).OracleHomeJSON(t.Context())
	var fact identity.OracleHomeObservation
	if err != nil || json.Unmarshal(payload, &fact) != nil || !fact.Inferred || fact.Home.Value != "/legacy/home" || len(fact.Attempts) != 3 || strings.Contains(string(payload), "private-source-message") {
		t.Fatal(string(payload), err)
	}
}
