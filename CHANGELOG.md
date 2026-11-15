# Changelog

## Unreleased

- Add a connected-source SYSDATE civil clock fact for finite local redo-history windows, preserving reporting/container identity and keeping source-local labels separate from client UTC observations.

- Add exact redo groups, members, bounded history/archive-copy inputs, tablespace/file/free-extent/segment inventories, LONG-column attribution, default temporary groups, per-instance SGA components and ASM connection facts.
- Preserve raw byte/block units, optional columns, SQL NULL, instance/container scope and local Gregorian time bounds; validate every JSON dataset with its matching domain decoder before publication.

- Add an opt-in fixed-table hidden-parameter catalog with distinct raw session/instance values, decoded modifiability, description and bound exact-name/literal-substring filters.
- Expose complete typed readers, actual native rows and bounded atomic JSON through the responsibility package and thin facade; preserve the existing visible-underscore API and caller-owned scope/privileges.

## v0.13.0 (2026-11-14)

- Add scalar-row instance-wide GV$SYSTEM_PARAMETER selectors, complete typed readers, borrowed native rows/JSON and facade methods without changing published SYSTEM_PARAMETER2 list APIs.
- Add qualified GV$INSTANCE visibility facts with raw instance/container/status fields, explicit empty/unavailable states and no complete RAC membership or reachability claim.
- Add a pure exact ISMODIFIED=MODIFIED selector and document per-member collection/partial-result ownership without persistence inference or a generic SDK orchestrator.

## v0.12.0 (2026-11-14)

- Add a separate visible/configured hidden-parameter dataset with native rows/JSON and complete typed buffered readers for effective, hidden and stored SPFILE facts, preserving raw NULL/default/source/instance/container/list semantics.
- Add metadata-only password-file selectors, decoders, owned-reader and borrowed native/JSON methods, with the 12.2 source floor and documented 21c Exascale-column defaults; preserve explicit capability choices and absence/unavailable states.
- Document the punctual fact-method pattern and application developer/request workflow without introducing generic execution or application report-selection policy.
## v0.11.0 (2026-11-14)

- Move generic query/native-row/JSON ownership to go-oracle-session/rowset and delegate Oracle fact readers directly to that functional package.
- Remove go-oracle-info/rowset without compatibility aliases or forwarders. Direct generic dataset consumers must update their imports; Oracle-specific fact filters, typed values, provenance and facade methods remain here.
- Keep borrowed native database/sql handles separate from the existing owned Session.Stream lease lifecycle. Historical v0.10.0 remains immutable.
- Require released go-oracle-session v0.6.0 and Go 1.27.4 for the shared dataset implementation.

## v0.10.0 (2026-11-14)

- Add a thin non-owning native fact client with reused scalar identity properties and effective-instance/SPFILE parameter datasets as actual database/sql rows or bounded atomic JSON; preserve source/NULL/type/multivalue/SID semantics and all existing APIs.
- Persist the JSON-only result serialization rule, excluding genuine DBMS_OUTPUT and typed scalar/SQL-source values.

## v0.9.0 (2026-11-14)

- Add independent connected architecture, numeric-version and explicit-edition property readers with guarded source selection, typed precision/provenance and safe failure/cancellation retention; preserve all low-level APIs.

## v0.8.0 (2026-11-14)

- Add bounded oldest table/partition/subpartition optimizer statistics selection and typed decoding, preserving nullable values, parent identities and statistics scope with explicit legacy owner-admission limits.

## v0.7.0 (2026-11-14)

- Add native session parameter NAME/VALUE/ISDEFAULT selection without filtering or default inference, preserving the existing display-value parameter API.

## v0.6.1 (2026-11-14)

- Add a conservative legacy11g banner helper for explicit edition descriptions, preserving raw evidence and descriptive provenance without synthesizing codes or changing modern edition APIs.

## v0.6.0 (2026-11-14)

- Add observed edition and complete version-banner decoding plus current-container dictionary-component selectors/decoders, preserving exact nullable values, raw modification labels and legacy/full-column provenance.

## v0.5.0 (2026-11-13)

- Add bound ordinary-table optimizer statistics selectors with NULL-preserving NUM_ROWS and Gregorian database-local LAST_ANALYZED text.
- Add validated optional PARALLEL(n) exact-count hints while preserving the existing CountStatement output and identifier quoting.

## v0.4.0 (2026-11-13)

- Add seven-column execution identity selection and decoding, preserving independent nullable facts and raw IDs alongside validated numeric instance/container IDs; version and target admission remain caller-owned.

## v0.3.0 (2026-11-13)

- Add independent scalar database-name and role selectors and decoders, preserving exact SQL/text, typed NULL/no-row states and existing identity APIs.

## v0.2.0 (2026-11-13)

- Add read-only database/environment, service/object/count, ADR, RMAN, Data Guard, scheduler and resource/blocker selectors, plus bounded connected-cell selection, preserving existing typed APIs and caller-owned execution policy.

## v0.1.0 (2026-11-12)

- Add reusable engineered-system cell/configuration/alert facts, reviewed SQL selectors and complete-result/XML decoders; preserve exact values, NULLs, counts, conflicts, raw rejected XML and provenance. Applications retain authorization, transport recovery and presentation.

- Add scoped Oracle identity/platform and configured-memory contracts, exact provenance and regression fixtures.
