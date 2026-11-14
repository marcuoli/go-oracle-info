// Package parameters selects read-only effective instance and stored SPFILE
// parameter rows in an already-admitted current container.
package parameters

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-session/rowset"
)

// Filter selects exact parameter names and reporting instance numbers. Empty
// slices select all visible values, not proven complete RAC membership.
type Filter struct {
	Names     []string
	Instances []int64
}

// SPFileFilter distinguishes reporting INST_ID from stored SID applicability.
// Empty SIDs preserves all clauses, including dormant SIDs. Named SIDs also
// retain wildcard defaults; rows are never merged into inferred effective values.
type SPFileFilter struct {
	Names     []string
	Instances []int64
	SIDs      []string
}

// MaxFilterValues includes an automatically retained SPFILE wildcard in its IN budget.
const MaxFilterValues = 1000

var (
	// ErrInvalidFilter rejects invalid strings/IDs or an Oracle IN-list budget overflow.
	ErrInvalidFilter = errors.New("invalid parameter filter")
	// ErrUnsupportedVersion rejects a major below the reviewed11g source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle parameter source version")
)

// Provenance describes only the selected dynamic-view observations. It is not
// evidence of complete physical SPFILE contents, RAC membership or inheritance.
type Provenance struct {
	Source      string   `json:"source"`
	OracleMajor int      `json:"oracle_major"`
	Scope       string   `json:"scope"`
	Names       []string `json:"names,omitempty"`
	Instances   []int64  `json:"instances,omitempty"`
	SIDs        []string `json:"sids,omitempty"`
	SIDPolicy   string   `json:"sid_policy,omitempty"`
	NameClass   string   `json:"name_class,omitempty"`
}

// EffectiveStatement selects instance-effective SYSTEM_PARAMETER2 values, not
// session PARAMETER values or stored definitions. CON_ID12+ is explicitly scoped
// to whole-database rows(0) plus the current session container, never other PDBs.
func EffectiveStatement(major int, filter Filter) (string, []any, error) {
	plan, err := effectivePlan(major, filter)
	return plan.sql, plan.args, err
}

// HiddenStatement selects only leading-underscore names visible in the same
// instance-effective source and shape as EffectiveStatement. It is not the
// complete internal/default parameter catalog. Report selection stays with callers.
func HiddenStatement(major int, filter Filter) (string, []any, error) {
	plan, err := hiddenPlan(major, filter)
	return plan.sql, plan.args, err
}

// SPFileStatement retains raw stored clauses and ISSPECIFIED, even when FALSE.
// INST_ID is the reporting instance; it is never equated to SID applicability.
func SPFileStatement(major int, filter SPFileFilter) (string, []any, error) {
	plan, err := spfilePlan(major, filter)
	return plan.sql, plan.args, err
}

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance Provenance
}

func effectivePlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	if err := validate(filter.Names, filter.Instances, nil); err != nil {
		return selection{}, err
	}
	provenance := source("GV$SYSTEM_PARAMETER2", major, filter.Names, filter.Instances)
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	for _, name := range []string{"NUM", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISDEFAULT", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE"} {
		kind := rowset.Text
		if name == "NUM" || name == "TYPE" || name == "ORDINAL" {
			kind = rowset.Integer
		}
		columns = append(columns, rowset.Column{Name: name, Kind: kind})
	}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "ISPDB_MODIFIABLE", Kind: rowset.Text})
	}
	for _, name := range []string{"ISINSTANCE_MODIFIABLE", "ISMODIFIED", "ISADJUSTED", "ISDEPRECATED", "ISBASIC", "DESCRIPTION", "UPDATE_COMMENT"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	return build(major, provenance, columns, nil), nil
}

