# PDB Resource Manager observations

Use the independent `containers.ResourceSamples` and
`containers.ResourceUsage` fact families after admitting the Oracle release
and connected scope. The application supplies the handle, reference and limits.
Inventory and saved-state reads remain usable independently. Parameter limits
come from the [scoped parameter facts](container-system-parameters.md).

`GV$RSRCPDBMETRIC` returns per-member/container observations with `BEGIN_TIME`
and `END_TIME` as Gregorian database-local civil DATE values using the shared NLS-independent projection, plus the interval in
centiseconds. CPU utilization, running and waiting session averages, IOPS,
IOMBPS, plan name, average I/O throttle wait and queued parallel statements
remain raw observations. The selected rate columns retain Oracle's operations
and megabytes per second. Throttle time remains in milliseconds. Oracle's
12.2 reference frames several metrics in consumer-group terms. The 26ai
reference describes PDB CPU utilization relative to the system CPU count.
The SDK retains the observed release and performs no normalization or rollup.
Consumers can choose which interval to render.

`GV$RSRC_PDB` returns separate current SGA/PGA gauges in bytes. Numeric fields
are exact decimal text or SQL NULL. Each observation retains the reporting
instance ID, observed container ID and executing container ID. Member scope and
ordering follow the admitted visibility. Duplicate observations remain present.

These are selected root-visible or current-container observations. Whole-CDB
`CON_ID=0` rows are outside these report datasets. Empty query results keep
their complete transport status. Privilege, source access, cancellation and
other failures remain separate from an empty sample. No Resource Manager plan
is enabled or changed by these reads.

## Release and scope matrix

| Release | Resource sample and gauge selection | Static verification |
| --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Unsupported | No query |
| 12.1.0.1, 12.1.0.2 | Unsupported | No query |
| 12.2.0.1 | Documented source projections | Exact column aliases |
| 18, 19, 21 | Reviewed source families | Exact column aliases |
| 23c/23ai and 26ai numeric 23.26.x through documented RU 23.26.4 | Reviewed source families | Exact column aliases |
| Known non-CDB | Unsupported scope | No query |

Oracle sources checked 2026-11-21.
- [12.2 metric reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-RSRCPDBMETRIC.html)
- [12.2 resource reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-RSRC_PDB.html)
- [19c metric reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-RSRCPDBMETRIC.html)
- [26ai metric reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RSRCPDBMETRIC.html)
- [26ai resource reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RSRC_PDB.html)


## Native observations

The exact projections ran on the approved Oracle 26ai 23.26.1.0.0 primary
as SYSTEM without SYSDBA in READ WRITE CDB$ROOT and OMLAB. Root returned
two resource samples and three memory-gauge rows. OMLAB returned one row
from each source. The Windows amd64 harness used Go 1.27.4,
go-oracle-session v0.15.0 and go-ora/v2 v2.9.0. It checked native cursor
iteration, column aliases and complete bounded JSON independently. The
buffered Reader path is covered by fixtures. The existing 12.2.0.1.0 LABPDB profile failed at connection before any SQL. No approved 12.2 root entry was available. Other releases, MOUNT, other
login privileges, RAC member coverage and other Resource Manager
configurations have no exact native qualification in this run.
