package releases

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
)

// AutoUpgradeManifestURL is the JSON checksum link published by Oracle's
// database upgrades page.
const AutoUpgradeManifestURL = "https://download.oracle.com/otn-pub/otn_software/autoupgrade.json"

const autoUpgradeDownloadURL = "https://download.oracle.com/otn-pub/otn_software/autoupgrade.jar"

// AutoUpgradeArtifact identifies a public manifest observation or an Oracle
// Support attachment. A public URL is mutable. Verify the fetched bytes against
// SHA256 and Size from the same manifest. An archived attachment has no
// public download URL. An empty Filename or SHA256 means the reviewed
// evidence did not identify a filename or exact digest.
type AutoUpgradeArtifact struct {
	Version         string
	Build           string
	Filename        string
	DownloadURL     string
	AttachmentID    string
	SHA256          string
	Size            int64
	ReleaseDateTime string
	Source          Source
	CapturedUTC     string
	ArchiveCommit   string
}

var (
	ErrInvalidAutoUpgrade     = errors.New("invalid Oracle AutoUpgrade metadata")
	ErrAutoUpgradeUnavailable = errors.New("Oracle AutoUpgrade metadata unavailable")
	ErrAmbiguousAutoUpgrade   = errors.New("ambiguous Oracle AutoUpgrade build")
)

// DecodeAutoUpgradeManifest reads Oracle's public JSON metadata. Fetching the
// manifest and verifying the JAR remain caller-owned.
func DecodeAutoUpgradeManifest(reader io.Reader) (AutoUpgradeArtifact, error) {
	var manifest struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		DownloadURL     string `json:"downloadUrl"`
		Checksum        string `json:"checksum"`
		ReleaseDateTime string `json:"releaseDateTime"`
		Size            int64  `json:"size"`
	}
	raw, err := readReleaseJSON(reader, &manifest)
	if err != nil {
		return AutoUpgradeArtifact{}, fmt.Errorf("%w: %w", ErrInvalidAutoUpgrade, err)
	}
	_, validVersion := oracleversion.Parse(manifest.Version)
	_, dateErr := time.Parse(time.RFC3339Nano, manifest.ReleaseDateTime)
	if !validVersion || dateErr != nil || manifest.Name != "autoupgrade.jar" ||
		manifest.DownloadURL != autoUpgradeDownloadURL || !isHexLength(manifest.Checksum, 64) || manifest.Size <= 0 {
		return AutoUpgradeArtifact{}, ErrInvalidAutoUpgrade
	}
	return AutoUpgradeArtifact{
		Version: manifest.Version, Filename: manifest.Name, DownloadURL: manifest.DownloadURL,
		SHA256: strings.ToLower(manifest.Checksum), Size: manifest.Size, ReleaseDateTime: manifest.ReleaseDateTime,
		Source: Source{ID: "oracle-autoupgrade-manifest", Title: "Oracle AutoUpgrade JSON Checksum",
			URL: AutoUpgradeManifestURL, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw))},
	}, nil
}

// LatestArchivedAutoUpgrade selects the numerically greatest reviewed build
// from Oracle Support article KB123450 (old Doc ID 2485457.1). It preserves
// the attachment identity and any digest verified for those exact bytes.
func LatestArchivedAutoUpgrade(reader io.Reader) (AutoUpgradeArtifact, error) {
	var snapshot struct {
		SchemaVersion int `json:"schema_version"`
		Builds        []struct {
			Version       string `json:"version"`
			Build         string `json:"build"`
			Filename      string `json:"filename"`
			AttachmentID  string `json:"attachment_id"`
			SHA256        string `json:"sha256,omitempty"`
			Source        Source `json:"source"`
			CapturedUTC   string `json:"captured_utc,omitempty"`
			ArchiveCommit string `json:"archive_commit,omitempty"`
		} `json:"builds"`
	}
	if _, err := readReleaseJSON(reader, &snapshot); err != nil {
		return AutoUpgradeArtifact{}, fmt.Errorf("%w: %w", ErrInvalidAutoUpgrade, err)
	}
	if snapshot.SchemaVersion != 1 || snapshot.Builds == nil {
		return AutoUpgradeArtifact{}, ErrInvalidAutoUpgrade
	}

	var latest AutoUpgradeArtifact
	var greatest [5]uint64
	seen := make(map[[5]uint64]struct{}, len(snapshot.Builds))
	attachments := make(map[string]struct{}, len(snapshot.Builds))
	for i, entry := range snapshot.Builds {
		build, validBuild := oracleversion.Parse(entry.Build)
		if !validBuild || strings.Count(entry.Version, ".") != 1 || strings.Count(entry.Build, ".") != 2 ||
			!strings.HasPrefix(entry.Build, entry.Version+".") ||
			(entry.Filename != "" && entry.Filename != "autoupgrade.jar" && entry.Filename != "autoupgrade_"+entry.Build+".jar") ||
			!validAutoUpgradeAttachment(entry.Source, entry.Build, entry.AttachmentID, entry.Filename, entry.CapturedUTC, entry.ArchiveCommit) ||
			(entry.SHA256 != "" && !isHexLength(entry.SHA256, 64)) ||
			!validOracleSupportCapture(entry.Source, entry.CapturedUTC, entry.ArchiveCommit) {
			return AutoUpgradeArtifact{}, fmt.Errorf("%w: invalid archive entry %d", ErrInvalidAutoUpgrade, i+1)
		}
		if _, found := seen[build]; found {
			return AutoUpgradeArtifact{}, fmt.Errorf("%w: multiple entries for build %s", ErrAmbiguousAutoUpgrade, entry.Build)
		}
		if _, found := attachments[entry.AttachmentID]; found {
			return AutoUpgradeArtifact{}, fmt.Errorf("%w: attachment reused across builds", ErrAmbiguousAutoUpgrade)
		}
		seen[build], attachments[entry.AttachmentID] = struct{}{}, struct{}{}
		if latest.Build == "" || slices.Compare(build[:], greatest[:]) > 0 {
			greatest = build
			latest = AutoUpgradeArtifact{Version: entry.Version, Build: entry.Build, Filename: entry.Filename,
				AttachmentID: entry.AttachmentID, SHA256: strings.ToLower(entry.SHA256), Source: entry.Source,
				CapturedUTC: entry.CapturedUTC, ArchiveCommit: entry.ArchiveCommit}
		}
	}
	if latest.Build == "" {
		return AutoUpgradeArtifact{}, ErrAutoUpgradeUnavailable
	}
	return latest, nil
}

func validAutoUpgradeAttachment(source Source, build, id, filename, capturedUTC, archiveCommit string) bool {
	switch source.ID {
	case "KB123450":
		return source.OldDocID == "2485457.1" && capturedUTC != "" && archiveCommit == "" && filename != "" && isHexLength(id, 32)
	case "2485457.1":
		return (source.OldDocID == "" || source.OldDocID == source.ID) && capturedUTC == "" && archiveCommit != "" &&
			id == "2485457.1:AUTOUPGRADE"+strings.ReplaceAll(build, ".", "")
	default:
		return false
	}
}
