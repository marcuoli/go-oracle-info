package goldengate

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// DictionaryVisibility chooses a source before execution. Denial never causes
// a fallback. The zero value selects the current user's accessible ALL view.
type DictionaryVisibility string

const (
	AccessibleDictionary DictionaryVisibility = "all"
	DatabaseDictionary   DictionaryVisibility = "dba"
)

// DiscoveryFilter holds exact caller selections. Schema is a heartbeat object
// owner, never inferred GGSCHEMA. Extracts and Replicats are external group names.
// Empty fields select all visible rows; they never select a conventional owner.
type DiscoveryFilter struct {
	Schema              string
	Extracts, Replicats []string
}

// ValidateDiscoveryFilter validates bounded exact names without normalizing them.
// Schema is at most 128 UTF-8 bytes; process filter names are at most 128 bytes,
// with at most 1000 entries per list. This bounds inputs, not source values.
func ValidateDiscoveryFilter(f DiscoveryFilter) error {
	if f.Schema != "" && !identifier(f.Schema) {
		return ErrInvalidRequest
	}
	if validateNames(f.Extracts, 128) != nil || validateNames(f.Replicats, 128) != nil {
		return ErrInvalidRequest
	}
	return nil
}

// ProcessOptions admits the observed version, dictionary visibility and exact
// group-name selection independently for Extracts or Replicats.
type ProcessOptions struct {
	OracleVersion string
	Visibility    DictionaryVisibility
	Names         []string
}

// ProcessRequest supplies the caller's admitted complete reader reference.
type ProcessRequest struct {
	Options   ProcessOptions
	Reference reference.Reference
}

func discoveryVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 5 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return false
		}
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return false
		}
	}
	switch parts[0] {
	case "19", "21", "23":
		return true
	}
	return false
}

func identifier(s string) bool {
	return s != "" && len(s) <= 128 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

func quoteIdentifier(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func validateNames(names []string, maxBytes int) error {
	if len(names) > 1000 {
		return ErrInvalidRequest
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == "" || len(name) > maxBytes || !utf8.ValidString(name) || strings.ContainsRune(name, 0) || seen[name] {
			return ErrInvalidRequest
		}
		seen[name] = true
	}
	return nil
}

func processSource(o ProcessOptions, suffix string) (string, error) {
	if !discoveryVersion(o.OracleVersion) {
		return "", ErrUnsupportedVersion
	}
	if validateNames(o.Names, 128) != nil {
		return "", ErrInvalidRequest
	}
	switch o.Visibility {
	case "", AccessibleDictionary:
		return "ALL_" + suffix, nil
	case DatabaseDictionary:
		return "DBA_" + suffix, nil
	}
	return "", ErrInvalidRequest
}

type discoveryPredicate struct{ columns, names []string }

// Explicit BINARY keys avoid session NLS_COMP/NLS_SORT changing exact matching.
// Filter inputs are bounded to keep each binary key below legacy RAW key limits.
func addDiscoveryFilter(p *selection, columns, names []string) {
	if len(names) == 0 {
		return
	}
	marks := make([]string, len(names))
	for i, name := range names {
		p.args = append(p.args, name)
		marks[i] = "NLSSORT(:" + strconv.Itoa(len(p.args)) + ", 'NLS_SORT=BINARY')"
	}
	clauses := make([]string, len(columns))
	for i, column := range columns {
		clauses[i] = "NLSSORT(v." + column + ", 'NLS_SORT=BINARY') IN (" + strings.Join(marks, ", ") + ")"
	}
	conjunction := " WHERE "
	if strings.Contains(p.sql, " WHERE ") {
		conjunction = " AND "
	}
	p.sql += conjunction + "(" + strings.Join(clauses, " OR ") + ")"
	p.filters = append(p.filters, discoveryPredicate{columns: columns, names: append([]string(nil), names...)})
}

var signedSeconds = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[Ee][+-]?[0-9]+)?$`)

const heartbeatTimeLayout = "2006-01-02T15:04:05.000000"

func discoveryColumns(result session.Result, index map[string]int, p selection) error {
	catalogStatuses := map[[3]string]session.Value{}
	for _, row := range result.Rows {
		if p.catalog {
			key := [3]string{row[index["OWNER"]].Text, row[index["OBJECT_NAME"]].Text, row[index["OBJECT_TYPE"]].Text}
			status := row[index["STATUS"]]
			if previous, exists := catalogStatuses[key]; exists && previous != status {
				return ErrInvalidEvidence
			}
			catalogStatuses[key] = status
		}
		if p.catalog && !catalogColumns(row, index) {
			return ErrInvalidEvidence
		}
		for _, name := range p.required {
			v := row[index[name]]
			if v.Null || v.Text == "" {
				return ErrInvalidEvidence
			}
		}
		if p.ggPurpose && row[index["PURPOSE"]].Text != "GoldenGate Capture" {
			return ErrInvalidEvidence
		}
		for _, name := range p.decimals {
			v := row[index[name]]
			if !v.Null && (len(v.Text) > 128 || !signedSeconds.MatchString(v.Text)) {
				return ErrInvalidEvidence
			}
		}
		for _, name := range p.timestamps {
			v := row[index[name]]
			if v.Null {
				continue
			}
			t, err := time.Parse(heartbeatTimeLayout, v.Text)
			if err != nil || t.Year() < 1 || t.Format(heartbeatTimeLayout) != v.Text {
				return ErrInvalidEvidence
			}
		}
		for _, filter := range p.filters {
			matched := false
			for _, column := range filter.columns {
				v := row[index[column]]
				for _, name := range filter.names {
					matched = matched || !v.Null && v.Text == name
				}
			}
			if !matched {
				return ErrInvalidEvidence
			}
		}
	}
	return nil
}

// heartbeatTimestamp renders Gregorian civil TIMESTAMP(6) fields independently
// of NLS_CALENDAR, numeric characters and driver timestamp/timezone conversion.
// Stored GoldenGate timestamps are UTC by its contract, without a zone column.
func heartbeatTimestamp(expression string) string {
	parts := []string{"YEAR", "MONTH", "DAY", "HOUR", "MINUTE", "SECOND"}
	formats := []string{"FM0000", "FM00", "FM00", "FM00", "FM00", "FM00D000000"}
	separators := []string{"-", "-", "T", ":", ":"}
	text := "CASE WHEN " + expression + " IS NULL THEN NULL ELSE "
	for i, part := range parts {
		if i > 0 {
			text += " || '" + separators[i-1] + "' || "
		}
		text += "TO_CHAR(EXTRACT(" + part + " FROM " + expression + "), '" + formats[i] + "', 'NLS_NUMERIC_CHARACTERS=''.,''')"
	}
	return text + " END"
}
