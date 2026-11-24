package objects

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const subobjectStatisticsSQL = `SELECT TABLE_NAME AS TABLE_NAME, PARTITION_NAME AS PARTITION_NAME, SUBPARTITION_NAME AS SUBPARTITION_NAME, ` + statisticsValues + `
FROM DBA_TAB_STATISTICS
WHERE NLSSORT(OWNER, 'NLS_SORT=BINARY')=NLSSORT(:1, 'NLS_SORT=BINARY')
  AND OBJECT_TYPE IN ('PARTITION','SUBPARTITION')
ORDER BY NLSSORT(TABLE_NAME, 'NLS_SORT=BINARY'), NLSSORT(PARTITION_NAME, 'NLS_SORT=BINARY'), NLSSORT(SUBPARTITION_NAME, 'NLS_SORT=BINARY') NULLS FIRST`

// SubobjectStatisticsRow retains one dictionary row. Partition holds the parent
// name for a subpartition. A partition row has a NULL Subpartition. EstimatedRows
// and LastAnalyzed preserve raw optimizer statistics and SQL NULL.
type SubobjectStatisticsRow struct {
	Name          session.Value
	Partition     session.Value
	Subpartition  session.Value
	EstimatedRows session.Value
	LastAnalyzed  session.Value
}

// SubobjectStatisticsOptions selects one caller-admitted exact owner in the
// connected dictionary. OracleMajor is the observed numeric major. The caller
// intersects results with its admitted ordinary-table inventory.
type SubobjectStatisticsOptions struct {
	OracleMajor int
	Owner       string
}

// SubobjectStatisticsRequest supplies admitted options and a caller-owned SQL reference.
type SubobjectStatisticsRequest struct {
	Options   SubobjectStatisticsOptions
	Reference reference.Reference
}

// SubobjectStatisticsResult preserves the source order and duplicate rows.
// Complete records completion of the selected transport, not inventory coverage.
type SubobjectStatisticsResult struct {
	Provenance DatasetProvenance
	Rows       []SubobjectStatisticsRow
}

// SubobjectStatisticsStatement selects bound owner partition and subpartition
// statistics without a SQL row limit. It reads no table data and gathers no statistics.
func SubobjectStatisticsStatement(options SubobjectStatisticsOptions) (string, []any, error) {
	p, err := subobjectStatisticsDatasetPlan(options)
	return p.sql, p.args, err
}

func subobjectStatisticsDatasetPlan(options SubobjectStatisticsOptions) (datasetSelection, error) {
	switch options.OracleMajor {
	case 10, 11, 12, 18, 19, 21, 23:
	default:
		return datasetSelection{}, ErrUnsupportedVersion
	}
	if _, err := QuoteIdentifier(options.Owner); err != nil {
		return datasetSelection{}, ErrInvalidRequest
	}
	return datasetSelection{
		sql:  subobjectStatisticsSQL,
		args: []any{options.Owner},
		schema: []rowset.Column{
			{Name: "TABLE_NAME", Kind: rowset.Text},
			{Name: "PARTITION_NAME", Kind: rowset.Text},
			{Name: "SUBPARTITION_NAME", Kind: rowset.Text},
			{Name: "ROW_COUNT", Kind: rowset.Text},
			{Name: "LAST_ANALYZED", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{
			Source:      "DBA_TAB_STATISTICS",
			OracleMajor: options.OracleMajor,
			Scope:       "exact_owner_partition_subpartition_optimizer_estimates",
			Owner:       options.Owner,
		},
	}, nil
}

// DecodeSubobjectStatistics validates every row and its required table and
// partition identities. It does not establish transport completion.
func DecodeSubobjectStatistics(result session.Result, options SubobjectStatisticsOptions) (SubobjectStatisticsResult, error) {
	p, err := subobjectStatisticsDatasetPlan(options)
	if err != nil {
		return SubobjectStatisticsResult{}, err
	}
	return decodeSubobjectStatistics(result, p)
}

func decodeSubobjectStatistics(result session.Result, p datasetSelection) (SubobjectStatisticsResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return SubobjectStatisticsResult{}, err
	}
	rows := make([]SubobjectStatisticsRow, len(result.Rows))
	for i, r := range result.Rows {
		row := SubobjectStatisticsRow{
			Name:          r[index["TABLE_NAME"]],
			Partition:     r[index["PARTITION_NAME"]],
			Subpartition:  r[index["SUBPARTITION_NAME"]],
			EstimatedRows: r[index["ROW_COUNT"]],
			LastAnalyzed:  r[index["LAST_ANALYZED"]],
		}
		if _, err := QuoteIdentifier(row.Name.Text); err != nil {
			return SubobjectStatisticsResult{}, ErrInvalidEvidence
		}
		if _, err := QuoteIdentifier(row.Partition.Text); err != nil {
			return SubobjectStatisticsResult{}, ErrInvalidEvidence
		}
		if !row.Subpartition.Null {
			if _, err := QuoteIdentifier(row.Subpartition.Text); err != nil {
				return SubobjectStatisticsResult{}, ErrInvalidEvidence
			}
		}
		rows[i] = row
	}
	return SubobjectStatisticsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSubobjectStatistics executes through an admitted complete bounded reader.
// Query, close, cancellation and validation failures publish no successful prefix.
func ReadSubobjectStatistics(ctx context.Context, reader Reader, request SubobjectStatisticsRequest) (SubobjectStatisticsResult, error) {
	p, err := subobjectStatisticsDatasetPlan(request.Options)
	if err != nil {
		return SubobjectStatisticsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SubobjectStatisticsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SubobjectStatisticsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SubobjectStatisticsResult{}, err
	}
	facts, err := decodeSubobjectStatistics(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SubobjectStatisticsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SubobjectStatisticsRows returns the actual caller-native cursor. The caller
// owns iteration, close, affinity, cancellation and finite stream budgets.
func SubobjectStatisticsRows(ctx context.Context, q rowset.Queryer, options SubobjectStatisticsOptions) (*sql.Rows, error) {
	p, err := subobjectStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// SubobjectStatisticsJSON independently consumes and validates a complete
// cursor. Failure or budget overflow returns no JSON payload.
func SubobjectStatisticsJSON(ctx context.Context, q rowset.Queryer, options SubobjectStatisticsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := subobjectStatisticsDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeSubobjectStatistics(result, p)
		return err
	})
}
