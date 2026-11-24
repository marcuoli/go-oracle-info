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

Keystore status requires separate [evidence and qualification](#tde-keystore-status-evidence).

- Oracle 10.1 [FRA totals](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_2034.htm), 10.2 [FRA file types](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1110.htm) and [restore points](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_2051.htm), 11.2 [replacement FRA source](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_2124.htm)
- Oracle 9.2 [incarnation ancestry](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch340.htm), 10.1 [promotion SCN](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_1054.htm), 11.1 [restore-point flags](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_2129.htm), 12.1 [restore points](https://docs.oracle.com/database/121/REFRN/GUID-19A363CF-E112-4BFE-8457-E5BB5FAF6FFB.htm) and 12.2 [PDB fields](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-RESTORE_POINT.html)
- Oracle [10g/11g time zone observation](https://docs.oracle.com/cd/E11882_01/readmes.112/e41331/chapter11202.htm), [12.1 Data Pump reference](https://docs.oracle.com/database/121/SUTIL/E41528-10.pdf), and [12.2 time zone file columns](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-TIMEZONE_FILE.html)
- 26ai [FRA quota](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RECOVERY_FILE_DEST.html), [file types](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RECOVERY_AREA_USAGE.html), [restore points](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-RESTORE_POINT.html), [incarnations](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE_INCARNATION.html), [database role transition SCN](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE.html), [time zone files](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-TIMEZONE_FILE.html), [keystore side effects](https://docs.oracle.com/en/database/oracle/oracle-database/26/dbtde/managing-keystore-and-tde-master-encryption-key.html)


## TDE keystore status evidence

Issue #166 requires that assessment leave keystore and database state unchanged.
Oracle documents that a query of `V$ENCRYPTION_WALLET` can open software
auto-login wallets and external auto-login keystores. The view also validates
TDE configuration and can write alert log warnings. A column list, row filter,
read-only transaction or SYSDBA login does not establish a safe execution path.
The equivalent `GV$` source increases the potential scope to visible instances.
In a CDB, the connected root can see wallet rows for PDBs. A direct PDB sees only
its own details. A root-only result filter does not independently establish
which wallets the server may inspect.

| Reviewed source | Evidence it supports |
| --- | --- |
| `TDE_CONFIGURATION` and `WALLET_ROOT` | Configured type and location. These parameters cannot prove effective wallet state or exclude every auto-login path on the affected members and containers. |
| `V$DATABASE_KEY_INFO` and `V$ENCRYPTED_TABLESPACES` | Control-file key metadata and tablespace encryption state, respectively. Neither supplies live wallet status, order, mode or backup flags. |
| Alert log or audit evidence | Observed historical messages or actions. Missing records and automatic opens prevent a complete current-state inference. |
| `V$ENCRYPTION_KEYS`, fixed tables or attempts to read encrypted data | No side-effect-free wallet status contract was verified for these paths. They stay outside assessment. |

The public reference review found no `V$ENCRYPTION_WALLET` listing in the
9.2, 10.1 or 10.2 reference indexes. The 11.1 and 11.2 references document
`STATUS` and locator fields. The 12.1 and 12.2 references add `WALLET_TYPE`,
`WALLET_ORDER`, `FULLY_BACKED_UP` and `CON_ID`. The reviewed 18, 19, 21,
23 and 26 references also document `KEYSTORE_MODE`. The Oracle 23 reference
redirects to the 26ai page. For 26ai, record the observed numeric 23.26 release
and RU. These are documented projection observations, with live collection
unqualified across the reviewed range. A password-only deployment would still
need independent evidence for the exact version, wallet configuration,
privilege, member and container scope, plus a guarantee that the query itself
has no prohibited effect. The affected view cannot verify its own preconditions.

### Historical snapshot contract

Assessment can safely consume an **already collected, sanitized snapshot**.
Reading the artifact requires only file input. The snapshot represents what its
producer observed at capture time, including the effect of the producer's
original method. It cannot establish the wallet state before that observation
or at the later assessment. Qualify the producer and capture separately from
the import.

- Require the recorded Oracle source, numeric release, UTC observation time,
  producer identity, and a verified evidence digest. Record whether the
  captured result completed, was truncated, failed or has unknown coverage.
  Source identity and the digest establish provenance and byte consistency,
  with producer trust assessed separately.
- Bind the capture to the exact database member and instance using DBID,
  database unique name, instance identity and startup where independently
  recorded. DBID alone cannot distinguish a Data Guard primary and standby.
  Preserve the original login role, OPEN/MOUNT mode, connected scope,
  reporting instance and row `CON_ID`. Use PDB identity when matching a PDB
  observation. Leave ambiguous or mismatched identity unavailable. A CDB-wide
  `CON_ID=0` row supplies no specific PDB observation.
- Preserve the original `STATUS`, `WALLET_TYPE`, `KEYSTORE_MODE`,
  `WALLET_ORDER` and `FULLY_BACKED_UP` fields where captured, including
  raw unknown values, SQL NULL, unprojected fields and multiple rows per scope.
  Keep complete empty, missing, incomplete, mismatched and stale evidence
  distinct. The caller owns the age policy and reports the original time
  alongside any accepted historical status.
- Exclude wallet paths, key identifiers and material, credentials, commands
  and original unsanitized output. Import must remain independent of database,
  wallet and external keystore connections. An already admitted, separate
  database identity check may bind an artifact to its source.

No qualified historical snapshot is currently available for either approved
SYSDBA LAB root. The previous 23.26.1.0.0 root receipts identify the READ WRITE
primary and READ ONLY WITH APPLY physical standby only. Both import checks
remain pending a qualified snapshot for each exact member and the consumer's
import path. Keep keystore status unavailable until evidence is admitted.

- Oracle references on auto-open behavior for [software wallets](https://docs.oracle.com/en/database/oracle/oracle-database/26/dbtde/configuring-united-mode2.html) and [external keystores](https://docs.oracle.com/en/database/oracle/oracle-database/26/dbtde/managing-keystore-and-tde-master-encryption-key.html).
- Wallet view references for [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_1112.htm), [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_1125.htm), [12.1](https://docs.oracle.com/database/121/REFRN/GUID-054B6FBC-4263-4A91-AEBD-D6FB50C22107.htm), [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-ENCRYPTION_WALLET.html), [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ENCRYPTION_WALLET.html) and [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ENCRYPTION_WALLET.html).
- Oracle references on [keystore configuration](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/TDE_CONFIGURATION.html), [database key metadata](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE_KEY_INFO.html), [tablespace encryption](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ENCRYPTED_TABLESPACES.html) and [dynamic view visibility](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/about-dynamic-performance-views.html).

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

## Object identities, redo threads and active services

These are three independent observations with Statement, Decode, Read, Rows,
and JSON paths and matching Client methods. The caller admits the readable
database, privileges, observed release, and relevant role. Each read retains
only what its selected source returns.

| Family | Source and interpretation |
| --- | --- |
| `objects.ObjectIdentities` | `SYS.DBA_OBJECTS` rows for a required list of exact owners, such as `SYS` and `SYSTEM`. Owner names use binary comparisons and bind values. Preserve object name, nullable subobject, type, exact dictionary object number and, when selected, the actual edition. The dictionary exposes the connection's current edition. Comparing names, classifying duplicates and deciding exceptions belong to the caller. |
| `redo.Threads` | `V$THREAD` keeps thread number, raw status and enabled flag, Oracle's optional instance name, and the numeric connected instance separately. It requires an observed `PRIMARY` role. Oracle documents that the view does not return meaningful results on a physical standby. Other roles and an absent role return `redo.ErrUnsupportedScope` before the view is queried. |
| `services.ActiveServices` | `GV$ACTIVE_SERVICES` preserves reporting `INST_ID`, actual service name and network name, including internal names and SQL NULL. It complements the existing configured service definitions. A visible active service does not establish listener or client reachability, and visibility depends on the admitted instance and `CONTAINER_DATA` scope. |

| Numeric release | Object identities | Redo threads on primary | Active services |
| --- | --- | --- | --- |
| 9.2 | Base projection | Base projection | Unsupported before SQL |
| 10.1, 10.2, 11.1 | Base projection. The 11.1 edition field is reserved. | Base projection | Base projection |
| 11.2 | Adds `EDITION_NAME` | Base projection | Base projection |
| 12.1.0.1, 12.1.0.2, 12.2.0.1, 18c, 19c, 21c, 23ai, 26ai numeric 23.26 | Adds `SOURCE_CON_ID` | Adds `CON_ID` and `SOURCE_CON_ID` | Adds `CON_ID` and `SOURCE_CON_ID` |

`SOURCE_CON_ID` records the connected container. A view's `CON_ID` describes
the row and can differ. Zero is a valid Oracle value. A nil optional field
means the projection omitted it. A selected SQL NULL, a successful empty
result, an unsupported selection, and an Oracle error remain distinct.

Object owners accept up to 128 Unicode characters. The caller supplies names
from the admitted database. Oracle documents OBJECT_ID as required through 21c.
A projected NULL is preserved for the numeric 23 series.

The object list has no SQL row limit. Use explicit adequate bounds for its
buffered reader or JSON envelope. When the SDK's default 10,000 rows are
insufficient, the caller can set higher finite limits or use the native
cursor with its own row and byte budget. With an already admitted stream,
the exact Statement and Decode can verify chunks. Completion is established
only after the entire cursor and its terminal status have been validated.

Oracle reference columns were checked on 2026-11-24. Fixtures verify the
numeric release boundaries through 23.26.4, exact owner binds and numbers,
separate instance/container identities, SQL NULL, unsupported-before-query,
capacity, and terminal failures. Native SQL, typed Read, Client Rows, and
bounded JSON passed on Windows amd64, Go 1.27.4, go-oracle-session v0.25.0
with go-ora V2, and Oracle 26ai 23.26.1.0.0.

| Native SYSDBA CDB-root scope | Object identities | Redo threads | Active services |
| --- | ---: | --- | ---: |
| READ WRITE primary | 53,821 | 1 | 6 |
| READ ONLY WITH APPLY physical standby | 53,821 | Unsupported before the view was queried | 4 |

Each object read had 53,732 SYS rows and 89 SYSTEM rows, with 3,782,848
bytes of buffered value text and 5,061,867 bytes of bounded JSON. The native
cursor, typed reader and JSON counts matched separate source counts in this
run. Direct PDB/non-CDB, ordinary accounts, RAC, MOUNT, other roles and
other numeric releases remain unqualified natively.

- Oracle 9.2 [ALL_OBJECTS](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch290.htm#1297575) and [V$THREAD](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch3229.htm#1128344)
- Oracle 10.1 [active service view introduction](https://docs.oracle.com/cd/B13789_01/server.101/b10755/whatsnew.htm), [projected columns](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_1005.htm) and [RAC service-by-instance source](https://docs.oracle.com/cd/B14117_01/rac.101/b10768/design.htm)
- Oracle [11.1 reserved edition field](https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_1145.htm#i1583352), [11.2 edition identity](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_1158.htm#i1583352) and [12.1 USERENV](https://docs.oracle.com/database/121/SQLRF/functions199.htm)
- Oracle 12.1 [V$THREAD](https://docs.oracle.com/database/121/REFRN/GUID-D1A0B100-BE3F-4098-8E71-719B7CCCFF4B.htm) and [V$ACTIVE_SERVICES](https://docs.oracle.com/database/121/REFRN/GUID-488BCD15-3125-4CD1-BE26-9E5CA6BC8AE9.htm)
- Oracle 26ai [DBA_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_OBJECTS.html), [V$THREAD](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-THREAD.html), [V$ACTIVE_SERVICES](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ACTIVE_SERVICES.html) and [dynamic view scope](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/about-dynamic-performance-views.html)

## Connected dictionary and root rollups

These six independent facts share the Statement, Decode, Read, Rows, and
bounded JSON contract. Retain connected and row container identities when
correlating them with a separate container inventory.

| Family | Source and observation |
| --- | --- |
| `identity.GlobalName` | `SYS.GLOBAL_NAME` returns the connected dictionary's raw global name as a scalar. The decoder accepts zero or one row and preserves SQL NULL. |
| `database.Properties` | `SYS.DATABASE_PROPERTIES` returns property name, nullable value and description. The caller sets finite row and byte bounds for a complete inventory. The dedicated default-temporary fact still provides its one-property selection. |
| `containers.RegistryComponents` | `CDB_REGISTRY` returns row `CON_ID`, component ID/name, version, optional full version, status, and Oracle's raw modified text. |
| `containers.InvalidObjectCounts` | `CDB_OBJECTS` groups the visible rows by `CON_ID` and counts only those with status `INVALID`. An observed group can have exact zero. A missing group stays unobserved. |
| `containers.SQLPatchHistory` | `CDB_REGISTRY_SQLPATCH` returns row `CON_ID`, exact patch ID and UID, raw action/status, six-digit Gregorian civil timestamp, and description. Separate attempts and duplicate rows remain visible. |
| `containers.LegacyAuditTablePlacement` | `CDB_TABLES` returns row `CON_ID`, owner, table name and nullable tablespace for the exact `SYS.AUD$` and `SYS.FGA_LOG$` identities. A nullable tablespace remains a nullable metadata observation. Current-container callers can use the existing owner- and name-bound `users.TableMetadata` API. |

The root families require `RootDictionaryOptions` with caller-admitted
`containers.Root` on the exact connection. The SQL uses SYS-qualified CDB views and guards execution
visibility with connected container ID 1. Oracle `CONTAINER_DATA`, dictionary
grants, PDB open or restricted state, and instance reachability determine which
rows are visible. The row `CON_ID` stays separate from the root's admission
scope. For the two connected dictionary readers, 12c and later return the
connection's `SOURCE_CON_ID`. A completed result establishes coverage of the
selected transport. Use a separate admitted PDB inventory to retain closed and
unobserved containers explicitly. Audit placement describes table metadata.

| Numeric release | Connected name and properties | Component, invalid-count and legacy audit root reads | Root SQL patch history |
| --- | --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Base projection | Unsupported before SQL | Unsupported before SQL |
| 12.1.0.1 | Adds `SOURCE_CON_ID` | Base including row `CON_ID` | Unsupported before SQL on this exact patch set |
| 12.1.0.2, 12.2.0.1 | Same connected projection | Same root projection | Base including row `CON_ID` |
| 18c, 19c, 21c, 23ai, 26ai numeric 23.26 | Same connected projection | Component projection adds `VERSION_FULL` | Same root projection |

Oracle's 12.1.0.1 PSU example qualifies the local DBA SQL patch source
with a narrower projection. The 12.1.0.2 root example and the corresponding
Oracle Reference definitions establish the conservative floor for the new
root patch projection. Oracle documents required component IDs and patch ID, UID, action and time.
Patch status can be SQL NULL on 12.1 and 12.2 and is required from 18c.
The online 23ai reference routes to 26ai. Oracle column
references were checked on 2026-11-24. Fixtures cover numerical admission
through 23.26.4, root scope rejection, visible zero, absent groups, exact
numbers and timestamps, capacity, invalid evidence, and access and terminal
failures.

Native reads used Windows amd64, Go 1.27.4, go-oracle-session v0.25.0 with
go-ora V2, and Oracle 26ai 23.26.1.0.0. On both SYSDBA CDB roots, the READ WRITE
primary and READ ONLY WITH APPLY physical standby, the typed reader, native
cursor, bounded JSON and independent source counts agreed.

| Observation | Rows per endpoint | Visible dictionary scope |
| --- | ---: | --- |
| Global name | 1 | Connected root |
| Database properties | 45 | Connected root |
| Registry components | 28 | 14 at `CON_ID=1`, 14 at `CON_ID=3` |
| Invalid object groups | 2 | `CON_ID=1` and `CON_ID=3`, with each exact count checked independently |
| SQL patch attempts | 2 | One at each of `CON_ID=1` and `CON_ID=3` |
| Legacy audit placements | 4 | Two at each of `CON_ID=1` and `CON_ID=3` |

The existing owner-bound local audit lookup returned only the two root
placements on each connection. The observed additional rows for
`CON_ID=3` establish the root coverage gap. Direct PDB/non-CDB, MOUNT,
ordinary login, RAC and other numeric releases remain unqualified natively.

- Oracle 9.2 [GLOBAL_NAME](https://docs.oracle.com/cd/B10500_01/server.920/a96536/ch2431.htm) and [DATABASE_PROPERTIES](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch2183.htm)
- Oracle 12.1 [CDB view visibility](https://docs.oracle.com/database/121/REFRN/GUID-2B8A1B8B-4F5B-4FBB-AE0B-68B6F8BA7D5E.htm), [USERENV](https://docs.oracle.com/database/121/SQLRF/functions199.htm), and [DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/database/121/REFRN/GUID-F6D2BC12-606C-41FE-B9E8-F3702CD32F89.htm)
- Oracle [12.1.0.1 local patch example](https://www.oracle.com/br/technical-resources/articles/database-performance/cdb-when-pdb-has-to-be-recovered.html), [12.1.0.2 root patch example](https://www.oracle.com/webfolder/technetwork/jp/ondemand/od12c-aug2014/03_DB12102_coretech_UPG_v2.pdf), and [18c registry full version](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY.html)
- Oracle 26ai [GLOBAL_NAME](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/GLOBAL_NAME.html), [DATABASE_PROPERTIES](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DATABASE_PROPERTIES.html), [CDB views](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/cdb_-views.html), [DBA_REGISTRY](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_REGISTRY.html), [DBA_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_OBJECTS.html), [DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_REGISTRY_SQLPATCH.html), and [DBA_TABLES](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_TABLES.html)
