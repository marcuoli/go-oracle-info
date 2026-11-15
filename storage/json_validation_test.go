package storage

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
		{name: "tablespaces", plan: func() (selection, error) { return tablespacesPlan(19) }, decode: func(r session.Result) error { _, e := DecodeTablespaces(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return TablespacesJSON(context.Background(), q, 19, limits)
		}, column: "BLOCK_SIZE", value: "-1", single: false},
		{name: "datafiles", plan: func() (selection, error) { return datafilesPlan(19) }, decode: func(r session.Result) error { _, e := DecodeDatafiles(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return DatafilesJSON(context.Background(), q, 19, limits)
		}, column: "SOURCE_INST_ID", value: "0", single: false},
		{name: "tempfiles", plan: func() (selection, error) { return tempfilesPlan(TempfileOptions{OracleMajor: 19}) }, decode: func(r session.Result) error { _, e := DecodeTempfiles(r, TempfileOptions{OracleMajor: 19}); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return TempfilesJSON(context.Background(), q, TempfileOptions{OracleMajor: 19}, limits)
		}, column: "SOURCE_INST_ID", value: "0", single: false},
		{name: "freeExtents", plan: func() (selection, error) { return freeExtentsPlan(19) }, decode: func(r session.Result) error { _, e := DecodeFreeExtents(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return FreeExtentsJSON(context.Background(), q, 19, limits)
		}, column: "BYTES", value: "-1", single: false},
		{name: "segments", plan: func() (selection, error) { return segmentsPlan(19) }, decode: func(r session.Result) error { _, e := DecodeSegments(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return SegmentsJSON(context.Background(), q, 19, limits)
		}, column: "BYTES", value: "-1", single: false},
		{name: "longColumns", plan: func() (selection, error) { return longColumnsPlan(19) }, decode: func(r session.Result) error { _, e := DecodeLongColumns(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return LongColumnsJSON(context.Background(), q, 19, limits)
		}, column: "DATA_TYPE", value: "VARCHAR2", single: false},
		{name: "defaultTemporary", plan: func() (selection, error) { return defaultTemporaryPlan(19) }, decode: func(r session.Result) error { _, e := DecodeDefaultTemporary(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return DefaultTemporaryJSON(context.Background(), q, 19, limits)
		}, column: "PROPERTY_NAME", value: "OTHER_PROPERTY", single: true},
		{name: "temporaryGroups", plan: func() (selection, error) { return temporaryGroupsPlan(19) }, decode: func(r session.Result) error { _, e := DecodeTemporaryGroups(r, 19); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return TemporaryGroupsJSON(context.Background(), q, 19, limits)
		}, column: "SOURCE_INST_ID", value: "0", single: false},
		{name: "sgaComponents", plan: func() (selection, error) { return sgaComponentsPlan(19, InstanceFilter{}) }, decode: func(r session.Result) error { _, e := DecodeSGAComponents(r, 19, InstanceFilter{}); return e }, json: func(q rowset.Queryer) (json.RawMessage, error) {
			return SGAComponentsJSON(context.Background(), q, 19, InstanceFilter{}, limits)
		}, column: "NAME", value: "unselected-component", single: false},
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

func TestJSONDomainValidationPreservesTempfileUnitsAndOptionalNull(t *testing.T) {
	options := TempfileOptions{OracleMajor: 19, IncludeLocal: true}
	plan, _ := tempfilesPlan(options)
	step := sourceStep(plan)
	index := map[string]int{}
	for i, c := range plan.schema {
		index[c.Name] = i
		switch c.Name {
		case "BYTES":
			step.Values[0][i] = "1099511627777"
		case "INCREMENT_BY":
			step.Values[0][i] = "128"
		case "CON_ID", "INST_ID":
			step.Values[0][i] = nil
		case "SHARED":
			step.Values[0][i] = "SHARED"
		}
	}
	db, _ := testsql.New(t, step)
	payload, err := TempfilesJSON(context.Background(), db, options, rowset.Limits{MaxRows: 1, MaxBytes: 16384})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data struct{ Rows [][]any } }
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err = decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	rows := envelope.Data.Rows
	if len(rows) != 1 || rows[0][index["BYTES"]] != json.Number("1099511627777") || rows[0][index["INCREMENT_BY"]] != json.Number("128") || rows[0][index["SOURCE_INST_ID"]] != json.Number("1") || rows[0][index["INST_ID"]] != nil || rows[0][index["CON_ID"]] != nil || rows[0][index["SHARED"]] != "SHARED" {
		t.Fatalf("lost tempfile unit/scope/NULL meaning: %s", payload)
	}
}
