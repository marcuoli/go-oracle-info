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
