package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// DatabaseInfo retains source spelling, exact integer text and selected SQL NULL.
// Date/time labels are source-local civil values with no inferred timezone.
type DatabaseInfo struct {
	DBID            session.Value
	Name            session.Value
	DBUniqueName    session.Value
	LogMode         session.Value
	ControlfileType session.Value
	VersionTime     session.Value
	OpenMode        session.Value
	DatabaseRole    session.Value
	SupplementalMin session.Value
	SupplementalPK  session.Value
	SupplementalUI  session.Value
	ForceLogging    session.Value
	PlatformID      session.Value
	PlatformName    session.Value
	CurrentSCN      session.Value
	FlashbackOn     session.Value
	SupplementalFK  session.Value
	SupplementalAll session.Value
	SupplementalPL  *session.Value
}

// DatabaseInfoResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type DatabaseInfoResult struct {
	Provenance Provenance
	Rows       []DatabaseInfo
}

// DatabaseInfoRequest admits the release/capabilities and a valid caller-owned SQL reference.
type DatabaseInfoRequest struct {
	Options   DatabaseInfoOptions
	Reference reference.Reference
}

// DatabaseInfoStatement selects V$DATABASE. The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func DatabaseInfoStatement(options DatabaseInfoOptions) (string, error) {
	if options.OracleMajor < 11 {
		return "", ErrUnsupportedVersion
	}
	statement := `SELECT TO_CHAR(DBID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS DBID, NAME AS NAME, DB_UNIQUE_NAME AS DB_UNIQUE_NAME, LOG_MODE AS LOG_MODE, CONTROLFILE_TYPE AS CONTROLFILE_TYPE, TO_CHAR(VERSION_TIME,'YYYY-MM-DD"T"HH24:MI:SS') AS VERSION_TIME, OPEN_MODE AS OPEN_MODE, DATABASE_ROLE AS DATABASE_ROLE, SUPPLEMENTAL_LOG_DATA_MIN AS SUPPLEMENTAL_LOG_DATA_MIN, SUPPLEMENTAL_LOG_DATA_PK AS SUPPLEMENTAL_LOG_DATA_PK, SUPPLEMENTAL_LOG_DATA_UI AS SUPPLEMENTAL_LOG_DATA_UI, FORCE_LOGGING AS FORCE_LOGGING, TO_CHAR(PLATFORM_ID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS PLATFORM_ID, PLATFORM_NAME AS PLATFORM_NAME, TO_CHAR(CURRENT_SCN,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS CURRENT_SCN, FLASHBACK_ON AS FLASHBACK_ON, SUPPLEMENTAL_LOG_DATA_FK AS SUPPLEMENTAL_LOG_DATA_FK, SUPPLEMENTAL_LOG_DATA_ALL AS SUPPLEMENTAL_LOG_DATA_ALL`
	if options.IncludeSupplementalPL {
		statement += ", SUPPLEMENTAL_LOG_DATA_PL AS SUPPLEMENTAL_LOG_DATA_PL"
	}
	return statement + " FROM V$DATABASE", nil
}

func databaseInfoSchema(options DatabaseInfoOptions) []rowset.Column {
	schema := []rowset.Column{
		{Name: "DBID", Kind: rowset.Integer},
		{Name: "NAME", Kind: rowset.Text},
		{Name: "DB_UNIQUE_NAME", Kind: rowset.Text},
		{Name: "LOG_MODE", Kind: rowset.Text},
		{Name: "CONTROLFILE_TYPE", Kind: rowset.Text},
		{Name: "VERSION_TIME", Kind: rowset.Text},
		{Name: "OPEN_MODE", Kind: rowset.Text},
		{Name: "DATABASE_ROLE", Kind: rowset.Text},
		{Name: "SUPPLEMENTAL_LOG_DATA_MIN", Kind: rowset.Text},
		{Name: "SUPPLEMENTAL_LOG_DATA_PK", Kind: rowset.Text},
		{Name: "SUPPLEMENTAL_LOG_DATA_UI", Kind: rowset.Text},
		{Name: "FORCE_LOGGING", Kind: rowset.Text},
		{Name: "PLATFORM_ID", Kind: rowset.Integer},
		{Name: "PLATFORM_NAME", Kind: rowset.Text},
		{Name: "CURRENT_SCN", Kind: rowset.Text},
		{Name: "FLASHBACK_ON", Kind: rowset.Text},
		{Name: "SUPPLEMENTAL_LOG_DATA_FK", Kind: rowset.Text},
		{Name: "SUPPLEMENTAL_LOG_DATA_ALL", Kind: rowset.Text},
	}
	if options.IncludeSupplementalPL {
		schema = append(schema, rowset.Column{Name: "SUPPLEMENTAL_LOG_DATA_PL", Kind: rowset.Text})
	}
	return schema
}

