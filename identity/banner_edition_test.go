package identity_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
)

const (
	enterprise11 = "Oracle Database 11g Enterprise Edition Release 11.2.0.4.0 - 64bit Production"
	standard11   = "Oracle Database 11g Standard Edition Release 11.2.0.1.0 - 64bit Production"
	express11    = "Oracle Database 11g Express Edition Release 11.2.0.1.0 - Production"
)

func banner(text string) identity.VersionBanner {
	return identity.VersionBanner{Banner: identity.Text{State: identity.Available, Value: text}}
}

func TestLegacyEditionDescriptionLiteralEvidence(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, source, want string }{
		{"Oracle Enterprise example", enterprise11, "Enterprise Edition"},
		{"Oracle Standard example", standard11, "Standard Edition"},
		{"Oracle Express example", express11, "Express Edition"},
		{"32bit source spelling", strings.Replace(enterprise11, "64bit", "32bit", 1), "Enterprise Edition"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := []identity.VersionBanner{banner("PL/SQL Release 11.2.0.4.0 - Production"), banner(tt.source), banner("TNS for Linux: Version 11.2.0.4.0 - Production")}
			original := append([]identity.VersionBanner(nil), input...)
			got, ok := identity.LegacyEditionDescription(input)
			want := identity.BannerEdition{Description: tt.want, Source: "v$version.banner", BannerIndex: 1}
			if !ok || got != want {
				t.Fatalf("got %+v,%t; want %+v,true", got, ok, want)
			}
			if !reflect.DeepEqual(input, original) {
				t.Fatal("helper rewrote raw banner evidence")
			}
			if input[got.BannerIndex].Banner.Value != tt.source || !strings.Contains(input[got.BannerIndex].Banner.Value, got.Description) {
				t.Fatal("description/index is not literal source evidence")
			}
		})
	}
}

func TestLegacyEditionDescriptionUnavailable(t *testing.T) {
	t.Parallel()
	noLabel := "Oracle Database 11g Release 11.2.0.4.0 - 64bit Production"
	full := identity.Text{State: identity.Available, Value: enterprise11}
	for _, tt := range []struct {
		name  string
		input []identity.VersionBanner
	}{
		{"no rows", nil},
		{"NULL", []identity.VersionBanner{{Banner: identity.Text{State: identity.Null, Value: enterprise11}}}},
		{"unavailable state", []identity.VersionBanner{{Banner: identity.Text{State: identity.NoRows, Value: enterprise11}}}},
		{"zero state", []identity.VersionBanner{{Banner: identity.Text{Value: enterprise11}}}},
		{"empty", []identity.VersionBanner{banner("")}},
		{"NULL plus valid", []identity.VersionBanner{banner(enterprise11), {Banner: identity.Text{State: identity.Null}}}},
		{"empty plus valid", []identity.VersionBanner{banner(enterprise11), banner(" ")}},
		{"unknown component", []identity.VersionBanner{banner("unknown")}},
		{"other component label", []identity.VersionBanner{banner("TNS for Linux: Enterprise Edition Release 11.2.0.4.0 - Production")}},
		{"missing description", []identity.VersionBanner{banner(noLabel)}},
		{"unknown description", []identity.VersionBanner{banner(strings.Replace(enterprise11, "Enterprise Edition", "Unknown Edition", 1))}},
		{"unsupported personal label", []identity.VersionBanner{banner(strings.Replace(enterprise11, "Enterprise Edition", "Personal Edition", 1))}},
		{"unsupported Standard One", []identity.VersionBanner{banner(strings.Replace(standard11, "Standard Edition", "Standard Edition One", 1))}},
		{"duplicate", []identity.VersionBanner{banner(enterprise11), banner(enterprise11)}},
		{"conflict", []identity.VersionBanner{banner(enterprise11), banner(standard11)}},
		{"unknown additional database", []identity.VersionBanner{banner(enterprise11), banner(noLabel)}},
		{"padded additional database", []identity.VersionBanner{banner(enterprise11), banner(" " + standard11 + " ")}},
		{"case changed additional database", []identity.VersionBanner{banner(enterprise11), banner(strings.ToLower(standard11))}},
		{"two labels in one row", []identity.VersionBanner{banner(strings.Replace(enterprise11, "Enterprise Edition", "Enterprise Edition Standard Edition", 1))}},
		{"prefixed source", []identity.VersionBanner{banner("Proxy " + enterprise11)}},
		{"leading padding", []identity.VersionBanner{banner(" " + enterprise11)}},
		{"trailing newline", []identity.VersionBanner{banner(enterprise11 + "\n")}},
		{"case changed label", []identity.VersionBanner{banner(strings.Replace(enterprise11, "Enterprise Edition", "enterprise edition", 1))}},
		{"bad version", []identity.VersionBanner{banner(strings.Replace(enterprise11, "11.2.0.4.0", "11.2.x", 1))}},
		{"modern banner", []identity.VersionBanner{banner(strings.Replace(enterprise11, "11g", "19c", 1))}},
		{"modern plus legacy", []identity.VersionBanner{banner(enterprise11), banner("Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production")}},
		{"no Full fallback", []identity.VersionBanner{{Banner: identity.Text{State: identity.Available, Value: noLabel}, Full: &full}}},
		{"extra label suffix", []identity.VersionBanner{banner(enterprise11 + " Express Edition")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]identity.VersionBanner(nil), tt.input...)
			got, ok := identity.LegacyEditionDescription(tt.input)
			if ok || got != (identity.BannerEdition{}) {
				t.Fatalf("got %+v,%t; want zero,false", got, ok)
			}
			if !reflect.DeepEqual(tt.input, original) {
				t.Fatal("unavailable result changed input")
			}
		})
	}
}
