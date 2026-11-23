package grants_test

import (
	"slices"
	"testing"

	"github.com/marcuoli/go-oracle-info/grants"
)

func TestLookupLegacyPredefinedRoles(t *testing.T) {
	for _, tc := range []struct {
		version string
		release string
		count   int
		role    string
	}{
		{"9.2.0.8.0", "9.2", 13, "SNMPAGENT"},
		{"10.1.0.5.0", "10.1", 12, "HS_ADMIN_ROLE"},
		{"10.2.0.5.0", "10.2", 12, "RECOVERY_CATALOG_OWNER"},
		{"11.1.0.7.0", "11.1", 52, "OWB$CLIENT"},
		{"11.2.0.4.0", "11.2", 56, "HS_ADMIN_EXECUTE_ROLE"},
		{"12.1.0.1.0", "12.1.0.1", 56, "EM_EXPRESS_BASIC"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			catalog, ok := grants.LookupLegacyPredefinedRoles(tc.version)
			if !ok || catalog.Release != tc.release || len(catalog.SourceURLs) == 0 || len(catalog.RoleNames) != tc.count {
				t.Fatalf("unexpected catalog: %+v, %t", catalog, ok)
			}
			for _, name := range []string{"CONNECT", "DBA", "RESOURCE", tc.role} {
				if !slices.Contains(catalog.RoleNames, name) {
					t.Errorf("documented role absent: %q", name)
				}
			}
			for _, name := range []string{"Public", "PUBLIC", "connect", "APP_READER", "EM_EXRESS_BASIC"} {
				if slices.Contains(catalog.RoleNames, name) {
					t.Errorf("unexpected exact match for %q", name)
				}
			}
			if !slices.IsSorted(catalog.RoleNames) || len(slices.Compact(slices.Clone(catalog.RoleNames))) != tc.count {
				t.Fatalf("catalog role names are not sorted and unique")
			}
		})
	}
}

func TestLegacyPredefinedRolesRejectUnsupportedAndAmbiguousVersions(t *testing.T) {
	for _, version := range []string{
		"", "11g", "11.1.-7", "9.1.0.0.0", "10.3",
		"12.1", "12.1.0", "12.1.0.2", "12.2.0.1", "18.0",
		"19.32.0.0.0", "21.0", "23.26.1.0.0", "26.0",
	} {
		catalog, ok := grants.LookupLegacyPredefinedRoles(version)
		if ok || catalog.Release != "" || len(catalog.RoleNames) != 0 || len(catalog.SourceURLs) != 0 {
			t.Errorf("%q selected a legacy role catalog: %+v, %t", version, catalog, ok)
		}
	}
}

func TestLegacyPredefinedRolesAreCallerOwned(t *testing.T) {
	first, ok := grants.LookupLegacyPredefinedRoles("12.1.0.1")
	if !ok || len(first.RoleNames) == 0 || len(first.SourceURLs) == 0 {
		t.Fatal("12.1.0.1 catalog unavailable")
	}
	roleName, source := first.RoleNames[0], first.SourceURLs[0]
	first.RoleNames[0], first.SourceURLs[0] = "APP", "changed"
	second, ok := grants.LookupLegacyPredefinedRoles("12.1.0.1.0")
	if !ok || len(second.RoleNames) == 0 || len(second.SourceURLs) == 0 ||
		second.RoleNames[0] != roleName || second.SourceURLs[0] != source {
		t.Fatalf("mutation affected a later result: %+v", second)
	}
}
