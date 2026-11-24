package objects

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestSubobjectStatisticsSelection(t *testing.T) {
	owner := "Mixed Case"
	var reviewedSQL string
	for _, major := range []int{10, 11, 12, 18, 19, 21, 23} {
		statement, args, err := SubobjectStatisticsStatement(SubobjectStatisticsOptions{OracleMajor: major, Owner: owner})
		if err != nil || !reflect.DeepEqual(args, []any{owner}) || strings.Contains(statement, owner) {
			t.Fatalf("version %d: query=%q args=%v error=%v", major, statement, args, err)
		}
		if reviewedSQL != "" && statement != reviewedSQL {
			t.Fatalf("version %d diverged from the reviewed common SQL", major)
		}
		reviewedSQL = statement
	}
	for _, clause := range []string{
		"TABLE_NAME AS TABLE_NAME, PARTITION_NAME AS PARTITION_NAME, SUBPARTITION_NAME AS SUBPARTITION_NAME",
		"TO_CHAR(NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT",
		`TO_CHAR(LAST_ANALYZED, 'YYYY-MM-DD"T"HH24:MI:SS', 'NLS_CALENDAR=GREGORIAN') AS LAST_ANALYZED`,
		"FROM DBA_TAB_STATISTICS",
		"NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')",
		"OBJECT_TYPE IN ('PARTITION','SUBPARTITION')",
	} {
		if !strings.Contains(reviewedSQL, clause) {
			t.Fatalf("missing %q in %s", clause, reviewedSQL)
		}
	}
	for _, clause := range []string{"COUNT(", "DBMS_STATS", "NVL(", "ROWNUM", "FETCH FIRST", "ROW_NUMBER("} {
		if strings.Contains(reviewedSQL, clause) {
			t.Fatalf("unexpected %q in %s", clause, reviewedSQL)
		}
	}
	reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		t.Fatal("invalid or unsupported request reached the reader")
		return session.Result{}, nil
	})
	ref, err := reference.NewReference("unit", "objects", "subobject-stats", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, major := range []int{0, 8, 9, 13, 26, 99} {
		options := SubobjectStatisticsOptions{OracleMajor: major, Owner: owner}
		statement, args, err := SubobjectStatisticsStatement(options)
		if !errors.Is(err, ErrUnsupportedVersion) || statement != "" || args != nil {
			t.Fatalf("version %d: query=%q args=%v error=%v", major, statement, args, err)
		}
		result, err := ReadSubobjectStatistics(t.Context(), reader, SubobjectStatisticsRequest{Options: options, Reference: ref})
		if !errors.Is(err, ErrUnsupportedVersion) || result.Rows != nil || result.Provenance.Complete {
			t.Fatalf("version %d: result=%#v error=%v", major, result, err)
		}
	}
	result, err := ReadSubobjectStatistics(t.Context(), reader, SubobjectStatisticsRequest{Options: SubobjectStatisticsOptions{OracleMajor: 23}, Reference: ref})
	if !errors.Is(err, ErrInvalidRequest) || result.Rows != nil {
		t.Fatal(result, err)
	}
}

func TestDecodeAndReadSubobjectStatistics(t *testing.T) {
	options := SubobjectStatisticsOptions{OracleMajor: 23, Owner: "CaseOwner"}
	ref, err := reference.NewReference("unit", "objects", "subobject-stats", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := SubobjectStatisticsRequest{Options: options, Reference: ref}
	columns := []string{"table_name", "ROW_COUNT", "LAST_ANALYZED", "PARTITION_NAME", "SUBPARTITION_NAME"}
	null := session.Value{Null: true}
	table, partition, child := session.Value{Text: "T雪"}, session.Value{Text: "PaRent"}, session.Value{Text: "Child"}
	estimate, date := session.Value{Text: "9999999999999999999999999999999999999.5"}, session.Value{Text: "2026-11-24T17:19:03"}
	parentRow := []session.Value{table, null, null, partition, null}
	childRow := []session.Value{table, estimate, date, partition, child}
	input := session.Result{Columns: columns, Rows: [][]session.Value{parentRow, childRow, childRow}}
	want := []SubobjectStatisticsRow{
		{Name: table, Partition: partition, Subpartition: null, EstimatedRows: null, LastAnalyzed: null},
		{Name: table, Partition: partition, Subpartition: child, EstimatedRows: estimate, LastAnalyzed: date},
		{Name: table, Partition: partition, Subpartition: child, EstimatedRows: estimate, LastAnalyzed: date},
	}
	decoded, err := DecodeSubobjectStatistics(input, options)
	if err != nil || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows, want) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(_ context.Context, statement session.Statement) (session.Result, error) {
		calls++
		if statement.Text != subobjectStatisticsSQL || !reflect.DeepEqual(statement.Args, []any{options.Owner}) || statement.Reference != ref {
			t.Fatalf("unexpected executed statement: %+v", statement)
		}
		return input, nil
	})
	got, err := ReadSubobjectStatistics(t.Context(), reader, request)
	if err != nil || calls != 1 || !got.Provenance.Complete || got.Provenance.Source != "DBA_TAB_STATISTICS" ||
		got.Provenance.Owner != options.Owner || got.Provenance.OracleMajor != 23 || !reflect.DeepEqual(got.Rows, want) {
		t.Fatal(got, err, calls)
	}
	input.Rows = nil
	got, err = ReadSubobjectStatistics(t.Context(), reader, request)
	if err != nil || !got.Provenance.Complete || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatal(got, err)
	}
	input.Rows = [][]session.Value{parentRow, {table, estimate, date, null, child}}
	got, err = ReadSubobjectStatistics(t.Context(), reader, request)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil || got.Provenance.Complete {
		t.Fatal(got, err)
	}
	input.Rows = [][]session.Value{parentRow}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader = func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return input, nil
	}
	got, err = ReadSubobjectStatistics(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil || got.Provenance.Complete {
		t.Fatal(got, err)
	}
}

