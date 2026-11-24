# Complete source fact APIs

The [public Go API reference](api-reference.md) lists exact exported signatures,
public fields, and constants across all importable packages. This inventory
identifies the recommended fact and Client entry points. The
[common pattern](implementation-pattern.md) defines ownership and completion;
[compatibility evidence](fact-api-compatibility.md) separates documentation,
static selection/decoding, native SQL and driver/application qualification.

## Calling a fact

Ordinary new families expose XStatement, DecodeX, ReadX, XRows and XJSON.
XOptions carries admitted choices; XRequest adds a valid caller-owned SQL
reference. XResult contains typed Rows and provenance. session.Value.Text is
exact source text; Null is SQL NULL. Optional pointers distinguish an unadmitted
column/layout. No flags, units, defaults, roles or visibility become policy.

~~~go
options := performance.BlockersOptions{OracleMajor: observedMajor}
facts, err := performance.ReadBlockers(ctx, admittedReader,
    performance.BlockersRequest{Options: options, Reference: callerRef})
// Check err before facts; the reader owns finite budgets and complete EOF/close.
rows, err := performance.BlockersRows(ctx, admittedNativeHandle, options)
// Check err; caller owns Next/Scan/Err/Close, deadlines, affinity and budgets.
data, err := performance.BlockersJSON(ctx, admittedNativeHandle, options,
    rowset.Limits{MaxRows: 10000, MaxBytes: 8 << 20})
// Check err before publishing the bounded standard JSON.
~~~

For a bound selection, `XStatement(options)` returns `(string, []any, error)`.
Some fixed selections, including `containers.InventoryStatement`, return
`(string, error)`. Decode takes a complete `session.Result` followed by the
selection. Read takes `(ctx, reader, request)`. Rows takes `(ctx, queryer, options)`
and returns `(*sql.Rows, error)`. JSON takes the same inputs followed by
`rowset.Limits` and returns `(json.RawMessage, error)`. Types and exact signatures
for each family are in its linked reference section.

The caller supplies SQL references and numeric version/capability evidence.
No connection, credential/driver selection, destination/container change,
capability cache, retry or report/scoring policy is introduced. Native rows are
the actual caller cursor. JSON independently consumes/finalizes and validates
its cursor, returning no bytes on any failure. Its budget includes the final
provenance envelope; exact-boundary/one-byte-short regressions constrain it.

Pure Decode leaves Provenance.Complete false. Successful owned reads/JSON mark
the selected transport complete, without certifying physical RAC membership,
all-container/history coverage or an atomic multi-query snapshot. A bounded
sample remains a sample even when it exactly fills its SQL limit.

## Families

Each ordinary family has Statement, Decode, Read, Rows and JSON operations.
Use the named forms in this table. Prefer an options-based selector or
`WithOptions` form when documented and available. Each package's reference
lists both forms and preserves the exact filter, version, and reference types.

