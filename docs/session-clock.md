# Connected session clock

The [session clock API](api-reference.md#database) samples six Oracle expressions
with one read-only SELECT on an already admitted connection. It uses SYS.DUAL and
retains the numeric release and connected-session scope in the result provenance.
The application owns connection affinity, permissions, timeouts, elapsed
measurements and clock comparison.

| Column | Oracle value | Exact rendering |
| --- | --- | --- |
| SYSDATE | Civil system DATE | YYYY-MM-DDTHH:mm:ss |
| SYSTIMESTAMP_UTC | System timestamp with time zone | YYYY-MM-DDTHH:mm:ss.fffffffffZ |
| CURRENT_DATE | Civil DATE in the session time zone | YYYY-MM-DDTHH:mm:ss |
| CURRENT_TIMESTAMP_UTC | Current timestamp with time zone in the session | YYYY-MM-DDTHH:mm:ss.fffffffffZ |
| LOCALTIMESTAMP | Civil timestamp in the session time zone | YYYY-MM-DDTHH:mm:ss.fffffffff |
| SESSION_TIME_ZONE | SESSIONTIMEZONE | Oracle offset or region text |

Use SessionClockOptions with the observed numeric Oracle version. The buffered
ReadSessionClock reader issues one SELECT and preserves SQL NULL and empty
results separately. It rejects an unexpected projection, multiple rows,
malformed values, cancellation or an incomplete read without returning a
successful prefix. Pure DecodeSessionClock leaves provenance completeness unset.
The SessionClockRows cursor belongs to its caller. SessionClockJSON completes
and bounds a separate read. Client.SessionClock and Client.SessionClockJSON can
discover the connected release before choosing the projection.

Oracle SYS_EXTRACT_UTC converts the two zoned expressions before rendering.
Gregorian EXTRACT fields and explicit decimal formatting keep the output
independent of the session's calendar and date/time formats. Nine fractional
positions preserve Oracle precision, including trailing zeros. The civil
readings carry no implicit time zone or offset. On 26ai, the system expressions
retain Oracle's documented PDB clock semantics when TIME_AT_DBTIMEZONE applies.
Existing redo.Clock and database.TimeZone retain their independent contracts.
[Oracle datetime and time-zone semantics](https://docs.oracle.com/en/database/oracle/oracle-database/26/nlspg/datetime-data-types-and-time-zone-support.html)

## Release and qualification matrix

Each admitted release uses the same six-column SELECT. Static selection admits
numeric 9.2, 10.1/10.2, 11.1/11.2, 12.1/12.2, 18, 19, 21 and 23, including the
numeric release line used by 26ai. The 9.2 floor defines supported admission.
Other numeric families and branding strings are rejected before SQL. Static
fixtures cover the listed release families and both the exact text and
bounded-consumption contract.

| Release | Oracle SQL reference | Static selection and decoding | Native qualification |
| --- | --- | --- | --- |
| 9.2 | [Datetime functions](https://docs.oracle.com/cd/B10501_01/server.920/a96540/functions2a.htm), [Gregorian EXTRACT](https://docs.oracle.com/cd/B10501_01/server.920/a96540/functions42a.htm), [UTC conversion](https://docs.oracle.com/cd/B10500_01/server.920/a96540/functions124a.htm) | Pass | Unexecuted |
| 10.1 | [Datetime functions](https://docs.oracle.com/cd/B14117_01/server.101/b10759/functions001.htm) | Pass | Unexecuted |
| 10.2 | [Datetime functions](https://docs.oracle.com/cd/B19306_01/server.102/b14200/functions001.htm) | Pass | Unexecuted |
| 11.1 | [Datetime functions](https://docs.oracle.com/cd/B28359_01/server.111/b28286/functions001.htm) | Pass | Unexecuted |
| 11.2 | [Datetime functions](https://docs.oracle.com/html/E10592_02/functions002.htm) | Pass | Unexecuted |
| 12.1.0.1 and 12.1.0.2 | [Datetime functions](https://docs.oracle.com/database/121/SQLRF/functions002.htm) | Pass | Unexecuted |
| 12.2.0.1 | [Datetime functions](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/Single-Row-Functions.html) | Pass | Unexecuted |
| 18c | [Datetime functions](https://docs.oracle.com/en/database/oracle/oracle-database/18/sqlrf/Single-Row-Functions.html) | Pass | Unexecuted |
| 19c | [Datetime functions](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/Single-Row-Functions.html) | Pass | 19.32.0.0.0 open primary PDB, SYSTEM, typed reader through go-ora V2 |
| 21c | [Datetime functions](https://docs.oracle.com/en/database/oracle/oracle-database/21/sqlrf/Single-Row-Functions.html) | Pass | Unexecuted |
| 23c/23ai | [Oracle 23 SQL reference URL](https://docs.oracle.com/en/database/oracle/oracle-database/23/sqlrf/Single-Row-Functions.html) redirects to 26ai | Pass | Unexecuted |
| 26ai | [Datetime functions](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/Single-Row-Functions.html) | Pass, including selector for 23.26.4.0.0 | 23.26.1.0.0 open primary PDB, SYSTEM, typed reader through go-ora V2 |

On 2026-11-25, both native PDBs returned the six columns and a complete row
through go-oracle-session v0.25.1 with Go 1.27.4. A separate read-only SQL
control converted +05:45 across a date boundary with all nine fractional
digits and preserved SQL NULL. Native rows and JSON contracts have fixture
coverage. Root, MOUNT, standby, ordinary-account operation and other numeric
versions remain unqualified for this fact. The SQL itself selects from
SYS.DUAL under the caller's privileges and has no role or container switch.
