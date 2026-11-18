# Implementation coverage and migration map

Use the [common implementation pattern](implementation-pattern.md) for all new
facts and changes. This assessment covers the existing API surface at baseline
`9cfbed11d37ddc006b1669e2949ec85d81abf95b` (issue #65). It preserves public
signatures, SQL bytes, projections, scalar kinds and behavior. It does not add
missing typed/native/JSON capabilities or claim equal API coverage everywhere.

## Why performance and redo look different

`performance/statements.go` publishes two compatibility SQL constants.
`redo/groups.go` implements a complete fact pipeline: reviewed source selection,
shape, pure decoding, admitted buffered reading, borrowed native rows and atomic
JSON. The former has **less API coverage**; it is not an alternative executor
pattern. Consumers of its SQL currently own complete transport and decoding.
New ordinary facts use the full pattern. Existing selectors remain callable
while migrations preserve their actual source, scope and value semantics.

One contract does not require artificial I/O for pure models, renaming published
packages, or identical private filenames. Existing scalar/property, aggregation,
payload and catalog contracts have different responsibilities. The shared rules
are ownership, deterministic selection, exact values, complete owned transport,
error/cancellation handling, provenance and meaningful behavioral tests.

## All-package assessment

The baseline contains 263 Go files: 156 implementation/example/helper files and
107 tests, across 39 Go packages; validation configuration is a fortieth area.
The private per-file assessment ledger records content identities and the exact
prior reviews reused for unchanged bytes. Current contract tracing supplements
those reviews; this is not a claim that every file was freshly reread. Inventory
and successful checks alone are not source review. Native qualification remains
separate from source review and fixture coverage.

| Area | Contract assessed | Disposition |
| --- | --- | --- |
| root `oracleinfo` | Non-owning native facade and identity bootstrap bridge | Thin wiring; use shared projection conversion |
| `alertlog` | Bound-window entries sample and grouped counts | SQL-only migration gaps; preserve bounds and offsets |
| `asm` | Client facts with member filtering | Full fact pipeline; retain domain integer/member checks |
| `backup` | Bounded RMAN control-file job sample | SQL-only migration gap; preserve sample and DATE meaning |
| `database` | NLS/options/encryption facts plus compatibility SQL | Full pipeline for typed facts; mappings below |
| `dataguard` | Eight admitted datasets, snapshot/latest pure helpers, old status/lag SQL | Full pipeline and pure helpers; status/lag gaps below |
| `dblinks` | Admitted dictionary inventory | Full fact pipeline; no remote connection |
| `directories` | Directory dictionary inventory | Full fact pipeline; no filesystem access |
| `engineeredsystem` | Payload model, complete-result decoders, XML normalization | Pure model/decoder; preserve independent valid facts and rejected payload evidence |
| `engineeredsystem/catalog` | Capability-selected SQL/schema catalog | Intentionally selector-only; no automatic collection |
| `goldengate` | Capture/apply/administrator facts and database properties | Full fact pipeline; configured GGSCHEMA remains explicitly unavailable from admitted DB source |
| `grants` | Direct/object/column/role grant facts | Full fact pipeline; raw grants do not imply effective access |
| `identity` | Scalar/bootstrap properties and complete database/session datasets | Preserve reviewed bootstrap/property contracts; mappings below |
| `instances` | Qualified visible-member context | Full fact pipeline; visibility is not physical RAC completeness |
| `memory` | Admitted configured-memory collection and RAC aggregation | Aggregate/property contract; preserve independent evidence, guarded fallback and cancellation |
| `metadata` | Explicit object DDL/CLOB capture | Full fact pipeline; shared conversion plus local complete CLOB validation |
| `networkacl` | ACL dictionary facts | Full fact pipeline; no network probes |
| `objects` | Invalid details, statistics decoder and compatibility selectors | Full pipeline for invalid details; mappings below |
| `parameters` | Effective/list/system/SPFILE/hidden facts and persistence evaluation | Full pipelines plus opt-in pure comparison; preserve source distinctions |
| `passwordfile` | Password-file metadata only | Complete observation/native/JSON contract; no users/verifiers/files |
| `patches` | Patch history facts and pure release-level evaluation | Full pipeline plus pure evaluation; unknown/parity limits preserved |
| `performance` | Current blocker and instance resource-limit SQL | Two SQL-only migration gaps; no workload or historical inference |
| `redo` | Redo groups/members/archive facts and pure clock helpers | Full fact pipelines; retain exact SCN, DATE and reporter checks |
| `registry` | Registered component facts | Full fact pipeline; no licensing inference |
| `scheduler` | Exact-owner job metadata SQL | SQL-only migration gap; no job execution |
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
are outside source-review scope. No new native deployment combinations are
qualified by this change. Runtime coverage and source-review coverage differ.

## Compatibility selector dispositions

A migration gap means the selector lacks a complete corresponding fact API;
it does not mean its existing SQL stops working. These gaps remain explicit
rather than being hidden behind a generic registry or falsely marked migrated.
An existing related fact is not necessarily a drop-in replacement.

| Public compatibility surface | Existing route or required migration |
| --- | --- |
| `performance.BlockersSQL`, `ResourceLimitsSQL` | Add separate complete fact families if API expansion is selected; current-session blockers and connected-instance resource limits retain distinct scope |
| `alertlog.EntriesSQL`, `CountsSQL` | Add separate sample/count families; retain positive seconds bind, entry bound, offsets and grouped-empty meaning |
| `backup.JobsSQL` | Add bounded sample family; reaching the limit never proves complete RMAN history |
| `scheduler.JobsSQL` | Add exact-owner job family, preserving owner bind and raw flags |
| `dataguard.StatusSQL`, `LagSQL` | Typed database context overlaps role/switchover only; it does not replace protection fields or lag observations |
| `database.ConfigurationSQL` | Legacy presentation labels; use admitted raw cluster parameter facts and consumer formatting for new work, without changing this SQL |
| `database.EndianSQL` | Separate platform-endian scalar gap; do not infer byte order from platform-name text |
| `database.CharacterSetSQL` | Database NLS fact family contains the character-set parameter; caller selects the named fact, preserving missing-name/NULL meaning |
| `database.CPUCountSQL` | Session-effective `V$PARAMETER` value; do not silently substitute instance-wide system values or host capacity |
| `database.ProductsSQL` | Registry facts are available; its selected component list remains legacy consumer policy, not a new SDK default |
| `database.DatafileSizeSQL` | Storage APIs expose exact allocation evidence; rounded GiB selector is not byte-for-byte interchangeable |
| `database.StorageSQL` | Storage facts/properties provide raw evidence; preserve NULL/mixed-source states and legacy display strings separately |
| `database.EncryptionSQL` | `database.EncryptionStatement`/`ReadEncryption` provide separately selected scoped counts; preserve legacy combined projection |
| `database.SGAAllocatedSQL`, `PGAAllocatedSQL` | Observed rounded allocation gaps; never substitute configured memory targets from `memory` |
| `database.PDBCountSQL` | Root-visible non-seed count gap; container identity APIs do not establish complete PDB membership |
| `database.ParametersSQL`, `ParametersWithDefaultsSQL` | Keep display/raw and executing-session semantics; instance `SYSTEM_PARAMETER2` and stored SPFILE APIs are not replacements |
| `database.NLSDatabaseSQL` | `NLSDatabaseStatement`/`DecodeNLSDatabase`/`ReadNLSDatabase` and native companions provide the typed source; retain legacy aliases/projection |
| `database.NLSInstanceSQL` | Separate instance-NLS fact gap; database-NLS facts are a different source |
| `services.NamesSQL` | Typed service details are related, but preserve the legacy `V$SERVICES` source and internal-service exclusion rather than aliasing blindly |
| `objects.InvalidStatement` | `InvalidDetailsStatement` and full typed family provide richer invalid-object evidence; preserve legacy projection |
| `objects.InventoryStatement`, `TotalsSQL` | Aggregated owner/type/status fact gaps; preserve exact counts and unknown status handling |
| `objects.CountStatement`, `CountStatementWithOptions` | Explicit quoted-table count selectors; workload/access/cutoff admission and optional bounded hint stay caller-owned |
| `objects.SchemaSQL`, `TablesSQL`, `TableSQL` | Dictionary discovery selectors; typed user/table metadata is related but has different projection/filter semantics |
| `objects.TablesStatisticsSQL`, `TableStatisticsSQL` | Optimizer estimates, not exact row counts; preserve NULL/stale statistics and source filters |
| `objects.OldestStatisticsStatement`, `DecodeStatistics` | Existing bounded sample + pure decoder; Read/native/JSON expansion remains a gap, with explicit Oracle-maintained capability and optional NULLs |
| `identity.DatabaseStatement`, `InstanceStatement`, `ExecutionStatement` | Existing pure identity decoders; complete database/member facts overlap without replacing projection or independently valid evidence semantics |
| `identity.NameStatement`, `RoleStatement` | Existing scalar decoder/property contracts; preserve stable SQL and distinct NULL/empty/unavailable states |
| `identity.VersionStatement`, `EditionStatement`, `BannersStatement` | Existing version/edition/bootstrap properties own their reviewed query sequence, fallback and retained attempts |
| `identity.SnapshotSQL`, `ContainerSQL`, `FullVersionSQL`, `PDBOpenModeSQL` | Compatibility projections; use reviewed identity property/dataset routes when semantically equivalent, never infer release/container admission |
| `identity.ConnectedArchitectureSQL`, `OracleHomeStatement`, `OracleHomeEnvironmentStatement` | Existing architecture/home property paths; procedural GET_ENV cannot be fabricated into native rows |
| `engineeredsystem/catalog` exported selectors | Intentional capability-selected catalog consumed by complete-result decoders; no automatic fixed-table fallback or physical-hardware acceptance gate |

## Changes adopted in issue #65

The identity native-reader bridge and metadata JSON bridge now use
`internal/projection.FromRowset`, as the other typed fact bridges already do.
They still delegate complete cursor consumption to the session rowset package.
Metadata keeps its own object identity, full CLOB UCS2-length and byte-budget
checks. No SQL, public signature, field kind, provenance or JSON format changes.

All development entry points link to the common contract. New ordinary facts
must share selection/schema/provenance across Statement/Decode/Read/Rows/JSON,
with thin useful facade wiring and behavior-sensitive tests. New pure models
and catalogs follow their explicit profiles. Existing gaps above require a
separately scoped API migration rather than an incompatible cosmetic rewrite.
