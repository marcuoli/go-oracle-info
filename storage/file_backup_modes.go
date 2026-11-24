package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FileBackupMode records a visible member's datafile backup status, together
// with any SCN and time reported for the backup start.
type FileBackupMode struct {
	FileHealthIdentity
	Status, StartSCN, StartTime session.Value
}

// FileBackupModesResult preserves the visible rows and their selected source.
type FileBackupModesResult struct {
	Provenance Provenance
	Rows       []FileBackupMode
}

// FileBackupModesStatement selects visible per-member datafile backup mode.
func FileBackupModesStatement(options FileHealthOptions) (string, error) {
	p, err := fileBackupModesPlan(options)
	return p.sql, err
}

func fileBackupModesPlan(options FileHealthOptions) (selection, error) {
	p, err := fileHealthSelection(options, "GV$BACKUP", "visible_reporting_members", "v.INST_ID", []fileHealthField{
		fileHealthRaw("STATUS"), fileHealthExact("v.CHANGE#", "START_SCN"), fileHealthDate("v.TIME", "START_TIME"),
	})
	if err != nil {
		return selection{}, err
	}
	p.sql += " ORDER BY " + fileHealthOrder(p)
	return p, nil
}

// DecodeFileBackupModes validates the full projection and preserves its raw source values.
func DecodeFileBackupModes(result session.Result, options FileHealthOptions) (FileBackupModesResult, error) {
	p, err := fileBackupModesPlan(options)
	if err != nil {
		return FileBackupModesResult{}, err
	}
	return decodeFileBackupModes(result, p)
}

func decodeFileBackupModes(result session.Result, p selection) (FileBackupModesResult, error) {
	index, err := fileHealthIndex(result, p)
	if err != nil {
		return FileBackupModesResult{}, err
	}
	rows := make([]FileBackupMode, len(result.Rows))
	for i, row := range result.Rows {
		v := FileBackupMode{
			FileHealthIdentity: fileHealthIdentity(row, index),
			Status:             row[index["STATUS"]], StartSCN: row[index["START_SCN"]], StartTime: row[index["START_TIME"]],
		}
		if !validFileHealthDate(v.StartTime) {
			return FileBackupModesResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = v
	}
	return FileBackupModesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFileBackupModes returns the complete buffered observation through an admitted reader.
func ReadFileBackupModes(ctx context.Context, reader Reader, request FileHealthRequest) (FileBackupModesResult, error) {
	p, err := fileBackupModesPlan(request.Options)
	if err != nil {
		return FileBackupModesResult{}, err
	}
	result, err := readFileHealth(ctx, reader, request, p)
	if err != nil {
		return FileBackupModesResult{}, err
	}
	facts, err := decodeFileBackupModes(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return FileBackupModesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FileBackupModesRows returns the caller's native cursor. The caller consumes and closes it.
func FileBackupModesRows(ctx context.Context, q rowset.Queryer, options FileHealthOptions) (*sql.Rows, error) {
	p, err := fileBackupModesPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// FileBackupModesJSON independently consumes and validates one complete bounded cursor.
func FileBackupModesJSON(ctx context.Context, q rowset.Queryer, options FileHealthOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fileBackupModesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeFileBackupModes(r, p)
		return err
	})
}
