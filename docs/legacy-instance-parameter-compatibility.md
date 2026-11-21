# Version-aware instance visibility and scalar system parameters

Issue #83 adds two read-only fact families for caller-admitted handles.
Pass the database's observed numeric version and a caller-owned SQL reference
for buffered reads. The selectors accept two to five numeric components and
require the minor component to identify legacy release families. The supported
numeric families are 9.2, 10.1, 10.2, 11.1, 11.2, 12.1, 12.2, 18, 19, 21 and 23.
Oracle AI Database 26ai uses the numeric 23 family.

- `instances.VisibilityStatement`, `DecodeVisibility`, `ReadVisibility`,
  `VisibilityRows` and `VisibilityJSON` use `instances.VisibilityOptions{OracleVersion: observed}`.
  The buffered `VisibilityRequest` adds the SQL reference.
- `parameters.ScalarSystemStatement`, `DecodeScalarSystem`, `ReadScalarSystem`,
  `ScalarSystemRows` and `ScalarSystemJSON` use `parameters.ScalarSystemOptions`.
  Options include `OracleVersion`, `Filter` and `Scope`. The buffered
  `ScalarSystemRequest` adds the SQL reference.

`Visibility` observes qualified rows from `GV$INSTANCE`. Preserve the reporting
`INST_ID`, instance number, name, host, status, database status and parallel
flag. Keep `BLOCKED` and `CON_ID` when projected. Mount and service-blocking
statuses are raw evidence. This query does not enumerate configured RAC
membership or establish reachability.

`ScalarSystem` observes instance-wide rows from `GV$SYSTEM_PARAMETER`. Keep
`VALUE` and `DISPLAY_VALUE` independent, including SQL NULL. The display
column is absent on 9.2. Embedded commas, whitespace, units, flags and case
belong to the source text. Consumer comparison can choose a display value
and normalize or sort its output after collection.

Both APIs retain observed version and source scope in provenance. In typed
results, a nil optional pointer means an unprojected column. A nonnil value
with `Null=true` means Oracle returned SQL NULL. JSON exposes only selected
aliases and places JSON null at the projected NULL position. Duplicate rows
are preserved. `Complete` marks successful buffered or JSON transport for the
selected source. Pure decoding leaves it false.

## Scope and diagnostics

Before 12c, the selected rows have non-CDB scope and no `CON_ID`. From 12.1,
visibility includes admitted whole-database and current-container observations,
selected by `CON_ID IN (0, TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))`.
`ScalarSystemCurrentAndDatabase` (zero value) uses that same policy.
`ScalarSystemDatabaseOnly` selects exactly `CON_ID = 0` on 12c+ and the
same non-CDB projection as the default scope on earlier releases.
A PDB login executing the database-only query can receive an empty result.
In a root session, grants and `CONTAINER_DATA` still govern the rows returned.
In both cases, zero denotes the documented whole-CDB/non-CDB row identity.
Keep source ID and reporting instance separately. [Dynamic performance views](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/about-dynamic-performance-views.html)
and [SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/SYS_CONTEXT.html).

The caller admits effective access to the two selected `GV$` sources.
For a least-privilege qualification, record the actual account/role and
source visibility in its own non-CDB/root/PDB session. A SYS observation
does not qualify another login. Track OPEN, MOUNT and any STARTED/NOMOUNT
attempts independently. The 9.2 Reference also documents a positive
`PARALLEL_MAX_SERVERS` prerequisite on instances mounting a database queried
through its global views. The SDK does not change that setting, grants,
state, containers or destinations. [9.2 dynamic performance views](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch3.htm).

