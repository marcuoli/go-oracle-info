# Connected core facts

These additive APIs retain the existing SQL constants, identity observations,
component model and name-only service/invalid-object selectors. They read only
caller-admitted handles. No API opens connections, changes grants/containers,
configures sessions, infers licensing or decides migration acceptance.

| Responsibility | Statement / decoder | Complete reader | Native rows / JSON |
| --- | --- | --- | --- |
| Permanent database NLS values | `database.NLSDatabaseStatement` / `DecodeNLSDatabase` | `ReadNLSDatabase` with `NLSDatabaseRequest` | `NLSDatabaseRows` / `NLSDatabaseJSON` |
| Installed option/feature flags | `database.OptionsStatement` / `DecodeOptions` | `ReadOptions` with `OptionsRequest` | `OptionsRows` / `OptionsJSON` |
| Dictionary components | existing `registry.Statement` / `Decode` | `registry.Read` with `Request` | `registry.Rows` / `JSON` |
| Control-file properties | `identity.DatabaseInfoStatement` / `DecodeDatabaseInfo` | `ReadDatabaseInfo` with `DatabaseInfoRequest` | `DatabaseInfoRows` / `DatabaseInfoJSON` |
| Observed session/client context | `identity.SessionContextStatement` / `DecodeSessionContext` | `ReadSessionContext` with `SessionContextRequest` | `SessionContextRows` / `SessionContextJSON` |
| Detailed services | `services.Statement` / `Decode` | `services.Read` with `Request` | `services.Rows` / `JSON` |
| Invalid-object dates | `objects.InvalidDetailsStatement` / `DecodeInvalidDetails` | `ReadInvalidDetails` with `InvalidDetailsRequest` | `InvalidDetailsRows` / `InvalidDetailsJSON` |
| Upgrade/downgrade/patch history | `patches.HistoryStatement` / `DecodeHistory` | `ReadHistory` with `HistoryRequest` | `HistoryRows` / `HistoryJSON` |
| Datapatch installation attempts | `patches.SQLPatchStatement` / `DecodeSQLPatch` | `ReadSQLPatch` with `SQLPatchRequest` | `SQLPatchRows` / `SQLPatchJSON` |

The root client supplies the corresponding `DatabaseNLS`, `DatabaseOptions`,
`Components`, `DatabaseInfo`, `SessionContext`, `Services`, `InvalidObjects`,
`PatchHistory` and `SQLPatches` native methods and their `JSON` companions. The
facade observes the source release independently per call. It caches no source
capabilities and does not turn an inaccessible dictionary into an empty inventory.

## Values, scope and completion

New typed facts use `session.Value`: `Text` preserves source characters and `Null`
is SQL NULL. Exact integer text is validated without a floating-point conversion.
Optional column pointers are nil only when deliberately unselected; a nonnil value
with `Null` is a selected SQL NULL. Existing `registry.Component` continues using
its published nullable-string fields, raw `MODIFIED` and optional `VERSION_FULL`.

Every result retains source, selected release and scope. Collections keep source
order and duplicates. Empty rows mean a successful empty selected source only.
The caller retains endpoint/instance/container identity and observation time; a
connected source is not proof of complete physical RAC or cross-container coverage.

DATE/TIMESTAMP projections are fixed civil labels, retaining seconds or six
fractional digits as appropriate, without appending a timezone or parsing an
instant. They follow the admitted source's calendar semantics; Gregorian/native
NLS compatibility remains a native qualification item. `VERSION_TIME` is only the
source's version-time label. `CURRENT_SCN` NULL remains NULL, including when not
available in the current open state. Supplemental flags and `FORCE_LOGGING` retain
unknown values and multiple documented modes instead of becoming booleans.

All `Read...` methods execute one statement through the existing complete bounded
reader, validate every row and reject post-query cancellation. The supplied reader
must retain terminal/close/budget failures; a pure decoder cannot prove completion.
The raw `Rows` methods return the caller's actual `*sql.Rows` unchanged, with its
iteration, error, close and budget ownership. Each JSON call owns a separate cursor
and delegates consumption to `go-oracle-session/rowset`. JSON includes a provenance
envelope and ordered typed rowset, and its byte budget includes that envelope.
No prefix is returned after iteration, scan, close, cancellation or capacity errors.
No resultset is serialized as CSV or plaintext.

