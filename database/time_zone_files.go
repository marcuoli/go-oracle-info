package database

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

// TimeZoneFilesOptions admits the observed numeric release of the caller's handle.
type TimeZoneFilesOptions struct {
	OracleVersion string
}

// TimeZoneFilesRequest supplies the caller reference and buffered bounds.
type TimeZoneFilesRequest struct {
	Options   TimeZoneFilesOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// TimeZoneFile preserves the exact observed version. Filename and ContainerID are
// absent before the full 12.2 projection. SourceContainerID identifies the
// connected container on multitenant-capable releases.
type TimeZoneFile struct {
	Version                                  session.Value
	Filename, ContainerID, SourceContainerID *session.Value
}

// TimeZoneFilesResult contains the selected rows and their provenance.
type TimeZoneFilesResult struct {
	Provenance DatasetProvenance
	Rows       []TimeZoneFile
}

// TimeZoneFilesStatement selects the observed database time zone file metadata.
func TimeZoneFilesStatement(options TimeZoneFilesOptions) (string, error) {
	p, err := timeZoneFilesPlan(options)
	return p.sql, err
}

func timeZoneFilesPlan(options TimeZoneFilesOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 10 {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql:        "SELECT TO_CHAR(v.VERSION, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VERSION",
		schema:     []rowset.Column{{Name: "VERSION", Kind: rowset.Text}},
		provenance: DatasetProvenance{Source: "V$TIMEZONE_FILE", OracleMajor: major, Scope: "visible_database_timezone_files"},
	}
	n, _ := oracleversion.Parse(options.OracleVersion)
	full := major > 12 || (major == 12 && n[1] == 2)
	if full {
		p.sql += ", v.FILENAME AS FILENAME, v.CON_ID AS CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "FILENAME", Kind: rowset.Text}, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM V$TIMEZONE_FILE v"
	if full {
		p.sql += " ORDER BY v.CON_ID, v.VERSION, v.FILENAME"
	}
	return p, nil
}

// DecodeTimeZoneFiles validates all returned rows and retains SQL NULL.
func DecodeTimeZoneFiles(result session.Result, options TimeZoneFilesOptions) (TimeZoneFilesResult, error) {
	p, err := timeZoneFilesPlan(options)
	if err != nil {
		return TimeZoneFilesResult{}, err
	}
	return decodeTimeZoneFiles(result, p)
}

func decodeTimeZoneFiles(result session.Result, p datasetSelection) (TimeZoneFilesResult, error) {
	index, err := factColumns(result, p.schema)
	if err != nil {
		return TimeZoneFilesResult{}, err
	}
	rows := make([]TimeZoneFile, len(result.Rows))
	for i, row := range result.Rows {
		v := TimeZoneFile{Version: row[index["VERSION"]]}
		if !v.Version.Null && !numeric.IsUnsignedInteger(v.Version.Text) {
			return TimeZoneFilesResult{}, ErrInvalidEvidence
		}
		if j, ok := index["FILENAME"]; ok {
			name, container := row[j], row[index["CON_ID"]]
			if !container.Null && !numeric.IsUnsignedInteger(container.Text) {
				return TimeZoneFilesResult{}, ErrInvalidEvidence
			}
			v.Filename, v.ContainerID = &name, &container
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			source := row[j]
			if source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return TimeZoneFilesResult{}, ErrInvalidEvidence
			}
			v.SourceContainerID = &source
		}
		rows[i] = v
	}
	return TimeZoneFilesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTimeZoneFiles executes once over an admitted complete reader.
func ReadTimeZoneFiles(ctx context.Context, reader Reader, request TimeZoneFilesRequest) (TimeZoneFilesResult, error) {
	p, err := timeZoneFilesPlan(request.Options)
	if err != nil {
		return TimeZoneFilesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TimeZoneFilesResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return TimeZoneFilesResult{}, err
	}
	if err := datasetReadError(ctx, p, nil); err != nil {
		return TimeZoneFilesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := datasetReadError(ctx, p, err); err != nil {
		return TimeZoneFilesResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return TimeZoneFilesResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeTimeZoneFiles(result, p)
	if err := datasetReadError(ctx, p, err); err != nil {
		return TimeZoneFilesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TimeZoneFilesRows transfers the caller-native cursor.
func TimeZoneFilesRows(ctx context.Context, q rowset.Queryer, options TimeZoneFilesOptions) (*sql.Rows, error) {
	p, err := timeZoneFilesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// TimeZoneFilesJSON consumes a separate complete bounded cursor.
func TimeZoneFilesJSON(ctx context.Context, q rowset.Queryer, options TimeZoneFilesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := timeZoneFilesPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeTimeZoneFiles(r, p)
		return err
	})
}
