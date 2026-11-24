package objects

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ErrSourceInaccessible marks a privilege rejection by Oracle.
var ErrSourceInaccessible = projection.ErrSourceInaccessible

// ErrSourceUnavailable marks an unresolved or unavailable Oracle source.
var ErrSourceUnavailable = projection.ErrSourceUnavailable

// EncryptedColumnsOptions carries the observed release of the admitted dictionary.
type EncryptedColumnsOptions struct{ OracleVersion string }

// EncryptedColumnsRequest supplies a caller reference and buffered result bounds.
type EncryptedColumnsRequest struct {
	Options   EncryptedColumnsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// EncryptedColumn retains the dictionary identity. It contains no column data,
// encryption key material or keystore observation.
type EncryptedColumn struct {
	Owner, TableName, ColumnName session.Value
}

// EncryptedColumnsResult retains connected-dictionary rows and provenance.
type EncryptedColumnsResult struct {
	Provenance DatasetProvenance
	Rows       []EncryptedColumn
}

// EncryptedColumnsStatement selects column identities from the connected dictionary.
func EncryptedColumnsStatement(options EncryptedColumnsOptions) (string, error) {
	plan, err := encryptedColumnsPlan(options)
	return plan.sql, err
}

func encryptedColumnsPlan(options EncryptedColumnsOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	version, _ := oracleversion.Parse(options.OracleVersion)
	if !ok || major < 10 || (major == 10 && version[1] < 2) {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	return datasetSelection{
		sql: "SELECT v.OWNER AS OWNER, v.TABLE_NAME AS TABLE_NAME, v.COLUMN_NAME AS COLUMN_NAME " +
			"FROM SYS.DBA_ENCRYPTED_COLUMNS v ORDER BY v.OWNER, v.TABLE_NAME, v.COLUMN_NAME",
		schema:     []rowset.Column{{Name: "OWNER", Kind: rowset.Text}, {Name: "TABLE_NAME", Kind: rowset.Text}, {Name: "COLUMN_NAME", Kind: rowset.Text}},
		provenance: DatasetProvenance{Source: "DBA_ENCRYPTED_COLUMNS", OracleMajor: major, Scope: "connected_dictionary"},
	}, nil
}

// DecodeEncryptedColumns validates all supplied dictionary observations.
func DecodeEncryptedColumns(result session.Result, options EncryptedColumnsOptions) (EncryptedColumnsResult, error) {
	plan, err := encryptedColumnsPlan(options)
	if err != nil {
		return EncryptedColumnsResult{}, err
	}
	return decodeEncryptedColumns(result, plan)
}

func decodeEncryptedColumns(result session.Result, plan datasetSelection) (EncryptedColumnsResult, error) {
	index, err := datasetColumns(result, plan)
	if err != nil {
		return EncryptedColumnsResult{}, err
	}
	rows := make([]EncryptedColumn, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = EncryptedColumn{Owner: row[index["OWNER"]], TableName: row[index["TABLE_NAME"]], ColumnName: row[index["COLUMN_NAME"]]}
	}
	return EncryptedColumnsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadEncryptedColumns executes once on an admitted complete bounded reader.
func ReadEncryptedColumns(ctx context.Context, reader Reader, request EncryptedColumnsRequest) (EncryptedColumnsResult, error) {
	plan, err := encryptedColumnsPlan(request.Options)
	if err != nil {
		return EncryptedColumnsResult{}, err
	}
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return EncryptedColumnsResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return EncryptedColumnsResult{}, err
	}
	if err := datasetReadError(ctx, plan, nil); err != nil {
		return EncryptedColumnsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql})
	if err := datasetReadError(ctx, plan, err); err != nil {
		return EncryptedColumnsResult{}, projection.ClassifySourceError(err)
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return EncryptedColumnsResult{}, datasetReadError(ctx, plan, rowset.ErrCapacity)
	}
	facts, err := decodeEncryptedColumns(result, plan)
	if err := datasetReadError(ctx, plan, err); err != nil {
		return EncryptedColumnsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// EncryptedColumnsRows transfers the actual caller-native cursor.
func EncryptedColumnsRows(ctx context.Context, q rowset.Queryer, options EncryptedColumnsOptions) (*sql.Rows, error) {
	plan, err := encryptedColumnsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, projection.ClassifySourceError(&DatasetError{plan.provenance.Source, err})
	}
	return rows, nil
}

// EncryptedColumnsJSON consumes a separate complete bounded cursor.
func EncryptedColumnsJSON(ctx context.Context, q rowset.Queryer, options EncryptedColumnsOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := encryptedColumnsPlan(options)
	if err != nil {
		return nil, err
	}
	payload, err := readDatasetJSON(ctx, q, plan, limits, func(r session.Result) error {
		_, err := decodeEncryptedColumns(r, plan)
		return err
	})
	return payload, projection.ClassifySourceError(err)
}
