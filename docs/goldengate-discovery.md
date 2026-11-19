# GoldenGate database discovery

Issue #72 adds independent database facts for the consumer's monitoring adapter.
These APIs do not own connections, containers, REST access, polling, process
control, health thresholds or recovery. Existing capture/apply and administrator
APIs, and explicitly unavailable configured GGSCHEMA, retain their contracts.

## API and admission

Each family exposes `XStatement`, `DecodeX`, `ReadX`, `XRows` and `XJSON`:

| X | Options / request | Typed row / result |
| --- | --- | --- |
| Extracts | ProcessOptions / ProcessRequest | Extract / ExtractsResult |
| Replicats | ProcessOptions / ProcessRequest | Replicat / ReplicatsResult |
| HeartbeatCatalog | HeartbeatCatalogOptions / HeartbeatCatalogRequest | HeartbeatObject / HeartbeatCatalogResult |
| Heartbeats | HeartbeatsOptions / HeartbeatsRequest | Heartbeat / HeartbeatsResult |
| HeartbeatLag | HeartbeatLagOptions / HeartbeatLagRequest | HeartbeatLag / HeartbeatLagResult |

`ProcessOptions` carries `OracleVersion`, `Visibility` and exact `Names`.
`AccessibleDictionary` (also the zero value) explicitly selects ALL;
`DatabaseDictionary` explicitly selects DBA. No error triggers another source.
`Extracts` selects CAPTURE_NAME, CLIENT_NAME, CAPTURE_USER, STATUS, CLIENT_STATUS,
PURPOSE and SOURCE_DATABASE from ALL/DBA_CAPTURE with GoldenGate-purpose filtering.
Names filters CLIENT_NAME, the external Extract identity, never CAPTURE_NAME.
`Replicats` selects REPLICAT_NAME, SERVER_NAME, APPLY_USER and STATUS from
ALL/DBA_GOLDENGATE_INBOUND; Names filters REPLICAT_NAME. NULL group identities stay
NULL. Database status and client attachment never become external RUNNING.

`ValidateDiscoveryFilter(DiscoveryFilter{Schema, Extracts, Replicats})` is the
shared input validator. It preserves case, spaces, quotes and Unicode, rejects
NUL/invalid UTF-8/empty list entries/duplicates, and bounds schema and process
filter inputs to 128 UTF-8 bytes, with 1000 entries per list. Source values are
not shortened or bounded to that filter limit. Schema only scopes heartbeat
objects: capture/apply users and grants do not establish the heartbeat owner.
Filters are bound with explicit BINARY comparisons independent of session
linguistic settings. SQL identifiers are escaped by quoting their exact names.

## Accessible objects and structural verification

`ReadHeartbeatCatalog` uses ALL_OBJECTS left joined to ALL_TAB_COLUMNS, optionally
filtered by exact Schema. It considers only the fixed names GG_HEARTBEAT and
GG_LAG, retaining object type/status and every available column's name, datatype
and datatype owner. It does not resolve synonyms or infer a conventional schema.
The typed result groups metadata by owner/name/type, preserves each candidate's
columns, and exposes Supported and Reason. Wrong object type, invalid status,
missing/duplicate required columns, incompatible types and user-defined required
types remain explicit rejected candidates. Extra columns are allowed and retained.

A supported GG_HEARTBEAT TABLE needs VARCHAR2 local/remote databases and both
Extract/routing/Replicat identities, plus TIMESTAMP(6) HEARTBEAT_TIMESTAMP and
incoming/outgoing heartbeat timestamps. A supported GG_LAG VIEW needs VARCHAR2
local/remote databases and incoming/outgoing paths, TIMESTAMP(6) CURRENT_LOCAL_TS,
and NUMBER incoming/outgoing lag and heartbeat age. This is compatibility with
the reviewed projection, not proof of Oracle origin, configuration or grants.

`HeartbeatObject.Source()` returns an immutable `HeartbeatSource` token only for
a supported candidate. `Owner()` and `Name()` expose the exact admitted identity.
Pure decoding establishes structure only; successful Read additionally establishes
that this catalog transport completed. The token owns no handle/lease/cache and
is no continuing accessibility or atomic snapshot guarantee. Callers retain the
same admitted endpoint/container and choose when to refresh discovery. A later
object/grant change is a read failure, never automatic fallback or empty success.

