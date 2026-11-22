package releases

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestResolvePatchPlatform(t *testing.T) {
	labels := map[string]string{
		"212P": "IBM AIX on POWER Systems (64-bit) (IBM_AIX_on_POWER_Systems_64-bit)",
		"267P": "Oracle Solaris on x86-64 (64-bit)",
		"227P": "IBM: Linux on POWER Big Endian Systems (IBM_Linux_on_POWER_Big_Endian_Systems)",
		"23P":  "Oracle Solaris on SPARC (64-bit) (Oracle_Solaris_on_x86-64_64-bit)",
		"233P": "Linux x86-64",
	}
	tests := []struct {
		input string
		want  string
		err   error
	}{
		{"226", LinuxX8664, nil},
		{WindowsX64, WindowsX64, nil},
		{"212P", "IBM AIX on POWER Systems (64-bit)", nil},
		{"267", "Oracle Solaris on x86-64 (64-bit)", nil},
		{"227P", "", ErrPatchPackageUnavailable},
		{"23P", "", ErrPatchPackageUnavailable},
		{"233P", "", ErrInvalidPatchPackageQuery},
		{"12", "", ErrInvalidPatchPackageQuery},
		{"212L", "", ErrInvalidPatchPackageQuery},
		{"", "", ErrInvalidPatchPackageQuery},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ResolvePatchPlatform(tc.input, labels)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Errorf("ResolvePatchPlatform(%q) = (%q, %v), want (%q, %v)", tc.input, got, err, tc.want, tc.err)
			}
		})
	}
}

const toolDigest = "981030de08a33df7e5865a8524325a064866b0ae8ee7cdca43ef3c735cc25d5a"

const publicManifestFixture = `{
	"name": "autoupgrade.jar", "version": "26.7.2",
	"downloadUrl": "https://download.oracle.com/otn-pub/otn_software/autoupgrade.jar",
	"checksum": "` + toolDigest + `", "releaseDateTime": "2026-11-11T10:00:00.000Z", "size": 7692717
}`

func TestDecodeAutoUpgradeManifest(t *testing.T) {
	got, err := DecodeAutoUpgradeManifest(strings.NewReader(publicManifestFixture))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "26.7.2" || got.Build != "" || got.Filename != "autoupgrade.jar" ||
		got.SHA256 != toolDigest || got.Size != 7692717 || got.ReleaseDateTime != "2026-11-11T10:00:00.000Z" ||
		got.DownloadURL != autoUpgradeDownloadURL || got.Source.URL != AutoUpgradeManifestURL ||
		got.Source.SHA256 != "bef80f4d39700ea566f6910907e1b11afc75bc41171e5346283274095f146ba5" || got.AttachmentID != "" {
		t.Fatalf("unexpected public manifest: %+v", got)
	}

	for _, fixture := range []string{
		strings.Replace(publicManifestFixture, "download.oracle.com", "download.oracle.com.example", 1),
		strings.Replace(publicManifestFixture, "7692717", "0", 1),
		strings.Replace(publicManifestFixture, toolDigest, "", 1),
		publicManifestFixture + "{}",
		strings.Repeat(" ", maxReleaseInputBytes+1),
	} {
		if artifact, err := DecodeAutoUpgradeManifest(strings.NewReader(fixture)); artifact != (AutoUpgradeArtifact{}) ||
			!errors.Is(err, ErrInvalidAutoUpgrade) {
			t.Fatalf("invalid manifest returned (%+v, %v)", artifact, err)
		}
	}
}

const patchToolSource = `"source": {
	"id": "KB123450", "old_doc_id": "2485457.1", "title": "AutoUpgrade Tool",
	"url": "https://support.oracle.com/support/?kmContentId=KB123450",
	"checked_utc": "2026-11-09T01:00:10Z", "sha256": "` + toolDigest + `"
}, "captured_utc": "2026-11-09T01:00:09Z"`

func TestLatestArchivedAutoUpgrade(t *testing.T) {
	const earlierID, laterID = "0000000000000000000000000000000a", "0000000000000000000000000000000b"
	earlier := `{"version":"25.9","build":"25.9.251016","filename":"autoupgrade.jar",
		"attachment_id":"` + earlierID + `", "sha256":"` + toolDigest + `",` + patchToolSource + `}`
	later := `{"version":"25.10","build":"25.10.251115","filename":"autoupgrade.jar",
		"attachment_id":"` + laterID + `",` + patchToolSource + `}`
	load := func(entries string) (AutoUpgradeArtifact, error) {
		t.Helper()
		return LatestArchivedAutoUpgrade(strings.NewReader(`{"schema_version":1,"builds":[` + entries + `]}`))
	}
	got, err := load(later + "," + earlier)
	if err != nil || got.Version != "25.10" || got.Build != "25.10.251115" ||
		got.AttachmentID != laterID || got.SHA256 != "" || got.DownloadURL != "" || got.Source.OldDocID != "2485457.1" {
		t.Fatalf("latest archive = (%+v, %v)", got, err)
	}
	got, err = load(earlier)
	if err != nil || got.SHA256 != toolDigest || got.DownloadURL != "" {
		t.Fatalf("archived digest = (%+v, %v)", got, err)
	}
	if _, err := load(later + "," + earlier + "," + strings.Replace(later, laterID, earlierID, 1)); !errors.Is(err, ErrAmbiguousAutoUpgrade) {
		t.Fatalf("conflicting build error = %v", err)
	}
	if _, err := load(""); !errors.Is(err, ErrAutoUpgradeUnavailable) {
		t.Fatalf("empty archive error = %v", err)
	}
	if _, err := load(strings.Replace(later, "25.10.251115", "25.9.251115", 1)); !errors.Is(err, ErrInvalidAutoUpgrade) {
		t.Fatalf("mismatched build error = %v", err)
	}
	crossed := strings.Replace(later, laterID, "2485457.1:AUTOUPGRADE2510251115", 1)
	if _, err := load(crossed); !errors.Is(err, ErrInvalidAutoUpgrade) {
		t.Fatalf("legacy attachment with current source error = %v", err)
	}
}

