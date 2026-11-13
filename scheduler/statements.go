// Package scheduler selects scheduler metadata without executing or changing jobs.
package scheduler

// JobsSQL reads jobs for the exact bound owner (:1).
// Callers supply the intended dictionary spelling and bound execution scope.
const JobsSQL = `SELECT OWNER, JOB_NAME, ENABLED, STATE, TO_CHAR(FAILURE_COUNT, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS FAILURE_COUNT, TO_CHAR(NEXT_RUN_DATE,'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM') AS NEXT_RUN_DATE FROM DBA_SCHEDULER_JOBS WHERE OWNER=:1 ORDER BY JOB_NAME`
