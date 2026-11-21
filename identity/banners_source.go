package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// BannersSourceRow preserves source values and SQL NULL without application policy.
type BannersSourceRow struct {
	Banner session.Value
	Full   session.Value
}

// BannersSourceOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type BannersSourceOptions struct{ OracleMajor int }

// BannersSourceRequest supplies admitted options and a caller-owned SQL reference.
type BannersSourceRequest struct {
	Options   BannersSourceOptions
	Reference reference.Reference
}

// BannersSourceResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type BannersSourceResult struct {
	Provenance DatasetProvenance
	Rows       []BannersSourceRow
}

// BannersSourceStatement selects reviewed SQL with bound values, owning no connection.
func BannersSourceStatement(options BannersSourceOptions) (string, []any, error) {
	p, err := bannersSourceDatasetPlan(options)
	return p.sql, p.args, err
}

func bannersSourceDatasetPlan(options BannersSourceOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query, err := BannersStatement(options.OracleMajor)
	if err != nil {
		return datasetSelection{}, err
	}
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "BANNER", Kind: rowset.Text}, {Name: "BANNER_FULL", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$VERSION", OracleMajor: options.OracleMajor, Scope: "connected_version_banners_unmodified"}}, nil
}

// DecodeBannersSource validates the entire shape and every row before returning facts.
func DecodeBannersSource(result session.Result, options BannersSourceOptions) (BannersSourceResult, error) {
	p, err := bannersSourceDatasetPlan(options)
	if err != nil {
		return BannersSourceResult{}, err
	}
	return decodeDatasetBannersSource(result, p)
}

func decodeDatasetBannersSource(result session.Result, p datasetSelection) (BannersSourceResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return BannersSourceResult{}, err
	}
	rows := make([]BannersSourceRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = BannersSourceRow{Banner: r[index["BANNER"]], Full: r[index["BANNER_FULL"]]}
	}
	return BannersSourceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadBannersSource executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadBannersSource(ctx context.Context, reader Reader, request BannersSourceRequest) (BannersSourceResult, error) {
	p, err := bannersSourceDatasetPlan(request.Options)
	if err != nil {
		return BannersSourceResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return BannersSourceResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return BannersSourceResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return BannersSourceResult{}, err
	}
	facts, err := decodeDatasetBannersSource(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return BannersSourceResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// BannersSourceRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func BannersSourceRows(ctx context.Context, q rowset.Queryer, options BannersSourceOptions) (*sql.Rows, error) {
	p, err := bannersSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// BannersSourceJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func BannersSourceJSON(ctx context.Context, q rowset.Queryer, options BannersSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := bannersSourceDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetBannersSource(r, p); return err })
}

// VersionBanner retains one complete V$VERSION row without trimming multiline text.
// Full is nil when BANNER_FULL was not selected on a legacy source. A non-nil Full
// with State Null represents a selected SQL NULL, not an unsupported column.
type VersionBanner struct {
	Banner Text
	Full   *Text
}

// BannersStatement selects reviewed version columns on an established Oracle 11+
// source. BANNER_FULL is selected only for 18+; no release-update text is invented
// for older releases. This reads the connected view, not every RAC instance.
func BannersStatement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	columns := "BANNER AS BANNER"
	if major >= 18 {
		columns += ", BANNER_FULL AS BANNER_FULL"
	}
	return "SELECT " + columns + " FROM V$VERSION ORDER BY BANNER", nil
}

// DecodeBanners accepts only complete results for BannersStatement's projection.
// It preserves duplicates, source order, NULLs and all banner bytes. Empty results
// are authoritative no rows only after the caller's execution/close checks succeed.
// A malformed row invalidates the inventory; no truncated prefix is returned.
func DecodeBanners(result session.Result, major int) ([]VersionBanner, error) {
	if _, err := BannersStatement(major); err != nil {
		return nil, err
	}
	names := []string{"BANNER"}
	if major >= 18 {
		names = append(names, "BANNER_FULL")
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	facts := make([]VersionBanner, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := VersionBanner{Banner: text(row[index["BANNER"]])}
		if major >= 18 {
			full := text(row[index["BANNER_FULL"]])
			fact.Full = &full
		}
		facts = append(facts, fact)
	}
	return facts, nil
}
