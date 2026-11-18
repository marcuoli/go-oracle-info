# Database-visible GoldenGate facts

The goldengate package exposes ReadCapture and ReadApplyCoordinators over a
caller-owned complete bounded Reader, observed OracleVersion and SQL Reference.
DecodeCapture and DecodeApplyCoordinators validate full supplied shapes but
cannot prove transport completion. Source admission, credentials/grants,
endpoint/container selection, deadlines, retries and recovery stay with callers.

CaptureRows / ApplyCoordinatorsRows return actual caller-native *sql.Rows.
Callers own Next, Scan, Err, Close and finite stream budgets. CaptureJSON /
ApplyCoordinatorsJSON independently consume their own native cursor through
go-oracle-session/rowset. No payload survives query, iteration, close, decoding,
cancellation or capacity failure. Root Client methods GoldenGateCapture /
GoldenGateApplyCoordinators and their JSON counterparts independently observe
the source version; JSON accepts optional JSONLimits.

Sources: GV$GOLDENGATE_CAPTURE and GV$GG_APPLY_COORDINATOR over qualified visible
instances in the admitted container scope. They do not prove complete physical
RAC membership, external process state, Classic process coverage, GoldenGate
version, throughput or end-to-end synchronization. Do not subtract capture and
apply message numbers to infer lag. Capture state can include a semicolon-
separated network state. IDLE and WAITING FOR REDO may be normal. Counters are
cumulative since process startup; the library applies no health/scoring policy.

Common source floor: conservatively 11.2.0.4, when the apply coordinator view is
documented. This does not assert when capture was introduced. A complete
five-part numeric version is required. CON_ID is unprojected (nil) on 11g, distinct
from selected SQL NULL. Oracle NUMBER message positions use an explicit TM9 text projection with fixed
NLS numeric characters, preserving exact values beyond int64 in native rows
and JSON. Message numbers remain exact text; counters are exact
native integers in JSON. Invalid/overflowing JSON values fail atomically.
Original order/duplicates and NULL/empty values remain. Successful zero-row
observations have Complete=true. Unsupported/denied/failed reads never become
complete empty sources.

## Oracle references checked 2026-11-16

- V$GOLDENGATE_CAPTURE, Database Reference 11.2:
  https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_1169.htm
- V$GG_APPLY_COORDINATOR, Database Reference 11.2, explicit 11.2.0.4 floor:
  https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_1163.htm
- Database Reference 19c:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-GOLDENGATE_CAPTURE.html
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-GG_APPLY_COORDINATOR.html
- Database Reference 26ai:
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-GOLDENGATE_CAPTURE.html
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-GG_APPLY_COORDINATOR.html
- GV$ qualified instance and CONTAINER_DATA semantics:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/about-dynamic-performance-views.html

References establish source/schema meanings, not native qualification of every
release/topology. Unit fixtures exercise selection/failures; configured integrated
processes require separate live qualification.

## Database-only schema and administrator properties

`Client.GoldenGate(ctx)` returns independent `ConfiguredSchema` and
`Administrators` properties. The functional equivalent is
`goldengate.ReadProperties(ctx, admittedReader, goldengate.Request{...})`.
`ReadNativeProperties` borrows an admitted Queryer while retaining the same
caller SQL reference and delegating buffering/finalization to shared rowset.

```go
facts, err := oracleinfo.New(admittedConn).GoldenGate(ctx)
if err != nil {
    return err
}
// facts.Administrators.Rows contains exact Username, PrivilegeType and
// GrantSelectPrivileges observations, including multiple grants/users.
// facts.ConfiguredSchema.State is goldengate.SchemaUnavailable.
// facts.ConfiguredSchema.Name is nil: unobserved configuration, not SQL NULL.
```

The administrator source is an explicit three-column projection of
`DBA_GOLDENGATE_PRIVILEGES`: `USERNAME`, `PRIVILEGE_TYPE`,
`GRANT_SELECT_PRIVILEGES`, ordered by those same columns. This reports privileges
recorded by `DBMS_GOLDENGATE_AUTH`; it does not establish a current login,
Extract/Replicat use, a running installation, object ownership or GGSCHEMA.
CREATE_TIME is outside this requested projection. No usernames are normalized,
trimmed, deduplicated or guessed from conventional names. NULL privilege fields
and repeated source rows survive. NULL/empty usernames or malformed final rows
reject the complete result.

`ConfiguredSchema` is explicitly unavailable in database-only operation, with
reason `not_exposed_by_database_source`. A single administrator, a capture/apply
user or a heartbeat table owner is not promoted to the configured value. There
is no fabricated GGSCHEMA SQL column, GLOBALS parser, file/REST access or default
GGADMIN value. Oracle documents GGSCHEMA as a GoldenGate GLOBALS parameter;
heartbeat schema lookup also depends on GoldenGate version, process and container.

