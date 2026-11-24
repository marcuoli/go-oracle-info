# Upgrade readiness facts

The application owns the assessment and chooses which admitted Oracle handles
to query. Each successful response covers its selected source and scope.
Empty observations retain their provenance and the application can report
unavailable sources separately.

## Recorded PDB plug-in violations

Use `containers.PlugInViolationsStatement`, `DecodePlugInViolations`,
`ReadPlugInViolations`, `PlugInViolationsRows`, and `PlugInViolationsJSON`.
The query inspects `PDB_PLUG_IN_VIOLATIONS`, including any recorded prospective
PDB names. It does not run a compatibility check, join against the live
container inventory, or change a violation's status.

| Numeric release | Documented projection and selector | Static coverage | Native SQL and driver coverage |
| --- | --- | --- | --- |
| 9.2, 10.1/10.2, 11.1/11.2 | Multitenant source unsupported | Rejects before query | Inapplicable |
| 12.1.0.1/12.1.0.2 | CDB root only. TIME, NAME, CAUSE, TYPE, ERROR_NUMBER, LINE, MESSAGE, STATUS, ACTION | Both patch-set selectors and absent CON_ID verified | Unqualified |
| 12.2.0.1, 18c, 19c, 21c, 23ai | Adds nullable CON_ID. Root or connected container selected by scope | Both admission predicates and nullable CON_ID verified | Unqualified |
| 26ai (23.26) | Same projection. Numeric server version is required | 23.26.1 and 23.26.4 selectors verified | 23.26.1.0.0 primary READ WRITE and physical standby READ ONLY WITH APPLY, both CDB$ROOT and SYSDBA. Rows and JSON completed through Windows amd64, Go 1.27.4, go-oracle-session v0.23.0 V2. Independent counts matched zero. The timestamp expression also returned six fractional digits for a nonzero timestamp literal. |

`Root` requires a caller-admitted root and selects visible findings, including
rows with zero or NULL `CON_ID`. `CurrentContainer` restricts rows to the
connected `CON_ID` where the view projects it. Direct PDB visibility, nonempty
native findings, ordinary accounts, RAC and MOUNT remain unqualified. A
successful empty table applies only to the admitted source and scope. Oracle
requires the connection to be able to query the selected view. Missing or
inaccessible view errors retain their classified cause.

TIME is civil Gregorian timestamp text with six fractional digits and no
timezone. LINE and ERROR_NUMBER are exact signed integer text. The optional
CON_ID is also exact text. Rows preserve the required type and the raw status,
including PENDING, RESOLVED, IGNORE, and SQL NULL. In typed results a nil ContainerID
represents the Oracle 12.1 projection, while a present value can be NULL or zero.
Native Rows transfers cursor ownership to the caller. Read validates the
complete buffered observation against raw-text and row bounds. JSON has its own
cursor and envelope-inclusive budget.

Oracle references checked 2026-11-24.

