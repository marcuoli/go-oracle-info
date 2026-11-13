# Read-only fact selection

The statement packages select Oracle query text. They do not execute it, admit a database, change containers, choose credentials/drivers, retry failed reads or evaluate health/readiness. Callers establish release/view support, grants, intended scope, deadlines and a reader that rejects failed or truncated cursor prefixes. Execute only through the admitted reader; keep returned exact text, SQL NULL, empty sources, independently valid facts and errors separate.

| Package | Responsibility and scope |
| --- | --- |
| identity | Existing typed identity/version decoders; supplemental connected database/instance, container, full-version and PDB-state projections. Bootstrap uses 11g+ columns; CDB/PDB SQL requires known 12c+, VERSION_FULL requires known 18+. |
| database | Connected environment, configuration, exact/rounded GiB observations, encryption counts and database/instance NLS and initialization values. Callers select root/PDB/instance applicability. |
| services | Registered connected-container names, excluding internal session services; no listener or workflow inference. |
| objects | Invalid objects, owner/type/status inventory, type totals, schema/table discovery and exact counts. Owner filters are binds; dictionary identifiers retain case and quotes. CountStatement does not authorize the table or establish cutoff consistency/cost. |
| alertlog | Connected-instance recent error entries or grouped counts, using a bound positive seconds window (:1). EntriesSQL takes an explicit row bound. |
| backup | Bounded recent RMAN job status from the connected control-file view; no backup/restore/recovery execution. DATE labels retain no timezone. |
| dataguard | Connected role/protection/switchover and available lag observations. Missing rows are not zero lag, topology confirmation or DR acceptance. |
| scheduler | Job metadata for the exact bound owner (:1); no execution or job modifications. |
| performance | Point-in-time blocker and resource-limit observations; no recurring sampling or workload qualification. |
| engineeredsystem/catalog | Existing capability-selected sources plus an explicitly bounded connected V$CELL projection. A legacy projection omits CELL_TYPE only when the caller establishes that capability. |
| memory | Existing scoped typed configured-memory collection, guarded effective fallback and exact RAC aggregation; unchanged. |

EntriesSQL, JobsSQL and ConnectedCells accept row bounds 1 through 10000. A caller requiring a complete inventory must request a sentinel beyond its budget and reject overflow; an exactly full bounded sample is not proof that all source rows were retained. Unbounded dictionary selectors require a bounded query reader and caller-owned row/byte limits; never pass a failed prefix as a complete result.

Static numeric projections use TM9 with explicit decimal characters to preserve exact count text independent of session locale. Configured SGA/PGA stay separate from observed allocation. Empty allocation aggregates and missing rows must not be converted into known zero. Full parameter selection is raw inventory; comparison allowlists and expected differences belong to callers.

No new database capability or version/platform qualification is claimed by moving these selectors. Existing memory/identity and engineered-system APIs remain compatible. Fixture tests establish selection, binding and escaping contracts; native grants, driver/LOB behavior, mounted-state access and deployment qualification require consumer evidence.
