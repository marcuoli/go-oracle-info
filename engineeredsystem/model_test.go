package engineeredsystem

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"slices"
	"strings"
	"testing"
	"time"
)

func testPayload(body string) Payload {
	return Payload{InstanceID: 2, CellHash: "9007199254740993", CellPath: "cell-a", CellType: "EXADATA", Source: Config, SourceType: "PHYSICALDISKS", XML: body, ObservedAt: time.Date(2026, 11, 11, 23, 0, 0, 0, time.UTC)}
}

func TestDecodeXMLRetainsUnknownNamespacedRepeatedAndMixedValues(t *testing.T) {
	payload := testPayload(`<cli-output xmlns:a="urn:vendor" xmlns:b="urn:other"><physicaldisk unit="GB"><name> PD_1 </name><status>normal</status><a:capacity unit="GiB">9007199254740993</a:capacity><a:extra mode="x">one</a:extra><a:extra>two</a:extra><b:extra>other</b:extra><empty/><blank>  </blank><mixed>before<part>middle</part>after</mixed></physicaldisk></cli-output>`)
	properties, err := DecodeXML(payload)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]Property)
	var mixed []string
	for i, p := range properties {
		if p.Ordinal != i+1 || p.Source != Config || p.InstanceID != 2 || p.CellHash != payload.CellHash || p.ObservedAt != payload.ObservedAt {
			t.Fatalf("source identity/order changed: %#v", p)
		}
		values[p.Name] = p
		if strings.Contains(p.Name, "/mixed[") && strings.Contains(p.Name, "text()") {
			mixed = append(mixed, p.Value)
		}
	}
	prefix := "/cli-output[1]/physicaldisk[1]"
	cases := map[string]string{
		"/@{xmlns}a":                                         "urn:vendor",
		"/physicaldisk[1]/name[1]/text()[1]":                 " PD_1 ",
		"/physicaldisk[1]/{urn:vendor}capacity[1]/text()[1]": "9007199254740993",
		"/physicaldisk[1]/{urn:vendor}extra[1]/text()[1]":    "one",
		"/physicaldisk[1]/{urn:vendor}extra[2]/text()[1]":    "two",
		"/physicaldisk[1]/{urn:other}extra[1]/text()[1]":     "other",
		"/physicaldisk[1]/empty[1]/text()[1]":                "",
		"/physicaldisk[1]/blank[1]/text()[1]":                "  ",
	}
	for suffix, want := range cases {
		p, found := values["/cli-output[1]"+suffix]
		if !found || p.Value != want || p.Null {
			t.Fatalf("property %q = %#v, found=%t; want %q", suffix, p, found, want)
		}
	}
	capacity := values[prefix+"/{urn:vendor}capacity[1]/text()[1]"]
	if capacity.Unit != "GiB" || capacity.ComponentType != "physicaldisk" || capacity.ComponentID != "PD_1" || capacity.ReportedStatus != "normal" {
		t.Fatalf("reported component metadata lost: %#v", capacity)
	}
	if values[prefix+"/name[1]/text()[1]"].Unit != "" {
		t.Fatal("parent unit was invented for a name field")
	}
	if strings.Join(mixed, "|") != "before|middle|after" {
		t.Fatalf("mixed text order lost: %v", mixed)
	}
}

func TestDecodeXMLRetainsLargeMultibyteValuesWithoutExcelLengthLimit(t *testing.T) {
	value := strings.Repeat("ação😀&", 9000)
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
		t.Fatal(err)
	}
	payload := testPayload("<cell><unknown>" + escaped.String() + "</unknown></cell>")
	properties, err := DecodeXML(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) != 1 || properties[0].Value != value {
		t.Fatal("large Unicode source value was shortened or changed")
	}
}

func TestDecodeXMLRejectsIncompleteDocumentsWithoutPublishingPrefix(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "truncated": "<cell><name>accepted prefix</name><disk>",
		"two roots": "<cell/><cell/>", "outside text": "stray<cell/>",
		"invalid UTF8": "<cell>\xff</cell>", "invalid character": "<cell>\x00</cell>",
		"external entity": `<!DOCTYPE cell SYSTEM "http://127.0.0.1:1/never-fetch"><cell>&outside;</cell>`,
	} {
		t.Run(name, func(t *testing.T) {
			payload := testPayload(body)
			got, err := DecodeXML(payload)
			if err == nil || got != nil {
				t.Fatalf("invalid XML produced accepted prefix: %#v, %v", got, err)
			}
			if payload.XML != body {
				t.Fatal("failed XML source bytes lost")
			}
		})
	}
}

