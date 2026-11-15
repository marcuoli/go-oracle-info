package patches

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SQLPatch retains source spelling, exact integer text and selected SQL NULL.
// Date/time labels are source-local civil values with no inferred timezone.
type SQLPatch struct {
	PatchID     session.Value
	PatchUID    session.Value
	Action      session.Value
	Status      session.Value
	ActionTime  session.Value
	Description session.Value
	Logfile     session.Value
	Flags       session.Value
	// Optional fields are nil only when their source column was not selected.
	InstallID              *session.Value
	Version                *session.Value
	BundleSeries           *session.Value
	BundleID               *session.Value
	PatchType              *session.Value
	SourceVersion          *session.Value
	SourceBuildDescription *session.Value
	SourceBuildTimestamp   *session.Value
	TargetVersion          *session.Value
	TargetBuildDescription *session.Value
	TargetBuildTimestamp   *session.Value
	RULogfile              *session.Value
}

// SQLPatchResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type SQLPatchResult struct {
	Provenance Provenance
	Rows       []SQLPatch
}

// SQLPatchRequest admits the release/capabilities and a valid caller-owned SQL reference.
type SQLPatchRequest struct {
	Options   SQLPatchOptions
	Reference reference.Reference
}

// SQLPatchStatement selects DBA_REGISTRY_SQLPATCH. The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func SQLPatchStatement(options SQLPatchOptions) (string, error) {
	if (options.OracleMajor == 12 && options.OracleMinor != 1 && options.OracleMinor != 2) || (options.OracleMajor != 12 && (options.OracleMajor < 18 || options.OracleMinor != 0)) {
		return "", ErrUnsupportedVersion
	}
	statement := `SELECT TO_CHAR(PATCH_ID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS PATCH_ID, TO_CHAR(PATCH_UID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS PATCH_UID, ACTION AS ACTION, STATUS AS STATUS, TO_CHAR(ACTION_TIME,'YYYY-MM-DD"T"HH24:MI:SS.FF6') AS ACTION_TIME, DESCRIPTION AS DESCRIPTION, LOGFILE AS LOGFILE, FLAGS AS FLAGS`
	if options.OracleMajor >= 18 || options.OracleMinor == 2 {
		statement += `, TO_CHAR(INSTALL_ID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS INSTALL_ID`
	}
	if options.OracleMajor == 12 {
		statement += `, VERSION AS VERSION, BUNDLE_SERIES AS BUNDLE_SERIES, TO_CHAR(BUNDLE_ID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS BUNDLE_ID`
	} else {
		statement += `, PATCH_TYPE AS PATCH_TYPE, SOURCE_VERSION AS SOURCE_VERSION, SOURCE_BUILD_DESCRIPTION AS SOURCE_BUILD_DESCRIPTION, TO_CHAR(SOURCE_BUILD_TIMESTAMP,'YYYY-MM-DD"T"HH24:MI:SS.FF6') AS SOURCE_BUILD_TIMESTAMP, TARGET_VERSION AS TARGET_VERSION, TARGET_BUILD_DESCRIPTION AS TARGET_BUILD_DESCRIPTION, TO_CHAR(TARGET_BUILD_TIMESTAMP,'YYYY-MM-DD"T"HH24:MI:SS.FF6') AS TARGET_BUILD_TIMESTAMP, RU_LOGFILE AS RU_LOGFILE`
	}
	return statement + " FROM DBA_REGISTRY_SQLPATCH ORDER BY ACTION_TIME, PATCH_ID, PATCH_UID, ACTION", nil
}

func sqlPatchSchema(options SQLPatchOptions) []rowset.Column {
	schema := []rowset.Column{
		{Name: "PATCH_ID", Kind: rowset.Integer},
		{Name: "PATCH_UID", Kind: rowset.Integer},
		{Name: "ACTION", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "ACTION_TIME", Kind: rowset.Text},
		{Name: "DESCRIPTION", Kind: rowset.Text},
		{Name: "LOGFILE", Kind: rowset.Text},
		{Name: "FLAGS", Kind: rowset.Text},
	}
	if options.OracleMajor >= 18 || options.OracleMinor == 2 {
		schema = append(schema, rowset.Column{Name: "INSTALL_ID", Kind: rowset.Integer})
	}
	if options.OracleMajor == 12 {
		schema = append(schema, rowset.Column{Name: "VERSION", Kind: rowset.Text}, rowset.Column{Name: "BUNDLE_SERIES", Kind: rowset.Text}, rowset.Column{Name: "BUNDLE_ID", Kind: rowset.Integer})
	} else {
		schema = append(schema, rowset.Column{Name: "PATCH_TYPE", Kind: rowset.Text}, rowset.Column{Name: "SOURCE_VERSION", Kind: rowset.Text}, rowset.Column{Name: "SOURCE_BUILD_DESCRIPTION", Kind: rowset.Text}, rowset.Column{Name: "SOURCE_BUILD_TIMESTAMP", Kind: rowset.Text}, rowset.Column{Name: "TARGET_VERSION", Kind: rowset.Text}, rowset.Column{Name: "TARGET_BUILD_DESCRIPTION", Kind: rowset.Text}, rowset.Column{Name: "TARGET_BUILD_TIMESTAMP", Kind: rowset.Text}, rowset.Column{Name: "RU_LOGFILE", Kind: rowset.Text})
	}
	return schema
}