## Explicit source choices

- NLS selects all `NLS_DATABASE_PARAMETERS` rows, preserving both character sets
  and independent language/territory values. It is not current-session NLS.
- `V$OPTION` flags describe installed options/features. They are separate from
  registered components, observed usage and licensing entitlement.
- `DatabaseInfoOptions.IncludeSupplementalPL` explicitly admits the procedural
  logging column on the exact source. Unselected is distinct from SQL NULL or NO.
- Services retain the requested sixteen columns, subject to `EDITION` support.
  `services.Statement(11)` omits `EDITION`; an optional `true` admits the documented
  11.2.0.2+ column. The same option is accepted by `Decode`, `Rows` and after the
  limits argument by `JSON`; `Request.IncludeEdition` supplies it to `Read`.
  Sources 12+ select it by default. The root facade uses the observed 11g patch-set
  version to select it. `MinCardinality`, `MaxCardinality` and `Enabled` are raw
  documented reserved fields, not service reachability/capacity conclusions.
- Invalid-object owners are bound exactly; empty selection means all visible
  owners. No schema blacklist or application ownership is inferred. `CREATED` and
  `LAST_DDL_TIME` supplement the existing identity/status selector.
- Observed `SYS_CONTEXT` HOST, OS_USER and SESSION_USER describe the executing
  client/session, never the database server host or configured connection identity.
  The explicit 4000-byte SYS_CONTEXT limit avoids its default 256-byte output bound.
- `HistoryOptions.IncludeBundleSeries` explicitly admits `BUNDLE_SERIES`. Oracle's
  11.2 reference identifies 11.2.0.2 as its boundary; the 18 reference instead says
  18c. Preserve this documentation conflict; there is no failed-query fallback.
  The documented history fields map ACTION_TIME, ACTION, NAMESPACE, VERSION, ID,
  COMMENTS and optional BUNDLE_SERIES. No internal `REGISTRY$HISTORY` table is read,
  and no assertion is made about additional fields in uninspected internal schemas.
- `SQLPatchOptions` requires major 12 plus minor 1 or 2, or major 18+ with minor 0.
  Both 12c layouts retain VERSION/BUNDLE_SERIES/BUNDLE_ID; only 12.2 includes
  INSTALL_ID. The 18+ layout selects INSTALL_ID, PATCH_TYPE, source/target versions,
  build descriptions/timestamps and RU_LOGFILE. Repeated apply/rollback attempts,
  statuses, timestamps, flags and NULLs survive independently of upgrade history.
  XML descriptors and binary patch-directory payloads are deliberately not read.

## Reuse and qualification

Source/projection ownership stays in each existing responsibility package; patches
is the separate cohesive history responsibility. These APIs are intended for a
migration-inventory consumer and retain compatibility with existing constants,
component properties and selectors. Generic execution/cursor/JSON mechanics use
the released `go-oracle-session/rowset` directly; no query registry, fabricated
production cursor, transport engine or new third-party dependency is introduced.
No session-parameter inventory or capability cache is added.

Checked 2026-11-15 using the Oracle references below. These support static source
selection only. Deterministic `database/sql` fixtures test lifecycle/serialization;
they do not qualify a native Oracle driver. No live Oracle tests were performed for
these additions. Native 11g patch sets, 12.1/12.2, 18+, 19c and 26ai Enterprise;
root/PDB/non-CDB; OPEN/MOUNTED; restricted grants; NLS calendars; and actual multi-node
RAC remain unqualified for the new projections until separately evidenced.

