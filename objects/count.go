package objects

import (
	"errors"
	"strconv"
)

// MaxParallelDegree is the API's bounded hint degree, not a database capacity
// limit or a promise that Oracle will honor the hint or finish the count sooner.
const MaxParallelDegree = 1024

// CountOptions controls an optional statement hint. Zero ParallelDegree preserves
// the legacy statement exactly; positive values request PARALLEL(n). The caller
// owns workload admission, query concurrency, timeouts and database qualification.
type CountOptions struct {
	ParallelDegree int
}

// CountStatementWithOptions returns an exact decimal COUNT(*) for safely quoted
// identifiers and a validated integer degree. It never changes session settings.
// Oracle may ignore the hint; it cannot establish cutoff consistency or access.
func CountStatementWithOptions(owner, table string, options CountOptions) (string, error) {
	if options.ParallelDegree < 0 || options.ParallelDegree > MaxParallelDegree {
		return "", errors.New("parallel degree outside 0..1024")
	}
	o, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	t, err := QuoteIdentifier(table)
	if err != nil {
		return "", err
	}
	selectClause := "SELECT "
	if options.ParallelDegree > 0 {
		selectClause += "/*+ PARALLEL(" + strconv.Itoa(options.ParallelDegree) + ") */ "
	}
	return selectClause + "TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT FROM " + o + "." + t, nil
}

// TablesStatisticsSQL reads optimizer row estimates for the bound owner (:1).
// NULL NUM_ROWS/LAST_ANALYZED remain NULL. Dates have database-local precision
// without an invented offset. The expression pins the Gregorian calendar;
// callers retain age, missing-statistics and policy
// decisions. These are recorded statistics, never an exact current row count.
const TablesStatisticsSQL = statisticsColumns + " FROM DBA_TABLES WHERE OWNER=:1 AND TEMPORARY='N' AND NESTED='NO' AND SECONDARY='N' AND NVL(IOT_TYPE,'HEAP')<>'IOT_OVERFLOW' ORDER BY TABLE_NAME"

// TableStatisticsSQL reads the same statistics for bound owner (:1) and table
// (:2), retaining empty results when the declared ordinary table is not visible.
const TableStatisticsSQL = statisticsColumns + " FROM DBA_TABLES WHERE OWNER=:1 AND TABLE_NAME=:2 AND TEMPORARY='N' AND NESTED='NO' AND SECONDARY='N' AND NVL(IOT_TYPE,'HEAP')<>'IOT_OVERFLOW'"

const statisticsColumns = "SELECT TABLE_NAME, TO_CHAR(NUM_ROWS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT, TO_CHAR(LAST_ANALYZED, 'YYYY-MM-DD\"T\"HH24:MI:SS', 'NLS_CALENDAR=GREGORIAN') AS LAST_ANALYZED"
