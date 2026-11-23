# Classified grant query compatibility

Oracle references checked 2026-11-23 for issue #125. Each selector reads named
users and roles in the admitted dictionary scope. The Oracle principal
namespace prevents a user and role from sharing a name. The join compares
binary name keys, excludes exact `PUBLIC`, and leaves case and whitespace in
every returned identifier intact. Name and owner restrictions are bound.
[ORA-01920](https://docs.oracle.com/en/error-help/db/ora-01920/),
[ORA-01921](https://docs.oracle.com/en/error-help/db/ora-01921/).

## Base projections

| Selector | Dictionary views | Base aliases |
| --- | --- | --- |
| `ObjectPrivilegeGrants` | `DBA_TAB_PRIVS`, `DBA_USERS`, `DBA_ROLES` | `OWNER`, `GRANTOR`, `GRANTEE`, `GRANTEE_KIND`, `OBJECT_TYPE`, `TABLE_NAME`, `PRIVILEGE`, `GRANTABLE`, `HIERARCHY` |
| `ColumnPrivilegeGrants` | `DBA_COL_PRIVS`, `DBA_USERS`, `DBA_ROLES`, `DBA_OBJECTS` | `OWNER`, `GRANTOR`, `GRANTEE`, `GRANTEE_KIND`, `OBJECT_TYPE`, `TABLE_NAME`, `COLUMN_NAME`, `PRIVILEGE`, `GRANTABLE` |
| `RoleMemberships` | `DBA_ROLE_PRIVS`, `DBA_USERS`, `DBA_ROLES` | `GRANTEE`, `GRANTEE_KIND`, `GRANTED_ROLE`, `ADMIN_OPTION`, `DEFAULT_ROLE` |

`GRANTEE_KIND` is `USER` or `ROLE`. Object-level `OBJECT_TYPE` is the raw
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