# Data Guard observations

`dataguard` exposes eight independent read-only datasets: `DatabaseContext`,
`StandbyLogs`, `Processes`, `RecoveryProgress`, `ArchiveGaps`,
`ArchiveDestinations`, `ArchiveDestinationStatus` and `ArchiveProgress`.
Every family provides `...Statement`, `Decode...`, `Read...`, `...Rows` and
`...JSON`. The root `Client.DataGuard...` facade provides the native rows and
JSON companions with uncached version observation. Existing `StatusSQL` and
`LagSQL`, and the published `redo` APIs, are unchanged.

Owned typed reads accept a complete `Reader` and caller SQL reference:

```go
facts, err := dataguard.ReadStandbyLogs(ctx, admittedReader, dataguard.Request{
    OracleVersion: observedNumericVersion,
    Filter: dataguard.Filter{Instances: admittedReporterIDs},
    Reference: callerReference,
})
```

Use `DatabaseContextRequest` for the separate `V$DATABASE` singleton, and
`ArchiveProgressRequest` with `ArchiveProgressOptions` for the incarnation
snapshot. All typed values are `session.Value`: SQL NULL is `.Null`, with exact
text retained in `.Text`. SCNs/DBIDs use decimal text beyond int64; actual native
integer columns retain numeric JSON kinds. Recovery SOFAR/TOTAL may be fractional
and retain exact Oracle TM9 text. Raw process NAME/ROLE/ACTION are modern
`GV$DATAGUARD_PROCESS` values, without a legacy PROCESS/STATUS translation.

Rows are actual caller-native cursors. Callers own Next/Scan/Err/Close, deadlines
and finite budgets. JSON independently consumes its own cursor through
`go-oracle-session/rowset`, validates every domain row and returns the standard
`{provenance,data}` envelope only after complete success. Query, late scan,
terminal, close, cancellation, decoding and capacity failures return no prefix.
A pure decoder cannot certify transport completeness. A complete empty view
means no rows in that admitted source, not general availability or health.

## Scope and correlation

`INST_ID` is the reporting instance, not complete physical RAC membership.
Preserve repeated GV observations and require caller-proven agreement before
correlating database-wide facts. `V$DATABASE.CON_ID` is that source row's scope,
not current session container evidence. Observe existing identity Architecture
separately when session/root/PDB admission is needed. Database context is a
separate observation; it is never cross joined onto every member row. All calls
are non-atomic and applications retain freshness, scope and endpoint admission.

Primary online logs are the existing `redo.ReadGroups`/`Client.RedoGroups`.
Standby logs preserve GROUP#/THREAD#/SEQUENCE#, primary-assignment DBID (including
UNASSIGNED), BYTES/USED, ARCHIVED/STATUS and paired FIRST/NEXT/LAST SCN/time fields.
Oracle defines ACTIVE+ARCHIVED=YES as currently receiving; ACTIVE+NO is complete
and waiting to archive. Primary CURRENT and standby ACTIVE have different
meanings. Applications correlate threads, sequences and database incarnation,
then display process/recovery mode evidence separately. These rows do not infer
synchronization, recovery readiness, protection or a guaranteed apply position.

`ArchiveGaps` is Oracle's current blocking gap observation, not an exhaustive
history or proof of gap absence. Destination/status families select only
DESTINATION IS NOT NULL and preserve source status, errors and configuration.
APPLIED_SCN is selected from the documented archive destination source. SYNC,
AFFIRM and destination synchronization flags remain distinct raw observations;
ASYNC is not classified using a SYNC-only acceptance rule.

## Latest archived and recovery observations

`ArchiveProgressOptions` requires both `ResetlogsChange` and `ResetlogsTime`
from the same admitted database context; time is exact database-local
`YYYY-MM-DDTHH:MM:SS`. There is no implicit seven-day or other historical cutoff.
The selector binds the incarnation, restricts REGISTRAR=RFS, and independently
ranks RECEIVED and APPLIED=YES by sequence within reporter/container/thread/kind.
All maximum-sequence destination copies remain tied rows. Their times and SCNs
come from the actual row; an independent MAX(time) is never paired with MAX(seq).
Deleted/status/name remain raw metadata; RFS registration does not prove current
file availability. APPLIED=IN-MEMORY is not classified as APPLIED=YES.

