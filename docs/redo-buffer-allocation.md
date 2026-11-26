# Redo buffer allocation

`memory.ReadRedoBufferAllocation` reads the `Redo Buffers` component from
`GV$SGA` as an exact integer byte value. `INST_ID` identifies the reporting
instance. A completed query can return no rows or a SQL NULL allocation.
Neither supplies an allocation value. Duplicates remain visible to consumers.
The caller supplies a qualified non-CDB or CDB root handle and any instance
filter. Transport completion describes the selected query's visibility.

Use `RedoBufferAllocationStatement` with `DecodeRedoBufferAllocation` when the
caller owns transport. `RedoBufferAllocationRows` and `RedoBufferAllocationJSON`
share the source selection. The root `Client` offers corresponding native rows
and JSON methods. These APIs read allocated memory. The separate initialization
parameter APIs supply the running LOG_BUFFER setting and its default flag.

## Release contracts

Reviewed on 2026-11-26. Oracle's `V$SGA` references establish `NAME` and exact
`VALUE` bytes. Oracle's RAC documentation specifies the `GV$` form and `INST_ID`.
The [19c tuning guide](https://docs.oracle.com/en/database/oracle/oracle-database/19/tgdba/tuning-system-global-area.html)
shows the `Redo Buffers` component. The [19c RAC monitoring guide](https://docs.oracle.com/en/database/oracle/oracle-database/19/racad/monitoring-performance.html)
documents qualified reporting-instance visibility. On 12c and newer numeric
families, the predicate `CON_ID = 0` selects the instance-wide row. Earlier
families omit the column entirely. The SQL returns only `INST_ID` and `BYTES`.

| Release | Oracle Database Reference | Static selection and decode | Native and driver coverage |
| --- | --- | --- | --- |
| 9iR2 (9.2) | [V$SGA](https://docs.oracle.com/cd/A97630_01/server.920/a96536/ch3178.htm) | Legacy shape | Pending a matching admitted instance |
| 10gR1 (10.1) | [V$SGA](https://docs.oracle.com/cd/B14117_01/server.101/b10755/dynviews_2084.htm) | Legacy shape | Pending a matching admitted instance |
| 10gR2 (10.2) | [V$SGA](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_2099.htm) | Legacy shape | Pending a matching admitted instance |
| 11gR1 (11.1) | [V$SGA](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_3028.htm) | Legacy shape | Pending a matching admitted instance |
| 11gR2 (11.2) | [V$SGA](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_3029.htm) | Legacy shape | Pending exact patch-set coverage |
| 12cR1 (12.1) | [V$SGA](https://docs.oracle.com/database/121/REFRN/GUID-4E216A4C-5C7E-43F6-8E2C-CDE442A1CEEC.htm) | Instance-wide predicate | Pending exact patch-set coverage |
| 12cR2 (12.2) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-SGA.html) | Instance-wide predicate | Pending a matching admitted instance |
| 18c (18.x) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-SGA.html) | Instance-wide predicate | Pending matching update and account |
| 19c (19.x) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SGA.html) | Instance-wide predicate | 19.3.0.0.0 RAC primary OPEN READ WRITE, CDB$ROOT SYSDBA. Both visible instances matched the independent SQL result (7,630,848 bytes each). Other RUs, non-CDB, MOUNT, PDB and other privilege scopes remain unqualified. |
| 21c (21.x) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-SGA.html) | Instance-wide predicate | Pending matching update and account |
| 23c / 23ai (23.x) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-SGA.html) redirects to the current 26ai reference | Instance-wide predicate | Earlier 23-series updates require a retained release reference and native qualification |
| 26ai (23.26.x numeric series) | [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SGA.html) | Instance-wide predicate using numeric major 23 | Pending matching update and account |

Unsupported numeric releases are rejected before SQL. No patch-set or RU split
in the reviewed projection is established by these references. Exact quantities,
empty results and nullable values have static tests independent of Oracle access.
The admitted 19.3 driver run used the official pinned session module.
