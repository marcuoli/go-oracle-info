// Package passwordfile selects and decodes observed Oracle password-file metadata.
// It reads no file contents, credentials or users and changes no cache or setting.
package passwordfile

import (
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
)

// Options admits a numeric Oracle release and optional source column. Use
// DefaultOptions for documented release defaults, or explicitly narrow/admit
// the projection. No option probes/retries after a source-query failure.
type Options struct {
	OracleVersion   string
	IncludeExascale bool
}

// Provenance describes a connected-instance metadata observation. CON_ID0 is
// CDB-shared file metadata, not a per-PDB file or complete RAC/file-system proof.
type Provenance struct {
	Source         string `json:"source"`
	OracleVersion  string `json:"oracle_version"`
	Scope          string `json:"scope"`
	ExascaleColumn bool   `json:"exascale_column"`
}

var (
	// ErrUnsupportedVersion rejects releases below12.2 or invalid numeric evidence.
	ErrUnsupportedVersion = errors.New("password-file metadata requires Oracle 12.2.0.1 or later")
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("password-file reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed metadata, values or extra source rows.
	ErrInvalidEvidence = errors.New("invalid Oracle password-file evidence")
)

type selection struct {
	sql        string
	schema     []rowset.Column
	provenance Provenance
}

// Statement selects metadata only. The base projection exists from12.2.0.1;
// optional IS_EXC requires explicit column admission, not a guessed version gate.
func Statement(options Options) (string, error) {
	plan, err := selectMetadata(options)
	return plan.sql, err
}

// DefaultOptions selects documented columns for numeric version evidence:
// the common view from12.2 and IS_EXC from21c. This is a static source contract,
// not a metadata probe or native compatibility certification. Explicit Options
// remain available when the caller admits a narrower or different projection.
func DefaultOptions(oracleVersion string) (Options, error) {
	if !supported(oracleVersion) {
		return Options{}, ErrUnsupportedVersion
	}
	leading, _, _ := strings.Cut(oracleVersion, ".")
	major, _ := strconv.Atoi(leading)
	return Options{OracleVersion: oracleVersion, IncludeExascale: major >= 21}, nil
}
func selectMetadata(options Options) (selection, error) {
	if !supported(options.OracleVersion) {
		return selection{}, ErrUnsupportedVersion
	}
	columns := []rowset.Column{{Name: "FILE_NAME", Kind: rowset.Text}, {Name: "FORMAT", Kind: rowset.Text}, {Name: "IS_ASM", Kind: rowset.Text}}
	if options.IncludeExascale {
		columns = append(columns, rowset.Column{Name: "IS_EXC", Kind: rowset.Text})
	}
	columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "p." + column.Name + " AS " + column.Name
	}
	return selection{
		sql:        "SELECT " + strings.Join(aliases, ", ") + " FROM V$PASSWORDFILE_INFO p",
		schema:     columns,
		provenance: Provenance{Source: "V$PASSWORDFILE_INFO", OracleVersion: options.OracleVersion, Scope: "connected_instance_database_password_file", ExascaleColumn: options.IncludeExascale},
	}, nil
}

func supported(version string) bool {
	if len(version) == 0 || len(version) > 128 {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) > 5 {
		return false
	}
	numbers := [5]int{}
	for i, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		numbers[i] = number
	}
	minimum := [5]int{12, 2, 0, 1, 0}
	for i, number := range numbers {
		if number != minimum[i] {
			return number > minimum[i]
		}
	}
	return true
}
