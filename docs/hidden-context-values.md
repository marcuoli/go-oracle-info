# Internal parameter context values

`parameters.HiddenContextStatement`, `DecodeHiddenContext`, `ReadHiddenContext`,
`HiddenContextRows` and `HiddenContextJSON` expose the distinct
`SYS.X$KSPPI + SYS.X$KSPPCV2` context source. The root facade provides
`HiddenParameterContext` and `HiddenParameterContextJSON` without capability
caching or connection/container ownership.

This is an explicit internal-source API. It does not replace the existing
underscore-only `HiddenParameterCatalog` (`X$KSPPCV`/`X$KSPPSV`) or visible
`HiddenParameters` (`GV$SYSTEM_PARAMETER2`) APIs. There is no source fallback or
inference that a context row equals effective instance, session, SPFILE or default
catalog evidence.

## Selection and exact values

The source identity is `a.INDX + 1 = b.KSPFTCTXPN`; joining equal indexes would
change the parameter identity. The projection retains reporting `b.INST_ID`,
`b.CON_ID` from major 12 onward, context row `b.INDX`, raw parameter/value numbers
`KSPFTCTXPN`/`KSPFTCTXVN`, `KSPPINM`, `KSPFTCTXVL`, raw `KSPFTCTXDF` and the parameter
description. Raw context row/parameter/value numbers preserve independent rows
and are not treated as `SYSTEM_PARAMETER2` list ordinals. Repeated names are not
collapsed or overwritten.

The zero `HiddenContextFilter` selects all exposed context rows, including default
rows and names without underscores. `Names` admits bound, exact, case-sensitive
names, including ordinary parameters such as `compatible`. `NonDefaultOnly` adds
literal binary `KSPFTCTXDF <> 'TRUE'`, reproducing the supplied query's predicate:
SQL NULL is excluded, literal `TRUE` is excluded, and other original flag text is
retained. Flags are not coerced to Boolean. The typed and JSON paths apply the
same name/default admission after complete consumption, rejecting out-of-scope
late rows rather than returning a valid-looking prefix.

`HiddenContextValue` uses `session.Value` for exact numeric/text/NULL observations.
`ContainerID == nil` means that the 11g projection omitted the column; a nonnil
value with `Null` means projected SQL NULL. Internal source container identity
must not be silently relabeled as the connected session container. Values retain
whitespace, numeric-looking strings and SQL NULL. Reporting instance and parameter
numbers are required positive exact integers; other projected numeric identities
are validated as exact signed 64-bit integers without float conversion. Ordinary
and hidden names are both valid source facts.

## Ownership, budgets and member coverage

`ReadHiddenContext` receives an already-admitted `parameters.Reader` and
`HiddenContextRequest{OracleMajor, Filter, Reference, Limits}`. The reader owns
session lifetime, full EOF, terminal/close errors and finite transport budgets.
The request applies finite row and retained-value-byte limits; zero uses the
shared rowset defaults. Invalid limits fail before SQL. Query, terminal,
post-query cancellation, late malformed evidence and capacity failures publish no
rows. A pure `DecodeHiddenContext` validates a supplied complete result but cannot
attest that its transport completed. Successful empty selection is a nonnil empty
slice, not proof that an unavailable member has no context values.

`HiddenContextRows` returns the actual caller-owned `*sql.Rows`, without
preconsumption, SQL row truncation or return-time cancellation. The caller owns
iteration, full value consumption, deadlines/budgets and `Err`/`Close`.
`HiddenContextJSON` opens a separate cursor and delegates native execution and
complete consumption/finalization to `go-oracle-session/rowset`; domain admission
then runs before bounded standard JSON is returned. Any query/scan/iteration/
close/cancellation/capacity error returns nil JSON. Numeric identity columns keep
native exact JSON integer meaning; parameter values remain textual. No CSV or
plaintext query-result serialization is provided.

The source scope is `connected_instance_context_rows`. It is not a remote-member
selector, an all-RAC query or a collection engine. Applications requiring every
qualified reachable reporting member must:

