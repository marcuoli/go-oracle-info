package database

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

func TestDatasetConfigurationCompleteAPI(t *testing.T) {
	options := ConfigurationOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "Configuration", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"TRUE"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "TRUE"}}}}
	decoded, err := DecodeConfiguration(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ConfigurationRow{Value: session.Value{Text: "TRUE"}}) {
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
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"TRUE"}}) {
		t.Fatal(string(payload), document, err)
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

func TestDatasetEndianCompleteAPI(t *testing.T) {
	options := EndianOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "Endian", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"Little"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "Little"}}}}
	decoded, err := DecodeEndian(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], EndianRow{Value: session.Value{Text: "Little"}}) {
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
	facts, err := ReadEndian(t.Context(), reader, EndianRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeEndian(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadEndian(ctx, reader, EndianRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadEndian(t.Context(), reader, EndianRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadEndian(t.Context(), reader, EndianRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := EndianRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := EndianJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"Little"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := EndianJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetCharacterSetCompleteAPI(t *testing.T) {
	options := CharacterSetOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "CharacterSet", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"AL32UTF8"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "AL32UTF8"}}}}
	decoded, err := DecodeCharacterSet(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], CharacterSetRow{Value: session.Value{Text: "AL32UTF8"}}) {
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
	facts, err := ReadCharacterSet(t.Context(), reader, CharacterSetRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeCharacterSet(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadCharacterSet(ctx, reader, CharacterSetRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadCharacterSet(t.Context(), reader, CharacterSetRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadCharacterSet(t.Context(), reader, CharacterSetRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := CharacterSetRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := CharacterSetJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"AL32UTF8"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := CharacterSetJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetCPUCountCompleteAPI(t *testing.T) {
	options := CPUCountOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "CPUCount", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"8"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "8"}}}}
	decoded, err := DecodeCPUCount(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], CPUCountRow{Value: session.Value{Text: "8"}}) {
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
	facts, err := ReadCPUCount(t.Context(), reader, CPUCountRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeCPUCount(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadCPUCount(ctx, reader, CPUCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadCPUCount(t.Context(), reader, CPUCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadCPUCount(t.Context(), reader, CPUCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := CPUCountRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := CPUCountJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"8"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := CPUCountJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetProductsCompleteAPI(t *testing.T) {
	options := ProductsOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "Products", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"XDB"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "XDB"}}}}
	decoded, err := DecodeProducts(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ProductsRow{ComponentID: session.Value{Text: "XDB"}}) {
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
	facts, err := ReadProducts(t.Context(), reader, ProductsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeProducts(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadProducts(ctx, reader, ProductsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadProducts(t.Context(), reader, ProductsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadProducts(t.Context(), reader, ProductsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := ProductsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := ProductsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"XDB"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := ProductsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetDatafileSizeCompleteAPI(t *testing.T) {
	options := DatafileSizeOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "DatafileSize", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Null: true}}}}
	decoded, err := DecodeDatafileSize(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], DatafileSizeRow{GiB: session.Value{Null: true}}) {
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
	facts, err := ReadDatafileSize(t.Context(), reader, DatafileSizeRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeDatafileSize(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadDatafileSize(ctx, reader, DatafileSizeRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadDatafileSize(t.Context(), reader, DatafileSizeRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadDatafileSize(t.Context(), reader, DatafileSizeRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := DatafileSizeRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := DatafileSizeJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{nil}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := DatafileSizeJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetStorageCompleteAPI(t *testing.T) {
	options := StorageOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "Storage", 1)
	cols := []string{"FILE_NAME", "FILE_KIND"}
	values := []driver.Value{"+DATA/db/file", "DATAFILE"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "+DATA/db/file"}, {Text: "DATAFILE"}}}}
	decoded, err := DecodeStorage(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], StorageRow{FileName: session.Value{Text: "+DATA/db/file"}, Kind: session.Value{Text: "DATAFILE"}}) {
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
	facts, err := ReadStorage(t.Context(), reader, StorageRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeStorage(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadStorage(ctx, reader, StorageRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadStorage(t.Context(), reader, StorageRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadStorage(t.Context(), reader, StorageRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := StorageRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := StorageJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"+DATA/db/file", "DATAFILE"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := StorageJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetEncryptionCountsCompleteAPI(t *testing.T) {
	options := EncryptionCountsOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "EncryptionCounts", 1)
	cols := []string{"ENCRYPTED_TABLESPACES", "ENCRYPTED_COLUMNS"}
	values := []driver.Value{"2", "3"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "2"}, {Text: "3"}}}}
	decoded, err := DecodeEncryptionCounts(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], EncryptionCountsRow{Tablespaces: session.Value{Text: "2"}, Columns: session.Value{Text: "3"}}) {
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
	facts, err := ReadEncryptionCounts(t.Context(), reader, EncryptionCountsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeEncryptionCounts(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadEncryptionCounts(ctx, reader, EncryptionCountsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadEncryptionCounts(t.Context(), reader, EncryptionCountsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadEncryptionCounts(t.Context(), reader, EncryptionCountsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := EncryptionCountsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := EncryptionCountsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"2", "3"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := EncryptionCountsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetSGAAllocatedCompleteAPI(t *testing.T) {
	options := SGAAllocatedOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "SGAAllocated", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"3.125"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "3.125"}}}}
	decoded, err := DecodeSGAAllocated(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], SGAAllocatedRow{GiB: session.Value{Text: "3.125"}}) {
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
	facts, err := ReadSGAAllocated(t.Context(), reader, SGAAllocatedRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeSGAAllocated(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadSGAAllocated(ctx, reader, SGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadSGAAllocated(t.Context(), reader, SGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSGAAllocated(t.Context(), reader, SGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := SGAAllocatedRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := SGAAllocatedJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"3.125"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := SGAAllocatedJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetPGAAllocatedCompleteAPI(t *testing.T) {
	options := PGAAllocatedOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "PGAAllocated", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Null: true}}}}
	decoded, err := DecodePGAAllocated(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], PGAAllocatedRow{GiB: session.Value{Null: true}}) {
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
	facts, err := ReadPGAAllocated(t.Context(), reader, PGAAllocatedRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodePGAAllocated(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadPGAAllocated(ctx, reader, PGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadPGAAllocated(t.Context(), reader, PGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadPGAAllocated(t.Context(), reader, PGAAllocatedRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := PGAAllocatedRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := PGAAllocatedJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{nil}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := PGAAllocatedJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetPDBCountCompleteAPI(t *testing.T) {
	options := PDBCountOptions{OracleMajor: 19, Root: true}
	ref, _ := reference.NewReference("unit", "database", "PDBCount", 1)
	cols := []string{"VALUE"}
	values := []driver.Value{"5"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "5"}}}}
	decoded, err := DecodePDBCount(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], PDBCountRow{Count: session.Value{Text: "5"}}) {
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
	facts, err := ReadPDBCount(t.Context(), reader, PDBCountRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodePDBCount(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadPDBCount(ctx, reader, PDBCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadPDBCount(t.Context(), reader, PDBCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadPDBCount(t.Context(), reader, PDBCountRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := PDBCountRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := PDBCountJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"5"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := PDBCountJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetSessionParametersCompleteAPI(t *testing.T) {
	options := SessionParametersOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "SessionParameters", 1)
	cols := []string{"NAME", "VALUE"}
	values := []driver.Value{"nls_date_format", "YYYY-MM-DD"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "nls_date_format"}, {Text: "YYYY-MM-DD"}}}}
	decoded, err := DecodeSessionParameters(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], SessionParametersRow{Name: session.Value{Text: "nls_date_format"}, Value: session.Value{Text: "YYYY-MM-DD"}}) {
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
	facts, err := ReadSessionParameters(t.Context(), reader, SessionParametersRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeSessionParameters(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadSessionParameters(ctx, reader, SessionParametersRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadSessionParameters(t.Context(), reader, SessionParametersRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSessionParameters(t.Context(), reader, SessionParametersRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := SessionParametersRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := SessionParametersJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"nls_date_format", "YYYY-MM-DD"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := SessionParametersJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetSessionParametersWithDefaultsCompleteAPI(t *testing.T) {
	options := SessionParametersWithDefaultsOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "SessionParametersWithDefaults", 1)
	cols := []string{"NAME", "VALUE", "ISDEFAULT"}
	values := []driver.Value{"optimizer_mode", nil, "UNKNOWN"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "optimizer_mode"}, {Null: true}, {Text: "UNKNOWN"}}}}
	decoded, err := DecodeSessionParametersWithDefaults(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], SessionParametersWithDefaultsRow{Name: session.Value{Text: "optimizer_mode"}, Value: session.Value{Null: true}, IsDefault: session.Value{Text: "UNKNOWN"}}) {
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
	facts, err := ReadSessionParametersWithDefaults(t.Context(), reader, SessionParametersWithDefaultsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeSessionParametersWithDefaults(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadSessionParametersWithDefaults(ctx, reader, SessionParametersWithDefaultsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadSessionParametersWithDefaults(t.Context(), reader, SessionParametersWithDefaultsRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadSessionParametersWithDefaults(t.Context(), reader, SessionParametersWithDefaultsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := SessionParametersWithDefaultsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := SessionParametersWithDefaultsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"optimizer_mode", nil, "UNKNOWN"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := SessionParametersWithDefaultsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetNLSInstanceCompleteAPI(t *testing.T) {
	options := NLSInstanceOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "database", "NLSInstance", 1)
	cols := []string{"NAME", "VALUE"}
	values := []driver.Value{"NLS_LANGUAGE", "AMERICAN"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "NLS_LANGUAGE"}, {Text: "AMERICAN"}}}}
	decoded, err := DecodeNLSInstance(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], NLSInstanceRow{Name: session.Value{Text: "NLS_LANGUAGE"}, Value: session.Value{Text: "AMERICAN"}}) {
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
	facts, err := ReadNLSInstance(t.Context(), reader, NLSInstanceRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeNLSInstance(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadNLSInstance(ctx, reader, NLSInstanceRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadNLSInstance(t.Context(), reader, NLSInstanceRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadNLSInstance(t.Context(), reader, NLSInstanceRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := NLSInstanceRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := NLSInstanceJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"NLS_LANGUAGE", "AMERICAN"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := NLSInstanceJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
