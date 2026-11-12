package catalog

import (
	"strings"
	"testing"
)

func schemaViews(global bool) Schema {
	prefix := "V_$"
	if global {
		prefix = "GV_$"
	}
	schema := Schema{prefix + "CELL": {"CELL_HASHVAL": "NUMBER", "CELL_PATH": "VARCHAR2", "CELL_TYPE": "VARCHAR2"}, prefix + "CELL_CONFIG": {"CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "CLOB"}}
	if global {
		for _, columns := range schema {
			columns["INST_ID"] = "NUMBER"
		}
	}
	return schema
}
func TestCapabilitySelectedProjections(t *testing.T) {
	for _, global := range []bool{false, true} {
		schema := schemaViews(global)
		cells, ok, err := Cells(schema)
		if err != nil || !ok {
			t.Fatalf("cells: %v %v", ok, err)
		}
		config, ok, err := Configuration(schema, false)
		if err != nil || !ok {
			t.Fatalf("config: %v %v", ok, err)
		}
		for _, command := range []Command{cells, config} {
			if command.Original == "" || strings.Contains(command.Text, "{{") || strings.Contains(command.Text, "SELECT *") {
				t.Fatalf("invalid reviewed command: %#v", command)
			}
			if global && !strings.Contains(command.Text, "C.INST_ID AS INST_ID") {
				t.Fatal("global instance provenance missing")
			}
			if !global && !strings.Contains(command.Text, "SYS_CONTEXT('USERENV', 'INSTANCE')") {
				t.Fatal("local current instance provenance missing")
			}
		}
		if !strings.Contains(config.Text, "C.CONFVAL AS XML_PAYLOAD") || !strings.Contains(config.Text, "DBMS_LOB.GETLENGTH(C.CONFVAL) AS XML_LENGTH") || strings.Contains(config.Text, "SUBSTR") || strings.Contains(config.Text, "WHERE") {
			t.Fatal("configuration payloads/types shortened or filtered")
		}
	}
}
func TestUnknownGlobalSchemaCanUseKnownLocalSource(t *testing.T) {
	schema := schemaViews(false)
	schema["GV_$CELL"] = map[string]string{"INST_ID": "NUMBER", "NEW_LAYOUT": "CLOB"}
	command, ok, err := Cells(schema)
	if err != nil || !ok || command.View != "SYS.V_$CELL" {
		t.Fatalf("local fallback lost: %#v %v %v", command, ok, err)
	}
	schema["GV_$CELL_CONFIG"] = map[string]string{"INST_ID": "NUMBER", "CELLNAME": "VARCHAR2", "CONFVAL": "BLOB", "CONFTYPE": "VARCHAR2"}
	command, ok, err = Configuration(schema, false)
	if err != nil || !ok || command.View != "SYS.V_$CELL_CONFIG" {
		t.Fatalf("local configuration fallback lost: %#v %v %v", command, ok, err)
	}
}
func TestUnknownAndMissingSchemasRemainDistinct(t *testing.T) {
	schema := Schema{"V_$CELL_CONFIG": {"CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "BLOB"}}
	command, ok, err := Configuration(schema, false)
	if err != nil || ok || command.View == "" || !Visible(schema, "CELL_CONFIG") {
		t.Fatal("unknown visible schema not reported")
	}
	command, ok, err = Configuration(schema, true)
	if err != nil || ok || command.View != "" || Visible(schema, "CELL_CONFIG_INFO") {
		t.Fatal("missing source not reported")
	}
	if err := ValidateSchema(Schema{"V_$OTHER": {"VALUE": "VARCHAR2"}}); err == nil {
		t.Fatal("unreviewed relation accepted")
	}
}
func TestSourceColumnVariantsAndTextLengths(t *testing.T) {
	schema := Schema{"V_$CELL_CONFIG_INFO": {"CELL_NAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "VARCHAR2", "CELLHASH": "NUMBER"}}
	command, ok, err := Configuration(schema, true)
	if err != nil || !ok || !strings.Contains(command.Text, "LENGTH2(C.CONFVAL)") || !strings.Contains(command.Text, "C.CELL_NAME AS CELL_NAME") {
		t.Fatalf("text/info variant lost: %#v %v", command, err)
	}
	schema = Schema{"V_$CELL_OPEN_ALERTS": {"CELL_NAME": "VARCHAR2", "ALERT_VALUE": "CLOB", "ALERT_TYPE": "VARCHAR2"}}
	command, ok, err = Alerts(schema)
	if err != nil || !ok || !strings.Contains(command.Text, "C.ALERT_VALUE AS XML_PAYLOAD") {
		t.Fatalf("observed XML alert shape lost: %#v %v", command, err)
	}
	schema["V_$CELL_OPEN_ALERTS"] = map[string]string{"MESSAGE": "VARCHAR2"}
	_, ok, err = Alerts(schema)
	if err != nil || ok {
		t.Fatal("unreviewed alert schema accepted")
	}
}
func TestNative122ScalarAlertProjectionPreservesFieldsAndScope(t *testing.T) {
	for _, global := range []bool{false, true} {
		s := schemaViews(global)
		view := "V_$CELL_OPEN_ALERTS"
		if global {
			view = "G" + view
		}
		s[view] = map[string]string{"CELL_NAME": "VARCHAR2", "CELL_HASH": "NUMBER", "BEGIN_TIME": "DATE", "SEQ_NO": "NUMBER", "MESSAGE": "VARCHAR2", "STATEFUL": "CHAR", "SEVERITY": "VARCHAR2", "CON_ID": "NUMBER"}
		if global {
			s[view]["INST_ID"] = "NUMBER"
		}
		command, ok, err := Alerts(s)
		if err != nil || !ok || command.ID != "engineered-alerts" {
			t.Fatalf("native alert shape rejected: %#v %v %v", command, ok, err)
		}
		for _, part := range []string{"TO_CHAR(C.CELL_HASH, 'TM9') AS CELL_HASHVAL", "C.MESSAGE AS MESSAGE", "C.SEVERITY AS SEVERITY", "TO_CHAR(C.CON_ID, 'TM9') AS CON_ID", "HH24:MI:SS AD", "NLS_CALENDAR=Gregorian"} {
			if !strings.Contains(command.Text, part) {
				t.Fatalf("projection lost %q", part)
			}
		}
		if strings.Contains(command.Text, "TZH") || strings.Contains(command.Text, "SUBSTR") {
			t.Fatal("source DATE gained a zone or source text was shortened")
		}
		s[view]["BEGIN_TIME"] = "TIMESTAMP"
		if _, supported, err := Alerts(s); err != nil || supported {
			t.Fatal("unreviewed timestamp precision accepted as DATE")
		}
	}
}
