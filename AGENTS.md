# Repository guidance

This is a reusable first-party Oracle facts library. Keep public responsibility packages small. Applications provide an admitted reader/scope; no code here may open connections, change destinations/containers, select credentials, install drivers, or own UI/scoring/recovery policy.

Use explicit SQL aliases, reviewed static source/version choices, exact numeric text and typed NULL/absence/provenance. Preserve independently valid facts; never return a truncated source as complete. Keep configured parameters separate from observed consumption and per-instance values separate from proven RAC totals. Comments explain constraints without private paths, credentials or consuming application names.

Every change uses a typed, area-labeled issue with `## Validation and acceptance`, an issue branch and reviewed PR plus CHANGELOG.md. Keep canonical source on I: and recorded temporary worktrees/logs/caches on project-first V:. Preserve unrelated work. Finish authorized integration, canonical synchronization and exact Git worktree cleanup before closure.

Use compatible official upstream dependencies and released first-party module versions. No patched third-party copies, permanent local replace directives or unqualified native compatibility claims. Implement shared mechanisms only for real consumers. Preserve UPL notices and keep private provenance evidence outside the repository.

Validate formatting, tests, vet, module verification, build, race and advisories with recorded compiler/settings and approved V: storage. Libraries use conventional immutable module tags; public distribution is a separate gate from private implementation history.

## Result APIs and serialization
Expose simple non-owning domain properties and companion complete native rows/JSON methods over caller-admitted handles. Hide source/version/reference wiring in the facade without capability caches, connection/container/driver ownership, unsafe lease escapes or generic query registries. Raw *sql.Rows must be the actual caller-native cursor; callers own iteration/Err/Close and budgets. JSON owns its separate cursor and publishes nothing on any iteration/scan/close/cancellation/budget failure. Preserve SQL NULL, native numeric meaning, raw text, ordering and provenance.

NEVER serialize query result data as CSV or plaintext; only genuine DBMS_OUTPUT may be textual output. Typed string columns/scalar properties and SQL-source strings are not formatted resultsets. Use standard JSON, keeping diagnostics separate. Preserve immutable historical releases.

Keep effective instance settings, session settings and stored SPFILE definitions distinct. SPFILE reporting INST_ID is not SID applicability; preserve wildcard/defaults, overrides and dormant SID clauses without inferring effective precedence or complete file/RAC/container coverage. Test-only database/sql fixture drivers are unit scaffolding, never production cursor fabrication or native Oracle qualification.

Generic query execution, native row consumption and JSON datasets belong exclusively in go-oracle-session/rowset. Domain facts delegate there; do not implement or preserve an info/rowset compatibility package. Keep Oracle SQL/filter/schema/provenance here. Borrowed caller-native rows do not expose an owned Session.Stream lease or private cursor.

Follow docs/developer-guide.md for punctual fact additions. Owned buffered paths
use admitted session.Statement/complete session.Result readers and caller refs;
share domain selection/schema/provenance with borrowed rows/JSON. Validate all
rows before returning a result, including post-query cancellation. Preserve raw
NULL/default flags, absent optional columns and unavailable/missing-name states.
Hidden facts use the reviewed visible underscore source; no all-default X$ scan.
Password-file APIs expose metadata only, with documented version/capability
selection; never inspect users/verifiers/file contents or flush metadata caches.
Applications retain non-default/explicit-name report unions and peer selection.