// Package backup selects read-only Oracle operational facts.
package backup

import (
	"errors"
	"strconv"
)

// JobsSQL selects at most maxRows recent RMAN jobs from the connected
// control-file view. DATE labels have no inferred timezone. No backup, restore,
// recovery or recovery-catalog connection is performed.
func JobsSQL(maxRows int) (string, error) {
	if maxRows < 1 || maxRows > 10000 {
		return "", errors.New("row limit must be between 1 and 10000")
	}
	return `SELECT INPUT_TYPE, STATUS, TO_CHAR(START_TIME,'YYYY-MM-DD"T"HH24:MI:SS') AS START_TIME, TO_CHAR(END_TIME,'YYYY-MM-DD"T"HH24:MI:SS') AS END_TIME, TO_CHAR(OUTPUT_BYTES, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OUTPUT_BYTES FROM (SELECT INPUT_TYPE,STATUS,START_TIME,END_TIME,OUTPUT_BYTES FROM V$RMAN_BACKUP_JOB_DETAILS ORDER BY END_TIME DESC NULLS LAST) WHERE ROWNUM<=` + strconv.Itoa(maxRows), nil
}