func TestDecodeXMLNullEmptyEntitiesAndDeepDocuments(t *testing.T) {
	payload := testPayload("")
	payload.Null = true
	got, err := DecodeXML(payload)
	if err != nil || len(got) != 1 || !got[0].Null || got[0].Value != "" {
		t.Fatalf("SQL NULL changed: %#v %v", got, err)
	}
	got, err = DecodeXML(testPayload(`<cell>&lt;&amp;&#x1F600;</cell>`))
	if err != nil || len(got) != 1 || got[0].Value != "<&😀" {
		t.Fatalf("literal XML values changed: %#v %v", got, err)
	}
	body := strings.Repeat("<unknown>", 2048) + "leaf" + strings.Repeat("</unknown>", 2048)
	got, err = DecodeXML(testPayload(body))
	if err != nil || len(got) != 1 || got[0].Value != "leaf" {
		t.Fatalf("deep XML property omitted: %d %v", len(got), err)
	}
}

func completeTestReport() Report {
	at := testPayload("").ObservedAt
	return Report{
		Cells:      []Cell{{InstanceID: 1, Hash: "7", Path: "cell-a", Type: "EXADATA"}, {InstanceID: 2, Hash: "7", Path: "cell-a", Type: "EXADATA"}},
		Properties: []Property{{InstanceID: 1, CellHash: "7", Name: "capacity", Value: "9007199254740993", Source: Config, ObservedAt: at}},
		Payloads:   []Payload{testPayload("<cell/>")},
		Sources:    []Coverage{{Source: Cells, State: Available, View: "SYS.GV_$CELL", ObservedAt: at, RowsRead: 2, RowsRetained: 2}, {Source: Config, State: Available, View: "SYS.V_$CELL_CONFIG", ObservedAt: at, RowsRead: 1, RowsRetained: 1}},
	}
}

func TestReportClonePreservesDuplicateInstanceAndSourceEvidence(t *testing.T) {
	report := completeTestReport()
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := report.Clone()
	clone.Cells[0].Path = "changed"
	clone.Properties[0].Value = "changed"
	clone.Payloads[0].XML = "changed"
	clone.Sources[0].RowsRetained = 0
	if report.Cells[0].Path != "cell-a" || report.Properties[0].Value != "9007199254740993" || report.Payloads[0].XML != "<cell/>" || report.Sources[0].RowsRetained != 2 {
		t.Fatal("cloned report shares mutable slices")
	}
	if len(report.Cells) != 2 {
		t.Fatal("same cell seen through separate instances was discarded")
	}
}

func TestReportValidationPreservesMalformedPayloadAlongsideIndependentRows(t *testing.T) {
	report := completeTestReport()
	report.Payloads = append(report.Payloads, testPayload("<invalid>\xff"))
	report.Sources[1].State = Failed
	report.Sources[1].Reason = ReasonInvalidPayload
	report.Sources[1].Error = "one XML payload rejected; raw source retained"
	report.Sources[1].RowsRead = 2
	report.Sources[1].RowsRetained = 1
	report.Sources[1].RowsRejected = 1
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(report.Properties) != 1 || report.Payloads[1].XML != "<invalid>\xff" {
		t.Fatal("invalid document discarded independent evidence or its raw bytes")
	}
}

func TestReportValidationRejectsInconsistentMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*Report){
		"unknown source":                       func(r *Report) { r.Sources[0].Source = "invented" },
		"unknown state":                        func(r *Report) { r.Sources[0].State = "invented" },
		"unknown reason":                       func(r *Report) { r.Sources[0].Reason = "invented" },
		"duplicate source coverage":            func(r *Report) { r.Sources = append(r.Sources, r.Sources[0]) },
		"negative counts":                      func(r *Report) { r.Sources[0].RowsRead = -1 },
		"overlapping accepted/rejected counts": func(r *Report) { r.Sources[0].RowsRejected = 1 },
		"hidden actual error":                  func(r *Report) { r.Sources[0].Error = "failed query" },
		"failed state lacks error":             func(r *Report) { r.Sources[0].State = Failed },
		"unavailable source has accepted rows": func(r *Report) { r.Sources[0].State = Unsupported },
		"available source claims unavailable":  func(r *Report) { r.Sources[0].Reason = ReasonNotVisible },
		"orphan property":                      func(r *Report) { r.Properties[0].Source = OpenAlerts },
		"NULL has value":                       func(r *Report) { r.Properties[0].Null = true },
		"invalid source metadata UTF8":         func(r *Report) { r.Properties[0].Name = "\xff" },
	} {
		t.Run(name, func(t *testing.T) {
			report := completeTestReport()
			mutate(&report)
			if err := report.Validate(); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
	if err := (Report{}).Validate(); err != nil {
		t.Fatalf("not-collected empty report invalid: %v", err)
	}
}

func TestReportJSONRoundTripPreservesRejectedXMLBytesAndSQLNull(t *testing.T) {
	report := completeTestReport()
	report.Payloads = append(report.Payloads, testPayload("<invalid>\xff\x00"), Payload{Source: Config, SourceType: "CELL", Null: true})
	report.Sources[1].State = Failed
	report.Sources[1].Reason = ReasonInvalidPayload
	report.Sources[1].Error = "rejected XML retained"
	report.Sources[1].RowsRead = 3
	report.Sources[1].RowsRetained = 2
	report.Sources[1].RowsRejected = 1
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored Report
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	for i := range report.Payloads {
		if restored.Payloads[i] != report.Payloads[i] {
			t.Fatalf("payload %d changed through snapshot JSON: got %#v want %#v", i, restored.Payloads[i], report.Payloads[i])
		}
	}
	if bytes.Contains(data, []byte(`"XML":`)) || !bytes.Contains(data, []byte(`"xml_bytes"`)) {
		t.Fatal("rejected raw XML was replaced instead of byte-encoded")
	}
	for _, malformed := range []string{`{}`, `{"xml_bytes":null}`} {
		var missing Payload
		if err := json.Unmarshal([]byte(malformed), &missing); err == nil {
			t.Fatal("missing encoded XML bytes accepted")
		}
	}
	var payload Payload
	if err := json.Unmarshal([]byte(`{"xml_bytes":"not-base64!"}`), &payload); err == nil {
		t.Fatal("invalid encoded XML bytes accepted")
	}
}

func TestDecodeXMLDoesNotSelectWinningConflictingIdentityOrStatus(t *testing.T) {
	properties, err := DecodeXML(testPayload(`<cell><name>cell-a</name><status>normal</status><status>failed</status><physicaldisk><name>disk-a</name><name>disk-b</name><size>42</size></physicaldisk></cell>`))
	if err != nil {
		t.Fatal(err)
	}
	var statuses, names []string
	for _, p := range properties {
		if strings.Contains(p.Name, "/status[") {
			statuses = append(statuses, p.Value)
		}
		if p.ComponentType == "physicaldisk" {
			if p.ComponentID != "/cell[1]/physicaldisk[1]" {
				t.Fatalf("conflicting component name selected: %#v", p)
			}
			if strings.Contains(p.Name, "/name[") {
				names = append(names, p.Value)
			}
		}
		if p.ReportedStatus != "" {
			t.Fatalf("conflicting component status selected: %#v", p)
		}
	}
	if strings.Join(statuses, "|") != "normal|failed" || strings.Join(names, "|") != "disk-a|disk-b" {
		t.Fatalf("conflict evidence lost: %v %v", statuses, names)
	}
}

func TestReportRetainsPerPayloadRejectedLengthAndReasonThroughJSON(t *testing.T) {
	report := completeTestReport()
	bad := testPayload("<cell/>")
	bad.Rejected = true
	bad.Reason = ReasonInvalidPayload
	bad.Error = "complete XML/CLOB length mismatch: native=999 UTF16-units=7"
	bad.NativeLengthKnown = true
	bad.NativeLength = 999
	report.Payloads = append(report.Payloads, bad)
	report.Sources[1].State = Failed
	report.Sources[1].Reason = ReasonInvalidPayload
	report.Sources[1].Error = bad.Error
	report.Sources[1].RowsRead = 2
	report.Sources[1].RowsRetained = 1
	report.Sources[1].RowsRejected = 1
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored Report
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if restored.Payloads[1] != bad || restored.Payloads[0].Rejected {
		t.Fatal("raw rejected prefix lost its own failure/length identity")
	}
	invalid := report.Clone()
	invalid.Payloads[1].Error = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("rejected XML lost error detail")
	}
	invalid = report.Clone()
	invalid.Payloads[0].Error = "hidden rejection"
	if err := invalid.Validate(); err == nil {
		t.Fatal("accepted XML silently carried an error")
	}
}

func TestAvailableCurrentInstanceLimitationIsNotAFailure(t *testing.T) {
	report := completeTestReport()
	report.Sources[0].Reason = ReasonCurrentInstanceOnly
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(Reasons(), ReasonCurrentInstanceOnly) {
		t.Fatal("current-instance limitation absent from registry")
	}
}

func TestFailedSourceRetainsIndependentLocalInstanceScopeThroughJSON(t *testing.T) {
	report := completeTestReport()
	report.Cells = report.Cells[:1]
	coverage := &report.Sources[0]
	coverage.View = "SYS.V_$CELL"
	coverage.InstanceLocal = true
	coverage.State = Failed
	coverage.Reason = ReasonReadFailed
	coverage.Error = "a later local source row was rejected"
	coverage.RowsRetained = 1
	coverage.RowsRejected = 1
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored Report
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := restored.Clone()
	if !clone.Sources[0].InstanceLocal || clone.Sources[0].Reason != ReasonReadFailed || clone.Sources[0].State != Failed {
		t.Fatal("actual failure erased current-instance source scope")
	}
	clone.Sources[0].InstanceLocal = false
	if !restored.Sources[0].InstanceLocal {
		t.Fatal("cloned source scope shares mutable storage")
	}
}

func TestCellSQLNullFlagsSurviveCloneAndSnapshotJSON(t *testing.T) {
	report := completeTestReport()
	report.Cells = []Cell{
		{InstanceID: 1, HashNull: true, Path: "cell-a", TypeNull: true},
		{InstanceID: 2, Hash: "7", PathNull: true, Type: "EXADATA"},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := report.Clone()
	clone.Cells[0].HashNull = false
	clone.Cells[0].TypeNull = false
	clone.Cells[1].PathNull = false
	if !report.Cells[0].HashNull || !report.Cells[0].TypeNull || !report.Cells[1].PathNull {
		t.Fatal("clone mutated original cell SQL NULL flags")
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored Report
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(restored.Cells, report.Cells) {
		t.Fatalf("cell SQL NULL evidence changed through snapshot: got %#v want %#v", restored.Cells, report.Cells)
	}
}

func TestCellSQLNullFlagsRejectContradictoryValuesWithoutRelaxingIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*Cell){
		"NULL hash has value":                             func(c *Cell) { c.HashNull = true },
		"NULL path has value":                             func(c *Cell) { c.PathNull = true },
		"NULL type has value":                             func(c *Cell) { c.TypeNull = true },
		"both identity descriptors NULL":                  func(c *Cell) { c.Hash = ""; c.Path = ""; c.HashNull = true; c.PathNull = true },
		"NULL type does not permit invalid UTF8 identity": func(c *Cell) { c.Type = ""; c.TypeNull = true; c.Path = "\xff" },
	} {
		t.Run(name, func(t *testing.T) {
			report := completeTestReport()
			mutate(&report.Cells[0])
			if err := report.Validate(); err == nil {
				t.Fatal("contradictory or invalid nullable cell identity accepted")
			}
		})
	}
}

func TestLUNComponentsRetainIndependentNativeNames(t *testing.T) {
	xml := "<cli-output><lun><name>0_0</name><lunSize>9795998318592</lunSize><physicalDrives>drive-one,drive-two</physicalDrives><status>normal</status></lun><lun><name>0_1</name><lunSize>6401246101504</lunSize><status>warning</status></lun></cli-output>"
	properties, err := DecodeXML(Payload{InstanceID: 1, Source: Config, SourceType: "LUNS", XML: xml})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range properties {
		if p.ComponentType != "lun" {
			t.Fatalf("LUN inherited document identity: %#v", p)
		}
		names[p.ComponentID] = true
		if p.Unit != "" {
			t.Fatal("unit invented from an unlabelled native value")
		}
		if p.ComponentID == "0_0" && p.ReportedStatus != "normal" {
			t.Fatal("reported status changed")
		}
		if p.ComponentID == "0_1" && p.ReportedStatus != "warning" {
			t.Fatal("sibling status inherited")
		}
	}
	if len(names) != 2 || !names["0_0"] || !names["0_1"] {
		t.Fatal("independent LUN identity lost")
	}
}
