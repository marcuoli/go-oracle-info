package objects

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MaterializedViewsOptions admits the observed numeric version of the connected handle.
type MaterializedViewsOptions struct {
	OracleVersion string
}

// MaterializedViewsRequest supplies the caller reference and buffered bounds.
type MaterializedViewsRequest struct {
	Options   MaterializedViewsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// MaterializedView preserves Oracle's raw staleness and compile-state observations.
// SourceContainerID is absent before 12.1 and describes the connected container.
type MaterializedView struct {
	Owner             session.Value
	Name              session.Value
	Staleness         session.Value
	CompileState      session.Value
	SourceContainerID *session.Value
}

// MaterializedViewsResult contains the selected rows and their connected-dictionary provenance.
type MaterializedViewsResult struct {
	Provenance DatasetProvenance
	Rows       []MaterializedView
}

// MaterializedViewsStatement selects the reviewed DBA_MVIEWS projection.
func MaterializedViewsStatement(options MaterializedViewsOptions) (string, error) {
	p, err := materializedViewsPlan(options)
	return p.sql, err
}

func materializedViewsPlan(options MaterializedViewsOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.OWNER AS OWNER" +
			", v.MVIEW_NAME AS MVIEW_NAME" +
			", v.STALENESS AS STALENESS" +
			", v.COMPILE_STATE AS COMPILE_STATE",
		schema: []rowset.Column{
			{Name: "OWNER", Kind: rowset.Text},
			{Name: "MVIEW_NAME", Kind: rowset.Text},
			{Name: "STALENESS", Kind: rowset.Text},
			{Name: "COMPILE_STATE", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "DBA_MVIEWS", OracleMajor: major, Scope: "connected_dictionary"},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DBA_MVIEWS v"
	return p, nil
}

// DecodeMaterializedViews validates every row and preserves source text and SQL NULL.
func DecodeMaterializedViews(result session.Result, options MaterializedViewsOptions) (MaterializedViewsResult, error) {
	p, err := materializedViewsPlan(options)
	if err != nil {
		return MaterializedViewsResult{}, err
	}
	return decodeMaterializedViews(result, p)
}

func decodeMaterializedViews(result session.Result, p datasetSelection) (MaterializedViewsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return MaterializedViewsResult{}, err
	}
	rows := make([]MaterializedView, len(result.Rows))
	for i, row := range result.Rows {
		v := MaterializedView{
			Owner:        row[index["OWNER"]],
			Name:         row[index["MVIEW_NAME"]],
			Staleness:    row[index["STALENESS"]],
			CompileState: row[index["COMPILE_STATE"]],
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			source := row[j]
			if source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return MaterializedViewsResult{}, ErrInvalidEvidence
			}
			v.SourceContainerID = &source
		}
		rows[i] = v
	}
	return MaterializedViewsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadMaterializedViews executes once over an admitted complete reader.
func ReadMaterializedViews(ctx context.Context, reader Reader, request MaterializedViewsRequest) (MaterializedViewsResult, error) {
	p, err := materializedViewsPlan(request.Options)
	if err != nil {
		return MaterializedViewsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return MaterializedViewsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return MaterializedViewsResult{}, err
	}
	if err := datasetReadError(ctx, p, nil); err != nil {
		return MaterializedViewsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := datasetReadError(ctx, p, err); err != nil {
		return MaterializedViewsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return MaterializedViewsResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeMaterializedViews(result, p)
	if err := datasetReadError(ctx, p, err); err != nil {
		return MaterializedViewsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// MaterializedViewsRows transfers the caller-native cursor.
func MaterializedViewsRows(ctx context.Context, q rowset.Queryer, options MaterializedViewsOptions) (*sql.Rows, error) {
	p, err := materializedViewsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// MaterializedViewsJSON consumes a separate complete bounded cursor.
func MaterializedViewsJSON(ctx context.Context, q rowset.Queryer, options MaterializedViewsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := materializedViewsPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeMaterializedViews(r, p)
		return err
	})
}
