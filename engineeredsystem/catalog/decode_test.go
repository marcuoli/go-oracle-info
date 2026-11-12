package catalog

import (
	"github.com/marcuoli/go-oracle-session/session"
	"testing"
)

func TestSharedSchemaDecodePreservesTypeConflictsAndViewAllowlist(t *testing.T) {
	result := session.Result{Columns: []string{"SOURCE_NAME", "COLUMN_NAME", "DATA_TYPE"}, Rows: [][]session.Value{{{Text: "V_$CELL"}, {Text: "CELL_HASHVAL"}, {Text: "NUMBER"}}}}
	schema, err := DecodeSchema(result)
	if err != nil || schema["V_$CELL"]["CELL_HASHVAL"] != "NUMBER" {
		t.Fatal("metadata changed", err)
	}
	result.Rows = append(result.Rows, []session.Value{{Text: "V_$CELL"}, {Text: "CELL_HASHVAL"}, {Text: "CLOB"}})
	if _, err := DecodeSchema(result); err == nil {
		t.Fatal("conflicting metadata accepted")
	}
	result.Rows = result.Rows[:1]
	result.Rows[0][0].Text = "V_$UNREVIEWED"
	if _, err := DecodeSchema(result); err == nil {
		t.Fatal("unreviewed relation accepted")
	}
	result.Rows[0][0] = session.Value{Null: true}
	if _, err := DecodeSchema(result); err == nil {
		t.Fatal("NULL metadata identity accepted")
	}
}
