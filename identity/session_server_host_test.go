package identity

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

var serverHostColumns = []string{"SERVER_HOST", "INSTANCE_NUMBER", "INSTANCE_NAME"}

func TestSessionServerHostSelectsDocumentedReleaseFamilies(t *testing.T) {
	const wantSQL = "SELECT SYS_CONTEXT('USERENV','SERVER_HOST',4000) AS SERVER_HOST, " +
		"SYS_CONTEXT('USERENV','INSTANCE') AS INSTANCE_NUMBER, " +
		"SYS_CONTEXT('USERENV','INSTANCE_NAME') AS INSTANCE_NAME FROM DUAL"
	for _, version := range []string{"10.2.0.1.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.1.0.1.0",
		"12.1.0.2.0", "12.2.0.1.0", "18.4.0.0.0", "19.3.0.0.0", "19.33.0.1.0", "21.3.0.0.0",
		"23.2.0.0.0", "23.9.0.25.07", "23.26.4.0.0"} {
		options := SessionServerHostOptions{OracleVersion: version}
		statement, args, err := SessionServerHostStatement(options)
		if err != nil || statement != wantSQL || len(args) != 0 {
			t.Errorf("%s: statement=%q args=%v err=%v", version, statement, args, err)
		}
	}
	for _, version := range []string{"", "9.2.0.8.0", "10", "10.1.0.5.0", "10.3.0.0.0", "12.3.0.0.0", "20.1", "23ai", "26.1"} {
		options := SessionServerHostOptions{OracleVersion: version}
		statement, args, err := SessionServerHostStatement(options)
		if statement != "" || args != nil || !errors.Is(err, ErrUnsupportedVersion) {
			t.Errorf("%s: statement=%q args=%v err=%v", version, statement, args, err)
		}
		reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
			t.Fatal("unsupported release issued SQL")
			return session.Result{}, nil
		})
		if _, err := ReadSessionServerHost(t.Context(), reader, SessionServerHostRequest{Options: options, Reference: coreReference(t)}); !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatal(err)
		}
	}
}

func TestSessionServerHostReadPreservesScopeAndAbsence(t *testing.T) {
	options := SessionServerHostOptions{OracleVersion: "19.33.0.1.0"}
	request := SessionServerHostRequest{Options: options, Reference: coreReference(t)}
	columnOrder := []string{"INSTANCE_NAME", "SERVER_HOST", "INSTANCE_NUMBER"}
	provided := session.Result{
		Columns: columnOrder,
		Rows:    [][]session.Value{{{Null: true}, {Text: "Mixed.Host.Example."}, {Text: "001"}}},
	}
	reader := datasetTestReader(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
		if ctx != t.Context() || stmt.Reference != request.Reference || stmt.Text != sessionServerHostSQL || len(stmt.Args) != 0 {
			t.Fatal(stmt)
		}
		return provided, nil
	})
	want := SessionServerHostRow{ServerHost: provided.Rows[0][1], InstanceNumber: provided.Rows[0][2], InstanceName: provided.Rows[0][0]}
	facts, err := ReadSessionServerHost(t.Context(), reader, request)
	if err != nil || !facts.Provenance.Complete || facts.Provenance.Source != "SYS_CONTEXT(USERENV.SERVER_HOST)" ||
		facts.Provenance.Scope != "executing_session_server_host" || facts.Provenance.OracleMajor != 19 ||
		!reflect.DeepEqual(facts.Rows, []SessionServerHostRow{want}) {
		t.Fatal(facts, err)
	}
	empty, err := DecodeSessionServerHost(session.Result{Columns: columnOrder}, options)
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 || empty.Provenance.Complete {
		t.Fatal(empty, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	failed := errors.New("private transport error")
	late := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return provided, failed
	})
	if facts, err := ReadSessionServerHost(ctx, late, request); facts.Rows != nil || facts.Provenance.Complete ||
		!errors.Is(err, context.Canceled) || !errors.Is(err, failed) {
		t.Fatal(facts, err)
	}
}

func TestSessionServerHostNativeRowsAndAtomicJSON(t *testing.T) {
	options := SessionServerHostOptions{OracleVersion: "23.26.4.0.0"}
	row := []driver.Value{nil, "001", "MixedCase"}
	step := testsql.Step{SQL: sessionServerHostSQL, Columns: serverHostColumns, Values: [][]driver.Value{row}}
	db, state := testsql.New(t, step)
	spy := &datasetSpy{db: db}
	cursor, err := SessionServerHostRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}

	db, state = testsql.New(t, step)
	data, err := SessionServerHostJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance DatasetProvenance
		Data       rowset.Table
	}
	if err != nil || json.Unmarshal(data, &envelope) != nil || !envelope.Provenance.Complete ||
		!reflect.DeepEqual(envelope.Data.Rows, [][]any{{nil, "001", "MixedCase"}}) || state.Closed() != 1 {
		t.Fatal(string(data), err, state.Closed())
	}

	for _, tc := range []struct {
		name    string
		columns []string
		rows    [][]driver.Value
		want    error
	}{
		{"wrong schema", []string{"CLIENT_HOST", "INSTANCE_NUMBER", "INSTANCE_NAME"}, [][]driver.Value{row}, rowset.ErrInvalidEvidence},
		{"fractional instance number", serverHostColumns, [][]driver.Value{{"host", "1.5", "I"}}, ErrInvalidEvidence},
		{"unexpected second row", serverHostColumns, [][]driver.Value{row, row}, ErrInvalidEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, testsql.Step{SQL: sessionServerHostSQL, Columns: tc.columns, Values: tc.rows})
			data, err := SessionServerHostJSON(t.Context(), db, options, rowset.Limits{})
			if data != nil || !errors.Is(err, tc.want) || state.Closed() != 1 {
				t.Fatal(string(data), err, state.Closed())
			}
		})
	}
}
