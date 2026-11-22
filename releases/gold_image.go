package releases

import (
	"errors"
	"fmt"
	"io"
	"slices"
)

// GoldImageHome identifies the home named in an Oracle Gold Image section.
type GoldImageHome string

const (
	GoldDatabaseHome GoldImageHome = "database"
	GoldGIHome       GoldImageHome = "grid_infrastructure"
	GoldClientHome   GoldImageHome = "client"
)

// GoldImageScope identifies one exact numeric release train and home.
type GoldImageScope struct {
	Line  string        `json:"line"`
	Train string        `json:"train"`
	Home  GoldImageHome `json:"home"`
}

// GoldImageReference identifies a package listed by Oracle. Platform is empty
// when no exact platform is named. AllPlatforms preserves the source's table
// label. Both still require checking the requested platform, deployment, and
// actual file in Oracle Support before download.
type GoldImageReference struct {
	Number                int    `json:"number"`
	Version               string `json:"version"`
	Platform              string `json:"platform,omitempty"`
	AllPlatforms          bool   `json:"all_platforms,omitempty"`
	EngineeredOrCloudOnly bool   `json:"engineered_or_cloud_only,omitempty"`
}

// GoldImageSet contains the references and provenance for one reviewed section.
// A non-nil empty Rows slice represents a reviewed empty section.
type GoldImageSet struct {
	Scope       GoldImageScope
	Source      Source
	CapturedUTC string
	Rows        []GoldImageReference
}

// GoldImageCatalog holds references from one reviewed Oracle Support capture.
type GoldImageCatalog struct {
	source      Source
	capturedUTC string
	sections    map[GoldImageScope][]GoldImageReference
}

var (
	ErrInvalidGoldImages            = errors.New("invalid Oracle Gold Image input")
	ErrGoldImageCoverageUnavailable = errors.New("Oracle Gold Image coverage unavailable")
)

// LoadGoldImages loads a bounded, caller-supplied JSON snapshot of KB153394
// (old Doc ID 3053981.1).
// The snapshot contains article references, without attached files or inferred
// package availability. Empty and unavailable sections remain distinct.
func LoadGoldImages(reader io.Reader) (GoldImageCatalog, error) {
	var snapshot struct {
		SchemaVersion int    `json:"schema_version"`
		Source        Source `json:"source"`
		CapturedUTC   string `json:"captured_utc"`
		Sections      []struct {
			Scope GoldImageScope       `json:"scope"`
			Rows  []GoldImageReference `json:"rows"`
		} `json:"sections"`
	}
	if _, err := readReleaseJSON(reader, &snapshot); err != nil {
		return GoldImageCatalog{}, fmt.Errorf("%w: %w", ErrInvalidGoldImages, err)
	}
	if snapshot.SchemaVersion != 1 || len(snapshot.Sections) == 0 ||
		snapshot.Source.ID != "KB153394" || snapshot.Source.OldDocID != "3053981.1" ||
		!validOracleSupportCapture(snapshot.Source, snapshot.CapturedUTC, "") {
		return GoldImageCatalog{}, ErrInvalidGoldImages
	}

	catalog := GoldImageCatalog{
		source: snapshot.Source, capturedUTC: snapshot.CapturedUTC,
		sections: make(map[GoldImageScope][]GoldImageReference, len(snapshot.Sections)),
	}
	for i, section := range snapshot.Sections {
		train, ok := parsePatchVersion(section.Scope.Train)
		if !ok || !section.Scope.valid(train) || section.Rows == nil || (train.line != "23" && train.line != "26") {
			return GoldImageCatalog{}, fmt.Errorf("%w: invalid section %d", ErrInvalidGoldImages, i+1)
		}
		if _, exists := catalog.sections[section.Scope]; exists {
			return GoldImageCatalog{}, fmt.Errorf("%w: duplicate section %d", ErrInvalidGoldImages, i+1)
		}
		client := section.Scope.Home == GoldClientHome
		for _, row := range section.Rows {
			version, ok := parsePatchVersion(row.Version)
			// The reviewed DB/GI tables name deployment classes without platform
			// labels. The client table says "all platforms" without naming any.
			if !ok || version.width != 5 || !version.sameTrain(train) || row.Number <= 0 || row.Number > 1<<31-1 ||
				row.Platform != "" || row.AllPlatforms != client || row.EngineeredOrCloudOnly == client {
				return GoldImageCatalog{}, fmt.Errorf("%w: invalid reference in section %d", ErrInvalidGoldImages, i+1)
			}
		}
		if len(section.Rows) > 1 {
			return GoldImageCatalog{}, fmt.Errorf("%w: multiple Gold Image references in section %d", ErrAmbiguousPatchPackage, i+1)
		}
		catalog.sections[section.Scope] = section.Rows
	}
	return catalog, nil
}

// Lookup returns every listed reference for the exact train and home.
func (c GoldImageCatalog) Lookup(scope GoldImageScope) (GoldImageSet, error) {
	train, ok := parsePatchVersion(scope.Train)
	if !ok || !scope.valid(train) {
		return GoldImageSet{}, ErrInvalidGoldImages
	}
	rows, found := c.sections[scope]
	if !found {
		return GoldImageSet{}, ErrGoldImageCoverageUnavailable
	}
	return GoldImageSet{Scope: scope, Source: c.source, CapturedUTC: c.capturedUTC, Rows: slices.Clone(rows)}, nil
}

func (s GoldImageScope) valid(train patchVersion) bool {
	return train.line == s.Line && train.width == train.train && train.trainText() == s.Train &&
		(s.Home == GoldDatabaseHome || s.Home == GoldGIHome || s.Home == GoldClientHome)
}
