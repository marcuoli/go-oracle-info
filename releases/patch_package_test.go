package releases

import (
	"errors"
	"slices"
	"testing"
)

func TestBundledPatchPackageSelection(t *testing.T) {
	catalog, err := Load(Database)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, line                                              string
		family                                                  PatchFamily
		platform, train, version, source, recorded, releaseDate string
		number, platformID                                      int
	}{
		{"19c Linux DB RU", "19", DatabaseRU, LinuxX8664, "19.33", "19.33.0.0.261020", "KB111276", "KB111276", "2026-10-20", 39915792, 13},
		{"19c Linux GI RU", "19", GridInfrastructureRU, LinuxX8664, "19.33", "19.33.0.0.261020", "KB111276", "KB111276", "2026-10-20", 39922757, 13},
		{"19c Windows DB bundle", "19", WindowsDatabaseBundle, WindowsX64, "19.32", "19.32.0.0.260721", "KB111276", "KB111276", "2026-07-21", 39418910, 12},
		{"19c Windows Data Pump", "19", DataPumpBundle, WindowsX64, "19.33", "19.33.0", "KB107134", "KB107134", "2026-10-20", 40160961, 12},
		{"19c Linux Data Pump", "19", DataPumpBundle, LinuxX8664, "19.33", "19.33.0", "KB107134", "KB107134", "2026-10-20", 40057600, 13},
		{"18c DB combo", "18", DatabaseCombo, LinuxX8664, "18.17", "18.17.0.0.220118", "KB910370", "KB910370", "2022-01-18", 33567275, 13},
		{"18c GI combo", "18", GridInfrastructureCombo, LinuxX8664, "18.17", "18.17.0.0.220118", "KB910370", "KB910370", "2022-01-18", 33567276, 13},
		{"19c DB MRP", "19", DatabaseMRP, LinuxX8664, "19.20", "19.20.0.0.230815", "KB111276", "2521164.1", "2023-08-15", 35656870, 13},
		{"19c GI MRP", "19", GridInfrastructureMRP, LinuxX8664, "19.20", "19.20.0.0.230815", "KB111276", "2521164.1", "2023-08-15", 35656840, 13},
		{"19c OJVM", "19", OJVM, LinuxX8664, "19.33", "19.33.0.0.261020", "KB111276", "KB111276", "2026-10-20", 39803650, 13},
		{"19c JDK", "19", JDK, LinuxX8664, "19.33", "19.33.0.0.261030", "KB111276", "KB111276", "2026-10-30", 40192005, 13},
		{"21c JDK", "21", JDK, LinuxX8664, "21.24", "21.24.0.0.261030", "KB117472", "KB117472", "2026-10-30", 40192133, 13},
		{"23ai Data Pump", "23", DataPumpBundle, LinuxX8664, "23.9", "23.9", "KB160561", "KB160561", "2025-07-15", 38171540, 13},
		{"26ai DB RU", "26", DatabaseRU, LinuxX8664, "23.26.4", "23.26.4.0.0", "KB153394", "KB153394", "2026-10-20", 40007731, 13},
		{"26ai GI RU", "26", GridInfrastructureRU, LinuxX8664, "23.26.4", "23.26.4.0.0", "KB153394", "KB153394", "2026-10-20", 40002013, 13},
		{"26ai Data Pump", "26", DataPumpBundle, LinuxX8664, "23.26.4", "23.26.4", "KB160561", "KB160561", "2026-10-20", 40089368, 13},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filter := PatchPackageFilter{Line: tc.line, Family: tc.family, Platform: tc.platform}
			latest, err := catalog.LatestPatchPackage(filter)
			if err != nil {
				t.Fatal(err)
			}
			a := latest.Artifact
			if latest.Filter != filter || latest.Train != tc.train || latest.Version != tc.version || latest.AsOf != catalog.AsOf ||
				a.Number != tc.number || a.MOSPlatform != tc.platform || a.PlatformID == nil || *a.PlatformID != tc.platformID ||
				a.ReleaseDate != tc.releaseDate || a.RecordedSourceID != tc.recorded {
				t.Fatalf("unexpected package: %+v", latest)
			}
			for _, id := range []string{"local-patch-export", tc.source} {
				if !slices.ContainsFunc(latest.Sources, func(source Source) bool { return source.ID == id && source.SHA256 != "" }) {
					t.Fatalf("missing verified source reference %s: %+v", id, latest.Sources)
				}
			}
			for _, version := range []string{latest.Train, latest.Version} {
				exact, err := catalog.ExactPatchPackage(filter, version)
				if err != nil || exact.Artifact.Number != tc.number || exact.Version != tc.version {
					t.Fatalf("exact %q = %+v, %v", version, exact, err)
				}
			}
		})
	}
}

