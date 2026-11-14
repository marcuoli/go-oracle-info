# Ownership and contracts

- identity selects release-appropriate database/version/platform facts and preserves SQL NULL and source provenance.
- memory owns scoped configured-parameter SQL, exact result decoding, the permitted ADB retry and RAC member resolution/aggregation.
- engineeredsystem owns exact cell/property/payload facts, defensive snapshots, complete-result row decoding and XML normalization; its catalog owns capability-selected static SQL. Consumers attach actual execution coverage and decide collection completion. Shared decoders accept only complete go-oracle-session Results and never acquire scopes, allocate application SQL references or recover sessions.
- The existing session module owns physical query and cursor lifecycle. Callers supply an already admitted reader and execution references; no driver dependency is introduced here.
- Applications retain endpoint/container authorization, deadlines, recovery, progress, completion policy, scoring and presentation.

Configured SGA is the instance maximum for non-CDB/root, or a positive PDB target with explicitly admitted instance inheritance. Configured PGA is its aggregate target, including a valid zero. Neither is observed memory consumption. RAC totals require complete member evidence and sum raw bytes before rounding.

An ADB effective query is eligible only for a known Oracle 18+ connected PDB after a primary SQL NULL or no-row outcome. Primary execution/decoding errors are retained and never trigger that retry. Effective SQL verifies the current container, one source row, a non-session-modifiable parameter and an established cloud-service value.

All fields retain source identities and availability. Invalid metrics do not erase independent valid metrics. Any row/result limit belongs to the supplied query transport and must surface as an error, not a retained prefix.

Scalar observations distinguish the actual execution container (`CurrentContainerID`) from the admitted logical target (`TargetContainerID`). A root-discovered PDB read uses root execution identity and its bound PDB target; it does not imply that the session changed containers. Root-view visibility and complete PDB parameter coverage remain caller admission requirements. The new module's fixtures do not establish native coverage for those views.

Reusable Oracle statement selection lives in the responsibility packages
documented in selector-contracts.md. No extra executor or collection engine is
introduced. Exact owner filters remain binds, dictionary identifiers are quoted,
and static bounded selectors validate their integer limits. A caller's successful
execution/admission determines evidence availability; query selection alone
cannot establish complete source coverage, acceptance or native compatibility.

The root oracleinfo.Client is a thin, non-owning facade over a caller-admitted native QueryContext handle. parameters owns effective-instance versus stored-SPFILE SQL, filters, schema and provenance. Generic query delegation, native cursor consumption and ordered bounded standard JSON live only in go-oracle-session/rowset; fact readers call that package directly. No rowset implementation or compatibility package remains in this module. Borrowed native handles and owned Session.Stream leases are separate contracts; no private cursor escapes its lease. Existing Oracle selector/decoder APIs remain available. Resultsets must never be serialized as CSV/plaintext except genuine DBMS_OUTPUT; typed scalar/string values and SQL source are separate from formatted resultsets.

Buffered `parameters.Reader` and `passwordfile.Reader` are structural admitted
`session.Statement`/complete-`session.Result` contracts, consistent with existing
identity/memory readers. Domain decoders validate every row before publishing
facts; they do not create a cursor or generic executor. Hidden facts reuse the
visible instance-effective source and retain nullable/default-state provenance.
Password-file facts are cached view metadata, with documented 12.2 availability
and 21c optional-column selection; they never inspect credentials/files or refresh
settings. Application-owned report unions do not alter raw source evidence.

Instance-wide scalar-row system parameters and qualified instance visibility are documented in [system parameter contracts](system-parameters.md). Preserve the published SYSTEM_PARAMETER2 list APIs; per-member scheduling, partial coverage and report selection remain consumer-owned.
