# Logical database identity

Use `identity.LogicalDatabase` on the admitted connection when the database
control-file DBID and name are needed. Supply the observed numeric version in
`LogicalDatabaseOptions`. Pre-12c queries expose `DBID, NAME`. From 12.1,
the query also retains the raw `V$DATABASE.CDB` flag. The typed `CDB`
pointer is absent before 12.1. A selected SQL NULL remains a nonnil value
with `Null` set, or JSON `null`.

The query reads one `V$DATABASE` row from the control file. In a CDB, the
logical DBID and name describe the parent database, including when queried
from an admitted PDB session. A container's own DBID, GUID and name come from
[container inventory](container-inventory.md) on the intended handle.
Applications decide which identity to use and whether a row is sufficient.
The library performs no tenant switching, host or service substitution,
version probing or resume-key construction.

`DBID` is formatted as decimal text by Oracle with `TM9` and fixed numeric
characters. It is decoded as unsigned exact text and encoded as a JSON
string. Raw name text and SQL NULLs are preserved. A successful empty selection
has a nonnil empty row slice. Invalid projections and more than one row fail
without returning partial facts. An unsupported numeric release fails before
SQL. Read and JSON establish transport completion for their selected source.
Pure Decode establishes shape and value validity. Rows is the original
caller-owned cursor. Privilege rejection and unavailable-source diagnostics
are recognizable through `ErrLogicalDatabaseInaccessible` and
`ErrLogicalDatabaseUnavailable`. Oracle diagnostics and combined errors retain
their original causes. [9.2 format models](https://docs.oracle.com/cd/B10500_01/server.920/a96540/sql_elements4a.htm) · [ORA-01507](https://docs.oracle.com/en/error-help/db/ora-01507/)

## Release and qualification matrix

Oracle references checked 2026-11-21. Native SQL and driver qualification
refer to these exact projections. The numeric 23 series includes 26ai.

| Oracle release | Oracle source and selected columns | Static selection and decoding | Native SQL | Driver/API |
| --- | --- | --- | --- | --- |
| 9iR2 9.2.0.8.0 | [9.2 Reference](https://docs.oracle.com/cd/B10500_01/server.920/a96536/ch338.htm), DBID and NAME | Pass | Unexecuted | Unqualified |
| 10gR1 10.1.0.5.0 | [10.1 Reference](https://docs.oracle.com/cd/B14117_01/server.101/b10755/dynviews_1054.htm), DBID and NAME | Pass | Unexecuted | Unqualified |
| 10gR2 10.2.0.5.0 | [10.2 Reference](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1073.htm), DBID and NAME | Pass | Unexecuted | Unqualified |
| 11gR1 11.1.0.7.0 | [11.1 Reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_1086.htm), DBID and NAME | Pass | Unexecuted | Unqualified |
| 11gR2 11.2.0.4.0 | [11.2 Reference](https://docs.oracle.com/cd/E18283_01/server.112/e17110/dynviews_1096.htm), DBID and NAME | Pass | Unexecuted | Unqualified |
| 12cR1 12.1.0.1.0 and 12.1.0.2.0 | [12.1 Reference](https://docs.oracle.com/database/121/REFRN/GUID-C62A7B96-2DD4-4E70-A0D9-26EE4BFBE256.htm), adds CDB | Pass | Unexecuted | Unqualified |
| 12cR2 12.2.0.1.0 | [12.2 Reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-DATABASE.html), same projection | Pass | Unexecuted | Unqualified |
| 18c 18.0.0.0.0 | [18c Reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-DATABASE.html), same projection | Pass | Unexecuted | Unqualified |
| 19c 19.32.0.0.0 | [19c Reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE.html), same projection | Pass | Unexecuted | Unqualified |
| 21c 21.3.0.0.0 | [21c Reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-DATABASE.html), same projection | Pass | Unexecuted | Unqualified |
| 23c/23ai 23.2.0.0.0 and 23.9.0.0.0 | [Archived 23 reference URL](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-DATABASE.html) redirects to 26ai. Same known column lineage. | Pass | Unexecuted | Unqualified |
| 26ai 23.26.1.0.0 and 23.26.4.0.0 | [26ai Reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE.html) and [RU reference changes](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/changes-this-release-oracle-database-reference.html), same projection | Pass | 23.26.1 root and PDB | Rows and bounded JSON via go-ora/v2 v2.9.0 |
| Other numeric families | Typed unsupported | No query | Not applicable | Not applicable |

On Oracle 26ai 23.26.1.0.0, the existing READ WRITE primary returned exactly
one logical database row on both CDB$ROOT and OMLAB as SYSTEM without SYSDBA.
The two connections agreed on the logical DBID/name and raw CDB flag. The
current OMLAB DBID from inventory was different from the logical DBID.
The probe used Windows amd64, Go 1.27.4, go-oracle-session v0.15.0 and
go-ora/v2 v2.9.0. Fixtures cover the buffered Reader path and exact version
selection. Exact historical, non-CDB, MOUNT, non-privileged login and RAC
observations remain unqualified. The selected view needs access to the
database control-file facts under the admitted account and state.