func TestSubobjectStatisticsNativeRowsAndJSON(t *testing.T) {
	options := SubobjectStatisticsOptions{OracleMajor: 19, Owner: "CaseOwner"}
	columns := []string{"TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "ROW_COUNT", "LAST_ANALYZED"}
	values := [][]driver.Value{{"T", "P", nil, "0", nil}, {"T", "P", "S", nil, nil}}
	db, state := testsql.New(t, testsql.Step{Columns: columns, Values: values})
	spy := &datasetSpy{db: db}
	rows, err := SubobjectStatisticsRows(t.Context(), spy, options)
	if err != nil || rows != spy.last {
		t.Fatal(rows, err)
	}
	defer rows.Close()
	var table, partition, estimate string
	var child, date *string
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	err = rows.Scan(&table, &partition, &child, &estimate, &date)
	if err != nil || table != "T" || partition != "P" || child != nil || estimate != "0" || date != nil || state.Closed() != 0 {
		t.Fatalf("native parent: %q %q %v %q %v, err=%v", table, partition, child, estimate, date, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	db, state = testsql.New(t, testsql.Step{Columns: columns, Values: values})
	payload, err := SubobjectStatisticsJSON(t.Context(), db, options, rowset.Limits{MaxRows: 2, MaxBytes: 8192})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct{ Rows [][]any }
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete ||
		!reflect.DeepEqual(document.Data.Rows, [][]any{{"T", "P", nil, "0", nil}, {"T", "P", "S", nil, nil}}) {
		t.Fatal(string(payload), err)
	}
	for _, tc := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{
		{"row budget", testsql.Step{Columns: columns, Values: values}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
		{"late invalid identity", testsql.Step{Columns: columns, Values: [][]driver.Value{values[0], {"T", nil, "S", "0", nil}}}, rowset.Limits{}, ErrInvalidEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			payload, err := SubobjectStatisticsJSON(t.Context(), db, options, tc.limits)
			if payload != nil || !errors.Is(err, tc.want) || state.Closed() != 1 {
				t.Fatal(string(payload), err, state.Closed())
			}
		})
	}
}

func TestCountSubobjectStatement(t *testing.T) {
	for _, tc := range []struct {
		partition, child, suffix string
		degree                   int
	}{
		{"p snow 雪", "", ` PARTITION ("p snow 雪")`, 0},
		{"Actual Parent", `s"Suffix`, ` SUBPARTITION ("s""Suffix")`, 4},
	} {
		options := CountOptions{ParallelDegree: tc.degree}
		tableCount, err := CountStatementWithOptions("Case Owner", "Mixed Table", options)
		if err != nil {
			t.Fatal(err)
		}
		statement, err := CountSubobjectStatement("Case Owner", "Mixed Table", tc.partition, tc.child, options)
		if err != nil || statement != tableCount+tc.suffix {
			t.Fatalf("query=%q error=%v", statement, err)
		}
	}
	for _, tc := range []struct {
		owner, table, partition, child string
		degree                         int
	}{
		{"APP", "T", "", "", 0},
		{"APP", "T", "", "S", 0},
		{"APP", "T", "P\x00", "S", 0},
		{"APP", "T", "P", "S\x00", 0},
		{"", "T", "P", "", 0},
		{"APP", "", "P", "", 0},
		{"APP", "T", "P", "S", MaxParallelDegree + 1},
	} {
		statement, err := CountSubobjectStatement(tc.owner, tc.table, tc.partition, tc.child, CountOptions{ParallelDegree: tc.degree})
		if err == nil || statement != "" {
			t.Fatalf("invalid scope emitted %q: %+v", statement, tc)
		}
	}
}
