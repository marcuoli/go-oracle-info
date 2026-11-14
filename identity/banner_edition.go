package identity

import (
	"regexp"
	"strings"
)

// BannerEdition is an explicit description found in legacy database banner text.
// Source is always v$version.banner; BannerIndex identifies the original input row.
// It is not an EDITION code, a license assertion or an admission decision. Retain
// the raw VersionBanner input separately alongside this descriptive provenance.
type BannerEdition struct {
	Description string
	Source      string
	BannerIndex int
}

var legacyBannerEdition = regexp.MustCompile(`^Oracle Database 11g (Enterprise Edition|Standard Edition|Express Edition) Release 11(?:\.[0-9]+){4} - (?:32bit |64bit )?Production$`)

// LegacyEditionDescription extracts only a known literal description from one
// exact legacy Oracle Database 11g BANNER line. Input must already be complete,
// admitted evidence; no helper can recover a stripped truncation/failure marker.
//
// Unknown formats, NULL/unavailable/empty rows, missing labels and multiple
// database banners (including identical duplicates) return the zero value,false.
// Other component banners are ignored. Full is never consulted, raw input is never
// rewritten and no edition code is inferred. Modern observed EDITION is separate.
func LegacyEditionDescription(banners []VersionBanner) (BannerEdition, bool) {
	index := -1
	const prefix = "Oracle Database"
	for i, row := range banners {
		if row.Banner.State != Available || strings.TrimSpace(row.Banner.Value) == "" {
			return BannerEdition{}, false
		}
		// Be broad when detecting ambiguity, but exact when accepting a label.
		// Padding/case changes must not hide another database banner from the count.
		candidate := strings.TrimSpace(row.Banner.Value)
		if len(candidate) < len(prefix) || !strings.EqualFold(candidate[:len(prefix)], prefix) {
			continue
		}
		if index >= 0 {
			return BannerEdition{}, false
		}
		index = i
	}
	if index < 0 {
		return BannerEdition{}, false
	}
	match := legacyBannerEdition.FindStringSubmatch(banners[index].Banner.Value)
	if match == nil {
		return BannerEdition{}, false
	}
	return BannerEdition{Description: strings.Clone(match[1]), Source: "v$version.banner", BannerIndex: index}, true
}
