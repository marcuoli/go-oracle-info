# Qualified SQL patch release consistency

The `patches` package keeps `ReadHistory` and `ReadSQLPatch` inventories separate.
`DBA_REGISTRY_HISTORY` describes past upgrades/downgrades/patch actions. Its
`VERSION`, descriptions and row order do not establish the running binary RU.
`DBA_REGISTRY_SQLPATCH` records datapatch attempts, including failed applies,
rollbacks and retries. Neither inventory is reduced for display by this library.
Applications choose separate endpoint tables, alerts and operational policy.

## Read actual member versions

`instances.VersionsStatement`, `DecodeVersions`, `ReadVersions(Request)`,
`VersionsRows` and `VersionsJSON` observe all qualified visible `GV$INSTANCE`
rows. The non-owning facade provides `Client.InstanceVersions` and
`Client.InstanceVersionsJSON`. Existing visibility and connected-instance
identity APIs are unchanged. No member connections, fallback query, filter to
only OPEN members, physical RAC discovery or host patch inventory is performed.

The version dataset retains `INST_ID`, source `CON_ID` when available,
`INSTANCE_NUMBER`, name, host, status and raw `VERSION`. Oracle18+ additionally
selects `VERSION_FULL`; earlier supported sources retain explicit legacy precision
and an absent full-version field. Selected SQL NULL remains distinct from an
unselected column. All source rows, including duplicates and mounted members,
are retained. Duplicate/contradictory member identity cannot pass assessment.

`CurrentContainerID` is the local execution session's
`SYS_CONTEXT('USERENV','CON_ID')` on12+, separately from each native member
`ContainerID`. `SessionUser` and `IsDBA` likewise describe this local execution
session, even when repeated beside a remote member row. `CloudService` is selected
only on the reviewed numeric23+ path, using the attribute documented for26ai;
the Oracle19 SQL reference does not document this attribute, so it is absent there.
The source is explicit in `VersionResult.ContextSource` and the JSON envelope.
`CURRENT_CONTAINER_ID` remains VARCHAR2 text in native JSON, not an invented
native NUMBER. Integer member columns retain numeric JSON meaning.

The documented cloud-service values identify Autonomous services; an unknown
non-NULL value is also unqualified. A NULL value alone does **not** establish that
the database is self-managed. The caller must supply positive deployment admission
for `SelfManaged`, retain its proof and leave uncertain services unknown. Neither
wallet/service-name absence nor a default Boolean provides that admission.
Collecting `SESSION_USER`/`ISDBA` does not grant privileges or itself certify every
managed-service boundary.

Owned reads require a complete bounded `session.Result` reader and stable caller
SQL reference. Native rows are the actual borrowed cursor, with caller-owned
iteration/Err/Close/deadlines. JSON uses the same source/schema, shared rowset
consumption/finalization and complete domain validation before publishing any
bytes. Query, late row, conversion, close, cancellation and capacity failures
return no JSON or successful typed prefix. Limits include envelope metadata.

## Evaluate admitted observations

```go
report, err := patches.EvaluateConsistency(ctx, patches.ConsistencyRequest{
    At: evaluationTime,
    MaxAge: allowedAge,
    Versions: memberVersions,
    VersionsEvidence: patches.ConsistencyEvidence{
        Complete: versionsComplete, ObservedAt: versionsObservedAt,
    },
    CoverageVerified: qualifiedMemberCoverageVerified,
    SelfManaged: deploymentAdmissionVerified,
    Scopes: []patches.SQLPatchScope{{
        ContainerID: actualCurrentContainerID,
        Verified: dictionaryScopeVerified,
        Patches: completeSQLPatchInventory,
        Evidence: patches.ConsistencyEvidence{
            Complete: sqlPatchComplete, ObservedAt: sqlPatchObservedAt,
        },
    }},
})
```