func hiddenPlan(major int, filter Filter) (selection, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return selection{}, err
	}
	plan.provenance.NameClass = "leading_underscore_only"
	return build(major, plan.provenance, plan.schema, nil), nil
}
func spfilePlan(major int, filter SPFileFilter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	sids := append([]string(nil), filter.SIDs...)
	if len(sids) > 0 {
		found := false
		for _, sid := range sids {
			found = found || sid == "*"
		}
		if !found {
			sids = append(sids, "*")
		}
	}
	if err := validate(filter.Names, filter.Instances, sids); err != nil {
		return selection{}, err
	}
	provenance := source("GV$SPPARAMETER", major, filter.Names, filter.Instances)
	provenance.SIDs = sids
	provenance.SIDPolicy = "all_stored_clauses"
	if len(sids) > 0 {
		provenance.SIDPolicy = "selected_sids_plus_wildcard"
	}
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	for _, name := range []string{"SID", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISSPECIFIED", "UPDATE_COMMENT"} {
		kind := rowset.Text
		if name == "ORDINAL" {
			kind = rowset.Integer
		}
		columns = append(columns, rowset.Column{Name: name, Kind: kind})
	}
	return build(major, provenance, columns, sids), nil
}

func source(view string, major int, names []string, instances []int64) Provenance {
	scope := "non_cdb_visible_instance_rows"
	if major >= 12 {
		scope = "current_container_and_whole_database_rows"
	}
	return Provenance{Source: view, OracleMajor: major, Scope: scope, Names: append([]string(nil), names...), Instances: append([]int64(nil), instances...)}
}

func validate(names []string, instances []int64, sids []string) error {
	if len(names) > MaxFilterValues || len(instances) > MaxFilterValues || len(sids) > MaxFilterValues {
		return ErrInvalidFilter
	}
	for _, values := range [][]string{names, sids} {
		for _, value := range values {
			if value == "" || len(value) > 80 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return ErrInvalidFilter
			}
		}
	}
	for _, id := range instances {
		if id < 1 {
			return ErrInvalidFilter
		}
	}
	return nil
}

func build(major int, provenance Provenance, columns []rowset.Column, sids []string) selection {
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "p." + column.Name + " AS " + column.Name
	}
	clauses := []string{}
	// Prefix admission must not inherit a caller's linguistic collation.
	if provenance.NameClass == "leading_underscore_only" {
		clauses = append(clauses, `NLSSORT(SUBSTR(p.NAME,1,1), 'NLS_SORT=BINARY') = NLSSORT('_', 'NLS_SORT=BINARY')`)
	}
	args := make([]any, 0, len(provenance.Names)+len(provenance.Instances)+len(sids))
	if major >= 12 {
		clauses = append(clauses, "p.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))")
	}
	if len(provenance.Instances) > 0 {
		binds := make([]string, len(provenance.Instances))
		for i, id := range provenance.Instances {
			args = append(args, id)
			binds[i] = ":" + strconv.Itoa(len(args))
		}
		clauses = append(clauses, "p.INST_ID IN ("+strings.Join(binds, ",")+")")
	}
	for _, filter := range []struct {
		name   string
		values []string
	}{{"NAME", provenance.Names}, {"SID", sids}} {
		if len(filter.values) == 0 {
			continue
		}
		binds := make([]string, len(filter.values))
		for i, value := range filter.values {
			args = append(args, value)
			binds[i] = "NLSSORT(:" + strconv.Itoa(len(args)) + ", 'NLS_SORT=BINARY')"
		}
		clauses = append(clauses, "NLSSORT(p."+filter.name+", 'NLS_SORT=BINARY') IN ("+strings.Join(binds, ",")+")")
	}
	text := "SELECT " + strings.Join(aliases, ", ") + " FROM " + provenance.Source + " p"
	if len(clauses) > 0 {
		text += " WHERE " + strings.Join(clauses, " AND ")
	}
	text += " ORDER BY p.INST_ID"
	if major >= 12 {
		text += ", p.CON_ID"
	}
	text += ", NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
	if provenance.Source == "GV$SPPARAMETER" {
		text += ", NLSSORT(p.SID, 'NLS_SORT=BINARY')"
	}
	text += ", p.ORDINAL"
	return selection{sql: text, args: args, schema: columns, provenance: provenance}
}
