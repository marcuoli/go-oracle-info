# Redo, storage and ASM fact sources

These additive APIs observe admitted read-only sources. They do not connect,
change containers/session settings, grant access, retry or own a driver. Source
selection is a static compatibility contract, not native qualification. Callers
admit endpoint/container/edition, source privileges and reporting-member coverage.

## Functional API and ownership

`redo.Groups`, `Members`, `History` and `ArchivedLogs` each expose `Statement`,
`Decode`, `Read`, `Rows` and `JSON` companions (for example `ReadGroups` and
`GroupsStatement`). Storage provides the same companions for `Tablespaces`,
`Datafiles`, `Tempfiles`, `FreeExtents`, `Segments`, `LongColumns`,
`DefaultTemporary`, `TemporaryGroups` and `SGAComponents`. `asm.Clients` provides
its own companions. There is no query registry or connection/cache manager.

Functional readers accept the observed Oracle major, a valid caller-owned SQL
reference and any explicitly admitted filter/options. `Request`, `HistoryRequest`,
`TempfileRequest` and `SGARequest` document those inputs. The root non-owning
`Client` hides version lookup for native rows and JSON. Its methods include
`RedoGroups`, `RedoMembers`, `RedoHistory`, `ArchivedLogs`, `Tablespaces`,
`Datafiles`, `Tempfiles`, `FreeExtents`, `Segments`, `LongColumns`,
`DefaultTemporary`, `TemporaryGroups`, `SGAComponents` and `ASMClients`, each
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
| History | GV$LOG_HISTORY, qualified visible IDs and finite first-time interval | Record/stamp/thread/sequence/resetlogs/SCN/time; no retention proof. |
| Archive copies | GV$ARCHIVED_LOG, qualified visible IDs and finite first-time interval | Destination/name, record/stamp, thread/sequence, resetlogs ID/change/time, activation, archival thread, block counts/size, first/next/completion times and raw status/role/copy flags. |
| Tablespaces | DBA_TABLESPACES, current-container dictionary | Contents/status/retention/bigfile, block size, logging/management/encryption. No undo/owner exclusions. |
| Datafiles | DBA_DATA_FILES, current-container dictionary | File/relative identity, bytes/blocks/user bytes, maximum, autoextension/increment, availability/online status. Keystore/source access remains caller admission; no wallet operation occurs. |
| Tempfiles | DBA_TEMP_FILES, current-container dictionary | Common 11g file projection. `TempfileOptions.IncludeLocal` explicitly admits SHARED and owning INST_ID from 18+. It is not selected automatically. |
| Free extents | DBA_FREE_SPACE, current-container dictionary | Tablespace/file/block identity and raw bytes/blocks. Offline locally managed files may have missing extent rows; absence does not independently establish zero free bytes. |
| Segments | DBA_SEGMENTS, current-container dictionary | Owner/name/type/subtype/partition/tablespace, allocated bytes/blocks/extents and header identity. |
| LONG attribution | DBA_TAB_COLUMNS, current-container dictionary | Only exact DATA_TYPE LONG/LONG RAW, owner/table/column/ordinal; excludes hidden system-generated columns by source definition. |
| Default temporary | DATABASE_PROPERTIES, current-container dictionary | Exact DEFAULT_TEMP_TABLESPACE property, raw value/description, at most one property row. |
| Temporary groups | DBA_TABLESPACE_GROUPS, current-container dictionary | Group-to-tablespace membership; default temporary may identify a group. |
| SGA components | GV$SGASTAT, qualified visible reporting IDs | All matching NAME=buffer_cache OR POOL=shared pool rows, with the predicate grouped before instance admission. Raw pool/name/bytes remain separate. |
| ASM connections | GV$ASM_CLIENT, qualified visible reporting IDs | Group/instance/database/software/compatible/status. `Filter.SourceRole` must explicitly admit database_instance or asm_instance. CLUSTER_NAME is intentionally not projected by the common source contract. |

All sources reject majors below 11. Modern CON_ID is projected from 12 onward;
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

## Time, totals and attribution limits

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
- Oracle 19c SQL Reference: [EXTRACT(datetime)](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/EXTRACT-datetime.html) documents Gregorian numeric fields; [Literals](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/Literals.html) defines ANSI DATE literals.

Unit tests cover exact values/NULL/optional absence, version/source and instance
selection, finite intervals and rejected late rows, separate archive copies,
temporary-group membership, referenced complete readers, post-query cancellation,
actual database/sql cursor lifetime and atomic JSON next/close/cancel/capacity
failures. The fixture driver is unit scaffolding, not native Oracle qualification.
Calendar tests verify the selected SQL construction; they do not execute an Oracle
session with an alternate calendar. Native 11g+,19c EE and 26ai EE qualification,
including alternate calendars, remains a separate acceptance gate.