This pure function performs no I/O. It owns detached copies of raw versions,
scopes and every SQL attempt, preserving source order, nullable fields and
provenance. Findings identify raw member/scope/candidate/failure rows by index.
The actual supplied current-container identity must match the selected SQL scope;
whole-database member `CON_ID=0` never invents a current container. Root/PDB-specific
member rows are not widened to another container. Coverage is qualified visibility
and caller admission, never proof that every configured RAC member is reachable.

`At` must be nonzero and `MaxAge` positive. Complete/fresh source evidence,
qualified version/source/precision, actual scope admission, self-managed deployment
admission and OPEN member state are required. Denied/missing/partial/stale/future,
mounted, managed/unverified, NULL full-version and unsupported evidence remain
`unknown`. The reviewed evaluator admits numeric18/19/21/23 metadata layouts;
this is source selection, not native qualification of all those releases.
11g/12c retain raw inventories but cannot establish this modern RU comparison.
Oracle26ai uses actual internal numeric23 (`23.26.x`), not a fabricated major26.

The evaluator resolves each patch ID/UID independently within each container,
using the source-local civil `ACTION_TIME`. IDs and `INSTALL_ID` are not chronology.
A later successful retry repairs that patch's earlier failed attempt. A successful
rollback removes its apply. Tied latest attempts remain unknown, including
conflicting actions; no arbitrary tie-breaker or timestamp timezone is invented.
A later successful cumulative release transition can supersede an older failed
release APPLY transition of the same major at an equal/higher numeric target level. Failed rollbacks require their own successful retry; no unrelated target-only apply proves their repair.
No such repair relation is inferred for an unrelated interim/JVM patch.

Only successful effective RU/RUI/RUR/CU apply candidates can establish a release
level. INTERIM and documented `J`-flagged JVM patches do not replace that level.
The latest unambiguous relevant transition is selected, rather than the largest
patch ID, lexicographic version or newest arbitrary patch. Later successful release
rollback without a later effective release apply remains unknown. Its target
version is not guessed to be the post-rollback version. Missing/ambiguous relevant
type/target/chronology remains unknown; all raw attempts stay available.

Outcomes are `matches_release_level`, `release_level_mismatch`, `sql_patch_errors`
and `unknown`. Exact five-component full-version equality is reported with
`ComparedComponents=5`. A differing qualified RU/RUR component is compared with
three components; differences only in MRP/recut sublevels are unqualified and
remain unknown (`ComparedComponents=0`). Raw full values are always retained.
Latest unresolved recorded failures remain explicit; old successes and failures
repaired by a later admitted successful transition do not falsely flag.

A mismatch supports investigation of incomplete patching, including possibly
omitted datapatch. It does not prove which command ran, establish all SQL-requiring
one-off/interim patch parity, certify OPatch inventory or authorize remediation.
Equal versions can coexist with an unapplied SQL-requiring one-off. No datapatch,
OPatch, DBMS_QOPATCH, scheduler job, restart or configuration change is executed.
Physical all-version/RAC/managed-service qualification is not claimed by fixtures.

## Oracle sources

Reviewed2026-11-16; match the actual release, source and caller scope before use:

- [19c V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html)
  and [26ai V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html): member state and VERSION_FULL source.
- [19c SQL patch registry](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY_SQLPATCH.html),
  [18c column introduction](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY_SQLPATCH.html)
  and [26ai SQL patch registry](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_REGISTRY_SQLPATCH.html): attempts, action/status/type/flags, target version and session ID meanings.
- [19c history](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY_HISTORY.html): historical upgrade/downgrade/patch facts.
- [26ai release numbering](https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/oracle-database-release-numbers.html): numeric23.26.x, RU/MRP/recut distinctions.
- [19c SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/SYS_CONTEXT.html)
  and [26ai SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/SYS_CONTEXT.html): local execution context, actual container and qualified cloud-service attribute.
- [19c DBMS_QOPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/19/arpls/DBMS_QOPATCH.html): SYS/OPEN restrictions and potential RAC job side effects explain why it is excluded from this path.