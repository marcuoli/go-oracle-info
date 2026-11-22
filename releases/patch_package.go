package releases

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
)

// PatchFamily identifies an explicit package family in the Database catalog.
type PatchFamily string

const (
	DatabaseRU              PatchFamily = "database_ru"
	GridInfrastructureRU    PatchFamily = "grid_infrastructure_ru"
	WindowsDatabaseBundle   PatchFamily = "windows_database_bundle"
	DatabaseCombo           PatchFamily = "database_combo"
	GridInfrastructureCombo PatchFamily = "grid_infrastructure_combo"
	DatabaseMRP             PatchFamily = "database_mrp"
	GridInfrastructureMRP   PatchFamily = "grid_infrastructure_mrp"
	OJVM                    PatchFamily = "ojvm"
	JDK                     PatchFamily = "jdk"
	DataPumpBundle          PatchFamily = "data_pump_bundle"
)

const (
	// LinuxX8664 is the catalog's canonical MOS name for Linux x86-64.
	LinuxX8664 = "Linux x86-64"
	// WindowsX64 is the catalog's canonical MOS name for Windows x64.
	WindowsX64 = "Microsoft Windows x64 (64-bit)"
)

var (
	// ErrInvalidPatchPackageQuery marks invalid release, family, or platform input.
	ErrInvalidPatchPackageQuery = errors.New("invalid Oracle patch package query")
	// ErrPatchPackageUnavailable marks a query with no eligible package in the snapshot.
	ErrPatchPackageUnavailable = errors.New("Oracle patch package unavailable")
	// ErrAmbiguousPatchPackage marks competing package numbers for the selected version.
	ErrAmbiguousPatchPackage = errors.New("ambiguous Oracle patch package")
)

// PatchPackageFilter bounds a Database catalog lookup to one line, family, and
// exact MOS platform name. Lines are "18", "19", "21", "23", and "26".
type PatchPackageFilter struct {
	Line     string
	Family   PatchFamily
	Platform string
}

// PatchPackage describes a numbered package recorded for the requested
// platform as of the bundled snapshot. Version preserves its numeric source
// spelling. Train is the compatibility prefix for related family lookups.
type PatchPackage struct {
	Filter   PatchPackageFilter
	Version  string
	Train    string
	AsOf     string
	Artifact Artifact
	Sources  []Source
}

// PatchReleaseLine resolves a numeric patch train or package version to its
// Database catalog line without consulting package coverage. The 23.26 train
// requires its third component to identify a 26ai release.
func PatchReleaseLine(version string) (string, error) {
	parsed, ok := parsePatchVersion(version)
	if !ok {
		return "", fmt.Errorf("%w: expected a supported numeric patch train or version", ErrInvalidPatchPackageQuery)
	}
	return parsed.line, nil
}

// ExactPatchPackage selects the requested full version or the highest eligible
// version in the requested train. A full version includes any components after
// the train, and is never widened to another version in that train.
func (c Catalog) ExactPatchPackage(filter PatchPackageFilter, version string) (PatchPackage, error) {
	parsed, ok := parsePatchVersion(version)
	if !ok || parsed.line != filter.Line {
		return PatchPackage{}, fmt.Errorf("%w: version does not identify a train or package on the requested line", ErrInvalidPatchPackageQuery)
	}
	return c.patchPackage(filter, parsed)
}

// LatestPatchPackage selects the greatest eligible numeric version observed
// for the requested family and named platform in this snapshot.
func (c Catalog) LatestPatchPackage(filter PatchPackageFilter) (PatchPackage, error) {
	return c.patchPackage(filter, patchVersion{})
}

type patchVersion struct {
	numbers [5]uint64
	width   int
	train   int
	line    string
}

func parsePatchVersion(text string) (patchVersion, bool) {
	numbers, ok := oracleversion.Parse(text)
	if !ok {
		return patchVersion{}, false
	}
	version := patchVersion{numbers: numbers, width: strings.Count(text, ".") + 1, train: 2}
	switch numbers[0] {
	case 18, 19, 21:
		version.line = strconv.FormatUint(numbers[0], 10)
	case 23:
		version.line = "23"
		if numbers[1] == 26 {
			version.line, version.train = "26", 3
		}
	default:
		return patchVersion{}, false
	}
	return version, version.width >= version.train
}

func (v patchVersion) compare(other patchVersion) int {
	return slices.Compare(v.numbers[:v.width], other.numbers[:other.width])
}

func (v patchVersion) sameTrain(other patchVersion) bool {
	return v.line == other.line && slices.Equal(v.numbers[:v.train], other.numbers[:other.train])
}

func (v patchVersion) trainText() string {
	parts := make([]string, v.train)
	for i := range parts {
		parts[i] = strconv.FormatUint(v.numbers[i], 10)
	}
	return strings.Join(parts, ".")
}

func isPatchPlatform(name string) bool {
	switch name {
	case LinuxX8664, WindowsX64, "Linux x86", "Linux ARM 64-bit",
		"IBM AIX on POWER Systems (64-bit)", "IBM: Linux on System z", "HP-UX Itanium",
		"Oracle Solaris on SPARC (32-bit)", "Oracle Solaris on SPARC (64-bit)", "Oracle Solaris on x86-64 (64-bit)":
		return true
	default:
		return false
	}
}

type patchClassification struct {
	kind       string
	sourceKind string
	component  string
}

