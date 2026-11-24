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

// TimeZoneColumnsOptions admits the observed numeric release of the handle.
type TimeZoneColumnsOptions struct{ OracleVersion string }

// TimeZoneColumnsRequest supplies caller admission, a reference and read bounds.
type TimeZoneColumnsRequest struct {
	Options   TimeZoneColumnsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// TimeZoneColumn preserves the declared Oracle type text, including precision.
// TableName is the dictionary's object name. ColumnID can be NULL for invisible
// columns. SourceContainerID is absent on pre-multitenant releases.
type TimeZoneColumn struct {
	Owner, TableName, ColumnName, DataType, ColumnID session.Value
	SourceContainerID                                *session.Value
}

// TimeZoneColumnsResult retains rows visible in the connected dictionary.
type TimeZoneColumnsResult struct {
	Provenance DatasetProvenance
	Rows       []TimeZoneColumn
}

// TimeZoneColumnsStatement selects the two builtin time-zone-aware column types.
func TimeZoneColumnsStatement(options TimeZoneColumnsOptions) (string, error) {
	p, err := timeZoneColumnsPlan(options)
	return p.sql, err
}

func timeZoneColumnsPlan(options TimeZoneColumnsOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.OWNER AS OWNER, v.TABLE_NAME AS TABLE_NAME, v.COLUMN_NAME AS COLUMN_NAME, v.DATA_TYPE AS DATA_TYPE, v.COLUMN_ID AS COLUMN_ID",
		schema: []rowset.Column{
			{Name: "OWNER", Kind: rowset.Text},
			{Name: "TABLE_NAME", Kind: rowset.Text},
			{Name: "COLUMN_NAME", Kind: rowset.Text},
			{Name: "DATA_TYPE", Kind: rowset.Text},
			{Name: "COLUMN_ID", Kind: rowset.Integer},
		},
		provenance: DatasetProvenance{Source: "DBA_TAB_COLUMNS", Scope: "connected_dictionary", OracleMajor: major},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	// Oracle can embed fractional precision in DATA_TYPE. LIKE and SUBSTR
	// cover that spelling on releases before regular expressions were available.
	p.sql += ` FROM SYS.DBA_TAB_COLUMNS v
WHERE v.DATA_TYPE_OWNER IS NULL
  AND (v.DATA_TYPE IN ('TIMESTAMP WITH TIME ZONE', 'TIMESTAMP WITH LOCAL TIME ZONE')
    OR (SUBSTR(v.DATA_TYPE, 11, 1) IN ('0','1','2','3','4','5','6','7','8','9')
      AND (v.DATA_TYPE LIKE 'TIMESTAMP(_) WITH TIME ZONE' OR v.DATA_TYPE LIKE 'TIMESTAMP(_) WITH LOCAL TIME ZONE')))
ORDER BY v.OWNER, v.TABLE_NAME, v.COLUMN_ID, v.COLUMN_NAME`
	return p, nil
}

// DecodeTimeZoneColumns validates all supplied rows before publishing the list.
func DecodeTimeZoneColumns(result session.Result, options TimeZoneColumnsOptions) (TimeZoneColumnsResult, error) {
	p, err := timeZoneColumnsPlan(options)
	if err != nil {
		return TimeZoneColumnsResult{}, err
	}
	return decodeTimeZoneColumns(result, p)
}

func decodeTimeZoneColumns(result session.Result, p datasetSelection) (TimeZoneColumnsResult, error) {
	index, err := datasetColumns(result, p, "COLUMN_ID")
	if err != nil {
		return TimeZoneColumnsResult{}, err
	}
	rows := make([]TimeZoneColumn, len(result.Rows))
	for i, row := range result.Rows {
		item := TimeZoneColumn{
			Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]],
			ColumnName: row[index["COLUMN_NAME"]], DataType: row[index["DATA_TYPE"]],
			ColumnID: row[index["COLUMN_ID"]],
		}
		if item.Owner.Text == "" || item.TableName.Text == "" || item.ColumnName.Text == "" || !isTimeZoneColumnType(item.DataType.Text) {
			return TimeZoneColumnsResult{}, ErrInvalidEvidence
		}
		if pos, ok := index["SOURCE_CON_ID"]; ok {
			v := row[pos]
			// TypedColumns checks integer syntax. Oracle uses nonnegative
			// container IDs, including zero for a non-CDB connection.
			if v.Null || !numeric.IsUnsignedInteger(v.Text) {
				return TimeZoneColumnsResult{}, ErrInvalidEvidence
			}
			item.SourceContainerID = &v
		}
		rows[i] = item
	}
	return TimeZoneColumnsResult{Provenance: p.provenance, Rows: rows}, nil
}

func isTimeZoneColumnType(value string) bool {
	suffix, ok := strings.CutPrefix(value, "TIMESTAMP")
	if !ok {
		return false
	}
	if len(suffix) >= 3 && suffix[0] == '(' && suffix[1] >= '0' && suffix[1] <= '9' && suffix[2] == ')' {
		suffix = suffix[3:]
	}
	return suffix == " WITH TIME ZONE" || suffix == " WITH LOCAL TIME ZONE"
}

// ReadTimeZoneColumns executes once over the caller's admitted bounded reader.
func ReadTimeZoneColumns(ctx context.Context, reader Reader, request TimeZoneColumnsRequest) (TimeZoneColumnsResult, error) {
	p, err := timeZoneColumnsPlan(request.Options)
	if err != nil {
		return TimeZoneColumnsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TimeZoneColumnsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return TimeZoneColumnsResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TimeZoneColumnsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TimeZoneColumnsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return TimeZoneColumnsResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeTimeZoneColumns(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return TimeZoneColumnsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TimeZoneColumnsRows transfers the actual native cursor to its caller.
func TimeZoneColumnsRows(ctx context.Context, q rowset.Queryer, options TimeZoneColumnsOptions) (*sql.Rows, error) {
	p, err := timeZoneColumnsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// TimeZoneColumnsJSON consumes an independent complete bounded cursor.
func TimeZoneColumnsJSON(ctx context.Context, q rowset.Queryer, options TimeZoneColumnsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := timeZoneColumnsPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeTimeZoneColumns(r, p)
		return err
	})
}