func sqlPatchProvenance(options SQLPatchOptions) Provenance {
	p := Provenance{Source: "DBA_REGISTRY_SQLPATCH", Scope: "current_container_datapatch_attempts", OracleMajor: options.OracleMajor}
	p.OracleMinor = options.OracleMinor
	return p
}

// DecodeSQLPatch validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeSQLPatch(result session.Result, options SQLPatchOptions) (SQLPatchResult, error) {
	if _, err := SQLPatchStatement(options); err != nil {
		return SQLPatchResult{}, err
	}
	index, err := factColumns(result, sqlPatchSchema(options))
	if err != nil {
		return SQLPatchResult{}, err
	}

	facts := make([]SQLPatch, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := SQLPatch{
			PatchID:     row[index["PATCH_ID"]],
			PatchUID:    row[index["PATCH_UID"]],
			Action:      row[index["ACTION"]],
			Status:      row[index["STATUS"]],
			ActionTime:  row[index["ACTION_TIME"]],
			Description: row[index["DESCRIPTION"]],
			Logfile:     row[index["LOGFILE"]],
			Flags:       row[index["FLAGS"]],
		}
		if options.OracleMajor >= 18 || options.OracleMinor == 2 {
			value := row[index["INSTALL_ID"]]
			fact.InstallID = &value
		}
		if options.OracleMajor == 12 {
			value0 := row[index["VERSION"]]
			fact.Version = &value0
			value1 := row[index["BUNDLE_SERIES"]]
			fact.BundleSeries = &value1
			value2 := row[index["BUNDLE_ID"]]
			fact.BundleID = &value2
		} else {
			value0 := row[index["PATCH_TYPE"]]
			fact.PatchType = &value0
			value1 := row[index["SOURCE_VERSION"]]
			fact.SourceVersion = &value1
			value2 := row[index["SOURCE_BUILD_DESCRIPTION"]]
			fact.SourceBuildDescription = &value2
			value3 := row[index["SOURCE_BUILD_TIMESTAMP"]]
			fact.SourceBuildTimestamp = &value3
			value4 := row[index["TARGET_VERSION"]]
			fact.TargetVersion = &value4
			value5 := row[index["TARGET_BUILD_DESCRIPTION"]]
			fact.TargetBuildDescription = &value5
			value6 := row[index["TARGET_BUILD_TIMESTAMP"]]
			fact.TargetBuildTimestamp = &value6
			value7 := row[index["RU_LOGFILE"]]
			fact.RULogfile = &value7
		}
		facts = append(facts, fact)
	}

	return SQLPatchResult{Provenance: sqlPatchProvenance(options), Rows: facts}, nil
}

// ReadSQLPatch executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadSQLPatch(ctx context.Context, reader Reader, request SQLPatchRequest) (SQLPatchResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return SQLPatchResult{}, err
	}
	statement, err := SQLPatchStatement(request.Options)
	if err != nil {
		return SQLPatchResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return SQLPatchResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return SQLPatchResult{}, err
	}
	facts, err := DecodeSQLPatch(result, request.Options)
	if err = factReadError(ctx, err); err != nil {
		return SQLPatchResult{}, err
	}
	return facts, nil
}

// SQLPatchRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; SQLPatchJSON independently owns its cursor.
func SQLPatchRows(ctx context.Context, q rowset.Queryer, options SQLPatchOptions) (*sql.Rows, error) {
	statement, err := SQLPatchStatement(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// SQLPatchJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func SQLPatchJSON(ctx context.Context, q rowset.Queryer, options SQLPatchOptions, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	provenance := sqlPatchProvenance(options)
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: provenance}
	metadata, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(metadata) - 4
	if budget.MaxBytes < 1 {
		return nil, rowset.ErrCapacity
	}

	cursor, err := SQLPatchRows(ctx, q, options)
	if err != nil {
		return nil, err
	}
	envelope.Data, err = rowset.ReadJSON(ctx, cursor, sqlPatchSchema(options), budget)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = factReadError(ctx, err); err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}
