package instances_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestClusterInterconnectsReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		version string
		layout  string
	}{
		{"9.2.0.8.0", ""}, {"10.1.0.5.0", ""}, {"10.2.0.5.0", "legacy"},
		{"11.1.0.7.0", "legacy"}, {"11.2.0.4.0", "legacy"},
		{"12.1.0.1.0", "container"}, {"12.2.0.1.0", "container"},
		{"18.4.0.0.0", "container"}, {"19.3.0.0.0", "container"}, {"19.33.0.0.0", "container"},
		{"21.3.0.0.0", "container"}, {"23.9.0.25.07", "container"}, {"23.26.1.0.0", "container"},
		{"26.1.0.0.0", ""}, {"19c", ""},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := instances.ClusterInterconnectsOptions{OracleVersion: tc.version}
			statement, err := instances.ClusterInterconnectsStatement(options)
			if tc.layout == "" {
				if statement != "" || !errors.Is(err, instances.ErrUnsupportedVersion) {
					t.Fatalf("unsupported selector returned %q, %v", statement, err)
				}
				db, observed := testsql.New(t)
				payload, err := instances.ClusterInterconnectsJSON(t.Context(), db, options, rowset.Limits{})
				if payload != nil || len(observed.Calls()) != 0 || !errors.Is(err, instances.ErrUnsupportedVersion) {
					t.Fatal("unsupported JSON opened SQL", err)
				}
				return
			}
			want := "SELECT i.INST_ID AS INST_ID, i.NAME AS NAME, i.IP_ADDRESS AS IP_ADDRESS, i.IS_PUBLIC AS IS_PUBLIC, i.SOURCE AS SOURCE"
			names := []string{"INST_ID", "NAME", "IP_ADDRESS", "IS_PUBLIC", "SOURCE"}
			row := []session.Value{{Text: "4"}, {Text: "net0"}, {Text: "2001:db8::4"}, {Null: true}, {Text: "Oracle Cluster Registry"}}
			if tc.layout == "container" {
				want += ", i.CON_ID AS CON_ID"
				names = append(names, "CON_ID")
				row = append(row, session.Value{Null: true})
			}
			want += " FROM SYS.GV_$CLUSTER_INTERCONNECTS i"
			if statement != want || err != nil {
				t.Fatal(statement, err)
			}
			got, err := instances.DecodeClusterInterconnects(session.Result{Columns: names, Rows: [][]session.Value{row}}, options)
			if err != nil || got.State != instances.Available || got.Provenance.Complete ||
				got.Provenance.Source != "GV$CLUSTER_INTERCONNECTS" || got.Provenance.Scope != "visible_runtime_interconnects" ||
				len(got.Rows) != 1 || got.Rows[0].Name.Text != "net0" || got.Rows[0].IPAddress.Text != "2001:db8::4" || !got.Rows[0].IsPublic.Null {
				t.Fatal(got, err)
			}
			id := got.Rows[0].ContainerID
			if (tc.layout == "container" && (id == nil || !id.Null)) || (tc.layout == "legacy" && id != nil) {
				t.Fatal("container absence/NULL changed", id)
			}
		})
	}
}

