package database

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

// TimeZoneOptions admits the observed numeric release of the connected endpoint.
type TimeZoneOptions struct{ OracleVersion string }

// TimeZoneRequest supplies caller admission, a SQL reference, and buffered bounds.
type TimeZoneRequest struct {
	Options   TimeZoneOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// TimeZoneRow preserves the raw DBTIMEZONE offset or region, including SQL NULL.
type TimeZoneRow struct{ Value session.Value }

// TimeZoneResult contains at most one row from the connected endpoint.
type TimeZoneResult struct {
	Provenance DatasetProvenance
	Rows       []TimeZoneRow
}

// TimeZoneStatement selects the database time zone visible on the admitted handle.
func TimeZoneStatement(options TimeZoneOptions) (string, error) {
	p, err := timeZonePlan(options)
	return p.sql, err
}

func timeZonePlan(options TimeZoneOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	return datasetSelection{
		// Qualify DUAL so caller schemas cannot shadow the singleton source.
		sql:        "SELECT DBTIMEZONE AS DB_TIME_ZONE FROM SYS.DUAL",
		schema:     []rowset.Column{{Name: "DB_TIME_ZONE", Kind: rowset.Text}},
		provenance: DatasetProvenance{Source: "DBTIMEZONE", OracleMajor: major, Scope: "connected_database_time_zone"},
	}, nil
}

// DecodeTimeZone validates the full projection and retains empty results and NULL.
func DecodeTimeZone(result session.Result, options TimeZoneOptions) (TimeZoneResult, error) {
	p, err := timeZonePlan(options)
	if err != nil {
		return TimeZoneResult{}, err
	}
	return decodeTimeZone(result, p)
}

func decodeTimeZone(result session.Result, p datasetSelection) (TimeZoneResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil || len(result.Rows) > 1 {
		return TimeZoneResult{}, ErrInvalidEvidence
	}
	rows := make([]TimeZoneRow, len(result.Rows))
	for i, row := range result.Rows {
		rows[i].Value = row[index["DB_TIME_ZONE"]]
	}
	return TimeZoneResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTimeZone reads one bounded result through the caller's admitted reader.
func ReadTimeZone(ctx context.Context, reader Reader, request TimeZoneRequest) (TimeZoneResult, error) {
	p, err := timeZonePlan(request.Options)
	if err != nil {
		return TimeZoneResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return TimeZoneResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return TimeZoneResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return TimeZoneResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err = datasetReadError(ctx, p, err); err != nil {
		return TimeZoneResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return TimeZoneResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeTimeZone(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return TimeZoneResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TimeZoneRows transfers the actual native cursor to its caller.
func TimeZoneRows(ctx context.Context, q rowset.Queryer, options TimeZoneOptions) (*sql.Rows, error) {
	p, err := timeZonePlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// TimeZoneJSON consumes a separate complete bounded cursor.
func TimeZoneJSON(ctx context.Context, q rowset.Queryer, options TimeZoneOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := timeZonePlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeTimeZone(r, p)
		return err
	})
}
