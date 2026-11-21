package releases

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestBundledCatalogsHaveValidSourceReferences(t *testing.T) {
	for _, product := range []Product{Database, Exadata, ODA, SuperCluster} {
		t.Run(string(product), func(t *testing.T) {
			catalog, err := Load(product)
			if err != nil {
				t.Fatal(err)
			}
			if catalog.Product != product || catalog.SchemaVersion != 1 ||
				catalog.AsOf == "" || product != SuperCluster && len(catalog.Releases) == 0 || len(catalog.Patches) == 0 {
				t.Fatal("missing product, release, or patch metadata")
			}

			f, err := data.Open("data/" + string(product) + ".json")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			decoder := json.NewDecoder(f)
			decoder.DisallowUnknownFields()
			var strict Catalog
			if err := decoder.Decode(&strict); err != nil {
				t.Fatalf("JSON and Go catalog schema differ: %v", err)
			}

			ids := make(map[string]bool, len(catalog.Sources))
			for _, source := range catalog.Sources {
				if source.ID == "" || source.Title == "" || ids[source.ID] {
					t.Fatalf("invalid source identity: %+v", source)
				}
				ids[source.ID] = true
			}
			for _, record := range catalog.Releases {
				if record.Line == "" || !ids[record.Source] {
					t.Fatalf("invalid release provenance: %+v", record)
				}
			}
			for _, record := range catalog.Patches {
				if record.Line == "" || record.Kind == "" || !ids[record.Source] {
					t.Fatalf("invalid patch provenance: %+v", record)
				}
				for _, a := range record.Artifacts {
					if a.CorroboratedBy != nil {
						for _, source := range *a.CorroboratedBy {
							if !ids[source] {
								t.Fatalf("missing corroborating source %q", source)
							}
						}
					}
					if a.Number < 0 || a.Source != "" && !ids[a.Source] {
						t.Fatalf("invalid patch artifact: %+v", a)
					}
				}
			}
			for _, item := range catalog.Coverage {
				if item.Status == "" || !ids[item.Source] {
					t.Fatalf("invalid coverage provenance: %+v", item)
				}
			}
		})
	}
	if _, err := Load("../database"); err == nil {
		t.Fatal("unsupported product accepted")
	}
}

func TestDatabaseBrandAndPatchFamilies(t *testing.T) {
	c, err := Load(Database)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(c.Releases, func(r Release) bool {
		return r.Line == "9.2" && r.GeneralAvailability == "2002-07"
	}) || !slices.ContainsFunc(c.Releases, func(r Release) bool {
		return r.Line == "26ai" && r.Classification == "lts" && r.ExtendedEnds != ""
	}) {
		t.Fatal("historical lifecycle or 26ai LTS classification missing")
	}
	if !slices.ContainsFunc(c.Patches, func(p Patch) bool {
		return p.ProductName == "26ai" && p.Version == "23.26.4.0.0" &&
			p.Component == "RDBMS" && p.Kind == "RU" && len(p.Artifacts) > 0
	}) {
		t.Fatal("26ai brand or exact numeric RU lost")
	}
	if !slices.ContainsFunc(c.Patches, func(p Patch) bool {
		return p.Line == "11.2.0.3" && p.Kind == "one_off" && p.SourceName == "17488100" && p.Version == ""
	}) {
		t.Fatal("one-off patch identifier was confused with a release version")
	}
	for _, expected := range []struct{ line, kind string }{
		{"11.2.0.4", "PSU"}, {"11.2.0.4", "SPU"}, {"12.2.0.1", "RUR"}, {"19", "MRP"},
	} {
		if !slices.ContainsFunc(c.Patches, func(p Patch) bool {
			return p.Line == expected.line && p.Kind == expected.kind && len(p.Artifacts) > 0
		}) {
			t.Fatalf("missing %s patch family for %s", expected.kind, expected.line)
		}
	}
}

func TestAppliancePackageNumbersAreScopedToRelease(t *testing.T) {
	c, err := Load(ODA)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"19.32.0.1.0", "19.32.0.2.0"} {
		if !hasArtifact(c, v, 39904249) {
			t.Fatalf("missing monthly update %s for shared patch number", v)
		}
	}
	scopes := make(map[string]bool)
	for _, p := range c.Patches {
		if p.Line != "19.10" {
			continue
		}
		for _, a := range p.Artifacts {
			if a.Number == 32351355 && a.Scope != "" {
				scopes[a.Scope] = true
			}
		}
	}
	if len(scopes) < 2 {
		t.Fatal("shared ODA package was collapsed across hardware scopes")
	}
	if !hasArtifact(c, "12.1.2.3.0", 20690087) {
		t.Fatal("archive patch with explicitly documented version is missing")
	}
}

func TestExadataSeparatesExactPackageAndAnnouncedBuild(t *testing.T) {
	c, err := Load(Exadata)
	if err != nil {
		t.Fatal(err)
	}
	if !hasArtifact(c, "26.2.1.0.0", 40145355) {
		t.Fatal("Exadata storage server package is missing")
	}
	if !slices.ContainsFunc(c.Releases, func(r Release) bool {
		return r.Line == "26.2" && r.Version == "26.2.2.0.0.261117.2"
	}) {
		t.Fatal("publicly announced Exadata build is missing")
	}
}

func hasArtifact(c Catalog, version string, number int) bool {
	return slices.ContainsFunc(c.Patches, func(p Patch) bool {
		return p.Version == version && slices.ContainsFunc(p.Artifacts, func(a Artifact) bool {
			return a.Number == number
		})
	})
}

func TestSuperClusterAdmitsOnlyExplicitFullStackRecords(t *testing.T) {
	c, err := Load(SuperCluster)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Releases) != 0 {
		t.Fatal("Database release lines were misrepresented as SuperCluster product releases")
	}
	seen := false
	for _, p := range c.Patches {
		if p.Kind != "QFSDP" || p.Component != "supercluster" || p.SourceKind != "QFSDP_SUPERCLUSTER" {
			t.Fatalf("unrelated patch in SuperCluster catalog: %+v", p)
		}
		for _, a := range p.Artifacts {
			if a.Number == 38273760 && a.CorroboratedBy != nil && len(*a.CorroboratedBy) > 0 && p.Line == "21" && a.Platform != "" {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("known QFSDP package, platform, or Oracle corroboration missing")
	}
}
