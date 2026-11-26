# Hidden parameter catalog

The fixed-table catalog returns leading-underscore parameters from
`SYS.X$KSPPI`, session values from `SYS.X$KSPPCV`, and instance values
from `SYS.X$KSPPSV`, joined by `INDX`. It includes default entries that
the visible dynamic-view dataset might not expose.

Use `HiddenParameterCatalog` for a caller-native `*sql.Rows` cursor or
`HiddenParameterCatalogJSON` for a separate bounded JSON read:

```go
facts := oracleinfo.New(admittedConn) // caller-owned, session-affine handle
filter := oracleinfo.HiddenCatalogFilter{NameContains: "waiters"}
payload, err := facts.HiddenParameterCatalogJSON(ctx, filter,
    oracleinfo.JSONLimits{MaxRows: 20000, MaxBytes: 8 << 20})
if err != nil {
    return err
}
// payload is standard JSON with provenance and ordered columns/rows.
```

An empty filter requests all exposed underscore parameters, including defaults.
`Names` selects one or more exact, case-sensitive names. `NameContains`
is a case-insensitive **literal ASCII substring**, up to 80 characters;
`"waiters"` corresponds to `LIKE '%waiters%'` in the example query.
Percent and underscore characters in this filter have no wildcard meaning.
When both filters are supplied, both must match. All values use bindings.

The columns are `NAME`, `SESSION_VALUE`, `INSTANCE_VALUE`,
`ISSES_MODIFIABLE`, `ISSYS_MODIFIABLE`, and `DESCRIPTION`.
All are textual facts: numeric-looking values remain strings, SQL NULL remains
null, and session and instance values are never merged. Session modifiability
uses bit 1 of `KSPPIFLG / 256`; system modifiability uses the low two bits of
`KSPPIFLG / 65536`: 1/3 maps to IMMEDIATE, 2 to DEFERRED, otherwise FALSE.
These flags do not establish that changing an internal parameter is supported.

## Complete typed reads and selectors

`parameters.ReadHiddenCatalog` accepts an already-admitted complete buffered
reader and `HiddenCatalogRequest{OracleMajor, Filter, Reference}`. It returns
`HiddenCatalogResult` with raw `session.Value` fields and source/filter
provenance. Reader transport budgets, terminal errors, deadlines and lifetime
remain caller-owned. The entire result is validated before any row is published.

`HiddenCatalogStatement` builds the same SQL for an existing execution layer;
`DecodeHiddenCatalog` validates an already-complete result.
`HiddenCatalogRows` and `HiddenCatalogJSON` are the lower-level native
companions when the caller already knows the observed Oracle major version.

Native rows are the original cursor. The caller must iterate, check `Err`,
close and enforce budgets. JSON owns its independent cursor and delegates
consumption to `go-oracle-session/rowset`; query, iteration, scan, close,
cancellation or capacity errors publish no JSON prefix. There is no CSV or
plaintext resultset serialization.

## Scope and access

This is an explicit opt-in internal source with an admitted major-version floor
of 11. Fixed-table availability and privilege must be checked for each actual
release, instance and container. The library does not grant access, connect as
SYS, change containers, retry or fall back to another source. Permission/source
failures stay failures. It opens no connections and adds no database driver.

Provenance is `connected_session_and_instance`. It does not claim all RAC
members, other sessions, all PDBs or stored SPFILE definitions. If an application
needs observations on multiple instances, it must supply separately admitted
session-affine handles and record coverage itself.

The existing `HiddenParameters` / `parameters.ReadHidden` APIs continue to
read visible underscore rows from `GV$SYSTEM_PARAMETER2`, with their original
shape and scope. Use those for visible effective-list facts. Catalog availability
must not silently broaden an application's non-default/explicit-name selection
or change its saved migration acceptance policy.


## Qualification

Read-only server execution of the exact generated selections was checked on
11.2.0.2, 19.32 Enterprise (root and PDB), and 23.26.1 Enterprise (root, PDB and
mounted physical standby). The checks cover all names, an exact name and the
literal waiters filter, the six-column projection and decoded flag shapes.
The mounted check used direct fixed-table SQL because DBMS_SQL was unavailable
in that mounted database.

Native cursor identity, typed decoding and JSON lifecycle/error behavior have
separate deterministic database/sql fixture coverage; those fixtures do not
qualify a live Oracle Go driver. These observations do not establish every RU,
restricted grant, Autonomous Database or multi-node RAC combination. Recheck
source availability and privileges for each admitted connection.

To inspect the selected SQL and binds without connecting:

```powershell
go run ./examples/hidden-catalog-selectors --oracle-major 19
```

## Session default flag

When the caller needs the raw session-side default flag, use
`parameters.ReadHiddenCatalogWithSessionDefault(ctx, reader, request)`.
Each row exposes `SessionIsDefault` as a `session.Value` sourced directly from
`SYS.X$KSPPCV.KSPPSTDF`, preserving exact text and SQL NULL. Interpret it only
as the flag from the session-side row. The instance value still comes from
`SYS.X$KSPPSV`. The SDK keeps the raw text and assigns no meaning to new values.

The extended Statement/Decode/Rows/JSON companions and the Client methods
`HiddenParameterCatalogWithSessionDefault` and
`HiddenParameterCatalogWithSessionDefaultJSON` append
`SESSION_IS_DEFAULT` to the six catalog columns. Existing catalog signatures,
SQL and native/JSON shapes are preserved. The same connected-session/instance
scope, name filters, required fixed-table access and caller-owned budgets apply.

A consumer can report admitted visible instance values from `ReadHidden`
separately when the fixed-table catalog is unavailable. The visible read has its
own coverage and provenance. Neither reader derives parameter values from the
alert log or substitutes for the independent `ReadHiddenContext` source.

### Extended projection qualification

The extension uses the same connected-session/instance source and Oracle major
floor as the original catalog. Public Oracle materials describe the supported
[session parameter view](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-PARAMETER.html).
No Oracle-published contract for the internal `KSPPSTDF` column or a supported
mapping to its raw flags was verified. The SDK exposes the column as an opt-in
internal fact. A missing or inaccessible column fails the whole selected read.

| Oracle release | Static selection | Native extended SQL / Go driver |
| --- | --- | --- |
| 9iR2 and 10gR1/R2 | Unsupported before executing SQL | Not applicable |
| 11gR1/R2 | Seven columns when the source is admitted | Unverified |
| 12cR1/R2 and 18c | Same projection | Unverified |
| 19c and 21c | Same projection | Unverified |
| 23c/23ai and 26ai (observed major 23) | Same projection | Unverified |

Tests verify selected SQL, binding, ordered legacy and extended column shapes,
exact non-Boolean flag text, SQL NULL, and rejection of malformed or over-budget
results. Prior native qualification above applies to the six-column catalog.
Restricted accounts, RAC, non-CDB/root/PDB login and OPEN/MOUNT combinations
remain unqualified for the added column unless independently observed.
