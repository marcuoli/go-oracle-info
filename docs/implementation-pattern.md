# Oracle fact implementation pattern

This is the common implementation contract for every first-party fact package.
Use it with [the developer guide](developer-guide.md), source-specific Oracle
contracts and [the coverage/migration ledger](implementation-coverage.md).
Uniform ownership and observable behavior matter; public package names and
historical API signatures are not renamed to make files look alike.

## Responsibilities

The responsibility package owns reviewed Oracle SQL, release/capability/filter
selection, source schema, exact typed values, domain decoding and provenance.
The application supplies an admitted scope/reader/native handle, SQL references,
connection lifetime, finite budgets and deadlines. It owns collection scheduling,
recovery, report policy and presentation. Generic query, cursor iteration,
finalization and rowset JSON mechanics belong to `go-oracle-session/rowset`.

Never add an executor, query registry, driver, connection/container switch,
capability cache, private cursor escape or resultset CSV/plaintext serializer.
A string scalar and SQL source text are not formatted resultsets. Test-only
`internal/testsql` fixtures do not qualify native Oracle execution.

## Ordinary fact pipeline

A fact has one deterministic source selection shared by all supported paths:

1. `...Statement`: validate the admitted version, capabilities and filters;
   produce reviewed SQL and bound values. Unsupported choices fail before SQL.
2. `Decode...`: validate the full shape and every row before returning typed
   data. Preserve raw values, SQL NULL, unprojected fields, order and duplicates.
   A decoder does not prove that the transport completed.
3. `Read...`: accept a structural
   `Query(context.Context, session.Statement) (session.Result, error)` reader,
   the caller reference and admitted options. Execute once; reject transport
   errors and post-query/post-decode cancellation without a successful prefix.
4. `...Rows`: return the actual caller-native `*sql.Rows`. Caller owns
   Next/Scan/Err/Close, handle affinity, deadlines and finite stream budgets.
   Do not wrap/fabricate rows or create return-time cancellation.
5. `...JSON` independently queries the same selection through
   `rowset.QueryEnvelopeJSON`. Supply the schema, provenance and optional
   domain validator. The shared helper opens and finalizes the cursor, checks
   the encoded envelope against the byte budget, and returns nil JSON on query,
   scan, terminal, close, validation, cancellation or capacity failure. Fact
   packages retain their historical error wrapping and any source row limit.
   Typed object serialization, such as metadata's result, encodes the typed
   domain model after shared rowset consumption.
6. A useful punctual root `Client` method hides source/version/reference wiring
   without caching or owning the borrowed handle. Do not require a native handle
   from callers that already own an admitted buffered reader.

Use a private selection value when source/schema/provenance vary together.
A fixed projection can use deterministic named helpers with identical admitted
inputs; a new struct is not mandatory merely for uniform spelling. SQL aliases,
column order/kinds, source and version selection must not drift between paths.

`redo.GroupsStatement`, `DecodeGroups`, `ReadGroups`, `GroupsRows` and `GroupsJSON`
are a complete dataset example. Use one admitted options value for a canonical
family. Statement takes options. Decode takes result then options. Read takes
context, reader and request. Rows takes context, queryer and options. JSON takes
context, queryer, options and limits. A single version argument already has the
same position. Group multiple existing selectors in options-based forms with
the `WithOptions` suffix where the original names already carry another
signature. Preserve existing generic names and scalar observations as
compatibility APIs.

## Values, scope and completion

Use the existing domain representation appropriate to the public contract.
`session.Value` retains exact text and SQL NULL; optional pointers distinguish an
unprojected field. Existing `identity.Text` availability and `sql.NullString`
contracts remain valid. Never coerce NULL, empty, no rows, unavailable,
unsupported or failed observations into one state or an invented zero.

Keep Oracle integer/decimal meaning exact. Use declared rowset scalar kinds and
reviewed exact numeric text for values that exceed their representation. Do not
introduce float conversions or silently change an existing JSON scalar kind.

Use `internal/projection.Columns` for complete shape indexing,
`TypedColumns` for shared typed scalar validation and `FromRowset` to convert a
fully consumed rowset. Add source-specific identity, scope, numeric, time, CLOB
or other validation locally. The converter does not replace domain admission.

