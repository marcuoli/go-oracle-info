package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestIdentityVersionBoundariesAndServerPlatformProvenance(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 26} {
		sql, err := identity.DatabaseStatement(major)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sql, "TNS for") != (major == 9) {
			t.Fatalf("release %d lost platform source: %s", major, sql)
		}
		version, err := identity.VersionStatement(major)
		if err != nil || strings.Contains(version, "VERSION_FULL") != (major >= 18) {
			t.Fatal(major, version, err)
		}
	}
	bootstrap, _ := identity.VersionStatement(0)
	if strings.Contains(bootstrap, "VERSION_FULL") {
		t.Fatal("unknown release probed modern column")
	}
	if _, err := identity.DatabaseStatement(0); !errors.Is(err, identity.ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}

func TestDecodePreservesNullEmptyTextAndOriginalVersion(t *testing.T) {
	result := session.Result{Columns: []string{"DATABASE_NAME", "DATABASE_ROLE", "PLATFORM_NAME", "LOG_MODE"}, Rows: [][]session.Value{{{Text: "DB"}, {Text: "PRIMARY"}, {Null: true}, {Text: ""}}}}
	info, err := identity.DecodeDatabase(result, 9)
	if err != nil || info.Platform.State != identity.Null || info.LogMode.State != identity.Available || info.LogMode.Value != "" || info.PlatformSource != identity.ServerTNSBanner {
		t.Fatalf("%+v %v", info, err)
	}
	version, err := identity.DecodeVersion(session.Result{Columns: []string{"VERSION"}, Rows: [][]session.Value{{{Text: "9.2.0.8.0"}}}})
	if err != nil || version.Value != "9.2.0.8.0" {
		t.Fatal(version, err)
	}
	major, err := identity.Major(version.Value)
	if err != nil || major != 9 {
		t.Fatal(major, err)
	}
	result.Rows = nil
	info, err = identity.DecodeDatabase(result, 19)
	if err != nil || info.Name.State != identity.NoRows {
		t.Fatal(info, err)
	}
}

func TestIdentityRejectsAmbiguousAndMalformedSources(t *testing.T) {
	for _, result := range []session.Result{
		{Columns: []string{"VERSION", "VERSION"}},
		{Columns: []string{"VERSION"}, Rows: [][]session.Value{{{Text: "19"}}, {{Text: "26"}}}},
		{Columns: []string{"VERSION"}, Rows: [][]session.Value{{}}},
	} {
		if _, err := identity.DecodeVersion(result); !errors.Is(err, identity.ErrInvalidEvidence) {
			t.Fatal("malformed source admitted", err)
		}
	}
	for _, version := range []string{"", "unknown", "8.1", "19x.1", " 19.1"} {
		if _, err := identity.Major(version); err == nil {
			t.Fatal("invalid major admitted", version)
		}
	}
}
