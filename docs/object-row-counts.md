# Table row counts and optimizer statistics

objects.CountStatement preserves its original exact decimal COUNT(*) query.
CountStatementWithOptions accepts CountOptions{ParallelDegree: n}:
zero leaves the SQL unchanged; 1 through MaxParallelDegree (1024) requests
the PARALLEL(n) hint. This API bound is not an Oracle capacity limit.
Hints are advisory; Oracle can ignore them. A hint does not prove actual DOP,
make a count consistent with another snapshot, or promise faster execution.
Callers retain concurrency, workload, authorization and timeout policy.

TablesStatisticsSQL binds owner at :1; TableStatisticsSQL also binds table at
:2. Both exclude temporary, nested, secondary and IOT overflow tables. They read
NUM_ROWS and LAST_ANALYZED from DBA_TABLES without gathering statistics.
NUM_ROWS is recorded optimizer statistics, not a fresh exact count. SQL NULL
remains NULL: never turn absent statistics into zero. Large numbers retain exact
decimal text. LAST_ANALYZED is formatted with a Gregorian calendar and seconds,
without inventing a timezone offset; freshness and scoring remain caller-owned.

## Oracle references and qualification

Reviewed 2026-11-13 (America/Sao_Paulo):

- [Oracle Database 19c ALL_TABLES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TABLES.html): NUM_ROWS, SAMPLE_SIZE, LAST_ANALYZED, table-type columns and statistics-populated fields.
- [Oracle Database 19c SQL Language Reference, Comments/PARALLEL](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/Comments.html): statement hints, integer degree and optimizer applicability.
- [Oracle Database 19c Globalization Support Guide, Table 9-1](https://docs.oracle.com/en/database/oracle/oracle-database/19/nlspg/sql-and-plsql-programming-in-global-environment.html): NLS_CALENDAR is accepted by TO_CHAR; the expression pins GREGORIAN without changing the session.
- The corresponding [11gR2 parameter table](https://docs.oracle.com/cd/E11882_01/server.112/e10729/ch9sql.htm) and [26ai parameter table](https://docs.oracle.com/en/database/oracle/oracle-database/26/nlspg/sql-and-plsql-programming-in-global-environment.html) document the same NLS parameter.

Focused tests cover unchanged default bytes, quoting, rejected names, numeric
degree bounds, bind positions, ordinary-table filters, NULL preservation and the
Gregorian contract. Full native selector and consumer observations are recorded
in the owning validation evidence; no general database-version/platform
qualification follows from the SQL contract tests alone.

Native observations: Windows Go 1.27.4 clients collected dictionary statistics
from Linux Oracle 11.2.0.2 XE, 19.32.0.0.0 Enterprise PDB and 26ai Free fixture
23.26.3.0.0. Existing example tables had NULL row statistics, preserved as
unavailable rather than zero. Read-only Gregorian/Buddhist TO_CHAR probes passed
on all three releases. Unhinted and PARALLEL(2) exact counts agreed on the tested
19c/26ai pair; actual database DOP and performance improvement were not measured.
These observations do not qualify arbitrary releases, platforms or privileges.
