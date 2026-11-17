# go-oracle-info

![Source compatibility](https://img.shields.io/badge/source%20compatibility-Go%201.27.4%2B-blue)
![Module and build requirement](https://img.shields.io/badge/module%2Fbuild-Go%201.27.4-blue)

Reusable read-only Oracle identity/platform, configured-memory and engineered-system facts.

Public packages select reviewed SQL and validate exact results from a caller-owned reader. The module opens no connections, changes no container or endpoint, and selects no credentials, driver, retry policy, scoring or report layout. It reuses the driver-neutral query/value contract in go-oracle-session.

The initial memory contract preserves non-CDB, CDB-root and PDB scope, inherited versus effective targets, the guarded ADB fallback, and validated RAC membership/totals. Scalar observations retain the source's rounded three-decimal GiB text. RAC observations retain raw member bytes and round only after byte aggregation. SQL NULL, no row and known zero remain distinct. Missing or invalid evidence never becomes an assumed zero.

Oracle platform names are not complete operating-system distribution/vendor/release evidence. Native release/platform qualification must be recorded by each consumer; synthetic tests do not certify a database deployment.

Generic query, borrowed native-row and JSON dataset mechanics belong to `go-oracle-session/rowset`. This module delegates there and owns Oracle SQL selection, fact filters/schema/provenance and decoding. The misplaced `go-oracle-info/rowset` package is removed without compatibility wrappers; update generic consumer imports directly. Owned `Session.Stream` leases remain in the session module.

Use released module dependencies, with GOWORK=off and no local replacements for final validation. See docs/architecture.md and scripts/validate.ps1. License: UPL 1.0.

The existing `*session.Session` satisfies `memory.Reader`; an application can also adapt its admitted executor to that interface. With a bounded context, an admitted direct-PDB reader, and two distinct application-owned SQL references:

```go
value, err := memory.Collect(ctx, reader, memory.Request{
    Parameter: memory.SGA,
    Options: memory.Options{
        Scope: memory.ConnectedPDB,
        OracleMajor: 19,
        TargetContainerID: admittedPDBID,
        PDB: memory.PDBOptions{AllowInherited: true},
    },
    PrimaryReference: primaryRef,
    EffectiveReference: effectiveRef,
})
```

Inspect both `err` and `value.State`. A failed effective query can return the valid primary observation alongside an error. The library does not acquire a different scope to make a query succeed. `identity` exposes statement selection and decoding separately so an application keeps its bootstrap/admission sequence.

Verified **source compatibility with the selected dependencies: Go 1.27.4** on Windows/amd64. The exact Go 1.27.3 compiler rejects an isolated, hash-matched source snapshot with only its main-module directive lowered, because released go-oracle-session v0.9.0 requires Go 1.27.4. This establishes the selected dependency boundary; it does not claim that every source file intrinsically needs that patch. The unchanged shipping **module requirement is Go 1.27.4**, and the **build/validation compiler is Go 1.27.4**. Native Oracle-version/platform qualification and consumer adoption are not established by the fixture suite. Public module releases use github.com/marcuoli/go-oracle-info; the private Gitea repository remains the source authority.

## Engineered-system facts

`engineeredsystem/catalog` selects reviewed cell/schema/configuration/alert
statements from actual dictionary types. `Command.Original` is the canonical
source body; `Text` is the fixed-allowlist rendering. Applications supply their
own SQL reference namespace/sites, Oracle-version/scope admission and complete
query transport. Selectors open no connection and borrow no AWR schema.

`engineeredsystem.DecodeCells`, `DecodeAlerts` and `DecodeConfiguration` consume
`go-oracle-session/session.Result`; `catalog.DecodeSchema` consumes its metadata
projection. The decoders return independent facts and exact source-row counts.
They preserve SQL NULL, exact numeric text, repeated/conflicting observations,
raw rejected XML bytes and source/time provenance. The caller retains both the
returned facts and error, attaches actual source-execution coverage, then uses
`Report.Validate`. XML property counts are not source-row counts. Never pass a
failed cursor's prefix as a complete Result.

```go
facts, counts, err := engineeredsystem.DecodeConfiguration(
    completeResult, engineeredsystem.Config, observedAt, observedCells,
)
// Retain facts.Payloads and accepted facts.Properties even when err is non-nil.
// counts.Read, counts.Retained and counts.Rejected describe source rows.
```

There is no system-family inference, capacity summation, health scoring,
connection/container change, application recovery or presentation. DATE alert
labels preserve Gregorian civil time and era without a fabricated timezone.
Private operator-output replay uses `ORACLE_INFO_ENGINEERED_DISPLAY_REPLAY` with
the integration build tag; this is development verification, not an XML import
feature. Display-derived Oracle 12.2 shapes and synthetic tests do not establish
original CLOB bytes, native Go-driver reads, grants or live collector scopes.
Consumers pin immutable public module versions; development workspace tests
remain separate from normal GOWORK=off dependency adoption.

## Database and operational selectors

Read-only selectors now cover environment/NLS/parameters, service names,
object inventories and exact table counts, ADR errors, RMAN jobs, Data Guard,
scheduler jobs and point-in-time blocker/resource observations. Caller-owned
scope, references, bounds and complete-result readers stay outside these
packages; selectors do not return health or acceptance findings. See
docs/selector-contracts.md for aliases, binds, scope and NULL/completeness rules.

Run the connection-free example with go run ./examples/queries.

For independent scalar identity reads, `identity.NameStatement()` and
`identity.RoleStatement()` preserve the established single-column projections.
Pass complete `session.Result` values to `identity.DecodeName` or
`identity.DecodeRole`. Each validates its column and scalar shape, returning
`identity.Text` with `Available`, `Null` or `NoRows` state; source text is never
trimmed or inferred. Existing combined database and instance APIs remain available.

`identity.ExecutionStatement()` selects the connected database/session identity
with `DATABASE_NAME`, `DBID`, `ORACLE_VERSION`, `INSTANCE_ID`, `CONTAINER_ID`,
`SERVICE_NAME` and `STARTUP_KEY`. Callers must establish `VERSION_FULL` and view
support before executing it. `identity.DecodeExecution` preserves those seven
fields as `Text` in `Execution.Database`, `DBID`, `Version`, `Instance`,
`Container`, `Service` and `Startup`. It also parses positive `InstanceID` and
nonnegative `ContainerID` values when their raw fields are available. Check both
the error and raw field states before using numeric IDs: NULL/no-row retain zero
IDs, while available container zero is valid. Invalid numeric IDs return
`ErrInvalidEvidence` alongside the raw and independently valid facts. Startup
text has no inferred timezone; the decoder neither gates on version text nor
admits a target. Failed or truncated samples must never be passed as complete
results. Application-required fields and target comparisons remain caller-owned.

Observed edition/full-banner APIs and the `registry` component inventory preserve
legacy versus full-column evidence without guessing editions, release updates or
timezones. See [edition/version/registry contracts](docs/edition-version-registry.md).

For bounded oldest table/partition/subpartition optimizer facts, see [statistics selection and admission](docs/oldest-statistics.md). The API preserves parent identities and nullable statistics scope; legacy sources require reliable caller-owned schema admission.

Use the independent [connected identity property readers](docs/identity-properties.md) when the shared library should own version/architecture/edition source selection. Each property retains source, precision, safe failures and compatibility provenance on an already-admitted reader.

For the simplest caller API, use [the non-owning fact client and parameter datasets](docs/parameter-datasets.md): oracleinfo.New(existingConn), scalar properties, native InitParameters/SPFileParameters and bounded JSON companions. SQL/version selection stays internal, the native handle lifetime stays with its owner, and result serialization uses JSON rather than CSV/plaintext.

## Punctual facts and developer integration

Use the [application developer guide](docs/developer-guide.md) to choose the
owned buffered reader or borrowed native facade, preserve source/NULL/completeness
and request additional methods. `parameters` now provides typed effective,
visible-hidden and stored-SPFILE reads alongside native rows/JSON.
`passwordfile` provides metadata-only statement/decoder/reader/native JSON APIs
from the documented 12.2 view, with the optional Exascale column selected by
`DefaultOptions` from Oracle 21c. Sources, version/capability rules and limits are
in [parameter datasets](docs/parameter-datasets.md) and
[password-file metadata](docs/password-file.md). Report selection stays in the
application; no internal hidden-default catalog, credentials or file contents
are collected.

Instance-wide scalar-row system parameters and qualified instance visibility are documented in [system parameter contracts](docs/system-parameters.md). Preserve the published SYSTEM_PARAMETER2 list APIs; per-member scheduling, partial coverage and report selection remain consumer-owned.

## Database capacity and encryption

See [scoped capacity and encryption facts](docs/database-capacity-encryption.md) for exact visible file allocation, ASM/FileSystem classification, independent encryption counts and matching typed/native/JSON APIs. Source visibility and mounted/open limitations remain explicit.

For conditional restart-persistence comparison of admitted existing parameter datasets, see [parameter persistence](docs/parameter-persistence.md). Raw APIs and ModifiedSystem retain their existing contracts; unknown startup/default/SID/scope evidence remains explicit.


For complete qualified member versions and pure within-database SQL release-level assessment, see [patch consistency](docs/patch-consistency.md). Raw history/SQL attempts remain complete; unknown deployment/scope/coverage and non-SQL binary parity remain explicit.

## GoldenGate activity facts

See [database-visible GoldenGate facts](docs/goldengate.md) for typed complete
reads, actual native row cursors and bounded JSON. Sources retain qualified
visible instance/container identity without opening connections, controlling
processes, inferring synchronization or deciding readiness.
