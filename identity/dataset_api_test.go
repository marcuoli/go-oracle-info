package identity

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
	"reflect"
	"testing"
)

type datasetTestReader func(context.Context, session.Statement) (session.Result, error)

func (f datasetTestReader) Query(c context.Context, s session.Statement) (session.Result, error) {
	return f(c, s)
}

type datasetSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *datasetSpy) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(c, q, a...)
	s.last = r
	return r, e
}

func TestDatasetDatabaseIdentityCompleteAPI(t *testing.T) {
	options := DatabaseIdentityOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "DatabaseIdentity", 1)
	cols := []string{"DATABASE_NAME", "DATABASE_ROLE", "PLATFORM_NAME", "LOG_MODE"}
	values := []driver.Value{"GG19", "PRIMARY", "Linux x86 64-bit", "ARCHIVELOG"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "GG19"}, {Text: "PRIMARY"}, {Text: "Linux x86 64-bit"}, {Text: "ARCHIVELOG"}}}}
	decoded, err := DecodeDatabaseIdentity(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], DatabaseIdentityRow{Name: session.Value{Text: "GG19"}, Role: session.Value{Text: "PRIMARY"}, Platform: session.Value{Text: "Linux x86 64-bit"}, LogMode: session.Value{Text: "ARCHIVELOG"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadDatabaseIdentity(t.Context(), reader, DatabaseIdentityRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeDatabaseIdentity(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadDatabaseIdentity(ctx, reader, DatabaseIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadDatabaseIdentity(t.Context(), reader, DatabaseIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadDatabaseIdentity(t.Context(), reader, DatabaseIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := DatabaseIdentityRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := DatabaseIdentityJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"GG19", "PRIMARY", "Linux x86 64-bit", "ARCHIVELOG"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadDatabaseIdentity(t.Context(), reader, DatabaseIdentityRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := DatabaseIdentityJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := DatabaseIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := DatabaseIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := DatabaseIdentityJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetInstanceIdentityCompleteAPI(t *testing.T) {
	options := InstanceIdentityOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "InstanceIdentity", 1)
	cols := []string{"INSTANCE_ID", "INSTANCE_NAME", "HOST_NAME", "VERSION", "STARTUP_LOCAL"}
	values := []driver.Value{"1", "GG19", "host雪", "19.0.0.0.0", "2026-11-18T12:34:56"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "1"}, {Text: "GG19"}, {Text: "host雪"}, {Text: "19.0.0.0.0"}, {Text: "2026-11-18T12:34:56"}}}}
	decoded, err := DecodeInstanceIdentity(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], InstanceIdentityRow{ID: session.Value{Text: "1"}, Name: session.Value{Text: "GG19"}, Host: session.Value{Text: "host雪"}, Version: session.Value{Text: "19.0.0.0.0"}, StartupLocal: session.Value{Text: "2026-11-18T12:34:56"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadInstanceIdentity(t.Context(), reader, InstanceIdentityRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeInstanceIdentity(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadInstanceIdentity(ctx, reader, InstanceIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadInstanceIdentity(t.Context(), reader, InstanceIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadInstanceIdentity(t.Context(), reader, InstanceIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := InstanceIdentityRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := InstanceIdentityJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"1", "GG19", "host雪", "19.0.0.0.0", "2026-11-18T12:34:56"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadInstanceIdentity(t.Context(), reader, InstanceIdentityRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := InstanceIdentityJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := InstanceIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := InstanceIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := InstanceIdentityJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetExecutionIdentityCompleteAPI(t *testing.T) {
	options := ExecutionIdentityOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "ExecutionIdentity", 1)
	cols := []string{"DATABASE_NAME", "DBID", "ORACLE_VERSION", "INSTANCE_ID", "CONTAINER_ID", "SERVICE_NAME", "STARTUP_KEY"}
	values := []driver.Value{"GG19", "9007199254740993", "19.32.0.0.0", "1", "3", "gg19", "2026-11-18 12:34:56"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "GG19"}, {Text: "9007199254740993"}, {Text: "19.32.0.0.0"}, {Text: "1"}, {Text: "3"}, {Text: "gg19"}, {Text: "2026-11-18 12:34:56"}}}}
	decoded, err := DecodeExecutionIdentity(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ExecutionIdentityRow{Database: session.Value{Text: "GG19"}, DBID: session.Value{Text: "9007199254740993"}, Version: session.Value{Text: "19.32.0.0.0"}, InstanceID: session.Value{Text: "1"}, ContainerID: session.Value{Text: "3"}, Service: session.Value{Text: "gg19"}, StartupKey: session.Value{Text: "2026-11-18 12:34:56"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadExecutionIdentity(t.Context(), reader, ExecutionIdentityRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeExecutionIdentity(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadExecutionIdentity(ctx, reader, ExecutionIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadExecutionIdentity(t.Context(), reader, ExecutionIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadExecutionIdentity(t.Context(), reader, ExecutionIdentityRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ExecutionIdentityRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ExecutionIdentityJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"GG19", "9007199254740993", "19.32.0.0.0", "1", "3", "gg19", "2026-11-18 12:34:56"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadExecutionIdentity(t.Context(), reader, ExecutionIdentityRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ExecutionIdentityJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ExecutionIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ExecutionIdentityJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := ExecutionIdentityJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetNameSourceCompleteAPI(t *testing.T) {
	options := NameSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "NameSource", 1)
	cols := []string{"NAME"}
	values := []driver.Value{"  Mixed雪  "}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "  Mixed雪  "}}}}
	decoded, err := DecodeNameSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], NameSourceRow{Value: session.Value{Text: "  Mixed雪  "}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadNameSource(t.Context(), reader, NameSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeNameSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadNameSource(ctx, reader, NameSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadNameSource(t.Context(), reader, NameSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadNameSource(t.Context(), reader, NameSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := NameSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := NameSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"  Mixed雪  "}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadNameSource(t.Context(), reader, NameSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := NameSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := NameSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := NameSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := NameSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetRoleSourceCompleteAPI(t *testing.T) {
	options := RoleSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "RoleSource", 1)
	cols := []string{"DATABASE_ROLE"}
	values := []driver.Value{"PRIMARY"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "PRIMARY"}}}}
	decoded, err := DecodeRoleSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], RoleSourceRow{Value: session.Value{Text: "PRIMARY"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadRoleSource(t.Context(), reader, RoleSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeRoleSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadRoleSource(ctx, reader, RoleSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadRoleSource(t.Context(), reader, RoleSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadRoleSource(t.Context(), reader, RoleSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := RoleSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := RoleSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"PRIMARY"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadRoleSource(t.Context(), reader, RoleSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := RoleSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := RoleSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := RoleSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := RoleSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetVersionSourceCompleteAPI(t *testing.T) {
	options := VersionSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "VersionSource", 1)
	cols := []string{"VERSION"}
	values := []driver.Value{"19.32.0.0.0"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "19.32.0.0.0"}}}}
	decoded, err := DecodeVersionSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], VersionSourceRow{Value: session.Value{Text: "19.32.0.0.0"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadVersionSource(t.Context(), reader, VersionSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeVersionSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadVersionSource(ctx, reader, VersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadVersionSource(t.Context(), reader, VersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVersionSource(t.Context(), reader, VersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := VersionSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := VersionSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"19.32.0.0.0"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVersionSource(t.Context(), reader, VersionSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := VersionSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := VersionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := VersionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := VersionSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetEditionSourceCompleteAPI(t *testing.T) {
	options := EditionSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "EditionSource", 1)
	cols := []string{"EDITION"}
	values := []driver.Value{"EE"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "EE"}}}}
	decoded, err := DecodeEditionSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], EditionSourceRow{Value: session.Value{Text: "EE"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadEditionSource(t.Context(), reader, EditionSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeEditionSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadEditionSource(ctx, reader, EditionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadEditionSource(t.Context(), reader, EditionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadEditionSource(t.Context(), reader, EditionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := EditionSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := EditionSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"EE"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadEditionSource(t.Context(), reader, EditionSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := EditionSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := EditionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := EditionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := EditionSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetBannersSourceCompleteAPI(t *testing.T) {
	options := BannersSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "BannersSource", 1)
	cols := []string{"BANNER", "BANNER_FULL"}
	values := []driver.Value{"Oracle Database 19c\n", "Oracle Database 19c Enterprise Edition\nVersion 19.32.0.0.0"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "Oracle Database 19c\n"}, {Text: "Oracle Database 19c Enterprise Edition\nVersion 19.32.0.0.0"}}}}
	decoded, err := DecodeBannersSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], BannersSourceRow{Banner: session.Value{Text: "Oracle Database 19c\n"}, Full: session.Value{Text: "Oracle Database 19c Enterprise Edition\nVersion 19.32.0.0.0"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadBannersSource(t.Context(), reader, BannersSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeBannersSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadBannersSource(ctx, reader, BannersSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadBannersSource(t.Context(), reader, BannersSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadBannersSource(t.Context(), reader, BannersSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := BannersSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := BannersSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"Oracle Database 19c\n", "Oracle Database 19c Enterprise Edition\nVersion 19.32.0.0.0"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadBannersSource(t.Context(), reader, BannersSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := BannersSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := BannersSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := BannersSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := BannersSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetSnapshotCompleteAPI(t *testing.T) {
	options := SnapshotOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "Snapshot", 1)
	cols := []string{"DB_UNIQUE_NAME", "DATABASE_NAME", "VERSION", "HOST_NAME", "PLATFORM_NAME", "OPEN_MODE", "DATABASE_ROLE", "LOG_MODE", "INSTANCE_STATUS"}
	values := []driver.Value{"GG19", "GG19", "19.0.0.0.0", "host", "Linux x86 64-bit", "READ WRITE", "PRIMARY", "ARCHIVELOG", "OPEN"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "GG19"}, {Text: "GG19"}, {Text: "19.0.0.0.0"}, {Text: "host"}, {Text: "Linux x86 64-bit"}, {Text: "READ WRITE"}, {Text: "PRIMARY"}, {Text: "ARCHIVELOG"}, {Text: "OPEN"}}}}
	decoded, err := DecodeSnapshot(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], SnapshotRow{UniqueName: session.Value{Text: "GG19"}, Name: session.Value{Text: "GG19"}, Version: session.Value{Text: "19.0.0.0.0"}, Host: session.Value{Text: "host"}, Platform: session.Value{Text: "Linux x86 64-bit"}, OpenMode: session.Value{Text: "READ WRITE"}, Role: session.Value{Text: "PRIMARY"}, LogMode: session.Value{Text: "ARCHIVELOG"}, InstanceStatus: session.Value{Text: "OPEN"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadSnapshot(t.Context(), reader, SnapshotRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeSnapshot(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadSnapshot(ctx, reader, SnapshotRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadSnapshot(t.Context(), reader, SnapshotRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSnapshot(t.Context(), reader, SnapshotRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := SnapshotRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := SnapshotJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"GG19", "GG19", "19.0.0.0.0", "host", "Linux x86 64-bit", "READ WRITE", "PRIMARY", "ARCHIVELOG", "OPEN"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSnapshot(t.Context(), reader, SnapshotRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := SnapshotJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := SnapshotJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := SnapshotJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := SnapshotJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetContainerSourceCompleteAPI(t *testing.T) {
	options := ContainerSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "ContainerSource", 1)
	cols := []string{"CONTAINER", "CDB"}
	values := []driver.Value{"GG19PDB", "YES"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "GG19PDB"}, {Text: "YES"}}}}
	decoded, err := DecodeContainerSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ContainerSourceRow{Name: session.Value{Text: "GG19PDB"}, CDB: session.Value{Text: "YES"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadContainerSource(t.Context(), reader, ContainerSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeContainerSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadContainerSource(ctx, reader, ContainerSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadContainerSource(t.Context(), reader, ContainerSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadContainerSource(t.Context(), reader, ContainerSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ContainerSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ContainerSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"GG19PDB", "YES"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadContainerSource(t.Context(), reader, ContainerSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ContainerSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ContainerSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ContainerSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := ContainerSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetFullVersionSourceCompleteAPI(t *testing.T) {
	options := FullVersionSourceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "FullVersionSource", 1)
	cols := []string{"VERSION_FULL"}
	values := []driver.Value{"19.32.0.0.0"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "19.32.0.0.0"}}}}
	decoded, err := DecodeFullVersionSource(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], FullVersionSourceRow{Value: session.Value{Text: "19.32.0.0.0"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadFullVersionSource(t.Context(), reader, FullVersionSourceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeFullVersionSource(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadFullVersionSource(ctx, reader, FullVersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadFullVersionSource(t.Context(), reader, FullVersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadFullVersionSource(t.Context(), reader, FullVersionSourceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := FullVersionSourceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := FullVersionSourceJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"19.32.0.0.0"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadFullVersionSource(t.Context(), reader, FullVersionSourceRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := FullVersionSourceJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := FullVersionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := FullVersionSourceJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := FullVersionSourceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetPDBOpenModeCompleteAPI(t *testing.T) {
	options := PDBOpenModeOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "PDBOpenMode", 1)
	cols := []string{"OPEN_MODE"}
	values := []driver.Value{"READ WRITE"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "READ WRITE"}}}}
	decoded, err := DecodePDBOpenMode(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], PDBOpenModeRow{OpenMode: session.Value{Text: "READ WRITE"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadPDBOpenMode(t.Context(), reader, PDBOpenModeRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodePDBOpenMode(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadPDBOpenMode(ctx, reader, PDBOpenModeRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadPDBOpenMode(t.Context(), reader, PDBOpenModeRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadPDBOpenMode(t.Context(), reader, PDBOpenModeRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := PDBOpenModeRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := PDBOpenModeJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"READ WRITE"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadPDBOpenMode(t.Context(), reader, PDBOpenModeRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := PDBOpenModeJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := PDBOpenModeJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := PDBOpenModeJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := PDBOpenModeJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetConnectedArchitectureCompleteAPI(t *testing.T) {
	options := ConnectedArchitectureOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "ConnectedArchitecture", 1)
	cols := []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}
	values := []driver.Value{"YES", "3", "GG19PDB"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "YES"}, {Text: "3"}, {Text: "GG19PDB"}}}}
	decoded, err := DecodeConnectedArchitecture(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ConnectedArchitectureRow{CDB: session.Value{Text: "YES"}, ContainerID: session.Value{Text: "3"}, Name: session.Value{Text: "GG19PDB"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadConnectedArchitecture(t.Context(), reader, ConnectedArchitectureRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeConnectedArchitecture(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadConnectedArchitecture(ctx, reader, ConnectedArchitectureRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadConnectedArchitecture(t.Context(), reader, ConnectedArchitectureRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadConnectedArchitecture(t.Context(), reader, ConnectedArchitectureRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ConnectedArchitectureRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ConnectedArchitectureJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"YES", "3", "GG19PDB"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadConnectedArchitecture(t.Context(), reader, ConnectedArchitectureRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ConnectedArchitectureJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ConnectedArchitectureJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ConnectedArchitectureJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := ConnectedArchitectureJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetVersionBootstrapCompleteAPI(t *testing.T) {
	options := VersionBootstrapOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "identity", "VersionBootstrap", 1)
	cols := []string{"VERSION"}
	values := []driver.Value{"19.0.0.0.0"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "19.0.0.0.0"}}}}
	decoded, err := DecodeVersionBootstrap(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], VersionBootstrapRow{Value: session.Value{Text: "19.0.0.0.0"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadVersionBootstrap(t.Context(), reader, VersionBootstrapRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeVersionBootstrap(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadVersionBootstrap(ctx, reader, VersionBootstrapRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadVersionBootstrap(t.Context(), reader, VersionBootstrapRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVersionBootstrap(t.Context(), reader, VersionBootstrapRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := VersionBootstrapRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := VersionBootstrapJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"19.0.0.0.0"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVersionBootstrap(t.Context(), reader, VersionBootstrapRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := VersionBootstrapJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := VersionBootstrapJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := VersionBootstrapJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
	if !errors.Is(err, rowset.ErrCapacity) || short != nil {
		t.Fatal("final envelope overflow", string(short), err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := VersionBootstrapJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
