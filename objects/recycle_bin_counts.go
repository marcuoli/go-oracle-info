package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RecycleBinCountsOptions admits the observed numeric version of the connected handle.
type RecycleBinCountsOptions struct {
	OracleVersion string
}

// RecycleBinCountsRequest supplies the caller reference and buffered bounds.
type RecycleBinCountsRequest struct {
	Options   RecycleBinCountsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// RecycleBinCount preserves the visible entry count for an owner. Oracle may
// report dependent objects as additional entries for one dropped table.
// SourceContainerID is absent before 12.1 and describes the connected container.
type RecycleBinCount struct {
	Owner             session.Value
	Count             session.Value
	SourceContainerID *session.Value
}

// RecycleBinCountsResult contains the selected rows and their connected-dictionary provenance.
type RecycleBinCountsResult struct {
	Provenance DatasetProvenance
	Rows       []RecycleBinCount
}

// RecycleBinCountsStatement selects the reviewed DBA_RECYCLEBIN projection.
func RecycleBinCountsStatement(options RecycleBinCountsOptions) (string, error) {
	p, err := recycleBinCountsPlan(options)
	return p.sql, err
}

func recycleBinCountsPlan(options RecycleBinCountsOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 10 {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.OWNER AS OWNER" +
			", " + "TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OBJECT_COUNT",
		schema: []rowset.Column{
			{Name: "OWNER", Kind: rowset.Text},
			{Name: "OBJECT_COUNT", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "DBA_RECYCLEBIN", OracleMajor: major, Scope: "connected_dictionary"},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DBA_RECYCLEBIN v"
	// Binary identity prevents name coalescing under a linguistic session sort.
	p.sql += " GROUP BY v.OWNER, NLSSORT(v.OWNER, 'NLS_SORT=BINARY')"
	return p, nil
}

// DecodeRecycleBinCounts validates every row and preserves source text and SQL NULL.
func DecodeRecycleBinCounts(result session.Result, options RecycleBinCountsOptions) (RecycleBinCountsResult, error) {
	p, err := recycleBinCountsPlan(options)
	if err != nil {
		return RecycleBinCountsResult{}, err
	}
	return decodeRecycleBinCounts(result, p)
}

func decodeRecycleBinCounts(result session.Result, p datasetSelection) (RecycleBinCountsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return RecycleBinCountsResult{}, err
	}
	rows := make([]RecycleBinCount, len(result.Rows))
	owners := make(map[string]struct{}, len(result.Rows))
	for i, row := range result.Rows {
		v := RecycleBinCount{
			Owner: row[index["OWNER"]],
			Count: row[index["OBJECT_COUNT"]],
		}
		// The query emits exactly one positive count for each exact owner group.
		if v.Owner.Text == "" || !numeric.IsUnsignedInteger(v.Count.Text) || !strings.ContainsAny(v.Count.Text, "123456789") {
			return RecycleBinCountsResult{}, ErrInvalidEvidence
		}
		if _, exists := owners[v.Owner.Text]; exists {
			return RecycleBinCountsResult{}, ErrInvalidEvidence
		}
		owners[v.Owner.Text] = struct{}{}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			source := row[j]
			if source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return RecycleBinCountsResult{}, ErrInvalidEvidence
			}
			v.SourceContainerID = &source
		}
		rows[i] = v
	}
	return RecycleBinCountsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRecycleBinCounts executes once over an admitted complete reader.
func ReadRecycleBinCounts(ctx context.Context, reader Reader, request RecycleBinCountsRequest) (RecycleBinCountsResult, error) {
	p, err := recycleBinCountsPlan(request.Options)
	if err != nil {
		return RecycleBinCountsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return RecycleBinCountsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return RecycleBinCountsResult{}, err
	}
	if err := datasetReadError(ctx, p, nil); err != nil {
		return RecycleBinCountsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := datasetReadError(ctx, p, err); err != nil {
		return RecycleBinCountsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return RecycleBinCountsResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeRecycleBinCounts(result, p)
	if err := datasetReadError(ctx, p, err); err != nil {
		return RecycleBinCountsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RecycleBinCountsRows transfers the caller-native cursor.
func RecycleBinCountsRows(ctx context.Context, q rowset.Queryer, options RecycleBinCountsOptions) (*sql.Rows, error) {
	p, err := recycleBinCountsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// RecycleBinCountsJSON consumes a separate complete bounded cursor.
func RecycleBinCountsJSON(ctx context.Context, q rowset.Queryer, options RecycleBinCountsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := recycleBinCountsPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeRecycleBinCounts(r, p)
		return err
	})
}