1. Retain the qualified visible member set and the independently observed
   instance/container identity. Visible members do not prove complete physical RAC
   membership.
2. Supply a separately admitted member-affine reader/handle for each reachable
   member. Compare returned reporting IDs with the admitted member; do not label
   local rows as another instance by adding an `INST_ID` predicate.
3. Record per-member success, successful empty, unavailable/unsupported source,
   missing requested names, cancellation and budget failure explicitly. Preserve
   independently valid member facts and keep mandatory incomplete coverage
   inconclusive in the application.

The library neither establishes connections nor infers reachability, credentials,
complete RAC/PDB coverage, retry/fallback policy or application report selection.

## Example

```go
filter := parameters.HiddenContextFilter{NonDefaultOnly: true}
facts, err := parameters.ReadHiddenContext(ctx, admittedReader,
    parameters.HiddenContextRequest{
        OracleMajor: observedMajor,
        Filter: filter,
        Reference: callerReference,
        Limits: rowset.Limits{MaxRows: 20000, MaxBytes: 8 << 20},
    })
// Check err before publishing facts; retain member/container coverage separately.
```

To inspect the selected SQL and binds without database access:

```powershell
go run ./examples/hidden-context-selectors --oracle-major 19
```

The example prints JSON selection metadata only. It opens no database and includes
all/nondefault/exact ordinary-name variants.

## Native qualification and limits

Read-only native SQL checks were performed on 2026-11-15 using existing fixtures.
Exact generated all/nondefault/`compatible` selections were executed and compared
with direct source counts using the `INDX+1` identity and raw nondefault predicate.
The following rows were observed; these are observations, not defaults or future
expected counts.

| Actual source/version and scope | All | Nondefault | Exact ordinary name |
| --- | ---: | ---: | ---: |
| Existing 11g fixture, 11.2.0.2.0 | 2659 | 29 | 1 |
| 19c Enterprise 19.32.0.0.0, CDB root | 5874 | 32 | 1 |
| 19c Enterprise 19.32.0.0.0, open PDB | 5874 | 32 | 1 |
| 26ai Enterprise 23.26.1.0.0, CDB root | 7508 | 49 | 1 |
| 26ai Enterprise 23.26.1.0.0, open PDB | 7508 | 50 | 1 |
| 26ai Enterprise 23.26.1.0.0, mounted physical standby | 7507 | 46 | 1 |

Every scope contained ordinary names as well as underscore names. Open sources
were fully fetched with native DBMS_SQL cursors, retaining NULL/default evidence.
The mounted scope used fixed-table-only SQL JSON summaries and independent raw
source-count parity, since a richer diagnostic wrapper failed ORA-01219 there.
The direct SDK SQL itself succeeded. Existing fixture container states were
restored; no database role, open mode, PDB open state or persistent parameter was
changed. PDB context changes were confined to temporary SQLplus processes.

Source metadata inspection showed that 11.2 lacks the modern `CON_ID` column and
that modern context sources expose additional internal columns. Those unrequested
columns are not assigned undocumented meanings. Native observations establish the
selected projection/identity on these fixtures; they do not document all internal
Oracle implementation semantics or prove equality with other parameter sources.

No applicable public Oracle documentation for `X$KSPPCV2`, `KSPFTCTXPN` or
`KSPFTCTXDF` was found in a fresh Oracle-only search on the check date. No
authenticated MOS supporting content was inspected, and no public/MOS semantic
support claim is made. Internal source availability and grants must be admitted
for each actual release/RU/container.

The server SQL evidence is separate from deterministic Go/database/sql fixture
coverage of typed decoding, actual native cursor identity, JSON admission,
late/close/cancellation failures and finite budgets. No live Oracle Go-driver,
restricted-grant, multi-node RAC, 11.1, every RU/platform, Autonomous or broader
container/source qualification is claimed. Missing capabilities remain unavailable
instead of being replaced by a different source with a completeness claim.
