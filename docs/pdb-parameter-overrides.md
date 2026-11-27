# PDB parameter override evidence

Use `parameters.EvaluatePDBOverrides` for one caller-admitted direct PDB and
reporting instance. It takes fresh observations from the existing
`ReadEffective` and `ReadSPFile` APIs. `PDBOverridesJSON` independently
evaluates the same inputs within finite row and byte limits. Callers obtain
and pair both observations on the same admitted PDB connection. The SDK
owns no connection, container switch, file access or parameter change.
An independently valid source remains usable if the other is incomplete.

Use `PDBOverridesRequest` with the observed numeric version, actual PDB
`CON_ID`, an observation time and maximum age. Its `PersistenceMember`
uses `PersistencePDB` for the admitted scope and includes the reporting
instance ID, raw results, and completion/time evidence for each read.
A bounded JSON report includes source provenance and the raw
effective and stored rows. State applies to the finding's exact parameter
name. The report keeps reporting instance, actual container, stored SID,
list ordinal and SQL NULL. It does not resolve SID precedence.

| State | Evidence |
| --- | --- |
| `persistent_override` | A fresh, unambiguous stored clause for this PDB has `ISSPECIFIED=TRUE`. The raw SID shows its scope, including a dormant SID. The effective value can differ or be unavailable. |
| `memory_only_observed` | In the qualified scalar integer shape, Oracle reports one modified, PDB-modifiable effective row for the exact PDB and one unspecified wildcard row with NULL value and ordinal 0. Fresh complete all-SID stored evidence and independently verified current SPFILE startup are required. |
| `unknown` | The available observations do not establish either state. Examples include inherited values, empty or stale stored evidence, unsupported runtime shapes, and contradictory clauses. |

The two reads have no shared snapshot. Set `RuntimePairVerified` only when
the caller has independently established that the paired observations
represent the same configuration state. Freshness alone allows a change
between reads. Without that admission a memory-only state remains unknown.
The effective modification flag does not identify the `ALTER SYSTEM` scope
or prove which command produced the current state.
A persistent clause may coexist with a different current value. Keep
those values visible independently. The existing restart-persistence
evaluator still returns unknown for PDB scope. Ordinary root collection
can read [visible PDB values](container-system-parameters.md). The 26ai
root view omitted the PDB's confirmed stored clause.

## Qualification

Oracle documentation checked on 2026-11-27 describes effective flags,
stored clauses, and the `SCOPE` / inheritance behavior. The implementation
reuses the original SQL statements and projections. 9.2, 10.1, 10.2,
11.1 and 11.2 reject PDB evaluation. For 12.1, 12.2, 18c, 19c, 21c and numeric
23c/23ai/26ai, the existing effective and stored projections supply the
exact per-row evidence. Only positive stored evidence supports a persistent
classification. The following exact release and parameter pairs qualify
for the memory-only inference when the same evidence gates pass.

| Observed Oracle version | Qualified parameters |
| --- | --- |
| 12.2 `12.2.0.1.0` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |
| 18c `18.4.0.0.0` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |
| 19c `19.32.0.0.0` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |
| 21c `21.3.0.0.0` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |
| 23ai `23.9.0.25.07` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |
| 26ai `23.26.3.0.0` | `open_cursors`, `optimizer_index_cost_adj`, `optimizer_dynamic_sampling` |

All other release and parameter combinations leave the memory-only state
unknown. Other accounts, RAC and other topology or startup configurations
retain separate qualification boundaries. Positive stored evidence can
establish a persistent override independently of this matrix.

The expanded 19c, 21c and 26ai runs used SYSDBA with an isolated single-instance CDB and a
direct read-write disposable PDB on 2026-11-27. Each parameter was checked
while inherited, after `MEMORY`, after setting a distinct `SPFILE` value,
after reopen, after `BOTH`, after resetting the stored clause, and after
reopening to inherit again. All three parameters reproduced the exact
effective and stored shapes above on all three versions. The CDB root values
stayed at their baselines. The 19c fixture emitted a warning when opening
its disposable PDB. The direct reads, changes and reopen controls completed.
The 26ai clone initially hit a datafile deletion/deadlock error. The follow-up
found zero database references and removed two empty task-owned directories.
The retry passed after the seed was available. The disposable PDBs were
removed and all fixtures returned to their original stopped state.

