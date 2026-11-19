package goldengate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

const discoveryTestVersion = "19.32.0.0.0"
const discoveryTestOwner = "GG\"\u96ea schema"

func catalogFixture() session.Result {
	r := session.Result{Columns: []string{"OWNER", "OBJECT_NAME", "OBJECT_TYPE", "STATUS", "COLUMN_NAME", "DATA_TYPE", "DATA_TYPE_OWNER"}}
	layouts := []struct {
		name, kind string
		columns    [][2]string
	}{
		{"GG_HEARTBEAT", "TABLE", [][2]string{{"LOCAL_DATABASE", "VARCHAR2"}, {"REMOTE_DATABASE", "VARCHAR2"}, {"HEARTBEAT_TIMESTAMP", "TIMESTAMP(6)"}, {"INCOMING_EXTRACT", "VARCHAR2"}, {"INCOMING_ROUTING_PATH", "VARCHAR2"}, {"INCOMING_REPLICAT", "VARCHAR2"}, {"INCOMING_HEARTBEAT_TS", "TIMESTAMP(6)"}, {"OUTGOING_EXTRACT", "VARCHAR2"}, {"OUTGOING_ROUTING_PATH", "VARCHAR2"}, {"OUTGOING_REPLICAT", "VARCHAR2"}, {"OUTGOING_HEARTBEAT_TS", "TIMESTAMP(6)"}}},
		{"GG_LAG", "VIEW", [][2]string{{"LOCAL_DATABASE", "VARCHAR2"}, {"REMOTE_DATABASE", "VARCHAR2"}, {"CURRENT_LOCAL_TS", "TIMESTAMP(6)"}, {"INCOMING_HEARTBEAT_AGE", "NUMBER"}, {"INCOMING_PATH", "VARCHAR2"}, {"INCOMING_LAG", "NUMBER"}, {"OUTGOING_HEARTBEAT_AGE", "NUMBER"}, {"OUTGOING_PATH", "VARCHAR2"}, {"OUTGOING_LAG", "NUMBER"}}},
	}
	for _, layout := range layouts {
		for _, column := range layout.columns {
			r.Rows = append(r.Rows, []session.Value{{Text: discoveryTestOwner}, {Text: layout.name}, {Text: layout.kind}, {Text: "VALID"}, {Text: column[0]}, {Text: column[1]}, {Null: true}})
		}
	}
	return r
}

func verifiedSources(t *testing.T) (HeartbeatSource, HeartbeatSource) {
	t.Helper()
	got, err := DecodeHeartbeatCatalog(catalogFixture(), HeartbeatCatalogOptions{OracleVersion: discoveryTestVersion})
	if err != nil || len(got.Rows) != 2 || got.Provenance.Complete {
		t.Fatalf("catalog: %+v %v", got, err)
	}
	h, err := got.Rows[0].Source()
	if err != nil {
		t.Fatal(err)
	}
	l, err := got.Rows[1].Source()
	if err != nil {
		t.Fatal(err)
	}
	return h, l
}