~~~go
catalog, err := goldengate.ReadHeartbeatCatalog(ctx, admittedReader,
    goldengate.HeartbeatCatalogRequest{
        Options: goldengate.HeartbeatCatalogOptions{OracleVersion: observedVersion},
        Reference: callerRef,
    })
if err != nil { return err }
for _, object := range catalog.Rows {
    if !object.Supported { continue } // retain the rejected candidate separately
    source, err := object.Source()
    if err != nil { return err }
    if source.Name() == "GG_HEARTBEAT" {
        facts, err := goldengate.ReadHeartbeats(ctx, admittedReader,
            goldengate.HeartbeatsRequest{
                Options: goldengate.HeartbeatsOptions{Source: source},
                Reference: heartbeatRef,
            })
        // Consume facts only on successful complete transport; retain err.
        _ = facts
        if err != nil { return err }
    }
}
~~~

The example illustrates one source. Consumers retain successful independent
Extract, inbound, catalog and per-object outcomes when another read is denied.
There is no all-or-nothing composite discovery operation in the SDK.

## Time, path and completion semantics

Heartbeat rows retain local/remote database, both directional component identities,
HEARTBEAT_TIMESTAMP and incoming/outgoing stored heartbeat timestamps. Extracts
and Replicats filters select rows containing the requested respective identities
on either direction; they do not prove those two selections form one direction.
Lag rows retain both full paths and signed lag/age seconds as exact TM9 text;
negative, zero, large, fractional and scientific values survive without floats.
Optional `HeartbeatLagOptions.Paths` selects exact incoming/outgoing full paths,
with up to 1000 values of at most 512 UTF-8 bytes. Substring parsing of group names
is not an admitted path selector. NULL is unknown, never zero or a fresh sample.

Both sources add DB_CURRENT_UTC from SYS_EXTRACT_UTC(SYSTIMESTAMP). Timestamp
projections use Gregorian EXTRACT fields and explicit numeric characters, retaining
six fractional digits without changing session settings. GoldenGate's stored
heartbeat timestamps are UTC civil values without zone columns; CURRENT_LOCAL_TS
remains the lag view's current civil timestamp. DB_CURRENT_UTC is explicitly UTC.
A new successful query does not advance stored heartbeat progress. Applications
compare ages/stored times to their own thresholds, retain clock differences and
report stale/missing evidence separately from transport success or an empty source.
Separate catalog/heartbeat/lag reads are not an atomic snapshot or proof of full
end-to-end, Classic, all-container, physical RAC or external process coverage.

Read requires an admitted complete bounded `Reader` and caller-owned Reference.
Pure Decode never claims transport completeness. Native Rows returns the actual
caller cursor; callers own Next/Scan/Err/Close, affinity, deadlines and budgets.
JSON consumes its own cursor through go-oracle-session/rowset; query/scan/terminal/
close/cancellation/validation/row/final-envelope-byte failures publish no bytes.
Catalog JSON exposes raw metadata, including incompatible candidates. Data JSON
provenance also records ObjectOwner and ObjectName. These optional provenance
fields do not alter existing source JSON when they are absent.

The root Client exposes GoldenGateExtractsRows/JSON, GoldenGateReplicatsRows/JSON,
GoldenGateHeartbeatCatalog (typed), GoldenGateHeartbeatCatalogRows/JSON,
GoldenGateHeartbeatsRows/JSON and GoldenGateHeartbeatLagRows/JSON. Source tokens
retain catalog version/admission; heartbeat facade calls require caller-preserved
handle/container affinity. ReadNativeHeartbeatCatalog is available for an admitted
native handle with a caller-owned reference. No private Session cursor escapes.
The selector-only example emits SQL/args JSON without opening a connection.

## Per-query release matrix and evidence

