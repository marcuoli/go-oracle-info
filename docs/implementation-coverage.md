# Implementation coverage and migration map

Use the [common implementation pattern](implementation-pattern.md) for all new
facts and changes. The initial assessment used baseline
`9cfbed11d37ddc006b1669e2949ec85d81abf95b`. The user then explicitly selected
full API coverage. Issue #65 now adds or completes **56 source fact families**,
preserving published signatures and compatibility SQL. See the exact
[API inventory](fact-apis.md) and [per-query release matrix](fact-api-compatibility.md).

## Why performance and redo look different

The old `performance/statements.go` exposed only SQL constants; `redo/groups.go`
already provided selection, decoding, buffered Read, native Rows and atomic JSON.
Performance now has complete Blockers and ResourceLimits families following the
same ownership and completion contract. Compatibility constants remain callable.
All other SQL-only facts now have the same five paths, with the explicit pure
and procedural exceptions below.

One contract does not require artificial I/O for pure models, renaming published
packages, or identical private filenames. Existing scalar/property, aggregation,
payload and catalog contracts have different responsibilities. The shared rules
are ownership, deterministic selection, exact values, complete owned transport,
error/cancellation handling, provenance and meaningful behavioral tests.

## All-package assessment

The original baseline contained 263 Go files: 156 implementation/example/helper files and
107 tests, across 39 Go packages; validation configuration is a fortieth area.
The private per-file assessment ledger records content identities and the exact
prior reviews reused for unchanged bytes. Current contract tracing supplements
those reviews; this is not a claim that every file was freshly reread. Inventory
and successful checks alone are not source review. Native qualification remains
separate from source review and fixture coverage.

| Area | Contract assessed | Disposition |
| --- | --- | --- |
| root `oracleinfo` | Non-owning native facade and identity bootstrap bridge | Thin wiring; use shared projection conversion |
| `alertlog` | Bound-window entries sample and grouped counts | Full Entries/Counts families; bounds, offsets and sample retained |
| `asm` | Client facts with member filtering | Full fact pipeline; retain domain integer/member checks |
| `backup` | Bounded RMAN control-file job sample | Full Jobs family; bounded sample and DATE meaning retained |
| `database` | NLS/options/encryption facts plus compatibility SQL | Full typed and compatibility-source families; mappings below |
| `dataguard` | Eight admitted datasets, snapshot/latest pure helpers, old status/lag SQL | Full pipelines including Status/Lag plus pure helpers |
| `dblinks` | Admitted dictionary inventory | Full fact pipeline; no remote connection |
| `directories` | Directory dictionary inventory | Full fact pipeline; no filesystem access |
| `engineeredsystem` | Payload model, complete-result decoders, XML normalization | Pure model/decoder; preserve independent valid facts and rejected payload evidence |
| `engineeredsystem/catalog` | Capability-selected SQL/schema catalog | Complete capability-selected datasets; no automatic discovery/collection |
| `goldengate` | Capture/apply/administrator facts and database properties | Full fact pipeline; configured GGSCHEMA remains explicitly unavailable from admitted DB source |
| `grants` | Direct/object/column/role grant facts | Full fact pipeline; raw grants do not imply effective access |
| `identity` | Scalar/bootstrap properties and complete database/session datasets | Complete source datasets plus preserved bootstrap/property contracts |
| `instances` | Qualified visible-member context | Full fact pipeline; visibility is not physical RAC completeness |
| `memory` | Admitted configured-memory collection and RAC aggregation | Complete scoped/PDB/RAC source adapters plus unchanged aggregate/property contracts |
| `metadata` | Explicit object DDL/CLOB capture | Full fact pipeline; shared conversion plus local complete CLOB validation |
| `networkacl` | ACL dictionary facts | Full fact pipeline; no network probes |
| `objects` | Invalid details, statistics decoder and compatibility selectors | Complete compatibility-source families plus invalid details |
| `parameters` | Effective/list/system/SPFILE/hidden facts and persistence evaluation | Full pipelines plus opt-in pure comparison; preserve source distinctions |
| `passwordfile` | Password-file metadata only | Complete observation/native/JSON contract; no users/verifiers/files |
| `patches` | Patch history facts and pure release-level evaluation | Full pipeline plus pure evaluation; unknown/parity limits preserved |
| `performance` | Current blocker and instance resource-limit SQL | Full Blockers/ResourceLimits families; no workload/history inference |
| `redo` | Redo groups/members/archive facts and pure clock helpers | Full fact pipelines; retain exact SCN, DATE and reporter checks |
| `registry` | Registered component facts | Full fact pipeline; no licensing inference |
| `scheduler` | Exact-owner job metadata SQL | Full exact-owner Jobs family; no job execution |
| `sequences` | Sequence dictionary facts | Full fact pipeline; observed metadata is not a future value guarantee |
| `services` | Registered service details and legacy name selector | Full pipeline; retain name-selector source/filter distinction |
| `sqlplan` | SQL profile and baseline metadata | Full fact pipelines; no optimizer changes |
| `storage` | Scoped files/allocation/segments/tablespaces/SGA facts | Full pipelines plus pure properties; retain exact units and instance/container identity |
| `triggers` | Trigger dictionary metadata | Full fact pipeline; no trigger firing or alteration |
| `users` | Account/table metadata and counts | Full fact pipelines; preserve raw flags and absence |
| `internal/datetime` | Reviewed static Gregorian DATE SQL expression | Pure SQL-expression helper; unchanged |
| `internal/projection` | Complete shape/scalar checks and rowset conversion | Shared mechanism; domain admission stays local |
| `internal/testsql` | Test-only database/sql fixtures | Unit scaffolding; never native Oracle qualification |
| `examples/hidden-catalog-selectors` | Opt-in fixed-table caller usage | Caller owns admission and execution |
| `examples/hidden-context-selectors` | Explicit hidden context usage | Caller owns admission and execution |
| `examples/identity-properties` | Scalar property usage | Caller owns reader/references and scope |
| `examples/queries` | Static selector usage | Demonstrates compatibility selectors, not a second execution layer |
| `examples/redo-clock-selectors` | Clock/source selection usage | Preserve local clock and timezone admission |
| `scripts`, `go.mod`, `go.sum` | Configured validation and released graph | Preserve selected compiler/dependencies; no new build framework |

