# Account, object metadata and security datasets

These APIs collect admitted read-only dictionary facts. They open no connections,
change no container, own no recovery policy, execute no database link or sequence,
and apply no grants, DDL, tuning operation or session-global metadata transform.
Callers retain privilege/capability admission, scope policy, deadlines, transport
budgets and comparison/report selection. SQL profiles and baselines require an
explicit application collection choice; their existence here enables no automatic
performance collection.

## API and completeness contract

The responsibility packages expose reviewed `Statement` selectors, pure `Decode`
functions, complete buffered `Read` functions and native `Rows`/`JSON` companions.
Families with several cohesive datasets use these prefixes:

| Package | Families | Typed records |
| --- | --- | --- |
| `users` | default, `AccountCount`, `TableMetadata` | `User`, `AccountCount`, `Table` |
| `sequences` | default | `Sequence` |
| `directories` | default | `Directory` |
| `dblinks` | default | `Link` |
| `grants` | `Direct`, `DirectColumn`, `Role` | `DirectGrant`, `DirectColumnGrant`, `RoleGrant` |
| `triggers` | default | `Trigger` |
| `sqlplan` | `Profile`, `Baseline` | `Profile`, `Baseline` |
| `networkacl` | explicit `Dataset` | `Record` with absent-column pointers |
| `metadata` | explicit object `Kind` | one complete DDL `Record` |

For example, the table metadata family is `TableMetadataStatement`,
`DecodeTableMetadata`, `ReadTableMetadata`, `TableMetadataRows` and
`TableMetadataJSON`. Each result retains `Provenance` and ordered `Records`.
The root facade observes the source version without caching and provides native
methods such as `Sequences`, `SequencesJSON`, `NetworkACL` and `MetadataDDLJSON`.
Independent methods and their JSON companions are independent observations.

A package `Options` selects the observed `OracleMajor` and exact bound `Owners`
and `Names` when meaningful. Names mean usernames for users, grantees/roles for
grants, and plan names for baselines; baseline owners are parsing schemas.
Unsupported selectors fail instead of being ignored. Empty lists retain all
admitted visible rows. Binary comparisons preserve case and treat names as
literal values. No user/role blacklist or application-schema inference is built in.
`AccountCount` rejects filters and counts all visible `DBA_USERS` accounts; it is
neither an application-schema count nor an object-owning-user count.

A buffered `Request` also supplies a caller-owned SQL reference and `rowset.Limits`.
Zero limits use the session library's finite defaults. Reads enforce maximum rows
and retained text bytes before returning records, in addition to the reader's
own finite transport/LOB budgets. Only the reader can establish successful EOF
and terminal/close completion. Pure decoders cannot attest transport completion.
Query errors, invalid late rows, budget failures and post-query cancellation
publish no successful prefix. An empty inventory is distinct from an error.
The scalar account count requires exactly one nonnull nonnegative count.

Native `Rows` returns the actual caller-owned `*sql.Rows`, without preconsumption,
row truncation or a return-time cancellation. Its owner must enforce deadlines
and budgets, consume every value, inspect `Err` and `Close` on every path. JSON
opens its own cursor and delegates all generic cursor consumption/finalization
to `go-oracle-session/rowset`. It validates the same domain schema, numeric text
and scope before returning bounded standard JSON. No bytes are returned after
query, scan, terminal, close, cancellation or capacity errors. No CSV/plaintext
query-result serialization exists.

Typed fields use `session.Value`: `Null` is SQL NULL and `Text` retains the selected
exact representation. A nil optional pointer means the release projection did
not select that column; a nonnil null pointer value means projected SQL NULL.
Numeric dictionary fields use Oracle `TO_CHAR(...,'TM9',...)` with explicit decimal
characters and remain exact decimal text, including values beyond signed 64-bit
range. JSON deliberately retains those SQL text columns as strings. DATE fields
are timezone-free ISO Gregorian text to seconds, rendered from EXTRACT components
independently of session NLS_CALENDAR. NULL remains NULL and no session state is
changed. TIMESTAMP fields retain fractional
precision to nine places without inventing a timezone. Flags and unknown statuses
remain raw, rather than coercing NULL/unknown to false.