func TestDiscoveryIdentityAdmissionFiltersAndMissingNames(t *testing.T) {
	opts := ProcessOptions{OracleVersion: discoveryTestVersion, Names: []string{"EX' OR 1=1 --"}}
	sql, args, err := ExtractsStatement(opts)
	if err != nil || !strings.Contains(sql, "FROM ALL_CAPTURE") || !strings.Contains(sql, "v.PURPOSE = 'GoldenGate Capture'") || strings.Contains(sql, opts.Names[0]) || !reflect.DeepEqual(args, []any{opts.Names[0]}) {
		t.Fatalf("unsafe or incorrect selection: %s %v %v", sql, args, err)
	}
	if !strings.Contains(sql, "NLSSORT(v.CLIENT_NAME, 'NLS_SORT=BINARY')") {
		t.Fatal(sql)
	}
	opts.Visibility = DatabaseDictionary
	sql, _, err = ReplicatsStatement(opts)
	if err != nil || !strings.Contains(sql, "DBA_GOLDENGATE_INBOUND") || !strings.Contains(sql, "v.REPLICAT_NAME") {
		t.Fatal(sql, err)
	}
	opts.Names = nil
	extract := session.Result{Columns: []string{"CAPTURE_NAME", "CLIENT_NAME", "CAPTURE_USER", "STATUS", "CLIENT_STATUS", "PURPOSE", "SOURCE_DATABASE"}, Rows: [][]session.Value{
		{{Text: "OGG$CAP_17"}, {Text: "EXT_A"}, {Text: "cap_user"}, {Text: "ENABLED"}, {Text: "DETACHED"}, {Text: "GoldenGate Capture"}, {Text: "source.db"}},
		{{Text: "OGG$CAP_18"}, {Null: true}, {Null: true}, {Text: "ABORTED"}, {Null: true}, {Text: "GoldenGate Capture"}, {Null: true}},
	}}
	got, err := ReadExtracts(t.Context(), &readerFixture{result: extract}, ProcessRequest{Options: opts, Reference: ref(t)})
	if err != nil || len(got.Rows) != 2 || got.Rows[0].ClientName.Text != "EXT_A" || got.Rows[0].CaptureName.Text != "OGG$CAP_17" || got.Rows[0].ClientStatus.Text != "DETACHED" || !got.Rows[1].ClientName.Null || !got.Provenance.Complete {
		t.Fatalf("identity loss %+v %v", got, err)
	}
	extract.Rows[1][5].Text = "XStream Out"
	if value, err := DecodeExtracts(extract, opts); err == nil || value.Rows != nil {
		t.Fatalf("wrong purpose admitted %+v %v", value, err)
	}
	inbound := session.Result{Columns: []string{"REPLICAT_NAME", "SERVER_NAME", "APPLY_USER", "STATUS"}, Rows: [][]session.Value{{{Text: "REP_A"}, {Text: "OGG$REP_17"}, {Text: "apply_user"}, {Text: "ATTACHED"}}, {{Null: true}, {Text: "OGG$REP_18"}, {Null: true}, {Text: "NEW_STATUS"}}}}
	repl, err := ReadReplicats(t.Context(), &readerFixture{result: inbound}, ProcessRequest{Options: opts, Reference: ref(t)})
	if err != nil || len(repl.Rows) != 2 || repl.Rows[0].Name.Text != "REP_A" || repl.Rows[0].ServerName.Text != "OGG$REP_17" || repl.Rows[1].Status.Text != "NEW_STATUS" || !repl.Rows[1].Name.Null || !repl.Provenance.Complete {
		t.Fatalf("inbound loss %+v %v", repl, err)
	}
	opts.Names = []string{"rep_a"}
	if got, err := DecodeReplicats(inbound, opts); err == nil || got.Rows != nil {
		t.Fatal("out-of-filter rows admitted", got, err)
	}
}

func TestHeartbeatCatalogRejectsUnverifiedLayoutsWithoutHidingOtherObjects(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "wrong-type", "user-type", "duplicate", "invalid-object", "synonym", "null-column"} {
		t.Run(mode, func(t *testing.T) {
			r := catalogFixture()
			switch mode {
			case "missing":
				r.Rows = r.Rows[1:]
			case "wrong-type":
				r.Rows[0][5].Text = "NUMBER"
			case "user-type":
				r.Rows[0][6] = session.Value{Text: "evil_type_owner"}
			case "duplicate":
				r.Rows = append(r.Rows, r.Rows[0])
			case "invalid-object":
				for _, row := range r.Rows {
					if row[1].Text == "GG_HEARTBEAT" {
						row[3].Text = "INVALID"
					}
				}
			case "synonym":
				for _, row := range r.Rows {
					if row[1].Text == "GG_HEARTBEAT" {
						row[2].Text = "SYNONYM"
					}
				}
			case "null-column":
				r.Rows[0][4] = session.Value{Null: true}
				r.Rows[0][5] = session.Value{Null: true}
			}
			got, err := ReadHeartbeatCatalog(t.Context(), &readerFixture{result: r}, HeartbeatCatalogRequest{Options: HeartbeatCatalogOptions{OracleVersion: discoveryTestVersion}, Reference: ref(t)})
			if err != nil || len(got.Rows) != 2 || !got.Rows[1].Supported || !got.Provenance.Complete {
				t.Fatalf("independent catalog lost %+v %v", got, err)
			}
			source, err := got.Rows[0].Source()
			if mode == "valid" {
				if err != nil || source.Owner() != discoveryTestOwner || source.Name() != "GG_HEARTBEAT" {
					t.Fatal(source, err)
				}
			} else if !errors.Is(err, ErrUnsupportedLayout) || source.Owner() != "" || got.Rows[0].Reason == "" {
				t.Fatalf("unverified source escaped %+v %v", got, err)
			}
		})
	}
	source := HeartbeatSource{}
	if _, _, err := HeartbeatsStatement(HeartbeatsOptions{Source: source}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}

