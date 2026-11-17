package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"testing"
)

func versionsStep(t *testing.T) testsql.Step {
	t.Helper()
	statement, err := instances.VersionsStatement(23)
	if err != nil {
		t.Fatal(err)
	}
	return testsql.Step{SQL: statement, Columns: []string{"INST_ID", "INSTANCE_NUMBER", "INSTANCE_NAME", "HOST_NAME", "STATUS", "VERSION", "SESSION_USER", "ISDBA", "CON_ID", "CURRENT_CONTAINER_ID", "CLOUD_SERVICE", "VERSION_FULL"}, Values: [][]driver.Value{{int64(7), int64(7), "node7", nil, "MOUNTED", "23.0.0.0.0", "SYS", "TRUE", int64(0), "1", nil, "23.26.1.0.0"}, {int64(8), int64(8), "node8", "host8", "OPEN", "23.0.0.0.0", "SYS", "TRUE", int64(0), "1", nil, "23.26.0.0.0"}}}
}
func TestInstanceVersionsFacadeNativeCursorAndJSON(t *testing.T) {
	db, state := testsql.New(t, append(versionSteps("23"), versionsStep(t))...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).InstanceVersions(t.Context())
	if err != nil || rows != spy.last || !rows.Next() {
		t.Fatal("native cursor ownership lost", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if state.Closed() != 3 {
		t.Fatal("native cursor not closed")
	}
	db, state = testsql.New(t, append(versionSteps("23"), versionsStep(t))...)
	payload, err := info.New(db).InstanceVersionsJSON(t.Context())
	if err != nil || state.Closed() != 3 {
		t.Fatal(string(payload), err)
	}
	var envelope struct {
		Precision     string
		ContextSource string `json:"context_source"`
		Data          struct {
			Columns []rowset.Column `json:"columns"`
			Rows    [][]any         `json:"rows"`
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Precision != "full" || envelope.ContextSource != "SYS_CONTEXT('USERENV')" || len(envelope.Data.Rows) != 2 {
		t.Fatal(string(payload))
	}
	for i, c := range envelope.Data.Columns {
		if c.Name == "CURRENT_CONTAINER_ID" && (c.Kind != rowset.Text || envelope.Data.Rows[0][i] != "1") {
			t.Fatal("native context text changed", string(payload))
		}
		if c.Name == "CLOUD_SERVICE" && envelope.Data.Rows[0][i] != nil {
			t.Fatal("NULL lost")
		}
	}
	for _, failure := range []string{"next", "close", "rows", "bytes", "invalid-id"} {
		t.Run(failure, func(t *testing.T) {
			sentinel := errors.New("private failure")
			step := versionsStep(t)
			limits := info.JSONLimits{}
			switch failure {
			case "invalid-id":
				step.Values[1][0] = int64(0)
			case "next":
				step.NextErr = sentinel
			case "close":
				step.CloseErr = sentinel
			case "rows":
				limits.MaxRows = 1
			case "bytes":
				limits.MaxBytes = 500
			}
			db, state := testsql.New(t, append(versionSteps("23"), step)...)
			data, err := info.New(db).InstanceVersionsJSON(t.Context(), limits)
			if err == nil || data != nil || state.Closed() != 3 {
				t.Fatalf("failed prefix published: %s %v closed%d", data, err, state.Closed())
			}
		})
	}
}
