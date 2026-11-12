//go:build integration

package engineeredsystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// Private operator-output replay is development verification, not an XML import
// feature. Display-derived documents do not qualify live driver/CLOB collection.
func TestReplayObserved122CellDocuments(t *testing.T) {
	directory := os.Getenv("ORACLE_INFO_ENGINEERED_DISPLAY_REPLAY")
	if directory == "" {
		t.Skip("requires preserved private operator-output inspection")
	}
	data, err := os.ReadFile(filepath.Join(directory, "inspection.json"))
	if err != nil {
		t.Fatal("cannot read private replay inspection")
	}
	var manifest struct {
		Payloads []struct {
			Section            string `json:"section"`
			Type               string `json:"configuration_type"`
			Length             int64  `json:"native_length"`
			File               string `json:"display_derived_file"`
			ExpectedProperties int    `json:"expected_property_count"`
			WellFormed         bool   `json:"well_formed"`
			LengthMatches      bool   `json:"native_length_matches_lf_variant"`
		} `json:"payloads"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal("invalid private replay inspection")
	}
	accepted, rejected, properties := 0, 0, 0
	for _, record := range manifest.Payloads {
		if record.File != filepath.Base(record.File) || !strings.HasSuffix(record.File, ".xml") {
			t.Fatal("unsafe replay document path")
		}
		body, err := os.ReadFile(filepath.Join(directory, record.File))
		if err != nil {
			t.Fatal("cannot read private replay document")
		}
		xml := string(body)
		units := int64(len(utf16.Encode([]rune(xml))))
		// Compare explicit LF display variants, including an optional final LF.
		// This replay convention does not certify original CLOB bytes.
		if units+1 == record.Length {
			xml += "\n"
			units++
		}
		source := Config
		if record.Section == "=== 5" {
			source = ConfigInfo
		} else if record.Section != "=== 4" {
			t.Fatal("unknown replay source section")
		}
		payload := Payload{InstanceID: 1, Source: source, SourceType: record.Type, XML: xml, NativeLengthKnown: true, NativeLength: record.Length}
		decoded, decodeErr := DecodeXML(payload)
		expected := record.WellFormed && record.LengthMatches
		if expected {
			if decodeErr != nil || units != record.Length {
				t.Fatalf("complete display-derived %s document rejected (%T)", record.Type, decodeErr)
			}
			if len(decoded) != record.ExpectedProperties {
				t.Fatalf("native XML leaf/attribute count changed: got=%d expected=%d", len(decoded), record.ExpectedProperties)
			}
			for _, property := range decoded {
				if property.Source != source || property.SourceType != record.Type {
					t.Fatal("replay source provenance changed")
				}
			}
			accepted++
			properties += len(decoded)
		} else {
			if decodeErr == nil || len(decoded) != 0 || units == record.Length {
				t.Fatal("clipped client text became apparently complete evidence")
			}
			rejected++
		}
	}
	if accepted != 27 || rejected != 6 {
		t.Fatalf("recorded dataset changed: accepted=%d rejected=%d", accepted, rejected)
	}
	t.Logf("Private Oracle 12.2 client-output replay: %d length-matching complete XML documents, %d clipped documents rejected, %d properties; no live collection or original-byte certification", accepted, rejected, properties)
}
