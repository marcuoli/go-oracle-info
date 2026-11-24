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
