package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SessionParametersWithDefaultsRow preserves source values and SQL NULL without application policy.
type SessionParametersWithDefaultsRow struct {
	Name      session.Value
	Value     session.Value
	IsDefault session.Value
}

// SessionParametersWithDefaultsOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type SessionParametersWithDefaultsOptions struct{ OracleMajor int }

// SessionParametersWithDefaultsRequest supplies admitted options and a caller-owned SQL reference.
type SessionParametersWithDefaultsRequest struct {
	Options   SessionParametersWithDefaultsOptions
	Reference reference.Reference
}

// SessionParametersWithDefaultsResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type SessionParametersWithDefaultsResult struct {
	Provenance DatasetProvenance
	Rows       []SessionParametersWithDefaultsRow
}

// SessionParametersWithDefaultsStatement selects reviewed SQL with bound values, owning no connection.
func SessionParametersWithDefaultsStatement(options SessionParametersWithDefaultsOptions) (string, []any, error) {
	p, err := sessionParametersWithDefaultsDatasetPlan(options)
	return p.sql, p.args, err
}

func sessionParametersWithDefaultsDatasetPlan(options SessionParametersWithDefaultsOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := ParametersWithDefaultsSQL
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}, {Name: "ISDEFAULT", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$PARAMETER", OracleMajor: options.OracleMajor, Scope: "executing_session_raw_values_and_default_flags"}}, nil
}

// DecodeSessionParametersWithDefaults validates the entire shape and every row before returning facts.
func DecodeSessionParametersWithDefaults(result session.Result, options SessionParametersWithDefaultsOptions) (SessionParametersWithDefaultsResult, error) {
	p, err := sessionParametersWithDefaultsDatasetPlan(options)
	if err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	return decodeDatasetSessionParametersWithDefaults(result, p)
}

func decodeDatasetSessionParametersWithDefaults(result session.Result, p datasetSelection) (SessionParametersWithDefaultsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	rows := make([]SessionParametersWithDefaultsRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = SessionParametersWithDefaultsRow{Name: r[index["NAME"]], Value: r[index["VALUE"]], IsDefault: r[index["ISDEFAULT"]]}
	}
	return SessionParametersWithDefaultsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSessionParametersWithDefaults executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadSessionParametersWithDefaults(ctx context.Context, reader Reader, request SessionParametersWithDefaultsRequest) (SessionParametersWithDefaultsResult, error) {
	p, err := sessionParametersWithDefaultsDatasetPlan(request.Options)
	if err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SessionParametersWithDefaultsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	facts, err := decodeDatasetSessionParametersWithDefaults(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionParametersWithDefaultsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SessionParametersWithDefaultsRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func SessionParametersWithDefaultsRows(ctx context.Context, q rowset.Queryer, options SessionParametersWithDefaultsOptions) (*sql.Rows, error) {
	p, err := sessionParametersWithDefaultsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// SessionParametersWithDefaultsJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func SessionParametersWithDefaultsJSON(ctx context.Context, q rowset.Queryer, options SessionParametersWithDefaultsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sessionParametersWithDefaultsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetSessionParametersWithDefaults(r, p); return err })
}

// ParametersWithDefaultsSQL retains native NAME, raw VALUE and ISDEFAULT text
// effective for the executing session in its admitted container. It is not a
// V$SYSTEM_PARAMETER/SPFILE or cross-container/RAC inventory. Unlike ParametersSQL,
// VALUE is not display-formatted. Preserve SQL NULL and unknown ISDEFAULT values;
// neither default comparisons nor report selection policy belong in this query.
const ParametersWithDefaultsSQL = `SELECT NAME AS NAME, VALUE AS VALUE, ISDEFAULT AS ISDEFAULT FROM V$PARAMETER ORDER BY NAME`
