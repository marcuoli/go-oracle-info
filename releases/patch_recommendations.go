package releases

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// PatchRecommendationHome identifies the home named in the source section.
type PatchRecommendationHome string

const (
	DatabaseHome           PatchRecommendationHome = "database"
	GridInfrastructureHome PatchRecommendationHome = "grid_infrastructure"
)

// PatchRecommendationScope identifies one exact release train and home.
type PatchRecommendationScope struct {
	Line  string                  `json:"line"`
	Train string                  `json:"train"`
	Home  PatchRecommendationHome `json:"home"`
}

// PatchRecommendation records a source listing, independent of MOS package
// availability or entitlement. Fixed-in fields preserve the source text.
// PlatformFamily is a documented restriction such as "Linux". An empty value
// means applicability was not explicitly resolved in the source review.
type PatchRecommendation struct {
	Number         int    `json:"number"`
	FixedInRU      string `json:"fixed_in_ru"`
	FixedInMRP     string `json:"fixed_in_mrp"`
	PlatformFamily string `json:"platform_family,omitempty"`
}

// PatchRecommendationSource binds a section to the actual Oracle article
// revision and capture bytes. Archived HTML can have a Git commit and no
// verified capture timestamp or Oracle revision date.
type PatchRecommendationSource struct {
	Document      Source `json:"document"`
	CapturedUTC   string `json:"captured_utc,omitempty"`
	ArchiveCommit string `json:"archive_commit,omitempty"`
}

// PatchRecommendationSet is an observed source section. Rows preserves source
// order, including a non-nil empty slice for a reviewed empty table.
type PatchRecommendationSet struct {
	Scope  PatchRecommendationScope  `json:"scope"`
	Source PatchRecommendationSource `json:"source"`
	Rows   []PatchRecommendation     `json:"rows"`
}

var (
	// ErrInvalidPatchRecommendations identifies an invalid query or catalog input.
	ErrInvalidPatchRecommendations = errors.New("invalid Oracle patch recommendations")
	// ErrPatchRecommendationCoverageUnavailable identifies an unobserved section.
	ErrPatchRecommendationCoverageUnavailable = errors.New("Oracle patch recommendation coverage unavailable")
)

// PatchRecommendationCatalog contains validated source sections. It does not
// share data with the embedded Oracle release and patch-package catalogs.
type PatchRecommendationCatalog struct {
	sections map[PatchRecommendationScope]PatchRecommendationSet
}

const maxPatchRecommendationBytes = 1 << 20

// LoadPatchRecommendations loads a caller-supplied reviewed JSON snapshot.
// It reads at most 1 MiB of data, and the caller owns the reader and source
// access. Use the source review and hash receipts to verify the supplied bytes.
func LoadPatchRecommendations(reader io.Reader) (PatchRecommendationCatalog, error) {
	if reader == nil {
		return PatchRecommendationCatalog{}, ErrInvalidPatchRecommendations
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPatchRecommendationBytes+1))
	if err != nil {
		return PatchRecommendationCatalog{}, fmt.Errorf("read Oracle patch recommendations: %w", err)
	}
	if len(raw) > maxPatchRecommendationBytes {
		return PatchRecommendationCatalog{}, fmt.Errorf("%w: snapshot exceeds size limit", ErrInvalidPatchRecommendations)
	}
	var snapshot struct {
		SchemaVersion int `json:"schema_version"`
		Sections      []struct {
			Scope  PatchRecommendationScope  `json:"scope"`
			Source PatchRecommendationSource `json:"source"`
			Rows   []struct {
				Number         int     `json:"number"`
				FixedInRU      *string `json:"fixed_in_ru"`
				FixedInMRP     *string `json:"fixed_in_mrp"`
				PlatformFamily string  `json:"platform_family,omitempty"`
			} `json:"rows"`
		} `json:"sections"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil || snapshot.SchemaVersion != 1 || len(snapshot.Sections) == 0 {
		return PatchRecommendationCatalog{}, fmt.Errorf("%w: expected a version 1 snapshot with source sections", ErrInvalidPatchRecommendations)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return PatchRecommendationCatalog{}, fmt.Errorf("%w: unexpected trailing data", ErrInvalidPatchRecommendations)
	}

	sections := make(map[PatchRecommendationScope]PatchRecommendationSet, len(snapshot.Sections))
	for i, section := range snapshot.Sections {
		if !validRecommendationScope(section.Scope) || !validOracleSupportCapture(section.Source.Document, section.Source.CapturedUTC, section.Source.ArchiveCommit) || section.Rows == nil {
			return PatchRecommendationCatalog{}, fmt.Errorf("%w: invalid source section %d", ErrInvalidPatchRecommendations, i+1)
		}
		if _, exists := sections[section.Scope]; exists {
			return PatchRecommendationCatalog{}, fmt.Errorf("%w: duplicate source section %d", ErrInvalidPatchRecommendations, i+1)
		}
		rows := make([]PatchRecommendation, len(section.Rows))
		for j, row := range section.Rows {
			if row.Number <= 0 || row.Number > 1<<31-1 || row.FixedInRU == nil || row.FixedInMRP == nil ||
				!validRecommendationPlatform(row.PlatformFamily) {
				return PatchRecommendationCatalog{}, fmt.Errorf("%w: invalid row in source section %d", ErrInvalidPatchRecommendations, i+1)
			}
			rows[j] = PatchRecommendation{Number: row.Number, FixedInRU: *row.FixedInRU,
				FixedInMRP: *row.FixedInMRP, PlatformFamily: row.PlatformFamily}
		}
		sections[section.Scope] = PatchRecommendationSet{Scope: section.Scope, Source: section.Source, Rows: rows}
	}
	return PatchRecommendationCatalog{sections: sections}, nil
}

// Lookup returns the exact observed release train and home. The lookup retains
// every listed row, including rows with fixed-in text or a platform restriction.
func (c PatchRecommendationCatalog) Lookup(scope PatchRecommendationScope) (PatchRecommendationSet, error) {
	if !validRecommendationScope(scope) {
		return PatchRecommendationSet{}, ErrInvalidPatchRecommendations
	}
	section, found := c.sections[scope]
	if !found {
		return PatchRecommendationSet{}, ErrPatchRecommendationCoverageUnavailable
	}
	section.Rows = slices.Clone(section.Rows)
	return section, nil
}

func validRecommendationScope(scope PatchRecommendationScope) bool {
	version, ok := parsePatchVersion(scope.Train)
	return ok && version.line == scope.Line && version.width == version.train && version.trainText() == scope.Train &&
		(scope.Home == DatabaseHome || scope.Home == GridInfrastructureHome)
}

func validRecommendationPlatform(platform string) bool {
	switch platform {
	case "", "Linux", "Windows", "AIX", "Solaris", "HP-UX":
		return true
	default:
		return false
	}
}
