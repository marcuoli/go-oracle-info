# CDB root PDB visibility and account facts

The caller admits the root or PDB session, privileges, version and connection
lifecycle. These facts perform dictionary reads and retain Oracle's exact
values. They never switch a session, infer a user's classification when the
flag is unknown, or establish visibility beyond the queried instance.

- `containers.VisiblePDBsStatement` selects user PDB identifiers and open modes
  from `V$PDBS` on the reporting instance. Its root guard and `CON_ID > 2`
  exclude the CDB root and seed.
- `containers.SessionLocationStatement` records the current session's container
  name, container number and instance number through `SYS_CONTEXT`.
- `users.PDBAccountsStatement` binds exactly one user PDB's local `CON_ID`
  in `CDB_USERS` and retains both Oracle-maintained and non-maintained flags.
  A visible PDB can return an empty result under `CONTAINER_DATA` restrictions.
  Consumers establish dictionary coverage and choose their own account scope.

Each statement has a typed complete `Read` and a pure `Decode` companion.
The native `Rows` returns caller-owned rows, and `JSON` uses shared bounded
encoding for a complete dataset.

| Oracle release family | PDB visibility and session location | CDB account maintenance flag | Native coverage |
| --- | --- | --- | --- |
| 9iR2, 10gR1, 10gR2, 11gR1, 11gR2 | Unsupported, no SQL | Unsupported, no SQL | Not applicable |
| 12cR1 12.1.0.1 | Selected | Unsupported, no account SQL | Not run |
| 12cR1 12.1.0.2 | Selected | Selected | Not run |
| 12cR2 12.2.0.1 | Selected | Selected | Not run |
| 18c | Selected | Selected | Not run |
| 19c | Selected | Selected | Not run |
| 21c | Selected | Selected | Not run |
| 23c/23ai | Selected | Selected | Not run |
| 26ai 23.26.1.0.0 | Selected | Selected | Native read-only CDB root/PDB checks on a primary and open physical standby as SYSDBA |

Statement tests cover numeric release and patch-set selection. Live coverage
does not establish corresponding privileges for another account, closed,
restricted or RAC PDBs. The `CDB_USERS` result remains scoped to its CDB and
selected local `CON_ID`.

Oracle references reviewed 2026-11-24.

- [Oracle Database 19c CDB_* views](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/cdb_-views.html)
- [Oracle Database 19c V$PDBS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PDBS.html)
- [Oracle Database 19c DBA_USERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_USERS.html)
- [Oracle Database 26ai CDB administration and session context](https://docs.oracle.com/en/database/oracle/oracle-database/26/multi/administering-a-cdb-with-sql-plus.html)
