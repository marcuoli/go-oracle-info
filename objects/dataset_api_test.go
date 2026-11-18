package objects

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

func TestDatasetInvalidObjectsCompleteAPI(t *testing.T) {
	options := InvalidObjectsOptions{OracleMajor: 19, Owners: []string{"CaseOwner"}}
	ref, _ := reference.NewReference("unit", "objects", "InvalidObjects", 1)
	cols := []string{"OWNER", "OBJECT_TYPE", "OBJECT_NAME", "STATUS"}
	values := []driver.Value{"CaseOwner", "TABLE", "T雪", "INVALID"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "CaseOwner"}, {Text: "TABLE"}, {Text: "T雪"}, {Text: "INVALID"}}}}
	decoded, err := DecodeInvalidObjects(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], InvalidObjectsRow{Owner: session.Value{Text: "CaseOwner"}, Type: session.Value{Text: "TABLE"}, Name: session.Value{Text: "T雪"}, Status: session.Value{Text: "INVALID"}}) {
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
	facts, err := ReadInvalidObjects(t.Context(), reader, InvalidObjectsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeInvalidObjects(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadInvalidObjects(ctx, reader, InvalidObjectsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadInvalidObjects(t.Context(), reader, InvalidObjectsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadInvalidObjects(t.Context(), reader, InvalidObjectsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := InvalidObjectsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := InvalidObjectsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner", "TABLE", "T雪", "INVALID"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadInvalidObjects(t.Context(), reader, InvalidObjectsRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := InvalidObjectsJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := InvalidObjectsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := InvalidObjectsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := InvalidObjectsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetObjectInventoryCompleteAPI(t *testing.T) {
	options := ObjectInventoryOptions{OracleMajor: 19, Owners: []string{"CaseOwner"}}
	ref, _ := reference.NewReference("unit", "objects", "ObjectInventory", 1)
	cols := []string{"OWNER", "OBJECT_TYPE", "STATUS", "OBJECT_COUNT"}
	values := []driver.Value{"CaseOwner", "TABLE", nil, "9007199254740993"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "CaseOwner"}, {Text: "TABLE"}, {Null: true}, {Text: "9007199254740993"}}}}
	decoded, err := DecodeObjectInventory(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ObjectInventoryRow{Owner: session.Value{Text: "CaseOwner"}, Type: session.Value{Text: "TABLE"}, Status: session.Value{Null: true}, Count: session.Value{Text: "9007199254740993"}}) {
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
	facts, err := ReadObjectInventory(t.Context(), reader, ObjectInventoryRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeObjectInventory(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadObjectInventory(ctx, reader, ObjectInventoryRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadObjectInventory(t.Context(), reader, ObjectInventoryRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadObjectInventory(t.Context(), reader, ObjectInventoryRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ObjectInventoryRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ObjectInventoryJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner", "TABLE", nil, "9007199254740993"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadObjectInventory(t.Context(), reader, ObjectInventoryRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ObjectInventoryJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ObjectInventoryJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ObjectInventoryJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := ObjectInventoryJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetSchemaCompleteAPI(t *testing.T) {
	options := SchemaOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "objects", "Schema", 1)
	cols := []string{"USERNAME"}
	values := []driver.Value{"CaseOwner"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "CaseOwner"}}}}
	decoded, err := DecodeSchema(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], SchemaRow{Name: session.Value{Text: "CaseOwner"}}) {
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
	facts, err := ReadSchema(t.Context(), reader, SchemaRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeSchema(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadSchema(ctx, reader, SchemaRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadSchema(t.Context(), reader, SchemaRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSchema(t.Context(), reader, SchemaRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := SchemaRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := SchemaJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSchema(t.Context(), reader, SchemaRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := SchemaJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := SchemaJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := SchemaJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := SchemaJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetObjectTotalsCompleteAPI(t *testing.T) {
	options := ObjectTotalsOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "objects", "ObjectTotals", 1)
	cols := []string{"OBJECT_TYPE", "TOTAL", "VALID_COUNT", "INVALID_COUNT", "OTHER_COUNT"}
	values := []driver.Value{"TABLE", "5", "3", "1", "1"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "TABLE"}, {Text: "5"}, {Text: "3"}, {Text: "1"}, {Text: "1"}}}}
	decoded, err := DecodeObjectTotals(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ObjectTotalsRow{Type: session.Value{Text: "TABLE"}, Total: session.Value{Text: "5"}, Valid: session.Value{Text: "3"}, Invalid: session.Value{Text: "1"}, Other: session.Value{Text: "1"}}) {
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
	facts, err := ReadObjectTotals(t.Context(), reader, ObjectTotalsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeObjectTotals(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadObjectTotals(ctx, reader, ObjectTotalsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadObjectTotals(t.Context(), reader, ObjectTotalsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadObjectTotals(t.Context(), reader, ObjectTotalsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ObjectTotalsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ObjectTotalsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"TABLE", "5", "3", "1", "1"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadObjectTotals(t.Context(), reader, ObjectTotalsRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ObjectTotalsJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ObjectTotalsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ObjectTotalsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := ObjectTotalsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetTablesCompleteAPI(t *testing.T) {
	options := TablesOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "objects", "Tables", 1)
	cols := []string{"TABLE_NAME"}
	values := []driver.Value{"T雪"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "T雪"}}}}
	decoded, err := DecodeTables(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], TablesRow{Name: session.Value{Text: "T雪"}}) {
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
	facts, err := ReadTables(t.Context(), reader, TablesRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeTables(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTables(ctx, reader, TablesRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTables(t.Context(), reader, TablesRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTables(t.Context(), reader, TablesRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := TablesRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := TablesJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"T雪"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTables(t.Context(), reader, TablesRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := TablesJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := TablesJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := TablesJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := TablesJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetTableCompleteAPI(t *testing.T) {
	options := TableOptions{OracleMajor: 19, Owner: "CaseOwner", Table: "T雪"}
	ref, _ := reference.NewReference("unit", "objects", "Table", 1)
	cols := []string{"TABLE_NAME"}
	values := []driver.Value{"T雪"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "T雪"}}}}
	decoded, err := DecodeTable(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], TableRow{Name: session.Value{Text: "T雪"}}) {
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
	facts, err := ReadTable(t.Context(), reader, TableRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeTable(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTable(ctx, reader, TableRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTable(t.Context(), reader, TableRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTable(t.Context(), reader, TableRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := TableRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := TableJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"T雪"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTable(t.Context(), reader, TableRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := TableJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := TableJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := TableJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := TableJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetTableCountCompleteAPI(t *testing.T) {
	options := TableCountOptions{OracleMajor: 19, Owner: "CaseOwner", Table: "T雪"}
	ref, _ := reference.NewReference("unit", "objects", "TableCount", 1)
	cols := []string{"ROW_COUNT"}
	values := []driver.Value{"9007199254740993"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "9007199254740993"}}}}
	decoded, err := DecodeTableCount(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], TableCountRow{Count: session.Value{Text: "9007199254740993"}}) {
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
	facts, err := ReadTableCount(t.Context(), reader, TableCountRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeTableCount(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTableCount(ctx, reader, TableCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTableCount(t.Context(), reader, TableCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTableCount(t.Context(), reader, TableCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := TableCountRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := TableCountJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"9007199254740993"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTableCount(t.Context(), reader, TableCountRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := TableCountJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := TableCountJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := TableCountJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := TableCountJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetTablesStatisticsCompleteAPI(t *testing.T) {
	options := TablesStatisticsOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "objects", "TablesStatistics", 1)
	cols := []string{"TABLE_NAME", "ROW_COUNT", "LAST_ANALYZED"}
	values := []driver.Value{"T雪", "12.5", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "T雪"}, {Text: "12.5"}, {Null: true}}}}
	decoded, err := DecodeTablesStatistics(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], TablesStatisticsRow{Name: session.Value{Text: "T雪"}, EstimatedRows: session.Value{Text: "12.5"}, LastAnalyzed: session.Value{Null: true}}) {
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
	facts, err := ReadTablesStatistics(t.Context(), reader, TablesStatisticsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeTablesStatistics(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTablesStatistics(ctx, reader, TablesStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTablesStatistics(t.Context(), reader, TablesStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTablesStatistics(t.Context(), reader, TablesStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := TablesStatisticsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := TablesStatisticsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"T雪", "12.5", nil}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTablesStatistics(t.Context(), reader, TablesStatisticsRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := TablesStatisticsJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := TablesStatisticsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := TablesStatisticsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := TablesStatisticsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetTableStatisticsCompleteAPI(t *testing.T) {
	options := TableStatisticsOptions{OracleMajor: 19, Owner: "CaseOwner", Table: "T雪"}
	ref, _ := reference.NewReference("unit", "objects", "TableStatistics", 1)
	cols := []string{"TABLE_NAME", "ROW_COUNT", "LAST_ANALYZED"}
	values := []driver.Value{"T雪", "12.5", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "T雪"}, {Text: "12.5"}, {Null: true}}}}
	decoded, err := DecodeTableStatistics(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], TableStatisticsRow{Name: session.Value{Text: "T雪"}, EstimatedRows: session.Value{Text: "12.5"}, LastAnalyzed: session.Value{Null: true}}) {
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
	facts, err := ReadTableStatistics(t.Context(), reader, TableStatisticsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeTableStatistics(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadTableStatistics(ctx, reader, TableStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadTableStatistics(t.Context(), reader, TableStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTableStatistics(t.Context(), reader, TableStatisticsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := TableStatisticsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := TableStatisticsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"T雪", "12.5", nil}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadTableStatistics(t.Context(), reader, TableStatisticsRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 11, 18, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := TableStatisticsJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := TableStatisticsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := TableStatisticsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := TableStatisticsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