- [12.1 PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/database/121/REFRN/GUID-845E5369-CCB0-4F8D-AE09-447EF0CAC93F.htm)
- [12.2 PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [18c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [19c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [21c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [26ai PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [26ai upgrade troubleshooting in the target CDB root](https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/troubleshooting-oracle-database-upgrade.html)

## Datafile health

Each source has an independent Statement, Decode, Read, Rows and JSON family.
Pass the observed numeric release as `storage.FileHealthOptions.OracleVersion`.
The Client provides the corresponding native cursor and JSON methods and
observes the release over the same caller-owned handle.

| Family | Oracle source and evidence |
| --- | --- |
| `storage.FileStatuses` | `V$DATAFILE` and `V$TEMPFILE` control-file status, file name and absolute file number. File type distinguishes their number spaces. |
| `storage.FileRecovery` | `V$RECOVER_FILE` error/reason, starting SCN and time. OFFLINE NORMAL means recovery is unnecessary for that entry. Oracle can report NULL for an unknown reason. A control file restored from backup or recreated after the failure can make the observation incomplete. |
| `storage.FileBackupModes` | `GV$BACKUP` backup mode status and any start SCN/time for each visible reporting INST_ID. This is the status of datafiles in user-managed backup mode. Oracle may return an error description in STATUS. |
| `storage.FileHeaders` | `V$DATAFILE_HEADER` RECOVER, FUZZY, ERROR, checkpoint SCN and time observed by the connected instance. An unreadable header can leave fields NULL and failed validation can leave unreliable header values. Keep those rows and their ERROR. |
| `storage.BlockCorruptions` | Independently recorded [block corruption](redo-storage-asm.md). |

All four new facts keep native file and reporting instance IDs. The row's
CON_ID and the reading session's SOURCE_CON_ID are separate optional
columns available in the projected 12c and higher layout. SQL NULL, zero
and an unprojected column remain distinct where applicable. Root and PDB
visibility are determined by the admitted account, Oracle's container data
rules and the database state. Use SCN strings without rounding, and treat
DATE values as zone-free Gregorian civil text. The library preserves the
raw status/error codes, accepts complete empty sources, and returns no
successful row prefix after malformed evidence, cancellation or a budget,
query, terminal or close error. Separate calls can observe different times.

| Numeric release | Oracle documentation | Static selectors and decoding | Native and driver qualification |
| --- | --- | --- | --- |
| 9.2, 10.1/10.2, 11.1/11.2 | All five view references describe the selected base columns. Legacy projections omit both container columns. | Tested for each release family | Unqualified |
| 12.1.0.1/12.1.0.2, 12.2.0.1 | References document CON_ID for all five views. Connected CON_ID is evaluated independently. | Tested for all three patch-set profiles | Unqualified |
| 18c, 19c, 21c, 23ai | Current container-aware projection. Oracle's online 23ai paths now resolve to the 26ai reference. | Tested 18.1, 19.32, 21.21, 23.9 | Unqualified |
| 26ai (23.26) | Same projected columns | Tested 23.26.1 and 23.26.4 | 23.26.1.0.0 SYSDBA CDB root on the READ WRITE primary and the READ ONLY WITH APPLY physical standby, Windows amd64, Go 1.27.4, go-oracle-session v0.23.0 V2. Client Rows and bounded JSON completed on both members, with independent counts of 14 file statuses, 0 recovery entries, 11 backup mode rows, and 11 header rows per member. |

Each view requires read access. Native PDB/non-CDB, MOUNT, ordinary accounts,
RAC and positive recovery entries remain unqualified. All SQL and native
cursor paths use the same release-specific projection. Source checks were
performed on 2026-11-24.

- [9.2 V$DATAFILE](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch342.htm), [V$TEMPFILE](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch3225.htm), [V$RECOVER_FILE](https://docs.oracle.com/cd/A97630_01/server.920/a96536/ch3147.htm), [V$BACKUP](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch312.htm), [V$DATAFILE_HEADER](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch343.htm), [SYS_CONTEXT](https://docs.oracle.com/cd/B10500_01/server.920/a96540/functions122a.htm)
- Oracle Database Reference dynamic performance view chapters for [10.1](https://docs.oracle.com/cd/B13789_01/server.101/b10755/toc.htm), [10.2](https://docs.oracle.com/cd/B19306_01/server.102/b14237/toc.htm), [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/toc.htm), [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/toc.htm) and [12.1](https://docs.oracle.com/database/121/REFRN/toc.htm)
- 26ai [V$DATAFILE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATAFILE.html), [V$TEMPFILE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-TEMPFILE.html), [V$RECOVER_FILE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RECOVER_FILE.html), [V$BACKUP](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-BACKUP.html), [V$DATAFILE_HEADER](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATAFILE_HEADER.html), [recovery caveats](https://docs.oracle.com/en/database/oracle/oracle-database/26/bradv/rman-complete-database-recovery.html)

## Recovery and upgrade environment

The SDK reads these six sources independently. Each family provides Statement,
Decode, Read, Rows and JSON. The Client provides matching native cursor and
bounded JSON methods. Callers retain admission, connection affinity and
assessment policy.

| Family | Observation |
| --- | --- |
| `storage.RecoveryArea` | The configured FRA location and exact quota, used and reclaimable bytes plus file count from `V$RECOVERY_FILE_DEST`. Oracle's space-used counter can include files created in previous recovery areas. |
| `storage.RecoveryAreaUsage` | File type, exact percentages of FRA quota, and file count from the selected legacy or current usage view. Preserve zero, NULL, and the original file-type label. |
| `backup.RestorePoints` | Recorded name, SCN, database incarnation, guarantee flag, Oracle's approximate reclaimable byte estimate and creation time. The estimate applies if this restore point is the oldest and is dropped and is zero for a non-guaranteed point. Optional fields retain the point time, preserved flag and PDB provenance when selected. `TIME` and `RESTORE_POINT_TIME` keep nine fractional digits without a timezone. |
| `dataguard.DatabaseIncarnations` | Prior and current RESETLOGS SCNs and civil dates, plus available incarnation IDs, branch status and Flashback Database flag. |
| `dataguard.StandbyPromotionSCN` | Exact `V$DATABASE.STANDBY_BECAME_PRIMARY_SCN` and available container identity. Zero, NULL and an empty observation remain separate. |
| `database.TimeZoneFiles` | Observed time zone file version and, where admitted, the filename and row container ID. This is a database observation and does not determine the client time zone file or whether a DST upgrade is required. |

Transport completeness applies to the selected source. The caller's view
privileges and Oracle's database and container state determine visibility.
The connected `SOURCE_CON_ID` and the view's row `CON_ID` retain independent
meanings. A missing optional field is nil in typed results. A present field
can carry SQL NULL or exact zero. No aggregation, entitlement, recovery action,
state change or policy follows from an individual observation.

| Numeric release | FRA totals and usage | Restore-point projection | Ancestry and promotion SCN | Time zone file |
| --- | --- | --- | --- | --- |
| 9.2 | Both unsupported | Unsupported | Four RESETLOGS fields. Promotion SCN unsupported. | Unsupported |
| 10.1 | Totals. Per-type usage unsupported. | Unsupported | Adds incarnation/parent record IDs, branch ID and status. Promotion SCN available. | Exact version |
| 10.2 | Totals and `V$FLASH_RECOVERY_AREA_USAGE` | Base name, SCN, database incarnation, guarantee, size and nine-digit time | Adds Flashback Database flag | Exact version |
| 11.1 | Same as 10.2 | Adds point time and `PRESERVED` | Same as 10.2 | Exact version |
| 11.2 | Usage now selects `V$RECOVERY_AREA_USAGE`. | Same as 11.1 | Same as 11.1 | Exact version |
| 12.1.0.1/12.1.0.2 | Adds row and connected container IDs. | Adds row and connected container IDs. | Adds row and connected container IDs. | Version plus connected container ID |
| 12.2.0.1, 18c, 19c, 21c, 23ai, 26ai numeric 23.26 | Same as 12.1 | Adds `PDB_RESTORE_POINT`, `CLEAN_PDB_RESTORE_POINT` and `PDB_INCARNATION#`. | Same as 12.1 | Adds filename and row container ID. |

The matrix is documented selection and static coverage. Native and driver
qualification covers Windows amd64, Go 1.27.4, go-oracle-session v0.23.0 V2,
Oracle 26ai 23.26.1.0.0, SYSDBA in CDB$ROOT on a READ WRITE primary and a
physical standby READ ONLY WITH APPLY. All six native cursor and JSON reads
matched independent counts on both members. Each had one FRA total row, eight
file-type rows, zero restore points, two incarnation rows, one promotion-SCN
row and one time-zone-file row. A separate constant SQL read preserved all
nine fractional digits. Direct PDB, older releases, MOUNT, ordinary accounts,
RAC and nonempty restore-point sources remain unqualified natively.

Oracle documents that reading `V$ENCRYPTION_WALLET` can open auto-login
software or external keystores. Keystore evidence is tracked in #166. A safe
collector would need independent, scoped evidence that no affected wallet
could auto-open. An imported snapshot can establish a dated observation of
the exact instance and container with the source's raw state and coverage.
The six facts above require neither wallet inspection nor privileged tools.

- Oracle 10.1 [FRA totals](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_2034.htm), 10.2 [FRA file types](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1110.htm) and [restore points](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_2051.htm), 11.2 [replacement FRA source](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_2124.htm)
- Oracle 9.2 [incarnation ancestry](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch340.htm), 10.1 [promotion SCN](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_1054.htm), 11.1 [restore-point flags](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_2129.htm), 12.1 [restore points](https://docs.oracle.com/database/121/REFRN/GUID-19A363CF-E112-4BFE-8457-E5BB5FAF6FFB.htm) and 12.2 [PDB fields](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-RESTORE_POINT.html)
- Oracle [10g/11g time zone observation](https://docs.oracle.com/cd/E11882_01/readmes.112/e41331/chapter11202.htm), [12.1 Data Pump reference](https://docs.oracle.com/database/121/SUTIL/E41528-10.pdf), and [12.2 time zone file columns](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-TIMEZONE_FILE.html)
- 26ai [FRA quota](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RECOVERY_FILE_DEST.html), [file types](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RECOVERY_AREA_USAGE.html), [restore points](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RESTORE_POINT.html), [incarnations](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE_INCARNATION.html), [database role transition SCN](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE.html), [time zone files](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-TIMEZONE_FILE.html), [keystore side effects](https://docs.oracle.com/en/database/oracle/oracle-database/26/dbtde/managing-keystore-and-tde-master-encryption-key.html)

## Runtime dictionary evidence

The connected dictionary supplies four independent observations. Each family
has Statement, Decode, Read, Rows and JSON, plus the Client's native cursor and
bounded JSON methods. The queries use SYS-qualified views to select the
dictionary even when the login has a local object with the same name.

| Family | Reviewed source and projection |
| --- | --- |
| `performance.InDoubtTransactions` | `DBA_2PC_PENDING`, with local/global transaction IDs, raw state, mixed and advice flags, and failure, force and retry times. Oracle's text `COMMIT#` is exposed as `COMMIT_NUMBER`. Dates retain whole seconds in Gregorian civil time without a time zone. |
| `scheduler.RunningJobs` | `DBA_SCHEDULER_RUNNING_JOBS`, with owner, job name, available subname, exact session ID and exact running instance. Oracle documents that a synchronous `RUN_JOB` with `use_current_session => TRUE` can be absent. Repeated observations are separate snapshots. |
| `objects.MaterializedViews` | `DBA_MVIEWS`, with owner, name, raw staleness and compile state. The two flags are independent observations. |
| `objects.RecycleBinCounts` | `DBA_RECYCLEBIN`, with a count for each visible exact owner. Indexes, LOBs and other dependent entries may contribute. An owner with no visible entries produces no row. |

| Numeric release | In-doubt transactions | Running Scheduler jobs | Materialized views | Recycle-bin counts |
| --- | --- | --- | --- | --- |
| 9.2 | Base projection | Unsupported before SQL | Base projection | Unsupported before SQL |
| 10.1 | Base | Base without subname | Base | Base |
| 10.2, 11.1, 11.2 | Base | Adds `JOB_SUBNAME` | Base | Base |
| 12.1.0.1, 12.1.0.2, 12.2.0.1, 18c, 19c, 21c, 23ai, 26ai numeric 23.26 | Base + connected container | Includes subname + connected container | Base + connected container | Base + connected container |

The 12c and later projections derive `SOURCE_CON_ID` from the admitted
connection's `USERENV`. It identifies the connected container, while
`RUNNING_INSTANCE` describes the job's observed execution instance. The
caller admits a readable, open dictionary and read access to each view.
The connected root scope establishes visibility only through that root's
dictionary. A nil optional field means its column was unprojected. A projected
SQL NULL, a successful empty result, an unsupported selector, and an Oracle
error remain distinct.

Oracle reference columns were checked on 2026-11-24. Static fixtures cover
each selection boundary and modern 23.26.1 and 23.26.4, exact numeric text,
duplicate job observations, NULL, capacity, cancellation and terminal errors.
Native SQL, typed Read, Client Rows and bounded JSON passed on Windows amd64
with Go 1.27.4, go-oracle-session v0.25.0 V2 and Oracle 26ai 23.26.1.0.0.
Both SYSDBA CDB$ROOT endpoints were open. The primary was READ WRITE and the
physical standby was READ ONLY WITH APPLY. All four sources were successfully
empty on both, matching separate dictionary counts. Nonempty native results,
ordinary login, direct PDB/non-CDB, RAC, MOUNT and other numeric releases remain
unqualified.

- Oracle 9.2 [DBA_2PC_PENDING](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch2185.htm#1306535), [DBA_MVIEWS](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch2309.htm#1311218) and [ALL_MVIEWS](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch286.htm#1296735)
- Oracle 10.1 [new dictionary views](https://docs.oracle.com/cd/B13789_01/server.101/b10755/whatsnew.htm), [running job columns](https://docs.oracle.com/cd/B13789_01/server.101/b10755/statviews_1144.htm) and [recycle-bin columns](https://docs.oracle.com/cd/B13789_01/server.101/b10755/statviews_2276.htm), Oracle 10.2 [running job subname](https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_2053.htm) and Oracle 12.1 [synchronous Scheduler jobs](https://docs.oracle.com/database/121/ARPLS/d_sched.htm)
- Oracle Database Reference indexes for [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/toc.htm), [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/toc.htm), [12.1](https://docs.oracle.com/database/121/REFRN/toc.htm) and [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/)
- Oracle 26ai [DBA_2PC_PENDING](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_2PC_PENDING.html), [DBA_SCHEDULER_RUNNING_JOBS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_SCHEDULER_RUNNING_JOBS.html), [DBA_MVIEWS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_MVIEWS.html) and [DBA_RECYCLEBIN](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_RECYCLEBIN.html)