`Client.GoldenGateJSON` encodes the complete typed properties, with a separate
final encoded-byte check. `GoldenGateAdministratorsRows` is the actual native
administrator cursor; `GoldenGateAdministratorsJSON` uses the standard
`{provenance,data}` dataset envelope. Functional `ReadAdministrators`,
`DecodeAdministrators`, `AdministratorsStatement`, `AdministratorsRows` and
`AdministratorsJSON` are also available. Raw cursor callers own Err/Close and
budgets; typed/native buffering and JSON publish no successful prefix on query,
late scan, terminal, close, decoding, cancellation or capacity failure.

The dictionary source is the caller-admitted current container. There is no
CDB/PDB/member fan-out, container switch, GV row cloning or USER/ALL fallback
when DBA access is denied. A complete empty result means no rows visible in that
admitted source; it does not mean GoldenGate is absent. Existing capture/apply
APIs and their independent version gates are unchanged.

### Administrator query compatibility

| Numeric release | Source/documentation | Static selection/decoding | Native SQL and driver |
| --- | --- | --- | --- |
| 9.2 | No reviewed privilege projection | Unsupported without SQL | Unqualified |
| 10.1 / 10.2 | No reviewed privilege projection | Unsupported without SQL | Unqualified |
| 11.1 / 11.2 | No reviewed privilege projection | Unsupported without SQL | Unqualified |
| 12.1 | Views introduced in 12.1.0.1; this projection not yet reviewed there | Unsupported without SQL | Unqualified |
| 12.2 | All three fields documented | Explicit source/decoder checks | Unqualified |
| 18 | All three fields documented | Explicit source/decoder checks | Unqualified |
| 19 | All three fields documented | Explicit source/decoder checks | Qualified below on the admitted version/account/PDB scope |
| 21 | All three fields documented | Explicit source/decoder checks | Unqualified |
| 23 (including observed 26ai numeric 23.26.x) | All three fields documented in 23/26ai references | Explicit source/decoder checks | Qualified below on the admitted version/account/PDB scope |
| Hypothetical numeric 26.x | Branding does not establish a numeric release | Unsupported without SQL | Unqualified |

Observed full five-part versions are retained. Documentation and static fixtures
establish neither native execution nor account/OPEN-MOUNT/container/platform
qualification. No narrower selected-column patch boundary was identified in the
reviewed 12.2/18/19/21/23/26ai references. Exact native coverage is recorded
separately; unsupported releases never trigger a trial query.

Official references checked 2026-11-18:

- [GGSCHEMA](https://docs.oracle.com/en/database/goldengate/core/26/reference/ggschema.html), GLOBALS configuration scope and qualified syntax.
- [Oracle heartbeat schema behavior](https://docs.oracle.com/en/middleware/goldengate/core/21.3/gclir/add-heartbeattable-1.html), Oracle-specific user/container/process rules.
- [DBA_GOLDENGATE_PRIVILEGES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_GOLDENGATE_PRIVILEGES.html) and [ALL_GOLDENGATE_PRIVILEGES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_GOLDENGATE_PRIVILEGES.html), matching columns and privilege meanings. Both were also read for 12.2, 18, 21, 23 and 26ai.
- [12.1 release changes](https://docs.oracle.com/database/121/REFRN/GUID-509A6343-5882-4260-BAD0-DC6B2BDC8301.htm), privilege-view introduction in 12.1.0.1; identity/introduction evidence alone does not qualify this projection.
Administrator native qualification on 2026-11-18 executed ReadAdministrators and
ReadProperties on each existing admitted GG WSLC database: Oracle19.32.0.0.0 as
SYSTEM in PDB CON_ID3 returned one administrator row; Oracle23.26.1.0.0 as SYSTEM
in PDB CON_ID3 returned zero rows. Both reported PRIMARY and READ WRITE. A zero
privilege-view result does not establish absence of a GoldenGate installation. Exact numeric
versions, session users, connected container, database role/open mode, selected
SQL/result SHA256 and source-file hashes were retained privately. This used
an isolated caller-owned harness with the official go-ora/v2 v2.9.0 connector,
released session v0.10.0 and the issue63 SDK source; no library connection or
configuration ownership was added. The credentials inventory was read in place
and its hash remained unchanged. No database/container state, configuration,
users, grants or data were changed. These reads do not qualify other accounts,
legacy/21 releases, physical RAC or GoldenGate's external configuration value.