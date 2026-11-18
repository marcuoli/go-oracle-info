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