Unsupported numeric versions return `ErrUnsupportedVersion` before SQL.
A successful empty result has state `no_rows` and an empty nonnil row slice.
`ORA-01031` gives `inaccessible` and `ErrSourceInaccessible`. `ORA-00942`,
`ORA-01109` and `ORA-01219` give `unavailable` and `ErrSourceUnavailable`
when the query reports that single diagnostic. An unresolved view does not
prove a missing grant. Other query, cancellation, decode and combined
failures remain failed. The original causes are available for private
inspection through the error chain. There is no fallback query. Native Rows
returns the original cursor with caller-owned iteration and close. JSON
consumes a separate cursor and returns nil on any error. [ORA-01031](https://docs.oracle.com/en/error-help/db/ora-01031/),
[ORA-00942](https://docs.oracle.com/en/error-help/db/ora-00942/),
[ORA-01109](https://docs.oracle.com/en/error-help/db/ora-01109/),
[ORA-01219](https://docs.oracle.com/en/error-help/db/ora-01219/).

## Release matrix

Oracle documentation checked 2026-11-21. The table identifies release-specific
column choices for each query. Numeric versions are static test inputs, not
observed server versions for these APIs. The same static tests qualify both
source selection and decoding; the two native qualification columns remain
separate. Oracle 9.2 documents `NLSSORT` for binary name matching and sorting.
[9.2 SQL Reference](https://docs.oracle.com/cd/B10500_01/server.920/a96540.pdf)
and [9.2 globalization guide](https://docs.oracle.com/cd/B10501_01/server.920/a96529/ch7.htm).

| Oracle release and static input | `GV$INSTANCE` projection and source | `GV$SYSTEM_PARAMETER` projection and source | Static SQL and decoder | Native SQL for this family/layout | Oracle driver/application |
| --- | --- | --- | --- | --- | --- |
| 9iR2 9.2.0.8.0 | Base instance fields, no `BLOCKED` or `CON_ID`. [9.2 reference](https://docs.oracle.com/cd/B10500_01/server.920/a96536/ch383.htm) | Base scalar fields, no `DISPLAY_VALUE`, instance-modifiable, deprecated, basic, PDB or container fields. [9.2 reference](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch3218.htm) | Pass | Unexecuted | Unqualified |
| 10gR1 10.1.0.5.0 | 9.2 instance projection. [10.1 reference](https://docs.oracle.com/cd/B12037_01/server.101/b10755/dynviews_1108.htm) | Adds `DISPLAY_VALUE`, `ISINSTANCE_MODIFIABLE` and `ISDEPRECATED`. [10.1 reference](https://docs.oracle.com/cd/B12037_01/server.101/b10755/dynviews_2132.htm) | Pass | Unexecuted | Unqualified |
| 10gR2 10.2.0.5.0 | 9.2 instance projection. [10.2 reference](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1131.htm) | 10.1 scalar projection. [10.2 reference](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_2153.htm) | Pass | Unexecuted | Unqualified |
| 11gR1 11.1.0.7.0 | Adds `BLOCKED`. [11.1 reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_2002.htm) | Adds `ISBASIC`. [11.1 reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_3090.htm) | Pass | Unexecuted | Unqualified |
| 11gR2 11.2.0.4.0 | 11.1 projection. [11.2 reference](https://docs.oracle.com/cd/E18283_01/server.112/e17110/dynviews_2002.htm) | 11.1 projection. [11.2 reference](https://docs.oracle.com/cd/E18283_01/server.112/e17110/dynviews_3097.htm) | Pass | Unexecuted | Unqualified |
| 12cR1 12.1.0.2.0 | Adds `CON_ID`. [12.1 reference](https://docs.oracle.com/database/121/REFRN/GUID-6A0C9B51-1714-4223-B166-9D54C4E65D67.htm) | Adds `CON_ID` and `ISPDB_MODIFIABLE`. [12.1 reference](https://docs.oracle.com/database/121/REFRN/GUID-2F2801AE-44CB-4C86-ADBE-91CFBC0B8407.htm) | Pass | Unexecuted | Unqualified |
| 12cR2 12.2.0.1.0 | 12.1 projection. [12.2 reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-INSTANCE.html) | 12.1 projection. [12.2 reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-SYSTEM_PARAMETER.html) | Pass | Unexecuted | Unqualified |
| 18c 18.0.0.0.0 | 12.1 projection. [18 reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-INSTANCE.html) | 12.1 projection. [18 reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-SYSTEM_PARAMETER.html) | Pass | Unexecuted | Unqualified |
| 19c 19.32.0.0.0 | 12.1 projection. [19 reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html) | 12.1 projection. [19 reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER.html) | Pass | Unexecuted | Unqualified |
| 21c 21.3.0.0.0 | 12.1 projection. [21 reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-INSTANCE.html) | 12.1 projection. [21 reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-SYSTEM_PARAMETER.html) | Pass | Unexecuted | Unqualified |
| 23c/23ai 23.2.0.0.0, 23.9.0.0.0 | 12.1 projection inferred from the current Oracle reference. [23 reference redirects to 26ai](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-INSTANCE.html) | 12.1 projection inferred from the same source lineage. [23 reference redirects to 26ai](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-SYSTEM_PARAMETER.html) | Pass | Unexecuted | Unqualified |
| 26ai 23.26.1.0.0, 23.26.4.0.0 | 12.1 projection. [26 reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html) | 12.1 projection. [26 reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER.html) | Pass | 23.26.1.0.0 root and PDB | Both Rows and JSON via go-ora v2.9.0 |
| Outside reviewed numeric release families | Unsupported without execution | Unsupported without execution | Pass | Not applicable | Not applicable |

The current 26ai release references were checked through October 2026 RU
23.26.4. The revision history identifies no change to these two projections.
Exact historical references for 23c and early 23ai redirected to the current
manual, so patch-specific applicability remains open. [Numeric release scheme](https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/oracle-database-release-numbers.html),
[RU 23.26.4](https://docs.oracle.com/en/database/oracle/oracle-database/26/nfcoa/ru23_26_4.html)
and [database reference change history](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/changes-this-release-oracle-database-reference.html).

The tests exercise complete buffered transport and database/sql fixtures,
including raw text, SQL NULL, unprojected columns, terminal failures, cursor
ownership, exact SQL and bind contracts, and atomic JSON. Both exact selectors completed native Rows and bounded JSON on 23.26.1.0.0
as SYSTEM without SYSDBA on the approved READ WRITE primary. CDB$ROOT had
one visibility row, 557 whole-database scalar rows and 557 current/database
rows. OMLAB had one visibility row, 510 whole-database scalar rows and 557
current/database rows. The harness used Windows amd64, Go 1.27.4,
go-oracle-session v0.15.0 and go-ora/v2 v2.9.0. The earlier 9.2 driver
admission, original 12.2 application collectors and 19c parity receipts
used different SQL. The existing 12.2 PDB login failed before query
execution and no approved 12.2 root profile was available. Legacy exact
SQL and driver execution, other modern releases, MOUNT, other account
privileges and RAC coverage remain unqualified.
