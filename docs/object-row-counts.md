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

## Partition and subpartition row counts

`objects.SubobjectStatisticsStatement`, `DecodeSubobjectStatistics`,
`ReadSubobjectStatistics`, `SubobjectStatisticsRows` and
`SubobjectStatisticsJSON` share one exact-owner projection from
`DBA_TAB_STATISTICS`. The binary owner bind at `:1` selects
`OBJECT_TYPE` values `PARTITION` and `SUBPARTITION`. Each row retains
`TABLE_NAME`, `PARTITION_NAME`, `SUBPARTITION_NAME`, `ROW_COUNT` (from
`NUM_ROWS`) and `LAST_ANALYZED`. Partition rows have a NULL subpartition.
Subpartition rows require the actual parent name captured in the source. Names,
zero, fractional estimates, large values and SQL NULL stay distinct. The
caller intersects these rows with the admitted ordinary table inventory from
`DBA_TABLES` and decides how to handle ambiguous, duplicate or missing evidence.

The statistics query uses no fixed row cap. Buffered and JSON paths require
complete bounded transport and return an error on budget exhaustion. The
borrowed native cursor leaves iteration and budgets to the caller. A completed
empty dictionary read describes the selected scope under the admitted login.
Optimizer estimates and timestamps have the same exact text and Gregorian
database-local formatting as table-level statistics.

`objects.CountSubobjectStatement` generates a separately executed read-only
`COUNT(*)` for the named partition or subpartition. It requires a valid
partition identifier for both forms and uses the child name for the Oracle
`SUBPARTITION` extension. Subpartition names are unique within a table. The
caller verifies that the child belongs to the admitted parent, that the
connected login can read the table, and that the workload and snapshot are
suitable. Counting a composite partition includes its children. Treat
partition/subpartition counts as different levels of detail to avoid double
counting. Oracle documents a statement-level `PARALLEL(n)` hint starting in
11g Release 2. The shared builder emits the optional hint unchanged on request.
The SELECT and count remain independent of hint acceptance.

## Partition and subpartition compatibility

Oracle references reviewed 2026-11-24 (America/Sao_Paulo). **Stats** means the
exact five-column projection and binary owner bind above. **Count** means the
same quoted partition and subpartition SELECT extensions for each admitted
family. The count builder is a syntax helper with no version argument. Caller
admission controls execution. No native SQL or driver/application probe is
recorded here for this new pair of operations. Earlier table-level probes above
are separate evidence.

| Oracle release | Dictionary reference and selected SQL | Static selection/decoding | Native SQL | Driver/application |
| --- | --- | --- | --- | --- |
| 9iR2 (9.2) | Stats selector rejects numeric 9 without SQL. Count is not selected by this statistics contract. | Unsupported, no reader call | Unexecuted | Unqualified |
| 10gR1 (10.1) | [Oracle 10.1 reference][stats101]. Stats and Count. | Checked at major 10 | Unexecuted | Unqualified |
| 10gR2 (10.2) | [Oracle 10.2 reference][stats102]. Stats and Count. | Checked at major 10 | Unexecuted | Unqualified |
| 11gR1 (11.1) | [Oracle 11.1 reference][stats111]. Stats and Count. | Checked at major 11 | Unexecuted | Unqualified |
| 11gR2 (11.2) | [Oracle 11.2 reference][stats112]. Stats and Count. Statement-level parallel hint documented from this release. | Checked at major 11 | Unexecuted | Unqualified |
| 12cR1 (12.1) | [Oracle 12.1 reference][stats121]. Stats and Count. | Checked at major 12 | Unexecuted | Unqualified |
| 12cR2 (12.2) | [Oracle 12.2 reference][stats122]. Stats and Count. Widened identifiers depend on `COMPATIBLE`. | Checked at major 12 | Unexecuted | Unqualified |
| 18c (18) | [Oracle 18c reference][stats18]. Stats and Count. | Checked at major 18 | Unexecuted | Unqualified |
| 19c (19) | [Oracle 19c reference][stats19]. Stats and Count. | Checked at major 19 | Unexecuted | Unqualified |
| 21c (21) | [Oracle 21c reference][stats21]. Stats and Count. | Checked at major 21 | Unexecuted | Unqualified |
| 23c/23ai (23) | Oracle's [23 database reference URL][stats23] redirects to the current 26ai page. Stats and Count. A separately archived 23 release reference was not verified. | Checked at major 23 | Unexecuted | Unqualified |
| 26ai (23.26 release line) | [Oracle 26ai reference][stats26]. Stats and Count. Numeric 23 selects the reviewed projection. Numeric 26 is rejected. | Checked at major 23 and numeric 26 rejection | Unexecuted | Unqualified |

The selected fields require no patch-set/RU predicate. Open non-CDB/CDB
root/PDB scope is the caller's already connected dictionary. MOUNT and
cross-container collection are outside this contract. Native qualification
must record the actual numeric version/RU, edition, container and open mode,
and independently verify dictionary and table access under the intended
login. The [legacy SELECT syntax][count102] and [current SELECT syntax][count26]
document both named extensions. See the [parallel hint boundary][parallel],
[identifier limits][identifiers] and [subpartition namespace][subpartition].

[stats101]: https://docs.oracle.com/cd/B13789_01/server.101/b10755/statviews_1188.htm
[stats102]: https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_2102.htm
[stats111]: https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_2102.htm
[stats112]: https://docs.oracle.com/cd/E18283_01/server.112/e17110/statviews_2114.htm
[stats121]: https://docs.oracle.com/database/121/REFRN/GUID-C567F729-3748-4DFE-A22C-115CB7575D63.htm
[stats122]: https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/ALL_TAB_STATISTICS.html
[stats18]: https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/ALL_TAB_STATISTICS.html
[stats19]: https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TAB_STATISTICS.html
[stats21]: https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_TAB_STATISTICS.html
[stats23]: https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/ALL_TAB_STATISTICS.html
[stats26]: https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_TAB_STATISTICS.html
[count102]: https://docs.oracle.com/cd/B19306_01/server.102/b14200/statements_10002.htm
[count26]: https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/img_text/partition_extension_clause.html
[parallel]: https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/Comments.html
[identifiers]: https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/Database-Object-Names-and-Qualifiers.html
[subpartition]: https://docs.oracle.com/en/error-help/db/ora-14159/