## Source and release selections

The static base projections require an admitted Oracle 11g-or-later source.
The legacy references inspected are 11.2 documentation; these source choices and
unit fixtures do not certify any release/RU/platform or Oracle 11.1 capability.
Query/source/grant failures never trigger a narrower fallback.

| Dataset | Selected source | Scope and qualifications |
| --- | --- | --- |
| Users | `DBA_USERS` | Username/ID/account state, dates, default and temporary tablespaces, profile, initial resource group, authentication type and editions flag. No password, verifier, external identity or credential fields. `COMMON`/`ORACLE_MAINTAINED` from major 12; conservative `INHERITED` projection from 18. Root visibility and complete CDB membership remain caller concerns. |
| Account count | `DBA_USERS` | Exact total account count in the admitted connected-container dictionary scope, independently of user selection or object ownership. |
| Table metadata | `DBA_TABLES` | Owner/name/tablespace/status; temporary/nested/secondary/IOT name/type/partitioning/duration; nullable recorded row/block/empty-block/average-row/sample statistics and last analysis; global/user statistics, degree and instances. No filters exclude temporary, nested, secondary or IOT overflow rows. It performs no table counts and does not change existing count selectors. |
| Sequences | `DBA_SEQUENCES` | Owner/name, min/max/increment/cache, cycle/order and `LAST_NUMBER`. Session/keep flags from 12, scale/extend from 18 and sharded flag from 19. `LAST_NUMBER` is stored/cache metadata, may exceed the last used value, and is to be ignored for session sequences; it is neither current value nor safe next value. |
| Directories | `DBA_DIRECTORIES` | Owner/name/path, plus conservative `ORIGIN_CON_ID` projection from 18. Origin ID retains the source's textual type. No filesystem access. |
| Database links | `DBA_DB_LINKS` | Owner/name/declared username/raw host connect string/creation date only. No execution, reachability claim or credential/verifier access. |
| Direct object grants | `DBA_TAB_PRIVS` joined to `DBA_USERS` | Direct grants to users, including grantor/privilege/grant option/hierarchy. `COMMON` from 12, `INHERITED` from 18. Owner and user policy are explicit caller filters. |
| Direct column grants | `DBA_COL_PRIVS` joined to `DBA_USERS` | Column identity and grantor/grant option remain separate from object-wide grants. The same common/inherited projection applies. No concatenated GRANT reconstruction. |
| Role grants | `ROLE_TAB_PRIVS` | Only roles accessible to the connected user, even under SYSDBA; this is explicitly not complete DBA role coverage. Preserve column identity/grantability and optional common/inherited flags. No grantor is invented. |
| Triggers | `DBA_TRIGGERS` left joined to `DBA_OBJECTS` | Exact trigger owner/name/type/event/base/table/action, trigger status, separate object validity and `LAST_DDL_TIME`. Join restricted to the trigger object type; missing object metadata remains NULL. Caller selects SYS/SYSTEM when required. No trigger body execution. |
| SQL profiles | `DBA_SQL_PROFILES` | Name/category/signature/status/type/force matching/description/creation and modification times. Complete records permit exact category/status grouping. No SQL text, tuning actions or live sampling. |
| SQL baselines | `DBA_SQL_PLAN_BASELINES` | Signature/handle/plan/creator/origin/parsing schema/description/source version/timestamps and raw enabled/accepted/fixed/reproduced/autopurge flags. `LAST_EXECUTED` may lag actual use. No SQL text or baseline mutation. |

The conservative omission of some 12.2 columns is explicit absence, not a claim
that those values are false or unsupported by the database. A new capability
projection should be added against release-specific evidence rather than inferred
from a major-version label.

### Classified object, column and role grants

