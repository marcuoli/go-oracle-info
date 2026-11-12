# go-oracle-info

Reusable read-only Oracle identity/platform and configured-memory facts.

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

Validation uses Go 1.27.4 on Windows/amd64 with released module versions. The module's language minimum remains Go 1.27.3. Native Oracle-version/platform qualification and consumer adoption are not established by the fixture suite. This private source implementation has no public module release yet.
