package objects

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/session"
)

// These limits bound the selected result and the Oracle IN list, respectively.
// They are API budgets, not database limits or query-cost guarantees.
const (
	MaxStatisticsRows   = 1000
	MaxStatisticsOwners = 1000
)

var (
	ErrUnsupportedStatisticsVersion = errors.New("unsupported Oracle statistics version")
	ErrInvalidStatisticsOptions     = errors.New("invalid Oracle statistics options")
	ErrStatisticsOwnersRequired     = errors.New("admitted statistics owners required")
	ErrInvalidStatisticsEvidence    = errors.New("invalid Oracle statistics evidence")
)

// StatisticsOptions selects a bounded current-container dictionary observation.
// OracleMaintainedAvailable is an admitted dictionary capability, not inferred
// from major 12: DBA_USERS documents this column from 12.1.0.2. Without it,
// callers must supply a reliably classified application-owner allowlist.
type StatisticsOptions struct {
	OracleMajor               int
	Limit                     int
	OracleMaintainedAvailable bool
}

// Statistics retains recorded optimizer values, not current row counts. A NULL
// NumRows or LastAnalyzed is unrecorded/unknown, never a synthesized zero or date.
// A selected NULL StatTypeLocked denotes no statistics lock; non-NULL lock labels
// remain exact source text. Partition and Subpartition are independent identity
// fields. Scope is unselected (nil) on 11g, or a selected nullable SHARED/SESSION
// value on 12+. No timezone, freshness, application-owner or health is inferred.
type Statistics struct {
	Owner          sql.NullString
	Table          sql.NullString
	Partition      sql.NullString
	Subpartition   sql.NullString
	ObjectType     sql.NullString
	NumRows        sql.NullString
	LastAnalyzed   sql.NullString
	StatTypeLocked sql.NullString
	Scope          *sql.NullString
}

// OldestStatisticsStatement selects at most Limit rows across admitted owners,
// with NULL analysis dates first, then oldest dates and binary identity ordering.
// The limit and owner names are bound values. An empty owner list requires the
// admitted ORACLE_MAINTAINED capability and selects only its exact N rows;
// that flag alone does not establish application ownership. Otherwise the caller
// supplies the entire owner admission, with no heuristic internal-schema list.
// The complete bounded selection is not a complete dictionary inventory.
// Callers own grants, current-container admission, deadlines and complete reads.
func OldestStatisticsStatement(options StatisticsOptions, owners ...string) (string, []any, error) {
	if err := validateStatisticsSelection(options.OracleMajor, options.Limit); err != nil {
		return "", nil, err
	}
	if len(owners) > MaxStatisticsOwners || (options.OracleMaintainedAvailable && options.OracleMajor < 12) {
		return "", nil, ErrInvalidStatisticsOptions
	}
	if !options.OracleMaintainedAvailable && len(owners) == 0 {
		return "", nil, ErrStatisticsOwnersRequired
	}
	for _, owner := range owners {
		if _, err := QuoteIdentifier(owner); err != nil {
			return "", nil, ErrInvalidStatisticsOptions
		}
	}
	filter, args := ownerFilter(owners)
	if options.OracleMaintainedAvailable {
		if filter == "" {
			filter = " WHERE "
		} else {
			filter += " AND "
		}
		filter += "EXISTS (SELECT 1 FROM DBA_USERS u WHERE u.USERNAME=s.OWNER AND u.ORACLE_MAINTAINED='N')"
	}
	names := statisticsNames(options.OracleMajor)
	outer := make([]string, len(names))
	for i, name := range names {
		outer[i] = name + " AS " + name
	}
	// Rank the raw DATE, never its formatted label. Scope disambiguates shared
	// and session-specific GTT statistics rather than merging their identities.
	order := "s.LAST_ANALYZED ASC NULLS FIRST"
	for _, name := range []string{"OWNER", "TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE"} {
		order += ", NLSSORT(s." + name + ", 'NLS_SORT=BINARY') ASC NULLS FIRST"
	}
	inner := "SELECT s.OWNER AS OWNER, s.TABLE_NAME AS TABLE_NAME, s.PARTITION_NAME AS PARTITION_NAME, s.SUBPARTITION_NAME AS SUBPARTITION_NAME, s.OBJECT_TYPE AS OBJECT_TYPE, " +
		"TO_CHAR(s.NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS NUM_ROWS, " +
		"TO_CHAR(s.LAST_ANALYZED, 'YYYY-MM-DD\"T\"HH24:MI:SS', 'NLS_CALENDAR=GREGORIAN') AS LAST_ANALYZED, s.STATTYPE_LOCKED AS STATTYPE_LOCKED"
	if options.OracleMajor >= 12 {
		inner += ", s.SCOPE AS SCOPE"
		order += ", NLSSORT(s.SCOPE, 'NLS_SORT=BINARY') ASC NULLS FIRST"
	}
	inner += ", ROW_NUMBER() OVER (ORDER BY " + order + ") AS STATISTICS_RANK FROM DBA_TAB_STATISTICS s" + filter
	args = append(args, options.Limit)
	return "SELECT " + strings.Join(outer, ", ") + " FROM (" + inner + ") WHERE STATISTICS_RANK<=:" +
		strconv.Itoa(len(args)) + " ORDER BY STATISTICS_RANK", args, nil
}

// DecodeStatistics validates all aliases, row widths and the admitted row bound
// before returning any facts. It preserves source order, duplicates and raw NULL
// values. A successful empty selection differs from an unavailable/failed read.
// Never pass a failed, truncated or unfinalized cursor prefix as a complete Result.
func DecodeStatistics(result session.Result, major, limit int) ([]Statistics, error) {
	if err := validateStatisticsSelection(major, limit); err != nil {
		return nil, err
	}
	if len(result.Rows) > limit {
		return nil, ErrInvalidStatisticsEvidence
	}
	index, ok := projection.Columns(result, statisticsNames(major)...)
	if !ok {
		return nil, ErrInvalidStatisticsEvidence
	}
	facts := make([]Statistics, 0, len(result.Rows))
	for _, row := range result.Rows {
		field := func(name string) sql.NullString {
			value := row[index[name]]
			if value.Null {
				return sql.NullString{}
			}
			return sql.NullString{String: strings.Clone(value.Text), Valid: true}
		}
		fact := Statistics{
			Owner: field("OWNER"), Table: field("TABLE_NAME"),
			Partition: field("PARTITION_NAME"), Subpartition: field("SUBPARTITION_NAME"),
			ObjectType: field("OBJECT_TYPE"), NumRows: field("NUM_ROWS"),
			LastAnalyzed: field("LAST_ANALYZED"), StatTypeLocked: field("STATTYPE_LOCKED"),
		}
		if major >= 12 {
			scope := field("SCOPE")
			fact.Scope = &scope
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func validateStatisticsSelection(major, limit int) error {
	if major < 11 {
		return ErrUnsupportedStatisticsVersion
	}
	if limit < 1 || limit > MaxStatisticsRows {
		return ErrInvalidStatisticsOptions
	}
	return nil
}

func statisticsNames(major int) []string {
	names := []string{"OWNER", "TABLE_NAME", "PARTITION_NAME", "SUBPARTITION_NAME", "OBJECT_TYPE", "NUM_ROWS", "LAST_ANALYZED", "STATTYPE_LOCKED"}
	if major >= 12 {
		names = append(names, "SCOPE")
	}
	return names
}
