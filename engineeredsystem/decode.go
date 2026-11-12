package engineeredsystem

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Counts describe complete caller-supplied source rows, not XML property counts.
// Cursor/transport failures must be handled before passing a result here.
type Counts struct{ Read, Retained, Rejected int64 }

var ErrConflictingEvidence = errors.New("conflicting cell identity observations retained; no winner selected")

func sourceRows(result session.Result, columns int) error {
	if len(result.Columns) != columns {
		return fmt.Errorf("engineered-system source returned %d columns; expected %d", len(result.Columns), columns)
	}
	for _, row := range result.Rows {
		if len(row) != columns {
			return errors.New("engineered-system source has an inconsistent row width")
		}
	}
	return nil
}
func nullable(value session.Value) sql.NullString {
	return sql.NullString{String: value.Text, Valid: !value.Null}
}

// DecodeCells preserves duplicates and conflicting observations without choosing
// a winner. It performs no scope admission, connection access or completion policy.
func DecodeCells(result session.Result) (Report, Counts, error) {
	report := Report{}
	counts := Counts{Read: int64(len(result.Rows))}
	if err := sourceRows(result, 4); err != nil {
		counts.Rejected = counts.Read
		return report, counts, err
	}
	var failures []error
	for _, values := range result.Rows {
		instance, e := positiveInstance(nullable(values[0]))
		hash, path, kind := nullable(values[1]), nullable(values[2]), nullable(values[3])
		if e != nil || (!hash.Valid && !path.Valid) || (strings.TrimSpace(hash.String) == "" && strings.TrimSpace(path.String) == "") {
			counts.Rejected++
			failures = append(failures, errors.New("engineered-cell row has invalid instance or empty cell identity"))
			continue
		}
		cell := Cell{InstanceID: instance, Hash: hash.String, Path: path.String, Type: kind.String, HashNull: !hash.Valid, PathNull: !path.Valid, TypeNull: !kind.Valid}
		for _, prior := range report.Cells {
			same := prior.InstanceID == cell.InstanceID && ((prior.Hash != "" && prior.Hash == cell.Hash) || (prior.Path != "" && prior.Path == cell.Path))
			if same && prior != cell {
				failures = append(failures, ErrConflictingEvidence)
			}
		}
		report.Cells = append(report.Cells, cell)
		counts.Retained++
	}
	return report, counts, errors.Join(failures...)
}

// DecodeAlerts retains every scalar field and source-row grouping, including
// repeated sequence numbers. Reported severity never becomes inferred health.
func DecodeAlerts(result session.Result, observedAt time.Time) (Report, Counts, error) {
	report := Report{}
	counts := Counts{Read: int64(len(result.Rows))}
	if err := sourceRows(result, 9); err != nil {
		counts.Rejected = counts.Read
		return report, counts, err
	}
	var failures []error
	for index, values := range result.Rows {
		instance, rowErr := positiveInstance(nullable(values[0]))
		names := []string{"CELL_HASH", "CELL_NAME", "BEGIN_TIME", "SEQ_NO", "MESSAGE", "STATEFUL", "SEVERITY", "CON_ID"}
		for i, name := range names {
			if !values[i+1].Null && !utf8.ValidString(values[i+1].Text) {
				rowErr = errors.Join(rowErr, fmt.Errorf("alert field %s has invalid UTF-8", name))
			}
		}
		if rowErr != nil {
			counts.Rejected++
			failures = append(failures, fmt.Errorf("alert row %d: %w", index+1, rowErr))
			continue
		}
		id := "source-row[" + strconv.Itoa(index+1) + "]"
		if !values[4].Null {
			id += "/seq-no[" + values[4].Text + "]"
		}
		for i, name := range names {
			field := values[i+1]
			report.Properties = append(report.Properties, Property{InstanceID: instance, CellHash: values[1].Text, CellPath: values[2].Text, ComponentType: "alert", ComponentID: id, Name: name, Value: field.Text, Null: field.Null, ReportedStatus: values[7].Text, Source: OpenAlerts, SourceType: "OPEN_ALERTS", ObservedAt: observedAt, Ordinal: index*len(names) + i + 1})
		}
		counts.Retained++
	}
	return report, counts, errors.Join(failures...)
}

