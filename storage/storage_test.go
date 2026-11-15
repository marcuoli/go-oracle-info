package storage

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"strings"
	"testing"
)

func TestStorageExactValuesAndOptionalScope(t *testing.T) {
	plan, err := tempfilesPlan(TempfileOptions{OracleMajor: 19, IncludeLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	source := sourceResult(plan)
	for i, name := range source.Columns {
		switch name {
		case "BYTES":
			source.Rows[0][i] = session.Value{Text: "1099511627777"}
		case "INCREMENT_BY":
			source.Rows[0][i] = session.Value{Text: "128"}
		case "INST_ID", "CON_ID":
			source.Rows[0][i] = session.Value{Null: true}
		case "FILE_NAME":
			source.Rows[0][i] = session.Value{Text: "+DATA/quoted\"file.dbf"}
		}
	}
	facts, err := DecodeTempfiles(source, TempfileOptions{OracleMajor: 19, IncludeLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	v := facts.Rows[0]
	if facts.Provenance.Complete || v.Bytes.Text != "1099511627777" || v.IncrementBy.Text != "128" || v.OwningInstanceID == nil || !v.OwningInstanceID.Null || v.ContainerID == nil || !v.ContainerID.Null || v.ReportingInstanceID.Text != "1" {
		t.Fatalf("lost exact/null/scope meaning: %+v", facts)
	}
	legacy, _ := tempfilesPlan(TempfileOptions{OracleMajor: 11})
	old, err := DecodeTempfiles(sourceResult(legacy), TempfileOptions{OracleMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	if old.Rows[0].ContainerID != nil || old.Rows[0].Shared != nil || old.Rows[0].OwningInstanceID != nil {
		t.Fatal("fabricated legacy optional fields")
	}
	if _, err = TempfilesStatement(TempfileOptions{OracleMajor: 12, IncludeLocal: true}); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("unsupported local-temp columns: %v", err)
	}
}
func TestAllStorageDecodersRejectMalformedLateRows(t *testing.T) {
	cases := []struct {
		name   string
		plan   func() (selection, error)
		decode func(session.Result) error
	}{
		{"tablespaces", func() (selection, error) { return tablespacesPlan(19) }, func(r session.Result) error { _, e := DecodeTablespaces(r, 19); return e }},
		{"datafiles", func() (selection, error) { return datafilesPlan(19) }, func(r session.Result) error { _, e := DecodeDatafiles(r, 19); return e }},
		{"tempfiles", func() (selection, error) { return tempfilesPlan(TempfileOptions{OracleMajor: 19}) }, func(r session.Result) error { _, e := DecodeTempfiles(r, TempfileOptions{OracleMajor: 19}); return e }},
		{"free", func() (selection, error) { return freeExtentsPlan(19) }, func(r session.Result) error { _, e := DecodeFreeExtents(r, 19); return e }},
		{"segments", func() (selection, error) { return segmentsPlan(19) }, func(r session.Result) error { _, e := DecodeSegments(r, 19); return e }},
		{"long", func() (selection, error) { return longColumnsPlan(19) }, func(r session.Result) error { _, e := DecodeLongColumns(r, 19); return e }},
		{"property", func() (selection, error) { return defaultTemporaryPlan(19) }, func(r session.Result) error { _, e := DecodeDefaultTemporary(r, 19); return e }},
		{"sga", func() (selection, error) { return sgaComponentsPlan(19, InstanceFilter{}) }, func(r session.Result) error { _, e := DecodeSGAComponents(r, 19, InstanceFilter{}); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.plan()
			if err != nil {
				t.Fatal(err)
			}
			source := sourceResult(plan)
			if err = tc.decode(source); err != nil {
				t.Fatal(err)
			}
			late := append([]session.Value(nil), source.Rows[0]...)
			late[0] = session.Value{Text: "1.5"}
			source.Rows = append(source.Rows, late)
			if err = tc.decode(source); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("accepted malformed late row: %v", err)
			}
		})
	}
}
func TestStorageSourcesKeepOwnerAndAllocationPolicyOut(t *testing.T) {
	sql, err := SegmentsStatement(11)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "CON_ID") || strings.Contains(sql, "SUM(") || strings.Contains(sql, "ROWNUM") || strings.Contains(sql, "NOT IN") || !strings.Contains(sql, "PARTITION_NAME") {
		t.Fatalf("policy leaked into segment facts: %s", sql)
	}
	sql, _ = LongColumnsStatement(19)
	if !strings.Contains(sql, "IN ('LONG','LONG RAW')") || strings.Contains(sql, "LIKE") {
		t.Fatalf("ambiguous LONG attribution: %s", sql)
	}
	plan, _ := sgaComponentsPlan(19, InstanceFilter{Instances: []int64{2, 5}})
	if len(plan.args) != 2 || !strings.Contains(plan.sql, "(v.NAME = 'buffer_cache' OR v.POOL = 'shared pool') AND v.INST_ID IN (:1,:2)") {
		t.Fatalf("SGA instance scope/precedence: %s %+v", plan.sql, plan.args)
	}
	source := sourceResult(plan)
	if facts, err := DecodeSGAComponents(source, 19, InstanceFilter{Instances: []int64{2, 5}}); err == nil || facts.Rows != nil {
		t.Fatal("accepted outside selected reporting instances")
	}
}
func TestStorageReadersNeverPublishFailedPrefix(t *testing.T) {
	plan := testPlan(t)
	source := sourceResult(plan)
	sentinel := errors.New("private failure")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, tc := range []struct {
		name   string
		reader *fixtureReader
		ctx    context.Context
		cause  error
	}{{"late-query", &fixtureReader{result: source, err: sentinel}, context.Background(), sentinel}, {"cancel", &fixtureReader{result: source, after: cancel}, ctx, context.Canceled}} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := ReadDatafiles(tc.ctx, tc.reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
			if !errors.Is(err, tc.cause) || facts.Rows != nil || facts.Provenance.Complete {
				t.Fatalf("failed prefix: %+v %v", facts, err)
			}
		})
	}
	reader := &fixtureReader{result: source}
	ref := sourceRef(t)
	facts, err := ReadDatafiles(context.Background(), reader, Request{OracleMajor: 19, Reference: ref})
	if err != nil || !facts.Provenance.Complete || len(reader.calls) != 1 || reader.calls[0].Reference != ref {
		t.Fatalf("lost complete/reference contract: %+v %v", facts, err)
	}
}

func TestTemporaryGroupsPreserveDefaultGroupMembership(t *testing.T) {
	plan, _ := temporaryGroupsPlan(19)
	source := sourceResult(plan)
	source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
	for i, name := range source.Columns {
		if name == "TABLESPACE_NAME" {
			source.Rows[0][i] = session.Value{Text: "TEMP_A"}
			source.Rows[1][i] = session.Value{Text: "TEMP_B"}
		}
	}
	facts, err := DecodeTemporaryGroups(source, 19)
	if err != nil || len(facts.Rows) != 2 || facts.Rows[0].TablespaceName.Text != "TEMP_A" || facts.Rows[1].TablespaceName.Text != "TEMP_B" {
		t.Fatalf("group members collapsed: %+v %v", facts, err)
	}
	reader := &fixtureReader{result: source}
	complete, err := ReadTemporaryGroups(context.Background(), reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if err != nil || !complete.Provenance.Complete {
		t.Fatalf("group read incomplete %+v %v", complete, err)
	}
}

func TestEmptyStorageReadIsCompleteButNotZeroCapacity(t *testing.T) {
	plan := testPlan(t)
	source := sourceResult(plan)
	source.Rows = nil
	reader := &fixtureReader{result: source}
	facts, err := ReadDatafiles(context.Background(), reader, Request{OracleMajor: 19, Reference: sourceRef(t)})
	if err != nil || !facts.Provenance.Complete || facts.Rows == nil || len(facts.Rows) != 0 {
		t.Fatalf("empty/unavailable conflated: %+v %v", facts, err)
	}
}
func TestSingularTemporaryPropertyJSONRejectsExtraSourceRows(t *testing.T) {
	plan, err := defaultTemporaryPlan(19)
	if err != nil {
		t.Fatal(err)
	}
	step := sourceStep(plan)
	step.Values = append(step.Values, step.Values[0])
	db, state := testsql.New(t, step)
	payload, err := DefaultTemporaryJSON(context.Background(), db, 19, rowset.Limits{MaxRows: 5, MaxBytes: 16384})
	if !errors.Is(err, rowset.ErrCapacity) || payload != nil || state.Closed() != 1 {
		t.Fatalf("singular property prefix: %s %v closes=%d", payload, err, state.Closed())
	}
}
