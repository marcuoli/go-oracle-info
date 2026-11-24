package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-info/services"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func scanSteps(t *testing.T, remote driver.Value) []testsql.Step {
	t.Helper()
	param := parameterStep(true, false)
	param.Values = [][]driver.Value{append([]driver.Value(nil), param.Values[0]...), append([]driver.Value(nil), param.Values[0]...)}
	for i, col := range param.Columns {
		switch col {
		case "NAME":
			param.Values[0][i], param.Values[1][i] = "listener_networks", "remote_listener"
		case "VALUE":
			param.Values[0][i], param.Values[1][i] = nil, remote
		}
	}
	var err error
	param.SQL, _, err = parameters.EffectiveStatement(23, parameters.Filter{Names: []string{"remote_listener", "listener_networks"}})
	if err != nil {
		t.Fatal(err)
	}
	listenerSQL, err := services.ListenerNetworksStatement(services.ListenerNetworksOptions{OracleVersion: "23.0.0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	network := testsql.Step{SQL: listenerSQL, Columns: []string{"INST_ID", "NETWORK", "VALUE", "CON_ID"},
		Values: [][]driver.Value{{int64(1), nil, "(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.21)(PORT=1521))", int64(0)}}}
	return append(versionSteps("23"), param, network)
}

func TestScanFacadeUsesParametersAndKeepsRegistrationIPs(t *testing.T) {
	for _, test := range []struct {
		remote, want string
	}{
		{"scan.example.com:1521", "scan.example.com"},
		{"192.0.2.21:1521", ""},
	} {
		t.Run(test.remote, func(t *testing.T) {
			steps := scanSteps(t, test.remote)
			db, trace := testsql.New(t, steps...)
			data, err := info.New(db).ScanJSON(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var facts services.ScanObservation
			if err := json.Unmarshal(data, &facts); err != nil {
				t.Fatal(err)
			}
			if facts.Name != test.want || facts.RegistrationState != services.ScanRegistrationAvailable ||
				facts.Registration == nil || !facts.Registration.Provenance.Complete || len(facts.Registration.Rows) != 1 ||
				!facts.Registration.Rows[0].Network.Null || facts.Registration.Rows[0].Value.Text != "(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.21)(PORT=1521))" {
				t.Fatalf("unexpected discovery %s", data)
			}
			if len(facts.Parameters) != 2 || !facts.Parameters[0].Value.Null || facts.Parameters[1].Value.Text != test.remote || trace.Closed() != len(steps) {
				t.Fatalf("missing source facts or cursor cleanup: %s", data)
			}
			call := trace.Calls()[len(steps)-2]
			if len(call.Args) != 2 || call.Args[0].Value != "remote_listener" || call.Args[1].Value != "listener_networks" {
				t.Fatalf("wrong parameter filter: %+v", call.Args)
			}
		})
	}
}

func TestScanFacadeDistinguishesUnavailabilityFromFailure(t *testing.T) {
	transport := errors.New("test stream failure")
	terminalOracle := errors.New("ORA-00942: terminal Oracle error")
	for _, test := range []struct {
		name                       string
		queryErr, nextErr, wantErr error
		wantState                  services.ScanRegistrationState
	}{
		{"inaccessible", errors.New("ORA-01031: insufficient privileges"), nil, nil, services.ScanRegistrationInaccessible},
		{"unavailable", errors.New("ORA-00942: table or view does not exist"), nil, nil, services.ScanRegistrationUnavailable},
		{"empty", nil, nil, nil, services.ScanRegistrationNoRows},
		{"terminal", nil, transport, transport, ""},
		{"singleton terminal Oracle error", nil, terminalOracle, terminalOracle, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			steps := scanSteps(t, "scan.example.com:1521")
			view := &steps[len(steps)-1]
			view.QueryErr, view.NextErr = test.queryErr, test.nextErr
			if test.name == "empty" {
				view.Values = nil
			}
			db, _ := testsql.New(t, steps...)
			data, err := info.New(db).ScanJSON(t.Context())
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || data != nil {
					t.Fatalf("failed cursor emitted %s, %v", data, err)
				}
				return
			}
			var facts services.ScanObservation
			if err != nil || json.Unmarshal(data, &facts) != nil || facts.Name != "scan.example.com" || facts.RegistrationState != test.wantState {
				t.Fatalf("unexpected evidence: %s, %v", data, err)
			}
		})
	}
}

func TestListenerNetworksCursorAndScanJSONBudget(t *testing.T) {
	steps := scanSteps(t, "scan.example.com:1521")
	listeners := append(append([]testsql.Step{}, steps[:len(steps)-2]...), steps[len(steps)-1])
	db, _ := testsql.New(t, listeners...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).ListenerNetworks(t.Context())
	if err != nil || rows != spy.last || !rows.Next() {
		t.Fatalf("native listener view cursor: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	db, _ = testsql.New(t, steps...)
	data, err := info.New(db).ScanJSON(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, bounds := range []struct {
		limit   int
		wantErr error
	}{{len(data), nil}, {len(data) - 1, rowset.ErrCapacity}} {
		db, _ := testsql.New(t, steps...)
		got, err := info.New(db).ScanJSON(t.Context(), info.JSONLimits{MaxRows: 16, MaxBytes: bounds.limit})
		if !errors.Is(err, bounds.wantErr) || (bounds.wantErr != nil && got != nil) || (bounds.wantErr == nil && string(got) != string(data)) {
			t.Fatalf("SQL discovery JSON limit %d: %s, %v", bounds.limit, got, err)
		}
	}
}

func TestScanCaseInsensitiveMetadataAndAggregateRows(t *testing.T) {
	steps := scanSteps(t, "scan.example.com:1521")
	for _, step := range steps[len(steps)-2:] {
		for i, col := range step.Columns {
			step.Columns[i] = strings.ToLower(col)
		}
	}
	db, _ := testsql.New(t, steps...)
	observation, err := info.New(db).Scan(t.Context(), info.JSONLimits{MaxRows: 3})
	if err != nil || observation.Name != "scan.example.com" {
		t.Fatalf("case-insensitive metadata rejected: %+v, %v", observation, err)
	}

	db, _ = testsql.New(t, steps...)
	data, err := info.New(db).ScanJSON(t.Context(), info.JSONLimits{MaxRows: 2})
	if !errors.Is(err, rowset.ErrCapacity) || data != nil {
		t.Fatalf("aggregate row limit emitted %s, %v", data, err)
	}
}
