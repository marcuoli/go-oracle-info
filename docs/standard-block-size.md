# Standard database block size

Use database.ReadStandardBlockSize on the caller's admitted reader with the
observed numeric release. It preserves exact byte text or SQL NULL and source
attempts. V$PARAMETER supplies the effective DB_BLOCK_SIZE value. Oracle defines
this standard at database creation and does not permit a PDB to change it.
Other tablespaces may use nonstandard block sizes.

An admitted open dictionary can provide the equivalent SYSTEM tablespace
block size. Set AllowSystemTablespaceFallback to enable that one alternate
after an empty, NULL or known unavailable primary source. The alternate uses
only the visible SYSTEM row in the current container. Terminal read errors,
mixed failures, invalid rows, bounds and cancellation stop collection. A
missing alternate never turns a denied primary into a successful absence.
A value selected from SYSTEM keeps its dictionary provenance. The parameter
inventory is independent and may retain masked SQL NULL values.

Statement, Decode, Rows and JSON each select one explicit source. The native
cursor remains caller-owned. JSON consumes a separate bounded cursor and has no
fallback. The Client methods expose the primary cursor and JSON. Individual
tablespaces may have different block sizes. Use the existing storage.Tablespaces for their individual block sizes.

## Release and scope

| Oracle release | Documented application of the fact and static selection | Native qualification |
| --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | V$PARAMETER.VALUE and current dictionary SYSTEM.BLOCK_SIZE. | Pending for the new typed fact. |
| 12.1, 12.2, 18, 19, 21 | Same aliases and current-container dictionary. DB_BLOCK_SIZE is fixed and not PDB-modifiable. | Typed native checks passed on the 19.3 and 19.33 PDB scopes below. |
| 23c/23ai and 26ai, numeric 23 | Same source contract. Numeric 26 and version labels are rejected. | Pending for the new typed fact. |

Callers admit connected database/container privileges and state. The
dictionary alternate requires an open readable scope. There is no
container switch, fixed-table, filesystem, grant, or CDB rollup.

Oracle references checked 2026-11-28.

- [Oracle9i R2 Administrator's Guide, Creating an Oracle Database](https://docs.oracle.com/cd/A97630_01/server.920/a96521/create.htm)
- [Oracle Database 19c Reference, DB_BLOCK_SIZE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DB_BLOCK_SIZE.html)
- [Oracle Database 19c Administrator's Guide, Specifying Database Block Sizes](https://docs.oracle.com/en/database/oracle/oracle-database/19/admin/creating-and-configuring-an-oracle-database.html)
- [Oracle Database 19c Reference, DBA_TABLESPACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TABLESPACES.html)
- [Oracle Database 26ai Concepts, Tablespace Block Size](https://docs.oracle.com/en/database/oracle/oracle-database/26/cncpt/logical-storage-structures.html)

Exact typed reads independently returned 8192 bytes from each source on the
19.3.0.0.0 self-managed PDB as SYSDBA and the 19.33.0.1.0 OLTP Autonomous PDB as
ADMIN without SYSDBA. The released Windows pure-Go V2 session layer was used.
Each primary succeeded directly. Fixture tests cover fallback selection,
refusal and cancellation. The same Autonomous session preserved masked SQL
NULL values in its broader effective parameter inventories. Other accounts,
instances, states and releases have no exact native coverage in this run.
