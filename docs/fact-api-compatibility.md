# Fact API compatibility and qualification

This is the issue #65 source matrix for the 56 new or completed families in
[fact APIs](fact-apis.md). Check date: **2026-11-18**. Existing public selectors
and their historical behavior remain available; a rejected new adapter is not
an assertion that the old SQL cannot execute on that release.

## Reading the per-query matrix

Each row names one family and its selected projection; all five paths share that
selection/schema/provenance. The following states keep documentation and static
checks distinct from native execution and driver qualification:

- **U**: new adapter rejects this numeric release before SQL. No execution or
  compatibility claim; older documentation was not used to infer support.
- **S**: retained legacy source admitted and statically checked. Release-specific
  documentation/native qualification remains incomplete.
- **DS**: source column descriptions reviewed in the corresponding Oracle
  19/21/23/26 reference and source/decoder checks pass. Native coverage is only
  the exact cases below. Documentation alone does not qualify login or scope.
- **C**: static capability-selected path, requiring the actual caller-admitted
  dictionary Schema. No automatic discovery/retry. Metadata catalogue columns
  are documented; standard reference URLs for cell views were unavailable.
  Their historical source contracts and admitted schemas remain separate
  evidence, not inferred release-wide availability.

The 10.1/10.2, 11.1/11.2 and 12.1/12.2 columns are deliberately separate. These
new major-only choices establish no patch-set/RU transition. On 11g oldest
statistics requires admitted owners; ORACLE_MAINTAINED is used only with explicit
capability evidence on applicable releases. Memory PDB scope requires 12+, and
its EffectiveADB variant additionally requires known18+, connected-PDB and a
caller-established primary NULL/no-row outcome. This is never an automatic
fallback in the new source API. Unknown-version VersionBootstrapOptions major0
is a separate explicit bootstrap: it reads VERSION, not VERSION_FULL/RU, and
validates the returned five-part numeric text. Known ordinary identities use
only19/21/23. Numeric26 is rejected; 26ai branding does not admit it.

