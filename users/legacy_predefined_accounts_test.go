package users_test

import (
	"slices"
	"testing"

	"github.com/marcuoli/go-oracle-info/users"
)

func TestLookupLegacyPredefinedAccounts(t *testing.T) {
	cases := []struct {
		version string
		release string
		present []string
		unknown string
	}{
		{"9.2.0.8.0", "9.2", []string{"AURORA$JIS$UTILITY$"}, "SYSMAN"},
		{"10.1.0.5.0", "10.1", []string{"DMSYS"}, "TSMSYS"},
		{"10.2.0.5.0", "10.2", []string{"TSMSYS"}, "ORDDATA"},
		{"11.1.0.7.0", "11.1", []string{"FLOWS_30000"}, "APPQOSSYS"},
		{"11.2.0.4.0", "11.2", []string{"APEX_030200", "APPQOSSYS", "ORDDATA", "OWBSYS_AUDIT"}, "AUDSYS"},
		{"12.1.0.1.0", "12.1.0.1", []string{"AUDSYS", "APEX_040200", "SYSBACKUP", "SYSDG", "SYSKM"}, "SYSRAC"},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			catalog, ok := users.LookupLegacyPredefinedAccounts(tc.version)
			if !ok || catalog.Release != tc.release || len(catalog.SourceURLs) == 0 {
				t.Fatalf("lookup returned %+v, %t", catalog, ok)
			}
			for _, account := range append([]string{"SYS", "SYSTEM"}, tc.present...) {
				if !slices.Contains(catalog.Usernames, account) {
					t.Errorf("missing documented account %q", account)
				}
			}
			for _, account := range []string{"sys", tc.unknown, "APP", "GGADMIN", "HR", "SCOTT", "OE", "QS", "RMAN"} {
				if slices.Contains(catalog.Usernames, account) {
					t.Errorf("unexpected classification for %q", account)
				}
			}
			if !slices.IsSorted(catalog.Usernames) || len(slices.Compact(slices.Clone(catalog.Usernames))) != len(catalog.Usernames) {
				t.Errorf("usernames must be sorted and unique: %v", catalog.Usernames)
			}
		})
	}
}

func TestLegacyPredefinedAccountsRejectUnknownAndDictionaryVersions(t *testing.T) {
	for _, version := range []string{
		"", "11g", "11", "11.2.-1", "11.2.+1", "11.2.0.4.0.1", "9.1.0.1.0",
		"12.1", "12.1.0", "12.1.0.0.0", "12.1.0.2.0", "12.2.0.1.0",
		"18.0", "19.0", "21.0", "23.4", "26.0",
	} {
		catalog, ok := users.LookupLegacyPredefinedAccounts(version)
		if ok || catalog.Release != "" || len(catalog.Usernames) != 0 || len(catalog.SourceURLs) != 0 {
			t.Errorf("%q selected %+v, %t", version, catalog, ok)
		}
	}
}

func TestLegacyPredefinedAccountsAreCallerOwned(t *testing.T) {
	first, ok := users.LookupLegacyPredefinedAccounts("11.2")
	if !ok || len(first.Usernames) == 0 || len(first.SourceURLs) == 0 {
		t.Fatal("11.2 catalog missing")
	}
	name, source := first.Usernames[0], first.SourceURLs[0]
	first.Usernames[0], first.SourceURLs[0] = "APP", "changed"
	second, ok := users.LookupLegacyPredefinedAccounts("11.2.0.4.0")
	if !ok || len(second.Usernames) == 0 || len(second.SourceURLs) == 0 || second.Usernames[0] != name || second.SourceURLs[0] != source {
		t.Fatalf("mutation changed a later lookup: %+v", second)
	}
}