`ArchiveProgressIdentities` groups logical identities while preserving all
Copies; disagreements in incarnation/SCN/time metadata set Conflicting. It never
merges reporters or chooses a preferred destination. Active SRL receiving/apply
belongs to standby-log/process observations; it is not invented as an archived
record. This snapshot is bounded by the admitted reader/JSON budgets, not SQL
row truncation, and does not establish complete history.

`LatestLastAppliedRedo` requires a complete recovery result. It retains the
actual maximum-TIMESTAMP rows per reporter/container, every tie, and separately
Undated rows. COMMENTS remains raw and is not interpreted as a sequence, number
or paired with an unrelated maximum. Both helpers preserve observation order.

## Query release matrix

The following applies independently to all eight new families, with static
source/decoding checks per family. Documentation, native SQL execution and actual
application-driver execution are different evidence levels.

| Numeric release family | Source selection | Documentation and static checks | Native SQL / driver |
| --- | --- | --- | --- |
| 9.2 | Unsupported without SQL | New projections unverified | Unqualified |
| 10.1 / 10.2 | Unsupported without SQL | New projections unverified | Unqualified |
| 11.1 / 11.2 | Unsupported without SQL | New projections unverified | Unqualified |
| 12.1 / 12.2 | Unsupported without SQL | New projections unverified | Unqualified |
| 18 | Unsupported without SQL | New projections unverified | Unqualified |
| 19 | Explicit reviewed projection | All selected columns checked in 19c references | See qualification below |
| 21 | Explicit reviewed projection | All selected columns checked in 21c references | Unqualified |
| 23 | Explicit reviewed projection | All selected columns checked in 23ai references | See qualification below |
| Hypothetical numeric 26.x | Unsupported without SQL | Product branding is not numeric release evidence | Unqualified |

Full observed numeric versions are retained; branding is never a version test.
Numeric 23.26.1 remains family 23. Source selection does not certify every patch,
RU, login, OPEN/MOUNT state, non-CDB/root/PDB or member configuration. No selected
column has a narrower documented patch boundary in the four reviewed reference
sets. The 26ai reference set was also checked for observed numeric23.26.x; it does not establish numeric26.x. Newer optional process fields, including APPLY/TENANT_KEY, are omitted.
A missing privilege/column produces the original error, with no query probe,
retry, cache or alternative source fallback. Earlier releases are not newly
qualified by these APIs. Existing unrelated selectors retain their own contracts.

Read-only native qualification on 2026-11-18 executed all eight SDK `Read...`
selectors/decoders against four existing admitted entries: Oracle19.32.0.0.0
PRIMARY READ WRITE as SYSTEM; two Oracle23.26.1.0.0 PRIMARY READ WRITE entries as
SYSTEM; and Oracle23.26.1.0.0 PHYSICAL STANDBY MOUNTED as SYS AS SYSDBA. All32
reads completed. This used official go-ora/v2 v2.9.0 through the consumer's
production QueryWithOptions path and complete bounded SDK Reader. Source-file
hashes matched before/after. It includes APPLIED_SCN and the full selected process
projection. No separate SQLPlus/SQLcl client qualification is claimed. Empty
sources were observed on their actual roles; the mounted standby supplied
recovery and archived-progress rows. No physical RAC, PDB/session-container,
legacy-release or different-account/state combinations are established by this
proof. No database state, lifecycle, grants or data were changed.

Official references checked 2026-11-18 (each page was read for 19, 21, 23 and 26):

- [V$DATABASE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE.html)
- [V$STANDBY_LOG](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-STANDBY_LOG.html)
- [V$DATAGUARD_PROCESS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATAGUARD_PROCESS.html)
- [V$RECOVERY_PROGRESS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-RECOVERY_PROGRESS.html)
- [V$ARCHIVE_GAP](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ARCHIVE_GAP.html)
- [V$ARCHIVE_DEST](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ARCHIVE_DEST.html)
- [V$ARCHIVE_DEST_STATUS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ARCHIVE_DEST_STATUS.html)
- [V$ARCHIVED_LOG](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ARCHIVED_LOG.html)
- [GV$ scope and INST_ID](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/about-dynamic-performance-views.html)