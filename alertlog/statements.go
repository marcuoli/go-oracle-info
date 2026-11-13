// Package alertlog selects read-only Oracle operational facts.
package alertlog

import (
	"errors"
	"strconv"
)

// EntriesSQL selects the newest bounded error entries in the bound positive
// window (:1, seconds). Timestamps retain source offsets; this is a sample of
// the connected instance, not complete ADR history or all RAC members.
func EntriesSQL(maxRows int) (string, error) {
	if maxRows < 1 || maxRows > 10000 {
		return "", errors.New("row limit must be between 1 and 10000")
	}
	return `SELECT TO_CHAR(ORIGINATING_TIMESTAMP,'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM') AS ORIGINATING_TIMESTAMP, TO_CHAR(MESSAGE_TYPE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MESSAGE_TYPE, TO_CHAR(MESSAGE_LEVEL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MESSAGE_LEVEL, MESSAGE_TEXT FROM (SELECT ORIGINATING_TIMESTAMP,MESSAGE_TYPE,MESSAGE_LEVEL,MESSAGE_TEXT FROM V$DIAG_ALERT_EXT WHERE ORIGINATING_TIMESTAMP>=SYSTIMESTAMP-NUMTODSINTERVAL(:1,'SECOND') AND MESSAGE_TYPE IN (2,3) ORDER BY ORIGINATING_TIMESTAMP DESC,"PARTITION" DESC,RECORD_ID DESC) WHERE ROWNUM<=` + strconv.Itoa(maxRows), nil
}

// CountsSQL groups errors in the bound positive window (:1, seconds).
// Zero rows report no groups for that sample, not workload acceptance.
const CountsSQL = `SELECT TO_CHAR(MESSAGE_LEVEL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MESSAGE_LEVEL, TO_CHAR(MESSAGE_TYPE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MESSAGE_TYPE, TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MESSAGE_COUNT, TO_CHAR(MIN(ORIGINATING_TIMESTAMP),'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM') AS FIRST_SEEN, TO_CHAR(MAX(ORIGINATING_TIMESTAMP),'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM') AS LAST_SEEN FROM V$DIAG_ALERT_EXT WHERE ORIGINATING_TIMESTAMP>=SYSTIMESTAMP-NUMTODSINTERVAL(:1,'SECOND') AND MESSAGE_TYPE IN (2,3) GROUP BY MESSAGE_LEVEL,MESSAGE_TYPE ORDER BY MESSAGE_LEVEL,MESSAGE_TYPE`
