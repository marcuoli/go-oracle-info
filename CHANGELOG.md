# Changelog

## Unreleased

- Add 55 Oracle Support SuperCluster QFSDP observations with hardware model scope and documented release dates, including the final Q1 2026 bundle (#167).
- Add separate FRA, restore-point, incarnation, promotion-SCN, and time zone file observations (#152).

## v0.32.0 (2026-11-24)

- Add independent control-file status, recovery, backup-mode, and datafile-header facts (#151).

- Refresh Oracle Database and Exadata patch metadata from the November 24 MOS captures. Add confirmed numbers and dates for 27 Oracle 11.2.0.4 Grid Infrastructure PSUs (#160).
- Read recorded database block corruption ranges with exact file, block and change numbers through the storage API and Client (#158).

- Read recorded PDB plug-in violations with exact nullable fields and release-aware container scope (#150).

## v0.31.0 (2026-11-24)

- Add bounded CDB root PDB visibility, session location, and per-PDB CDB_USERS account selectors. (#149)

## v0.30.0 (2026-11-24)

- Read raw object status for a finite batch of exact dictionary keys while retaining subobject and edition evidence (#145).

- Read exact Oracle-maintained role metadata from the connected dictionary without depending on role grants (#143).

## v0.29.1 (2026-11-23)

- Update go-oracle-session to v0.23.0 and refresh four indirect dependency versions. Verify source and build compatibility on Go 1.27.4 (#141).

- Use the managed MSYS2 UCRT compiler for Windows race validation (#141).

- Review the four bundled Oracle catalogs against Oracle web sources and retained My Oracle Support evidence (#141).

## v0.29.0 (2026-11-23)

- Document every public package and the retained compatibility entry points. Standardize native Client method pairs for invalid objects and GoldenGate (#137).

- Move observed SGA/PGA allocations and components into memory and file-capacity facts into storage. Preserve existing imports, types and behavior (#135).

## v0.28.0 (2026-11-23)

- Select scoped PUBLIC object and column grants when either recorded account is non-Oracle-maintained (#133).

- Catalog documented predefined Oracle roles for 9.2 through 12.1.0.1 (#131).

## v0.27.0 (2026-11-23)

- Allow exact PUBLIC in classified grant reads and preserve granted-role maintenance metadata (#129).
- Catalog documented predefined Oracle accounts for 9.2 through 12.1.0.1 (#126).

## v0.26.0 (2026-11-23)

- Read direct object and column privileges and role memberships for named users and roles, with release-aware grant options (#125).

- Save a reusable MOS refresh prompt alongside the documentation skill (#123).

## v0.25.2 (2026-11-23)

- Regenerate private recommendations, Gold Images and AutoUpgrade JSON from local MOS captures and reviewed archived HTML (#119).

## v0.25.1 (2026-11-23)

- Update go-oracle-session to v0.19.0 and refresh five stable indirect dependency versions. Verify the Go 1.27.4 source and build requirements (#117).
- Review the four Oracle catalogs and refresh their snapshot date while retaining verified facts and original capture dates (#117).

- Add the repository-scoped go-oracle-info-docs skill for Oracle documentation, catalog, and MOS evidence workflows (#115).

## v0.25.0 (2026-11-22)

- Resolve Oracle patch platform aliases, read reviewed Gold Image references and decode AutoUpgrade metadata (#111).

- Load reviewed Oracle patch recommendations and look up an exact release train and home (#109).

## v0.24.0 (2026-11-22)

- Select exact and latest Oracle database patch packages by release train, family and named platform (#105).

## v0.23.0 (2026-11-21)

- Review verified existing Oracle source captures for publication and retain the original capture dates (#102).

- Add opt-in direct-object diagnostics for reviewed engineered-system catalog statements (#99).
- Refresh release catalogs from directly reviewed Oracle Support captures and public Oracle sources before publication (#98).

- Add the SuperCluster QFSDP catalog and require an official-source, history-preserving catalog refresh before each module publish (#91).
- Add read-only logical database DBID and name from Oracle 9.2 onward, with the raw CDB flag from 12.1 (#84).

- Reuse dictionary inventory validation and buffered budget checks, simplify native fact JSON opening and verify options-based API parity (#87).
- Add an offline Oracle Database, Exadata and ODA release catalog seeded from a local patch export and verified Oracle sources (#86).
- Use `local-patch-export` for imported catalog provenance and document empty patch artifact lists (#96).
- Add numeric-release-aware instance visibility and scalar system parameter facts from Oracle 9.2 onward, including optional source columns, whole-database filter selection and distinct read outcomes (#83).
- Add independent PDB-scoped scalar parameter, Resource Manager sample and memory gauge facts (#82).
- Add version-aware per-member container inventory and independently readable PDB saved states (#81).
- Organize complete Oracle facts in uniform source files with their SQL, decoding and native APIs together, preserving compatibility symbols and removing superseded files (#76).

- Simplify exact RAC GiB rounding after byte aggregation and share integer-text validation across fact decoders (#77).
- Share bounded native fact JSON through go-oracle-session/rowset and complete the named fact APIs and options-based forms while retaining existing callers and payloads (#76).

## v0.22.0 (2026-11-20)

- Add exact temporary tablespace free-space observations with separate session and owning-instance scope (#74).

## v0.21.0 (2026-11-19)

- Add independent GoldenGate Extract/Replicat identity and attachment facts, structurally verified accessible heartbeat discovery, and complete typed/native/atomic JSON heartbeat and signed lag/freshness evidence without schema guessing, process-health policy or connection ownership (#72).

## v0.20.0 (2026-11-18)

- Add complete tablespace quota and independent direct system-privilege fact APIs, preserving exact numeric/NULL values, raw dropped/container flags, scoped admission and atomic bounded results (#70).

## v0.19.0 (2026-11-18)

- Correct the fact API compatibility guide's guarded memory source reference to V$PARAMETER, matching the unchanged selector (#65).

- Add complete typed, admitted-reader, caller-native rows and atomic JSON APIs for operational and compatibility fact datasets, preserving existing SQL/public APIs and explicit sample/scope/value semantics (#65).

- Document one implementation contract and explicit package/compatibility-selector migration map; reuse the shared rowset converter for identity and metadata without changing existing APIs or SQL (#65).

- Add database-only GoldenGate properties with exact administrator privilege rows and separate explicitly unavailable configured-schema evidence; expose complete typed reads, actual native rows and atomic JSON without inferring GGSCHEMA from users or taking configuration/connection ownership (#63).

## v0.18.0 (2026-11-18)

- Add independent admitted Data Guard database context, standby logs, process/recovery progress, archive gaps/destinations/status and current-incarnation RFS archived progress, with typed complete reads, caller-native rows and atomic JSON; preserve reporter scope, exact NULL/SCN/time pairs, latest ties and destination-copy conflicts (#60).

- Refresh released go-oracle-session to v0.10.0 and all eligible existing graph nodes; adopt compatible x/exp with newly released x/tools v0.53.0, preserving Go 1.27.4 and the separately verified dependency compatibility boundary (#59).

## v0.17.0 (2026-11-17)

- Update released go-oracle-session to v0.9.1; recheck official compiler/dependency/tool releases and independently verify the selected-dependency Go boundary, retaining documented upstream compatibility exceptions (#57).

- Admit Oracle 9i/10g for ORACLE_HOME properties and direct datasets; retain
  explicit library inference and unavailable GET_ENV attempts on 9i; admit their
  five-part numeric version bootstrap while retaining fact-specific gates (#55).

- Add executing-instance ORACLE_HOME facts, safe context/GET_ENV/library fallback with retained attempts, native rows/JSON and a punctual facade; distinguish documented 12c+ USERENV metadata from inferred 9i-11g dictionary library paths, preserving NULL/error/provenance and caller ownership (#55).

- Reject cancelled memory collections and preserve independent errors joined with no-row outcomes, while retaining valid primary fallback evidence with errors (#56).
- Validate parameter/password-file/instance domain admission before publishing complete JSON, and strengthen positive JSON/fallback output regressions (#56).
- Render documented ISO DATE fields from Gregorian components without changing session state; reuse reviewed DATE SQL and exact rowset conversion/validation, and remove unreachable storage policy from ASM/redo (#56).

## v0.16.0 (2026-11-17)

- Update released go-oracle-session from v0.6.0 to v0.9.0 and current official metadata-only graph versions; refresh govulncheck to v1.9.0 and document the independently checked compiler boundary (#53).

- Correct GoldenGate message-number text projections and order-independent latest patch selection; share instance-source validation without changing public APIs (#51, #52).

- Add generic database-visible GoldenGate capture and integrated apply coordinator facts, typed complete reads, caller-native rows/JSON and facade methods; preserve instance/container identity, exact values and source availability (#52).

- Add complete qualified visible-member full version/context facts and a pure admitted SQL release-level evaluator with raw attempt retention, retry/rollback/tie handling and explicit unknown/parity limits (#51).

- Add an opt-in typed conditional parameter persistence evaluator and bounded standard JSON over admitted effective/SPFILE observations, preserving raw APIs, per-member unknown states and exact-value/provenance semantics (#50).

- Add exact visible controlfile file datasets and allocation/storage properties, independent scoped encryption counts, native/JSON facade methods and global visible-cell presence; preserve legacy APIs and explicit unknown/visibility states (#49).

## v0.15.1 (2026-11-15)

- Add explicit aliases to every inner/outer connected-cell projection while preserving column order, values, scope, sorting and row limits.

## v0.15.0 (2026-11-15)

- Add complete read-only database properties, observed session/client context, database NLS, installed options, component and patch history, detailed services and invalid-object facts with explicit source/version choices.
- Expose typed complete readers, caller-native rows and atomic JSON with matching domain admission; preserve exact nullable values, source/container semantics and existing APIs.

- Add a connected-source SYSDATE civil clock fact for finite local redo-history windows, preserving reporting/container identity and keeping source-local labels separate from client UTC observations.

- Add exact redo groups, members, bounded history/archive-copy inputs, tablespace/file/free-extent/segment inventories, LONG-column attribution, default temporary groups, per-instance SGA components and ASM connection facts.
- Preserve raw byte/block units, optional columns, SQL NULL, instance/container scope and local Gregorian time bounds; validate every JSON dataset with its matching domain decoder before publication.

- Add complete account/table metadata, sequences, directories, database links, direct/column/role grants, triggers and explicitly admitted ACL, SQL profile/baseline and metadata DDL datasets.
- Preserve source/version/filter provenance and exact nullable fields, validate full CLOB length/budgets in owned typed/JSON readers, and retain caller-owned native cursors without executing links, sequences, grants or tuning operations.

- Add an explicit X$KSPPI/X$KSPPCV2 context-value source with the INDX+1 parameter identity, ordinary and underscore names, raw default flags and exact instance/container/context-row observations.
- Expose complete typed readers, caller-native rows and bounded JSON with identical name/default admission and a SQL-only selector example; retain existing hidden catalogs and distinguish server SQL evidence from Go-driver qualification.

## v0.14.0 (2026-11-15)

- Rename the shared EngineeredSystem SQL catalog to statements.go; SQL bytes, capability selection and public APIs are unchanged.
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
