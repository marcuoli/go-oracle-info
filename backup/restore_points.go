package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RestorePointsOptions admits the observed release on the caller's connection.
type RestorePointsOptions struct {
	OracleVersion string
}

// RestorePointsRequest supplies the caller's statement reference and bounds.
type RestorePointsRequest struct {
	Options   RestorePointsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// RestorePoint preserves one visible restore point. Time is the recorded creation
// time. RestorePointTime, where available, is the source time associated with
// the point. Both are civil timestamp text with nine fractional digits.
// Nil pointers mark unprojected columns. Numeric values use exact integer
// text, including zero and SQL NULL. StorageSize is Oracle's approximate
// reclaimable byte estimate if this restore point is the oldest and is dropped. It is
// zero for a non-guaranteed restore point.
type RestorePoint struct {
	Name, SCN, DatabaseIncarnation, GuaranteeFlashbackDatabase, StorageSize, Time      session.Value
	RestorePointTime, Preserved, PDBRestorePoint, CleanPDBRestorePoint, PDBIncarnation *session.Value
	ContainerID, SourceContainerID                                                     *session.Value
}

// RestorePointsResult contains the selected recorded rows and provenance.
type RestorePointsResult struct {
	Provenance Provenance
	Rows       []RestorePoint
}

// RestorePointsStatement reads recorded points through the caller's admitted scope.
func RestorePointsStatement(options RestorePointsOptions) (string, error) {
	p, err := restorePointsPlan(options)
	return p.sql, err
}

func restorePointsPlan(options RestorePointsOptions) (selection, error) {
	n, parsed := oracleversion.Parse(options.OracleVersion)
	major, admitted := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !parsed || !admitted || major < 10 || (major == 10 && n[1] != 2) {
		return selection{}, ErrUnsupportedVersion
	}
	p := selection{provenance: Provenance{Source: "V$RESTORE_POINT", OracleMajor: major, Scope: "visible_restore_points"}}
	var fields []string
	add := func(name, expr string, kind rowset.Kind) {
		fields = append(fields, expr+" AS "+name)
		p.schema = append(p.schema, rowset.Column{Name: name, Kind: kind})
	}
	exact := func(name, expr string) {
		add(name, "TO_CHAR("+expr+", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')", rowset.Text)
	}
	add("NAME", "v.NAME", rowset.Text)
	exact("SCN", "v.SCN")
	exact("DATABASE_INCARNATION", "v.DATABASE_INCARNATION#")
	add("GUARANTEE_FLASHBACK_DATABASE", "v.GUARANTEE_FLASHBACK_DATABASE", rowset.Text)
	exact("STORAGE_SIZE", "v.STORAGE_SIZE")
	add("TIME", datetime.GregorianTimestamp9("v.TIME"), rowset.Text)
	if major >= 11 {
		add("RESTORE_POINT_TIME", datetime.GregorianTimestamp9("v.RESTORE_POINT_TIME"), rowset.Text)
		add("PRESERVED", "v.PRESERVED", rowset.Text)
	}
	if major >= 12 {
		add("CON_ID", "v.CON_ID", rowset.Integer)
		add("SOURCE_CON_ID", "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))", rowset.Integer)
	}
	if major > 12 || (major == 12 && n[1] == 2) {
		add("PDB_RESTORE_POINT", "v.PDB_RESTORE_POINT", rowset.Text)
		add("CLEAN_PDB_RESTORE_POINT", "v.CLEAN_PDB_RESTORE_POINT", rowset.Text)
		exact("PDB_INCARNATION", "v.PDB_INCARNATION#")
	}
	p.sql = "SELECT " + strings.Join(fields, ", ") + " FROM V$RESTORE_POINT v ORDER BY v.TIME, v.NAME, v.DATABASE_INCARNATION#, v.SCN"
	return p, nil
}

// DecodeRestorePoints validates the complete supplied projection.
func DecodeRestorePoints(result session.Result, options RestorePointsOptions) (RestorePointsResult, error) {
	p, err := restorePointsPlan(options)
	if err != nil {
		return RestorePointsResult{}, err
	}
	return decodeRestorePoints(result, p)
}

func decodeRestorePoints(result session.Result, p selection) (RestorePointsResult, error) {
	numbers := []string{"SCN", "DATABASE_INCARNATION", "STORAGE_SIZE"}
	for _, c := range p.schema {
		switch c.Name {
		case "PDB_INCARNATION", "CON_ID", "SOURCE_CON_ID":
			numbers = append(numbers, c.Name)
		}
	}
	index, err := columns(result, p, numbers...)
	if err != nil {
		return RestorePointsResult{}, err
	}
	getOptional := func(row []session.Value, key string) *session.Value {
		i, ok := index[key]
		if !ok {
			return nil
		}
		value := row[i]
		return &value
	}
	rows := make([]RestorePoint, len(result.Rows))
	for i, row := range result.Rows {
		v := RestorePoint{
			Name: row[index["NAME"]], SCN: row[index["SCN"]],
			DatabaseIncarnation:        row[index["DATABASE_INCARNATION"]],
			GuaranteeFlashbackDatabase: row[index["GUARANTEE_FLASHBACK_DATABASE"]],
			StorageSize:                row[index["STORAGE_SIZE"]], Time: row[index["TIME"]],
			RestorePointTime: getOptional(row, "RESTORE_POINT_TIME"), Preserved: getOptional(row, "PRESERVED"),
			ContainerID: getOptional(row, "CON_ID"), SourceContainerID: getOptional(row, "SOURCE_CON_ID"),
			PDBRestorePoint: getOptional(row, "PDB_RESTORE_POINT"), CleanPDBRestorePoint: getOptional(row, "CLEAN_PDB_RESTORE_POINT"),
			PDBIncarnation: getOptional(row, "PDB_INCARNATION"),
		}
		if v.Name.Null || v.Name.Text == "" || !validRestorePointTime(v.Time) ||
			(v.RestorePointTime != nil && !validRestorePointTime(*v.RestorePointTime)) ||
			(v.SourceContainerID != nil && v.SourceContainerID.Null) {
			return RestorePointsResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return RestorePointsResult{Provenance: p.provenance, Rows: rows}, nil
}

func validRestorePointTime(value session.Value) bool {
	if value.Null {
		return true
	}
	const layout = "2006-01-02T15:04:05.000000000"
	stamp, err := time.Parse(layout, value.Text)
	return err == nil && stamp.Format(layout) == value.Text
}

// ReadRestorePoints returns a complete bounded observation over an admitted reader.
func ReadRestorePoints(ctx context.Context, reader Reader, request RestorePointsRequest) (RestorePointsResult, error) {
	p, err := restorePointsPlan(request.Options)
	if err != nil {
		return RestorePointsResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return RestorePointsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return RestorePointsResult{}, err
	}
	if err := readError(ctx, p, nil); err != nil {
		return RestorePointsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := readError(ctx, p, err); err != nil {
		return RestorePointsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return RestorePointsResult{}, readError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeRestorePoints(result, p)
	if err := readError(ctx, p, err); err != nil {
		return RestorePointsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RestorePointsRows returns the actual cursor for the caller to consume and close.
func RestorePointsRows(ctx context.Context, q rowset.Queryer, options RestorePointsOptions) (*sql.Rows, error) {
	p, err := restorePointsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

// RestorePointsJSON returns one complete bounded dataset from a separate cursor.
func RestorePointsJSON(ctx context.Context, q rowset.Queryer, options RestorePointsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := restorePointsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeRestorePoints(r, p)
		return err
	})
}
