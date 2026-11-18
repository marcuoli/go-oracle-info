package catalog

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestCellSourceOptionalColumnAndFrozenSelection(t *testing.T) {
	options := CellsOptions{OracleMajor: 19, Schema: Schema{"GV_$CELL": {"INST_ID": "NUMBER", "CELL_HASHVAL": "NUMBER", "CELL_PATH": "VARCHAR2", "CELL_TYPE": "VARCHAR2"}}}
	cols := []string{"INST_ID", "CELL_HASHVAL", "CELL_PATH", "CELL_TYPE"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "1"}, {Text: "0007"}, {Text: "cell-a"}, {Null: true}}, {{Text: "2"}, {Text: "0008"}, {Text: "cell-b"}, {Text: "EXADATA"}}}}
	facts, err := DecodeCells(input, options)
	if err != nil || len(facts.Rows) != 2 || facts.Rows[0].Type == nil || !facts.Rows[0].Type.Null || facts.Rows[1].Type.Text != "EXADATA" || facts.Provenance.Complete {
		t.Fatal(facts, err)
	}
	ref, _ := reference.NewReference("unit", "catalog", "cells", 1)
	reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		delete(options.Schema["GV_$CELL"], "CELL_TYPE")
		return input, nil
	})
	facts, err = ReadCells(t.Context(), reader, CellsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || facts.Rows[0].Type == nil {
		t.Fatal(facts, err)
	}
	input.Rows = input.Rows[:1]
	facts, err = DecodeCells(input, options)
	if err != nil || facts.Rows[0].Type != nil {
		t.Fatal(facts, err)
	}
	values := [][]driver.Value{{int64(1), "0007", "cell-a", nil}, {int64(2), "0008", "cell-b", nil}}
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: values})
	spy := &datasetSpy{db: db}
	cursor, err := CellsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || !cursor.Next() || state.Closed() != 0 {
		t.Fatal(cursor, err)
	}
	cursor.Close()
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: values})
	payload, err := CellsJSON(t.Context(), db, options, rowset.Limits{})
	var doc struct {
		Provenance DatasetProvenance
		Data       struct{ Rows [][]any }
	}
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	if err = json.Unmarshal(payload, &doc); err != nil || !doc.Provenance.Complete || len(doc.Data.Rows) != 2 || doc.Data.Rows[1][1] != "0008" || doc.Data.Rows[0][3] != nil {
		t.Fatal(string(payload), err)
	}
}
func TestConnectedCellSampleAndMissingCapability(t *testing.T) {
	options := ConnectedCellsOptions{OracleMajor: 19, Limit: 2, CellTypeKnownAbsent: true}
	cols := []string{"CELL_HASHVAL", "CELL_PATH"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "0007"}, {Text: "cell-a"}}, {{Text: "0008"}, {Text: "cell-b"}}}}
	facts, err := DecodeConnectedCells(input, options)
	if err != nil || facts.Rows[0].Type != nil || facts.Provenance.Limit != 2 {
		t.Fatal(facts, err)
	}
	query, _, err := ConnectedCellsStatement(options)
	if err != nil || strings.Contains(query, "CELL_TYPE") {
		t.Fatal(query, err)
	}
	options.CellTypeKnownAbsent = false
	if _, _, err = ConnectedCellsStatement(options); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	options.CellTypeKnownAbsent = true
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{{"0007", "cell-a"}, {"0008", "cell-b"}}})
	b, err := ConnectedCellsJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 || !json.Valid(b) {
		t.Fatal(string(b), err)
	}
}
func TestAlertScalarAndCompleteXMLVariants(t *testing.T) {
	scalar := AlertsOptions{OracleMajor: 19, Schema: Schema{"V_$CELL_OPEN_ALERTS": {"CELL_NAME": "VARCHAR2", "CELL_HASH": "NUMBER", "BEGIN_TIME": "DATE", "SEQ_NO": "NUMBER", "MESSAGE": "VARCHAR2", "STATEFUL": "VARCHAR2", "SEVERITY": "VARCHAR2", "CON_ID": "NUMBER"}}}
	cols := []string{"INST_ID", "CELL_HASHVAL", "CELL_NAME", "BEGIN_TIME", "SEQ_NO", "MESSAGE", "STATEFUL", "SEVERITY", "CON_ID"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "1"}, {Text: "7"}, {Text: "cell-a"}, {Text: "2026-11-18T12:34:56 AD"}, {Text: "3"}, {Text: "message\nraw"}, {Text: "TRUE"}, {Text: "critical"}, {Null: true}}}}
	facts, err := DecodeAlerts(input, scalar)
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Message == nil || facts.Rows[0].Message.Text != "message\nraw" || facts.Rows[0].Payload != nil || facts.Rows[0].ContainerID == nil || !facts.Rows[0].ContainerID.Null {
		t.Fatal(facts, err)
	}
	xml := AlertsOptions{OracleMajor: 19, Schema: Schema{"V_$CELL_OPEN_ALERTS": {"CELL_NAME": "VARCHAR2", "ALERT_VALUE": "CLOB"}}}
	cols = []string{"INST_ID", "CELL_HASHVAL", "CELL_NAME", "SOURCE_TYPE", "XML_PAYLOAD", "XML_LENGTH"}
	input = session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "1"}, {Null: true}, {Text: "cell-a"}, {Text: "OPEN_ALERTS"}, {Text: "<x>\U0001f600</x>"}, {Text: "9"}}}}
	facts, err = DecodeAlerts(input, xml)
	if err != nil || facts.Rows[0].Payload == nil || facts.Rows[0].Payload.Text != "<x>\U0001f600</x>" || facts.Rows[0].Message != nil {
		t.Fatal(facts, err)
	}
	values := []driver.Value{int64(1), nil, "cell-a", "OPEN_ALERTS", "<x>\U0001f600</x>", int64(9)}
	late := append([]driver.Value(nil), values...)
	late[5] = int64(8)
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, late}})
	b, err := AlertsJSON(t.Context(), db, xml, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || b != nil || state.Closed() != 1 {
		t.Fatal(string(b), err)
	}
}
