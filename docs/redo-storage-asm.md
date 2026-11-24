# Redo, storage and ASM fact sources

These additive APIs observe admitted read-only sources. They do not connect,
change containers/session settings, grant access, retry or own a driver. Source
selection is a static compatibility contract, not native qualification. Callers
admit endpoint/container/edition, source privileges and reporting-member coverage.

## Functional API and ownership

`redo.Groups`, `Members`, `History`, `ArchivedLogs` and `Clock` each expose `Statement`,
`Decode`, `Read`, `Rows` and `JSON` companions (for example `ReadGroups` and
`GroupsStatement`). Storage provides the same companions for `Tablespaces`,
`Datafiles`, `Tempfiles`, `FreeExtents`, `Segments`, `LongColumns`,
`DefaultTemporary`, `TemporaryGroups` and `BlockCorruptions`. `memory.SGAComponents` provides
observed reporting-instance allocations. Its legacy `storage` entry points
preserve their public types and behavior. `asm.Clients` provides
its own companions. There is no query registry or connection/cache manager.

Functional readers accept the observed Oracle major, a valid caller-owned SQL
reference and any explicitly admitted filter/options. `Request`, `HistoryRequest`,
`TempfileRequest` and `SGARequest` document those inputs. The root non-owning
`Client` hides version lookup for native rows and JSON. Its methods include
`RedoGroups`, `RedoMembers`, `RedoHistory`, `ArchivedLogs`, `Tablespaces`,
`Datafiles`, `Tempfiles`, `FreeExtents`, `Segments`, `LongColumns`,
`DefaultTemporary`, `TemporaryGroups`, `BlockCorruptions`, `SGAComponents` and `ASMClients`, each
with a `JSON` companion. Independent calls are separate observations.

A `Reader.Query` must return only complete finite-budget results and retain query,
iteration, close and cancellation failures. `Read` queries once and publishes no
slice after any failure, malformed late row or post-query cancellation. A pure
`Decode` cannot establish transport completion; its `Provenance.Complete` remains
false. Successful `Read` and JSON set it true for that selected admitted source,
never for every RAC member, container or historical period. Successful empty
slices differ from an unavailable source; errors publish no prefix. Callers retain
partial coverage by storing independent member/source outcomes.

`Rows` returns the actual caller-native `*sql.Rows`; callers own Next/Scan/Err/Close,
finite budgets and deadlines. JSON runs its own identical selection and delegates
all cursor consumption/finalization to `go-oracle-session/rowset`. After that complete read, the exact
typed domain decoder validates every row, including instance/time/column/property
admission. Only then is the original native table encoded; JSON never bypasses
the domain decoder or consumes a second cursor. JSON limits
include the complete provenance envelope. Any row/byte capacity or terminal
failure returns nil JSON. No selector uses ROWNUM, FETCH FIRST, top 50 or an
implicit complete-looking prefix. Large segment/free-extent/history inventories
can exceed the default rowset limits: callers must supply finite reviewed limits
or use native streaming with complete terminal accounting. Existing application
buffer limits are not evidence of exhaustive source coverage.

## Sources and explicit capability selection

