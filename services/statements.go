// Package services selects registered Oracle service names, not listener or application reachability.
package services

// NamesSQL lists connected-container services except the two internal session services.
const NamesSQL = `SELECT NAME AS SERVICE_NAME FROM V$SERVICES WHERE NAME NOT IN ('SYS$BACKGROUND','SYS$USERS') ORDER BY NAME`
