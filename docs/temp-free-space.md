# Temporary tablespace free space

The storage package exposes TempFreeSpaceStatement, DecodeTempFreeSpace,
ReadTempFreeSpace, TempFreeSpaceRows and TempFreeSpaceJSON. The root Client adds
TempFreeSpace and TempFreeSpaceJSON. Callers admit the session, scope, version,
SQL reference and finite budgets. Each call observes DBA_TEMP_FREE_SPACE in
the connected container.

The source reports tablespace-level byte counts. TABLESPACE_SIZE is total
observed size. ALLOCATED_SPACE includes allocations in use and allocations
available for reuse. FREE_SPACE includes reusable allocated space and currently
unallocated space. It is measured directly, independently of tempfile
allocation, autoextend limits, host free space and permanent free extents.
Allocations can overlap available space. Subtracting ALLOCATED_SPACE from
TABLESPACE_SIZE would omit reusable space.

The byte columns use an explicit Oracle decimal text projection with fixed
numeric characters. The decoder preserves nonnegative integer text, SQL NULL
and zero, validates every row, and keeps duplicates and source order. The JSON
dataset carries byte counts as strings and projected SQL NULL as JSON null.
Native cursor callers own iteration, close and budgets. JSON uses the shared
rowset reader and returns bytes only after successful completion and validation,
including the final encoded size. A successful empty result has complete
provenance. Unsupported releases are rejected before the TEMP statement;
dictionary access, database state and other driver errors retain safe diagnosis
and yield no completed result.

SOURCE_INST_ID and SOURCE_CON_ID describe the executing session. SOURCE_CON_ID
is projected from 12.1. SHARED and OWNING_INST_ID are selected from 12.2.0.1;
OWNING_INST_ID is the raw DBA_TEMP_FREE_SPACE.INST_ID, distinct from the
executing instance. Omitted fields have nil pointers and selected SQL NULL has
a nonnil null value. Observed SHARED rows may have NULL owning IDs. Consumers
retain the exact raw ownership flag and ID, and must establish which local TEMP
row applies to the relevant instance. The SDK retains rows separately and
does no cross-container correlation or local-instance total. Root results
describe the root and PDB results describe that PDB. No CDB-wide discovery,
container changes, SYS retry, grant changes or fallback query occurs.

## Release and qualification matrix

Static projections are B (TABLESPACE_NAME, TABLESPACE_SIZE, ALLOCATED_SPACE,
FREE_SPACE, SOURCE_INST_ID), C (B plus SOURCE_CON_ID), and L (C plus SHARED
and OWNING_INST_ID). The numeric version must identify an admitted family.

| Numeric release | Oracle source and static projection | Native SQL | Go driver / SDK |
| --- | --- | --- | --- |
| 9.2 | View introduced in 11.1. Unsupported before SQL. | Unexecuted | Unqualified |
| 10.1 | View introduced in 11.1. Unsupported before SQL. | Unexecuted | Unqualified |
| 10.2 | View introduced in 11.1. Unsupported before SQL. | Unexecuted | Unqualified |
| 11.1 | Introduction documented. B. | Unexecuted | Unqualified |
| 11.2 | Reference documents the four view columns. B. | Unexecuted | Unqualified |
| 12.1 | Admin guide shows the four view columns. C. | Unexecuted | Unqualified |
| 12.2.0.1 | Reference documents new ownership columns starting at this patch boundary. L. Insufficient 12.2 version rejected before SQL. | Unexecuted | Unqualified |
| 18 | Reference documents ownership. L. | Unexecuted | Unqualified |
| 19 | Reference documents ownership. L. | 19.32.0.0.0 SYSTEM PDB PRIMARY READ WRITE, one shared row | Complete on this exact scope |
| 21 | Reference documents ownership. L. | Unexecuted | Unqualified |
| 23 / 23ai | 23 reference redirects to 26ai. L for numeric 23. | Unexecuted for 23ai environments outside the observed 26ai RU | Unqualified outside the observed RU |
| 26ai | Numeric 23.26.1.0.0 in the lab. L. Hypothetical numeric major 26 is unsupported before SQL. | SYSTEM root and two PDB PRIMARY READ WRITE services, one shared row each | Complete on these exact scopes |

Native read-only checks on 2026-11-20 used the selected SQL and complete
ReadTempFreeSpace through released go-oracle-session v0.10.0 and official
go-ora/v2 v2.9.0. SYSTEM was the least-privileged existing account tested
(ISDBA FALSE). This is not a minimum privilege prescription. The standby
service failed connection preflight in the initial Go checks. OPEN READ ONLY
WITH APPLY, MOUNT, legacy/non-CDB, actual local TEMP and multi-instance RAC
remain unqualified unless covered by separately recorded evidence. Synthetic
tests cover local ownership, NULL, duplicate names, zero and large exact byte
values, unsupported versions and atomic failures.

Oracle documentation reviewed 2026-11-20:

- [11.1 What's New in Oracle Database Reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/whatsnew.htm)
- [11.2 DBA_TEMP_FREE_SPACE](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/statviews_5062.htm)
- [12.1 Managing Tablespaces](https://docs.oracle.com/database/121/ADMIN/tspaces.htm)
- [12.2 DBA_TEMP_FREE_SPACE](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_TEMP_FREE_SPACE.html)
- [12.2 Introduction to Oracle RAC](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/racad/introduction-to-oracle-rac.html)
- DBA_TEMP_FREE_SPACE in the [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_TEMP_FREE_SPACE.html), [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TEMP_FREE_SPACE.html), [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/DBA_TEMP_FREE_SPACE.html), [23](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/DBA_TEMP_FREE_SPACE.html) and [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_TEMP_FREE_SPACE.html) references.
- [26ai CDB_* Views](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/cdb_-views.html)
- [26ai Managing Tablespaces](https://docs.oracle.com/en/database/oracle/oracle-database/26/admin/managing-tablespaces.html)