| Dataset | Source and scope | Columns/capability |
| --- | --- | --- |
| Redo groups | GV$LOG, qualified visible reporting IDs | Common 11g group/thread/sequence/bytes/member/archive/status/first-SCN/time. BLOCK_SIZE selected from 12; NEXT_CHANGE/NEXT_TIME conservatively from 18 because a major 12 alone cannot establish the reviewed 12.2 source. |
| Redo members | GV$LOGFILE, qualified visible reporting IDs | Group/member/status/type/recovery-destination. NULL status stays NULL (documented in-use member). |
| Source-local clock | SYSDATE via SYS.DUAL, connected source instance | Exactly one non-NULL Gregorian whole-second civil label, reporting instance and optional connected CON_ID; no timezone/UTC interpretation. |
| History | GV$LOG_HISTORY, qualified visible IDs and finite first-time interval | Record/stamp/thread/sequence/resetlogs/SCN/time; no retention proof. |
| Archive copies | GV$ARCHIVED_LOG, qualified visible IDs and finite first-time interval | Destination/name, record/stamp, thread/sequence, resetlogs ID/change/time, activation, archival thread, block counts/size, first/next/completion times and raw status/role/copy flags. |
| Tablespaces | DBA_TABLESPACES, current-container dictionary | Contents/status/retention/bigfile, block size, logging/management/encryption. No undo/owner exclusions. |
| Datafiles | DBA_DATA_FILES, current-container dictionary | File/relative identity, bytes/blocks/user bytes, maximum, autoextension/increment, availability/online status. Keystore/source access remains caller admission; no wallet operation occurs. |
| Recorded block corruption | V$DATABASE_BLOCK_CORRUPTION, connected fixed view | Recorded file/block ranges, change number and corruption type. Actual CON_ID from 12.1. See the [release matrix](#recorded-block-corruption). |
| Tempfiles | DBA_TEMP_FILES, current-container dictionary | Common 11g file projection. `TempfileOptions.IncludeLocal` explicitly admits SHARED and owning INST_ID from 18+. It is not selected automatically. |
| Free extents | DBA_FREE_SPACE, current-container dictionary | Tablespace/file/block identity and raw bytes/blocks. Offline locally managed files may have missing extent rows; absence does not independently establish zero free bytes. |
| Segments | DBA_SEGMENTS, current-container dictionary | Owner/name/type/subtype/partition/tablespace, allocated bytes/blocks/extents and header identity. |
| LONG attribution | DBA_TAB_COLUMNS, current-container dictionary | Only exact DATA_TYPE LONG/LONG RAW, owner/table/column/ordinal; excludes hidden system-generated columns by source definition. |
| Default temporary | DATABASE_PROPERTIES, current-container dictionary | Exact DEFAULT_TEMP_TABLESPACE property, raw value/description, at most one property row. |
| Temporary groups | DBA_TABLESPACE_GROUPS, current-container dictionary | Group-to-tablespace membership; default temporary may identify a group. |
| SGA components | GV$SGASTAT, qualified visible reporting IDs | All matching NAME=buffer_cache OR POOL=shared pool rows, with the predicate grouped before instance admission. Raw pool/name/bytes remain separate. |
| ASM connections | GV$ASM_CLIENT, qualified visible reporting IDs | Group/instance/database/software/compatible/status. `Filter.SourceRole` must explicitly admit database_instance or asm_instance. CLUSTER_NAME is intentionally not projected by the common source contract. |

Except for recorded block corruption, these sources reject majors below 11.
Modern CON_ID is projected from 12 onward;
11g models have nil ContainerID. For dictionary views CON_ID is the connected
container from SYS_CONTEXT, not invented CDB-wide visibility. Dictionary rows
also retain SOURCE_INST_ID as ReportingInstanceID. Local tempfile OwningInstanceID
is independent and may be selected SQL NULL for shared tempfiles. GV rows retain
actual source CON_ID; they are never collapsed to the connected member. An empty
instance filter selects every visible row; named filters are bound, positive,
unique IDs with an explicit maximum 1000. Visibility is not physical RAC proof.

Raw values use `session.Value`, preserving SQL NULL, exact text and unknown flags.
Optional *Value nil means unselected, distinct from selected SQL NULL. Numeric
bytes and blocks are unrounded integers; INCREMENT_BY is Oracle blocks, not bytes.
SCNs use reviewed TO_CHAR(TM9) decimal text because documented values can exceed
int64. They remain JSON strings deliberately, never rounded floats. Other native
integer JSON columns use the shared rowset integer contract and fail on a value
it cannot represent exactly. No resultset CSV/plaintext serialization is added.

## Recorded block corruption

Use `V$DATABASE_BLOCK_CORRUPTION` to read ranges Oracle has already recorded.
Each row retains the absolute file number, starting block, length, detection
change number and Oracle's corruption type. A change number of zero is Oracle's
media corruption marker. File and block numbers, range length and the change
number use exact decimal text. `CON_ID` retains Oracle's actual value, including
0 for whole-CDB or non-CDB data, 1 for the root, a PDB ID or SQL NULL when
returned. On releases with five selected columns the field is unprojected.
The view is read in the caller's admitted scope. The API does not switch
containers, persist findings, merge ranges or validate datafiles. A completed
empty read establishes zero recorded ranges visible at capture time.

`BlockCorruptionsStatement`, `DecodeBlockCorruptions`, `ReadBlockCorruptions`,
`BlockCorruptionsRows` and `BlockCorruptionsJSON` take an observed numeric Oracle
version. `ReadBlockCorruptions` takes `BlockCorruptionsRequest{OracleVersion, Reference}`.
The facade exposes `Client.BlockCorruptions` and `Client.BlockCorruptionsJSON`.
Explicit-version methods require access to the fixed view. The facade also reads
the Oracle version from V$INSTANCE. Oracle errors propagate through the
caller-facing read API. An unsupported numeric version is rejected before the
corruption query runs.

Oracle Database Reference reviewed on 2026-11-24. Each row below has static
selector and decoder coverage. Native coverage is recorded separately.

| Release family | Oracle reference | Selected columns |
| --- | --- | --- |
| 9iR2 (9.2) | [9.2](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch339.htm) | Five base columns |
| 10gR1 (10.1) | [10.1](https://docs.oracle.com/cd/B14117_01/server.101/b10755/dynviews_1055.htm) | Five base columns |
| 10gR2 (10.2) | [10.2](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1074.htm) | Five base columns |
| 11gR1 (11.1) | [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_1087.htm) | Five base columns |
| 11gR2 (11.2) | [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_1098.htm) | Five base columns |
| 12cR1 (12.1) | [12.1](https://docs.oracle.com/cd/E57425_01/121/REFRN/GUID-6CA088AF-B7B0-4CA1-8DC6-95372368BACC.htm) | Base plus CON_ID |
| 12cR2 (12.2) | [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID |
| 18c | [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID |
| 19c | [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID |
| 21c | [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID |
| 23c/23ai | [23c reference redirects to 26ai](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID, selector tested with 23.4 |
| 26ai (observed 23.26) | [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE_BLOCK_CORRUPTION.html) | Base plus CON_ID |

The reviewed projection changes with the 12.1 multitenant metadata. No additional
patch-set or RU predicate is established for these columns. Earlier releases,
unreviewed numeric families and malformed version strings are rejected.

Native SQL and SDK/driver verification ran on 23.26.1.0.0 in CDB$ROOT as SYSDBA,
on a primary in READ WRITE and a physical standby in READ ONLY WITH APPLY.
Go 1.27.4 on Windows amd64 and go-oracle-session v0.23.0 with go-ora V2 ran
the SDK cursor, bounded JSON and typed reader. The queries matched independent
zero-row counts on both endpoints. A separate Oracle literal confirmed the
exact 19-digit change-number formatting. Native recorded ranges, MOUNT,
direct-PDB and non-CDB sessions, other logins and other numeric releases
remain unqualified. Nonempty range decoding is covered by fixtures.


## ASM disk-group capacity

`asm.DiskGroupCapacityStatement`, `DecodeDiskGroupCapacity`, `ReadDiskGroupCapacity`,
`DiskGroupCapacityRows` and `DiskGroupCapacityJSON` share one projection. The
`Client.ASMDiskGroupCapacity` and `Client.ASMDiskGroupCapacityJSON` methods use
the numeric version observed on the borrowed handle. The caller supplies
`asm.DatabaseInstance` or `asm.ASMInstance` from its admitted scope.

The read uses the documented local `V$ASM_DISKGROUP_STAT` view. Oracle documents
the same projected columns as `V$ASM_DISKGROUP`. The STAT view omits disk discovery
and can omit groups new to the connected instance. A database handle reports
groups visible to that instance (`database_visible_asm_disk_groups`). An ASM
handle reports groups known to the connected ASM instance
(`asm_instance_disk_groups`). Neither scope establishes RAC-wide or storage-wide
completeness. Group numbers can change on remount. The view is independent of
FRA quota and usage.

| Numeric release | Projection | Oracle documentation and static coverage | Native execution and SDK qualification |
| --- | --- | --- | --- |
| 9.2, 10.1 | Unsupported before SQL | STAT is introduced in [10.2](https://docs.oracle.com/cd/B19306_01/server.102/b14237/whatsnew.htm). Static tests check selection and absence of a read. | Unsupported |
| 10.2, 11.1, 11.2 | `GROUP_NUMBER, NAME, STATE, TYPE, TOTAL_MB, FREE_MB, REQUIRED_MIRROR_FREE_MB, USABLE_FILE_MB` | [10.2 STAT](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1022.htm) and [column definitions](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1021.htm). Both 11g references document the same fields. Static selection and decoder tests pass. | Unqualified |
| 12.1, 12.2, 18, 19, 21, 23/23ai, 26ai (numeric 23.26) | Same columns plus `CON_ID` | [12.1 columns](https://docs.oracle.com/database/121/REFRN/GUID-5CF77719-75BE-4312-84A3-49A7C6A20393.htm), [19c STAT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ASM_DISKGROUP_STAT.html), [26ai STAT](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ASM_DISKGROUP_STAT.html) and [26ai columns](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ASM_DISKGROUP.html). The 23 reference redirects to 26ai. Static selection and decoder tests pass. | Native Rows, bounded JSON and typed reads returned complete empty sets at 23.26.1.0.0 on two SYSDBA CDB roots. |

Values retain the documented Oracle MB units, exact decimal text and SQL NULL.
`USABLE_FILE_MB` can be negative. Unmounted groups can have zero group number
and zero total/free MB. With FLEX or EXTEND redundancy,
`REQUIRED_MIRROR_FREE_MB` and `USABLE_FILE_MB` can be zero when no calculation
is supplied. From 12.1 onward, Oracle documents `CON_ID = 0` for this view. The
decoder preserves SQL NULL separately and leaves the field absent on legacy
releases. An empty result records no groups visible through the selected
view. It cannot establish the storage type, the capacity or the absence of ASM.
See [Oracle's capacity guidance](https://docs.oracle.com/en/database/oracle/oracle-database/26/ostmg/capacity-diskgroups.html)
and [ASM dynamic views](https://docs.oracle.com/en/database/oracle/oracle-database/26/ostmg/views-asm-info.html).

The caller must already have access to the view on the admitted handle. Direct
ASM administration uses SYSASM on current Oracle releases and SYSDBA in the
[10.2 ASM documentation](https://docs.oracle.com/cd/B19306_01/server.102/b14231/storeman.htm).
Native qualification covered two RDBMS roots with Oracle 23.26.1.0.0,
one primary READ WRITE and one physical standby READ ONLY WITH APPLY, on
Windows amd64 with Go 1.27.4, go-oracle-session v0.25.0 and go-ora V2. Both
matched an independent count of zero. Legacy servers, ordinary-account access,
direct ASM, PDB and MOUNT access, positive capacity rows and RAC-wide visibility
remain unqualified. Fixture assertions cover populated values, NULLs, limits
and cursor failure.

## Time, totals and attribution limits

`ClockStatement`, `DecodeClock`, `ReadClock`, `ClockRows` and `ClockJSON` provide
one separately observed source-local `SYSDATE` label. `ClockResult.LocalTime`
retains exact `YYYY-MM-DDTHH:MM:SS` text, with `ReportingInstanceID` and optional
`ContainerID`. `ReadClock` accepts `Request{OracleMajor, Reference}` and rejects
nonempty instance filters because the source is connected-instance only. The
facade methods are `RedoClock` and `RedoClockJSON`. Empty, multiple, malformed or
SQL NULL clock rows are invalid evidence; JSON applies the same decoder.

Derive a history interval from this admitted civil label instead of a client UTC
clock. Parsing the fields for civil arithmetic does not establish a UTC instant
or offset. Retain the original source label and the application's independently
collected UTC observation separately. No session setting, timezone conversion,
shared clock-synchronization claim or connection ownership is introduced. The
label observes SYSDATE as configured, including a possible FIXED_DATE override;
it does not establish that the source clock is accurate. The
connection-free `examples/redo-clock-selectors` command emits only exact SQL
selection metadata as JSON for native verification.

History filters require explicit increasing `[From, Until)` source-local wall-clock
bounds, whole seconds, one location and at most 31 calendar days. Zone offsets are
not converted; the caller must know the source-local intended dates. Gregorian
EXTRACT fields from CAST(DATE AS TIMESTAMP) produce stable ISO-shaped local text
without depending on NLS_CALENDAR. Validated numeric date fields alone enter ANSI
DATE literals; subday offsets and instance IDs stay bound. No session NLS change
or UTC assumption occurs. Bounds apply to FIRST_TIME, not COMPLETION_TIME.

Control-file records can age out and contain repeated GV observations, destinations,
restores or archive copies. Preserve incarnation/thread/sequence/SCN and copy
identity before caller-owned deduplication. Blocks times block size describes
archive copy bytes, not exact historical generated redo. APPLIED is meaningful
in the documented standby/RFS context and is never normalized to a generic bool.
The switch-count/current-log-size formula is an estimate; missing days or rows
must retain missing coverage. No daily total, group allocation or version min/max
is calculated here.

File capacity, free extents, segment allocation and configured undo assignments
are different facts. Segment bytes are allocated space, not measured live data.
LOB/index/table/partition categories intentionally overlap. LONG column metadata
supports only attribution of whole containing segments, not LONG payload bytes;
several columns must not multiply the same segment. Owner/name matching can miss
clustered/nested storage. Applications own exclusions, stable top 50 selection,
exact arithmetic, category definitions and coverage-aware comparisons.

## References and validation boundary

Checked 2026-11-15 using Oracle-published documentation. These references establish
specific source semantics, not native testing across every version/RU/edition,
restricted-grant profile, root/PDB/non-CDB, mounted state or multi-node RAC. No MOS
content was authenticated for this change. Current identifiers are view/function
names; no old MOS Doc ID transition is asserted.

- Oracle 19c Database Reference: [V$LOG](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-LOG.html), [V$LOGFILE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-LOGFILE.html), [V$LOG_HISTORY](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-LOG_HISTORY.html), [V$ARCHIVED_LOG](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ARCHIVED_LOG.html).
- Oracle 11.2 Database Reference: [V$LOG](https://docs.oracle.com/cd/E18283_01/server.112/e17110/dynviews_2031.htm); Oracle 12.2 [V$LOG](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-LOG.html). Conservative major-only capability choices are intentional.
- Oracle 19c Database Reference: [DBA_TABLESPACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TABLESPACES.html), [DBA_DATA_FILES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_DATA_FILES.html), [DBA_TEMP_FILES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TEMP_FILES.html), [DBA_FREE_SPACE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_FREE_SPACE.html), [DBA_SEGMENTS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_SEGMENTS.html), [DBA_TAB_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TAB_COLUMNS.html), [DATABASE_PROPERTIES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DATABASE_PROPERTIES.html), [DBA_TABLESPACE_GROUPS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TABLESPACE_GROUPS.html).
- Oracle 19c [V$SGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SGASTAT.html) and [V$ASM_CLIENT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ASM_CLIENT.html); Oracle 26ai [V$SGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SGASTAT.html). The 26ai 23.26.1 ALLOCATION_COUNT column is deliberately outside this common projection.
- Oracle 19c [SYSDATE](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/SYSDATE.html) defines the read-only source; its returned DATE contains no timezone field.
- Oracle 19c SQL Reference: [EXTRACT(datetime)](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/EXTRACT-datetime.html) documents Gregorian numeric fields; [Literals](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/Literals.html) defines ANSI DATE literals.

Unit tests cover exact values/NULL/optional absence, version/source and instance
selection, finite intervals and rejected late rows, separate archive copies,
temporary-group membership, referenced complete readers, post-query cancellation,
actual database/sql cursor lifetime and atomic JSON next/close/cancel/capacity
failures. The fixture driver is unit scaffolding, not native Oracle qualification.
Calendar tests verify the selected SQL construction; they do not execute an Oracle
session with an alternate calendar. Native 11g+,19c EE and 26ai EE qualification,
including alternate calendars, remains a separate acceptance gate.

## Datafile storage hardware

`asm.DatafileStorageHardwareStatement`, `DecodeDatafileStorageHardware`,
`ReadDatafileStorageHardware`, `DatafileStorageHardwareRows` and
`DatafileStorageHardwareJSON` share the SQL below. The root client exposes the
native cursor through `Client.DatafileStorageHardware` and the bounded JSON
result through `Client.DatafileStorageHardwareJSON`.

```sql
SELECT CASE WHEN COUNT(*) > 0 THEN 'EXADATA_STORAGE' ELSE 'UNKNOWN' END AS HARDWARE_TYPE,
       COUNT(*) AS EXADATA_DISK_COUNT
FROM V$ASM_DISK_STAT d
JOIN V$ASM_DISKGROUP_STAT g ON g.GROUP_NUMBER = d.GROUP_NUMBER
WHERE d.GROUP_NUMBER > 0
  AND d.PATH LIKE 'o/%'
  AND EXISTS (
      SELECT 1
      FROM V$DATAFILE f
      WHERE UPPER(SUBSTR(f.NAME, 1, LENGTH(g.NAME) + 2)) = '+' || UPPER(g.NAME) || '/'
  );
```

Run in the standby database's CDB root or non-CDB using an admitted session
with SELECT access to the three views. Oracle documents [`o/` paths for
Exadata grid disks](https://docs.oracle.com/en/engineered-systems/exadata-database-machine/sagug/creating-oracle-asm-disk-groups.html)
and the [non-discovering disk STAT view](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ASM_DISK_STAT.html).
[`V$DATAFILE.NAME`](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATAFILE.html)
links the visible group name to a datafile recorded in the control file. The
probe requires a complete group-name match and never counts disks in another
group based on a common name prefix.

`EXADATA_STORAGE` means at least one grid-disk row was visible in a group
referenced by a datafile. `UNKNOWN` preserves a successful zero count. SQL,
permission, transport, and validation errors remain errors. This observation
covers the local database instance and its visible ASM datafile groups. It
cannot identify the database server model, the placement of file extents,
Exascale-only storage, or full RAC coverage. Oracle documents empty
`V$ASM_DISK_STAT` results from a PDB in [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-ASM_DISK_STAT.html)
and [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-ASM_DISK_STAT.html).
Use a CDB root connection for the probe.

Oracle references checked on 2026-11-24. Every admitted numeric release uses
the same projection. The selector does not infer a hardware release or RU.

| Numeric release | Oracle reference and static selection | Native SQL evidence |
| --- | --- | --- |
| 9.2, 10.1 | Reject before execution. [10.2 introduces the STAT views](https://docs.oracle.com/cd/B19306_01/server.102/b14237/whatsnew.htm). | Unsupported |
| 10.2 | Select common columns. [Disk STAT](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1020.htm), [group STAT](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1022.htm) | Unqualified |
| 11.1, 11.2 | Select common columns. [11.1 ASM guide](https://docs.oracle.com/cd/B28359_01/server.111/b31107.pdf), [11.2 ASM views](https://docs.oracle.com/cd/E18675_01/doc/server.112/e16102/asmviews.htm) | Unqualified |
| 12.1, 12.2 | Select common columns without optional container columns. [12.1 ASM views](https://docs.oracle.com/database/121/OSTMG/GUID-7CAB3FBA-5B76-4F30-A13F-C4C16BD59147.htm), [12.2 ASM views](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/ostmg/views-asm-info.html) | Unqualified |
| 18 | Select common columns. [18c disk STAT](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-ASM_DISK_STAT.html) | Unqualified |
| 19 | Select common columns. [19c ASM views](https://docs.oracle.com/en/database/oracle/oracle-database/19/ostmg/part-ii-oracle-asm-instances-and-disk-groups/using-views-to-display-oracle-asm-information.html) | 19.32.0.0.0 SYSDBA CDB root, READ WRITE primary, zero matches |
| 21 | Select common columns. [21c disk STAT](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-ASM_DISK_STAT.html) | Unqualified |
| 23/23ai | Select common columns. [23ai ASM guide](https://docs.oracle.com/en/database/oracle/oracle-database/23/ostmg/automatic-storage-management-administrators-guide.pdf) | Unqualified |
| 26ai (numeric 23.26) | Select common columns. [26ai ASM views](https://docs.oracle.com/en/database/oracle/oracle-database/26/ostmg/views-asm-info.html) | 23.26.1.0.0 SYSDBA CDB root, READ WRITE primary and READ ONLY WITH APPLY physical standby, zero matches |

Fixtures cover positive counts, exact count decoding, unknown, inconsistent
rows and cursor errors. An Oracle SQL fixture with synthetic disk and datafile
rows returned one match for two files in the same group and excluded disks
in other groups. The SQL*Plus runs above against the actual views returned
zero matches. The SDK's native rows and bounded JSON both returned the same
result on the 23.26.1.0.0 physical standby using go-oracle-session v0.25.0
with go-ora V2. Native positive Exadata, MOUNT, non-CDB, direct PDB,
restricted grants and multi-instance RAC remain unqualified.

## Source-clock SQL qualification

On 2026-11-15 the exact generated `ClockStatement` projections were fully fetched
as server-produced JSON through SQLPlus on the existing 11.2.0.2 legacy XE fixture,
19.32 Enterprise CDB root, and 23.26.1 Enterprise CDB root and mounted physical
standby. Each returned one non-NULL whole-second Gregorian civil label and the
actual reporting instance; the 11g SQL omitted CON_ID and modern SQL retained it.
Before/after database role and open-mode observations matched within each probe.
The two initially stopped fixtures were started separately and restored; the
shared running Enterprise instances were not stopped or reconfigured.

This is a source-SQL result, separate from fixture-based Go reader/native-cursor/
JSON tests. It does not qualify the Oracle Go driver, alternate-calendar native
sessions, PDB scopes, restricted grants, multi-node RAC, every RU/platform or the
other redo/storage datasets. Raw source labels and independent host collection
UTC observations remain separate in the private qualification evidence.

[Temporary tablespace free space](temp-free-space.md) is a separate observed
dataset. Tempfile allocation and growth limits remain file metadata.
