# Container inventory and saved PDB state

Use the same fact API for each admitted Oracle release. An application supplies
the observed numeric release, authenticated handle and known connected scope.
`containers.Inventory` exposes root-visible members or exact current-container
observations. `containers.SavedStates` exposes independently read dictionary
definitions for an admitted CDB root. An entry for an inactive instance remains
visible by its stored instance name. No live instance join is needed.

Inventory rows retain the actual reporting `INST_ID`, observed `CON_ID` and
executing `SOURCE_CON_ID`. Root scope keeps the root and visible PDBs, including
any visible seed or application containers. Current-container scope matches
the executing container. Preserve source open/restricted states and duplicates.
DBID and CON_UID use exact decimal text and GUID uses Oracle's hexadecimal RAW
representation. Sizes remain exact bytes. The SDK preserves the value Oracle
reports when the PDB is mounted, including zero or NULL. A missing projected
field is distinct from a selected SQL NULL.

The application establishes same-session scope and privileges. Scope guards
prevent cross-container overreads if a handle is misplaced. An empty guarded
result does not establish the actual connected scope. Complete means the
selected cursor finished. Visibility can depend on `CONTAINER_DATA`, connected
scope, the login and database/member state. Saved states are configuration.
They do not establish current open mode or an instance's reachability.

Unsupported releases and scopes fail before the cursor opens. A successful
empty read has nonnil empty rows. An explicit ORA-01031 is inaccessible.
ORA-00942, ORA-01109 and ORA-01219 are unavailable observations. The private
cause remains inspectable. Other failures discard the source's prefix. Native
Rows return the caller's actual cursor. JSON consumes a separate cursor through
rowset and counts the complete envelope against the byte limit.

## Source selection and qualification

| Numeric release | GV$CONTAINERS | Saved state | Initial static fixture |
| --- | --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Unsupported | Unsupported | Version rejected before SQL |
| 12.1.0.1 | Base identity, scope, open/restricted | Unsupported | Base projection without sizes |
| 12.1.0.2 | Base plus TOTAL_SIZE | DBA_PDB_SAVED_STATES | Partial optional projection |
| 12.2.0.1 | Add MAX_SIZE and application container fields | Same source | Modern projection |
| 18, 19, 21 | Modern projection | Same source | Each release selected |
| 23c/23ai, and 26ai numeric 23.26.x through documented RU 23.26.4 | Modern projection | Same source | Selected on observed numeric 23, not branding |

The maintained 12.1 reference documents TOTAL_SIZE. The early 12.1.0.1
projection conservatively omits size fields until qualified on that patch set.
The 12.2 reference dates MAX_SIZE and application container fields to
12.2.0.1. Database architecture, OPEN/MOUNT and privileges are independent
of the release profile.
Shortened 12.x version strings without an admitted patch-set number fail before
SQL. An admitted explicit non-CDB scope is unsupported.

Oracle sources checked 2026-11-21.
- [12.1 V$CONTAINERS](https://docs.oracle.com/database/121/REFRN/GUID-8865FE4F-C22F-4B04-BC21-A28FFFC92072.htm)
- [12.1 DBA_PDB_SAVED_STATES](https://docs.oracle.com/cd/E57425_01/121/REFRN/GUID-274837D6-59B3-4C9D-80C5-481BF8298DBA.htm)
- [12.2 V$CONTAINERS](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-CONTAINERS.html)
- [19c V$CONTAINERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-CONTAINERS.html)
- [26ai V$CONTAINERS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-CONTAINERS.html)
- [26ai DBA_PDB_SAVED_STATES](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_PDB_SAVED_STATES.html)
- [26ai release update 23.26.4](https://docs.oracle.com/en/database/oracle/oracle-database/26/nfcoa/ru23_26_4.html)

## Native qualification

On 2026-11-21, Windows amd64 Go 1.27.4 used go-oracle-session v0.15.0 and
official go-ora/v2 v2.9.0. The Oracle 26ai primary reported numeric
23.26.1.0.0 and READ WRITE. SYSTEM connected without SYSDBA. The root and
its PDB were separate already-admitted connections.

| Scope | Borrowed Rows and bounded JSON | Observed rows |
| --- | --- | --- |
| CDB root | Inventory | 3 |
| CDB root | Saved states | 1 |
| Connected PDB | Inventory | 1 |

Native cursor metadata and JSON column names matched for each dataset. The
shared JSON pipeline consumed the cursor and validated the complete typed
projection. Early 12.1, 12.2 and other release/RU combinations, different
privileges, MOUNT, application roots and multi-member RAC are unqualified.
The reported row counts are observations for those sessions.