No first-party implementation or test area is excluded from the assessment.
Dependency/tool caches, private inputs, artifacts and native Oracle deployments
are outside source-review scope. Native SQL-only evidence and remaining deployment gaps are recorded in the
release matrix; runtime and source-review coverage remain independent.

## Uniform file layout

Issue #76 applies the common layout across the 41 Go packages at baseline
`ac6b5b615bf15266fe2c15b5e523b842c791b219`. The 125 complete named families
below each keep their Statement, Decode, Read, Rows and JSON APIs in one
file. Options-based forms live with their original entry points. Shared
contracts live in `contract.go` when used across facts.

| Package | Families | Fact files |
| --- | ---: | --- |
| `alertlog` | 2 | `entries.go`, `counts.go` |
| `asm` | 1 | `clients.go` |
| `backup` | 1 | `jobs.go` |
| `database` | 17 | `configuration.go`, `character_set.go`, `cpu_count.go`, `datafile_size.go`, `encryption_counts.go`, `encryption.go`, `endian.go`, `nls_database.go`, `nls_instance.go`, `options.go`, `pdb_count.go`, `products.go`, `pga_allocated.go`, `session_parameters.go`, `sga_allocated.go`, `storage.go`, `session_parameters_with_defaults.go` |
| `dataguard` | 10 | `archive_destination_status.go`, `archive_destinations.go`, `archive_progress.go`, `archive_gaps.go`, `database_context.go`, `status.go`, `processes.go`, `standby_logs.go`, `recovery_progress.go`, `lag.go` |
| `dblinks` | 1 | `inventory.go` |
| `directories` | 1 | `inventory.go` |
| `engineeredsystem/catalog` | 6 | `cells.go`, `configuration.go`, `alerts.go`, `connected_cells.go`, `visible_cell_count.go`, `metadata.go` |
| `goldengate` | 8 | `replicats.go`, `apply_coordinators.go`, `capture_processes.go`, `heartbeat_lag.go`, `administrators.go`, `extracts.go`, `heartbeats.go`, `heartbeat_catalog.go` |
| `grants` | 7 | `direct.go`, `direct_column.go`, `role.go`, `direct_system.go`, `object_privilege_grants.go`, `column_privilege_grants.go`, `role_memberships.go` |
| `identity` | 17 | `banners_source.go`, `connected_architecture.go`, `container_source.go`, `database_info.go`, `database_identity.go`, `edition_source.go`, `execution_identity.go`, `full_version_source.go`, `instance_identity.go`, `name_source.go`, `oracle_home.go`, `pdb_open_mode.go`, `version_source.go`, `version_bootstrap.go`, `role_source.go`, `snapshot.go`, `session_context.go` |
| `instances` | 2 | `inventory.go`, `versions.go` |
| `memory` | 3 | `parameter.go`, `pdb.go`, `rac.go` |
| `metadata` | 1 | `metadata.go` |
| `networkacl` | 1 | `inventory.go` |
| `objects` | 11 | `invalid_objects.go`, `invalid_details.go`, `object_inventory.go`, `object_totals.go`, `schema.go`, `oldest_statistics.go`, `tables_statistics.go`, `table.go`, `tables.go`, `table_count.go`, `table_statistics.go` |
| `parameters` | 6 | `system.go`, `spfile.go`, `hidden_catalog.go`, `effective.go`, `hidden_context.go`, `hidden.go` |
| `passwordfile` | 1 | `info.go` |
| `patches` | 2 | `sql_patch.go`, `history.go` |
| `performance` | 2 | `resource_limits.go`, `blockers.go` |
| `redo` | 5 | `archived_logs.go`, `clock.go`, `groups.go`, `members.go`, `history.go` |
| `registry` | 1 | `components.go` |
| `scheduler` | 1 | `jobs.go` |
| `sequences` | 1 | `inventory.go` |
| `services` | 2 | `names.go`, `definitions.go` |
| `sqlplan` | 2 | `baseline.go`, `profile.go` |
| `storage` | 11 | `default_temporary.go`, `database_files.go`, `data_files.go`, `long_columns.go`, `segments.go`, `free_extents.go`, `sga_components.go`, `temporary_groups.go`, `tablespaces.go`, `temp_files.go`, `temp_free_space.go` |
| `triggers` | 1 | `inventory.go` |
| `users` | 4 | `table_metadata.go`, `tablespace_quotas.go`, `inventory.go`, `account_count.go` |

