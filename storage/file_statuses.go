package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FileStatus is control-file status for one datafile or tempfile. The file
// type disambiguates independent FILE# values in those two inventories.
type FileStatus struct {
	FileHealthIdentity
	Type, Name, Status session.Value
}

// FileStatusesResult preserves the visible rows and their selected source.
type FileStatusesResult struct {
	Provenance Provenance
	Rows       []FileStatus
}

// FileStatusesStatement selects the control-file datafile and tempfile status columns.
func FileStatusesStatement(options FileHealthOptions) (string, error) {
	p, err := fileStatusesPlan(options)
	return p.sql, err
}

func fileStatusesPlan(options FileHealthOptions) (selection, error) {
	fields := []fileHealthField{
		{name: "FILE_TYPE", expression: "'DATAFILE'", kind: rowset.Text},
		{name: "FILE_NAME", expression: "v.NAME", kind: rowset.Text},
		fileHealthRaw("STATUS"),
	}
	data, err := fileHealthSelection(options, "V$DATAFILE", "visible_controlfile", fileSourceInstanceSQL, fields)
	if err != nil {
		return selection{}, err
	}
	fields[0].expression = "'TEMPFILE'"
	temp, err := fileHealthSelection(options, "V$TEMPFILE", "visible_controlfile", fileSourceInstanceSQL, fields)
	if err != nil {
		return selection{}, err
	}
	data.provenance.Source = "V$DATAFILE,V$TEMPFILE"
	data.sql += " UNION ALL " + temp.sql + " ORDER BY FILE_TYPE, " + fileHealthOrder(data)
	return data, nil
}

// DecodeFileStatuses validates the full projection and preserves its raw source values.
func DecodeFileStatuses(result session.Result, options FileHealthOptions) (FileStatusesResult, error) {
	p, err := fileStatusesPlan(options)
	if err != nil {
		return FileStatusesResult{}, err
	}
	return decodeFileStatuses(result, p)
}

func decodeFileStatuses(result session.Result, p selection) (FileStatusesResult, error) {
	index, err := fileHealthIndex(result, p)
	if err != nil {
		return FileStatusesResult{}, err
	}
	rows := make([]FileStatus, len(result.Rows))
	for i, row := range result.Rows {
		v := FileStatus{
			FileHealthIdentity: fileHealthIdentity(row, index),
			Type:               row[index["FILE_TYPE"]], Name: row[index["FILE_NAME"]], Status: row[index["STATUS"]],
		}
		if v.Type.Null || (v.Type.Text != "DATAFILE" && v.Type.Text != "TEMPFILE") {
			return FileStatusesResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = v
	}
	return FileStatusesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadFileStatuses returns the complete buffered observation through an admitted reader.
func ReadFileStatuses(ctx context.Context, reader Reader, request FileHealthRequest) (FileStatusesResult, error) {
	p, err := fileStatusesPlan(request.Options)
	if err != nil {
		return FileStatusesResult{}, err
	}
	result, err := readFileHealth(ctx, reader, request, p)
	if err != nil {
		return FileStatusesResult{}, err
	}
	facts, err := decodeFileStatuses(result, p)
	if err = fileSummaryReadError(ctx, p, err); err != nil {
		return FileStatusesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// FileStatusesRows returns the caller's native cursor. The caller consumes and closes it.
func FileStatusesRows(ctx context.Context, q rowset.Queryer, options FileHealthOptions) (*sql.Rows, error) {
	p, err := fileStatusesPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// FileStatusesJSON independently consumes and validates one complete bounded cursor.
func FileStatusesJSON(ctx context.Context, q rowset.Queryer, options FileHealthOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := fileStatusesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeFileStatuses(r, p)
		return err
	})
}
