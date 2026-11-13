// Package dataguard selects connected Data Guard observations without topology or readiness inference.
package dataguard

// StatusSQL reads role, protection and switchover status from the connected database.
const StatusSQL = `SELECT DATABASE_ROLE, PROTECTION_MODE, PROTECTION_LEVEL, SWITCHOVER_STATUS FROM V$DATABASE`

// LagSQL reads available transport/apply observations; absent rows are not zero lag.
const LagSQL = `SELECT NAME, VALUE, UNIT, TIME_COMPUTED, DATUM_TIME FROM V$DATAGUARD_STATS WHERE NAME IN ('transport lag','apply lag','apply finish time') ORDER BY NAME`
