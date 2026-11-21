# Complete source fact APIs

Issue #65 adds or completes 56 source fact families. Existing selectors,
properties, pure decoders and immutable releases remain available. The
[common pattern](implementation-pattern.md) defines ownership and completion;
[compatibility evidence](fact-api-compatibility.md) separates documentation,
static selection/decoding, native SQL and driver/application qualification.

## Calling a fact

Ordinary new families expose XStatement, DecodeX, ReadX, XRows and XJSON.
XOptions carries admitted choices; XRequest adds a valid caller-owned SQL
reference. XResult contains typed Rows and provenance. session.Value.Text is
exact source text; Null is SQL NULL. Optional pointers distinguish an unadmitted
column/layout. No flags, units, defaults, roles or visibility become policy.

~~~go
options := performance.BlockersOptions{OracleMajor: observedMajor}
facts, err := performance.ReadBlockers(ctx, admittedReader,
    performance.BlockersRequest{Options: options, Reference: callerRef})
// Check err before facts; the reader owns finite budgets and complete EOF/close.
rows, err := performance.BlockersRows(ctx, admittedNativeHandle, options)
// Check err; caller owns Next/Scan/Err/Close, deadlines, affinity and budgets.
data, err := performance.BlockersJSON(ctx, admittedNativeHandle, options,
    rowset.Limits{MaxRows: 10000, MaxBytes: 8 << 20})
// Check err before publishing the bounded standard JSON.
~~~

The caller supplies SQL references and numeric version/capability evidence.
No connection, credential/driver selection, destination/container change,
capability cache, retry or report/scoring policy is introduced. Native rows are
the actual caller cursor. JSON independently consumes/finalizes and validates
its cursor, returning no bytes on any failure. Its budget includes the final
provenance envelope; exact-boundary/one-byte-short regressions constrain it.

Pure Decode leaves Provenance.Complete false. Successful owned reads/JSON mark
the selected transport complete, without certifying physical RAC membership,
all-container/history coverage or an atomic multi-query snapshot. A bounded
sample remains a sample even when it exactly fills its SQL limit.

## Families

Substitute each ordinary family name for X in the five methods above. Each
package owns one frozen SQL/schema/provenance selection. Owned paths validate
every row before publishing. Buffered readers remain usable without extracting
private native cursors from a Session lease.

| Package | Complete families |
| --- | --- |
| alertlog | Entries, Counts |
| backup | Jobs |
| database | Configuration, Endian, CharacterSet, CPUCount, Products, DatafileSize, Storage, EncryptionCounts, SGAAllocated, PGAAllocated, PDBCount, SessionParameters, SessionParametersWithDefaults, NLSInstance |
| dataguard | Status, Lag |
| engineeredsystem/catalog | Metadata, Configuration, VisibleCellCount, Cells, Alerts, ConnectedCells |
| identity | DatabaseIdentity, InstanceIdentity, ExecutionIdentity, NameSource, RoleSource, VersionSource, EditionSource, BannersSource, Snapshot, ContainerSource, FullVersionSource, PDBOpenMode, ConnectedArchitecture, VersionBootstrap, LogicalDatabase |
| instances | Visibility |
| parameters | ScalarSystem |
| objects | InvalidObjects, ObjectInventory, Schema, ObjectTotals, Tables, Table, TableCount, TablesStatistics, TableStatistics, OldestStatistics |
| performance | Blockers, ResourceLimits |
| scheduler | Jobs |
| services | Names |

### Named families for existing APIs

The named functions select the same sources as the generic functions from prior
releases. Existing function signatures remain supported.

| Package | Named family |
| --- | --- |
| dblinks, directories, networkacl, sequences, triggers, users | Inventory |
| instances | Inventory |
| memory | Parameter |
| metadata | Object |
| passwordfile | Info |
| registry | Components |
| services | Definitions |
| identity | OracleHome, including OracleHomeJSON for the native projection |

Existing families with multiple positional source selectors expose
`StatementWithOptions`, `Decode…WithOptions`, `RowsWithOptions` and
`JSONWithOptions` forms. This covers ASM clients, encryption, Data Guard
transport facts, configured PDB memory, invalid details and oldest statistics,
parameter facts, redo inventories and SGA components. `ReadRACWithOptions`
takes a request containing the RAC selection and SQL reference.

