package goldengate

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func administratorNativeStep(t *testing.T) testsql.Step {
	t.Helper()
	sql, e := AdministratorsStatement("19.32.0.0.0")
	if e != nil {
		t.Fatal(e)
	}
	s := testsql.Step{SQL: sql, Columns: []string{"USERNAME", "PRIVILEGE_TYPE", "GRANT_SELECT_PRIVILEGES"}}
	for i := 0; i < 20; i++ {
		s.Values = append(s.Values, []driver.Value{fmt.Sprintf("Quoted Admin %02d", i), "*", nil})
	}
	return s
}
func TestAdministratorNativePropertiesAndJSONAreCompleteOrFailed(t *testing.T) {
	request := Request{OracleVersion: "19.32.0.0.0", Reference: ref(t)}
	for _, failure := range []string{"valid", "query", "late", "close", "cancel", "rows", "bytes", "invalidUsername"} {
		t.Run(failure, func(t *testing.T) {
			step := administratorNativeStep(t)
			limits := rowset.Limits{MaxRows: 30, MaxBytes: 32768}
			cause := io.ErrUnexpectedEOF
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch failure {
			case "query":
				step.QueryErr = cause
			case "late":
				step.NextErr = cause
			case "close":
				step.CloseErr = cause
			case "cancel":
				step.OnNext = func(int) { cancel() }
			case "rows":
				limits.MaxRows = 19
			case "bytes":
				limits.MaxBytes = 1
			case "invalidUsername":
				step.Values[len(step.Values)-1][0] = nil
			}
			db, state := testsql.New(t, step)
			facts, err := ReadNativeProperties(ctx, db, request, limits)
			if failure == "valid" {
				if err != nil || len(facts.Administrators.Rows) != 20 || facts.Administrators.Rows[19].Username.Text != "Quoted Admin 19" || !facts.Administrators.Provenance.Complete || facts.ConfiguredSchema.Name != nil || facts.ConfiguredSchema.State != SchemaUnavailable || state.Closed() != 1 {
					t.Fatalf("20rows property truncated/changed: %+v %v", facts, err)
				}
			} else if err == nil || len(facts.Administrators.Rows) != 0 || facts.Administrators.Provenance.Complete {
				t.Fatalf("published %s prefix %+v %v", failure, facts, err)
			}
			jsonCtx, jsonCancel := context.WithCancel(t.Context())
			defer jsonCancel()
			if failure == "cancel" {
				step.OnNext = func(int) { jsonCancel() }
			}
			db, state = testsql.New(t, step)
			payload, err := AdministratorsJSON(jsonCtx, db, request.OracleVersion, limits)
			if failure == "valid" {
				var got struct {
					Provenance Provenance
					Data       struct{ Rows [][]any }
				}
				if err != nil || json.Unmarshal(payload, &got) != nil || len(got.Data.Rows) != 20 || got.Data.Rows[19][0] != "Quoted Admin 19" || got.Data.Rows[19][2] != nil || !got.Provenance.Complete || state.Closed() != 1 {
					t.Fatalf("JSON source changed %s %v", payload, err)
				}
				return
			}
			if payload != nil || err == nil {
				t.Fatalf("published %s JSONprefix %s %v", failure, payload, err)
			}
			if (failure == "late" || failure == "close" || failure == "query") && !errors.Is(err, cause) {
				t.Fatalf("lost originalcause: %v", err)
			}
		})
	}
}
func TestAdministratorUnsupportedVersionDoesNotQuery(t *testing.T) {
	for _, version := range []string{"9.2.0.8.0", "11.2.0.4.0", "12.1.0.2.0", "26.1.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			db, state := testsql.New(t)
			request := Request{OracleVersion: version, Reference: ref(t)}
			_, e := ReadNativeProperties(t.Context(), db, request, rowset.Limits{})
			if !errors.Is(e, ErrUnsupportedVersion) {
				t.Fatal(e)
			}
			data, e := AdministratorsJSON(t.Context(), db, version, rowset.Limits{})
			if !errors.Is(e, ErrUnsupportedVersion) || data != nil {
				t.Fatalf("unsupportedJSON %s %v", data, e)
			}
			rows, e := AdministratorsRows(t.Context(), db, version)
			if !errors.Is(e, ErrUnsupportedVersion) || rows != nil || len(state.Calls()) != 0 {
				t.Fatalf("unsupported queried %v calls%d", e, len(state.Calls()))
			}
		})
	}
}
