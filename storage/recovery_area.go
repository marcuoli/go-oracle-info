package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// RecoveryAreaRow preserves the displayed location and exact byte and file
// counts. A NULL Name remains distinct from a failed or empty read.
type RecoveryAreaRow struct {
	Name, SpaceLimit, SpaceUsed, SpaceReclaimable, FileCount session.Value
	ReportingInstanceID                                      session.Value
	ContainerID, SourceContainerID                           *session.Value
}

// RecoveryAreaResult contains the selected rows and their source visibility.
type RecoveryAreaResult struct {
	Provenance Provenance
	Rows       []RecoveryAreaRow
}

// RecoveryAreaStatement selects documented observations at the admitted release.
func RecoveryAreaStatement(options RecoveryAreaOptions) (string, error) {
	p, err := recoveryAreaPlan(options)
	return p.sql, err
}

func recoveryAreaPlan(options RecoveryAreaOptions) (selection, error) {
	major, err := recoveryAreaMajor(options)
	if err != nil {
		return selection{}, err
	}
	return recoveryAreaSelection(major, "V$RECOVERY_FILE_DEST", []storageField{
		storageRaw("NAME"),
		storageExact("v.SPACE_LIMIT", "SPACE_LIMIT"),
		storageExact("v.SPACE_USED", "SPACE_USED"),
		storageExact("v.SPACE_RECLAIMABLE", "SPACE_RECLAIMABLE"),
		storageExact("v.NUMBER_OF_FILES", "NUMBER_OF_FILES"),
	}), nil
}

// DecodeRecoveryArea validates all projected values and preserves SQL NULL.
func DecodeRecoveryArea(result session.Result, options RecoveryAreaOptions) (RecoveryAreaResult, error) {
	p, err := recoveryAreaPlan(options)
	if err != nil {
		return RecoveryAreaResult{}, err
	}
	return decodeRecoveryArea(result, p)
}

func decodeRecoveryArea(result session.Result, p selection) (RecoveryAreaResult, error) {
	index, err := recoveryAreaIndex(result, p)
	if err != nil {
		return RecoveryAreaResult{}, err
	}
	rows := make([]RecoveryAreaRow, len(result.Rows))
	for i, row := range result.Rows {
		v := RecoveryAreaRow{
			Name: row[index["NAME"]], SpaceLimit: row[index["SPACE_LIMIT"]], SpaceUsed: row[index["SPACE_USED"]],
			SpaceReclaimable: row[index["SPACE_RECLAIMABLE"]], FileCount: row[index["NUMBER_OF_FILES"]],
			ReportingInstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			SourceContainerID: optional(row, index, "SOURCE_CON_ID"),
		}

		rows[i] = v
	}
	return RecoveryAreaResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRecoveryArea returns one complete bounded observation over an admitted reader.
func ReadRecoveryArea(ctx context.Context, reader Reader, request RecoveryAreaRequest) (RecoveryAreaResult, error) {
	p, err := recoveryAreaPlan(request.Options)
	if err != nil {
		return RecoveryAreaResult{}, err
	}
	result, err := readBoundedStorage(ctx, reader, request.Reference, request.Limits, p)
	if err != nil {
		return RecoveryAreaResult{}, err
	}
	facts, err := decodeRecoveryArea(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return RecoveryAreaResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RecoveryAreaRows transfers the actual native cursor to its caller.
func RecoveryAreaRows(ctx context.Context, q rowset.Queryer, options RecoveryAreaOptions) (*sql.Rows, error) {
	p, err := recoveryAreaPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// RecoveryAreaJSON returns one independently consumed and validated bounded dataset.
func RecoveryAreaJSON(ctx context.Context, q rowset.Queryer, options RecoveryAreaOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := recoveryAreaPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeRecoveryArea(result, p)
		return err
	})
}
