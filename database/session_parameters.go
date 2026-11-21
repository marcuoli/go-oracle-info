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

// SessionParametersRow preserves source values and SQL NULL without application policy.
type SessionParametersRow struct {
	Name  session.Value
	Value session.Value
}

// SessionParametersOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SessionParametersOptions struct{ OracleMajor int }

// SessionParametersRequest supplies admitted options and a caller-owned SQL reference.
type SessionParametersRequest struct {
	Options   SessionParametersOptions
	Reference reference.Reference
}

// SessionParametersResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SessionParametersResult struct {
	Provenance DatasetProvenance
	Rows       []SessionParametersRow
}

// SessionParametersStatement selects reviewed SQL with bound values, owning no connection.
func SessionParametersStatement(options SessionParametersOptions) (string, []any, error) {
	p, err := sessionParametersDatasetPlan(options)
	return p.sql, p.args, err
}

func sessionParametersDatasetPlan(options SessionParametersOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := strings.Replace(ParametersSQL, "SELECT NAME,", "SELECT NAME AS NAME,", 1)
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PARAMETER", OracleMajor: options.OracleMajor, Scope: "executing_session_display_or_raw_values"}}, nil
}

// DecodeSessionParameters validates the entire shape and every row before returning facts.
func DecodeSessionParameters(result session.Result, options SessionParametersOptions) (SessionParametersResult, error) {
	p, err := sessionParametersDatasetPlan(options)
	if err != nil {
		return SessionParametersResult{}, err
	}
	return decodeDatasetSessionParameters(result, p)
}

func decodeDatasetSessionParameters(result session.Result, p datasetSelection) (SessionParametersResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SessionParametersResult{}, err
	}
	rows := make([]SessionParametersRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = SessionParametersRow{Name: r[index["NAME"]], Value: r[index["VALUE"]]}
	}
	return SessionParametersResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSessionParameters executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadSessionParameters(ctx context.Context, reader Reader, request SessionParametersRequest) (SessionParametersResult, error) {
	p, err := sessionParametersDatasetPlan(request.Options)
	if err != nil {
		return SessionParametersResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SessionParametersResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SessionParametersResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionParametersResult{}, err
	}
	facts, err := decodeDatasetSessionParameters(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionParametersResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SessionParametersRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func SessionParametersRows(ctx context.Context, q rowset.Queryer, options SessionParametersOptions) (*sql.Rows, error) {
	p, err := sessionParametersDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// SessionParametersJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func SessionParametersJSON(ctx context.Context, q rowset.Queryer, options SessionParametersOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sessionParametersDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetSessionParameters(r, p); return err })
}

// ParametersSQL reads display/raw initialization values without choosing comparison policy.
const ParametersSQL = `SELECT NAME, COALESCE(DISPLAY_VALUE,VALUE) AS VALUE FROM V$PARAMETER ORDER BY NAME`
