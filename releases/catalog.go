// Package releases reads versioned Oracle release metadata bundled with this module.
// It has no connection, network or patching responsibilities.
package releases

import (
	"embed"
	"encoding/json"
	"fmt"
)

// Product identifies a catalog with its own release and patch terminology.
type Product string

const (
	Database     Product = "database"
	Exadata      Product = "exadata"
	ODA          Product = "oda"
	SuperCluster Product = "supercluster"
)

// Source identifies an Oracle document or a named, reproducible Harvester export.
// A checked date describes the source, without claiming verification of all rows.
type Source struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
	OldDocID   string `json:"old_doc_id,omitempty"`
	CheckedUTC string `json:"checked_utc,omitempty"`
	Revision   string `json:"revision,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}

// Release describes lifecycle data or a documented product release. Dates use
// Oracle's stated precision, including YYYY-MM when only a month is published.
type Release struct {
	Line                string `json:"line"`
	Name                string `json:"name,omitempty"`
	Version             string `json:"version,omitempty"`
	Edition             string `json:"edition,omitempty"`
	Classification      string `json:"classification,omitempty"`
	GeneralAvailability string `json:"general_availability,omitempty"`
	PremierEnds         string `json:"premier_ends,omitempty"`
	ExtendedEnds        string `json:"extended_ends,omitempty"`
	SustainingEnds      string `json:"sustaining_ends,omitempty"`
	SupersededBy        string `json:"superseded_by,omitempty"`
	Terminal            *bool  `json:"terminal_release,omitempty"`
	Source              string `json:"source"`
}

// Patch retains a recorded release and package classification. Kind and
// Component use the catalog's labels; SourceKind retains a finer source label.
// Artifacts may be empty when the source records no verified package.
type Patch struct {
	Line          string     `json:"line"`
	Version       string     `json:"version,omitempty"`
	Kind          string     `json:"kind"`
	Component     string     `json:"component,omitempty"`
	SourceKind    string     `json:"source_kind,omitempty"`
	SourceName    string     `json:"source_name,omitempty"`
	SourceSubtype string     `json:"source_subtype,omitempty"`
	ProductName   string     `json:"product_name,omitempty"`
	Source        string     `json:"source"`
	Base          *bool      `json:"base_release,omitempty"`
	Terminal      *bool      `json:"terminal_release,omitempty"`
	Restricted    *bool      `json:"restricted,omitempty"`
	DisabledAt    string     `json:"disabled_at,omitempty"`
	Artifacts     []Artifact `json:"artifacts,omitempty"`
}

// Artifact carries the source's platform or hardware scope and package details.
// Numbers may repeat across versions and scopes. Empty dates are unresolved.
type Artifact struct {
	Number           int       `json:"number,omitempty"`
	Version          string    `json:"version,omitempty"`
	Component        string    `json:"component,omitempty"`
	Scope            string    `json:"scope,omitempty"`
	Platform         string    `json:"platform,omitempty"`
	MOSPlatform      string    `json:"mos_platform,omitempty"`
	PlatformID       *int      `json:"platform_id,omitempty"`
	ReleaseDate      string    `json:"release_date,omitempty"`
	Source           string    `json:"source,omitempty"`
	RecordedSourceID string    `json:"recorded_source_id,omitempty"`
	CorroboratedBy   *[]string `json:"corroborated_by,omitempty"`
	Base             *bool     `json:"base_release,omitempty"`
	Terminal         *bool     `json:"terminal_release,omitempty"`
	DisabledAt       string    `json:"disabled_at,omitempty"`
}

// Coverage records what an input provided for a product or release line.
// A status describes observed coverage, without asserting Oracle-wide completeness.
type Coverage struct {
	Line   string `json:"line,omitempty"`
	Source string `json:"source"`
	Status string `json:"status"`
}

// History holds the previously observed metadata for records corrected by
// a later verified refresh. Records dropped by a rolling source stay in
// the main catalog; they continue to reference their original snapshot.
type History struct {
	Releases []Release `json:"releases,omitempty"`
	Patches  []Patch   `json:"patches,omitempty"`
}

// Catalog is the reproducible offline view bundled with a module release.
type Catalog struct {
	SchemaVersion int        `json:"schema_version"`
	Product       Product    `json:"product"`
	AsOf          string     `json:"as_of"`
	Sources       []Source   `json:"sources"`
	Releases      []Release  `json:"releases"`
	Patches       []Patch    `json:"patches"`
	Coverage      []Coverage `json:"coverage"`
	Notes         []string   `json:"notes"`
	History       *History   `json:"history,omitempty"`
}

//go:embed data/*.json
var data embed.FS

// Load returns an independent copy of the bundled catalog for product.
func Load(product Product) (Catalog, error) {
	switch product {
	case Database, Exadata, ODA, SuperCluster:
	default:
		return Catalog{}, fmt.Errorf("unknown Oracle catalog product %q", product)
	}
	raw, err := data.ReadFile("data/" + string(product) + ".json")
	if err != nil {
		return Catalog{}, fmt.Errorf("read Oracle %s catalog: %w", product, err)
	}
	var catalog Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode Oracle %s catalog: %w", product, err)
	}
	if catalog.SchemaVersion != 1 || catalog.Product != product {
		return Catalog{}, fmt.Errorf("unexpected Oracle %s catalog schema or product", product)
	}
	return catalog, nil
}
