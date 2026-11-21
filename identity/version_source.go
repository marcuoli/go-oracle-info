package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// VersionStatement uses VERSION for bootstrap/legacy reads and VERSION_FULL only
// for a known Oracle 18+ source. Unknown release zero never probes a modern column.
func VersionStatement(major int) (string, error) {
	if major != 0 && major < 9 {
		return "", ErrUnsupportedVersion
	}
	column := "VERSION"
	if major >= 18 {
		column = "VERSION_FULL"
	}
	return "SELECT " + column + " AS VERSION FROM V$INSTANCE", nil
}

// DecodeVersion preserves full source version text; normalization belongs to the
// consuming comparison/display contract, not to this evidence decoder.
func DecodeVersion(result session.Result) (Text, error) {
	r, absent, err := row(result, "VERSION")
	if err != nil {
		return Text{}, err
	}
	if absent {
		return Text{State: NoRows}, nil
	}
	return text(r["VERSION"]), nil
}

// VersionSourceRow preserves source values and SQL NULL without application policy.
type VersionSourceRow struct{ Value session.Value }

// VersionSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type VersionSourceOptions struct{ OracleMajor int }

// VersionSourceRequest supplies admitted options and a caller-owned SQL reference.
type VersionSourceRequest struct {
	Options   VersionSourceOptions
	Reference reference.Reference
}

// VersionSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type VersionSourceResult struct {
	Provenance DatasetProvenance
	Rows       []VersionSourceRow
}

// VersionSourceStatement selects reviewed SQL with bound values, owning no connection.
func VersionSourceStatement(options VersionSourceOptions) (string, []any, error) {
	p, err := versionSourceDatasetPlan(options)
	return p.sql, p.args, err
}

func versionSourceDatasetPlan(options VersionSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query, err := VersionStatement(options.OracleMajor)
	if err != nil {
		return datasetSelection{}, err
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "VERSION", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$INSTANCE.VERSION_FULL", OracleMajor: options.OracleMajor, Scope: "admitted_numeric_full_version_source_alias"}}, nil
}

// DecodeVersionSource validates the entire shape and every row before returning facts.
func DecodeVersionSource(result session.Result, options VersionSourceOptions) (VersionSourceResult, error) {
	p, err := versionSourceDatasetPlan(options)
	if err != nil {
		return VersionSourceResult{}, err
	}
	return decodeDatasetVersionSource(result, p)
}

func decodeDatasetVersionSource(result session.Result, p datasetSelection) (VersionSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return VersionSourceResult{}, err
	}
	if len(result.Rows) > 1 {
		return VersionSourceResult{}, ErrInvalidEvidence
	}
	for _, r := range result.Rows {
		v := r[index["VERSION"]]
		if !v.Null {
			major, err := numericVersionMajor(v.Text)
			if err != nil || major != p.provenance.OracleMajor {
				return VersionSourceResult{}, ErrInvalidEvidence
			}
		}
	}
	rows := make([]VersionSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = VersionSourceRow{Value: r[index["VERSION"]]}
	}
	return VersionSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadVersionSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadVersionSource(ctx context.Context, reader Reader, request VersionSourceRequest) (VersionSourceResult, error) {
	p, err := versionSourceDatasetPlan(request.Options)
	if err != nil {
		return VersionSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return VersionSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return VersionSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return VersionSourceResult{}, err
	}
	facts, err := decodeDatasetVersionSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return VersionSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// VersionSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func VersionSourceRows(ctx context.Context, q rowset.Queryer, options VersionSourceOptions) (*sql.Rows, error) {
	p, err := versionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// VersionSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func VersionSourceJSON(ctx context.Context, q rowset.Queryer, options VersionSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := versionSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetVersionSource(r, p); return err })
}
