package identity

import (
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/session"
)

// EditionStatement reads the observed instance edition code on known Oracle 12+.
// The documented 11g view has no EDITION column; use its banner as separate
// evidence instead of synthesizing a code. An observed code is not a license grant.
func EditionStatement(major int) (string, error) {
	if major < 12 {
		return "", ErrUnsupportedVersion
	}
	return "SELECT EDITION AS EDITION FROM V$INSTANCE", nil
}

// DecodeEdition retains the exact observed code, SQL NULL or authoritative no row.
// Unknown codes are evidence, not an instruction to infer a different edition.
func DecodeEdition(result session.Result) (Text, error) {
	return decodeScalar(result, "EDITION")
}

// DecodeFullVersion decodes the existing FullVersionSQL projection. It neither
// manufactures an RU nor falls back on behalf of a caller after an execution error.
func DecodeFullVersion(result session.Result) (Text, error) {
	return decodeScalar(result, "VERSION_FULL")
}

// VersionBanner retains one complete V$VERSION row without trimming multiline text.
// Full is nil when BANNER_FULL was not selected on a legacy source. A non-nil Full
// with State Null represents a selected SQL NULL, not an unsupported column.
type VersionBanner struct {
	Banner Text
	Full   *Text
}

// BannersStatement selects reviewed version columns on an established Oracle 11+
// source. BANNER_FULL is selected only for 18+; no release-update text is invented
// for older releases. This reads the connected view, not every RAC instance.
func BannersStatement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	columns := "BANNER AS BANNER"
	if major >= 18 {
		columns += ", BANNER_FULL AS BANNER_FULL"
	}
	return "SELECT " + columns + " FROM V$VERSION ORDER BY BANNER", nil
}

// DecodeBanners accepts only complete results for BannersStatement's projection.
// It preserves duplicates, source order, NULLs and all banner bytes. Empty results
// are authoritative no rows only after the caller's execution/close checks succeed.
// A malformed row invalidates the inventory; no truncated prefix is returned.
func DecodeBanners(result session.Result, major int) ([]VersionBanner, error) {
	if _, err := BannersStatement(major); err != nil {
		return nil, err
	}
	names := []string{"BANNER"}
	if major >= 18 {
		names = append(names, "BANNER_FULL")
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	facts := make([]VersionBanner, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := VersionBanner{Banner: text(row[index["BANNER"]])}
		if major >= 18 {
			full := text(row[index["BANNER_FULL"]])
			fact.Full = &full
		}
		facts = append(facts, fact)
	}
	return facts, nil
}
