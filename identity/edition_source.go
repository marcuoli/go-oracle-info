package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// EditionSourceRow preserves source values and SQL NULL without application policy.
type EditionSourceRow struct{ Value session.Value }

// EditionSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type EditionSourceOptions struct{ OracleMajor int }

// EditionSourceRequest supplies admitted options and a caller-owned SQL reference.
type EditionSourceRequest struct {
	Options   EditionSourceOptions
	Reference reference.Reference
}

// EditionSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type EditionSourceResult struct {
	Provenance DatasetProvenance
	Rows       []EditionSourceRow
}

// EditionSourceStatement selects reviewed SQL with bound values, owning no connection.
func EditionSourceStatement(options EditionSourceOptions) (string, []any, error) {
	p, err := editionSourceDatasetPlan(options)
	return p.sql, p.args, err
}

func editionSourceDatasetPlan(options EditionSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query, err := EditionStatement(options.OracleMajor)
	if err != nil {
		return datasetSelection{}, err
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "EDITION", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$INSTANCE.EDITION", OracleMajor: options.OracleMajor, Scope: "observed_edition_code_not_licensing"}}, nil
}

// DecodeEditionSource validates the entire shape and every row before returning facts.
func DecodeEditionSource(result session.Result, options EditionSourceOptions) (EditionSourceResult, error) {
	p, err := editionSourceDatasetPlan(options)
	if err != nil {
		return EditionSourceResult{}, err
	}
	return decodeDatasetEditionSource(result, p)
}

func decodeDatasetEditionSource(result session.Result, p datasetSelection) (EditionSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return EditionSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return EditionSourceResult{}, ErrInvalidEvidence
	}
	rows := make([]EditionSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = EditionSourceRow{Value: r[index["EDITION"]]}
	}
	return EditionSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadEditionSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadEditionSource(ctx context.Context, reader Reader, request EditionSourceRequest) (EditionSourceResult, error) {
	p, err := editionSourceDatasetPlan(request.Options)
	if err != nil {
		return EditionSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return EditionSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return EditionSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return EditionSourceResult{}, err
	}
	facts, err := decodeDatasetEditionSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return EditionSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// EditionSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func EditionSourceRows(ctx context.Context, q rowset.Queryer, options EditionSourceOptions) (*sql.Rows, error) {
	p, err := editionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// EditionSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func EditionSourceJSON(ctx context.Context, q rowset.Queryer, options EditionSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := editionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetEditionSource(r, p); return err })
}

// EditionStatement reads the observed instance edition code on known Oracle 12+.
// The documented 11g view has no EDITION column; use its banner as separate
// evidence instead of synthesizing a code. An observed code is not a license grant.
func EditionStatement(major int) (string, error) {
	if major < 12 {
		return "", ErrUnsupportedVersion
	}
	return "SELECT EDITION AS EDITION FROM V$INSTANCE", nil
}

// DecodeEdition retains the exact observed code, SQL NULL or authoritative no row.
// Unknown codes are evidence, not an instruction to infer a different edition.
func DecodeEdition(result session.Result) (Text, error) {
	return decodeScalar(result, "EDITION")
}
