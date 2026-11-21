package dataguard

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestReleaseSelectionAndAdmission(t *testing.T) {
	for _, version := range []string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.1.0.2.0", "12.2.0.1.0", "18.0.0.0.0", "20.0.0.0.0", "24.0.0.0.0", "19garbage", "19.1.bad"} {
		if sql, _, err := StandbyLogsStatement(version, Filter{}); err == nil || sql != "" {
			t.Fatalf("unreviewed version executed: %s %s %v", version, sql, err)
		}
	}
	for _, version := range []string{"19.32.0.0.0", "21.3.0.0.0", "23.26.1.0.0"} {
		sql, args, err := StandbyLogsStatement(version, Filter{Instances: []int64{2, 7}})
		if err != nil || !strings.Contains(sql, "FROM GV$STANDBY_LOG v") || !strings.Contains(sql, "v.GROUP# AS GROUP_NUMBER") || !strings.Contains(sql, "v.LAST_CHANGE#") || len(args) != 2 || strings.Contains(sql, "V$DATABASE") {
			t.Fatalf("wrong SRL contract %s %v %v", sql, args, err)
		}
		sql, _, err = ProcessesStatement(version, Filter{})
		if err != nil || !strings.Contains(sql, "GV$DATAGUARD_PROCESS") || !strings.Contains(sql, " AS RESETLOG_ID") || strings.Contains(sql, "MANAGED_STANDBY") {
			t.Fatalf("wrong process source %s %v", sql, err)
		}
	}
	for _, ids := range [][]int64{{0}, {-1}, {1, 1}} {
		if _, _, err := StandbyLogsStatement("19.32.0.0.0", Filter{Instances: ids}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("invalid IDs accepted: %v %v", ids, err)
		}
	}
	sql, args, err := ArchiveProgressStatementWithOptions(ArchiveProgressSourceOptions{OracleVersion: "23.26.1.0.0", Filter: ArchiveProgressOptions{Instances: []int64{7}, ResetlogsChange: "18446744073709551616", ResetlogsTime: "2026-11-18T00:00:00"}})
	if err != nil || len(args) != 3 || args[0] != "18446744073709551616" || args[1] != 0 || args[2] != int64(7) || !strings.Contains(sql, "REGISTRAR = 'RFS'") || !strings.Contains(sql, "APPLIED = 'YES'") || !strings.Contains(sql, "DENSE_RANK()") || strings.Contains(sql, "MAX(") || strings.Contains(sql, "FIRST_TIME >=") || strings.Contains(sql, "SYSDATE") {
		t.Fatalf("unsafe latest snapshot %s %v %v", sql, args, err)
	}
	if strings.Contains(sql, "18446744073709551616") {
		t.Fatal("incarnation was interpolated")
	}
	if _, _, err = ArchiveProgressStatement("19.1.0.0.0", ArchiveProgressOptions{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing incarnation accepted: %v", err)
	}
}