Successful Read/JSON completion never means complete physical RAC, all PDBs,
all grants, historical coverage or one atomic multi-query snapshot. Preserve
source scope and actual reporting/container identity. Existing Complete flags
retain their documented meaning; absence of a flag in a legacy type is not a
reason to change its wire format. No-error Read semantics still require complete
transport. Pure Decode must not assert transport completeness.

A bounded sample is explicitly a sample. Hitting a SQL limit does not prove a
complete source. Do not silently substitute sample APIs for inventories or remove
an established bound to make adapters easier to share.

## Pure models, aggregations and compatibility selectors

Pure models, evaluators and payload decoders share the ownership/value/error
rules; they do not acquire meaningless Read/Rows/JSON layers. Examples include
parameter persistence, patch evaluation and engineered XML normalization.
An existing aggregate may preserve independently valid facts alongside an error;
this is different from publishing a failed cursor's prefix. Keep its explicit
acceptance/count/error contract and tests.

Capability-selected SQL catalogs remain selectors. Legacy exported SQL constants
and selector functions remain compatibility surfaces during migration; their
result shape, bind/order/scope/rounding/sample contracts cannot be silently
changed. Every such surface needs an explicit disposition in the coverage ledger:
canonical equivalent, compatible adapter to implement, or intentionally pure
catalog. Do not label a missing ordinary fact path complete by calling it legacy.
Do not duplicate an existing typed source or promote legacy presentation policy
into a new database fact merely to fill a naming checklist.

See [complete source fact APIs](fact-apis.md) for all Statement/Decode/Read/Rows/JSON families, legacy mappings and independently recorded release/native qualification.

## Code organization and extension

Use one lowercase snake_case `<fact>.go` file per complete source fact.
Keep the SQL selector, schema, provenance, decoder, and named Statement,
Decode, Read, Rows and JSON entry points together. The argument order and
ownership rules above apply to every complete family.

Use `contract.go` for package-wide reader, error, provenance and selection
helpers when several facts share them. Keep shared options there when they
represent the same admitted source semantics. Single-fact packages can keep
their contract in the fact file. Give pure models, evaluators, procedural
operations, facade wiring and internal helpers files named for their
responsibility.

Keep published compatibility names and signatures beside the fact they
represent. If a compatibility selector has a distinct contract, give it a
responsibility-based home and record the mapping. Preserve its SQL and
result contract, routing through the same implementation where contracts
coincide. Remove superseded files and private duplicate paths. Retire the
old `statements.go`, `read.go` and `*_dataset.go` layers as facts move.
See the [coverage map](implementation-coverage.md#uniform-file-layout).

Prefer existing standard-library and first-party mechanisms. Consolidate actual
duplicated mechanics only when ownership, scalar semantics, errors, budgets and
cancellation remain identical. Small domain-specific wrappers are preferable to
flags, generic registries or reflection frameworks that conceal source policy.
Comments explain concrete constraints, source distinctions and tested limits.

## Compatibility and verification

New, changed, reused or newly enabled SQL needs its per-query 9.2, 10.1/10.2,
11.1/11.2, 12.1/12.2, 18, 19, 21, 23/23ai and 26ai matrix. Record documentation,
static selection/decoding, native SQL and actual driver/application evidence
separately. Include applicable patch/RU, login, OPEN/MOUNT, container and instance
scope. Unexecuted combinations remain unqualified. Oracle branding is not a
numeric predicate; 26ai documentation does not admit hypothetical numeric26.x.
Use sufficient numeric precision/capability evidence for the selected columns.
Retain explicit documented bootstrap/fallback exceptions; never invent a probe.

Tests constrain values and behavior: multiple distinct rows, NULL/absence,
exact numbers, order/duplicates, malformed final rows, filters, unsupported
no-SQL cases, error identity, cancellation, close and row/byte budgets. Native
cursor tests compare pointer identity and exercise a usable cursor. Positive JSON
tests decode and assert independent expected columns/values, not just json.Valid
or provenance presence. Keep native/replay/fixture evidence distinct.

Follow the typed issue, issue branch, CHANGELOG, reviewed PR, full configured
validation, canonical synchronization and exact source/worktree cleanup rules.
Preserve public APIs and immutable releases. Public module publication and
consumer adoption remain separate verified delivery gates.