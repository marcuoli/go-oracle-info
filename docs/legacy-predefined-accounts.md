# Predefined Oracle accounts on legacy releases

`users.LookupLegacyPredefinedAccounts(version)` returns exact usernames with
Oracle documentation and the release scope used by the lookup. For example,
`11.2.0.4.0` selects the 11gR2 documentation. Treat a matched name as a
documented predefined account and an unlisted name as unclassified. The
catalog does not enumerate a particular database or determine whether an
optional component was installed, removed or carried forward during an upgrade.

| Numeric release scope | Evidence reviewed on 2026-11-23 | Coverage |
| --- | --- | --- |
| 9.2 | [Getting Started for Windows, default accounts table](https://docs.oracle.com/cd/B10501_01/win.920/a95490/username.htm), [Administrator's Guide, DBCA accounts](https://docs.oracle.com/cd/B10501_01/server.920/a96521/dba.htm) | Windows default and documented additional DBCA accounts, with the demonstration account omitted. |
| 10.1 | [Database Security Guide, default accounts](https://docs.oracle.com/cd/B12037_01/network.101/b10773/policies.htm) | Typical DBCA database. Omit examples and generic recovery-catalog owners whose name alone is insufficient. |
| 10.2 | [Database Security Guide, table 7-2](https://docs.oracle.com/cd/B19306_01/network.102/b14266/policies.htm) | Typical DBCA database, including the documented `TSMSYS` entry. Same exclusions as 10.1. |
| 11.1 | [2 Day + Security Guide, tables 3-1 and 3-2](https://docs.oracle.com/cd/B28359_01/server.111/b28337/tdpsg_user_accounts.htm) | Predefined administrative and non-administrative accounts. Sample schemas are described in a separate table. |
| 11.2 | [2 Day + Security Guide, tables 3-1 and 3-2](https://docs.oracle.com/cd/E24693_01/server.11203/e10575/tdpsg_user_accounts.htm), [Installation Guide for Linux, table 6-1](https://docs.oracle.com/cd/E14075_01/doc/install.112/e10840.pdf), [Warehouse Builder Installation Guide](https://docs.oracle.com/cd/E25154_01/doc/owb.11203/e17130/install.htm) | Predefined accounts plus `APEX_030200` and `APPQOSSYS` from the documented Linux installation, and `OWBSYS_AUDIT` from Warehouse Builder. Other APEX versions and component installations may create other names. |
| 12.1.0.1 | [2 Day + Security Guide, tables 2-4 and 2-5](https://docs.oracle.com/database/121/TDPSG/GUID-3EC7A894-D620-4497-AFB1-64EB8C33D854.htm), [12.1.0.1 New Features](https://docs.oracle.com/database/121/NEWFT/chapter12101.htm), [APEX release bundled with Oracle Database](https://docs.oracle.com/en/database/oracle/application-express/19.2/htmig/upgrading-from-previous-Oracle-application-express-release.html), [APEX schema names](https://docs.oracle.com/cd/E59726_01/install.50/e39144/overview.htm) | Release-family predefined administrative and non-administrative accounts, with 12.1.0.1 evidence for the new administrative privileges and unified auditing, and the documented bundled APEX schema. The catalog is not a PDB or root visibility claim. |
| 12.1.0.2, 12.2, 18c, 19c, 21c, 23c/23ai and 26ai | [Database Reference, DBA_USERS](https://docs.oracle.com/database/121/REFRN/GUID-309FCCB2-2E8D-4371-9FC5-7F3B10E2A8C0.htm) | No catalog selection. Use the observed dictionary `ORACLE_MAINTAINED` flag where admitted. |

The catalog does not use Zero Downtime Migration or Data Pump exclusions as
a database account list. Those exclusions include migration infrastructure
and have different scope. [Zero Downtime Migration guide](https://docs.oracle.com/en/database/oracle/zero-downtime-migration/26.1/zdmug/introduction-to-zero-downtime-migration.html)

The SDK lookup issues no SQL. Static tests cover release routing, the patch-set
cutoff, exact identifiers and stable caller-owned results. Native SQL and
driver qualification do not apply to this lookup. Installed account coverage
remains dependent on the database and component configuration.

For documented predefined role names, see the [legacy predefined role catalog](legacy-predefined-roles.md).
