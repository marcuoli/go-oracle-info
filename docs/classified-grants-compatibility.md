# Classified grant query compatibility

Oracle references checked 2026-11-23 for issue #125. Each selector reads named
users and roles in the admitted dictionary scope. The Oracle principal
namespace prevents a user and role from sharing a name. The join compares
binary name keys, excludes exact `PUBLIC` by default, and leaves case and whitespace in
every returned identifier intact. Name and owner restrictions are bound.
[ORA-01920](https://docs.oracle.com/en/error-help/db/ora-01920/),
[ORA-01921](https://docs.oracle.com/en/error-help/db/ora-01921/).

## Base projections

| Selector | Dictionary views | Base aliases |
| --- | --- | --- |
| `ObjectPrivilegeGrants` | `DBA_TAB_PRIVS`, `DBA_USERS`, `DBA_ROLES` | `OWNER`, `GRANTOR`, `GRANTEE`, `GRANTEE_KIND`, `OBJECT_TYPE`, `TABLE_NAME`, `PRIVILEGE`, `GRANTABLE`, `HIERARCHY` |
| `ColumnPrivilegeGrants` | `DBA_COL_PRIVS`, `DBA_USERS`, `DBA_ROLES`, `DBA_OBJECTS` | `OWNER`, `GRANTOR`, `GRANTEE`, `GRANTEE_KIND`, `OBJECT_TYPE`, `TABLE_NAME`, `COLUMN_NAME`, `PRIVILEGE`, `GRANTABLE` |
| `RoleMemberships` | `DBA_ROLE_PRIVS`, `DBA_USERS`, `DBA_ROLES` | `GRANTEE`, `GRANTEE_KIND`, `GRANTED_ROLE`, `ADMIN_OPTION`, `DEFAULT_ROLE` |

`GRANTEE_KIND` identifies a `USER`, `ROLE` or explicitly requested `PUBLIC` grantee. Object-level `OBJECT_TYPE` is the raw
`DBA_TAB_PRIVS.TYPE` where documented and a typed SQL NULL for 11g. The column
selector uses an exact owner and object name lookup in the same statement. It
looks for top-level table/view/materialized-view object types and returns the
type only when all matching candidates agree. For example, an ambiguous
materialized view and backing table yield SQL NULL. No hierarchy flag is
projected for column grants. Role memberships preserve direct role-to-role edges
and default-role values as recorded. The selectors apply no eligibility policy
and perform no transitive role expansion.

Oracle source evidence for the lookup is in
[DBA_OBJECTS (11.2)](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_4158.htm),
[ALL_OBJECTS (26ai)](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/ALL_OBJECTS.html)
and [GRANT (19c)](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/GRANT.html).

## Numeric release matrix

Each admitted row below selects the base projections and exactly the listed
additions. Release strings are selector test inputs, not observations of a
running database. Native Oracle execution and SDK driver/application
qualification are recorded separately. Legacy adapters keep their historical
behavior.

| Product release | Numeric selector input | Object projection | Column projection | Membership projection | Static | Native SQL | SDK driver/app |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 9iR2 | 9.2.0.8.0 | Unsupported | Unsupported | Unsupported | Reject | No execution | No execution |
| 10gR1 | 10.1.0.5.0 | Unsupported | Unsupported | Unsupported | Reject | No execution | No execution |
| 10gR2 | 10.2.0.5.0 | Unsupported | Unsupported | Unsupported | Reject | No execution | No execution |
| 11gR1 | 11.1.0.7.0 | Base, type NULL | Base | Base | Checked | Unqualified | Unqualified |
| 11gR2 | 11.2.0.4.0 | Base, type NULL | Base | Base | Checked | Unqualified | Unqualified |
| 12cR1 | 12.1.0.1.0 | Base + COMMON | Base + COMMON | Base + COMMON | Checked | Unqualified | Unqualified |
| 12cR1 | 12.1.0.2.0 | Base + COMMON | Base + COMMON | Base + COMMON, DELEGATE_OPTION | Checked | Unqualified | Unqualified |
| 12cR2 | 12.2.0.1.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Unqualified | Unqualified |
| 18c | 18.0.0.0.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Unqualified | Unqualified |
| 19c | 19.3.0.0.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Passed at 19.32.0.0.0 in scope below | Passed at 19.32.0.0.0 in scope below |
| 21c | 21.3.0.0.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Unqualified | Unqualified |
| 23c/23ai before 26ai | 23.3.0.0.0 / 23.9.0.0.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Unqualified | Unqualified |
| 26ai | 23.26.1.0.0 | Base + COMMON, INHERITED | Base + COMMON, INHERITED | Base + COMMON, INHERITED, DELEGATE_OPTION | Checked | Passed in scope below | Passed in scope below |

The conservative major-only Oracle 12 selection projects `COMMON` alone.
`Read…ForRelease` and `ClassifiedOptions.OracleRelease` take the observed
five-part numeric version and require the same `OracleMajor`. The 12.1.0.2
boundary enables `DELEGATE_OPTION` for memberships. The 12.2.0.1 boundary
enables `INHERITED` for all three sources. Actual source NULL and a column
omitted by selection remain distinct. Numeric major 26 and unreviewed major
families are rejected before SQL. [Oracle Database release numbering](https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/oracle-database-release-numbers.html).

## Source evidence and qualification scope

- Oracle 9iR2, 10gR1 and 10gR2 document historical grant views. All three new
  selectors explicitly reject these releases without reading them.
  [Oracle9i Database Reference](https://docs.oracle.com/cd/A97385_01/server.920/a96536.pdf),
  [DBA_ROLE_PRIVS (10.1)](https://docs.oracle.com/cd/B13789_01/server.101/b10755/statviews_2291.htm),
  [DBA_ROLE_PRIVS (10.2)](https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_4064.htm).
- 11g publishes the base grant aliases without `TYPE` for objects or
  multitenant flags. The 11.1 object/column pages describe a user grantee,
  while 11.2 explicitly describes users or roles. Classified 11.1 role
  recipients in both grant views remain a native evidence gap.
  [11gR1 Database Reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/toc.htm),
  [11gR2 Database Reference](https://docs.oracle.com/cd/E11882_01/server.112/e40402/toc.htm).
- 12.1 documents object `TYPE` and `COMMON` in the three views and gives
  a patch-set footnote for membership `DELEGATE_OPTION`.
  [DBA_TAB_PRIVS (12.1)](https://docs.oracle.com/database/121/REFRN/GUID-4AC57A96-FF55-4788-9301-AFED23AE3934.htm),
  [DBA_COL_PRIVS (12.1)](https://docs.oracle.com/database/121/REFRN/GUID-30FF23CE-B6F4-4C70-98BD-8AF602BDCFE7.htm),
  [DBA_ROLE_PRIVS (12.1)](https://docs.oracle.com/database/121/REFRN/GUID-9BE97833-92B5-4CCE-AAC1-16C094C1A530.htm).
- Oracle documents 12.2.0.1 as the starting release for `INHERITED`.
  [DBA_TAB_PRIVS (12.2)](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_TAB_PRIVS.html),
  [DBA_COL_PRIVS (12.2)](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_COL_PRIVS.html),
  [DBA_ROLE_PRIVS (12.2)](https://docs.oracle.com/cd/E82638_01/refrn/DBA_ROLE_PRIVS.html).
- The 18c, 19c, 21c and 26ai references list the full projected columns.
  The three linked 23 references redirect to the 26ai reference. They do
  not independently establish pre-26ai RU-specific layouts.
  [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_ROLE_PRIVS.html),
  [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_ROLE_PRIVS.html),
  [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/DBA_ROLE_PRIVS.html),
  [23](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/DBA_ROLE_PRIVS.html),
  [26ai object](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_TAB_PRIVS.html),
  [26ai column](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_COL_PRIVS.html),
  [26ai membership](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_ROLE_PRIVS.html).

## Native SQL and driver qualification

On 2026-11-23, SQL*Plus executed the three full projected statements selected
from SDK commit `d66bb9fc0c0be006a170829476d6ba17523c90c0`. SQL*Plus consumed
the projected rows and recorded successful row counts in four existing
Enterprise Edition CDB-root sessions under SYSDBA.

| Observed numeric release | Role and open mode | Sessions | Object / column / membership rows per session |
| --- | --- | --- | --- |
| 19.32.0.0.0 | Primary, READ WRITE | 1 | 8641 / 3 / 162 |
| 23.26.1.0.0 | Primary, READ WRITE | 2 | 9364 / 3 / 210 |
| 23.26.1.0.0 | Physical standby, READ ONLY WITH APPLY | 1 | 9364 / 3 / 210 |

These 12 executions qualify the unfiltered projections in the recorded sessions.

The admitted Go driver and application independently completed all three
collectors in two primary, READ WRITE PDB sessions as SYSTEM.

| Observed numeric release | Object / column / membership rows |
| --- | --- |
| 19.32.0.0.0 | 8645 / 3 / 163 |
| 23.26.1.0.0 | 9376 / 3 / 212 |

The driver results qualify these exact account, version and PDB cases.
Non-CDB, MOUNT, other RUs, named binds and other accounts or roles remain
unqualified. Each run observes one admitted session and its connected container.
Qualification requires the intended account and read access to every selected
view in the actual connected container. Root visibility is
root-scoped. [Monitoring CDBs and PDBs](https://docs.oracle.com/en/database/oracle/oracle-database/19/multi/viewing-information-about-cdbs-and-pdbs-with-sql-plus.html).
## Explicit PUBLIC scope

Issue #129 adds explicit exact `PUBLIC` selection on the three views above.
The named arm keeps its binary comparison, while an independent `DUAL` row
classifies the special grantee even when it is absent from `DBA_ROLES`. A
named dictionary match for exact `PUBLIC` cannot duplicate the row. Ordinary
named selection keeps its previous SQL, aliases and behavior. Object and
column callers can bind `Owners` to relevant schemas before collecting PUBLIC
grants. The dictionary can return many grants on Oracle-owned objects.
`ApplicationPublicOnly` requires the exact single `PUBLIC` scope and applies
the positive `ORACLE_MAINTAINED='N'` selector in Oracle before buffering any
rows. On Oracle 12 the selector requires precise 12.1.0.2 or 12.2.0.1 and later
numeric release evidence. Earlier or major-only evidence returns
`ErrApplicationPublicUnavailable` for the filtered request. Raw explicit
PUBLIC queries retain their separate, wider coverage.

| Release | Explicit PUBLIC source selection | Granted role flag | Static | Native PUBLIC SQL | PUBLIC driver/application |
| --- | --- | --- | --- | --- | --- |
| 9iR2, 10gR1/R2 | Existing selectors reject before SQL | Unprojected | Boundary tests | No execution | No execution |
| 11gR1/R2 | Existing base views and PUBLIC arm | Unprojected | Scope and boundary tests | Unqualified | Unqualified |
| 12.1.0.1 and major-only 12 | Existing Oracle 12 base views and PUBLIC arm | Unprojected | Boundary tests | Unqualified | Unqualified |
| 12.1.0.2, 12.2.0.1, 18c | Existing projections above and PUBLIC arm | `DBA_ROLES.ORACLE_MAINTAINED` | Boundary tests | Unqualified | Unqualified |
| 19c (19.32.0.0.0) | Existing projections above and PUBLIC arm | Same flag | Scope and decoder tests | Passed as SYSDBA in OPEN CDB root. 43,514 raw object rows, zero raw column and role rows. | Passed as SYSTEM in an OPEN PDB. 43,518 raw object grants and zero qualifying grants in all three views. |
| 21c, 23c/23ai | Existing projections above and PUBLIC arm | Same flag | Boundary tests | Unqualified | Unqualified |
| 26ai (23.26.1.0.0) | Numeric-major-23 projections and PUBLIC arm | Same flag | Scope and decoder tests | Passed as SYSDBA in OPEN CDB root. 38,732 raw object rows, zero raw column and role rows. | Passed as SYSTEM in an OPEN PDB. 38,738 raw object grants and zero qualifying grants in all three views. |

The SQL*Plus run used the SDK generated statements with the exact PUBLIC bind,
forced their projections and checked all counts against the direct dictionary
views. The SDK's Go driver also read both OPEN PDBs as SYSTEM through released
go-oracle-session v0.19.0. Filtered counts agreed with independently
classifying raw grant rows using owner and role maintenance metadata.
The fixtures had no qualifying grant, so positive filtered population is
covered by static tests only. Nonempty native filtered results, native
column and role population, MOUNT, other logins and other releases remain
unqualified. SDK tests preserve named, PUBLIC, omitted and SQL NULL cases. The catalog records database-maintained status of
the granted role. Attribution of an individual grant remains outside these
dictionary projections. In particular, Oracle can record the object owner as
the grantor when a different account exercised GRANT ANY OBJECT PRIVILEGE.

Sources checked 2026-11-23.
[Oracle 11gR2 GRANT](https://docs.oracle.com/html/E26088_02/statements_9013.htm),
[12.1 DBA_ROLES](https://docs.oracle.com/database/121/REFRN/GUID-C8E5500E-A013-45D3-AEFB-2C9C0AC87EC7.htm),
[12.2 DBA_ROLES](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_ROLES.html),
[19c GRANT](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/GRANT.html),
[19c PUBLIC role](https://docs.oracle.com/en/database/oracle/oracle-database/19/dbseg/grants-and-revokes-privileges-and-public-role.html),
[26ai DBA_ROLE_PRIVS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_ROLE_PRIVS.html).

## PUBLIC owner or grantor selection (#133)

ApplicationPublicOwnerOrGrantor is an explicit object and column option.
It accepts PUBLIC as the sole named grantee. The bounded Oracle statement
compares DBA_USERS.USERNAME with the recorded OWNER or GRANTOR using binary
identity predicates and requires ORACLE_MAINTAINED = 'N'. Its provenance scope
is connected_container_dba_application_public_owner_or_grantor. ApplicationPublicOnly
retains its existing owner-only object and column scope and its existing role
scope. Supplying both options or applying the new option to role membership
fails before execution.

The DBA_USERS visibility and ORACLE_MAINTAINED release gate is unchanged.
Oracle 9iR2, 10gR1/R2, and 11gR1/R2 cannot execute this option. Oracle 12.1.0.1
and major-only 12 observations cannot select it. The exact 12.1.0.2 boundary,
12.2.0.1 and the supported 18c, 19c, 21c and numeric 23 family use the
reviewed classification projection. The rest of the matrix above still
governs the selected grant columns. Fixture checks distinguish query selection
from native lab execution and retain exact recorded actor identities.

Native Go driver collection ran as SYSTEM in open primary PDBs on
19.32.0.0.0 and 23.26.1.0.0. Each returned seven complete object grant rows
qualified by the recorded grantor only. Both column collections completed
with zero rows. The owner-only and both-actor cases have fixture coverage.

## Standalone role maintenance catalog (#143)

`grants.ReadRoleCatalog` reads the complete visible set of `ROLE` and
`ORACLE_MAINTAINED` values from `DBA_ROLES` in the connected dictionary.
The role's exact spelling and its `Y`, `N` or SQL NULL flag are retained.
A role can appear even when the admitted scope has no incoming membership
for it. The result does not identify who granted the role or prove effective
privileges or visibility across containers.

`RoleCatalogStatement`, `DecodeRoleCatalog`, `RoleCatalogRows` and
`RoleCatalogJSON` use the same ordered two-column selection. The caller supplies
the admitted reader or query handle, observed numeric release, reference and
limits as applicable. Rows returns the native cursor for caller-owned iteration
and close. JSON consumes its own cursor. Read and JSON retain the numeric release
and connected dictionary scope and return no successful prefix after a failure.
A missing role and a NULL flag remain distinct observations.

This selector reuses the classified grants release guard. A numeric release
that cannot qualify the flag returns `ErrRoleCatalogUnavailable` before
execution. The 12.1 reference lists the column at the release-family level.
The SDK uses the existing conservative 12.1.0.2 and 12.2.0.1 patch-set
boundaries. Earlier releases can use the separately documented [predefined
role list](legacy-predefined-roles.md) where applicable.

| Numeric release | Documented source and selected projection | Static tests | Native SQL | SDK driver |
| --- | --- | --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Flag unavailable, no statement | Pass | No execution | No execution |
| 12.1.0.1, ambiguous 12.1 or 12.2, 12.2.0.0 | 12c patch-set gate rejects before querying | Pass | No execution | No execution |
| 12.1.0.2 and 12.2.0.1 | [12.1](https://docs.oracle.com/database/121/REFRN/GUID-C8E5500E-A013-45D3-AEFB-2C9C0AC87EC7.htm) and [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_ROLES.html), `ROLE` and `ORACLE_MAINTAINED` | Pass | Unqualified | Unqualified |
| 18c, 19c, 21c | Same columns in [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_ROLES.html), [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_ROLES.html) and [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/DBA_ROLES.html) | Pass | Unqualified | Unqualified |
| 23c/23ai | Numeric 23 selector. The live 23 reference redirects to 26ai. | Pass | Unqualified | Unqualified |
| 26ai, 23.26.1.0.0 | [26ai DBA_ROLES](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_ROLES.html), same two columns | Pass | Exact selection and independent flag counts passed in both SYSDBA CDB root scopes | Go 1.27.4 and go-oracle-session v0.23.0 V2 buffered read passed in both scopes |

The 26ai primary was OPEN READ WRITE. The physical standby was OPEN READ ONLY
WITH APPLY. Each observation contained 119 roles with `Y` and zero with `N`
or SQL NULL. Fixture tests exercise `N`, SQL NULL, empty results and rejected
evidence. Native positive `N` and NULL, native Rows and JSON, other RUs, PDB,
non-CDB, MOUNT and non-SYSDBA cases remain unqualified for this selector.
Oracle references checked 2026-11-23.