The following applies independently to ALL_CAPTURE, DBA_CAPTURE,
ALL_GOLDENGATE_INBOUND, DBA_GOLDENGATE_INBOUND, the object/column catalog,
verified GG_HEARTBEAT and verified GG_LAG. Query, schema, filtering, timestamp and
exact-number selections have static tests for each admitted profile. Oracle
Database version and installed GoldenGate object layout are independent inputs.

| Numeric release | Reviewed selection/documentation | Static | Native SQL | Go driver/application |
| --- | --- | --- | --- | --- |
| 9.2 | New projections not admitted | Reject before SQL | Unperformed | Unperformed |
| 10.1 / 10.2 | New projections not admitted | Reject before SQL | Unperformed | Unperformed |
| 11.1 / 11.2 | New projections not admitted | Reject before SQL | Unperformed | Unperformed |
| 12.1 / 12.2 | New projections not admitted | Reject before SQL | Unperformed | Unperformed |
| 18 | New projections not admitted | Reject before SQL | Unperformed | Unperformed |
| 19 | 19c dictionary + verified GG layout | Covered | 19.32 SYSTEM/PDB, five metadata selections empty; data sources unavailable | Unperformed |
| 21 | 21c dictionary + verified GG layout | Covered | Unperformed | Unperformed |
| 23 / 23ai | 23 references redirect to 26ai; verified layout required | Covered | Unperformed except observed numeric 23.26.1 below | Unperformed |
| 26ai, observed numeric 23.26.1.0.0 | 26ai dictionary + verified GG layout | Covered as numeric 23 | SYSTEM/PDB, five metadata selections empty; data sources unavailable | Unperformed |
| Hypothetical numeric 26.x / unknown future majors | Branding is not numeric capability | Reject before SQL | Unperformed | Unperformed |

Five-part numeric version evidence is required. These conservative gates do not
assert that GoldenGate or the underlying views were introduced in 19c. No narrower
RU boundary was identified for the selected 19/21/26ai dictionary columns. Earlier
release projections have not been admitted; existing legacy APIs retain their
independent gates. No source probe/retry manufactures compatibility.

Read-only SQLcl/JDBC checks on 2026-11-19 observed 19.32.0.0.0 and 23.26.1.0.0,
SYSTEM, PDB CON_ID3, PRIMARY/READ WRITE. Each ALL/DBA identity query and accessible
catalog completed empty. This proves execution only for these login/container/
version selections; it does not establish GoldenGate absence, positive process
identity, populated heartbeat layouts, heartbeat/lag SELECT execution or Go-driver/
application qualification. No DDL/grants/container/lab changes were made.
Other accounts/roles, root/non-CDB/MOUNT/standby/RAC, patch levels, native 21 and
positive heartbeat/process cases remain unqualified. Unit fixture drivers are
scaffolding, not native Oracle evidence. Exact per-query matrix, JSON, hashes and
scope evidence are retained privately by the issue.

Oracle references checked 2026-11-19 (titles and applicability):

- Database Reference ALL_CAPTURE / related DBA_CAPTURE, CLIENT_NAME/PURPOSE/status:
  [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_CAPTURE.html),
  [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_CAPTURE.html),
  [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_CAPTURE.html).
- Database Reference ALL_GOLDENGATE_INBOUND / related DBA view, group/server/status:
  [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_GOLDENGATE_INBOUND.html),
  [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_GOLDENGATE_INBOUND.html),
  [26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_GOLDENGATE_INBOUND.html).
- Database Reference accessible object/column metadata:
  [19c ALL_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_OBJECTS.html),
  [19c ALL_TAB_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TAB_COLUMNS.html),
  [21c ALL_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_OBJECTS.html),
  [21c ALL_TAB_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/ALL_TAB_COLUMNS.html),
  [26ai ALL_OBJECTS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_OBJECTS.html),
  [26ai ALL_TAB_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_TAB_COLUMNS.html).
- [GoldenGate 19.1: Monitoring Oracle GoldenGate Processing](https://docs.oracle.com/en/middleware/goldengate/core/19.1/admin/monitoring-oracle-goldengate-processing.html),
  heartbeat/lag schemas, seconds, directional path identity, UTC timestamps and
  clock discrepancy behavior. Installed structures are independently verified;
  database version alone never implies this GoldenGate layout.