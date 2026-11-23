package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"testing"
)

func administratorStep(t *testing.T) testsql.Step {
	t.Helper()
	sql, e := goldengate.AdministratorsStatement("19.26.1.0.0")
	if e != nil {
		t.Fatal(e)
	}
	return testsql.Step{SQL: sql, Columns: []string{"USERNAME", "PRIVILEGE_TYPE", "GRANT_SELECT_PRIVILEGES"}, Values: [][]driver.Value{{"GGADMIN", "*", "YES"}, {"Case Sensitive Admin", "APPLY", nil}}}
}
func TestGoldenGatePropertiesAndCompanions(t *testing.T) {
	t.Run("typed", func(t *testing.T) {
		db, state := testsql.New(t, append(versionSteps("19"), administratorStep(t))...)
		got, err := info.New(db).GoldenGate(t.Context())
		if err != nil || len(got.Administrators.Rows) != 2 || got.Administrators.Rows[1].Username.Text != "Case Sensitive Admin" || !got.Administrators.Provenance.Complete || got.ConfiguredSchema.State != goldengate.SchemaUnavailable || got.ConfiguredSchema.Name != nil || state.Closed() != 3 {
			t.Fatalf("wrong independent properties: %+v %v", got, err)
		}
	})
	t.Run("native", func(t *testing.T) {
		checkClientCursorPair(t, append(versionSteps("19"), administratorStep(t)),
			(*info.Client).GoldenGateAdministratorsRows, (*info.Client).GoldenGateAdministrators)
	})
	t.Run("propertiesJSON", func(t *testing.T) {
		db, _ := testsql.New(t, append(versionSteps("19"), administratorStep(t))...)
		data, err := info.New(db).GoldenGateJSON(t.Context())
		var got goldengate.Properties
		if err != nil || json.Unmarshal(data, &got) != nil || got.ConfiguredSchema.State != goldengate.SchemaUnavailable || got.ConfiguredSchema.Name != nil || len(got.Administrators.Rows) != 2 {
			t.Fatalf("wrong property JSON %s %v", data, err)
		}
	})
	t.Run("datasetJSON", func(t *testing.T) {
		db, _ := testsql.New(t, append(versionSteps("19"), administratorStep(t))...)
		data, err := info.New(db).GoldenGateAdministratorsJSON(t.Context())
		var got struct {
			Provenance goldengate.Provenance
			Data       struct{ Rows [][]any }
		}
		if err != nil || json.Unmarshal(data, &got) != nil || !got.Provenance.Complete || len(got.Data.Rows) != 2 || got.Data.Rows[1][0] != "Case Sensitive Admin" || got.Data.Rows[1][2] != nil {
			t.Fatalf("wrong native JSON %s %v", data, err)
		}
	})
}

func TestGoldenGatePropertyJSONRejectsFinalEnvelopeOverflow(t *testing.T) {
	db, _ := testsql.New(t, append(versionSteps("19"), administratorStep(t))...)
	valid, err := info.New(db).GoldenGateJSON(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	db, state := testsql.New(t, append(versionSteps("19"), administratorStep(t))...)
	data, err := info.New(db).GoldenGateJSON(t.Context(), info.JSONLimits{MaxRows: 2, MaxBytes: len(valid) - 1})
	if !errors.Is(err, rowset.ErrCapacity) || data != nil || state.Closed() != 3 {
		t.Fatalf("oversize properties published: %s %v", data, err)
	}
	db, state = testsql.New(t)
	data, err = info.New(db).GoldenGateJSON(t.Context(), info.JSONLimits{MaxRows: -1})
	if err == nil || data != nil || len(state.Calls()) != 0 {
		t.Fatalf("invalidlimits didSQL: %s %v calls%d", data, err, len(state.Calls()))
	}
}
