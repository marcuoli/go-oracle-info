package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestConfiguredScanName(t *testing.T) {
	const addr = "(ADDRESS=(PROTOCOL=TCP)(HOST=RAC-scan.Example.com.)(PORT=1521))"
	const expected = "rac-scan.example.com"
	for _, test := range []struct {
		name, remote, networks, want string
	}{
		{"easy connect", "RAC-scan.Example.com.:1521", "", expected},
		{"nested descriptor", "(DESCRIPTION=(ADDRESS_LIST=(LOAD_BALANCE=ON)" + addr + ")(CONNECT_DATA=(SERVICE_NAME=local.example.com)))", "", expected},
		{"listener networks only", "", "((NAME=public)(LOCAL_LISTENER=node1.example.com:1521)(REMOTE_LISTENER=" + addr + "))", expected},
		{"quoted remote list", "", "((NAME=public)(REMOTE_LISTENER=\"rac-scan.example.com:1521,RAC-SCAN.EXAMPLE.COM.:1522\"))", expected},
		{"multiple networks same name", expected + ":1521", "((NAME=public)(REMOTE_LISTENER=" + expected + ":1522)),((NAME=private)(REMOTE_LISTENER=" + addr + "))", expected},
		{"local and forward only", "", "((NAME=private)(LOCAL_LISTENER=node1:1521)(FORWARD_LISTENER=forward:1521))", ""},
		{"IPv4", "192.0.2.21:1521", "", ""},
		{"IPv6", "[2001:db8::21]:1521", "", ""},
		{"descriptor IP", strings.Replace(addr, "RAC-scan.Example.com.", "2001:db8::21", 1), "", ""},
		{"alias with dots", "rac-scan.example.com", "", ""},
		{"conflicting networks", expected + ":1521", "((NAME=private)(REMOTE_LISTENER=other-scan.example.com:1521))", ""},
		{"hostname and IP in configured list", expected + ":1521,192.0.2.21:1521", "", ""},
		{"hostname and unresolved alias", expected + ":1521", "((NAME=private)(REMOTE_LISTENER=remote2))", ""},
		{"malformed second target", addr, "((NAME=public)(REMOTE_LISTENER=" + addr + ")", ""},
		{"non-TCP protocol", strings.Replace(addr, "PROTOCOL=TCP", "PROTOCOL=IPC", 1), "", ""},
		{"bad port", expected + ":65536", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := []ScanParameter{
				{InstanceID: session.Value{Text: "1"}, Name: session.Value{Text: "remote_listener"}, Value: session.Value{Text: test.remote, Null: test.remote == ""}},
				{InstanceID: session.Value{Text: "1"}, Name: session.Value{Text: "listener_networks"}, Value: session.Value{Text: test.networks, Null: test.networks == ""}},
			}
			if got := configuredScanName(rows); got != test.want {
				t.Errorf("configured name = %q, want %q", got, test.want)
			}
		})
	}
}

func TestListenerNetworksSourceMatrix(t *testing.T) {
	for _, test := range []struct {
		version             string
		admitted, container bool
	}{
		{"9.2.0.8.0", false, false}, {"10.1.0.5.0", false, false}, {"10.2.0.5.0", false, false},
		{"11.1.0.7.0", false, false}, {"11.2.0.1.0", true, false}, {"11.2.0.4.0", true, false},
		{"12.1.0.1.0", true, true}, {"12.1.0.2.0", true, true}, {"12.2.0.1.0", true, true},
		{"18.0.0.0.0", true, true}, {"19.0.0.0.0", true, true}, {"21.0.0.0.0", true, true},
		{"23.4.0.0.0", true, true}, {"23.26.1.0.0", true, true}, {"26.0.0.0.0", false, false},
	} {
		t.Run(test.version, func(t *testing.T) {
			options := ListenerNetworksOptions{OracleVersion: test.version}
			sql, err := ListenerNetworksStatement(options)
			if !test.admitted {
				if sql != "" || !errors.Is(err, ErrUnsupportedVersion) {
					t.Fatalf("unexpected unsupported selection: %q, %v", sql, err)
				}
				return
			}
			if err != nil || !strings.Contains(sql, "WHERE v.TYPE = 'REMOTE LISTENER'") || strings.Contains(sql, "v.CON_ID AS CON_ID") != test.container {
				t.Fatalf("wrong selection: %q, %v", sql, err)
			}
			result := session.Result{Columns: []string{"INST_ID", "NETWORK", "VALUE"}, Rows: [][]session.Value{{{Text: "2"}, {Null: true}, {Text: "192.0.2.21:1521"}}}}
			if test.container {
				result.Columns = append(result.Columns, "CON_ID")
				result.Rows[0] = append(result.Rows[0], session.Value{Text: "0"})
			}
			facts, err := DecodeListenerNetworks(result, options)
			if err != nil || len(facts.Rows) != 1 || !facts.Rows[0].Network.Null || facts.Rows[0].InstanceID.Text != "2" ||
				facts.Rows[0].Value.Text != "192.0.2.21:1521" || (facts.Rows[0].ContainerID != nil) != test.container || facts.Provenance.Complete {
				t.Fatalf("incorrectly decoded rows: %+v, %v", facts, err)
			}
			result.Rows = append(result.Rows, append([]session.Value(nil), result.Rows[0]...))
			result.Rows[1][0] = session.Value{Text: "0"}
			if facts, err := DecodeListenerNetworks(result, options); !errors.Is(err, ErrInvalidEvidence) || len(facts.Rows) != 0 {
				t.Fatalf("malformed late row yielded facts: %+v, %v", facts, err)
			}
		})
	}
}

type scanTestReader func(context.Context, session.Statement) (session.Result, error)

func (r scanTestReader) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	return r(ctx, statement)
}

func TestScanRejectsPreSCANAndCancellation(t *testing.T) {
	ref, err := reference.NewReference("info", "services", "scan", 1)
	if err != nil {
		t.Fatal(err)
	}
	r := scanTestReader(func(context.Context, session.Statement) (session.Result, error) {
		t.Fatal("read executed before version and context admission")
		return session.Result{}, nil
	})
	req := ScanRequest{OracleVersion: "11.1.0.7.0", ParametersReference: ref, ListenersReference: ref}
	if _, err = ReadScan(t.Context(), r, req); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req.OracleVersion = "19.0.0.0.0"
	if _, err = ReadScan(ctx, r, req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestConfiguredScanNameRequiresSameReportingScope(t *testing.T) {
	rows := []ScanParameter{
		{InstanceID: session.Value{Text: "1"}, ContainerID: &session.Value{Text: "0"}, Name: session.Value{Text: "remote_listener"}, Value: session.Value{Text: "scan.example.com:1521"}},
		{InstanceID: session.Value{Text: "1"}, ContainerID: &session.Value{Text: "3"}, Name: session.Value{Text: "listener_networks"}, Value: session.Value{Null: true}},
	}
	if got := configuredScanName(rows); got != "" {
		t.Fatalf("separate container projections were combined: %s", got)
	}
}
