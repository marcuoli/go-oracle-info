// Package patches selects documented patch and upgrade metadata without patching
// the database. History and datapatch installation attempts remain separate.
package patches

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// HistoryEntry retains source spelling, exact integer text and selected SQL NULL.
// Date/time labels are source-local civil values with no inferred timezone.
type HistoryEntry struct {
	ActionTime   session.Value
	Action       session.Value
	Namespace    session.Value
	Version      session.Value
	ID           session.Value
	Comments     session.Value
	BundleSeries *session.Value
}

// HistoryResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type HistoryResult struct {
	Provenance Provenance
	Rows       []HistoryEntry
}

// HistoryRequest admits the release/capabilities and a valid caller-owned SQL reference.
type HistoryRequest struct {
	Options   HistoryOptions
	Reference reference.Reference
}

// HistoryStatement selects DBA_REGISTRY_HISTORY. The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func HistoryStatement(options HistoryOptions) (string, error) {
	if options.OracleMajor < 11 {
		return "", ErrUnsupportedVersion
	}
	statement := `SELECT TO_CHAR(ACTION_TIME,'YYYY-MM-DD"T"HH24:MI:SS.FF6') AS ACTION_TIME, ACTION AS ACTION, NAMESPACE AS NAMESPACE, VERSION AS VERSION, TO_CHAR(ID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS ID, COMMENTS AS COMMENTS`
	if options.IncludeBundleSeries {
		statement += ", BUNDLE_SERIES AS BUNDLE_SERIES"
	}
	return statement + " FROM DBA_REGISTRY_HISTORY ORDER BY ACTION_TIME, ID, ACTION, NAMESPACE, VERSION", nil
}

func historySchema(options HistoryOptions) []rowset.Column {
	schema := []rowset.Column{
		{Name: "ACTION_TIME", Kind: rowset.Text},
		{Name: "ACTION", Kind: rowset.Text},
		{Name: "NAMESPACE", Kind: rowset.Text},
		{Name: "VERSION", Kind: rowset.Text},
		{Name: "ID", Kind: rowset.Integer},
		{Name: "COMMENTS", Kind: rowset.Text},
	}
	if options.IncludeBundleSeries {
		schema = append(schema, rowset.Column{Name: "BUNDLE_SERIES", Kind: rowset.Text})
	}
	return schema
}

func historyProvenance(options HistoryOptions) Provenance {
	p := Provenance{Source: "DBA_REGISTRY_HISTORY", Scope: "current_container_dictionary", OracleMajor: options.OracleMajor}
	if options.IncludeBundleSeries {
		p.OptionalColumns = []string{"BUNDLE_SERIES"}
	}
	return p
}

// DecodeHistory validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeHistory(result session.Result, options HistoryOptions) (HistoryResult, error) {
	if _, err := HistoryStatement(options); err != nil {
		return HistoryResult{}, err
	}
	index, err := factColumns(result, historySchema(options))
	if err != nil {
		return HistoryResult{}, err
	}

	facts := make([]HistoryEntry, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := HistoryEntry{
			ActionTime: row[index["ACTION_TIME"]],
			Action:     row[index["ACTION"]],
			Namespace:  row[index["NAMESPACE"]],
			Version:    row[index["VERSION"]],
			ID:         row[index["ID"]],
			Comments:   row[index["COMMENTS"]],
		}
		if options.IncludeBundleSeries {
			value := row[index["BUNDLE_SERIES"]]
			fact.BundleSeries = &value
		}
		facts = append(facts, fact)
	}

	return HistoryResult{Provenance: historyProvenance(options), Rows: facts}, nil
}

// ReadHistory executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadHistory(ctx context.Context, reader Reader, request HistoryRequest) (HistoryResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return HistoryResult{}, err
	}
	statement, err := HistoryStatement(request.Options)
	if err != nil {
		return HistoryResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return HistoryResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return HistoryResult{}, err
	}
	facts, err := DecodeHistory(result, request.Options)
	if err = factReadError(ctx, err); err != nil {
		return HistoryResult{}, err
	}
	return facts, nil
}

// HistoryRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; HistoryJSON independently owns its cursor.
func HistoryRows(ctx context.Context, q rowset.Queryer, options HistoryOptions) (*sql.Rows, error) {
	statement, err := HistoryStatement(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// HistoryJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func HistoryJSON(ctx context.Context, q rowset.Queryer, options HistoryOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return HistoryRows(ctx, q, options)
	}, historySchema(options), limits, rowset.EnvelopeOptions{
		Provenance: historyProvenance(options),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}

// HistoryOptions admits BUNDLE_SERIES independently for older patch-set sources.
// A nil BundleSeries is unselected; a nonnil Null value is selected SQL NULL.
type HistoryOptions struct {
	OracleMajor         int
	IncludeBundleSeries bool
}

// SQLPatchOptions distinguishes the documented 12.1, 12.2 and 18+ projections.
// OracleMinor must be 1 or 2 for 12; 18+ requires zero because its major selects
// the modern metadata layout. XML descriptors and binary patch payloads are not read.
type SQLPatchOptions struct {
	OracleMajor int
	OracleMinor int
}