The 12 other audited packages have different responsibilities. The root
`oracleinfo` package is the non-owning client facade. `engineeredsystem`
models and normalizes admitted payloads. Its SQL catalog is the separate
subpackage above. `internal/datetime`, `internal/numeric`,
`internal/projection` and `internal/testsql` remain focused helper
packages. The six `examples` packages retain their executable `main.go` files.

Compatibility selectors in `identity` (Banners, Database, Edition, Execution,
Instance, Name, OracleHomeEnvironment, Version and Role) and `objects`
(Invalid, Inventory and Count) retain their original contracts. They live
with their related source facts or, for standalone edition/version lookups,
in the corresponding property file. The [API inventory](fact-apis.md) maps
compatibility names to their complete source families. Pure, procedural and
compatibility-only operations keep their existing contracts.

## Compatibility selector dispositions

Every ordinary SQL-only surface is mapped below. The new canonical fact can
intentionally have cleaner raw source semantics; related display projections
remain compatible and are not silently substituted. Each named ordinary family
provides Statement, Decode, Read, Rows and JSON as listed in [fact APIs](fact-apis.md).

| Compatibility surface | Complete canonical family / retained distinction |
| --- | --- |
| performance.BlockersSQL / ResourceLimitsSQL | Blockers / ResourceLimits |
| alertlog.EntriesSQL / CountsSQL | Entries / Counts; bounded entries versus grouped counts |
| backup.JobsSQL / scheduler.JobsSQL | Each package Jobs; sample versus exact owner |
| dataguard.StatusSQL / LagSQL | Status / Lag; protection and lag observations separate |
| database.ConfigurationSQL / EndianSQL / CharacterSetSQL / CPUCountSQL | Configuration / Endian / CharacterSet / CPUCount; raw configuration replaces no legacy labels |
| database.ProductsSQL / DatafileSizeSQL / StorageSQL / EncryptionSQL | Products / DatafileSize / Storage / EncryptionCounts; report allowlists/classifiers remain consumer-owned |
| database.SGAAllocatedSQL / PGAAllocatedSQL / PDBCountSQL | SGAAllocated / PGAAllocated / PDBCount; observed allocation and admitted root scope |
| database.ParametersSQL / ParametersWithDefaultsSQL | SessionParameters / SessionParametersWithDefaults; not instance or SPFILE |
| database.NLSDatabaseSQL / NLSInstanceSQL | Existing NLSDatabase / new NLSInstance |
| services.NamesSQL | Names; original V$SERVICES source/filter retained |
| objects.InvalidStatement / InventoryStatement / TotalsSQL | InvalidObjects / ObjectInventory / ObjectTotals |
| objects.CountStatement / CountStatementWithOptions | TableCount; explicit quoted ordinary-table/workload admission |
| objects.SchemaSQL / TablesSQL / TableSQL | Schema / Tables / Table |
| objects.TablesStatisticsSQL / TableStatisticsSQL | TablesStatistics / TableStatistics; raw optimizer estimates |
| objects.OldestStatisticsStatement / DecodeStatistics | OldestStatistics uses existing selector/options/nullable model; new DecodeOldestStatistics/Read/native/JSON adapters |
| identity.DatabaseStatement / InstanceStatement / ExecutionStatement | DatabaseIdentity / InstanceIdentity / ExecutionIdentity; legacy independently valid property evidence preserved |
| identity.NameStatement / RoleStatement / VersionStatement / EditionStatement / BannersStatement | NameSource / RoleSource / VersionSource / EditionSource / BannersSource |
| identity.SnapshotSQL / ContainerSQL / FullVersionSQL / PDBOpenModeSQL / ConnectedArchitectureSQL | Snapshot / ContainerSource / FullVersionSource / PDBOpenMode / ConnectedArchitecture |
| identity unknown-version VERSION bootstrap | VersionBootstrap; explicit major0 admission; no fabricated RU precision |
| identity.OracleHomeStatement | Existing OracleHome native/JSON/property paths; do not duplicate the source |
| identity.OracleHomeEnvironmentStatement | Procedural GET_ENV OUT binds have no native cursor; existing typed property/JSON route; never fabricate Rows |
| memory.Statement / Decode / PDBStatement / DecodePDB / RACStatement / DecodeRAC | Read/Rows/JSON, ReadPDB/PDBRows/PDBJSON, ReadRAC/RACRows/RACJSON; preserve independent aggregate evidence |
| engineeredsystem/catalog exported selectors | Metadata / Configuration (config or info) / VisibleCellCount / Cells / Alerts / ConnectedCells; caller-admitted actual schema, raw payload/LOB/layout evidence |

## Pure and procedural responsibilities

Pure aggregate/XML/patch/persistence evaluators and SQL-expression/template helpers
remain pure. They do not invent database I/O, native cursors or redundant source
families. OracleHomeEnvironment is procedural OUT-bind execution and cannot
return caller-native rows. These are semantic exceptions, not unresolved ordinary
fact gaps. The root facade remains thin useful wiring; responsibility-package
functions supply the complete APIs without adding a generic registry.

## Changes and review coverage

The common implementation standard applies across all 39 first-party Go packages
and validation configuration. The initial identity/metadata bridges now share
internal/projection.FromRowset. New source families freeze domain selection,
schema and provenance and delegate generic cursor ownership to session/rowset.
Legacy implementation/source files are preserved; new canonical source differences
and exact values are documented, not hidden behind presentation aliases.

The initial assessment reused exact prior reviews for unchanged content and
freshly traced remaining contracts; the new API branch receives one independent
whole-branch source/test/documentation review with a per-file identity receipt.
Inventory, passing tests and native SQL receipts do not substitute for that review.
The issue tracks reviewed, excluded and remaining coverage and required full
validation/integration/cleanup gates. No source area is intentionally excluded.
