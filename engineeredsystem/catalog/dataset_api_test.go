package catalog

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

func TestDatasetMetadataCompleteAPI(t *testing.T) {
	options := MetadataOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "engineeredsystem_catalog", "Metadata", 1)
	cols := []string{"SOURCE_NAME", "COLUMN_NAME", "DATA_TYPE"}
	values := []driver.Value{"GV_$CELL", "INST_ID", "NUMBER"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "GV_$CELL"}, {Text: "INST_ID"}, {Text: "NUMBER"}}}}
	decoded, err := DecodeMetadata(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], MetadataColumnRow{Source: session.Value{Text: "GV_$CELL"}, Column: session.Value{Text: "INST_ID"}, DataType: session.Value{Text: "NUMBER"}}) {
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
	facts, err := ReadMetadata(t.Context(), reader, MetadataRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeMetadata(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadMetadata(ctx, reader, MetadataRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadMetadata(t.Context(), reader, MetadataRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadMetadata(t.Context(), reader, MetadataRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := MetadataRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := MetadataJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"GV_$CELL", "INST_ID", "NUMBER"}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadMetadata(t.Context(), reader, MetadataRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 10, 11, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := MetadataJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := MetadataJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := MetadataJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := MetadataJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetConfigurationCompleteAPI(t *testing.T) {
	options := ConfigurationOptions{OracleMajor: 19, Schema: Schema{"V_$CELL_CONFIG": {"CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "CLOB"}}}
	ref, _ := reference.NewReference("unit", "engineeredsystem_catalog", "Configuration", 1)
	cols := []string{"INST_ID", "CELL_HASHVAL", "CELL_NAME", "SOURCE_TYPE", "XML_PAYLOAD", "XML_LENGTH"}
	values := []driver.Value{int64(1), nil, "cell雪", "CELL", "<x>😀</x>", int64(9)}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "1"}, {Null: true}, {Text: "cell雪"}, {Text: "CELL"}, {Text: "<x>😀</x>"}, {Text: "9"}}}}
	decoded, err := DecodeConfiguration(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ConfigurationRow{InstanceID: session.Value{Text: "1"}, CellHash: session.Value{Null: true}, CellName: session.Value{Text: "cell雪"}, SourceType: session.Value{Text: "CELL"}, Payload: session.Value{Text: "<x>😀</x>"}, CodeUnits: session.Value{Text: "9"}}) {
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
	facts, err := ReadConfiguration(t.Context(), reader, ConfigurationRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeConfiguration(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadConfiguration(ctx, reader, ConfigurationRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadConfiguration(t.Context(), reader, ConfigurationRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadConfiguration(t.Context(), reader, ConfigurationRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ConfigurationRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ConfigurationJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{float64(1), nil, "cell雪", "CELL", "<x>😀</x>", float64(9)}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadConfiguration(t.Context(), reader, ConfigurationRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 10, 11, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := ConfigurationJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := ConfigurationJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := ConfigurationJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := ConfigurationJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetVisibleCellCountCompleteAPI(t *testing.T) {
	options := VisibleCellCountOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "engineeredsystem_catalog", "VisibleCellCount", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{int64(0)}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "0"}}}}
	decoded, err := DecodeVisibleCellCount(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], VisibleCellCountRow{Count: session.Value{Text: "0"}}) {
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
	facts, err := ReadVisibleCellCount(t.Context(), reader, VisibleCellCountRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], make([]session.Value, len(cols)+1)}
	got, err := DecodeVisibleCellCount(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadVisibleCellCount(ctx, reader, VisibleCellCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadVisibleCellCount(t.Context(), reader, VisibleCellCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVisibleCellCount(t.Context(), reader, VisibleCellCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := VisibleCellCountRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := VisibleCellCountJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{float64(0)}}) {
		t.Fatal(string(payload), document, err)
	}

	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadVisibleCellCount(t.Context(), reader, VisibleCellCountRequest{Options: options})
	if !errors.Is(err, ErrInvalidRequest) || calls != 0 {
		t.Fatal("invalid reference executed SQL", err, calls)
	}
	for _, major := range []int{9, 10, 11, 26} {
		options.OracleMajor = major
		db, state = testsql.New(t)
		b, err := VisibleCellCountJSON(t.Context(), db, options, rowset.Limits{})
		if !errors.Is(err, ErrUnsupportedVersion) || b != nil || len(state.Calls()) != 0 {
			t.Fatal("unsupported JSON issued SQL", major, err)
		}
	}
	options.OracleMajor = 19
	exact := len(payload)
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	same, err := VisibleCellCountJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact})
	if err != nil || !reflect.DeepEqual(same, payload) {
		t.Fatal("exact final envelope budget", string(same), err)
	}
	db, _ = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	short, err := VisibleCellCountJSON(t.Context(), db, options, rowset.Limits{MaxBytes: exact - 1})
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
			b, err := VisibleCellCountJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
