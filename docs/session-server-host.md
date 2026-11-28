# Executing-session server hostname

Issue #236 exposes the hostname reported by `SYS_CONTEXT('USERENV','SERVER_HOST')`
and the same session's `INSTANCE` and `INSTANCE_NAME` attributes in one
statement from `DUAL`. The three column aliases are `SERVER_HOST`,
`INSTANCE_NUMBER`, and `INSTANCE_NAME`. Oracle reports each attribute as
text. The decoder validates an available instance number as unsigned integer
text and preserves spelling and SQL NULL. An empty successful result remains
distinct from a row of SQL NULLs and from a failed read.

`identity.ReadSessionServerHost` accepts an observed numeric version and a
caller-admitted complete reader. `Client.SessionServerHost` returns the native
cursor on the caller's handle. `Client.SessionServerHostJSON` reads a separate
bounded cursor. The convenience facade reads the database version through its
existing `V$INSTANCE` bootstrap. Callers with independent release evidence
can use the options-based fact APIs, whose source query needs only `DUAL`.
The source and scope are `SYS_CONTEXT(USERENV.SERVER_HOST)`
and `executing_session_server_host`.

The server context and the current client-host fact describe different ends
of a connection. Existing session context uses `USERENV.HOST` for the client.
The original `V$INSTANCE` and `GV$INSTANCE` values carry their own provenance.
Host labels from the context and the instance views can differ in spelling.
An application can associate a context hostname with a uniquely matching
instance number and name collected on the same admitted session.
Preserve the native row and its SQL NULLs. A managed service can report a
session hostname label without exposing its physical infrastructure.

## Release and privilege scope

The selector uses the documented attributes supported from 10.2. Oracle 9.2
and 10.1 document the instance number but omit the server-host and instance-name
attributes used by the full projection. The library rejects those families
before issuing the fact query. Only reviewed numeric families are admitted.
Oracle AI Database 26ai continues to use its observed numeric 23 release.

Oracle references checked on 2026-11-27.

| Numeric family | Oracle contract and static selection | Native qualification |
| --- | --- | --- |
| 9.2, 10.1 | No execution for this fact. [9.2](https://docs.oracle.com/cd/A97385_01/server.920/a96540/functions122a.htm) [10.1](https://docs.oracle.com/cd/B13789_01/server.101/b10759/functions150.htm) | Outside selection. |
| 10.2 | Exact three-attribute projection. 10.2.0.1.0 and 10.2.0.5.0 selector cases. [10.2](https://docs.oracle.com/cd/B19306_01/server.102/b14200/functions165.htm) | Unexecuted. |
| 11.1, 11.2 | Same projection. 11.1.0.7.0 and 11.2.0.4.0 selector cases. [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28286/functions172.htm) [11.2](https://docs.oracle.com/html/E26088_02/functions184.htm) | Unexecuted. |
| 12.1, 12.2 | Same projection. 12.1.0.1.0, 12.1.0.2.0, and 12.2.0.1.0 selector cases. [12.1](https://docs.oracle.com/database/121/SQLRF/functions199.htm) [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/SYS_CONTEXT.html) | Unexecuted. |
| 18 | Same projection. 18.4.0.0.0 selector case. [18](https://docs.oracle.com/en/database/oracle/oracle-database/18/sqlrf/SYS_CONTEXT.html) | Unexecuted. |
| 19 | Same projection. 19.3.0.0.0 and 19.33.0.1.0 selector cases. [19](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/SYS_CONTEXT.html) | OPEN RAC PDB on 19.3.0.0.0 through TCP as SYSDBA, on two member instances. OPEN Autonomous PDB on 19.33.0.1.0 through configured TCPS without SYSDBA. Oracle JDBC Thin 23.26.2.0.0 verified the three attributes and the same-session native instance comparisons. The released go-oracle-session v0.25.1 V2 path verified the owned reader and the native rows and JSON facade on both scopes. |
| 21 | Same projection. 21.3.0.0.0 selector case. [21](https://docs.oracle.com/en/database/oracle/oracle-database/21/sqlrf/SYS_CONTEXT.html) | Unexecuted. |
| 23c, 23ai, 26ai | Same projection. 23.2.0.0.0, 23.9.0.25.07, and 23.26.4.0.0 selector cases. [23](https://docs.oracle.com/en/database/oracle/oracle-database/23/sqlrf/SYS_CONTEXT.html) [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/SYS_CONTEXT.html) | Unexecuted. |

The 19.33.0.1.0 Autonomous sessions returned a server-host label while both
instance views reported SQL NULL for the current instance's host. The Oracle
contexts and the visible current instance matched by number and name. The RAC
comparison returned a matching first DNS label and different full spellings
from the context and the instance view. The statements collect no peer hosts
from session context. All SQL runs used existing privileges. MOUNT, CDB root,
non-CDB, other grant combinations and other patch levels remain unqualified.
