package redo

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"testing"
)

func TestJSONDomainAdmissionParity(t *testing.T) {
	limits := rowset.Limits{MaxRows: 4, MaxBytes: 32768}
	cases := []struct {
		name          string
		plan          func() (selection, error)
		decode        func(session.Result) error
		json          func(rowset.Queryer) (json.RawMessage, error)
		column, value string
		single        bool
	}{
		{name: "groups", plan: func() (selection, error) { return groupsPlan(19, Filter{}) }, decode: func(r session.Result) error { _, e := DecodeGroups(r, 19, Filter{}); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return GroupsJSON(context.Background(), q, 19, Filter{}, limits)
		}, column: "NEXT_CHANGE", value: "not-an-scn"},
		{name: "members", plan: func() (selection, error) { return membersPlan(19, Filter{Instances: []int64{1}}) }, decode: func(r session.Result) error { _, e := DecodeMembers(r, 19, Filter{Instances: []int64{1}}); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return MembersJSON(context.Background(), q, 19, Filter{Instances: []int64{1}}, limits)
		}, column: "INST_ID", value: "2"},
		{name: "history", plan: func() (selection, error) { return historyPlan(19, bounds()) }, decode: func(r session.Result) error { _, e := DecodeHistory(r, 19, bounds()); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return HistoryJSON(context.Background(), q, 19, bounds(), limits)
		}, column: "FIRST_TIME", value: "2026-11-08T00:00:00"},
		{name: "archivedLogs", plan: func() (selection, error) { return archivedLogsPlan(19, bounds()) }, decode: func(r session.Result) error { _, e := DecodeArchivedLogs(r, 19, bounds()); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return ArchivedLogsJSON(context.Background(), q, 19, bounds(), limits)
		}, column: "RESETLOGS_CHANGE", value: "not-an-scn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.plan()
			if err != nil {
				t.Fatal(err)
			}
			source := sourceResult(plan)
			step := sourceStep(plan)
			validDB, validState := testsql.New(t, sourceStep(plan))
			validPayload, validErr := tc.json(validDB)
			if validErr != nil || validPayload == nil {
				t.Fatalf("matching JSON path rejected admitted source: %v", validErr)
			}
			var validEnvelope struct {
				Provenance Provenance `json:"provenance"`
			}
			if err = json.Unmarshal(validPayload, &validEnvelope); err != nil || !validEnvelope.Provenance.Complete || validEnvelope.Provenance.Source != plan.provenance.Source || validState.Closed() != 1 || len(validState.Calls()) != 1 {
				t.Fatalf("valid JSON lost source/completion/one-cursor contract: %s %v", validPayload, err)
			}

			// A valid complete source must remain accepted before corrupting its last row.
			if err = tc.decode(source); err != nil {
				t.Fatalf("invalid fixture before mutation: %v", err)
			}
			badRow := append([]session.Value(nil), source.Rows[0]...)
			badNative := append([]driver.Value(nil), step.Values[0]...)
			found := false
			for i, name := range source.Columns {
				if name == tc.column {
					badRow[i] = session.Value{Text: tc.value}
					badNative[i] = tc.value
					found = true
				}
			}
			if !found {
				t.Fatalf("missing mutation column %s", tc.column)
			}
			if tc.single {
				source.Rows[0] = badRow
				step.Values[0] = badNative
			} else {
				source.Rows = append(source.Rows, badRow)
				step.Values = append(step.Values, badNative)
			}
			if err = tc.decode(source); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("decoder accepted invalid domain evidence: %v", err)
			}
			db, state := testsql.New(t, step)
			payload, err := tc.json(db)
			if !errors.Is(err, ErrInvalidEvidence) || payload != nil {
				t.Fatalf("JSON diverged from decoder and published invalid source: %s, %v", payload, err)
			}
			if state.Closed() != 1 || len(state.Calls()) != 1 {
				t.Fatalf("expected one finalized cursor, got closes=%d calls=%d", state.Closed(), len(state.Calls()))
			}
		})
	}
}

func TestJSONDomainValidationPreservesSCNsNullAndObservationOrder(t *testing.T) {
	plan, _ := groupsPlan(19, Filter{})
	step := sourceStep(plan)
	second := append([]driver.Value(nil), step.Values[0]...)
	for i, c := range plan.schema {
		switch c.Name {
		case "INST_ID":
			step.Values[0][i] = "1"
			second[i] = "2"
		case "BYTES":
			step.Values[0][i] = "1099511627777"
		case "ARCHIVED":
			step.Values[0][i] = nil
		case "STATUS":
			step.Values[0][i] = "raw\"status\\value"
		}
	}
	step.Values = append(step.Values, second)
	db, _ := testsql.New(t, step)
	payload, err := GroupsJSON(context.Background(), db, 19, Filter{}, rowset.Limits{MaxRows: 2, MaxBytes: 16384})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data struct{ Rows [][]any } }
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err = decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	index := map[string]int{}
	for i, c := range plan.schema {
		index[c.Name] = i
	}
	rows := envelope.Data.Rows
	if len(rows) != 2 || rows[0][index["INST_ID"]] != json.Number("1") || rows[1][index["INST_ID"]] != json.Number("2") || rows[0][index["BYTES"]] != json.Number("1099511627777") || rows[0][index["NEXT_CHANGE"]] != "18446744073709551615" || rows[0][index["ARCHIVED"]] != nil || rows[0][index["STATUS"]] != "raw\"status\\value" {
		t.Fatalf("domain validation changed native JSON meaning: %s", payload)
	}
}