type xmlRow struct {
	instance, hash, cell, kind, xml sql.NullString
	nativeLength                    sql.NullInt64
}

// DecodeConfiguration retains each original payload before validation, including
// invalid UTF-8. Complete siblings survive a malformed/short document. The caller
// supplies actual source/time and independently collected cell identities.
func DecodeConfiguration(result session.Result, source Source, observedAt time.Time, cells []Cell) (Report, Counts, error) {
	report := Report{}
	counts := Counts{Read: int64(len(result.Rows))}
	if err := sourceRows(result, 6); err != nil {
		counts.Rejected = counts.Read
		return report, counts, err
	}
	if source != Config && source != ConfigInfo && source != OpenAlerts {
		counts.Rejected = counts.Read
		return report, counts, errors.New("invalid engineered-system XML source")
	}
	var failures []error
	for index, values := range result.Rows {
		row := xmlRow{instance: nullable(values[0]), hash: nullable(values[1]), cell: nullable(values[2]), kind: nullable(values[3]), xml: nullable(values[4])}
		var lengthErr error
		if !values[5].Null {
			row.nativeLength.Valid = true
			row.nativeLength.Int64, lengthErr = strconv.ParseInt(values[5].Text, 10, 64)
		}
		instance, instanceErr := positiveInstance(row.instance)
		payload := Payload{InstanceID: instance, CellHash: row.hash.String, CellPath: row.cell.String, SourceType: row.kind.String, Source: source, XML: row.xml.String, Null: !row.xml.Valid, ObservedAt: observedAt, NativeLength: row.nativeLength.Int64, NativeLengthKnown: row.nativeLength.Valid}
		payload = decoratePayload(payload, cells)
		rowErr := errors.Join(instanceErr, lengthErr)
		if rowErr == nil {
			rowErr = validateLength(row)
		}
		var properties []Property
		if rowErr == nil {
			properties, rowErr = DecodeXML(payload)
		}
		if rowErr != nil {
			payload.Rejected = true
			payload.Reason = ReasonInvalidPayload
			payload.Error = rowErr.Error()
		}
		report.Payloads = append(report.Payloads, payload)
		if rowErr != nil {
			counts.Rejected++
			failures = append(failures, fmt.Errorf("configuration row %d: %w", index+1, rowErr))
			continue
		}
		report.Properties = append(report.Properties, properties...)
		counts.Retained++
	}
	return report, counts, errors.Join(failures...)
}

func positiveInstance(value sql.NullString) (int64, error) {
	if !value.Valid {
		return 0, errors.New("source instance identity is NULL")
	}
	n, err := strconv.ParseInt(value.String, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("source instance identity is invalid")
	}
	return n, nil
}
func decoratePayload(payload Payload, cells []Cell) Payload {
	var selected Cell
	found := false
	for _, cell := range cells {
		if cell.InstanceID != payload.InstanceID || (payload.CellHash != "" && cell.Hash != "" && payload.CellHash != cell.Hash) || (payload.CellPath != "" && cell.Path != "" && payload.CellPath != cell.Path) || !((payload.CellPath != "" && cell.Path == payload.CellPath) || (payload.CellHash != "" && cell.Hash == payload.CellHash)) {
			continue
		}
		if found && (selected.Hash != cell.Hash || selected.Type != cell.Type) {
			return payload
		}
		selected, found = cell, true
	}
	if found {
		if payload.CellHash == "" {
			payload.CellHash = selected.Hash
		}
		payload.CellType = selected.Type
	}
	return payload
}
func validateLength(row xmlRow) error {
	if !row.xml.Valid {
		if row.nativeLength.Valid {
			return errors.New("NULL XML unexpectedly has a known native CLOB length")
		}
		return nil
	}
	if !row.nativeLength.Valid || !utf8.ValidString(row.xml.String) {
		return errors.New("XML native length or UTF-8 representation is unavailable")
	}
	var units int64
	for _, r := range row.xml.String {
		units++
		if r > 0xffff {
			units++
		}
	}
	if units != row.nativeLength.Int64 {
		return fmt.Errorf("complete XML/CLOB length mismatch: native=%d UTF16-units=%d", row.nativeLength.Int64, units)
	}
	return nil
}