func allPlans(t *testing.T) []selection {
	t.Helper()
	var plans []selection
	for _, fn := range []func() (selection, error){func() (selection, error) { return databaseContextPlan("19.32.0.0.0") }, func() (selection, error) { return standbyLogsPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) { return processesPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) { return recoveryProgressPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) { return archiveGapsPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) { return archiveDestinationsPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) { return archiveDestinationStatusPlan("19.32.0.0.0", Filter{}) }, func() (selection, error) {
		return archiveProgressPlan("19.32.0.0.0", ArchiveProgressOptions{ResetlogsChange: "18446744073709551616", ResetlogsTime: "2026-11-18T00:00:00"})
	}} {
		p, e := fn()
		if e != nil {
			t.Fatal(e)
		}
		plans = append(plans, p)
	}
	return plans
}
func validSource(p selection) session.Result {
	r := session.Result{}
	row := []session.Value{}
	for _, c := range p.schema {
		r.Columns = append(r.Columns, c.Name)
		v := session.Value{Null: true}
		switch c.Name {
		case "INST_ID":
			v = session.Value{Text: "7"}
		case "CON_ID":
			v = session.Value{Text: "0"}
		case "RESETLOGS_CHANGE":
			v = session.Value{Text: "18446744073709551616"}
		case "RESETLOGS_TIME":
			if p.provenance.ResetlogsTime != "" {
				v = session.Value{Text: p.provenance.ResetlogsTime}
			}
		case "REGISTRAR":
			v = session.Value{Text: "RFS"}
		case "KIND":
			v = session.Value{Text: "RECEIVED"}
		case "THREAD_NUMBER", "SEQUENCE_NUMBER":
			v = session.Value{Text: "1"}
		}
		row = append(row, v)
	}
	r.Rows = [][]session.Value{row}
	return r
}
func nativeStep(p selection, r session.Result) testsql.Step {
	s := testsql.Step{SQL: p.sql, Columns: r.Columns}
	for _, r := range r.Rows {
		v := []driver.Value{}
		for _, c := range r {
			if c.Null {
				v = append(v, nil)
			} else {
				v = append(v, c.Text)
			}
		}
		s.Values = append(s.Values, v)
	}
	return s
}
func TestAllJSONDomainAdmissionAndAtomicFailure(t *testing.T) {
	for _, p := range allPlans(t) {
		t.Run(p.provenance.Source, func(t *testing.T) {
			r := validSource(p)
			db, state := testsql.New(t, nativeStep(p, r))
			payload, err := readJSON(context.Background(), db, p, rowset.Limits{MaxRows: 4, MaxBytes: 65536})
			if err != nil || !json.Valid(payload) || state.Closed() != 1 {
				t.Fatalf("valid JSON lost: %s %v closes%d", payload, err, state.Closed())
			}
			var got struct {
				Provenance Provenance
				Data       struct{ Rows [][]any }
			}
			if err = json.Unmarshal(payload, &got); err != nil || !got.Provenance.Complete || len(got.Data.Rows) != 1 {
				t.Fatalf("lost completion/rows: %s %v", payload, err)
			}
			bad := append([]session.Value(nil), r.Rows[0]...)
			for i, c := range p.schema {
				if c.Name == "INST_ID" {
					bad[i] = session.Value{Text: "0"}
				}
				if c.Name == "CURRENT_SCN" {
					bad[i] = session.Value{Text: "1.5"}
				}
			}
			r.Rows = append(r.Rows, bad)
			db, _ = testsql.New(t, nativeStep(p, r))
			payload, err = readJSON(context.Background(), db, p, rowset.Limits{MaxRows: 4, MaxBytes: 65536})
			if payload != nil || !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("published invalid final row %s %v", payload, err)
			}
			r = validSource(p)
			step := nativeStep(p, r)
			late := errors.New("late source failure")
			step.NextErr = late
			db, _ = testsql.New(t, step)
			payload, err = readJSON(context.Background(), db, p, rowset.Limits{})
			if payload != nil || !errors.Is(err, late) {
				t.Fatalf("lost terminal error %s %v", payload, err)
			}
		})
	}
}

func TestNumeric26RejectedWithoutSQL(t *testing.T) {
	const version = "26.1.0.0.0"
	progress := ArchiveProgressOptions{ResetlogsChange: "100", ResetlogsTime: "2026-11-18T01:02:03"}
	statements := []func() (string, []any, error){
		func() (string, []any, error) { return DatabaseContextStatement(version) },
		func() (string, []any, error) { return StandbyLogsStatement(version, Filter{}) },
		func() (string, []any, error) { return ProcessesStatement(version, Filter{}) },
		func() (string, []any, error) { return RecoveryProgressStatement(version, Filter{}) },
		func() (string, []any, error) { return ArchiveGapsStatement(version, Filter{}) },
		func() (string, []any, error) { return ArchiveDestinationsStatement(version, Filter{}) },
		func() (string, []any, error) { return ArchiveDestinationStatusStatement(version, Filter{}) },
		func() (string, []any, error) { return ArchiveProgressStatement(version, progress) },
	}
	for i, fn := range statements {
		sql, args, err := fn()
		if !errors.Is(err, ErrUnsupportedVersion) || sql != "" || len(args) != 0 {
			t.Errorf("family%d admitted hypothetical numeric26: %q %v %v", i, sql, args, err)
		}
	}
	jsonMethods := []func(rowset.Queryer) (json.RawMessage, error){
		func(q rowset.Queryer) (json.RawMessage, error) {
			return DatabaseContextJSON(t.Context(), q, version, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return StandbyLogsJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return ProcessesJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return RecoveryProgressJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return ArchiveGapsJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return ArchiveDestinationsJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return ArchiveDestinationStatusJSON(t.Context(), q, version, Filter{}, rowset.Limits{})
		},
		func(q rowset.Queryer) (json.RawMessage, error) {
			return ArchiveProgressJSON(t.Context(), q, version, progress, rowset.Limits{})
		},
	}
	for i, fn := range jsonMethods {
		db, state := testsql.New(t)
		data, err := fn(db)
		if !errors.Is(err, ErrUnsupportedVersion) || data != nil || len(state.Calls()) != 0 {
			t.Errorf("family%d executed numeric26 SQL: %s %v calls%d", i, data, err, len(state.Calls()))
		}
	}
}