func TestExactPatchPackageDoesNotWidenOrSubstitute(t *testing.T) {
	catalog, err := Load(Database)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		line, platform, version string
		family                  PatchFamily
		want                    error
	}{
		{"19", WindowsX64, "19.33", WindowsDatabaseBundle, ErrPatchPackageUnavailable},
		{"19", WindowsX64, "19.33", DatabaseRU, ErrPatchPackageUnavailable},
		{"19", LinuxX8664, "19.32", WindowsDatabaseBundle, ErrPatchPackageUnavailable},
		{"19", WindowsX64, "19.20", DatabaseMRP, ErrPatchPackageUnavailable},
		{"19", LinuxX8664, "19.33.0.0.261020", JDK, ErrPatchPackageUnavailable},
		{"23", WindowsX64, "23.9", DataPumpBundle, ErrPatchPackageUnavailable},
		{"26", LinuxX8664, "23.26.2", DataPumpBundle, ErrPatchPackageUnavailable},
		{"23", LinuxX8664, "23.26.4", DatabaseRU, ErrInvalidPatchPackageQuery},
		{"19", "226P", "19.33", DatabaseRU, ErrInvalidPatchPackageQuery},
		{"19", "Platform Independent", "19.33", DataPumpBundle, ErrInvalidPatchPackageQuery},
		{"19", "Linux x86 64-bit", "19.33", DatabaseRU, ErrInvalidPatchPackageQuery},
		{"19", LinuxX8664, "19.33", "BP", ErrInvalidPatchPackageQuery},
	} {
		filter := PatchPackageFilter{Line: tc.line, Platform: tc.platform, Family: tc.family}
		if result, err := catalog.ExactPatchPackage(filter, tc.version); !errors.Is(err, tc.want) || result.Artifact.Number != 0 {
			t.Errorf("exact %v %q = %+v, %v; want %v", filter, tc.version, result, err, tc.want)
		}
	}
	filter := PatchPackageFilter{Line: "19", Platform: LinuxX8664, Family: JDK}
	exact, err := catalog.ExactPatchPackage(filter, "19.32.0.0.260721")
	if err != nil || exact.Artifact.Number != 39329591 {
		t.Fatalf("older exact JDK in 19.32 = %+v, %v", exact, err)
	}
	exact, err = catalog.ExactPatchPackage(filter, "19.32")
	if err != nil || exact.Artifact.Number != 39791916 || exact.Version != "19.32.0.0.260818" {
		t.Fatalf("highest JDK in 19.32 = %+v, %v", exact, err)
	}
}

func TestPatchReleaseLineWithoutCatalog(t *testing.T) {
	for input, want := range map[string]string{
		"18.17": "18", "19.33": "19", "19.2.0.0.0": "19", "21.100": "21",
		"23.9": "23", "23.9.0.25.07": "23", "23.26.4": "26", "23.26.4.0.0": "26",
	} {
		if got, err := PatchReleaseLine(input); err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "19", "19c", "23.26", "26.4", "12.2.0.1", "19..33", "+19.33", "19.٣٣", "19.33.0.0.0.0"} {
		if got, err := PatchReleaseLine(input); !errors.Is(err, ErrInvalidPatchPackageQuery) || got != "" {
			t.Errorf("%q = %q, %v; want invalid input", input, got, err)
		}
	}
}