`grants.ReadObjectPrivilegeGrants`, `grants.ReadColumnPrivilegeGrants` and
`grants.ReadRoleMemberships` provide independent, direct dictionary selections for
named users and roles. The type of a column grant's object stays NULL when the
dictionary cannot establish a unique type. Membership records retain direct
role-to-role edges. The default selection includes visible named accounts and
roles. Set `Options.Names` to include exact `PUBLIC` when its direct grants are
needed. The SDK returns those rows with `GRANTEE_KIND=PUBLIC` and retains any
exact named scope alongside it. `Owners` can further scope object and column
grants to a selected schema set.

On admitted releases, a PUBLIC role-membership request also projects
`GRANTED_ROLE_ORACLE_MAINTAINED` from `DBA_ROLES`. A SQL NULL means the lookup
did not establish a flag. An omitted field indicates the projection lacked it.

Set `ApplicationPublicOnly` in `ClassifiedOptions` to apply the maintenance
filter before transport. This mode requires exactly `Names: []string{"PUBLIC"}`
and qualified maintenance metadata. Object and column grants select owners
marked `N` in `DBA_USERS`. Role grants select roles marked `N` in `DBA_ROLES`.
The filtered selection uses the scope `connected_container_dba_application_public`.
Use `Read…WithOptions` with `grants.ClassifiedRequest` for buffered reads.
This metadata classifies the granted role. The grant views describe the
privilege and recipient but do not establish whether Oracle supplied the grant.

Select the family using `grants.ClassifiedOptions` in its Statement, Decode,
Rows and JSON entry points. Buffered readers take `grants.Request`. Their
`Read…ForRelease` forms accept the observed five-part numeric release for
precise Oracle 12 optional columns. See the
[classified grants compatibility matrix](classified-grants-compatibility.md).

## Distinct ACL visibility

`networkacl.Options.Dataset` admits exactly one dataset. Mapping records, DBA ACE
facts and current-user privilege status are never silently combined. Optional
record pointers preserve absence of source-specific columns. `ACLID` is represented
with lossless `RAWTOHEX`; neither a fixed RAW length nor common ACL ownership is
invented across releases.

| Dataset | Oracle 11g selection | Major 12+ selection |
| --- | --- | --- |
| `DBAHostMappings` | `DBA_NETWORK_ACLS`: host/ports/ACL/ACLID | `DBA_HOST_ACLS`, also ACL owner |
| `DBAHostPrivileges` | `DBA_NETWORK_ACL_PRIVILEGES`: ACL/ACLID/principal/privilege/raw `IS_GRANT` and `INVERT`/start/end | `DBA_HOST_ACES`: host/ports/ACE order/times/grant type/inverted principal/principal/type/privilege |
| `DBAWalletMappings` | `DBA_WALLET_ACLS`: wallet path/ACL/ACLID | Same view, also ACL owner |
| `DBAWalletPrivileges` | Explicit `ErrUnsupportedCapability` for this projection | `DBA_WALLET_ACES` |
| `UserHostPrivileges` | `USER_NETWORK_ACL_PRIVILEGES` | `USER_HOST_ACES` |
| `UserWalletPrivileges` | Explicit `ErrUnsupportedCapability` for this projection | `USER_WALLET_ACES` |

Legacy `IS_GRANT` and `INVERT` use the unified aliases `GRANT_TYPE` and
`INVERTED_PRINCIPAL` while retaining original raw values. Their legacy/modern
interpretation must use provenance. Current-user status retains `GRANTED`/`DENIED`
and never claims visibility of another user, an entire role graph or all matching
ACLs. These are facts for assessment, not an effective network authorization engine.

## Optional DDL evidence

`metadata.Options` permits only `Tablespace`, `Directory` and `Trigger`, with an
explicit name, required trigger owner and finite `MaxDDLBytes` (default 8MiB).
`Statement` uses bound object identity and native `DBMS_METADATA.GET_DDL` CLOBs.
It applies no `SUBSTR`, VARCHAR conversion, source replay or persistent transform
setting. Provenance records `caller_session_inherited_unmodified` transforms;
the library does not claim they are default or inspect/change caller settings.
DBMS_METADATA's privileges and shared-server rules remain applicable.

