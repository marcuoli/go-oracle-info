package objects

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
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestOldestStatisticsOwnedAndNative(t *testing.T) {
	options := StatisticsOptions{OracleMajor: 19, Limit: 2}
	owners := []string{"CaseOwner"}
	admitted := OldestStatisticsOptions{StatisticsOptions: options, Owners: owners}
	ref, _ := reference.NewReference("unit", "objects", "oldest", 1)
	columns := []string{"OWNER", "TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED", "SCOPE"}
	values := []driver.Value{"CaseOwner", "T雪", nil, nil, "TABLE", "12.5", nil, nil, "SHARED"}
	input := session.Result{Columns: columns, Rows: [][]session.Value{{{Text: "CaseOwner"}, {Text: "T雪"}, {Null: true}, {Null: true}, {Text: "TABLE"}, {Text: "12.5"}, {Null: true}, {Null: true}, {Text: "SHARED"}}}}
	facts, err := DecodeOldestStatisticsWithOptions(input, admitted)
	if err != nil || facts.Provenance.Complete || len(facts.Rows) != 1 || facts.Rows[0].NumRows.String != "12.5" || facts.Rows[0].LastAnalyzed.Valid || facts.Rows[0].Scope == nil || facts.Rows[0].Scope.String != "SHARED" {
		t.Fatal(facts, err)
	}
	reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) { return input, nil })
	facts, err = ReadOldestStatistics(t.Context(), reader, OldestStatisticsRequest{Options: options, Owners: owners, Reference: ref})
	if err != nil || !facts.Provenance.Complete || facts.Provenance.Limit != 2 {
		t.Fatal(facts, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	facts, err = ReadOldestStatistics(ctx, reader, OldestStatisticsRequest{Options: options, Owners: owners, Reference: ref})
	if !errors.Is(err, context.Canceled) || facts.Rows != nil {
		t.Fatal(facts, err)
	}
	db, state := testsql.New(t, testsql.Step{Columns: columns, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	r, err := OldestStatisticsRowsWithOptions(t.Context(), spy, admitted)
	if err != nil || r != spy.last || !r.Next() || state.Closed() != 0 {
		t.Fatal(r, err)
	}
	r.Close()
	db, state = testsql.New(t, testsql.Step{Columns: columns, Values: [][]driver.Value{values}})
	payload, err := OldestStatisticsJSONWithOptions(t.Context(), db, admitted, rowset.Limits{MaxRows: 2, MaxBytes: 8192})
	if err != nil || !json.Valid(payload) || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}

	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []struct{ Name string }
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || document.Provenance.Limit != 2 || !reflect.DeepEqual(document.Provenance.Owners, []string{"CaseOwner"}) || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner", "T\u96ea", nil, nil, "TABLE", "12.5", nil, nil, "SHARED"}}) {
		t.Fatal(string(payload), err)
	}
	if len(document.Data.Columns) != 9 {
		t.Fatal(document.Data.Columns)
	}
	for i, name := range []string{"OWNER", "TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED", "SCOPE"} {
		if document.Data.Columns[i].Name != name {
			t.Fatal(document.Data.Columns)
		}
	}
	// The options-based JSON path forwards the exact encoded byte limit.
	db, state = testsql.New(t, testsql.Step{Columns: columns, Values: [][]driver.Value{values}})
	short, err := OldestStatisticsJSONWithOptions(t.Context(), db, admitted, rowset.Limits{MaxRows: 2, MaxBytes: len(payload) - 1})
	if short != nil || !errors.Is(err, rowset.ErrCapacity) || state.Closed() != 1 {
		t.Fatal(string(short), err, state.Closed())
	}
	late := append([]driver.Value(nil), values...)
	late[0] = "DifferentOwner"
	db, state = testsql.New(t, testsql.Step{Columns: columns, Values: [][]driver.Value{values, late}})
	payload, err = OldestStatisticsJSON(t.Context(), db, options, rowset.Limits{}, owners...)
	if !errors.Is(err, ErrInvalidEvidence) || payload != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
}