var packageFamilies = map[PatchFamily]patchClassification{
	DatabaseRU:              {"RU", "RU_DB", "RDBMS"},
	GridInfrastructureRU:    {"RU", "RU_GI", "GI"},
	WindowsDatabaseBundle:   {"bundle", "BP", "RDBMS"},
	DatabaseCombo:           {"bundle", "DB_COMBO", "RDBMS"},
	GridInfrastructureCombo: {"bundle", "GI_COMBO", "GI"},
	DatabaseMRP:             {"MRP", "MRP", "RDBMS"},
	GridInfrastructureMRP:   {"MRP", "MRP", "GI"},
	OJVM:                    {"bundle", "OJVM", "RDBMS"},
	JDK:                     {"bundle", "JDK", "RDBMS"},
	DataPumpBundle:          {"one_off", "DPBP", "DATA_PUMP"},
}

type packageObservation struct {
	version     patchVersion
	versionText string
	patchSource string
	artifact    Artifact
}

type packageIdentity struct {
	version patchVersion
	number  int
}

func (c Catalog) patchPackage(filter PatchPackageFilter, requested patchVersion) (PatchPackage, error) {
	family, ok := packageFamilies[filter.Family]
	if c.Product != Database || !ok || !isPatchPlatform(filter.Platform) {
		return PatchPackage{}, ErrInvalidPatchPackageQuery
	}
	switch filter.Line {
	case "18", "19", "21", "23", "26":
	default:
		return PatchPackage{}, ErrInvalidPatchPackageQuery
	}
	// The seed includes Windows observations on RU_DB rows. A Windows
	// database package requires the source's dedicated BP classification.
	if (filter.Family == DatabaseRU && filter.Platform == WindowsX64) ||
		(filter.Family == WindowsDatabaseBundle && filter.Platform != WindowsX64) {
		return PatchPackage{}, ErrPatchPackageUnavailable
	}
	candidates, excluded := c.patchObservations(filter, family, requested)
	var best []packageObservation
	for _, candidate := range candidates {
		key := packageIdentity{candidate.version, candidate.artifact.Number}
		if excluded[key] {
			continue
		}
		if len(best) == 0 || candidate.version.compare(best[0].version) > 0 {
			best = append(best[:0], candidate)
		} else if candidate.version.compare(best[0].version) == 0 {
			best = append(best, candidate)
		}
	}
	if len(best) == 0 {
		return PatchPackage{}, ErrPatchPackageUnavailable
	}
	selected := best[0]
	for _, other := range best[1:] {
		if other.artifact.Number != selected.artifact.Number {
			return PatchPackage{}, ErrAmbiguousPatchPackage
		}
	}
	return PatchPackage{
		Filter: filter, Version: selected.versionText, Train: selected.version.trainText(),
		AsOf: c.AsOf, Artifact: clonePackageArtifact(selected.artifact), Sources: c.packageSources(best),
	}, nil
}

func (c Catalog) patchObservations(filter PatchPackageFilter, family patchClassification, requested patchVersion) ([]packageObservation, map[packageIdentity]bool) {
	var candidates []packageObservation
	excluded := make(map[packageIdentity]bool)
	for _, patch := range c.Patches {
		if patch.Line != filter.Line || patch.Kind != family.kind || patch.SourceKind != family.sourceKind || patch.Component != family.component {
			continue
		}
		groupVersion, ok := parsePatchVersion(patch.Version)
		if !ok || groupVersion.line != filter.Line {
			continue
		}
		for _, artifact := range patch.Artifacts {
			if artifact.Number <= 0 || artifact.MOSPlatform != filter.Platform || artifact.Platform == "Platform Independent" ||
				(artifact.PlatformID != nil && *artifact.PlatformID == 0) ||
				(artifact.Component != "" && artifact.Component != patch.Component) {
				continue
			}
			version, versionText := groupVersion, patch.Version
			if artifact.Version != "" {
				version, ok = parsePatchVersion(artifact.Version)
				if !ok || !version.sameTrain(groupVersion) {
					continue
				}
				versionText = artifact.Version
			}
			if !matchesPatchVersion(requested, version) {
				continue
			}
			if patch.DisabledAt != "" || (patch.Restricted != nil && *patch.Restricted) || artifact.DisabledAt != "" {
				excluded[packageIdentity{version, artifact.Number}] = true
				continue
			}
			candidates = append(candidates, packageObservation{version, versionText, patch.Source, artifact})
		}
	}
	return candidates, excluded
}

func matchesPatchVersion(requested, observed patchVersion) bool {
	if requested.width == 0 {
		return true
	}
	if requested.width == requested.train {
		return requested.sameTrain(observed)
	}
	return requested.compare(observed) == 0
}

func (c Catalog) packageSources(records []packageObservation) []Source {
	refs := make(map[string]bool)
	for _, record := range records {
		refs[record.patchSource] = true
		refs[record.artifact.Source] = true
		refs[record.artifact.RecordedSourceID] = true
		if record.artifact.CorroboratedBy != nil {
			for _, id := range *record.artifact.CorroboratedBy {
				refs[id] = true
			}
		}
	}
	delete(refs, "")
	var sources []Source
	for _, source := range c.Sources {
		if refs[source.ID] || refs[source.OldDocID] {
			sources = append(sources, source)
		}
	}
	slices.SortFunc(sources, func(a, b Source) int { return cmp.Compare(a.ID, b.ID) })
	return sources
}

func clonePackageArtifact(artifact Artifact) Artifact {
	artifact.PlatformID = clonePackageValue(artifact.PlatformID)
	artifact.Base = clonePackageValue(artifact.Base)
	artifact.Terminal = clonePackageValue(artifact.Terminal)
	if artifact.CorroboratedBy != nil {
		ids := slices.Clone(*artifact.CorroboratedBy)
		artifact.CorroboratedBy = &ids
	}
	return artifact
}

func clonePackageValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
