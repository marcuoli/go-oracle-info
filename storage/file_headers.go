package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FileHeader preserves datafile header observations. A populated Error can
// leave the remaining fields NULL or reflect failed validation.
type FileHeader struct {
	FileHealthIdentity
	Error, Recover, Fuzzy, CheckpointSCN, CheckpointTime session.Value
}

// FileHeadersResult preserves the visible rows and their selected source.
type FileHeadersResult struct {
	Provenance Provenance
	Rows       []FileHeader
}

// FileHeadersStatement selects local-instance datafile header observations.
func FileHeadersStatement(options FileHealthOptions) (string, error) {
	p, err := fileHeadersPlan(options)
	return p.sql, err
}

func fileHeadersPlan(options FileHealthOptions) (selection, error) {
	p, err := fileHealthSelection(options, "V$DATAFILE_HEADER", "visible_local_file_headers", fileSourceInstanceSQL, []fileHealthField{
		fileHealthRaw("ERROR"), fileHealthRaw("RECOVER"), fileHealthRaw("FUZZY"),
		fileHealthExact("v.CHECKPOINT_CHANGE#", "CHECKPOINT_SCN"), fileHealthDate("v.CHECKPOINT_TIME", "CHECKPOINT_TIME"),
	})
	if err != nil {
		return selection{}, err
	}
	p.sql += " ORDER BY " + fileHealthOrder(p)
	return p, nil
}

// DecodeFileHeaders validates the full projection and preserves its raw source values.
func DecodeFileHeaders(result session.Result, options FileHealthOptions) (FileHeadersResult, error) {
	p, err := fileHeadersPlan(options)
	if err != nil {
		return FileHeadersResult{}, err
	}
	return decodeFileHeaders(result, p)
}

func decodeFileHeaders(result session.Result, p selection) (FileHeadersResult, error) {
	index, err := fileHealthIndex(result, p)
	if err != nil {
		return FileHeadersResult{}, err
	}
	rows := make([]FileHeader, len(result.Rows))
	for i, row := range result.Rows {
		v := FileHeader{
			FileHealthIdentity: fileHealthIdentity(row, index),
			Error:              row[index["ERROR"]], Recover: row[index["RECOVER"]], Fuzzy: row[index["FUZZY"]],
			CheckpointSCN: row[index["CHECKPOINT_SCN"]], CheckpointTime: row[index["CHECKPOINT_TIME"]],
		}
		if !validFileHealthDate(v.CheckpointTime) {
			return FileHeadersResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = v
	}
	return FileHeadersResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFileHeaders returns the complete buffered observation through an admitted reader.
func ReadFileHeaders(ctx context.Context, reader Reader, request FileHealthRequest) (FileHeadersResult, error) {
	p, err := fileHeadersPlan(request.Options)
	if err != nil {
		return FileHeadersResult{}, err
	}
	result, err := readFileHealth(ctx, reader, request, p)
	if err != nil {
		return FileHeadersResult{}, err
	}
	facts, err := decodeFileHeaders(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return FileHeadersResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FileHeadersRows returns the caller's native cursor. The caller consumes and closes it.
func FileHeadersRows(ctx context.Context, q rowset.Queryer, options FileHealthOptions) (*sql.Rows, error) {
	p, err := fileHeadersPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// FileHeadersJSON independently consumes and validates one complete bounded cursor.
func FileHeadersJSON(ctx context.Context, q rowset.Queryer, options FileHealthOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fileHeadersPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeFileHeaders(r, p)
		return err
	})
}
