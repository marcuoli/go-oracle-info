package memory

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"testing"
)

type sourceReader func(context.Context, session.Statement) (session.Result, error)

func (f sourceReader) Query(c context.Context, s session.Statement) (session.Result, error) {
	return f(c, s)
}

type sourceSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *sourceSpy) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(c, q, a...)
	s.last = r
	return r, e
}
func TestMemorySourceOwnedNativeAndJSON(t *testing.T) {
	ref, _ := reference.NewReference("unit", "memory", "source", 1)
	options := Options{Scope: NonCDB, OracleMajor: 19}
	cols := []string{"VALUE", "SOURCE_CONTAINER_ID", "CURRENT_CONTAINER_ID", "PARAMETER_NAME", "SOURCE_ROWS", "INSTANCE_ID", "TARGET_CONTAINER_ID"}
	values := []driver.Value{"3.125", "0", "0", "sga_max_size", "1", "1", "0"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "3.125"}, {Text: "0"}, {Text: "0"}, {Text: "sga_max_size"}, {Text: "1"}, {Text: "1"}, {Text: "0"}}}}
	reader := sourceReader(func(context.Context, session.Statement) (session.Result, error) { return input, nil })
	got, err := Read(t.Context(), reader, SourceRequest{Parameter: SGA, Options: options, Reference: ref})
	if err != nil || got.State != Available || got.ValueGiB != "3.125" || got.Origin != InstanceParameter {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = Read(ctx, reader, SourceRequest{Parameter: SGA, Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.State == Available {
		t.Fatal(got, err)
	}
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &sourceSpy{db: db}
	rows, err := Rows(t.Context(), spy, SGA, options)
	if err != nil || rows != spy.last || !rows.Next() || state.Closed() != 0 {
		t.Fatal(rows, err)
	}
	rows.Close()
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := JSON(t.Context(), db, SGA, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var doc struct {
		Provenance DatasetProvenance
		Data       struct{ Rows [][]any }
	}
	if err = json.Unmarshal(payload, &doc); err != nil || !doc.Provenance.Complete || len(doc.Data.Rows) != 1 || doc.Data.Rows[0][0] != "3.125" {
		t.Fatal(string(payload), err)
	}
	terminal := errors.New("terminal")
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: terminal})
	payload, err = JSON(t.Context(), db, SGA, options, rowset.Limits{})
	if !errors.Is(err, terminal) || payload != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	options.Scope = ConnectedPDB
	options.TargetContainerID = 3
	input.Rows[0] = []session.Value{{Text: "1.5"}, {Text: "3"}, {Text: "3"}, {Text: "sga_target"}, {Text: "1"}, {Text: "1"}, {Text: "3"}}
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, nil }
	got, err = ReadPDB(t.Context(), reader, SourceRequest{Parameter: SGA, Options: options, Reference: ref})
	if err != nil || got.ValueGiB != "1.5" || got.Origin != ContainerParameter {
		t.Fatal(got, err)
	}
}
func TestRACSourceJSONAndReadAlias(t *testing.T) {
	ref, _ := reference.NewReference("unit", "memory", "rac", 1)
	options := Options{Scope: NonCDB, OracleMajor: 19}
	cols := []string{"PARAMETER_NAME", "PARAMETER_VALUE", "INSTANCE_ID", "SOURCE_CONTAINER_ID", "TARGET_CONTAINER_ID"}
	values := [][]driver.Value{{"sga_max_size", "3221225472", "1", "0", "0"}, {"pga_aggregate_target", "0", "1", "0", "0"}}
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: values})
	b, err := RACJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 || !json.Valid(b) {
		t.Fatal(string(b), err)
	}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "sga_max_size"}, {Text: "3221225472"}, {Text: "1"}, {Text: "0"}, {Text: "0"}}, {{Text: "pga_aggregate_target"}, {Text: "0"}, {Text: "1"}, {Text: "0"}, {Text: "0"}}}}
	reader := sourceReader(func(context.Context, session.Statement) (session.Result, error) { return input, nil })
	got, err := ReadRAC(t.Context(), reader, options, ref)
	if err != nil || got.SGA.State != Available || got.SGA.ValueGiB != "3" || got.PGA.State != Available || got.PGA.ValueGiB != "0" {
		t.Fatal(got, err)
	}
}
