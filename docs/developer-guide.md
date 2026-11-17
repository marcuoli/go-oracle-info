# Developing with punctual Oracle facts

Use one small fact method for a cohesive Oracle responsibility. The fact library
owns reviewed source/version choices, filters, typed decoding and provenance.
The session library owns generic query execution, cursor consumption and JSON.
The application owns endpoint/container/grant admission, connection lifetime,
deadlines, budgets, recovery, report selection, comparison and presentation.

## Choose the reader that you already own

For an existing session-owned buffered reader, call the responsibility package.
`*session.Session` satisfies the structural `parameters.Reader` and
`passwordfile.Reader` contracts. Supply a stable caller-owned SQL reference and
an already-observed Oracle version. Do not open a second connection or extract a
private cursor to use these methods.

```go
func readHidden(ctx context.Context, reader parameters.Reader, major int,
    ref reference.Reference) (parameters.EffectiveResult, error) {
    return parameters.ReadHidden(ctx, reader, parameters.Request{
        OracleMajor: major,
        Filter: parameters.Filter{}, // all visible underscore instance facts
        Reference: ref,
    })
}
```

`ReadEffective`, `ReadHidden` and `ReadSPFile` execute their selected statement
once and decode only a complete successful `session.Result`. The supplied reader
must enforce finite budgets and retain terminal/close errors. Failed prefixes,
post-query cancellation, malformed metadata or any malformed late row publish no
successful row slice. These functions own no retry, session lease or disposal.
Use `EffectiveStatement`/`HiddenStatement`/`SPFileStatement` plus the corresponding
`Decode...` only when the existing collector already executes reviewed SQL. A
pure decoder cannot prove that a transport completed or that the caller was
admitted to the source.

Typed parameter fields use `session.Value`: `.Text` retains exact characters and
`.Null` is SQL NULL. Numeric identity/type/ordinal columns are validated as exact
signed integers while retaining their original text. SPFILE `Type` remains text.
A nil `ContainerID` or `IsPDBModifiable` pointer means the column was not projected
on 11g; a nonnil pointer with `.Null` means a projected SQL NULL. Row order,
repeated names and list ordinals remain separate. Avoid mapping NULL to empty,
zero or false.

For a caller-owned native `database/sql` handle, use the root facade. It obtains
version/source/reference wiring without capability caching or connection changes.

```go
facts := oracleinfo.New(existingConn) // caller-admitted *sql.Conn or *sql.Tx
rows, err := facts.HiddenParameters(ctx, oracleinfo.ParameterFilter{})
// Check err; caller owns Next/Scan/Err/Close and stream budgets on rows.

data, err := facts.HiddenParametersJSON(ctx, oracleinfo.ParameterFilter{},
    oracleinfo.JSONLimits{MaxRows: 10000, MaxBytes: 8 << 20})
// Check err before encoding/writing the returned JSON.
```

A pool/custom Queryer needs caller-proven compatible targets and supported cursor
concurrency. Raw rows are the actual native pointer. A JSON companion executes
its own identical selection, delegates consumption/finalization to
`go-oracle-session/rowset`, and returns nil on failure. Separate observations are
not one snapshot. For generic owned-stream JSON, use the session module's
existing Stream and rowset APIs; never cast its private cursor to `*sql.Rows`.

## Preserve source semantics before selecting report rows

Effective instance values, current-session values and stored SPFILE clauses are
different facts. `ReadEffective` and `ReadHidden` use `GV$SYSTEM_PARAMETER2`, not
session `V$PARAMETER`. The hidden method adds a literal leading-underscore filter
and returns the same effective projection. It describes visible/configured view
rows, not every unset internal default. It scans no `X$` tables.

The application can select non-default rows plus explicitly configured names,
and align the corresponding peer row even when that peer is default. Collect
visible facts before calculating that cross-endpoint union. Keep hidden and
ordinary tables separate as a report decision. `ISDEFAULT`, `ISMODIFIED` and all
other flags stay raw strings; unknown/NULL states need explicit presentation.
Missing requested names remain unavailable, never invented defaults or zero.
An authoritative empty admitted result differs from a privilege/source failure.

Instance filters mean reporting `INST_ID`, not complete RAC membership. Modern
parameter selectors admit current-container and whole-database rows; they do not
switch containers or collect every PDB. Preserve actual instance/container/name/
ordinal identity during comparison. SPFILE `SID` is a stored applicability clause,
independent of reporting instance; wildcard/dormant definitions are retained
without resolving precedence or proving complete file contents.