The 12.1, 12.2, 18c and 23ai runs used the same native SQL*Plus controls
in isolated, read-write, single-instance SYSDBA disposable PDBs on
2026-11-27. All three parameters passed in 12.2, 18c and 23ai. The
12.2 `optimizer_dynamic_sampling` baseline and post-reset reopen
reported the PDB's own `CON_ID` and `ISMODIFIED=FALSE` with the root
value. The memory-only phase had `ISMODIFIED=MODIFIED` and an
unspecified stored placeholder for the same PDB.

On 12.1 `12.1.0.2.0`, `GV$SPPARAMETER` returned `CON_ID=0` from the
direct PDB for both unspecified and explicitly stored rows. This leaves
all three parameters unqualified for memory-only inference. The first
12.1 attempt failed on the password length limit before opening a PDB.
The initial 12.2 PDB open failed with an asynchronous I/O error
(`ORA-27090`). The 12c retries emitted open warnings and completed all
scope and reopen controls. All four fixtures returned to their original
stopped state after the disposable PDBs and files were removed.

The 12c numeric values are the patch-set versions from `V$INSTANCE.VERSION`.
That field does not distinguish individual bundle patches. The later runs
recorded `VERSION_FULL`. Go tests exercise the evaluator using the qualified
row shapes. These four runs provide native SQL evidence, with driver and
application qualification recorded separately as untested.

A single-instance 23.26.3.0.0 SYSDBA disposable PDB ran the following
`OPEN_CURSORS` cases. The root stayed at 300. The PDB was dropped and the
fixture returned to its original stopped state.

| Case | Effective value | Stored value | Observation |
| --- | --- | --- | --- |
| Inherited | 300, whole-CDB row | NULL, unspecified | Unknown for explicit setting |
| `SCOPE=MEMORY` | 733, PDB row marked `MODIFIED` | NULL, unspecified | Memory-only value observed |
| `SCOPE=SPFILE` after the memory change | 733 | 734, specified | Persistent override with a different effective value |
| Reopened | 734 | 734, specified | Persistent override |
| `SCOPE=BOTH` | 733 | 733, specified | Persistent override |
| `RESET SCOPE=SPFILE` while open | 733, PDB row marked `MODIFIED` | NULL, unspecified | Same observable memory-only state as the first memory change |
| Reopened after reset | 300, whole-CDB row | NULL, unspecified | Unknown for explicit setting |

Existing direct 23.26.1.0.0 PDB sessions also reported a stored
`spatial_vector_acceleration` clause with wildcard SID. The 19.32 direct
PDB comparison supplied no positive stored clauses. The existing Go
readers and bounded evaluator JSON also passed on both direct SYSDBA
connections using go-oracle-session v0.25.1 compatibility V2. Neither a false
`ISDEFAULT` flag nor a successful empty read proved inheritance or the
absence of an override.

- [Oracle 12.1 SPPARAMETER](https://docs.oracle.com/database/121/REFRN/GUID-190DC64D-1EDE-4C35-B6F4-70BAAF46F0BF.htm)
- [Oracle 12.2 ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/ALTER-SYSTEM.html)
- [Oracle 18c ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/18/sqlrf/ALTER-SYSTEM.html)
- [Oracle 19c parameter reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/OPEN_CURSORS.html), [optimizer index cost](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/OPTIMIZER_INDEX_COST_ADJ.html), and [dynamic sampling](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/OPTIMIZER_DYNAMIC_SAMPLING.html)
- [Oracle 21c parameter reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/OPEN_CURSORS.html), [optimizer index cost](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/OPTIMIZER_INDEX_COST_ADJ.html), and [dynamic sampling](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/OPTIMIZER_DYNAMIC_SAMPLING.html)
- [Oracle 26ai SYSTEM_PARAMETER2](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER2.html)
- [Oracle 26ai SPPARAMETER](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SPPARAMETER.html)
- [Oracle 26ai ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/ALTER-SYSTEM.html)
- [Oracle 26ai OPEN_CURSORS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/OPEN_CURSORS.html), [optimizer index cost](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/OPTIMIZER_INDEX_COST_ADJ.html), and [dynamic sampling](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/OPTIMIZER_DYNAMIC_SAMPLING.html)