func TestDiscoveryValidationAndUnsupportedSourcesDoNotExecute(t *testing.T) {
	for _, f := range []DiscoveryFilter{{Schema: "a\x00b"}, {Schema: strings.Repeat("x", 129)}, {Extracts: []string{""}}, {Replicats: []string{"R", "R"}}, {Extracts: []string{string([]byte{0xff})}}} {
		if ValidateDiscoveryFilter(f) == nil {
			t.Fatalf("invalid filter admitted %+v", f)
		}
	}
	if err := ValidateDiscoveryFilter(DiscoveryFilter{Schema: discoveryTestOwner, Extracts: []string{"Ex a'"}, Replicats: []string{"r\u96ea"}}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.1.0.2.0", "12.2.0.1.0", "18.16.0.0.0", "26.1.0.0.0", "27.0.0.0.0", "19", "19.1.x.0.0"} {
		db, state := testsql.New(t)
		o := ProcessOptions{OracleVersion: v}
		co := HeartbeatCatalogOptions{OracleVersion: v}
		if rows, e := ExtractsRows(t.Context(), db, o); rows != nil || !errors.Is(e, ErrUnsupportedVersion) {
			t.Fatal(v, rows, e)
		}
		if data, e := ReplicatsJSON(t.Context(), db, o, rowset.Limits{}); data != nil || !errors.Is(e, ErrUnsupportedVersion) {
			t.Fatal(v, string(data), e)
		}
		if _, e := ReadHeartbeatCatalog(t.Context(), nil, HeartbeatCatalogRequest{Options: co, Reference: ref(t)}); !errors.Is(e, ErrUnsupportedVersion) {
			t.Fatal(v, e)
		}
		if rows, e := HeartbeatCatalogRows(t.Context(), db, co); rows != nil || !errors.Is(e, ErrUnsupportedVersion) || len(state.Calls()) != 0 {
			t.Fatal(v, e, state.Calls())
		}
	}
	for _, v := range []string{"19.0.0.0.0", "21.3.0.0.0", "23.26.1.0.0"} {
		if _, _, err := ExtractsStatement(ProcessOptions{OracleVersion: v}); err != nil {
			t.Fatal(v, err)
		}
	}
}

