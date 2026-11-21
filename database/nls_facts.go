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

// NLSParameter retains source spelling, exact integer text and selected SQL NULL.
type NLSParameter struct {
	Name  session.Value
	Value session.Value
}

// NLSDatabaseResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type NLSDatabaseResult struct {
	Provenance Provenance
	Rows       []NLSParameter
}

// NLSDatabaseRequest admits the release/capabilities and a valid caller-owned SQL reference.
type NLSDatabaseRequest struct {
	OracleMajor int
	Reference   reference.Reference
}

// NLSDatabaseStatement selects NLS_DATABASE_PARAMETERS. The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func NLSDatabaseStatement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	return `SELECT PARAMETER AS NAME, VALUE AS VALUE FROM NLS_DATABASE_PARAMETERS ORDER BY PARAMETER`, nil
}

func nlsDatabaseSchema(major int) []rowset.Column {
	schema := []rowset.Column{
		{Name: "NAME", Kind: rowset.Text},
		{Name: "VALUE", Kind: rowset.Text},
	}

	return schema
}

func nlsDatabaseProvenance(major int) Provenance {
	p := Provenance{Source: "NLS_DATABASE_PARAMETERS", Scope: "permanent_database_parameters", OracleMajor: major}

	return p
}

// DecodeNLSDatabase validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeNLSDatabase(result session.Result, major int) (NLSDatabaseResult, error) {
	if _, err := NLSDatabaseStatement(major); err != nil {
		return NLSDatabaseResult{}, err
	}
	index, err := factColumns(result, nlsDatabaseSchema(major))
	if err != nil {
		return NLSDatabaseResult{}, err
	}

	facts := make([]NLSParameter, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := NLSParameter{
			Name:  row[index["NAME"]],
			Value: row[index["VALUE"]],
		}

		facts = append(facts, fact)
	}

	return NLSDatabaseResult{Provenance: nlsDatabaseProvenance(major), Rows: facts}, nil
}

// ReadNLSDatabase executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadNLSDatabase(ctx context.Context, reader Reader, request NLSDatabaseRequest) (NLSDatabaseResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return NLSDatabaseResult{}, err
	}
	statement, err := NLSDatabaseStatement(request.OracleMajor)
	if err != nil {
		return NLSDatabaseResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return NLSDatabaseResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return NLSDatabaseResult{}, err
	}
	facts, err := DecodeNLSDatabase(result, request.OracleMajor)
	if err = factReadError(ctx, err); err != nil {
		return NLSDatabaseResult{}, err
	}
	return facts, nil
}

// NLSDatabaseRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; NLSDatabaseJSON independently owns its cursor.
func NLSDatabaseRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	statement, err := NLSDatabaseStatement(major)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// NLSDatabaseJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func NLSDatabaseJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return NLSDatabaseRows(ctx, q, major)
	}, nlsDatabaseSchema(major), limits, rowset.EnvelopeOptions{
		Provenance: nlsDatabaseProvenance(major),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}
