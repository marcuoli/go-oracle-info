# Oldest recorded optimizer statistics

Checked against Oracle documentation on 2026-11-14. This connection-free API
selects table, partition and subpartition statistics in the caller-admitted
current database/container. It neither gathers statistics nor changes sessions,
containers, grants or endpoints. Existing count and DBA_TABLES selectors are
unchanged.

## Selection and admission

Use objects.OldestStatisticsStatement with StatisticsOptions and optional bound
owners. Limit must be 1..MaxStatisticsRows (1000). A reporting limit of 10 selects
ten rows total across the admitted owners, not ten per owner. Owners remain
exact, case-preserving bound values; at most MaxStatisticsOwners (1000) are
accepted. A limit is required; there is no unbounded mode.

OracleMaintainedAvailable must reflect admitted DBA_USERS column availability.
Oracle's 12.1 reference explicitly documents ORACLE_MAINTAINED from 12.1.0.2.
A major version of 12 alone is insufficient. When true, an EXISTS predicate
requires the matching user's ORACLE_MAINTAINED='N'; optional owners narrow this
further. This excludes users marked as Oracle-maintained by that dictionary,
not every possible vendor product or operator-defined internal account.
Application-owner classification remains the caller's responsibility.

When that capability is false, a nonempty, reliably admitted application-owner
allowlist is mandatory. This includes 11g and a 12.1 deployment without the
column. No static username denylist, USER_ID threshold, migration-schema input
or account-name pattern establishes that classification. Without reliable owner
admission, report unavailable (ErrStatisticsOwnersRequired), not zero objects
or a proven Oracle-owned-schema exclusion. There is no fallback to all users.

The SQL reads DBA_TAB_STATISTICS with explicit aliases. ROW_NUMBER ranks the raw
LAST_ANALYZED DATE ascending, NULLs first, followed by binary owner/table/
partition/subpartition/type order and statistics scope where selected. The outer
query applies the bound limit and explicitly orders by that rank. It uses no
post-11g FETCH syntax, locale-dependent identifier ordering or formatted-date
sorting. The row bound limits the selected result, not database work or elapsed
time; grants, source availability and execution budgets remain caller-owned.
The result is the complete bounded selection, not the full inventory. It does
not report how many later records exist. NULL analysis dates come first as
unrecorded dates; they do not prove an object has never had statistics historically.

## Decoding and source semantics

Pass only the complete, successful session.Result to
objects.DecodeStatistics(result, major, limit). It rejects mismatched aliases,
duplicate columns, any malformed row and any result exceeding the admitted
limit before returning facts. Query, cursor-finalization, truncation and access
errors must not be converted to a successful empty selection. The Result type
does not encode transport completeness; the caller establishes that first.

Statistics retains Owner, Table, Partition, Subpartition, ObjectType, NumRows,
LastAnalyzed and StatTypeLocked as sql.NullString. Both parent partition and
subpartition survive; the caller must preserve both in display/export identity.
NUM_ROWS is recorded optimizer evidence, never an exact current COUNT(*).
TM9 plus explicit numeric characters preserves exact decimal text without a
float conversion. LAST_ANALYZED uses Gregorian civil seconds without an invented
offset/timezone. SQL NULL numeric/date evidence remains unrecorded/unknown.
A selected SQL NULL STATTYPE_LOCKED is the unlocked state; non-NULL lock labels
(including unexpected future labels) remain raw, not reduced to a boolean.
The decoder does not derive an age, stale status or action.

On Oracle 12+, Scope is a selected nullable pointer value that retains shared
versus session-specific global-temporary-table statistics. On 11g it is nil
because the column is not selected. A non-nil pointer with Valid=false is a
selected SQL NULL. Repeated or conflicting rows and source order are preserved,
not deduplicated or combined. No native Oracle/driver/platform qualification
follows from source review or fixtures.

## Oracle sources

- Oracle Database Reference 11.2, ALL_TAB_STATISTICS (E17110-04), base identity,
  numeric/date/lock columns:
  https://docs.oracle.com/cd/E18283_01/server.112/e17110/statviews_2114.htm
- Oracle Database Reference 11.2, DBA_USERS (E24448-01), no documented
  ORACLE_MAINTAINED:
  https://docs.oracle.com/cd/E24693_01/server.11203/e24448/statviews_5081.htm
- Oracle Database Reference 12.1, DBA_USERS, 12.1.0.2 column footnote:
  https://docs.oracle.com/database/121/REFRN/GUID-309FCCB2-2E8D-4371-9FC5-7F3B10E2A8C0.htm
- Oracle Database Reference 19c and 26ai, ALL_TAB_STATISTICS:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TAB_STATISTICS.html
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_TAB_STATISTICS.html
- Oracle Database SQL Tuning Guide 26ai, Optimizer Statistics Concepts,
  session-specific GTT statistics from 12c:
  https://docs.oracle.com/en/database/oracle/oracle-database/26/tgsql/optimizer-statistics-concepts.html
- Oracle Database SQL Language Reference 11.2, ROW_NUMBER (E26088-02), nested
  top-N and deterministic ordering:
  https://docs.oracle.com/html/E26088_02/functions156.htm

Number/calendar conversion references and their exact tested native limits
remain in object-row-counts.md. This additive selector requires separate native
qualification of its own full query, grants and admitted release/container.
