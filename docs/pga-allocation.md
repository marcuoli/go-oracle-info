# Exact allocated PGA for the executing instance

Use memory.ReadPGAAllocation with an admitted handle, the observed numeric
release, a SQL reference and optional buffered bounds. The selected
V$PGASTAT row for total PGA allocated returns the current allocation of the
executing instance in bytes. The same statement records both the raw source
CON_ID and the executing session's container ID, instance number and instance
name. Database activity can change the value between reads.

The typed reader accepts at most one row, with a byte unit, source CON_ID 0
and valid executing-session identity. For a CDB, that row has whole-CDB
scope within the executing instance. It can be visible from an admitted PDB,
where the session CON_ID still identifies that PDB. In a non-CDB source and
session CON_ID are both 0. A PDB's resource gauges, configured targets,
current allocation and fleet totals describe different facts. Callers retain
their scope admission, connection lifetime and reporting policy.

A successful empty result keeps complete transport provenance. A row can
retain SQL NULL bytes while proving its source scope. Missing or opposing
scope, absent identity, unexpected units and duplicate rows fail validation.
Failed transport, cursor finalization, cancellation and capacity checks
produce no successful rows. Typed values retain exact decimal byte text.
The Client rows path transfers the source cursor, and the JSON path consumes
a separate bounded cursor and validates it. The existing PGAAllocated reader
and JSON retain the previous rounded GiB projection and release support.

## Oracle release coverage

| Numeric release | Documented source / static selection | Native SDK qualification of this API |
| --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Historical PGA statistic lacks CON_ID. Unsupported without SQL. | None. |
| 12.1, 12.2, 18, 19, 21 | V$PGASTAT includes VALUE, UNIT and CON_ID. The same six-column selection is checked by fixtures. | Native scope listed below. |
| 23c/23ai and 26ai numeric 23.26.x | Same named statistic and columns. Static coverage includes 23.26.4. | None for this API. |

Oracle documentation reviewed 2026-11-28.

- [Oracle9i Release 2 memory guidance](https://docs.oracle.com/cd/A97630_01/server.920/a96533/memory.htm)
- [Oracle Database 10g Release 1 V$PGASTAT](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_2016.htm)
- [Oracle Database 12.1 V$PGASTAT](https://docs.oracle.com/database/121/REFRN/GUID-4666F72E-1E2F-4FFF-89C7-E8144657F78A.htm)
- [Oracle Database 12.2 V$PGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-PGASTAT.html)
- [Oracle Database 19c V$PGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PGASTAT.html)
- [Oracle Database 26ai V$PGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-PGASTAT.html)

## Native scope

Native qualification is pending.
