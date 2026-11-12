// Package identity selects and decodes Oracle database, instance and platform facts.
// Platform text is not evidence of an operating-system distribution or release.
package identity

import (
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// State preserves known text, SQL NULL and authoritative no-row results.
type State string

const (
	Available State = "available"
	Null      State = "null"
	NoRows    State = "no_rows"
)

// Text preserves the original source text without inferred or localized values.
type Text struct {
	State State
	Value string
}

// PlatformSource identifies the actual platform evidence source.
type PlatformSource string

const (
	DatabasePlatform PlatformSource = "v$database.platform_name"
	ServerTNSBanner  PlatformSource = "v$version.tns_banner"
)

// Database retains independent nullable database facts and platform provenance.
type Database struct {
	Name           Text
	Role           Text
	Platform       Text
	LogMode        Text
	PlatformSource PlatformSource
}

// Instance retains source instance identity. StartupLocal has no inferred timezone.
type Instance struct {
	ID           int
	Name         Text
	HostName     Text
	Version      Text
	StartupLocal Text
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle identity version")
	ErrInvalidEvidence    = errors.New("invalid Oracle identity evidence")
)

// DatabaseStatement selects the established platform source for a known release.
// Oracle 9 uses the server TNS banner because PLATFORM_NAME is absent there.
func DatabaseStatement(major int) (string, error) {
	if major < 9 {
		return "", ErrUnsupportedVersion
	}
	platform := "PLATFORM_NAME"
	if major == 9 {
		platform = "(SELECT MAX(SUBSTR(BANNER,9,INSTR(BANNER,':')-9)) AS PLATFORM_NAME FROM V$VERSION WHERE BANNER LIKE 'TNS for%:%')"
	}
	return "SELECT NAME AS DATABASE_NAME, DATABASE_ROLE AS DATABASE_ROLE, " + platform + " AS PLATFORM_NAME, LOG_MODE AS LOG_MODE FROM V$DATABASE", nil
}

// VersionStatement uses VERSION for bootstrap/legacy reads and VERSION_FULL only
// for a known Oracle 18+ source. Unknown release zero never probes a modern column.
func VersionStatement(major int) (string, error) {
	if major != 0 && major < 9 {
		return "", ErrUnsupportedVersion
	}
	column := "VERSION"
	if major >= 18 {
		column = "VERSION_FULL"
	}
	return "SELECT " + column + " AS VERSION FROM V$INSTANCE", nil
}

// InstanceStatement returns current-instance identity and a timezone-free startup
// label. It does not describe all RAC members or establish container admission.
func InstanceStatement() string {
	return "SELECT TO_CHAR(INSTANCE_NUMBER,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS INSTANCE_ID, INSTANCE_NAME AS INSTANCE_NAME, HOST_NAME AS HOST_NAME, VERSION AS VERSION, TO_CHAR(STARTUP_TIME,'YYYY-MM-DD\"T\"HH24:MI:SS') AS STARTUP_LOCAL FROM V$INSTANCE"
}

// Major parses the leading numeric release component without rewriting the value.
func Major(version string) (int, error) {
	if len(version) > 128 {
		return 0, ErrUnsupportedVersion
	}
	component, _, _ := strings.Cut(version, ".")
	if component == "" || strings.IndexFunc(component, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, ErrUnsupportedVersion
	}
	major, err := strconv.Atoi(component)
	if err != nil || major < 9 {
		return 0, ErrUnsupportedVersion
	}
	return major, nil
}

func row(result session.Result, names ...string) (map[string]session.Value, bool, error) {
	if len(result.Columns) != len(names) || len(result.Rows) > 1 {
		return nil, false, ErrInvalidEvidence
	}
	index := make(map[string]int, len(names))
	for i, name := range result.Columns {
		name = strings.ToUpper(name)
		if _, ok := index[name]; name == "" || ok {
			return nil, false, ErrInvalidEvidence
		}
		index[name] = i
	}
	for _, name := range names {
		if _, ok := index[name]; !ok {
			return nil, false, ErrInvalidEvidence
		}
	}
	if len(result.Rows) == 0 {
		return nil, true, nil
	}
	if len(result.Rows[0]) != len(names) {
		return nil, false, ErrInvalidEvidence
	}
	values := make(map[string]session.Value, len(names))
	for name, i := range index {
		values[name] = result.Rows[0][i]
	}
	return values, false, nil
}
func text(value session.Value) Text {
	if value.Null {
		return Text{State: Null}
	}
	return Text{State: Available, Value: strings.Clone(value.Text)}
}

// DecodeDatabase validates cardinality/columns and preserves every source value.
func DecodeDatabase(result session.Result, major int) (Database, error) {
	if _, err := DatabaseStatement(major); err != nil {
		return Database{}, err
	}
	source := DatabasePlatform
	if major == 9 {
		source = ServerTNSBanner
	}
	r, absent, err := row(result, "DATABASE_NAME", "DATABASE_ROLE", "PLATFORM_NAME", "LOG_MODE")
	if err != nil {
		return Database{}, err
	}
	if absent {
		return Database{Name: Text{State: NoRows}, Role: Text{State: NoRows}, Platform: Text{State: NoRows}, LogMode: Text{State: NoRows}, PlatformSource: source}, nil
	}
	return Database{Name: text(r["DATABASE_NAME"]), Role: text(r["DATABASE_ROLE"]), Platform: text(r["PLATFORM_NAME"]), LogMode: text(r["LOG_MODE"]), PlatformSource: source}, nil
}

// DecodeVersion preserves full source version text; normalization belongs to the
// consuming comparison/display contract, not to this evidence decoder.
func DecodeVersion(result session.Result) (Text, error) {
	r, absent, err := row(result, "VERSION")
	if err != nil {
		return Text{}, err
	}
	if absent {
		return Text{State: NoRows}, nil
	}
	return text(r["VERSION"]), nil
}

// DecodeInstance rejects an invalid instance ID rather than inventing identity.
func DecodeInstance(result session.Result) (Instance, error) {
	r, absent, err := row(result, "INSTANCE_ID", "INSTANCE_NAME", "HOST_NAME", "VERSION", "STARTUP_LOCAL")
	if err != nil {
		return Instance{}, err
	}
	if absent {
		return Instance{Name: Text{State: NoRows}, HostName: Text{State: NoRows}, Version: Text{State: NoRows}, StartupLocal: Text{State: NoRows}}, nil
	}
	id, err := strconv.Atoi(r["INSTANCE_ID"].Text)
	if err != nil || r["INSTANCE_ID"].Null || id <= 0 {
		return Instance{}, ErrInvalidEvidence
	}
	return Instance{ID: id, Name: text(r["INSTANCE_NAME"]), HostName: text(r["HOST_NAME"]), Version: text(r["VERSION"]), StartupLocal: text(r["STARTUP_LOCAL"])}, nil
}
