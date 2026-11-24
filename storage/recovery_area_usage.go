package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// RecoveryAreaUsageRow retains the source file-type label, exact percentage
// text and file count. Percentages are reported by Oracle relative to the
// recovery-area quota, with no conversion into estimated bytes.
type RecoveryAreaUsageRow struct {
	FileType, PercentSpaceUsed, PercentSpaceReclaimable, FileCount session.Value
	ReportingInstanceID                                            session.Value
	ContainerID, SourceContainerID                                 *session.Value
}

// RecoveryAreaUsageResult contains the selected rows and their source visibility.
type RecoveryAreaUsageResult struct {
	Provenance Provenance
	Rows       []RecoveryAreaUsageRow
}

// RecoveryAreaUsageStatement selects documented observations at the admitted release.
func RecoveryAreaUsageStatement(options RecoveryAreaOptions) (string, error) {
	p, err := recoveryAreaUsagePlan(options)
	return p.sql, err
}

func recoveryAreaUsagePlan(options RecoveryAreaOptions) (selection, error) {
	major, err := recoveryAreaMajor(options)
	if err != nil {
		return selection{}, err
	}
	version, _ := oracleversion.Parse(options.OracleVersion)
	source := "V$RECOVERY_AREA_USAGE"
	switch {
	case major == 10 && version[1] == 1:
		return selection{}, ErrUnsupportedVersion
	case major == 10 || (major == 11 && version[1] == 1):
		source = "V$FLASH_RECOVERY_AREA_USAGE"
	}
	p := recoveryAreaSelection(major, source, []storageField{
		storageRaw("FILE_TYPE"),
		storageNumber("v.PERCENT_SPACE_USED", "PERCENT_SPACE_USED"),
		storageNumber("v.PERCENT_SPACE_RECLAIMABLE", "PERCENT_SPACE_RECLAIMABLE"),
		storageExact("v.NUMBER_OF_FILES", "NUMBER_OF_FILES"),
	})
	if major >= 12 {
		p.sql += " ORDER BY v.CON_ID, v.FILE_TYPE"
	} else {
		p.sql += " ORDER BY v.FILE_TYPE"
	}
	return p, nil
}

// DecodeRecoveryAreaUsage validates all projected values and preserves SQL NULL.
func DecodeRecoveryAreaUsage(result session.Result, options RecoveryAreaOptions) (RecoveryAreaUsageResult, error) {
	p, err := recoveryAreaUsagePlan(options)
	if err != nil {
		return RecoveryAreaUsageResult{}, err
	}
	return decodeRecoveryAreaUsage(result, p)
}

func decodeRecoveryAreaUsage(result session.Result, p selection) (RecoveryAreaUsageResult, error) {
	index, err := recoveryAreaIndex(result, p)
	if err != nil {
		return RecoveryAreaUsageResult{}, err
	}
	rows := make([]RecoveryAreaUsageRow, len(result.Rows))
	for i, row := range result.Rows {
		v := RecoveryAreaUsageRow{
			FileType: row[index["FILE_TYPE"]], PercentSpaceUsed: row[index["PERCENT_SPACE_USED"]],
			PercentSpaceReclaimable: row[index["PERCENT_SPACE_RECLAIMABLE"]], FileCount: row[index["NUMBER_OF_FILES"]],
			ReportingInstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			SourceContainerID: optional(row, index, "SOURCE_CON_ID"),
		}
		if v.FileType.Null || v.FileType.Text == "" || !validRecoveryAreaPercent(v.PercentSpaceUsed) || !validRecoveryAreaPercent(v.PercentSpaceReclaimable) {
			return RecoveryAreaUsageResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = v
	}
	return RecoveryAreaUsageResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRecoveryAreaUsage returns one complete bounded observation over an admitted reader.
func ReadRecoveryAreaUsage(ctx context.Context, reader Reader, request RecoveryAreaRequest) (RecoveryAreaUsageResult, error) {
	p, err := recoveryAreaUsagePlan(request.Options)
	if err != nil {
		return RecoveryAreaUsageResult{}, err
	}
	result, err := readBoundedStorage(ctx, reader, request.Reference, request.Limits, p)
	if err != nil {
		return RecoveryAreaUsageResult{}, err
	}
	facts, err := decodeRecoveryAreaUsage(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return RecoveryAreaUsageResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RecoveryAreaUsageRows transfers the actual native cursor to its caller.
func RecoveryAreaUsageRows(ctx context.Context, q rowset.Queryer, options RecoveryAreaOptions) (*sql.Rows, error) {
	p, err := recoveryAreaUsagePlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// RecoveryAreaUsageJSON returns one independently consumed and validated bounded dataset.
func RecoveryAreaUsageJSON(ctx context.Context, q rowset.Queryer, options RecoveryAreaOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := recoveryAreaUsagePlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeRecoveryAreaUsage(result, p)
		return err
	})
}
