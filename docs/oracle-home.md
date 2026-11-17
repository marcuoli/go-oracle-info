# Server ORACLE_HOME

Use Client.OracleHome(ctx) for a typed executing-instance observation and
Client.OracleHomeJSON(ctx, limits...) for its independently collected bounded
JSON counterpart. Both retain the selected source, inference flag and every
fallback attempt. identity.ReadOracleHome(ctx, reader, request) uses an existing
admitted complete-result reader without opening or moving connections.

## Fallback order

1. Oracle 12c+: documented SYS_CONTEXT('USERENV','ORACLE_HOME').
2. DBMS_SYSTEM.GET_ENV with native OUT binds, when the admitted reader supports
   the existing session Exec contract. Instance identity is read in the same
   fixed anonymous block. No DBMS_OUTPUT state or text dataset is introduced.
3. SYS.DBA_LIBRARIES, only the SYS-owned DBMS_SUMADV_LIB object. Its absolute
   Unix /lib/ parent is an explicitly inferred library-path candidate.

9i-11g skip the unsupported context attribute. Oracle 9.2.0.8 also lacks GET_ENV;
its ORA-06550 source error advances to the library candidate. A NULL/empty/no-row source or a
qualified source-unavailable Oracle error may advance the chain. Missing OUT
execution capability is retained as executor_unavailable. Cancellation,
deadlines, retired/bad connections, malformed evidence, capacity errors and
unknown/cleanup failures stop it. Every terminal cause must allow continuation;
one Oracle code cannot mask another failure. OUT values from failed executions
are discarded. Public errors/JSON never expose driver messages.

SYS_CONTEXT avoids the privileged package/dictionary dependencies when it
returns a value. GET_ENV requires executable access to SYS.DBMS_SYSTEM; no grant
is attempted. The library source requires dictionary visibility and may refer to
an old installation after migration. Symbolic, Windows or unrecognized library
paths yield NULL candidates while the original FILE_SPEC is retained. No source
proves a file exists, its binary version or all-member RAC coverage. Query each
separately admitted member when that coverage is required; never copy local
USERENV values onto GV$INSTANCE rows.

## Complete native datasets

Client.OracleHomeRows(ctx) and Client.OracleHomeRowsJSON(ctx, limits...) expose
the matching release-selected SQL dataset without procedural fallback. Their
functional counterparts are identity.OracleHomeRows and OracleHomeRowsJSON.
Rows is the actual native *sql.Rows: callers own Next/Scan/Err/Close and budgets.
A procedural OUT-bind result cannot be fabricated into a native cursor.

OracleHomeStatement/DecodeOracleHome retain the direct dataset selection;
DecodeOracleHomeSource validates an explicitly selected context/environment/
library observation, including source/release and raw/candidate coherence.
JSON datasets use complete rowset.Read, projection.FromRowset and the same
domain decoder. NULL, available empty text and successful no rows stay distinct.
Any terminal, close, cancellation, domain or budget failure publishes no prefix.

## Oracle references and qualification

Checked 2026-11-17:

- [12.1 SYS_CONTEXT](https://docs.oracle.com/database/121/SQLRF/functions199.htm)
  and [12.2 SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/SYS_CONTEXT.html)
  list ORACLE_HOME among USERENV attributes.
- [26ai SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/SYS_CONTEXT.html)
  retains the attribute.
- [11.2 SYS_CONTEXT](https://docs.oracle.com/cd/E11882_01/server.112/e41084/functions184.htm)
  does not list it.

Linux native SQL probes passed on 12.2.0.1 Enterprise, 19c Enterprise and 26ai
Enterprise; context homes matched separate GET_ENV calls. On 11.2.0.2 Express,
context returned ORA-02003; the supplied library query and GET_ENV agreed.
The consumer also exercised GET_ENV via the actual session/driver OUT-bind path
on 11.2.0.2. Library agreement qualifies that fixture only; it does not establish
runtime reliability of arbitrary dictionary paths. 12.1, Windows, RAC, ADB and
other 11g releases are not natively qualified by this change.

Additional Linux native SQL probes on 2026-11-17 covered 9.2.0.8 and 10.2.0.1 XE.
Both reject the context attribute with ORA-02003. On 9i, GET_ENV is absent
(ORA-06550 / PLS-00302) and the SYS.DBMS_SUMADV_LIB path yields a candidate.
On 10g XE, GET_ENV and the library parent agree. The normal 10g reader also
passed real Go functional/scalar/JSON/native Rows/RowsJSON parity; its GET_ENV
privilege failure correctly selected the inferred library candidate. The existing
SYS TCP credential returned ORA-01017 and was not changed; its GET_ENV source
was checked through SQL*Plus OS authentication. On 9i, SQL*Plus authenticated
the existing reader over TCP while the Go connection path returned ORA-01017.
The 9i API/native-driver qualification remains open; SQL source availability
and unit decoder tests are not a successful native Go read.
The ORACLE_HOME-specific release floor is 9; this does not lower other fact
selectors' version requirements or qualify a whole application for 9i/10g.