- [NLS_DATABASE_PARAMETERS 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/NLS_DATABASE_PARAMETERS.html)
- [V$OPTION 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-OPTION.html)
- [V$DATABASE 11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_1097.htm) and [19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE.html)
- [ALL_SERVICES 11.2](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/statviews_2063.htm), [19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_SERVICES.html), and [DBA_SERVICES 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_SERVICES.html)
- [SYS_CONTEXT 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/SYS_CONTEXT.html)
- [DBA_REGISTRY_HISTORY 11.2](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/statviews_4210.htm), [18](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY_HISTORY.html) and [19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY_HISTORY.html)
- [DBA_REGISTRY_SQLPATCH 12.1](https://docs.oracle.com/database/121/REFRN/GUID-F6D2BC12-606C-41FE-B9E8-F3702CD32F89.htm), [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_REGISTRY_SQLPATCH.html), [18](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY_SQLPATCH.html) and [19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY_SQLPATCH.html)

The linked page/view names are the current identifiers; no MOS article content or
identifier migration was verified. Existing registry selection references remain
in [edition/version/registry contracts](edition-version-registry.md).

## Exact-key object status (#145)

`objects.ReadObjectStatus` and its Statement, Decode, Rows and JSON companions
select raw `STATUS` for one batch of 1 to 100 exact owner, type and object
name keys from `DBA_OBJECTS`. The caller supplies the observed numeric release,
reference, admitted reader or native handle and finite limits as applicable.
The selector binds every key value. Its semijoin avoids duplicating dictionary
rows when overlapping inputs match the same object. Native Rows leaves cursor
iteration and close to the caller. Read and JSON return no successful prefix
after access, cancellation, budget, cursor or validation failure.

A nil key `SubobjectName` leaves subobject identity unobserved and admits any
matching candidate under that triple. A nonnil SQL NULL selects rows with a NULL subobject name.
A nonnil name selects the exact subobject. The result preserves duplicate rows,
raw status and SQL NULL. An unmatched key has no result row. A unique row
describes only its admitted container and current-edition visibility.

The existing invalid-detail selector keeps its original projection. It cannot
attest a source subobject or edition. A consumer using that evidence can
request the triple and inspect all candidates. It needs independently matched
source identity for a named subobject or cross-edition inference. Oracle's
`EDITION_NAME` identifies the edition in which the object is actual.
[Oracle edition visibility](https://docs.oracle.com/en/database/oracle/oracle-database/26/adfns/editions.html)

| Numeric release | Oracle reference and projection | Static | Native SQL | SDK driver |
| --- | --- | --- | --- | --- |
| 9.2, 10.1, 10.2 | Unsupported before the invalid-detail comparison floor | Pass | No execution | No execution |
| 11.1 | [11.1 ALL_OBJECTS](https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_1145.htm) documents the five base columns. Edition is reserved and omitted. [11.1 SELECT grammar](https://docs.oracle.com/cd/B28359_01/server.111/b28286/img_text/subquery_factoring_clause.htm) uses aliases inside the factored SELECT. | Pass | Unqualified | Unqualified |
| 11.2 | [11.2 ALL_OBJECTS](https://docs.oracle.com/cd/E18283_01/server.112/e17110/statviews_1155.htm) documents the six columns including edition. | Pass | Unqualified | Unqualified |
| 12.1.0.1, 12.1.0.2, 12.2.0.1 | Six columns in [12.1](https://docs.oracle.com/database/121/REFRN/GUID-AA6DEF8B-F04F-482A-8440-DBCB18F6C976.htm) and [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/ALL_OBJECTS.html). | Pass | Unqualified | Unqualified |
| 18c, 19c, 21c | Six columns in [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/ALL_OBJECTS.html), [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_OBJECTS.html), and [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_OBJECTS.html). | Pass | Unqualified | Unqualified |
| 23c/23ai | Numeric 23 projection. The live 23 reference redirects to 26ai. | Pass | Unqualified | Unqualified |
| 26ai, 23.26.1.0.0 | [26ai ALL_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_OBJECTS.html), same six columns. | Pass | Exact SQL and direct comparison passed in SYSDBA CDB roots, OPEN READ WRITE and OPEN READ ONLY WITH APPLY. | Go 1.27.4 and go-oracle-session v0.23.0 V2 buffered read passed on both roots. |

Both native roots returned one `VALID` row for the same 100-key, 398-bind
batch. The exact SQL NULL subobject matched and the case and named subobject
nonmatches produced successful empty results. Fixture tests exercise distinct
edition candidates, duplicates, malformed metadata, SQL NULL and other status
text. Native positive named subobjects, edition ambiguity, Rows and JSON,
PDB, non-CDB, MOUNT, other accounts and other RUs remain unqualified.
Oracle references checked 2026-11-23.