func TestClusterInterconnectsReadPreservesEvidenceAndBounds(t *testing.T) {
	options := instances.ClusterInterconnectsOptions{OracleVersion: "19.3.0.0.0"}
	ref, _ := reference.NewReference("unit", "instances", "interconnects", 1)
	request := instances.ClusterInterconnectsRequest{Options: options, Reference: ref}
	statement, _ := instances.ClusterInterconnectsStatement(options)
	columns := []string{"source", "INST_ID", "IP_ADDRESS", "IS_PUBLIC", "NAME", "CON_ID"}
	first := []session.Value{{Text: "  OS  "}, {Text: "2"}, {Text: " 192.0.2.20 "}, {Text: "NO"}, {Text: "eth2"}, {Text: "0"}}
	second := []session.Value{{Null: true}, {Text: "1"}, {Null: true}, {Text: "NEW"}, {Null: true}, {Text: "1"}}
	input := session.Result{Columns: columns, Rows: [][]session.Value{first, second, first}}
	calls := 0
	reader := readerFunc(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
		calls++
		if stmt.Text != statement || stmt.Reference != ref || len(stmt.Args) != 0 {
			t.Fatal("incorrect selected statement or reference")
		}
		return input, nil
	})
	got, err := instances.ReadClusterInterconnects(t.Context(), reader, request)
	if err != nil || calls != 1 || !got.Provenance.Complete || len(got.Rows) != 3 ||
		!reflect.DeepEqual(got.Rows[0], got.Rows[2]) || got.Rows[0].InstanceID.Text != "2" || got.Rows[0].Source.Text != "  OS  " ||
		got.Rows[0].IPAddress.Text != " 192.0.2.20 " || got.Rows[0].ContainerID.Text != "0" ||
		!got.Rows[1].IPAddress.Null || got.Rows[1].IsPublic.Text != "NEW" {
		t.Fatal(got, err)
	}
	request.Limits = rowset.Limits{MaxRows: 2}
	bounded, err := instances.ReadClusterInterconnects(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || bounded.State != instances.Failed || bounded.Rows != nil || bounded.Provenance.Complete {
		t.Fatal(bounded, err)
	}
	request.Limits = rowset.Limits{MaxBytes: 1}
	if bounded, err = instances.ReadClusterInterconnects(t.Context(), reader, request); !errors.Is(err, rowset.ErrCapacity) || bounded.Rows != nil {
		t.Fatal(bounded, err)
	}
	request.Limits = rowset.Limits{}
	terminal := errors.New("ORA-00942: deferred cursor error")
	failed, err := instances.ReadClusterInterconnects(t.Context(), readerFunc(func(context.Context, session.Statement) (session.Result, error) {
		return input, terminal
	}), request)
	if !errors.Is(err, terminal) || errors.Is(err, instances.ErrSourceUnavailable) || failed.State != instances.Failed || failed.Rows != nil {
		t.Fatal(failed, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failed, err = instances.ReadClusterInterconnects(ctx, readerFunc(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return input, nil
	}), request)
	if !errors.Is(err, context.Canceled) || failed.Rows != nil || failed.Provenance.Complete {
		t.Fatal(failed, err)
	}
	input.Rows = nil
	empty, err := instances.ReadClusterInterconnects(t.Context(), reader, request)
	if err != nil || empty.State != instances.NoRows || empty.Rows == nil || len(empty.Rows) != 0 || !empty.Provenance.Complete {
		t.Fatal(empty, err)
	}
}

func TestClusterInterconnectsNativeRowsAndJSON(t *testing.T) {
	options := instances.ClusterInterconnectsOptions{OracleVersion: "19.3.0.0.0"}
	statement, _ := instances.ClusterInterconnectsStatement(options)
	step := testsql.Step{
		SQL: statement, Columns: []string{"INST_ID", "NAME", "IP_ADDRESS", "IS_PUBLIC", "SOURCE", "CON_ID"},
		Values: [][]driver.Value{{int64(2), "eth2", "192.0.2.20", nil, "OCR", int64(0)}},
	}
	db, state := testsql.New(t, step, step)
	rows, err := instances.ClusterInterconnectsRows(t.Context(), db, options)
	if err != nil || state.Closed() != 0 || !rows.Next() {
		t.Fatal("native cursor not transferred", err)
	}
	var id, con int64
	var name, ip, source string
	var flag any
	if err := rows.Scan(&id, &name, &ip, &flag, &source, &con); err != nil || id != 2 || con != 0 || name != "eth2" || ip != "192.0.2.20" || source != "OCR" || flag != nil {
		t.Fatal("native content differs", id, con, name, ip, source, flag, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	value, err := instances.ClusterInterconnectsJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance instances.VisibilityProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err != nil || json.Unmarshal(value, &envelope) != nil ||
		!envelope.Provenance.Complete || len(envelope.Data.Columns) != len(step.Columns) ||
		!reflect.DeepEqual(envelope.Data.Rows, [][]any{{float64(2), "eth2", "192.0.2.20", nil, "OCR", float64(0)}}) || state.Closed() != 2 {
		t.Fatal(string(value), err, state.Closed())
	}
	openCause := errors.New("ORA-01031: insufficient privileges")
	db, _ = testsql.New(t, testsql.Step{QueryErr: openCause})
	value, err = instances.ClusterInterconnectsJSON(t.Context(), db, options, rowset.Limits{})
	if value != nil || !errors.Is(err, openCause) || !errors.Is(err, instances.ErrSourceInaccessible) {
		t.Fatal("open taxonomy", err)
	}
	step.NextErr = errors.New("ORA-00942: iteration failed")
	db, state = testsql.New(t, step)
	value, err = instances.ClusterInterconnectsJSON(t.Context(), db, options, rowset.Limits{})
	if value != nil || !errors.Is(err, step.NextErr) || errors.Is(err, instances.ErrSourceUnavailable) || state.Closed() != 1 {
		t.Fatal("late error taxonomy", err)
	}
	step.NextErr = nil
	step.Values = append(step.Values, []driver.Value{int64(0), "eth3", "2001:db8::3", "NO", "OCR", int64(0)})
	db, state = testsql.New(t, step)
	value, err = instances.ClusterInterconnectsJSON(t.Context(), db, options, rowset.Limits{})
	if value != nil || !errors.Is(err, instances.ErrInvalidEvidence) || state.Closed() != 1 || strings.Contains(err.Error(), "2001:db8") {
		t.Fatal("invalid final row was retained", err)
	}
}
