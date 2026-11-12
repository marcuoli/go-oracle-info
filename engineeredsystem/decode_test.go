package engineeredsystem

import (
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func resultRows(columns int, rows ...[]session.Value) session.Result {
	return session.Result{Columns: make([]string, columns), Rows: rows}
}
func known(value string) session.Value { return session.Value{Text: value} }
func TestSharedCellsRetainNullsDuplicatesConflictsAndRejectedCounts(t *testing.T) {
	row := []session.Value{known("1"), known("9007199254740993"), known("lab"), {Null: true}}
	conflict := append([]session.Value(nil), row...)
	conflict[3] = known("EXADATA")
	invalid := append([]session.Value(nil), row...)
	invalid[0] = session.Value{Null: true}
	report, counts, err := DecodeCells(resultRows(4, row, row, conflict, invalid))
	if !errors.Is(err, ErrConflictingEvidence) || counts != (Counts{Read: 4, Retained: 3, Rejected: 1}) || len(report.Cells) != 3 || !report.Cells[0].TypeNull || report.Cells[0].Hash != "9007199254740993" {
		t.Fatalf("shared cell facts/counts changed: %+v %v", counts, err)
	}
}
func TestSharedScalarAlertsPreserveEveryFieldAndRepeatedIdentity(t *testing.T) {
	row := []session.Value{known("2"), known("9007199254740993"), known("lab"), known("2026-11-12T09:00:00 AD"), known("9007199254740995"), known("= literal é 😀"), {Null: true}, known("critical"), known("0")}
	report, counts, err := DecodeAlerts(resultRows(9, row, row), time.Unix(1, 0))
	if err != nil || counts != (Counts{Read: 2, Retained: 2}) || len(report.Properties) != 16 || report.Properties[0].ComponentID == report.Properties[8].ComponentID {
		t.Fatalf("repeated alerts/counts changed: %+v %v", counts, err)
	}
	fields := map[string]Property{}
	for _, p := range report.Properties[:8] {
		fields[p.Name] = p
	}
	if fields["MESSAGE"].Value != "= literal é 😀" || !fields["STATEFUL"].Null || fields["SEQ_NO"].Value != "9007199254740995" {
		t.Fatal("exact alert values/NULL lost")
	}
}
func TestSharedConfigurationsRetainBadRawBytesAndValidSibling(t *testing.T) {
	xml := "<cli-output><lun name=\"lab\"><size>1 GiB é 😀</size></lun></cli-output>"
	native := len(utf16.Encode([]rune(xml)))
	rows := resultRows(6, []session.Value{known("1"), known("7"), known("lab"), known("LUNS"), known(xml), known(strconv.Itoa(native))}, []session.Value{known("1"), known("7"), known("lab"), known("LUNS"), known("<bad>" + string([]byte{0xff})), known("7")})
	report, counts, err := DecodeConfiguration(rows, Config, time.Unix(1, 0), nil)
	if err == nil || counts != (Counts{Read: 2, Retained: 1, Rejected: 1}) || len(report.Payloads) != 2 || !report.Payloads[1].Rejected || !strings.Contains(report.Payloads[1].XML, string([]byte{0xff})) || len(report.Properties) == 0 {
		t.Fatalf("shared payload preservation changed: %+v %v", counts, err)
	}
}
func TestSharedDecodersRejectUnexpectedProjection(t *testing.T) {
	if _, _, err := DecodeCells(resultRows(5)); err == nil {
		t.Fatal("unknown cell shape accepted")
	}
	if _, _, err := DecodeAlerts(resultRows(8), time.Time{}); err == nil {
		t.Fatal("unknown alert shape accepted")
	}
	if _, _, err := DecodeConfiguration(resultRows(5), Config, time.Time{}, nil); err == nil {
		t.Fatal("unknown XML shape accepted")
	}
}
