package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FileRecoveryEntry is an observation from V$RECOVER_FILE. OFFLINE NORMAL
// and SQL NULL remain distinct from actionable errors.
type FileRecoveryEntry struct {
	FileHealthIdentity
	Error, RecoverySCN, RecoveryTime session.Value
}

// FileRecoveryResult preserves the visible rows and their selected source.
type FileRecoveryResult struct {
	Provenance Provenance
	Rows       []FileRecoveryEntry
}

// FileRecoveryStatement selects recorded recovery candidates from the admitted control file.
func FileRecoveryStatement(options FileHealthOptions) (string, error) {
	p, err := fileRecoveryPlan(options)
	return p.sql, err
}

func fileRecoveryPlan(options FileHealthOptions) (selection, error) {
	p, err := fileHealthSelection(options, "V$RECOVER_FILE", "visible_media_recovery", fileSourceInstanceSQL, []fileHealthField{
		fileHealthRaw("ERROR"), fileHealthExact("v.CHANGE#", "RECOVERY_SCN"), fileHealthDate("v.TIME", "RECOVERY_TIME"),
	})
	if err != nil {
		return selection{}, err
	}
	p.sql += " ORDER BY " + fileHealthOrder(p)
	return p, nil
}

// DecodeFileRecovery validates the full projection and preserves its raw source values.
func DecodeFileRecovery(result session.Result, options FileHealthOptions) (FileRecoveryResult, error) {
	p, err := fileRecoveryPlan(options)
	if err != nil {
		return FileRecoveryResult{}, err
	}
	return decodeFileRecovery(result, p)
}

func decodeFileRecovery(result session.Result, p selection) (FileRecoveryResult, error) {
	index, err := fileHealthIndex(result, p)
	if err != nil {
		return FileRecoveryResult{}, err
	}
	rows := make([]FileRecoveryEntry, len(result.Rows))
	for i, row := range result.Rows {
		v := FileRecoveryEntry{
			FileHealthIdentity: fileHealthIdentity(row, index),
			Error:              row[index["ERROR"]], RecoverySCN: row[index["RECOVERY_SCN"]], RecoveryTime: row[index["RECOVERY_TIME"]],
		}
		if !validFileHealthDate(v.RecoveryTime) {
			return FileRecoveryResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = v
	}
	return FileRecoveryResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFileRecovery returns the complete buffered observation through an admitted reader.
func ReadFileRecovery(ctx context.Context, reader Reader, request FileHealthRequest) (FileRecoveryResult, error) {
	p, err := fileRecoveryPlan(request.Options)
	if err != nil {
		return FileRecoveryResult{}, err
	}
	result, err := readFileHealth(ctx, reader, request, p)
	if err != nil {
		return FileRecoveryResult{}, err
	}
	facts, err := decodeFileRecovery(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return FileRecoveryResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FileRecoveryRows returns the caller's native cursor. The caller consumes and closes it.
func FileRecoveryRows(ctx context.Context, q rowset.Queryer, options FileHealthOptions) (*sql.Rows, error) {
	p, err := fileRecoveryPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// FileRecoveryJSON independently consumes and validates one complete bounded cursor.
func FileRecoveryJSON(ctx context.Context, q rowset.Queryer, options FileHealthOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fileRecoveryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeFileRecovery(r, p)
		return err
	})
}
