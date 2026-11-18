package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// VersionBootstrapRow preserves source values and SQL NULL without application policy.
type VersionBootstrapRow struct{ Value session.Value }

// VersionBootstrapOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type VersionBootstrapOptions struct{ OracleMajor int }

// VersionBootstrapRequest supplies admitted options and a caller-owned SQL reference.
type VersionBootstrapRequest struct {
	Options   VersionBootstrapOptions
	Reference reference.Reference
}

// VersionBootstrapResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type VersionBootstrapResult struct {
	Provenance DatasetProvenance
	Rows       []VersionBootstrapRow
}

// VersionBootstrapStatement selects reviewed SQL with bound values, owning no connection.
func VersionBootstrapStatement(options VersionBootstrapOptions) (string, []any, error) {
	p, err := versionBootstrapDatasetPlan(options)
	return p.sql, p.args, err
}
func versionBootstrapDatasetPlan(options VersionBootstrapOptions) (datasetSelection, error) {
	if options.OracleMajor != 0 {
		if err := datasetMajor(options.OracleMajor); err != nil {
			return datasetSelection{}, err
		}
	}
	query, err := VersionStatement(0)
	if err != nil {
		return datasetSelection{}, err
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VERSION", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$INSTANCE.VERSION", OracleMajor: options.OracleMajor, Scope: "legacy_version_bootstrap_no_ru_or_full_precision"}}, nil
}

// DecodeVersionBootstrap validates the entire shape and every row before returning facts.
func DecodeVersionBootstrap(result session.Result, options VersionBootstrapOptions) (VersionBootstrapResult, error) {
	p, err := versionBootstrapDatasetPlan(options)
	if err != nil {
		return VersionBootstrapResult{}, err
	}
	return decodeDatasetVersionBootstrap(result, p)
}
func decodeDatasetVersionBootstrap(result session.Result, p datasetSelection) (VersionBootstrapResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return VersionBootstrapResult{}, err
	}
	if len(result.Rows) > 1 {
		return VersionBootstrapResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["VERSION"]]
		if !v.Null {
			major, err := numericVersionMajor(v.Text)
			if err != nil || (p.provenance.OracleMajor != 0 && major != p.provenance.OracleMajor) {
				return VersionBootstrapResult{}, ErrInvalidEvidence
			}
		}
	}
	rows := make([]VersionBootstrapRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = VersionBootstrapRow{Value: r[index["VERSION"]]}
	}
	return VersionBootstrapResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadVersionBootstrap executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadVersionBootstrap(ctx context.Context, reader Reader, request VersionBootstrapRequest) (VersionBootstrapResult, error) {
	p, err := versionBootstrapDatasetPlan(request.Options)
	if err != nil {
		return VersionBootstrapResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return VersionBootstrapResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return VersionBootstrapResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return VersionBootstrapResult{}, err
	}
	facts, err := decodeDatasetVersionBootstrap(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return VersionBootstrapResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// VersionBootstrapRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func VersionBootstrapRows(ctx context.Context, q rowset.Queryer, options VersionBootstrapOptions) (*sql.Rows, error) {
	p, err := versionBootstrapDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// VersionBootstrapJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func VersionBootstrapJSON(ctx context.Context, q rowset.Queryer, options VersionBootstrapOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := versionBootstrapDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetVersionBootstrap(r, p); return err })
}
