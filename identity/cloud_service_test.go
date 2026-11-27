package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const wantCloudServiceSQL = "SELECT SYS_CONTEXT('USERENV','CLOUD_SERVICE') AS CLOUD_SERVICE FROM DUAL"

func TestCloudServiceTypedRead(t *testing.T) {
	ref, err := reference.NewReference("unit", "identity", "CloudService", 1)
	if err != nil {
		t.Fatal(err)
	}
	columns := []string{"CLOUD_SERVICE"}
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		options := CloudServiceOptions{OracleMajor: major}
		statement, args, err := CloudServiceStatement(options)
		if major != 19 && major != 21 && major != 23 {
			if statement != "" || !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("invalid selection for major %d: %q, %v", major, statement, err)
			}
			_, err = ReadCloudService(t.Context(), datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
				t.Fatalf("major %d reached the reader", major)
				return session.Result{}, nil
			}), CloudServiceRequest{Options: options, Reference: ref})
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatal(err)
			}
			continue
		}
		if err != nil || statement != wantCloudServiceSQL || len(args) != 0 {
			t.Fatal(statement, args, err)
		}
		for _, cell := range []session.Value{{Text: "OLTP"}, {Text: " future "}, {Null: true}, {Text: ""}} {
			native := session.Result{Columns: columns, Rows: [][]session.Value{{cell}}}
			called := 0
			reader := datasetTestReader(func(_ context.Context, s session.Statement) (session.Result, error) {
				called++
				if s.Text != wantCloudServiceSQL || s.Reference != ref || len(s.Args) != 0 {
					t.Fatal(s)
				}
				return native, nil
			})
			facts, err := ReadCloudService(t.Context(), reader, CloudServiceRequest{Options: options, Reference: ref})
			if err != nil || called != 1 || !facts.Provenance.Complete || facts.Provenance.OracleMajor != major ||
				len(facts.Rows) != 1 || facts.Rows[0].Value != cell {
				t.Fatal(facts, err, called)
			}
		}
	}
	for _, rows := range [][][]session.Value{nil, {{{Text: "first"}}, {{Text: "second"}}}, {{{Text: "contradiction", Null: true}}}} {
		facts, err := DecodeCloudService(session.Result{Columns: columns, Rows: rows}, CloudServiceOptions{OracleMajor: 19})
		if !errors.Is(err, ErrInvalidEvidence) || len(facts.Rows) != 0 {
			t.Fatal(facts, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	got, err := ReadCloudService(ctx, datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return session.Result{Columns: columns, Rows: [][]session.Value{{{Text: "OLTP"}}}}, nil
	}), CloudServiceRequest{Options: CloudServiceOptions{OracleMajor: 19}, Reference: ref})
	if !errors.Is(err, context.Canceled) || len(got.Rows) != 0 {
		t.Fatal(got, err)
	}
}

func TestCloudServiceRowsAndJSONPreserveNullableSource(t *testing.T) {
	options := CloudServiceOptions{OracleMajor: 19}
	source := testsql.Step{SQL: wantCloudServiceSQL, Columns: []string{"CLOUD_SERVICE"}, Values: [][]driver.Value{{nil}}}
	db, state := testsql.New(t, source)
	spy := &datasetSpy{db: db}
	rows, err := CloudServiceRows(t.Context(), spy, options)
	if err != nil || rows != spy.last || state.Closed() != 0 || !rows.Next() {
		t.Fatal(rows, err)
	}
	var field sql.NullString
	if err := rows.Scan(&field); err != nil || field.Valid {
		t.Fatal(field, err)
	}
	if err := rows.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, source)
	payload, err := CloudServiceJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance DatasetProvenance
		Data       struct{ Rows [][]any }
	}
	if err != nil || state.Closed() != 1 || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		!reflect.DeepEqual(envelope.Data.Rows, [][]any{{nil}}) {
		t.Fatal(string(payload), err)
	}
	source.Values = [][]driver.Value{{nil}, {"late duplicate"}}
	db, state = testsql.New(t, source)
	payload, err = CloudServiceJSON(t.Context(), db, options, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || len(payload) != 0 || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
}
