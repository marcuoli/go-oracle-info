package releases

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func encodeRecommendationSnapshot(t *testing.T, sections ...PatchRecommendationSet) []byte {
	t.Helper()
	raw, err := json.Marshal(struct {
		SchemaVersion int                      `json:"schema_version"`
		Sections      []PatchRecommendationSet `json:"sections"`
	}{1, sections})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testRecommendationSource() PatchRecommendationSource {
	return PatchRecommendationSource{
		Document: Source{
			ID: "KB100000", OldDocID: "100.1", Title: "Synthetic review", URL: "https://support.oracle.com/support/?kmContentId=100",
			Revision: "Nov 3, 2026", CheckedUTC: "2026-11-10T00:00:00Z", SHA256: strings.Repeat("a", 64),
		},
		CapturedUTC: "2026-11-09T00:00:00Z",
	}
}

func TestPatchRecommendationsKeepExactScopeRowsAndSource(t *testing.T) {
	scope := PatchRecommendationScope{"19", "19.28", DatabaseHome}
	giScope := PatchRecommendationScope{"19", "19.28", GridInfrastructureHome}
	emptyScope := PatchRecommendationScope{"26", "23.26.4", DatabaseHome}
	source := testRecommendationSource()
	archived := source
	archived.CapturedUTC = ""
	archived.ArchiveCommit = strings.Repeat("b", 40)
	archived.Document.Revision = ""
	sections := []PatchRecommendationSet{
		{scope, archived, []PatchRecommendation{
			{Number: 10010000},
			{Number: 10010001, FixedInRU: "19.28", FixedInMRP: "DBMRP 19.28.0.0.250801", PlatformFamily: "Linux"},
			{Number: 10010000, FixedInRU: "retained source value"},
		}},
		{giScope, source, []PatchRecommendation{{Number: 10020000}}},
		{emptyScope, source, []PatchRecommendation{}},
	}
	catalog, err := LoadPatchRecommendations(bytes.NewReader(encodeRecommendationSnapshot(t, sections...)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.Lookup(scope)
	if err != nil || got.Scope != scope || got.Source != archived || len(got.Rows) != 3 ||
		got.Rows[0] != sections[0].Rows[0] || got.Rows[1] != sections[0].Rows[1] || got.Rows[2] != sections[0].Rows[2] {
		t.Fatalf("unexpected exact DB result: %+v, %v", got, err)
	}
	got.Rows[0].Number = 1
	again, err := catalog.Lookup(scope)
	if err != nil || again.Rows[0].Number != 10010000 {
		t.Fatalf("lookup leaked catalog mutation: %+v, %v", again, err)
	}
	gi, err := catalog.Lookup(giScope)
	if err != nil || len(gi.Rows) != 1 || gi.Rows[0].Number != 10020000 {
		t.Fatalf("same-train GI result = %+v, %v", gi, err)
	}
	empty, err := catalog.Lookup(emptyScope)
	if err != nil || empty.Source != source || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("explicit empty table = %+v, %v", empty, err)
	}
	for _, absent := range []PatchRecommendationScope{
		{"19", "19.29", DatabaseHome},
		{"26", "23.26.4", GridInfrastructureHome},
		{"23", "23.9", DatabaseHome},
	} {
		if _, err := catalog.Lookup(absent); !errors.Is(err, ErrPatchRecommendationCoverageUnavailable) {
			t.Errorf("absent section %v = %v", absent, err)
		}
	}
	for _, invalid := range []PatchRecommendationScope{
		{"19", "19.28.0.0.250715", DatabaseHome},
		{"19", "19.028", DatabaseHome},
		{"23", "23.26.4", DatabaseHome},
		{"26", "23.26", DatabaseHome},
		{"19", "19.28", "other"},
	} {
		if _, err := catalog.Lookup(invalid); !errors.Is(err, ErrInvalidPatchRecommendations) {
			t.Errorf("invalid scope %v = %v", invalid, err)
		}
	}
}

func TestLoadPatchRecommendationsRejectsUnqualifiedInput(t *testing.T) {
	source := testRecommendationSource()
	scope := PatchRecommendationScope{"19", "19.28", DatabaseHome}
	valid := PatchRecommendationSet{scope, source, []PatchRecommendation{{Number: 10010000}}}
	noRows := valid
	noRows.Rows = nil
	badNumber := valid
	badNumber.Rows = []PatchRecommendation{{Number: 0}}
	badProof := valid
	badProof.Source.Document.SHA256 = "unverified"
	badProof2 := valid
	badProof2.Source.CapturedUTC = ""
	badCapture := valid
	badCapture.Source.CapturedUTC = "2026-11-11T00:00:00Z"
	badURL := valid
	badURL.Source.Document.URL += "&session=private"
	wrongDoc := valid
	wrongDoc.Source.Document.URL = "https://support.oracle.com/epmos/faces/DocumentDisplay?id=wrong"
	badPlatform := valid
	badPlatform.Rows = []PatchRecommendation{{Number: 10010000, PlatformFamily: "Platform Independent"}}

	for _, tc := range []struct {
		name  string
		input io.Reader
	}{
		{"nil reader", nil},
		{"empty", strings.NewReader("")},
		{"unqualified schema", strings.NewReader(`{"schema_version":2,"sections":[]}`)},
		{"missing rows", bytes.NewReader(encodeRecommendationSnapshot(t, noRows))},
		{"duplicate scope", bytes.NewReader(encodeRecommendationSnapshot(t, valid, valid))},
		{"missing capture", bytes.NewReader(encodeRecommendationSnapshot(t, badProof2))},
		{"bad hash", bytes.NewReader(encodeRecommendationSnapshot(t, badProof))},
		{"capture chronology", bytes.NewReader(encodeRecommendationSnapshot(t, badCapture))},
		{"wrong document", bytes.NewReader(encodeRecommendationSnapshot(t, wrongDoc))},
		{"session URL", bytes.NewReader(encodeRecommendationSnapshot(t, badURL))},
		{"non-patch number", bytes.NewReader(encodeRecommendationSnapshot(t, badNumber))},
		{"non-source platform", bytes.NewReader(encodeRecommendationSnapshot(t, badPlatform))},
		{"trailing content", bytes.NewReader(append(encodeRecommendationSnapshot(t, valid), []byte("{}")...))},
		{"size", strings.NewReader(strings.Repeat(" ", maxPatchRecommendationBytes+1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadPatchRecommendations(tc.input); !errors.Is(err, ErrInvalidPatchRecommendations) {
				t.Fatalf("load error = %v", err)
			}
		})
	}
}