`Read`, `Decode` and `JSON` require exactly one object with matching identity,
nonnull full DDL, a valid Oracle `DBMS_LOB.GETLENGTH`, and both transport/dataset
budgets. Oracle CLOB length is in UCS2 code units; supplementary Unicode scalars
consume two units. Exact length validation detects a driver/codec-provided prefix.
No rows is `ErrNoRows`, not a complete empty capture. `Result.Complete` becomes
true only after these checks. The raw `Rows` path leaves full CLOB consumption,
length validation, deadlines and `Err`/`Close` to its owner. Shared native JSON
supports complete string/byte CLOB values; other driver values fail explicitly.
No native CLOB compatibility is inferred from fixture success.

DDL is stored as raw evidence for a separate application artifact/report workflow.
It has no cell-width, font, pagination, syntax-color, grant-generation or execution
responsibility. A document/cell capacity limit cannot authorize truncating it.

## Source review and validation boundary

Oracle documentation was checked on 2026-11-15. Identifiers below are the current
view/package identifiers; no MOS identifier migration is asserted. These checks
review meaning and source projection, not native driver/grant/RU qualification.
No authenticated MOS or actual Oracle session was used for these additions.

- [DBA_USERS (11.2)](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5081.htm) and [DBA_USERS (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_USERS.html): authentication metadata, account fields and common/maintenance semantics.
- [ALL_TABLES (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TABLES.html): DBA_TABLES related-view projection, nullable/stale recorded statistics and table category flags.
- [ALL_SEQUENCES (12.2)](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/ALL_SEQUENCES.html) and [ALL_SEQUENCES (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_SEQUENCES.html): DBA_SEQUENCES related-view columns, cache/session caveats and 19c sharded flag.
- [ALL_DIRECTORIES (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_DIRECTORIES.html) and [ALL_DB_LINKS (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_DB_LINKS.html): dictionary-only paths, textual origin container and declared link facts.
- [DBA_HOST_ACLS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_HOST_ACLS.html), [DBA_HOST_ACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_HOST_ACES.html), [DBA_NETWORK_ACL_PRIVILEGES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_NETWORK_ACL_PRIVILEGES.html), [USER_NETWORK_ACL_PRIVILEGES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/USER_NETWORK_ACL_PRIVILEGES.html): legacy deprecation and distinct mapping/ACE/status visibility.
- [DBA_WALLET_ACLS (11.2)](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5088.htm), [DBA_WALLET_ACLS (12.2)](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_WALLET_ACLS.html), [DBA_WALLET_ACES (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_WALLET_ACES.html), [USER_HOST_ACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/USER_HOST_ACES.html) and [USER_WALLET_ACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/USER_WALLET_ACES.html): wallet assignments versus privileges and current-user status.
- [DBA_TAB_PRIVS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TAB_PRIVS.html), [DBA_COL_PRIVS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_COL_PRIVS.html), [ROLE_TAB_PRIVS (11.2)](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5172.htm) and [ROLE_TAB_PRIVS (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ROLE_TAB_PRIVS.html): grant option, hierarchy, column scope and restricted role visibility.
- [ALL_TRIGGERS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/ALL_TRIGGERS.html), [DBA_SQL_PROFILES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_SQL_PROFILES.html) and [DBA_SQL_PLAN_BASELINES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_SQL_PLAN_BASELINES.html): independent trigger/object states and read-only profile/baseline metadata.
- [DBMS_METADATA](https://docs.oracle.com/en/database/oracle/oracle-database/19/arpls/DBMS_METADATA.html), [DBMS_LOB](https://docs.oracle.com/en/database/oracle/oracle-database/19/arpls/DBMS_LOB.html) and [SQL Semantics and LOBs](https://docs.oracle.com/en/database/oracle/oracle-database/19/adlob/SQL-semantics-and-LOBs.html): named-object CLOB capture, unchanged inherited transforms and UCS2 length semantics.

Unit tests cover old/modern source selection, empty/NULL/absent data, scope/case
integrity, large exact integers, all scoped table categories, raw flags/options,
independent statuses/times, denied sources, malformed late rows, complete CLOB
Unicode/length/caps, actual native cursor identity and atomic JSON failure paths.
Fixtures are unit scaffolding, not native Oracle compatibility evidence.