OldestStatistics reuses the existing OldestStatisticsStatement and
StatisticsOptions. DecodeOldestStatistics/ReadOldestStatistics and
OldestStatisticsRows/OldestStatisticsJSON preserve the nullable Statistics model,
including sql.NullString and optional Scope. Owners remain variadic; JSON places
Limits before that list. The result remains a bounded sample with the original
admitted ORACLE_MAINTAINED capability and owner requirements.

Memory completes Statement/Decode with Read/Rows/JSON; PDBStatement/DecodePDB
with ReadPDB/PDBRows/PDBJSON; and RACStatement/DecodeRAC with
ReadRAC/RACRows/RACJSON. SourceRequest contains Parameter/Options/Reference.
PDB native methods require complete connected-PDB Options; discovered targets
use scoped forms and the exact named bind. Read selects one source with no
fallback. ReadRAC retains CollectRAC's independently valid metrics alongside
errors; JSON returns no bytes on decoding/transport errors. Collect/CollectRAC
are unchanged. Effective ADB selection retains the caller-established primary
NULL/no-row and known18+ connected-PDB preconditions.

The existing database.NLSDatabase family already provides all five paths and
remains the canonical full API for NLSDatabaseSQL.

## Compatibility mappings and values

Compatibility SQL/constants remain byte-identical. New canonical facts remove
legacy presentation/report selection where necessary:

- Configuration returns raw cluster_database text, including NULL, rather than
  RAC/Single Instance display labels.
- Products returns all visible registered component IDs. The old six-component
  allowlist belongs to reports; registration is not licensing evidence.
- Storage returns data/temp file names and source kinds. Consumers format
  classifications from raw facts; unknown names remain NULL.
- DatafileSize, SGAAllocated and PGAAllocated explicitly retain observed,
  source-rounded GiB text (three-decimal SQL rounding), not exact raw bytes or
  configured targets. Empty/missing observations never become zero.
- SessionParameters uses executing-session display/raw values;
  SessionParametersWithDefaults retains raw VALUE/ISDEFAULT. Neither is an
  instance-wide SYSTEM_PARAMETER2 or stored SPFILE observation.
- ObjectInventory, ObjectTotals, TableCount, discovery and statistics preserve
  distinct counts/estimates/filters. Exact owner/table predicates use binary
  comparison. Quoting/hint bounds do not authorize tables, workload or cutoff.
- Identity Source/Identity datasets supplement existing scalar/multi-query
  properties. VersionBootstrap admits unknown zero and reads legacy VERSION,
  never full-version/RU precision. Existing property fallback, attempts and
  independently valid evidence remain unchanged.

- [LogicalDatabase](logical-database-identity.md) reads the control-file DBID and name on reviewed releases from 9.2, with the CDB flag from 12.1.

## Engineered-source capabilities

MetadataResult.CapabilitySchema creates a fresh validated snapshot without I/O
or caching. Callers explicitly supply it to Cells/Configuration/Alerts. No fact
automatically discovers capabilities. Unsupported source choices fail before
SQL. Existing static global/local capability selection remains; a failed query
never triggers another source or an AWR substitute.

Cells retains the legacy fixed raw projection, including synthetic NULL CELL_TYPE
when the dictionary column is absent. OptionalColumns records actual source
column admission: typed Type=nil means unavailable; nonnil Null means selected
source NULL. Raw JSON retains the native projection plus that provenance.
ConnectedCells genuinely omits the unadmitted column and requires either
IncludeType or CellTypeKnownAbsent. Its limit remains a sample bound.

Alerts explicitly retains scalar/XML layouts and nil unselected fields.
Configuration/XML alerts validate complete native lengths in UCS2 units
(supplementary Unicode uses two) before owned publication. Raw malformed XML
remains evidence; normalization/acceptance stays in the existing pure model.
These source APIs require no physical hardware lab. Unperformed nonempty native
and driver combinations remain unqualified.

OracleHomeEnvironmentStatement is procedural OUT-bind SQL, not a query cursor.
Its existing admitted OracleHome property/JSON route remains; no fabricated Rows
API is added. Pure models/evaluators/expression/quoting helpers are not SQL-only
fact gaps and acquire no artificial query API.

See [GoldenGate database discovery](goldengate-discovery.md) for separate process identity, verified accessible heartbeat sources and exact lag/freshness evidence (#72).
