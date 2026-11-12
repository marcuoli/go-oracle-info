// Package catalog owns capability-selected engineered-system SQL. No discovered
// identifier is interpolated: every relation, column and expression is fixed.
package catalog

import (
	"errors"
	"strings"

	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
)

type Schema map[string]map[string]string

type Command struct {
	ID, Text, View string
	Original       string
}

type fragments struct{ Instance, View, CellType, CellHash, CellName, SourceType, Payload, NativeLength sqltemplate.Fragment }

func Metadata() (Command, error) {
	return Command{ID: "engineered-schema", Text: MetadataSQL, View: "SYS.ALL_TAB_COLUMNS", Original: MetadataSQL}, nil
}

func Cells(schema Schema) (Command, bool, error) {
	view, columns := choose(schema, "CELL")
	if view == "" {
		return Command{}, false, nil
	}
	if !instanceValid(view, columns) || columns["CELL_HASHVAL"] != "NUMBER" || !textType(columns["CELL_PATH"]) {
		if local := localFallback(schema, view); local != nil {
			return Cells(local)
		}
		return Command{View: "SYS." + view}, false, nil
	}
	f := fragments{Instance: instance(view), View: sqltemplate.Fragment("SYS." + view), CellType: sqltemplate.Fragment("CAST(NULL AS VARCHAR2(32))")}
	if textType(columns["CELL_TYPE"]) {
		f.CellType = "C.CELL_TYPE"
	}
	text, err := sqltemplate.Render("engineered-cells", CellsTemplate, f)
	return Command{ID: "engineered-cells", Text: text, View: "SYS." + view, Original: CellsTemplate}, true, err
}

// Configuration accepts the two reviewed cell-name spellings without guessing
// singular/plural CONFTYPE values; all payload types/rows remain selected.
func Configuration(schema Schema, info bool) (Command, bool, error) {
	suffix := "CELL_CONFIG"
	if info {
		suffix = "CELL_CONFIG_INFO"
	}
	view, columns := choose(schema, suffix)
	if view == "" {
		return Command{}, false, nil
	}
	name := "CELLNAME"
	if !textType(columns[name]) {
		name = "CELL_NAME"
	}
	if !instanceValid(view, columns) || !textType(columns[name]) || !textType(columns["CONFTYPE"]) {
		if local := localFallback(schema, view); local != nil {
			return Configuration(local, info)
		}
		return Command{View: "SYS." + view}, false, nil
	}
	length, ok := payloadLength(columns["CONFVAL"], "C.CONFVAL")
	if !ok {
		if local := localFallback(schema, view); local != nil {
			return Configuration(local, info)
		}
		return Command{View: "SYS." + view}, false, nil
	}
	f := fragments{Instance: instance(view), View: sqltemplate.Fragment("SYS." + view), CellHash: "CAST(NULL AS VARCHAR2(128))", CellName: sqltemplate.Fragment("C." + name), SourceType: "C.CONFTYPE", Payload: "C.CONFVAL", NativeLength: length}
	if columns["CELLHASH"] == "NUMBER" {
		f.CellHash = "TO_CHAR(C.CELLHASH, 'TM9')"
	}
	return xmlCommand("engineered-config", view, f)
}