func TestPatchPackageEligibilityConflictsAndNumericOrder(t *testing.T) {
	restricted := true
	platformID := 13
	references := []string{"Oracle"}
	patch := func(version string, artifacts ...Artifact) Patch {
		return Patch{Line: "19", Version: version, Kind: "RU", SourceKind: "RU_DB", Component: "RDBMS", Source: "export", Artifacts: artifacts}
	}
	number := func(n int) Artifact {
		return Artifact{Number: n, MOSPlatform: LinuxX8664}
	}
	active := number(10)
	active.PlatformID, active.CorroboratedBy = &platformID, &references
	blocked := patch("19.11", number(11))
	blocked.Restricted = &restricted
	disabledGroup := patch("19.12", number(12))
	disabledGroup.DisabledAt = "2026-11-08"
	disabledArtifact := number(13)
	disabledArtifact.DisabledAt = "2026-11-08"
	wrongPlatform := number(14)
	wrongPlatform.MOSPlatform = "Platform Independent"
	wrongTrain := number(15)
	wrongTrain.Version = "19.99.0"
	wrongComponent := number(16)
	wrongComponent.Component = "GI"
	catalog := Catalog{
		Product: Database, AsOf: "2026-11-21",
		Sources: []Source{{ID: "export"}, {ID: "Oracle"}, {ID: "follow-up", OldDocID: "555.1"}},
		Patches: []Patch{
			patch("19.9", number(9)), patch("19.10", active), patch("19.11", number(11)), blocked,
			disabledGroup, patch("19.13", disabledArtifact), patch("19.14", wrongPlatform),
			patch("19.15", wrongTrain), patch("19.16", wrongComponent), patch("19.17", number(0)), patch("19.18"),
		},
	}
	filter := PatchPackageFilter{Line: "19", Family: DatabaseRU, Platform: LinuxX8664}
	latest, err := catalog.LatestPatchPackage(filter)
	if err != nil || latest.Artifact.Number != 10 {
		t.Fatalf("latest = %+v, %v; want numeric 19.10", latest, err)
	}
	*latest.Artifact.PlatformID = 99
	(*latest.Artifact.CorroboratedBy)[0] = "changed"
	if platformID != 13 || references[0] != "Oracle" {
		t.Fatal("result aliases catalog artifact metadata")
	}
	for _, v := range []string{"19.11", "19.12", "19.13", "19.14", "19.15", "19.16", "19.17", "19.18"} {
		if _, err := catalog.ExactPatchPackage(filter, v); !errors.Is(err, ErrPatchPackageUnavailable) {
			t.Errorf("exact %s = %v, want unavailable", v, err)
		}
	}

	duplicate := number(10)
	duplicate.RecordedSourceID = "555.1"
	catalog.Patches = append(catalog.Patches, patch("19.10", duplicate))
	latest, err = catalog.LatestPatchPackage(filter)
	if err != nil || latest.Artifact.Number != 10 || len(latest.Sources) != 3 ||
		latest.Sources[0].ID != "Oracle" || latest.Sources[1].ID != "export" || latest.Sources[2].ID != "follow-up" {
		t.Fatalf("collapsed observation = %+v, %v", latest, err)
	}
	qualifiedArtifact := number(101)
	qualifiedArtifact.Version = "19.10.1"
	catalog.Patches = append(catalog.Patches, patch("19.10", qualifiedArtifact), patch("19.10.1", number(102)))
	for range 2 {
		if _, err := catalog.LatestPatchPackage(filter); !errors.Is(err, ErrAmbiguousPatchPackage) {
			t.Fatalf("latest blocking ambiguity = %v", err)
		}
		if _, err := catalog.ExactPatchPackage(filter, "19.10.1"); !errors.Is(err, ErrAmbiguousPatchPackage) {
			t.Fatalf("exact blocking ambiguity = %v", err)
		}
		slices.Reverse(catalog.Patches)
	}
	if _, err := (Catalog{Product: Exadata}).LatestPatchPackage(filter); !errors.Is(err, ErrInvalidPatchPackageQuery) {
		t.Fatalf("non-Database catalog must be invalid, got %v", err)
	}
}