func databaseInfoProvenance(options DatabaseInfoOptions) Provenance {
	p := Provenance{Source: "V$DATABASE", Scope: "controlfile_connected_database", OracleMajor: options.OracleMajor}
	if options.IncludeSupplementalPL {
		p.OptionalColumns = []string{"SUPPLEMENTAL_LOG_DATA_PL"}
	}
	return p
}

// DecodeDatabaseInfo validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeDatabaseInfo(result session.Result, options DatabaseInfoOptions) (DatabaseInfoResult, error) {
	if _, err := DatabaseInfoStatement(options); err != nil {
		return DatabaseInfoResult{}, err
	}
	index, err := factColumns(result, databaseInfoSchema(options))
	if err != nil {
		return DatabaseInfoResult{}, err
	}
	if len(result.Rows) > 1 {
		return DatabaseInfoResult{}, ErrInvalidEvidence
	}
	facts := make([]DatabaseInfo, 0, len(result.Rows))
	for _, row := range result.Rows {
		// Oracle SCNs are unsigned decimal identifiers, not signed64 counters.
		// Keep their exact text even beyond MaxInt64; JSON uses the same decoder.
		scn := row[index["CURRENT_SCN"]]
		if !scn.Null && !numeric.IsUnsignedInteger(scn.Text) {
			return DatabaseInfoResult{}, ErrInvalidEvidence
		}
		fact := DatabaseInfo{
			DBID:            row[index["DBID"]],
			Name:            row[index["NAME"]],
			DBUniqueName:    row[index["DB_UNIQUE_NAME"]],
			LogMode:         row[index["LOG_MODE"]],
			ControlfileType: row[index["CONTROLFILE_TYPE"]],
			VersionTime:     row[index["VERSION_TIME"]],
			OpenMode:        row[index["OPEN_MODE"]],
			DatabaseRole:    row[index["DATABASE_ROLE"]],
			SupplementalMin: row[index["SUPPLEMENTAL_LOG_DATA_MIN"]],
			SupplementalPK:  row[index["SUPPLEMENTAL_LOG_DATA_PK"]],
			SupplementalUI:  row[index["SUPPLEMENTAL_LOG_DATA_UI"]],
			ForceLogging:    row[index["FORCE_LOGGING"]],
			PlatformID:      row[index["PLATFORM_ID"]],
			PlatformName:    row[index["PLATFORM_NAME"]],
			CurrentSCN:      row[index["CURRENT_SCN"]],
			FlashbackOn:     row[index["FLASHBACK_ON"]],
			SupplementalFK:  row[index["SUPPLEMENTAL_LOG_DATA_FK"]],
			SupplementalAll: row[index["SUPPLEMENTAL_LOG_DATA_ALL"]],
		}
		if options.IncludeSupplementalPL {
			value := row[index["SUPPLEMENTAL_LOG_DATA_PL"]]
			fact.SupplementalPL = &value
		}
		facts = append(facts, fact)
	}

	return DatabaseInfoResult{Provenance: databaseInfoProvenance(options), Rows: facts}, nil
}

// ReadDatabaseInfo executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadDatabaseInfo(ctx context.Context, reader Reader, request DatabaseInfoRequest) (DatabaseInfoResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return DatabaseInfoResult{}, err
	}
	statement, err := DatabaseInfoStatement(request.Options)
	if err != nil {
		return DatabaseInfoResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return DatabaseInfoResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return DatabaseInfoResult{}, err
	}
	facts, err := DecodeDatabaseInfo(result, request.Options)
	if err = factReadError(ctx, err); err != nil {
		return DatabaseInfoResult{}, err
	}
	return facts, nil
}

// DatabaseInfoRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; DatabaseInfoJSON independently owns its cursor.
func DatabaseInfoRows(ctx context.Context, q rowset.Queryer, options DatabaseInfoOptions) (*sql.Rows, error) {
	statement, err := DatabaseInfoStatement(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// DatabaseInfoJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func DatabaseInfoJSON(ctx context.Context, q rowset.Queryer, options DatabaseInfoOptions, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return DatabaseInfoRows(ctx, q, options)
	}, databaseInfoSchema(options), limits, rowset.EnvelopeOptions{
		MaxRows:    1,
		Provenance: databaseInfoProvenance(options),
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := DecodeDatabaseInfo(result, options)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}