| Package | Canonical fact families |
| --- | --- |
| [alertlog](api-reference.md#alertlog) | `Counts`, `Entries` |
| [asm](api-reference.md#asm) | `Clients`, [`DiskGroupCapacity`](redo-storage-asm.md#asm-disk-group-capacity) |
| [backup](api-reference.md#backup) | `Jobs`, `RestorePoints` |
| [containers](api-reference.md#containers) | `InvalidObjectCounts`, `Inventory`, `LegacyAuditTablePlacement`, [`PlugInViolations`](upgrade-readiness.md#recorded-pdb-plug-in-violations), `RegistryComponents`, `ResourceSamples`, `ResourceUsage`, `SQLPatchHistory`, `SavedStates` |
| [database](api-reference.md#database) | `CPUCount`, `CharacterSet`, `Configuration`, `Encryption`, `EncryptionCounts`, `Endian`, `NLSDatabase`, `NLSInstance`, `Options`, `PDBCount`, `Products`, `Properties`, `SessionParameters`, `SessionParametersWithDefaults`, `TimeZoneFiles` |
| [dataguard](api-reference.md#dataguard) | `ArchiveDestinationStatus`, `ArchiveDestinations`, `ArchiveGaps`, `ArchiveProgress`, `DatabaseContext`, `DatabaseIncarnations`, `Lag`, `Processes`, `RecoveryProgress`, `StandbyLogs`, `StandbyPromotionSCN`, `Status` |
| [dblinks](api-reference.md#dblinks) | `Inventory` |
| [directories](api-reference.md#directories) | `Inventory` |
| [engineeredsystem/catalog](api-reference.md#engineeredsystemcatalog) | `Alerts`, `Cells`, `Configuration`, `ConnectedCells`, `Metadata`, `VisibleCellCount` |
| [goldengate](api-reference.md#goldengate) | `Administrators`, `ApplyCoordinators`, `Capture`, `Extracts`, `HeartbeatCatalog`, `HeartbeatLag`, `Heartbeats`, `Replicats` |
| [grants](api-reference.md#grants) | `ColumnPrivilegeGrants`, `Direct`, `DirectColumn`, `DirectSystem`, `ObjectPrivilegeGrants`, `Role`, `RoleCatalog`, `RoleMemberships` |
| [identity](api-reference.md#identity) | `BannersSource`, `ConnectedArchitecture`, `ContainerSource`, `DatabaseIdentity`, `DatabaseInfo`, `EditionSource`, `ExecutionIdentity`, `FullVersionSource`, `GlobalName`, `InstanceIdentity`, `LogicalDatabase`, `NameSource`, `OracleHome`, `PDBOpenMode`, `RoleSource`, `SessionContext`, `Snapshot`, `VersionBootstrap`, `VersionSource` |
| [instances](api-reference.md#instances) | `Inventory`, `Versions`, `Visibility` |
| [memory](api-reference.md#memory) | `PDB`, `PGAAllocated`, `Parameter`, `RAC`, `SGAAllocated`, `SGAComponents` |
| [metadata](api-reference.md#metadata) | `Object` |
| [networkacl](api-reference.md#networkacl) | `Inventory` |
| [objects](api-reference.md#objects) | `InvalidDetails`, `InvalidObjects`, `MaterializedViews`, `ObjectIdentities`, `ObjectInventory`, `ObjectStatus`, `ObjectTotals`, `OldestStatistics`, `RecycleBinCounts`, `Schema`, `Table`, `TableCount`, `TableStatistics`, `Tables`, `TablesStatistics` |
| [parameters](api-reference.md#parameters) | `ContainerSystem`, `Effective`, `Hidden`, `HiddenCatalog`, `HiddenContext`, `SPFile`, `ScalarSystem`, `System` |
| [passwordfile](api-reference.md#passwordfile) | `Info` |
| [patches](api-reference.md#patches) | `History`, `SQLPatch` |
| [performance](api-reference.md#performance) | `Blockers`, `InDoubtTransactions`, `ResourceLimits` |
| [redo](api-reference.md#redo) | `ArchivedLogs`, `Clock`, `Groups`, `History`, `Members`, `Threads` |
| [registry](api-reference.md#registry) | `Components` |
| [scheduler](api-reference.md#scheduler) | `Jobs`, `RunningJobs` |
| [sequences](api-reference.md#sequences) | `Inventory` |
| [services](api-reference.md#services) | `ActiveServices`, `Definitions`, `Names` |
| [sqlplan](api-reference.md#sqlplan) | `Baseline`, `Profile` |
| [storage](api-reference.md#storage) | `BlockCorruptions`, `DatabaseFiles`, `DatafileSize`, `Datafiles`, `DefaultTemporary`, `FileBackupModes`, `FileHeaders`, `FileNames`, `FileRecovery`, `FileStatuses`, `FreeExtents`, `LongColumns`, `RecoveryArea`, `RecoveryAreaUsage`, `Segments`, `Tablespaces`, `TempFreeSpace`, `Tempfiles`, `TemporaryGroups` |
| [triggers](api-reference.md#triggers) | `Inventory` |
| [users](api-reference.md#users) | `AccountCount`, `Inventory`, `TableMetadata`, `TablespaceQuotas` |

### Facade, pure APIs and procedural sources

- [`oracleinfo.Client`](api-reference.md#oracleinfo) provides non-owning native
  queries, bounded JSON and scalar or typed observations. See the method pairs
  below.
- [`engineeredsystem`](api-reference.md#engineeredsystem) models and decodes
  database-visible payloads. Its query operations are in `engineeredsystem/catalog`.
- [`releases`](api-reference.md#releases) loads offline release catalogs and
  decodes reviewed patch metadata. The account and role lookups in `users` and
  `grants`, parameter persistence, patch consistency and redo time calculations
  are also pure operations over supplied evidence or offline data.
- `identity.ReadOracleHome` is the typed property and can try several sources.
  The native Oracle Home cursor and its JSON companion use the selected
  single SQL projection. `OracleHomeEnvironmentStatement` uses procedural
  OUT binds. See [Oracle Home source contracts](identity-properties.md).

## Canonical Client methods

Use `X(ctx, filter)` for a native cursor and `XJSON(ctx, filter, limits...)` for
its bounded JSON companion. Omit the filter when the operation takes none.
Where `X` already has a separate typed or legacy contract, use `XRows`
and `XRowsJSON`. The caller owns the native cursor. JSON opens and finalizes
its own cursor. The original methods below continue to accept their original
arguments.

| Canonical cursor | Canonical JSON | Compatibility methods |
| --- | --- | --- |
| `InvalidObjectsRows(ctx, owners []string)` | `InvalidObjectsRowsJSON(ctx, owners []string, limits ...JSONLimits)` | `InvalidObjects(ctx, owners ...string)`, `InvalidObjectsJSON(ctx, owners []string, limits ...JSONLimits)` |
| `GoldenGateExtracts(ctx, f GoldenGateProcessFilter)` | `GoldenGateExtractsJSON(ctx, f GoldenGateProcessFilter, limits ...JSONLimits)` | `GoldenGateExtractsRows` |
| `GoldenGateReplicats(ctx, f GoldenGateProcessFilter)` | `GoldenGateReplicatsJSON(ctx, f GoldenGateProcessFilter, limits ...JSONLimits)` | `GoldenGateReplicatsRows` |
| `GoldenGateHeartbeats(ctx, o goldengate.HeartbeatsOptions)` | `GoldenGateHeartbeatsJSON(ctx, o goldengate.HeartbeatsOptions, limits ...JSONLimits)` | `GoldenGateHeartbeatsRows` |
| `GoldenGateHeartbeatLag(ctx, o goldengate.HeartbeatLagOptions)` | `GoldenGateHeartbeatLagJSON(ctx, o goldengate.HeartbeatLagOptions, limits ...JSONLimits)` | `GoldenGateHeartbeatLagRows` |
| `GoldenGateAdministrators(ctx)` | `GoldenGateAdministratorsJSON(ctx, limits ...JSONLimits)` | `GoldenGateAdministratorsRows` |
| `GoldenGateHeartbeatCatalogRows(ctx, schema string)` | `GoldenGateHeartbeatCatalogRowsJSON(ctx, schema string, limits ...JSONLimits)` | `GoldenGateHeartbeatCatalogJSON` |
| `OracleHomeRows(ctx)` | `OracleHomeRowsJSON(ctx, limits ...JSONLimits)` | `OracleHome` and `OracleHomeJSON` are the separate typed observation pair. |

~~~go
owners := []string{"APP"}
rows, err := client.InvalidObjectsRows(ctx, owners)
// Check err, then iterate and close rows.
data, err := client.InvalidObjectsRowsJSON(ctx, owners, oracleinfo.JSONLimits{MaxRows: 1000})
~~~

## Compatibility aliases

The named families below select the same sources as the generic
`Statement`, `Decode`, `Read`, `Rows` and `JSON` functions from prior releases.
For example, `directories.Statement` corresponds to
`directories.InventoryStatement`. The original signatures remain supported.

| Package | Named family |
| --- | --- |
| dblinks, directories, networkacl, sequences, triggers, users | Inventory |
| instances | Inventory |
| memory | Parameter |
| metadata | Object |
| passwordfile | Info |
| registry | Components |
| services | Definitions |
| identity | OracleHome, including OracleHomeJSON for the native projection |

The former `database.SGAAllocated`, `database.PGAAllocated`,
`database.DatafileSize`, `database.Storage` and `storage.SGAComponents`
families delegate to `memory.SGAAllocated`, `memory.PGAAllocated`,
`storage.DatafileSize`, `storage.FileNames` and `memory.SGAComponents`,
respectively. Their original types and errors remain at the old imports.
The [selector dispositions](implementation-coverage.md#compatibility-selector-dispositions)
map older SQL constants and property projections to their related facts. In
`grants`, `Read…ForRelease` retains the compatibility request. Use
`Read…WithOptions` with `ClassifiedRequest` for the same options accepted by the
selector, decoder, native cursor and JSON.

Separate from dictionary reads, the [legacy predefined account catalog](legacy-predefined-accounts.md) lists documented Oracle-supplied usernames by release.

Existing families with multiple positional source selectors expose
`StatementWithOptions`, `Decode…WithOptions`, `RowsWithOptions` and
`JSONWithOptions` forms. This covers ASM clients, encryption, Data Guard
transport facts, configured PDB memory, invalid details and oldest statistics,
parameter facts, redo inventories and SGA components. `ReadRACWithOptions`
takes a request containing the RAC selection and SQL reference.

OldestStatistics reuses the existing OldestStatisticsStatement and
StatisticsOptions. DecodeOldestStatistics/ReadOldestStatistics and
OldestStatisticsRows/OldestStatisticsJSON preserve the nullable Statistics model,
including sql.NullString and optional Scope. Owners remain variadic; JSON places
Limits before that list. The result remains a bounded sample with the original
admitted ORACLE_MAINTAINED capability and owner requirements.

Memory completes Statement/Decode with Read/Rows/JSON; PDBStatement/DecodePDB
with ReadPDB/PDBRows/PDBJSON; and RACStatement/DecodeRAC with
ReadRAC/RACRows/RACJSON. SourceRequest contains Parameter/Options/Reference.
PDB native methods require complete connected-PDB Options; discovered targets
use scoped forms and the exact named bind. Read selects one source with no
fallback. ReadRAC retains CollectRAC's independently valid metrics alongside
errors; JSON returns no bytes on decoding/transport errors. Collect/CollectRAC
are unchanged. Effective ADB selection retains the caller-established primary
NULL/no-row and known18+ connected-PDB preconditions.

The existing database.NLSDatabase family already provides all five paths and
remains the canonical full API for NLSDatabaseSQL.

## Compatibility mappings and values

Compatibility SQL/constants remain byte-identical. New canonical facts remove
legacy presentation/report selection where necessary:

- Configuration returns raw cluster_database text, including NULL, rather than
  RAC/Single Instance display labels.
- Products returns all visible registered component IDs. The old six-component
  allowlist belongs to reports; registration is not licensing evidence.
- `storage.FileNames` returns data/temp file names and source kinds. Consumers format
  classifications from raw facts; unknown names remain NULL.
- DatafileSize, SGAAllocated and PGAAllocated explicitly retain observed,
  source-rounded GiB text (three-decimal SQL rounding), not exact raw bytes or
  configured targets. Empty/missing observations never become zero.
- SessionParameters uses executing-session display/raw values;
  SessionParametersWithDefaults retains raw VALUE/ISDEFAULT. Neither is an
  instance-wide SYSTEM_PARAMETER2 or stored SPFILE observation.
- ObjectInventory, ObjectTotals, TableCount, discovery and statistics preserve
  distinct counts/estimates/filters. Exact owner/table predicates use binary
  comparison. Quoting/hint bounds do not authorize tables, workload or cutoff.
- Identity Source/Identity datasets supplement existing scalar/multi-query
  properties. VersionBootstrap admits unknown zero and reads legacy VERSION,
  never full-version/RU precision. Existing property fallback, attempts and
  independently valid evidence remain unchanged.

- [LogicalDatabase](logical-database-identity.md) reads the control-file DBID and name on reviewed releases from 9.2, with the CDB flag from 12.1.

## Engineered-source capabilities

MetadataResult.CapabilitySchema creates a fresh validated snapshot without I/O
or caching. Callers explicitly supply it to Cells/Configuration/Alerts. No fact
automatically discovers capabilities. Unsupported source choices fail before
SQL. Existing static global/local capability selection remains; a failed query
never triggers another source or an AWR substitute.

Cells retains the legacy fixed raw projection, including synthetic NULL CELL_TYPE
when the dictionary column is absent. OptionalColumns records actual source
column admission: typed Type=nil means unavailable; nonnil Null means selected
source NULL. Raw JSON retains the native projection plus that provenance.
ConnectedCells genuinely omits the unadmitted column and requires either
IncludeType or CellTypeKnownAbsent. Its limit remains a sample bound.

Alerts explicitly retains scalar/XML layouts and nil unselected fields.
Configuration/XML alerts validate complete native lengths in UCS2 units
(supplementary Unicode uses two) before owned publication. Raw malformed XML
remains evidence; normalization/acceptance stays in the existing pure model.
These source APIs require no physical hardware lab. Unperformed nonempty native
and driver combinations remain unqualified.

OracleHomeEnvironmentStatement is procedural OUT-bind SQL, not a query cursor.
Its existing admitted OracleHome property/JSON route remains; no fabricated Rows
API is added. Pure models/evaluators/expression/quoting helpers are not SQL-only
fact gaps and acquire no artificial query API.

See [GoldenGate database discovery](goldengate-discovery.md) for separate process identity, verified accessible heartbeat sources and exact lag/freshness evidence (#72).
