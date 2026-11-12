# Repository guidance

This is a reusable first-party Oracle facts library. Keep public responsibility packages small. Applications provide an admitted reader/scope; no code here may open connections, change destinations/containers, select credentials, install drivers, or own UI/scoring/recovery policy.

Use explicit SQL aliases, reviewed static source/version choices, exact numeric text and typed NULL/absence/provenance. Preserve independently valid facts; never return a truncated source as complete. Keep configured parameters separate from observed consumption and per-instance values separate from proven RAC totals. Comments explain constraints without private paths, credentials or consuming application names.

Every change uses a typed, area-labeled issue with `## Validation and acceptance`, an issue branch and reviewed PR plus CHANGELOG.md. Keep canonical source on I: and recorded temporary worktrees/logs/caches on project-first V:. Preserve unrelated work. Finish authorized integration, canonical synchronization and exact Git worktree cleanup before closure.

Use compatible official upstream dependencies and released first-party module versions. No patched third-party copies, permanent local replace directives or unqualified native compatibility claims. Implement shared mechanisms only for real consumers. Preserve UPL notices and keep private provenance evidence outside the repository.

Validate formatting, tests, vet, module verification, build, race and advisories with recorded compiler/settings and approved V: storage. Libraries use conventional immutable module tags; public distribution is a separate gate from private implementation history.
