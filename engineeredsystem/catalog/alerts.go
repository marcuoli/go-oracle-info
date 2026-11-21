package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
)

// AlertRow retains exact raw facts. A nil optional pointer means its source
// column/layout was not admitted; a nonnil Null value is selected SQL NULL.
// XML remains raw evidence, not acceptance, normalization or health policy.
type AlertRow struct {
	InstanceID  session.Value
	Hash        session.Value
	Name        session.Value
	BeginTime   *session.Value
	Sequence    *session.Value
	Message     *session.Value
	Stateful    *session.Value
	Severity    *session.Value
	ContainerID *session.Value
	SourceType  *session.Value
	Payload     *session.Value
	CodeUnits   *session.Value
}

// AlertsOptions supplies an observed numeric major and admitted source capability.
// The caller owns dictionary visibility, state, container and reporter coverage.
// No metadata query or recovery is performed by the selector.
type AlertsOptions struct {
	OracleMajor int
	Schema      Schema
}

// AlertsRequest supplies caller admission and a stable caller-owned reference.
type AlertsRequest struct {
	Options   AlertsOptions
	Reference reference.Reference
}

// AlertsResult preserves row order/duplicates and selected-transport provenance.
// A completed bounded sample is not a complete source or physical inventory.
type AlertsResult struct {
	Provenance DatasetProvenance
	Rows       []AlertRow
}

// AlertsStatement delegates to the reviewed fixed capability-selected catalog.
func AlertsStatement(options AlertsOptions) (string, []any, error) {
	p, err := alertsDatasetPlan(options)
	return p.sql, p.args, err
}

func alertsDatasetPlan(options AlertsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	if err := ValidateSchema(options.Schema); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	command, ok, err := Alerts(options.Schema)
	if err != nil {
		return datasetSelection{}, err
	}
	if !ok {
		return datasetSelection{}, ErrUnsupportedSource
	}
	schema := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CELL_HASHVAL", Kind: rowset.Text}, {Name: "CELL_NAME", Kind: rowset.Text}}
	layout := "xml"
	var optional []string
	if command.ID == "engineered-alerts" {
		layout = "scalar"
		for _, name := range []string{"BEGIN_TIME", "SEQ_NO", "MESSAGE", "STATEFUL", "SEVERITY", "CON_ID"} {
			schema = append(schema, rowset.Column{Name: name, Kind: rowset.Text})
		}
		if options.Schema[strings.TrimPrefix(command.View, "SYS.")]["CON_ID"] == "NUMBER" {
			optional = []string{"CON_ID"}
		}
	} else {
		schema = append(schema, rowset.Column{Name: "SOURCE_TYPE", Kind: rowset.Text}, rowset.Column{Name: "XML_PAYLOAD", Kind: rowset.Text}, rowset.Column{Name: "XML_LENGTH", Kind: rowset.Integer})
	}
	return datasetSelection{sql: command.Text, schema: schema, provenance: DatasetProvenance{Source: command.View, OracleMajor: options.OracleMajor, Scope: "admitted_visible_reporter_raw_open_alert_rows", Layout: layout, OptionalColumns: optional}}, nil
}

// DecodeAlerts validates all selected rows before returning any source facts.
func DecodeAlerts(result session.Result, options AlertsOptions) (AlertsResult, error) {
	p, err := alertsDatasetPlan(options)
	if err != nil {
		return AlertsResult{}, err
	}
	return decodeDatasetAlerts(result, p)
}

func decodeDatasetAlerts(result session.Result, p datasetSelection) (AlertsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return AlertsResult{}, err
	}
	for _, r := range result.Rows {
		if !validDatasetInstance(r[index["INST_ID"]]) {
			return AlertsResult{}, ErrInvalidEvidence
		}
		if p.provenance.Layout == "xml" && !completeDatasetPayload(r[index["XML_PAYLOAD"]], r[index["XML_LENGTH"]]) {
			return AlertsResult{}, ErrInvalidEvidence
		}
	}
	rows := make([]AlertRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = AlertRow{InstanceID: r[index["INST_ID"]], Hash: r[index["CELL_HASHVAL"]], Name: r[index["CELL_NAME"]],
			BeginTime: optionalDatasetValue(r, index, "BEGIN_TIME", true), Sequence: optionalDatasetValue(r, index, "SEQ_NO", true), Message: optionalDatasetValue(r, index, "MESSAGE", true), Stateful: optionalDatasetValue(r, index, "STATEFUL", true), Severity: optionalDatasetValue(r, index, "SEVERITY", true), ContainerID: optionalDatasetValue(r, index, "CON_ID", slices.Contains(p.provenance.OptionalColumns, "CON_ID")),
			SourceType: optionalDatasetValue(r, index, "SOURCE_TYPE", true), Payload: optionalDatasetValue(r, index, "XML_PAYLOAD", true), CodeUnits: optionalDatasetValue(r, index, "XML_LENGTH", true)}
	}
	return AlertsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadAlerts reads once through a complete finite-budget reader. Its frozen
// selection is shared by decoding even if caller capability maps later change.
func ReadAlerts(ctx context.Context, reader Reader, request AlertsRequest) (AlertsResult, error) {
	p, err := alertsDatasetPlan(request.Options)
	if err != nil {
		return AlertsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return AlertsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return AlertsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return AlertsResult{}, err
	}
	facts, err := decodeDatasetAlerts(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return AlertsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// AlertsRows returns the actual native cursor. Caller owns Next/Scan/Err/Close,
// affinity, scope, complete LOB/length checks, cancellation and finite budgets.
func AlertsRows(ctx context.Context, q rowset.Queryer, options AlertsOptions) (*sql.Rows, error) {
	p, err := alertsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// AlertsJSON validates its independent complete cursor before encoding any bytes.
// Source/layout absence, SQL NULL, native numeric kinds and ordering are retained.
func AlertsJSON(ctx context.Context, q rowset.Queryer, options AlertsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := alertsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetAlerts(r, p); return err })
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

const ScalarAlertsTemplate = `SELECT {{.Instance}} AS INST_ID,
       TO_CHAR(C.CELL_HASH, 'TM9') AS CELL_HASHVAL,
       C.CELL_NAME AS CELL_NAME,
       TO_CHAR(C.BEGIN_TIME, 'YYYY-MM-DD"T"HH24:MI:SS AD', 'NLS_DATE_LANGUAGE=English NLS_CALENDAR=Gregorian') AS BEGIN_TIME,
       TO_CHAR(C.SEQ_NO, 'TM9') AS SEQ_NO,
       C.MESSAGE AS MESSAGE,
       C.STATEFUL AS STATEFUL,
       C.SEVERITY AS SEVERITY,
       {{.Container}} AS CON_ID
FROM {{.View}} C`
