// Package performance selects point-in-time observations without workload qualification or recurring sampling.
package performance

// BlockersSQL reads current sessions reporting a valid blocker, not historical lock coverage.
const BlockersSQL = `SELECT TO_CHAR(SID, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SID, TO_CHAR(SERIAL#, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS SERIAL_NUMBER, TO_CHAR(BLOCKING_INSTANCE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS BLOCKING_INSTANCE, TO_CHAR(BLOCKING_SESSION, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS BLOCKING_SESSION, EVENT FROM V$SESSION WHERE BLOCKING_SESSION_STATUS='VALID' ORDER BY SID`

// ResourceLimitsSQL reads current/peak limits for the connected instance.
const ResourceLimitsSQL = `SELECT RESOURCE_NAME, TO_CHAR(CURRENT_UTILIZATION, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CURRENT_UTILIZATION, TO_CHAR(MAX_UTILIZATION, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS MAX_UTILIZATION, LIMIT_VALUE FROM V$RESOURCE_LIMIT ORDER BY RESOURCE_NAME`