func TestHeartbeatEvidenceExactSignedSecondsTimesAndSafeIdentifiers(t *testing.T) {
	h, l := verifiedSources(t)
	sql, args, err := HeartbeatsStatement(HeartbeatsOptions{Source: h, Extracts: []string{"EX' --"}})
	if err != nil || !strings.Contains(sql, `FROM "GG""`+"\u96ea"+` schema"."GG_HEARTBEAT" v`) || strings.Contains(sql, "EX' --") || !reflect.DeepEqual(args, []any{"EX' --"}) || !strings.Contains(sql, "SYS_EXTRACT_UTC(SYSTIMESTAMP)") || strings.Contains(sql, "YYYY") {
		t.Fatalf("unsafe identifier or timestamp %s %v %v", sql, args, err)
	}
	step := lagStep(t, l)
	result := sessionResult(step)
	facts, err := ReadHeartbeatLag(t.Context(), &readerFixture{result: result}, HeartbeatLagRequest{Options: HeartbeatLagOptions{Source: l}, Reference: ref(t)})
	if err != nil || len(facts.Rows) != 2 || facts.Rows[0].IncomingLag.Text != "-0.250000" || facts.Rows[0].OutgoingLag.Text != "9007199254740993.125" || facts.Rows[1].IncomingLag.Text != "1.25E-20" || !facts.Rows[1].OutgoingLag.Null || facts.Rows[0].IncomingAge.Text != "7200" || facts.Rows[0].DatabaseCurrentUTC.Text != "2026-11-19T01:00:00.123456" || !facts.Provenance.Complete {
		t.Fatalf("lag/freshness loss %+v %v", facts, err)
	}
	heartbeat := sessionResult(heartbeatStep(t, h))
	hb, err := DecodeHeartbeats(heartbeat, HeartbeatsOptions{Source: h})
	if err != nil || hb.Provenance.Complete || hb.Rows[0].Timestamp.Text != "2026-11-18T23:00:00.000001" || hb.Rows[0].DatabaseCurrentUTC.Text == hb.Rows[0].Timestamp.Text || hb.Rows[1].IncomingReplicat.Text != "REP_B" || !hb.Rows[1].OutgoingTimestamp.Null {
		t.Fatalf("heartbeat progress fabricated %+v %v", hb, err)
	}
	for _, bad := range []string{"NaN", "+Inf", "1,2", "1e", " 12", "1.2.3"} {
		result := sessionResult(step)
		result.Rows[1][5].Text = bad
		if got, e := DecodeHeartbeatLag(result, HeartbeatLagOptions{Source: l}); e == nil || got.Rows != nil {
			t.Fatal(bad, got, e)
		}
	}
	heartbeat.Rows[1][2].Text = "2026-02-30T00:00:00.000000"
	if got, e := DecodeHeartbeats(heartbeat, HeartbeatsOptions{Source: h}); e == nil || got.Rows != nil {
		t.Fatal(got, e)
	}
}

func sessionResult(s testsql.Step) session.Result {
	r := session.Result{Columns: append([]string(nil), s.Columns...)}
	for _, row := range s.Values {
		values := make([]session.Value, len(row))
		for i, v := range row {
			if v == nil {
				values[i].Null = true
			} else {
				values[i].Text = v.(string)
			}
		}
		r.Rows = append(r.Rows, values)
	}
	return r
}

func heartbeatStep(t *testing.T, s HeartbeatSource) testsql.Step {
	t.Helper()
	sql, _, err := HeartbeatsStatement(HeartbeatsOptions{Source: s})
	if err != nil {
		t.Fatal(err)
	}
	return testsql.Step{SQL: sql, Columns: []string{"LOCAL_DATABASE", "REMOTE_DATABASE", "HEARTBEAT_TIMESTAMP", "INCOMING_EXTRACT", "INCOMING_ROUTING_PATH", "INCOMING_REPLICAT", "INCOMING_HEARTBEAT_TS", "OUTGOING_EXTRACT", "OUTGOING_ROUTING_PATH", "OUTGOING_REPLICAT", "OUTGOING_HEARTBEAT_TS", "DB_CURRENT_UTC"}, Values: [][]driver.Value{
		{"local", "remote", "2026-11-18T23:00:00.000001", "EXT_A", "PUMP_1", "REP_A", "2026-11-18T23:00:00.250001", "EXT_BACK", "PUMP_2", "REP_BACK", "2026-11-18T22:59:59.999999", "2026-11-19T01:00:00.123456"},
		{"local2", "remote2", nil, "EXT_B", nil, "REP_B", nil, nil, nil, nil, nil, "2026-11-19T01:00:00.123456"},
	}}
}
func lagStep(t *testing.T, s HeartbeatSource) testsql.Step {
	t.Helper()
	sql, _, err := HeartbeatLagStatement(HeartbeatLagOptions{Source: s})
	if err != nil {
		t.Fatal(err)
	}
	return testsql.Step{SQL: sql, Columns: []string{"LOCAL_DATABASE", "REMOTE_DATABASE", "CURRENT_LOCAL_TS", "INCOMING_HEARTBEAT_AGE", "INCOMING_PATH", "INCOMING_LAG", "OUTGOING_HEARTBEAT_AGE", "OUTGOING_PATH", "OUTGOING_LAG", "DB_CURRENT_UTC"}, Values: [][]driver.Value{
		{"local", "remote", "2026-11-19T01:00:00.123456", "7200", "EXT_A/PUMP_1/REP_A", "-0.250000", "7200.5", "EXT_BACK/PUMP_2/REP_BACK", "9007199254740993.125", "2026-11-19T01:00:00.123456"},
		{"local2", "remote2", nil, "0", "EXT_B/REP_B", "1.25E-20", nil, nil, nil, "2026-11-19T01:00:00.123456"},
	}}
}

