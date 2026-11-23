# Predefined Oracle roles on legacy releases

`grants.LookupLegacyPredefinedRoles(version)` returns exact documented names and
their Oracle sources. It uses the same numeric release scope as the
[predefined account catalog](legacy-predefined-accounts.md). Optional
components, installed editions and upgrades can affect the roles present on
an instance. A role absent from the catalog remains unclassified.

| Numeric release | Oracle sources reviewed 2026-11-23 | Documented role names |
| --- | --- | ---: |
| 9.2 | [Oracle9i SQL Reference, Table 17-2](https://docs.oracle.com/cd/A97385_01/server.920/a96540/statements_912a.htm) | 13 |
| 10.1 | [Database Security Guide, Table 10-1](https://docs.oracle.com/cd/B13789_01/network.101/b10773/admusers.htm) | 12 |
| 10.2 | [Database Security Guide, Table 11-1](https://docs.oracle.com/cd/B19306_01/network.102/b14266/admusers.htm) | 12 |
| 11.1 | [Database Security Guide, Table 4-3](https://docs.oracle.com/cd/B28359_01/network.111/b28531/authorization.htm) | 52 |
| 11.2 | [Database Security Guide, Table 4-3](https://docs.oracle.com/cd/E18283_01/network.112/e16543/authorization.htm) | 56 |
| 12.1.0.1 | [12.1 Database Security Guide](https://docs.oracle.com/database/121/DBSEG/authorization.htm), [12.1.0.1 New Features](https://docs.oracle.com/database/121/NEWFT/chapter12101.htm), [Database 2 Day DBA](https://docs.oracle.com/database/121/ADMQS/E51671-10.pdf) | 56 |

The 12.1 security guide covers the release family. The 12.1.0.1 catalog
includes carryover roles also present in the 11.2 table and roles for
components documented in the 12.1.0.1 New Features chapter. These include
Unified Auditing, Privilege Analysis, Multitenant and EM Express. The 2 Day
DBA guide confirms the spelling `EM_EXPRESS_BASIC`.

When the database projects it, `DBA_ROLES.ORACLE_MAINTAINED` is the live
classification for a role in its connected dictionary scope. This package
uses 12.1.0.2 and later as the conservative application boundary when
maintenance flags are required. The catalog lookup issues no SQL and does
not accept ambiguous numeric `12.1` input. [Oracle 19c DBA_ROLES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_ROLES.html)

Oracle exposes `PUBLIC` separately as a special grantee. The legacy tables
document predefined roles, while `DBA_ROLE_PRIVS` describes membership without
exposing a grantor. The catalog and the dictionary flag classify roles.
Determining who granted a role requires independent evidence. [Oracle DBA_ROLE_PRIVS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_ROLE_PRIVS.html)
