package asm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const datafileStorageHardwareSource = "V$ASM_DISK_STAT"

const datafileStorageHardwareSQL = `SELECT CASE WHEN COUNT(*) > 0 THEN 'EXADATA_STORAGE' ELSE 'UNKNOWN' END AS HARDWARE_TYPE,
       COUNT(*) AS EXADATA_DISK_COUNT
FROM V$ASM_DISK_STAT d
JOIN V$ASM_DISKGROUP_STAT g ON g.GROUP_NUMBER = d.GROUP_NUMBER
WHERE d.GROUP_NUMBER > 0
  AND d.PATH LIKE 'o/%'
  AND EXISTS (
      SELECT 1
      FROM V$DATAFILE f
      WHERE UPPER(SUBSTR(f.NAME, 1, LENGTH(g.NAME) + 2)) = '+' || UPPER(g.NAME) || '/'
  )`

// StorageHardwareType identifies hardware supported by visible storage evidence.
type StorageHardwareType string

const (
	// StorageHardwareUnknown means no matching Exadata grid disks were visible.
	StorageHardwareUnknown StorageHardwareType = "UNKNOWN"
	// StorageHardwareExadata means an Exadata grid disk was visible in a
	// disk group referenced by the database's recorded datafiles.
	StorageHardwareExadata StorageHardwareType = "EXADATA_STORAGE"
)

// DatafileStorageHardwareRequest supplies the observed numeric version, a
// caller-owned SQL reference and the buffered result limits.
type DatafileStorageHardwareRequest struct {
	OracleVersion string
	Reference     reference.Reference
	Limits        rowset.Limits
}

// DatafileStorageHardwareResult preserves the exact count of visible match
// rows. A match is storage evidence for the connected database instance; it
// does not identify its compute hardware, per-file extents or other members.
type DatafileStorageHardwareResult struct {
	Provenance       Provenance
	HardwareType     StorageHardwareType
	ExadataDiskCount session.Value
}

// DatafileStorageHardwareStatement selects the read-only ASM datafile-group
// probe. Call it in a non-CDB or CDB root on the admitted database instance.
func DatafileStorageHardwareStatement(oracleVersion string) (string, error) {
	p, err := datafileStorageHardwarePlan(oracleVersion)
	return p.sql, err
}

func datafileStorageHardwarePlan(oracleVersion string) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(oracleVersion)
	version, _ := oracleversion.Parse(oracleVersion)
	if !ok || major < 10 || (major == 10 && version[1] != 2) {
		return selection{}, ErrUnsupportedVersion
	}
	// The STAT views avoid disk discovery. Oracle documents o/ as the grid
	// disk path prefix; only count disks in groups referenced by datafiles.
	return selection{
		sql: datafileStorageHardwareSQL,
		schema: []rowset.Column{
			{Name: "HARDWARE_TYPE", Kind: rowset.Text},
			{Name: "EXADATA_DISK_COUNT", Kind: rowset.Integer},
		},
		provenance: Provenance{
			Source: datafileStorageHardwareSource, OracleMajor: major,
			Scope: "database_visible_datafile_disk_groups", SourceRole: string(DatabaseInstance),
		},
	}, nil
}

// DecodeDatafileStorageHardware validates the single aggregate and its
// classification. Decode makes no claim about transport completion.
func DecodeDatafileStorageHardware(result session.Result, oracleVersion string) (DatafileStorageHardwareResult, error) {
	p, err := datafileStorageHardwarePlan(oracleVersion)
	if err != nil {
		return DatafileStorageHardwareResult{}, err
	}
	return decodeDatafileStorageHardware(result, p)
}

func decodeDatafileStorageHardware(result session.Result, p selection) (DatafileStorageHardwareResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return DatafileStorageHardwareResult{}, err
	}
	if len(result.Rows) != 1 {
		return DatafileStorageHardwareResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	row := result.Rows[0]
	count, reported := row[index["EXADATA_DISK_COUNT"]], row[index["HARDWARE_TYPE"]]
	hardware := StorageHardwareUnknown
	if strings.TrimLeft(count.Text, "0") != "" {
		hardware = StorageHardwareExadata
	}
	if count.Null || reported.Null || reported.Text != string(hardware) {
		return DatafileStorageHardwareResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	return DatafileStorageHardwareResult{Provenance: p.provenance, HardwareType: hardware, ExadataDiskCount: count}, nil
}

// ReadDatafileStorageHardware queries the admitted database instance once.
// Query, capacity, decoding and cancellation errors return no classification.
func ReadDatafileStorageHardware(ctx context.Context, reader Reader, request DatafileStorageHardwareRequest) (DatafileStorageHardwareResult, error) {
	p, err := datafileStorageHardwarePlan(request.OracleVersion)
	if err != nil {
		return DatafileStorageHardwareResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return DatafileStorageHardwareResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, p)
	if err != nil {
		return DatafileStorageHardwareResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return DatafileStorageHardwareResult{}, &Error{p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err())}
	}
	fact, decodeErr := decodeDatafileStorageHardware(result, p)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return DatafileStorageHardwareResult{}, &Error{p.provenance.Source, err}
	}
	fact.Provenance.Complete = true
	return fact, nil
}

// DatafileStorageHardwareRows returns the caller-native cursor. The caller
// owns iteration, closure, cancellation and the admitted database scope.
func DatafileStorageHardwareRows(ctx context.Context, q rowset.Queryer, oracleVersion string) (*sql.Rows, error) {
	p, err := datafileStorageHardwarePlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// DatafileStorageHardwareJSON reads a separate bounded cursor and returns the
// complete aggregate and provenance only when the read and validation succeed.
func DatafileStorageHardwareJSON(ctx context.Context, q rowset.Queryer, oracleVersion string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := datafileStorageHardwarePlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeDatafileStorageHardware(result, p)
		return err
	})
}
