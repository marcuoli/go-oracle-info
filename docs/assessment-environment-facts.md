# Member startup, password versions, and encrypted columns

The three #197 families use a caller-admitted Oracle session. Each has a
Statement, Decode, Read, Rows, and JSON API. The root Client exposes native
rows and separate bounded JSON reads for an existing `*sql.Conn` or
`*sql.Tx`. A complete read covers its selected source only.

| Family | Oracle projection | Interpretation |
| --- | --- | --- |
| `instances.MemberStartup` | `GV$INSTANCE`: `INST_ID`, `STARTUP_TIME`, `STATUS` | Reporting members visible to the admitted session. Startup is Gregorian civil time, to the second, without a timezone. Status is Oracle's instance status. Preserve SQL NULL and repeated members. |
| `users.PasswordVersionCounts` | `DBA_USERS`: `PASSWORD_VERSIONS` grouped with an exact `ACCOUNT_COUNT` | One count per binary-distinct raw version set in the connected dictionary, including a SQL NULL group when present. No usernames, verifier values, password material, or login outcomes. |
| `objects.EncryptedColumns` | `DBA_ENCRYPTED_COLUMNS`: `OWNER`, `TABLE_NAME`, `COLUMN_NAME` | Visible encrypted-column identities in the connected dictionary. This statement reads metadata without reading table data, a keystore view, or encryption keys. |

`PasswordVersionCount.Class()` distinguishes unreported (SQL NULL), `10g_only`,
`case_sensitive`, `http_only`, and unknown. `10g_only` means 10G is the only
listed database password verifier. It can coexist with the separate HTTP
digest token. An 11G or 12C label records a case-sensitive verifier format.
A set with an unrecognized token remains unknown. The raw text and count
remain available regardless of classification. Oracle's effective
authentication policy, account state and client protocol determine whether
an account can log in.

The dictionary queries require an open database or PDB and ordinary access
to the respective SYS-owned view. CDB root rows describe the connected
dictionary. Use a separate caller-admitted connection for each scope.
The instance source needs access to `SYS.GV_$INSTANCE`. Oracle documents
status in NOMOUNT, MOUNT and OPEN. Admission and actual visibility depend on
the login and instance/container scope. Readers execute exactly one statement on the admitted handle. An
unsupported version stops before fact SQL. Source privilege rejection
and unresolved/unavailable view errors retain typed causes. A successful
empty read has zero rows. Cancellation, malformed rows and exceeded bounds
discard the entire result.

## Per-query release matrix

Public reference check date 2026-11-24. **D/S** means the projection is
documented and its selector was checked statically. **U/S** means the SDK
rejects it before SQL and that decision was checked statically. **N**
marks the specific native checks below. A public reference is evidence for
its release family. Only the tested numeric lab release has native evidence.

| Numeric release family | Member startup | Password versions | Encrypted columns | Native SQL and driver |
| --- | --- | --- | --- | --- |
| 9.2 | D/S | U/S | U/S | Unexecuted |
| 10.1 | D/S | U/S | U/S | Unexecuted |
| 10.2 | D/S | U/S | D/S | Unexecuted |
| 11.1 | D/S | D/S | D/S | Unexecuted |
| 11.2 | D/S | D/S | D/S | Unexecuted |
| 12.1 | D/S | D/S | D/S | Unexecuted |
| 12.2 | D/S | D/S | D/S | Unexecuted |
| 18 | D/S | D/S | D/S | Unexecuted |
| 19 | D/S | D/S | D/S | Unexecuted |
| 21 | D/S | D/S | D/S | Unexecuted |
| 23/23ai | D/S | D/S | D/S | Unexecuted |
| 26ai, observed numeric 23 series | D/S | D/S | D/S | N on 23.26.1.0.0 only |

No source-column transition beyond the 10.2 and 11.1 entry points was found
for these projections. The 23 reference URLs redirected to the 26 manual
during the review. Earlier 23-family static selection remains explicit.
The compatibility selectors use the observed numeric Oracle version.

On 23.26.1.0.0, the paired CDB roots were checked as SYS AS SYSDBA on
Windows amd64 Go 1.27.4, CGO disabled, with go-oracle-session v0.25.1
and go-ora V2. The primary was READ WRITE and the physical standby was
READ ONLY WITH APPLY. Each selected one startup row, two password-version
groups covering 36 accounts, and zero encrypted-column rows. The typed startup tuple, sum of password-version counts,
and empty encrypted inventory matched independent SELECTs. Native cursors and
bounded JSON matched each other cell for cell and the typed reads by row count.
The encrypted identities and their SQL NULL shape were exercised with
fixtures. Other roles, release/RUs, database states, PDB/non-CDB,
multiple-member deployments and positive encrypted-column rows remain
unqualified by those native runs.

## Public Oracle references

- 9.2 documents the instance observations and the reporting instance ID in the global dynamic views. [V$INSTANCE](https://docs.oracle.com/cd/B10500_01/server.920/a96536/ch383.htm) · [Oracle9i RAC Deployment and Performance](https://docs.oracle.com/cd/B10500_01/rac.920/a96598.pdf)
- The 10.1 reference lacks the encrypted-column view. The 10.2 DBA view documents the projection through its ALL counterpart. [10.1 Reference index](https://docs.oracle.com/cd/B13789_01/server.101/b10755/toc.htm) · [10.2 DBA_ENCRYPTED_COLUMNS](https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_3102.htm) · [10.2 ALL_ENCRYPTED_COLUMNS](https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_1051.htm)
- The 10.2 DBA_USERS reference omits PASSWORD_VERSIONS. The 11.1 and 11.2 references document it. [10.2](https://docs.oracle.com/cd/B19306_01/server.102/b14237/statviews_4174.htm) · [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_5073.htm) · [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5081.htm)
- The 12.1 through 26 references retain the three selected projections. [12.1](https://docs.oracle.com/database/121/REFRN/toc.htm) · [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/) · [18c](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/) · [19c](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/) · [21c](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/) · [26ai V$INSTANCE](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html) · [26ai DBA_USERS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_USERS.html) · [26ai DBA_ENCRYPTED_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_ENCRYPTED_COLUMNS.html)
- Oracle desupported SEC_CASE_SENSITIVE_LOGON in 21c. Password-version groups are observation evidence. [21c desupports](https://docs.oracle.com/en/database/oracle/oracle-database/21/upgrd/oracle-database-changes-deprecations-desupports.html) · [26ai password case sensitivity](https://docs.oracle.com/en/database/oracle/oracle-database/26/dbseg/managing-password-case-sensitivity.html)