// Alerts selects the observed 12.2 scalar schema or the reviewed XML shape.
// Unrecognized projections remain unsupported; no AWR schema is borrowed.
func Alerts(schema Schema) (Command, bool, error) {
	view, columns := choose(schema, "CELL_OPEN_ALERTS")
	if view == "" {
		return Command{}, false, nil
	}
	// Native 12.2 output supplies DATE without a timezone and NUMBER identities.
	// Retain its civil time plus era; retrieval time remains separate.
	scalar := instanceValid(view, columns) && textType(columns["CELL_NAME"]) &&
		columns["CELL_HASH"] == "NUMBER" && columns["BEGIN_TIME"] == "DATE" &&
		columns["SEQ_NO"] == "NUMBER" && textType(columns["MESSAGE"]) &&
		textType(columns["STATEFUL"]) && textType(columns["SEVERITY"])
	if scalar && (columns["CON_ID"] == "" || columns["CON_ID"] == "NUMBER") {
		container := sqltemplate.Fragment("CAST(NULL AS VARCHAR2(128))")
		if columns["CON_ID"] == "NUMBER" {
			container = "TO_CHAR(C.CON_ID, 'TM9')"
		}
		text, err := sqltemplate.Render("engineered-alerts", ScalarAlertsTemplate,
			struct{ Instance, View, Container sqltemplate.Fragment }{instance(view), sqltemplate.Fragment("SYS." + view), container})
		return Command{ID: "engineered-alerts", Text: text, View: "SYS." + view, Original: ScalarAlertsTemplate}, true, err
	}
	name := "CELL_NAME"
	if !textType(columns[name]) {
		name = "CELLNAME"
	}
	length, ok := payloadLength(columns["ALERT_VALUE"], "C.ALERT_VALUE")
	if !instanceValid(view, columns) || !textType(columns[name]) || !ok {
		if local := localFallback(schema, view); local != nil {
			return Alerts(local)
		}
		return Command{View: "SYS." + view}, false, nil
	}
	f := fragments{Instance: instance(view), View: sqltemplate.Fragment("SYS." + view), CellHash: "CAST(NULL AS VARCHAR2(128))", CellName: sqltemplate.Fragment("C." + name), SourceType: "'OPEN_ALERTS'", Payload: "C.ALERT_VALUE", NativeLength: length}
	if textType(columns["ALERT_TYPE"]) {
		f.SourceType = "C.ALERT_TYPE"
	}
	return xmlCommand("engineered-config", view, f)
}

func xmlCommand(id, view string, f fragments) (Command, bool, error) {
	text, err := sqltemplate.Render(id, XMLTemplate, f)
	return Command{ID: id, Text: text, View: "SYS." + view, Original: XMLTemplate}, true, err
}
func choose(s Schema, suffix string) (string, map[string]string) {
	for _, prefix := range []string{"GV_$", "V_$"} {
		view := prefix + suffix
		if columns := s[view]; len(columns) > 0 {
			return view, columns
		}
	}
	return "", nil
}
func instanceValid(view string, columns map[string]string) bool {
	return !strings.HasPrefix(view, "GV_$") || columns["INST_ID"] == "NUMBER"
}
func localFallback(schema Schema, global string) Schema {
	if !strings.HasPrefix(global, "GV_$") || len(schema[strings.TrimPrefix(global, "G")]) == 0 {
		return nil
	}
	next := make(Schema, len(schema))
	for view, columns := range schema {
		if view != global {
			next[view] = columns
		}
	}
	return next
}
func instance(view string) sqltemplate.Fragment {
	if strings.HasPrefix(view, "GV_$") {
		return "C.INST_ID"
	}
	return "TO_NUMBER(SYS_CONTEXT('USERENV', 'INSTANCE'))"
}
func has(columns map[string]string, names ...string) bool {
	for _, name := range names {
		if columns[name] == "" {
			return false
		}
	}
	return true
}
func textType(t string) bool {
	return t == "VARCHAR2" || t == "NVARCHAR2" || t == "CHAR" || t == "NCHAR"
}
func payloadLength(t, expression string) (sqltemplate.Fragment, bool) {
	if t == "CLOB" || t == "NCLOB" {
		return sqltemplate.Fragment("DBMS_LOB.GETLENGTH(" + expression + ")"), true
	}
	if textType(t) {
		return sqltemplate.Fragment("LENGTH2(" + expression + ")"), true
	}
	return "", false
}

// Visible separates an inaccessible/absent view from a visible unsupported
// schema. It never promises which grant or installation caused invisibility.
func Visible(schema Schema, suffix string) bool { view, _ := choose(schema, suffix); return view != "" }
func ValidateSchema(s Schema) error {
	for view := range s {
		switch view {
		case "GV_$CELL", "V_$CELL", "GV_$CELL_CONFIG", "V_$CELL_CONFIG", "GV_$CELL_CONFIG_INFO", "V_$CELL_CONFIG_INFO", "GV_$CELL_OPEN_ALERTS", "V_$CELL_OPEN_ALERTS":
		default:
			return errors.New("unexpected engineered-system dictionary relation")
		}
	}
	return nil
}
