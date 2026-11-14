# go-oracle-info

Reusable read-only Oracle identity/platform, configured-memory and engineered-system facts.

Public packages select reviewed SQL and validate exact results from a caller-owned reader. The module opens no connections, changes no container or endpoint, and selects no credentials, driver, retry policy, scoring or report layout. It reuses the driver-neutral query/value contract in go-oracle-session.

The initial memory contract preserves non-CDB, CDB-root and PDB scope, inherited versus effective targets, the guarded ADB fallback, and validated RAC membership/totals. Scalar observations retain the source's rounded three-decimal GiB text. RAC observations retain raw member bytes and round only after byte aggregation. SQL NULL, no row and known zero remain distinct. Missing or invalid evidence never becomes an assumed zero.

Oracle platform names are not complete operating-system distribution/vendor/release evidence. Native release/platform qualification must be recorded by each consumer; synthetic tests do not certify a database deployment.

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

Validation uses Go 1.27.4 on Windows/amd64 with released module versions. The module's language minimum remains Go 1.27.3. Native Oracle-version/platform qualification and consumer adoption are not established by the fixture suite. Public module releases use github.com/marcuoli/go-oracle-info; the private Gitea repository remains the source authority.

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