## Read password-file metadata

The base view is available from Oracle 12.2.0.1. Use observed numeric version
evidence with enough components to establish 12.2.0.1; sparse 12.2 evidence is rejected. `DefaultOptions` centrally selects
the documented common projection on 12.2/19 and `IS_EXC` from Oracle 21c onward.
This is source selection, not native certification. No metadata probe, fallback
or retry runs after a source error.

```go
func readPasswordFile(ctx context.Context, reader passwordfile.Reader,
    observedVersion string, ref reference.Reference) (passwordfile.Observation, error) {
    options, err := passwordfile.DefaultOptions(observedVersion)
    if err != nil {
        return passwordfile.Observation{State: passwordfile.Unsupported}, err
    }
    return passwordfile.Read(ctx, reader, passwordfile.Request{
        Options: options,
        Reference: ref,
    })
}
```

Explicit `passwordfile.Options` can admit a narrower projection or an independently
verified optional column. For borrowed native facade calls,
`PasswordFileInfo(ctx, PasswordFileOptions{IncludeExascale: ...})` and its JSON
companion use that explicit column admission while hiding version lookup.
The zero facade option selects the common projection; it is intentionally not a
claim that Exascale is false. Use the functional `DefaultOptions` path when the
collector wants the documented release-based projection.

`Observation.State` separates available row, successful `no_rows`, unavailable
read and unsupported source. `Value` is nil without an available row. Metadata
fields preserve exact text/SQL NULL. `IsExascale == nil` means unselected;
`IsExascale.Null` means a selected SQL NULL. `IS_ASM=FALSE` alone cannot identify an
OS file when Exascale is possible. Preserve unknown formats/flags. CDB `CON_ID=0`
describes shared database-file metadata, not a separate file for the connected
PDB or every RAC member. Metadata may be stale after a name/location change.
These APIs never flush caches, inspect file contents, read users/verifiers or
prove file existence, consistency, format compatibility or security posture.

## Add or request another method

Start with a concrete consuming use case and the owning fact package. An issue
should state the property/dataset, official Oracle source links and release/RU/
platform scope, reviewed projection and source capability, raw numeric/NULL/
absence meaning, filter/cardinality/completeness rules, required grants/admission,
and the real consumer that will validate it. Name expected failure and empty
states and the exact native acceptance matrix. Keep private examples outside the
repository. A request for one column does not authorize a general executor,
connection/scoping engine, scoring policy or internal-table fallback.

Implement a small statement selector and pure decoder, with a structural admitted
reader method where a real consumer needs it. Reuse one private domain selection
for SQL/schema/provenance across buffered/native/JSON paths. Add the facade only
for a useful punctual property/dataset. Extend an existing responsibility package
when appropriate; avoid one package per field or a query registry. Preserve
released APIs, concise exported Godoc, CHANGELOG, typed issue, reviewed PR and
versioned first-party dependencies. Include negative/NULL/no-row/late-failure/
capability tests and coordinated native evidence; fixtures alone are not native
qualification. Finish integration, public module proof, canonical sync and exact
source/worktree cleanup before closure.

Never serialize query result data as CSV/plaintext; only genuine `DBMS_OUTPUT`
may be textual result output. Typed scalar/string facts and SQL source are not
formatted resultsets. Use standard JSON and keep diagnostics separate. Safe
errors expose source/operation and preserve causes for `errors.Is/As/Unwrap`;
unwrap only in deliberate private diagnostics because driver causes may contain
SQL, bindings, paths or credentials.

Oracle references and bounded source contracts are in
[parameter datasets](parameter-datasets.md) and
[password-file metadata](password-file.md).

For scalar-row instance parameters, use parameters.ReadSystem and parameters.ModifiedSystem. Enumerate qualified IDs with instances.Read before bounded per-member collection; see [system parameter contracts](system-parameters.md). These APIs preserve raw modification flags and never infer persistence or certify physical RAC membership.

## Explicit internal hidden-parameter catalog

For distinct session/instance values and decoded modifiability from fixed tables,
use the [hidden catalog APIs](hidden-parameter-catalog.md). These are separate
from visible underscore facts and reuse the same shared rows/JSON ownership.

For the executing instance's server home and its explicitly inferred 11g library
path counterpart, see [ORACLE_HOME facts](oracle-home.md). Reuse its existing
statement/decoder or admitted reader; never substitute the client environment
or copy one member's value to other RAC members.
