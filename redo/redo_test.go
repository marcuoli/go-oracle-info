package redo

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"strings"
	"testing"
	"time"
)

func bounds() HistoryFilter {
	return HistoryFilter{From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)}
}
func TestRedoSelectionAndLargeSCN(t *testing.T) {
	plan, _ := groupsPlan(19, Filter{})
	source := sourceResult(plan)
	facts, err := DecodeGroups(source, 19, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if facts.Rows[0].NextChange.Text != "18446744073709551615" || facts.Rows[0].BlockSize == nil || facts.Provenance.Complete {
		t.Fatalf("lost exact SCN or capability: %+v", facts)
	}
	old, _ := groupsPlan(11, Filter{})
	legacy, err := DecodeGroups(sourceResult(old), 11, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Rows[0].ContainerID != nil || legacy.Rows[0].BlockSize != nil || legacy.Rows[0].NextTime != nil || strings.Contains(old.sql, "CON_ID") || strings.Contains(old.sql, "BLOCKSIZE") {
		t.Fatal("fabricated11g optional columns")
	}
	for _, ids := range [][]int64{{0}, {-1}, {1, 1}, make([]int64, 1001)} {
		if _, _, err = GroupsStatement(19, Filter{Instances: ids}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("accepted bad scope: %v", err)
		}
	}
	members, _ := membersPlan(19, Filter{Instances: []int64{1, 2}})
	if len(members.args) != 2 || !strings.Contains(members.sql, "GV$LOGFILE") || strings.Contains(members.sql, "SUM(") {
		t.Fatal("lost member identity")
	}
	source = sourceResult(members)
	for i, name := range source.Columns {
		if name == "STATUS" {
			source.Rows[0][i] = session.Value{Null: true}
		}
	}
	m, err := DecodeMembers(source, 19, Filter{Instances: []int64{1, 2}})
	if err != nil || !m.Rows[0].Status.Null {
		t.Fatalf("lost NULL in-use status: %+v %v", m, err)
	}
}
func TestHistoryFiniteLocalBoundsAndNoLatePrefix(t *testing.T) {
	filter := bounds()
	filter.Instances = []int64{1, 7}
	options := HistoryOptions{OracleMajor: 19, Filter: filter}
	sql, args, err := HistoryStatementWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 || args[2] != 0 || args[3] != 0 || !strings.Contains(sql, "DATE '2026-11-01' + (:3/86400)") || !strings.Contains(sql, "DATE '2026-11-08' + (:4/86400)") {
		t.Fatalf("bad finite boundaries %s %+v", sql, args)
	}
	for _, bad := range []HistoryFilter{{}, {From: filter.Until, Until: filter.From}, {From: filter.From, Until: filter.From.Add(32 * 24 * time.Hour)}, {From: filter.From.Add(time.Nanosecond), Until: filter.Until}} {
		if _, _, err = HistoryStatement(19, bad); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("accepted bad bounds: %v", err)
		}
	}
	plan, _ := historyPlan(19, filter)
	source := sourceResult(plan)
	good, err := DecodeHistoryWithOptions(source, options)
	if err != nil || len(good.Rows) != 1 {
		t.Fatalf("bad valid history: %+v %v", good, err)
	}
	for _, invalid := range []string{"2026-11-08T00:00:00", "2026-10-31T23:59:59", "garbage"} {
		late := append([]session.Value(nil), source.Rows[0]...)
		for i, name := range source.Columns {
			if name == "FIRST_TIME" {
				late[i] = session.Value{Text: invalid}
			}
		}
		bad := source
		bad.Rows = append(bad.Rows, late)
		facts, err := DecodeHistoryWithOptions(bad, options)
		if !errors.Is(err, ErrInvalidEvidence) || facts.Rows != nil {
			t.Fatalf("accepted outside/malformed late history: %+v %v", facts, err)
		}
	}
}
func TestRedoReaderNoTerminalOrCancellationPrefix(t *testing.T) {
	plan := testPlan(t)
	source := sourceResult(plan)
	sentinel := errors.New("private failure")
	reader := &fixtureReader{result: source, err: sentinel}
	facts, err := ReadGroups(context.Background(), reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if !errors.Is(err, sentinel) || facts.Rows != nil {
		t.Fatalf("query prefix: %+v %v", facts, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader = &fixtureReader{result: source, after: cancel}
	facts, err = ReadGroups(ctx, reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if !errors.Is(err, context.Canceled) || facts.Rows != nil {
		t.Fatalf("cancel prefix: %+v %v", facts, err)
	}
}

func TestArchivedLogsRetainDuplicateCopiesAndUnknownFlags(t *testing.T) {
	filter := bounds()
	plan, err := archivedLogsPlan(19, filter)
	if err != nil {
		t.Fatal(err)
	}
	source := sourceResult(plan)
	copyRow := append([]session.Value(nil), source.Rows[0]...)
	for i, name := range source.Columns {
		switch name {
		case "NAME":
			source.Rows[0][i] = session.Value{Null: true}
			copyRow[i] = session.Value{Text: "restored_copy.arc"}
		case "DEST_ID":
			source.Rows[0][i] = session.Value{Text: "0"}
			copyRow[i] = session.Value{Text: "2"}
		case "REGISTRAR":
			source.Rows[0][i] = session.Value{Text: "RFS"}
		case "APPLIED":
			source.Rows[0][i] = session.Value{Text: "IN-MEMORY"}
		case "BLOCKS":
			source.Rows[0][i] = session.Value{Text: "128"}
		case "BLOCK_SIZE":
			source.Rows[0][i] = session.Value{Text: "512"}
		}
	}
	source.Rows = append(source.Rows, copyRow)
	facts, err := DecodeArchivedLogs(source, 19, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Rows) != 2 || !facts.Rows[0].Name.Null || facts.Rows[1].DestinationID.Text != "2" || facts.Rows[0].Blocks.Text != "128" || facts.Rows[0].BlockSize.Text != "512" || facts.Rows[0].Applied.Text != "IN-MEMORY" || facts.Rows[0].NextChange.Text != "18446744073709551615" {
		t.Fatalf("lost archive copies/units/status: %+v", facts)
	}
	reader := &fixtureReader{result: source}
	complete, err := ReadArchivedLogs(context.Background(), reader, HistoryRequest{OracleMajor: 19, Filter: filter, Reference: sourceRef(t)})
	if err != nil || !complete.Provenance.Complete || complete.Provenance.From != "2026-11-01T00:00:00" {
		t.Fatalf("archive read scope %+v %v", complete, err)
	}
	late := append([]session.Value(nil), copyRow...)
	for i, name := range source.Columns {
		if name == "FIRST_TIME" {
			late[i] = session.Value{Text: "2026-11-08T00:00:00"}
		}
	}
	source.Rows = append(source.Rows, late)
	bad, err := DecodeArchivedLogs(source, 19, filter)
	if !errors.Is(err, ErrInvalidEvidence) || bad.Rows != nil {
		t.Fatalf("archive failed prefix %+v %v", bad, err)
	}
}

func TestRedoDateSQLUsesGregorianFieldsAndBoundSubdayOffsets(t *testing.T) {
	filter := bounds()
	filter.From = filter.From.Add(2*time.Hour + 3*time.Minute + 4*time.Second)
	plan, err := historyPlan(19, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.args) != 2 || plan.args[0] != 7384 || strings.Contains(plan.sql, "TO_DATE(") || !strings.Contains(plan.sql, "EXTRACT(YEAR FROM CAST(v.FIRST_TIME AS TIMESTAMP))") || !strings.Contains(plan.sql, "CASE WHEN v.FIRST_TIME IS NULL THEN NULL") {
		t.Fatalf("calendar-sensitive date rendering/bounds: %s %+v", plan.sql, plan.args)
	}
}
