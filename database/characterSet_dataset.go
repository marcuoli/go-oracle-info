package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// CharacterSetRow preserves source values and SQL NULL without application policy.
type CharacterSetRow struct{ Value session.Value }

// CharacterSetOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type CharacterSetOptions struct{ OracleMajor int }

// CharacterSetRequest supplies admitted options and a caller-owned SQL reference.
type CharacterSetRequest struct {
	Options   CharacterSetOptions
	Reference reference.Reference
}

// CharacterSetResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type CharacterSetResult struct {
	Provenance DatasetProvenance
	Rows       []CharacterSetRow
}

// CharacterSetStatement selects reviewed SQL with bound values, owning no connection.
func CharacterSetStatement(options CharacterSetOptions) (string, []any, error) {
	p, err := characterSetDatasetPlan(options)
	return p.sql, p.args, err
}
func characterSetDatasetPlan(options CharacterSetOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := strings.Replace(CharacterSetSQL, "SELECT VALUE FROM", "SELECT VALUE AS VALUE FROM", 1)
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "NLS_DATABASE_PARAMETERS", OracleMajor: options.OracleMajor, Scope: "database_character_set"}}, nil
}

// DecodeCharacterSet validates the entire shape and every row before returning facts.
func DecodeCharacterSet(result session.Result, options CharacterSetOptions) (CharacterSetResult, error) {
	p, err := characterSetDatasetPlan(options)
	if err != nil {
		return CharacterSetResult{}, err
	}
	return decodeDatasetCharacterSet(result, p)
}
func decodeDatasetCharacterSet(result session.Result, p datasetSelection) (CharacterSetResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return CharacterSetResult{}, err
	}
	if len(result.Rows) > 1 {
		return CharacterSetResult{}, ErrInvalidEvidence
	}
	rows := make([]CharacterSetRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = CharacterSetRow{Value: r[index["VALUE"]]}
	}
	return CharacterSetResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCharacterSet executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadCharacterSet(ctx context.Context, reader Reader, request CharacterSetRequest) (CharacterSetResult, error) {
	p, err := characterSetDatasetPlan(request.Options)
	if err != nil {
		return CharacterSetResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return CharacterSetResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return CharacterSetResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return CharacterSetResult{}, err
	}
	facts, err := decodeDatasetCharacterSet(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return CharacterSetResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// CharacterSetRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func CharacterSetRows(ctx context.Context, q rowset.Queryer, options CharacterSetOptions) (*sql.Rows, error) {
	p, err := characterSetDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// CharacterSetJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func CharacterSetJSON(ctx context.Context, q rowset.Queryer, options CharacterSetOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := characterSetDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetCharacterSet(r, p); return err })
}
