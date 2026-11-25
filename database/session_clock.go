package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SessionClockOptions admits the observed numeric release of the connected session.
type SessionClockOptions struct{ OracleVersion string }

// SessionClockRequest supplies caller admission, a SQL reference, and buffered bounds.
type SessionClockRequest struct {
	Options   SessionClockOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// SessionClockRow keeps the civil labels, the two UTC instants, and Oracle's raw
// SESSIONTIMEZONE text. Projected SQL NULL remains distinct from an empty result.
type SessionClockRow struct {
	SysDate             session.Value
	SysTimestampUTC     session.Value
	CurrentDate         session.Value
	CurrentTimestampUTC session.Value
	LocalTimestamp      session.Value
	SessionTimeZone     session.Value
}

// SessionClockResult contains at most one row from one admitted connection.
type SessionClockResult struct {
	Provenance DatasetProvenance
	Rows       []SessionClockRow
}

const (
	sessionClockDateLayout      = "2006-01-02T15:04:05"
	sessionClockTimestampLayout = sessionClockDateLayout + ".000000000"
	sessionClockUTCLayout       = sessionClockTimestampLayout + "Z"
)

// SessionClockStatement selects a connected-session observation without changing
// session formatting, calendar, or time zone.
func SessionClockStatement(options SessionClockOptions) (string, error) {
	p, err := sessionClockPlan(options)
	return p.sql, err
}

func sessionClockPlan(options SessionClockOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	return datasetSelection{
		sql: "SELECT " + datetime.GregorianDate("SYSDATE") + " AS \"SYSDATE\", " +
			datetime.GregorianUTC9("SYSTIMESTAMP") + " AS SYSTIMESTAMP_UTC, " +
			datetime.GregorianDate("CURRENT_DATE") + " AS \"CURRENT_DATE\", " +
			datetime.GregorianUTC9("CURRENT_TIMESTAMP") + " AS CURRENT_TIMESTAMP_UTC, " +
			datetime.GregorianTimestamp9("LOCALTIMESTAMP") + " AS \"LOCALTIMESTAMP\", " +
			"SESSIONTIMEZONE AS SESSION_TIME_ZONE FROM SYS.DUAL",
		schema: []rowset.Column{
			{Name: "SYSDATE", Kind: rowset.Text},
			{Name: "SYSTIMESTAMP_UTC", Kind: rowset.Text},
			{Name: "CURRENT_DATE", Kind: rowset.Text},
			{Name: "CURRENT_TIMESTAMP_UTC", Kind: rowset.Text},
			{Name: "LOCALTIMESTAMP", Kind: rowset.Text},
			{Name: "SESSION_TIME_ZONE", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "SYS.DUAL", OracleMajor: major, Scope: "connected_session_clock"},
	}, nil
}

// DecodeSessionClock checks the complete value shape without asserting cursor completion.
func DecodeSessionClock(result session.Result, options SessionClockOptions) (SessionClockResult, error) {
	p, err := sessionClockPlan(options)
	if err != nil {
		return SessionClockResult{}, err
	}
	return decodeSessionClock(result, p)
}

func decodeSessionClock(result session.Result, p datasetSelection) (SessionClockResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil || len(result.Rows) > 1 {
		return SessionClockResult{}, ErrInvalidEvidence
	}
	rows := make([]SessionClockRow, len(result.Rows))
	for i, row := range result.Rows {
		clock := SessionClockRow{
			SysDate:             row[index["SYSDATE"]],
			SysTimestampUTC:     row[index["SYSTIMESTAMP_UTC"]],
			CurrentDate:         row[index["CURRENT_DATE"]],
			CurrentTimestampUTC: row[index["CURRENT_TIMESTAMP_UTC"]],
			LocalTimestamp:      row[index["LOCALTIMESTAMP"]],
			SessionTimeZone:     row[index["SESSION_TIME_ZONE"]],
		}
		if !validSessionClockTime(clock.SysDate, sessionClockDateLayout) ||
			!validSessionClockTime(clock.SysTimestampUTC, sessionClockUTCLayout) ||
			!validSessionClockTime(clock.CurrentDate, sessionClockDateLayout) ||
			!validSessionClockTime(clock.CurrentTimestampUTC, sessionClockUTCLayout) ||
			!validSessionClockTime(clock.LocalTimestamp, sessionClockTimestampLayout) {
			return SessionClockResult{}, ErrInvalidEvidence
		}
		rows[i] = clock
	}
	return SessionClockResult{Provenance: p.provenance, Rows: rows}, nil
}

func validSessionClockTime(value session.Value, layout string) bool {
	if value.Null {
		return true
	}
	parsed, err := time.Parse(layout, value.Text)
	return err == nil && parsed.Year() > 0 && parsed.Format(layout) == value.Text
}

// ReadSessionClock executes one bounded SELECT on the caller's admitted reader.
func ReadSessionClock(ctx context.Context, reader Reader, request SessionClockRequest) (SessionClockResult, error) {
	p, err := sessionClockPlan(request.Options)
	if err != nil {
		return SessionClockResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SessionClockResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return SessionClockResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SessionClockResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionClockResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return SessionClockResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeSessionClock(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return SessionClockResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SessionClockRows transfers the actual native cursor to its caller.
func SessionClockRows(ctx context.Context, q rowset.Queryer, options SessionClockOptions) (*sql.Rows, error) {
	p, err := sessionClockPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// SessionClockJSON consumes a separate complete bounded native cursor.
func SessionClockJSON(ctx context.Context, q rowset.Queryer, options SessionClockOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sessionClockPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeSessionClock(result, p)
		return err
	})
}