type discoveryCase struct {
	name         string
	step         testsql.Step
	invalidIndex int
	rows         func(context.Context, rowset.Queryer) (*sql.Rows, error)
	json         func(context.Context, rowset.Queryer, rowset.Limits) (json.RawMessage, error)
	read         func(context.Context, Reader) (bool, int, error)
}

func discoveryCases(t *testing.T) []discoveryCase {
	t.Helper()
	h, l := verifiedSources(t)
	po := ProcessOptions{OracleVersion: discoveryTestVersion}
	co := HeartbeatCatalogOptions{OracleVersion: discoveryTestVersion}
	ho := HeartbeatsOptions{Source: h}
	lo := HeartbeatLagOptions{Source: l}
	ref := ref(t)
	exsql, _, _ := ExtractsStatement(po)
	resql, _, _ := ReplicatsStatement(po)
	casql, _, _ := HeartbeatCatalogStatement(co)
	catalog := catalogFixture()
	castep := testsql.Step{SQL: casql, Columns: catalog.Columns}
	for _, row := range catalog.Rows {
		values := make([]driver.Value, len(row))
		for i, v := range row {
			if !v.Null {
				values[i] = v.Text
			}
		}
		castep.Values = append(castep.Values, values)
	}
	return []discoveryCase{
		{name: "extracts", step: testsql.Step{SQL: exsql, Columns: []string{"CAPTURE_NAME", "CLIENT_NAME", "CAPTURE_USER", "STATUS", "CLIENT_STATUS", "PURPOSE", "SOURCE_DATABASE"}, Values: [][]driver.Value{{"CAP_A", "EXT_A", "u", "ENABLED", "DETACHED", "GoldenGate Capture", "remote"}, {"CAP_B", nil, nil, "ABORTED", nil, "GoldenGate Capture", nil}}}, invalidIndex: 0,
			rows: func(c context.Context, q rowset.Queryer) (*sql.Rows, error) { return ExtractsRows(c, q, po) }, json: func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
				return ExtractsJSON(c, q, po, l)
			}, read: func(c context.Context, r Reader) (bool, int, error) {
				v, e := ReadExtracts(c, r, ProcessRequest{Options: po, Reference: ref})
				return v.Provenance.Complete, len(v.Rows), e
			}},
		{name: "replicats", step: testsql.Step{SQL: resql, Columns: []string{"REPLICAT_NAME", "SERVER_NAME", "APPLY_USER", "STATUS"}, Values: [][]driver.Value{{"REP_A", "SERV_A", "u", "ATTACHED"}, {nil, "SERV_B", nil, "DETACHED"}}}, invalidIndex: 1,
			rows: func(c context.Context, q rowset.Queryer) (*sql.Rows, error) { return ReplicatsRows(c, q, po) }, json: func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
				return ReplicatsJSON(c, q, po, l)
			}, read: func(c context.Context, r Reader) (bool, int, error) {
				v, e := ReadReplicats(c, r, ProcessRequest{Options: po, Reference: ref})
				return v.Provenance.Complete, len(v.Rows), e
			}},
		{name: "catalog", step: castep, invalidIndex: 0,
			rows: func(c context.Context, q rowset.Queryer) (*sql.Rows, error) { return HeartbeatCatalogRows(c, q, co) }, json: func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
				return HeartbeatCatalogJSON(c, q, co, l)
			}, read: func(c context.Context, r Reader) (bool, int, error) {
				v, e := ReadHeartbeatCatalog(c, r, HeartbeatCatalogRequest{Options: co, Reference: ref})
				return v.Provenance.Complete, len(v.Rows), e
			}},
		{name: "heartbeats", step: heartbeatStep(t, h), invalidIndex: 11,
			rows: func(c context.Context, q rowset.Queryer) (*sql.Rows, error) { return HeartbeatsRows(c, q, ho) }, json: func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
				return HeartbeatsJSON(c, q, ho, l)
			}, read: func(c context.Context, r Reader) (bool, int, error) {
				v, e := ReadHeartbeats(c, r, HeartbeatsRequest{Options: ho, Reference: ref})
				return v.Provenance.Complete, len(v.Rows), e
			}},
		{name: "lag", step: lagStep(t, l), invalidIndex: 9,
			rows: func(c context.Context, q rowset.Queryer) (*sql.Rows, error) { return HeartbeatLagRows(c, q, lo) }, json: func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
				return HeartbeatLagJSON(c, q, lo, l)
			}, read: func(c context.Context, r Reader) (bool, int, error) {
				v, e := ReadHeartbeatLag(c, r, HeartbeatLagRequest{Options: lo, Reference: ref})
				return v.Provenance.Complete, len(v.Rows), e
			}},
	}
}

type discoverySpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *discoverySpy) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(ctx, q, args...)
	s.last = r
	return r, e
}

func TestDiscoveryNativeRowsAreCallerCursorAndUsable(t *testing.T) {
	for _, c := range discoveryCases(t) {
		t.Run(c.name, func(t *testing.T) {
			db, state := testsql.New(t, c.step)
			spy := &discoverySpy{db: db}
			rows, err := c.rows(t.Context(), spy)
			if err != nil || rows == nil || rows != spy.last || state.Closed() != 0 {
				t.Fatalf("native cursor ownership %v %v", rows, err)
			}
			if !rows.Next() {
				t.Fatal(rows.Err())
			}
			values := make([]any, len(c.step.Columns))
			dest := make([]any, len(values))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			if values[0] != c.step.Values[0][0] {
				t.Fatalf("cursor unusable %v", values)
			}
			if err := rows.Close(); err != nil || state.Closed() != 1 {
				t.Fatal(err, state.Closed())
			}
		})
	}
}

func TestDiscoveryJSONCompleteValuesAndFinalEnvelopeBoundary(t *testing.T) {
	for _, c := range discoveryCases(t) {
		t.Run(c.name, func(t *testing.T) {
			db, state := testsql.New(t, c.step)
			payload, err := c.json(t.Context(), db, rowset.Limits{MaxRows: 100, MaxBytes: 1 << 20})
			var got struct {
				Provenance Provenance
				Data       struct {
					Columns []rowset.Column
					Rows    [][]any
				}
			}
			if err != nil || json.Unmarshal(payload, &got) != nil || !got.Provenance.Complete || len(got.Data.Rows) != len(c.step.Values) || state.Closed() != 1 {
				t.Fatalf("incomplete JSON %s %v", payload, err)
			}
			names := make([]string, len(got.Data.Columns))
			for i, column := range got.Data.Columns {
				names[i] = column.Name
			}
			if !reflect.DeepEqual(names, c.step.Columns) {
				t.Fatalf("columns changed %v", names)
			}
			// The fixture values are independently specified raw strings/NULLs, not
			// re-decoded or regenerated from the production selector/serializer.
			for i, row := range c.step.Values {
				for j, v := range row {
					if got.Data.Rows[i][j] != v {
						t.Fatalf("row%d column%s: got%v want%v", i, c.step.Columns[j], got.Data.Rows[i][j], v)
					}
				}
			}
			if c.name == "lag" || c.name == "heartbeats" {
				if got.Provenance.ObjectOwner != discoveryTestOwner {
					t.Fatal(got.Provenance)
				}
			}
			for _, delta := range []int{0, -1} {
				db, _ := testsql.New(t, c.step)
				data, e := c.json(t.Context(), db, rowset.Limits{MaxRows: 100, MaxBytes: len(payload) + delta})
				if delta == 0 {
					if e != nil || !reflect.DeepEqual(data, payload) {
						t.Fatal("exact byte boundary", e)
					}
				} else if data != nil || !errors.Is(e, rowset.ErrCapacity) {
					t.Fatal("overflow published", string(data), e)
				}
			}
		})
	}
}