func TestGoldImageReferenceCoverage(t *testing.T) {
	const fixture = `{"schema_version":1,` + patchToolSource + `,"sections":[
		{"scope":{"line":"26","train":"23.26.4","home":"database"},
			"rows":[{"number":70000001,"version":"23.26.4.0.0","engineered_or_cloud_only":true}]},
		{"scope":{"line":"26","train":"23.26.4","home":"grid_infrastructure"},"rows":[]},
		{"scope":{"line":"23","train":"23.9","home":"client"},
			"rows":[{"number":70000003,"version":"23.9.0.25.07","all_platforms":true}]}
	]}`
	gold := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fixture, "KB123450", "KB153394"), "2485457.1", "3053981.1"), "AutoUpgrade Tool", "Oracle AI Database 26ai Proactive Patch Information")
	catalog, err := LoadGoldImages(strings.NewReader(gold))
	if err != nil {
		t.Fatal(err)
	}
	db := GoldImageScope{"26", "23.26.4", GoldDatabaseHome}
	got, err := catalog.Lookup(db)
	want := GoldImageReference{Number: 70000001, Version: "23.26.4.0.0", EngineeredOrCloudOnly: true}
	if err != nil || len(got.Rows) != 1 || got.Rows[0] != want || got.Source.ID != "KB153394" {
		t.Fatalf("database Gold Image = (%+v, %v)", got, err)
	}
	got.Rows[0].Number = 0
	got, err = catalog.Lookup(db)
	if err != nil || got.Rows[0] != want {
		t.Fatalf("Gold Image catalog changed after caller mutation = (%+v, %v)", got, err)
	}
	got, err = catalog.Lookup(GoldImageScope{"23", "23.9", GoldClientHome})
	if err != nil || len(got.Rows) != 1 || !got.Rows[0].AllPlatforms || got.Rows[0].Platform != "" ||
		got.Rows[0].EngineeredOrCloudOnly || got.Rows[0].Number != 70000003 {
		t.Fatalf("client Gold Image = (%+v, %v)", got, err)
	}
	got, err = catalog.Lookup(GoldImageScope{"26", "23.26.4", GoldGIHome})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatalf("reviewed empty section = (%+v, %v)", got, err)
	}
	if _, err := catalog.Lookup(GoldImageScope{"23", "23.9", GoldGIHome}); !errors.Is(err, ErrGoldImageCoverageUnavailable) {
		t.Fatalf("missing home error = %v", err)
	}
	if _, err := catalog.Lookup(GoldImageScope{"26", "23.26.4.0.0", GoldGIHome}); !errors.Is(err, ErrInvalidGoldImages) {
		t.Fatalf("non-train query error = %v", err)
	}
	if _, err := LoadGoldImages(strings.NewReader(strings.Replace(gold, "23.26.4.0.0", "23.26.3.0.0", 1))); !errors.Is(err, ErrInvalidGoldImages) {
		t.Fatalf("cross-train reference error = %v", err)
	}
	if _, err := LoadGoldImages(strings.NewReader(fixture)); !errors.Is(err, ErrInvalidGoldImages) {
		t.Fatalf("unrelated Oracle article error = %v", err)
	}
	unrestricted := strings.Replace(gold, `,"engineered_or_cloud_only":true`, "", 1)
	if _, err := LoadGoldImages(strings.NewReader(unrestricted)); !errors.Is(err, ErrInvalidGoldImages) {
		t.Fatalf("stripped deployment restriction error = %v", err)
	}
	const row = `{"number":70000001,"version":"23.26.4.0.0","engineered_or_cloud_only":true}`
	duplicate := strings.Replace(gold, row, row+","+strings.Replace(row, "70000001", "70000002", 1), 1)
	if _, err := LoadGoldImages(strings.NewReader(duplicate)); !errors.Is(err, ErrAmbiguousPatchPackage) {
		t.Fatalf("conflicting platform reference error = %v", err)
	}
	bad := io.MultiReader(strings.NewReader(gold), strings.NewReader("{}"))
	if _, err := LoadGoldImages(bad); !errors.Is(err, ErrInvalidGoldImages) {
		t.Fatalf("trailing value error = %v", err)
	}
}
