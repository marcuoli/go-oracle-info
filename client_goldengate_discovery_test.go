package oracleinfo_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
)

func TestGoldenGateDiscoveryFacadeIdentityRowsAndJSON(t *testing.T) {
	for _, kind := range []string{"extracts", "replicats"} {
		t.Run(kind, func(t *testing.T) {
			o := goldengate.ProcessOptions{OracleVersion: "19.26.1.0.0"}
			step := testsql.Step{}
			if kind == "extracts" {
				step.SQL, _, _ = goldengate.ExtractsStatement(o)
				step.Columns = []string{"CAPTURE_NAME", "CLIENT_NAME", "CAPTURE_USER", "STATUS", "CLIENT_STATUS", "PURPOSE", "SOURCE_DATABASE"}
				step.Values = [][]driver.Value{{"db_capture", "EXT_A", nil, "ENABLED", "DETACHED", "GoldenGate Capture", "db_source"}}
			} else {
				step.SQL, _, _ = goldengate.ReplicatsStatement(o)
				step.Columns = []string{"REPLICAT_NAME", "SERVER_NAME", "APPLY_USER", "STATUS"}
				step.Values = [][]driver.Value{{"REP_A", "db_server", nil, "ATTACHED"}}
			}
			legacy, canonical := (*info.Client).GoldenGateExtractsRows, (*info.Client).GoldenGateExtracts
			if kind == "replicats" {
				legacy, canonical = (*info.Client).GoldenGateReplicatsRows, (*info.Client).GoldenGateReplicats
			}
			checkClientCursorPair(t, append(versionSteps("19"), step),
				func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
					return legacy(c, ctx, info.GoldenGateProcessFilter{})
				},
				func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
					return canonical(c, ctx, info.GoldenGateProcessFilter{})
				})
			db, _ := testsql.New(t, append(versionSteps("19"), step)...)
			client := info.New(db)
			var err error
			var data json.RawMessage
			if kind == "extracts" {
				data, err = client.GoldenGateExtractsJSON(t.Context(), info.GoldenGateProcessFilter{})
			} else {
				data, err = client.GoldenGateReplicatsJSON(t.Context(), info.GoldenGateProcessFilter{})
			}
			var got struct {
				Provenance goldengate.Provenance
				Data       struct{ Rows [][]any }
			}
			if err != nil || json.Unmarshal(data, &got) != nil || !got.Provenance.Complete || len(got.Data.Rows) != 1 || got.Data.Rows[0][0] != step.Values[0][0] || got.Data.Rows[0][2] != nil {
				t.Fatal(string(data), err)
			}
		})
	}
}

func TestGoldenGateDiscoveryFacadeCatalogAndInvalidAdmission(t *testing.T) {
	o := goldengate.HeartbeatCatalogOptions{OracleVersion: "19.26.1.0.0", Schema: "quoted owner"}
	sql, _, err := goldengate.HeartbeatCatalogStatement(o)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{SQL: sql, Columns: []string{"OWNER", "OBJECT_NAME", "OBJECT_TYPE", "STATUS", "COLUMN_NAME", "DATA_TYPE", "DATA_TYPE_OWNER"}, Values: [][]driver.Value{{"quoted owner", "GG_LAG", "SYNONYM", "VALID", nil, nil, nil}}}
	db, state := testsql.New(t, append(versionSteps("19"), step)...)
	catalog, err := info.New(db).GoldenGateHeartbeatCatalog(t.Context(), o.Schema)
	if err != nil || !catalog.Provenance.Complete || len(catalog.Rows) != 1 || catalog.Rows[0].Supported || catalog.Rows[0].Reason != "unsupported_object_type" || state.Closed() != 3 {
		t.Fatal(catalog, err)
	}
	calls := state.Calls()
	if calls[len(calls)-1].Args[0].Value != "quoted owner" {
		t.Fatal(calls)
	}
	db, _ = testsql.New(t, append(versionSteps("19"), step)...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).GoldenGateHeartbeatCatalogRows(t.Context(), o.Schema)
	if err != nil || rows != spy.last || !rows.Next() {
		t.Fatal(rows, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	db, _ = testsql.New(t, append(versionSteps("19"), step)...)
	data, err := info.New(db).GoldenGateHeartbeatCatalogJSON(t.Context(), o.Schema)
	var got struct{ Data struct{ Rows [][]any } }
	if err != nil || json.Unmarshal(data, &got) != nil || len(got.Data.Rows) != 1 || got.Data.Rows[0][2] != "SYNONYM" || got.Data.Rows[0][4] != nil {
		t.Fatal(string(data), err)
	}
	db, state = testsql.New(t, append(versionSteps("19"), step)...)
	canonical, err := info.New(db).GoldenGateHeartbeatCatalogRowsJSON(t.Context(), o.Schema)
	if err != nil || !bytes.Equal(canonical, data) || state.Closed() != 3 {
		t.Fatal(string(canonical), err)
	}
	db, state = testsql.New(t)
	client := info.New(db)
	if _, err := client.GoldenGateExtractsRows(t.Context(), info.GoldenGateProcessFilter{Visibility: "invalid"}); !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if _, err := client.GoldenGateHeartbeatCatalog(t.Context(), "bad\x00owner"); !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if data, err := client.GoldenGateHeartbeatsJSON(t.Context(), goldengate.HeartbeatsOptions{}); data != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(string(data), err)
	}
	if data, err := client.GoldenGateHeartbeatLagJSON(t.Context(), goldengate.HeartbeatLagOptions{}); data != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(string(data), err)
	}
	if rows, err := client.GoldenGateHeartbeatsRows(t.Context(), goldengate.HeartbeatsOptions{}); rows != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(rows, err)
	}
	if rows, err := client.GoldenGateHeartbeatLagRows(t.Context(), goldengate.HeartbeatLagOptions{}); rows != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(rows, err)
	}
	if rows, err := client.GoldenGateHeartbeats(t.Context(), goldengate.HeartbeatsOptions{}); rows != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(rows, err)
	}
	if rows, err := client.GoldenGateHeartbeatLag(t.Context(), goldengate.HeartbeatLagOptions{}); rows != nil || !errors.Is(err, goldengate.ErrInvalidRequest) {
		t.Fatal(rows, err)
	}
	if len(state.Calls()) != 0 {
		t.Fatal("invalid admission executed SQL", state.Calls())
	}
}

func checkClientCursorPair(t *testing.T, steps []testsql.Step, legacy, canonical func(*info.Client, context.Context) (*sql.Rows, error)) {
	t.Helper()
	call := func(open func(*info.Client, context.Context) (*sql.Rows, error)) []testsql.Call {
		db, state := testsql.New(t, steps...)
		spy := &nativeSpy{db: db}
		rows, err := open(info.New(spy), t.Context())
		if err != nil || rows != spy.last || !rows.Next() {
			t.Fatalf("native cursor identity/lifetime: %v", err)
		}
		if err := rows.Close(); err != nil || state.Closed() != len(steps) {
			t.Fatalf("cursor close: %d %v", state.Closed(), err)
		}
		return state.Calls()
	}
	if oldCalls, newCalls := call(legacy), call(canonical); !reflect.DeepEqual(oldCalls, newCalls) {
		t.Fatalf("SQL or binds changed: %+v vs %+v", oldCalls, newCalls)
	}
}