| Fact family | 9.2 | 10.1 | 10.2 | 11.1 | 11.2 | 12.1 | 12.2 | 18 | 19 | 21 | 23/23ai | 26ai (23.26) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `alertlog.Counts` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `alertlog.Entries` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `backup.Jobs` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.CPUCount` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.CharacterSet` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.Configuration` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `storage.BlockCorruptions` | DS | DS | DS | DS | DS | DS | DS | DS | DS | DS | DS | DS |
| `storage.DatafileSize` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.EncryptionCounts` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.Endian` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.NLSInstance` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.PDBCount` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `memory.PGAAllocated` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.Products` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `memory.SGAAllocated` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `memory.SGAComponents` | U | U | U | S | S | S | S | S | S | S | S | S |
| `database.SessionParameters` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `database.SessionParametersWithDefaults` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `storage.FileNames` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `dataguard.Lag` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `dataguard.Status` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `engineeredsystem/catalog.Alerts` | U | U | U | U | U | C | C | C | C | C | C | C |
| `engineeredsystem/catalog.Cells` | U | U | U | U | U | C | C | C | C | C | C | C |
| `engineeredsystem/catalog.Configuration` | U | U | U | U | U | C | C | C | C | C | C | C |
| `engineeredsystem/catalog.ConnectedCells` | U | U | U | U | U | C | C | C | C | C | C | C |
| `engineeredsystem/catalog.Metadata` | U | U | U | U | U | C | C | C | C | C | C | C |
| `engineeredsystem/catalog.VisibleCellCount` | U | U | U | U | U | C | C | C | C | C | C | C |
| `identity.BannersSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.ConnectedArchitecture` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.ContainerSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.DatabaseIdentity` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.EditionSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.ExecutionIdentity` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.FullVersionSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.InstanceIdentity` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.NameSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.PDBOpenMode` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.RoleSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.Snapshot` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.VersionBootstrap` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `identity.VersionSource` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `memory.PDB` | U | U | U | U | U | S | S | S | DS | DS | DS | DS |
| `memory.RAC` | S | S | S | S | S | S | S | S | DS | DS | DS | DS |
| `memory.Scoped` | S | S | S | S | S | S | S | S | DS | DS | DS | DS |
| `objects.InvalidObjects` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.ObjectInventory` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.ObjectTotals` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.OldestStatistics` | U | U | U | S | S | S | S | S | DS | DS | DS | DS |
| `objects.Schema` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.Table` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.TableCount` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.TableStatistics` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.Tables` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `objects.TablesStatistics` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `performance.Blockers` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `performance.ResourceLimits` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `scheduler.Jobs` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |
| `services.Names` | U | U | U | U | U | U | U | U | DS | DS | DS | DS |

The four runtime dictionary sources added in #153 have an independent
[release and qualification matrix](upgrade-readiness.md#runtime-dictionary-evidence).
It records the 10.1 and 10.2 Scheduler boundaries, the recycle-bin floor and the
12c connected-container projection.

The three #154 families have a separate [release and qualification matrix](upgrade-readiness.md#object-identities-redo-threads-and-active-services).

The #155 dictionary observations have a separate [release and qualification matrix](upgrade-readiness.md#connected-dictionary-and-root-rollups).

#156 adds a separate [ASM disk-group capacity matrix](redo-storage-asm.md#asm-disk-group-capacity).

#190 adds a separate [partition and subpartition compatibility matrix](object-row-counts.md#partition-and-subpartition-compatibility).

#185 adds a separate [DBTIMEZONE release and scope matrix](upgrade-readiness.md#connected-database-time-zone).

#191 adds a separate [time-zone column release and scope matrix](upgrade-readiness.md#columns-with-time-zone-aware-timestamp-types).

#206 adds a separate [connected session clock matrix](session-clock.md#release-and-qualification-matrix).

## Source applicability

Each canonical file defines exact aliases and scalar kinds in its private frozen
selection. New APIs do not reuse legacy display expressions as database facts.
The [family map](implementation-coverage.md) identifies every compatibility
selector; [value contracts](fact-apis.md#compatibility-mappings-and-values)
record deliberate canonical source differences.

| Families | Selected source and relevant constraints |
| --- | --- |
| performance.Blockers / ResourceLimits | V$SESSION current valid blocking links / V$RESOURCE_LIMIT executing-instance current, peak-since-startup and raw limit text, including UNLIMITED; no historical workload or cluster total |
| alertlog.Entries / Counts | V$DIAG_ALERT_EXT current container; positive seconds window; entries bounded ordered sample with timestamp offsets, counts grouped by raw message type/level |
| backup.Jobs | V$RMAN_BACKUP_JOB_DETAILS control-file sample; DATE fields use the reviewed Gregorian expression, not timezone inference |
| scheduler.Jobs | DBA_SCHEDULER_JOBS exact owner; raw flags and timestamp-with-zone values; no job execution |
| database.Configuration / CPUCount / SessionParameters / SessionParametersWithDefaults | V$PARAMETER executing-session values/display/default flags, distinct from instance SYSTEM_PARAMETER2 and stored SPFILE |
| database.Endian / CharacterSet / NLSInstance | V$TRANSPORTABLE_PLATFORM+V$DATABASE / NLS_DATABASE_PARAMETERS / NLS_INSTANCE_PARAMETERS; not interchangeable NLS scopes |
| database.Products / EncryptionCounts / PDBCount | DBA_REGISTRY / DBA_TABLESPACES+DBA_ENCRYPTED_COLUMNS / V$PDBS. Non-seed PDB counts require admitted CDB root. |
| storage.BlockCorruptions | [V$DATABASE_BLOCK_CORRUPTION release and scope matrix](redo-storage-asm.md#recorded-block-corruption). The 23c reference redirects to 26ai. Native proof covers only the two 23.26.1.0.0 SYSDBA CDB root cases. |
| storage.DatafileSize / FileNames | DBA_DATA_FILES rounded allocation / admitted data+temp names and file-kind discriminators |
| memory.SGAAllocated / PGAAllocated | V$SGA / V$PGASTAT observed instance allocation rounded to three decimal GiB |
| memory.SGAComponents | Same GV$SGASTAT projection as the legacy storage API. CON_ID begins at 12, with the established reporting-instance filters and ordered binds. The documented 19c/26ai source and scope limits are in [the source guide](redo-storage-asm.md). |
| services.Names | V$SERVICES excluding the two internal names; not the richer registered-service projection |
| dataguard.Status / Lag | V$DATABASE role/protection/switchover / V$DATAGUARD_STATS raw name/value/unit/time_computed/datum_time; primary empty lag is valid, not standby qualification |
| objects.InvalidObjects / ObjectInventory / ObjectTotals | DBA_OBJECTS raw owner/type/status and exact count text; no report scoring |
| objects.Schema / Tables / Table / TablesStatistics / TableStatistics | DBA_USERS / DBA_TABLES; exact binary owner/table predicates; raw NUM_ROWS may be fractional optimizer statistics, never exact COUNT(*) |
| objects.TableCount / OldestStatistics | Explicit safely quoted admitted ordinary table COUNT(*) / DBA_TAB_STATISTICS bounded owner-admitted sample; no workload authorization inferred |
| identity.* | Reviewed V$DATABASE, V$INSTANCE, V$VERSION, V$PDBS and USERENV fields; VERSION_FULL versus VERSION bootstrap kept distinct; actual container/instance identity never inferred from branding |
| memory.Scoped / PDB / RAC | Existing GV$SYSTEM_PARAMETER, GV$INSTANCE/GV$PDBS and caller-admitted guarded V$PARAMETER sources; selected-source observation versus proven member totals distinct; no new fallback |
| catalog.Metadata / Configuration / VisibleCellCount / Cells / Alerts / ConnectedCells | SYS.ALL_TAB_COLUMNS and caller-admitted local/global cell/config/info/alert layouts; missing optional column, projected SQL NULL and absent source distinct. Complete UCS2 LOB length checked; scalar and XML layouts distinct; bounded connected sample stays a sample |

The relocated memory and storage entry points and their legacy adapters
use the same SQL, schema, version gates and caller-admitted scopes. Existing
native evidence applies only to the previously qualified combinations.

## Native SQL and driver evidence

The frozen production selection at 2f834626 was executed through unmodified
Oracle SQLcl26.3.1/JDBC on Oracle JDK25.0.5. Both existing profiles were freshly
observed as SYSTEM/current schema SYSTEM, PRIMARY, READ WRITE, CDB PDB CON_ID3:
**19.32.0.0.0** and **23.26.1.0.0** (26ai branding). Roles, NLS and actual dictionary
capabilities were captured before the fixed SELECTs. No SYS login, root switch,
DDL/DML, grants, users, tokens or configuration changes were used.

**116 selected SQL executions passed, 58 per profile**. This includes the actual
admitted cell configuration/info and scalar/XML capability choices, plus scoped
SGA/PGA with inheritance disabled/enabled and RAC memory source selection.
PDBCount was not executed: its required root scope was not admitted. TableCount
used only the explicitly admitted SYS.DUAL ordinary table. Where the SDK emits
binds, SQLcl used escaped typed literals; this qualifies SQL execution, not Go
bind behavior. Compiled bound SQL/args are captured; unit tests constrain selector and adapter behavior.

Native SQL success with an empty source preserves empty evidence; it does not
qualify nonempty engineered-system payloads, physical hardware, full LOB delivery,
complete RAC membership, historical coverage or standby/MOUNT/root visibility.
No engineered-system laboratory is required for this acceptance scope.
**No new Go API/Oracle-driver/application path was natively qualified.** Fixture
cursor, scan, NULL, close, cancellation and capacity checks are unit evidence.
All other RUs/releases/accounts/roles/scopes/variants remain unqualified. No
patch/RU discontinuity was established; do not generalize the two observed RUs.

Private evidence retains exact compiled SQL/bind/schema hashes, per-query 12-release
records, raw standard JSON frames, source identities and diagnostics separately.
It contains no copied profile credentials. This evidence is not a runtime query
registry and is not a new dependency for using the module.

## Oracle references checked

Oracle Database Reference19,21,23 and26 content was checked on 2026-11-18; private
receipts retain the exact URL/title/hash/date. For each link below, corresponding
/refrn/ pages in the other three reference sets were also checked. DBA dictionary
pages referring to ALL_* inherit their columns, not their visibility scope.
The NUM_ROWS* footnote in ALL_TABLES is a statistics qualifier, not a different
column name. The16 standard cell-view reference URL requests that failed are
recorded as unavailable documentation, never evidence the views do not exist.

- [V$SESSION](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SESSION.html)
- [V$RESOURCE_LIMIT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-RESOURCE_LIMIT.html)
- [V$DIAG_ALERT_EXT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DIAG_ALERT_EXT.html)
- [V$RMAN_BACKUP_JOB_DETAILS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-RMAN_BACKUP_JOB_DETAILS.html)
- [ALL_SCHEDULER_JOBS / DBA_SCHEDULER_JOBS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_SCHEDULER_JOBS.html)
- [V$DATABASE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE.html)
- [V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html)
- [V$VERSION](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-VERSION.html)
- [V$PARAMETER](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PARAMETER.html)
- [V$SYSTEM_PARAMETER](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER.html)
- [V$PDBS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PDBS.html)
- [V$DATAGUARD_STATS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATAGUARD_STATS.html)
- [V$SERVICES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SERVICES.html)
- [ALL_TABLES / DBA_TABLES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TABLES.html)
- [ALL_OBJECTS / DBA_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_OBJECTS.html)
- [ALL_TAB_STATISTICS / DBA_TAB_STATISTICS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TAB_STATISTICS.html)
- [ALL_TAB_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TAB_COLUMNS.html)
- [DBA_USERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_USERS.html)
- [DBA_DATA_FILES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_DATA_FILES.html)
- [DBA_TEMP_FILES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TEMP_FILES.html)
- [DBA_TABLESPACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TABLESPACES.html)
- [DBA_ENCRYPTED_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_ENCRYPTED_COLUMNS.html)
- [DBA_REGISTRY](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY.html)
- [V$SGA](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SGA.html)
- [V$PGASTAT](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PGASTAT.html)
- [V$TRANSPORTABLE_PLATFORM](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-TRANSPORTABLE_PLATFORM.html)
- [NLS_DATABASE_PARAMETERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/NLS_DATABASE_PARAMETERS.html)
- [NLS_INSTANCE_PARAMETERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/NLS_INSTANCE_PARAMETERS.html)

## SQL-only SCAN discovery

See the [SCAN release matrix](scan-discovery.md#release-checks) for the #196
parameter and listener source choices, isolated Oracle runs, and driver evidence.
## RAC runtime interconnects

See the [RAC network evidence matrix](rac-network-evidence.md#release-checks)
for the #220 selector, release boundaries and native qualification.

## Borrowed-sample connection identity (#221)

`identity.ReadBorrowedSampleIdentity` reads one connected-session identity from
`V$INSTANCE`, `V$DATABASE` and `SYS_CONTEXT('USERENV', ...)` on the admitted
connection. It requires access to both views and preserves native query errors.
It does not bootstrap a legacy version or select a different source. The session
SDK independently owns provider and profile qualification.

| Oracle release | Source applicability | Qualification |
| --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2, 12.1, 12.2 | Before the `VERSION_FULL` projection documented from 18c. The reader returns Oracle's native failure if dispatched. The SDK can decline before dispatch on an unqualified provider. | Native collection and driver reuse unqualified for this reader. |
| 18, 19, 21, 23c/23ai | `VERSION_FULL` is documented. The projection preserves session user, DBA status, container, database role, open mode and transport on the current lease. | Source and decoder fixtures only. Neither view access nor reusable cursor sampling is implied. |
| 26ai | Same source projection. The SDK independently matches the exact admitted profile. | Native consumer parity on 23.26.1.0.0, TCP, PRIMARY READ WRITE, CDB root, SYSTEM without SYSDBA. Windows amd64 retained V2 and Linux amd64 standalone OCI client 23.26.1.0.0 with godror v0.51.0 passed. Linux V2 declined before the identity probe, as expected for this provider profile. |

Oracle Database Reference and SQL Language Reference checked on 2026-11-27.
[12.2 V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-INSTANCE.html) ·
[18c V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-INSTANCE.html) ·
[19c V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html) ·
[21c V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-INSTANCE.html) ·
[23ai V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-INSTANCE.html) ·
[26ai V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html) ·
[26ai V$DATABASE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-DATABASE.html) ·
[26ai SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/SYS_CONTEXT.html).