func TestDiscoveryJSONNeverPublishesFailedPrefixes(t *testing.T) {
	for _, c := range discoveryCases(t) {
		t.Run(c.name, func(t *testing.T) {
			for _, failure := range []string{"query", "late", "close", "cancel", "invalid", "scan", "rows", "bytes"} {
				t.Run(failure, func(t *testing.T) {
					step := c.step
					step.Values = make([][]driver.Value, len(c.step.Values))
					for i, row := range c.step.Values {
						step.Values[i] = append([]driver.Value(nil), row...)
					}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					cause := io.ErrUnexpectedEOF
					limits := rowset.Limits{MaxRows: 100, MaxBytes: 1 << 20}
					switch failure {
					case "query":
						step.QueryErr = cause
					case "late":
						step.NextErr = cause
					case "close":
						step.CloseErr = cause
					case "cancel":
						step.OnNext = func(i int) {
							if i == len(step.Values) {
								cancel()
							}
						}
					case "invalid":
						step.Values[len(step.Values)-1][c.invalidIndex] = nil
					case "scan":
						step.Values[len(step.Values)-1][0] = []byte{0xff}
					case "rows":
						limits.MaxRows = len(step.Values) - 1
					case "bytes":
						limits.MaxBytes = 1
					}
					db, _ := testsql.New(t, step)
					data, err := c.json(ctx, db, limits)
					if err == nil || data != nil {
						t.Fatalf("%s published prefix %s %v", failure, data, err)
					}
					if (failure == "query" || failure == "late" || failure == "close") && !errors.Is(err, cause) {
						t.Fatalf("lost original cause %v", err)
					}
					if failure == "cancel" && !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestDiscoveryBufferedCompleteEmptyAndAtomicFailures(t *testing.T) {
	for _, c := range discoveryCases(t) {
		t.Run(c.name, func(t *testing.T) {
			for _, mode := range []string{"valid", "empty", "late", "cancel", "malformed"} {
				t.Run(mode, func(t *testing.T) {
					r := &readerFixture{result: sessionResult(c.step)}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					switch mode {
					case "empty":
						r.result.Rows = nil
					case "late":
						r.err = io.ErrUnexpectedEOF
					case "cancel":
						r.after = cancel
					case "malformed":
						r.result.Rows[len(r.result.Rows)-1][c.invalidIndex] = session.Value{Null: true}
					}
					complete, count, err := c.read(ctx, r)
					if mode == "valid" {
						expected := len(c.step.Values)
						if c.name == "catalog" {
							expected = 2
						}
						if err != nil || !complete || count != expected || r.calls != 1 {
							t.Fatal(complete, count, err, r.calls)
						}
					} else if mode == "empty" {
						if err != nil || !complete || count != 0 {
							t.Fatal(complete, count, err)
						}
					} else if err == nil || complete || count != 0 {
						t.Fatalf("failed prefix %v %d %v", complete, count, err)
					}
					if mode == "late" && !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatal(err)
					}
					if mode == "cancel" && !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestNativeDiscoveryFiltersStayBound(t *testing.T) {
	o := ProcessOptions{OracleVersion: discoveryTestVersion, Names: []string{"REP' --"}}
	sql, _, err := ReplicatsStatement(o)
	if err != nil {
		t.Fatal(err)
	}
	db, state := testsql.New(t, testsql.Step{SQL: sql, Columns: []string{"REPLICAT_NAME", "SERVER_NAME", "APPLY_USER", "STATUS"}, Values: [][]driver.Value{{"REP' --", "s", nil, "DETACHED"}}})
	data, err := ReplicatsJSON(t.Context(), db, o, rowset.Limits{})
	calls := state.Calls()
	if err != nil || len(data) == 0 || len(calls) != 1 || len(calls[0].Args) != 1 || calls[0].Args[0].Value != "REP' --" {
		t.Fatal(string(data), err, calls)
	}
}

func TestDiscoveryReviewedProfilesHaveIdenticalProjectionsAndLegacyIsNoSQL(t *testing.T) {
	var baseline []string
	for _, version := range []string{"19.32.0.0.0", "21.3.0.0.0", "23.26.1.0.0"} {
		catalog, err := DecodeHeartbeatCatalog(catalogFixture(), HeartbeatCatalogOptions{OracleVersion: version})
		if err != nil {
			t.Fatal(err)
		}
		h, err := catalog.Rows[0].Source()
		if err != nil {
			t.Fatal(err)
		}
		l, err := catalog.Rows[1].Source()
		if err != nil {
			t.Fatal(err)
		}
		selections := []string{}
		for _, visibility := range []DictionaryVisibility{AccessibleDictionary, DatabaseDictionary} {
			o := ProcessOptions{OracleVersion: version, Visibility: visibility}
			sql, _, err := ExtractsStatement(o)
			if err != nil {
				t.Fatal(err)
			}
			selections = append(selections, sql)
			sql, _, err = ReplicatsStatement(o)
			if err != nil {
				t.Fatal(err)
			}
			selections = append(selections, sql)
		}
		sql, _, err := HeartbeatCatalogStatement(HeartbeatCatalogOptions{OracleVersion: version})
		if err != nil {
			t.Fatal(err)
		}
		selections = append(selections, sql)
		sql, _, err = HeartbeatsStatement(HeartbeatsOptions{Source: h})
		if err != nil {
			t.Fatal(err)
		}
		selections = append(selections, sql)
		sql, _, err = HeartbeatLagStatement(HeartbeatLagOptions{Source: l})
		if err != nil {
			t.Fatal(err)
		}
		selections = append(selections, sql)
		if baseline == nil {
			baseline = selections
		} else if !reflect.DeepEqual(baseline, selections) {
			t.Fatal("profile projection drift", version)
		}
		if got, err := DecodeHeartbeats(sessionResult(heartbeatStep(t, h)), HeartbeatsOptions{Source: h}); err != nil || got.Provenance.OracleVersion != version {
			t.Fatal(version, got, err)
		}
		if got, err := DecodeHeartbeatLag(sessionResult(lagStep(t, l)), HeartbeatLagOptions{Source: l}); err != nil || got.Provenance.OracleVersion != version {
			t.Fatal(version, got, err)
		}
	}
	for _, version := range []string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.1.0.2.0", "12.2.0.1.0", "18.16.0.0.0", "26.1.0.0.0"} {
		db, state := testsql.New(t)
		h := HeartbeatSource{owner: "admitted", name: "GG_HEARTBEAT", version: version}
		l := HeartbeatSource{owner: "admitted", name: "GG_LAG", version: version}
		if rows, e := HeartbeatsRows(t.Context(), db, HeartbeatsOptions{Source: h}); rows != nil || !errors.Is(e, ErrUnsupportedVersion) {
			t.Fatal(version, rows, e)
		}
		if data, e := HeartbeatLagJSON(t.Context(), db, HeartbeatLagOptions{Source: l}, rowset.Limits{}); data != nil || !errors.Is(e, ErrUnsupportedVersion) || len(state.Calls()) != 0 {
			t.Fatal(version, string(data), e, state.Calls())
		}
	}
}

func TestCatalogTypedAndJSONBothRejectContradictoryObjectStatus(t *testing.T) {
	r := catalogFixture()
	r.Rows[1][3].Text = "INVALID"
	o := HeartbeatCatalogOptions{OracleVersion: discoveryTestVersion}
	if got, e := DecodeHeartbeatCatalog(r, o); e == nil || got.Rows != nil {
		t.Fatal(got, e)
	}
	sql, _, err := HeartbeatCatalogStatement(o)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{SQL: sql, Columns: r.Columns}
	for _, row := range r.Rows {
		values := make([]driver.Value, len(row))
		for i, v := range row {
			if !v.Null {
				values[i] = v.Text
			}
		}
		step.Values = append(step.Values, values)
	}
	db, _ := testsql.New(t, step)
	if data, e := HeartbeatCatalogJSON(t.Context(), db, o, rowset.Limits{}); e == nil || data != nil {
		t.Fatal(string(data), e)
	}
}
