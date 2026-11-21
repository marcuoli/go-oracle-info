package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Option retains source spelling, exact integer text and selected SQL NULL.
type Option struct {
	Parameter session.Value
	Value     session.Value
}

// OptionsResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type OptionsResult struct {
	Provenance Provenance
	Rows       []Option
}

// OptionsRequest admits the release/capabilities and a valid caller-owned SQL reference.
type OptionsRequest struct {
	OracleMajor int
	Reference   reference.Reference
}

// OptionsStatement selects V$OPTION. The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func OptionsStatement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	return `SELECT PARAMETER AS PARAMETER, VALUE AS VALUE FROM V$OPTION ORDER BY PARAMETER`, nil
}

func optionsSchema(major int) []rowset.Column {
	schema := []rowset.Column{
		{Name: "PARAMETER", Kind: rowset.Text},
		{Name: "VALUE", Kind: rowset.Text},
	}

	return schema
}

func optionsProvenance(major int) Provenance {
	p := Provenance{Source: "V$OPTION", Scope: "connected_instance_visible_options", OracleMajor: major}

	return p
}

// DecodeOptions validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeOptions(result session.Result, major int) (OptionsResult, error) {
	if _, err := OptionsStatement(major); err != nil {
		return OptionsResult{}, err
	}
	index, err := factColumns(result, optionsSchema(major))
	if err != nil {
		return OptionsResult{}, err
	}

	facts := make([]Option, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := Option{
			Parameter: row[index["PARAMETER"]],
			Value:     row[index["VALUE"]],
		}

		facts = append(facts, fact)
	}

	return OptionsResult{Provenance: optionsProvenance(major), Rows: facts}, nil
}

// ReadOptions executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadOptions(ctx context.Context, reader Reader, request OptionsRequest) (OptionsResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return OptionsResult{}, err
	}
	statement, err := OptionsStatement(request.OracleMajor)
	if err != nil {
		return OptionsResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return OptionsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return OptionsResult{}, err
	}
	facts, err := DecodeOptions(result, request.OracleMajor)
	if err = factReadError(ctx, err); err != nil {
		return OptionsResult{}, err
	}
	return facts, nil
}

// OptionsRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; OptionsJSON independently owns its cursor.
func OptionsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	statement, err := OptionsStatement(major)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// OptionsJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func OptionsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return OptionsRows(ctx, q, major)
	}, optionsSchema(major), limits, rowset.EnvelopeOptions{
		Provenance: optionsProvenance(major),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}
