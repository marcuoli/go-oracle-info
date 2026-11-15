package asm

import (
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
		{name: "unselected-instance", plan: func() (selection, error) {
			return clientsPlan(19, Filter{Instances: []int64{1}, SourceRole: DatabaseInstance})
		}, decode: func(r session.Result) error {
			_, e := DecodeClients(r, 19, Filter{Instances: []int64{1}, SourceRole: DatabaseInstance})
			return e
		}, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return ClientsJSON(context.Background(), q, 19, Filter{Instances: []int64{1}, SourceRole: DatabaseInstance}, limits)
		}, column: "INST_ID", value: "2"},
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

func TestJSONDomainValidationPreservesRawASMVersionAndNull(t *testing.T) {
	filter := Filter{SourceRole: DatabaseInstance}
	plan, _ := clientsPlan(19, filter)
	step := sourceStep(plan)
	index := map[string]int{}
	for i, c := range plan.schema {
		index[c.Name] = i
		switch c.Name {
		case "SOFTWARE_VERSION":
			step.Values[0][i] = "19.10.0.0.0"
		case "COMPATIBLE_VERSION":
			step.Values[0][i] = nil
		case "STATUS":
			step.Values[0][i] = "FUTURE_STATUS"
		}
	}
	db, _ := testsql.New(t, step)
	payload, err := ClientsJSON(context.Background(), db, 19, filter, rowset.Limits{MaxRows: 1, MaxBytes: 16384})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data struct{ Rows [][]any } }
	if err = json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	rows := envelope.Data.Rows
	if len(rows) != 1 || rows[0][index["SOFTWARE_VERSION"]] != "19.10.0.0.0" || rows[0][index["COMPATIBLE_VERSION"]] != nil || rows[0][index["STATUS"]] != "FUTURE_STATUS" {
		t.Fatalf("normalized raw connection facts: %s", payload)
	}
}
