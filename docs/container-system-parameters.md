# Container-scoped scalar system parameters

go-oracle-info [issue #82](https://git.lan.orcl.com.br/marcuoli/go-oracle-info/issues/82) adds the `ContainerSystem` fact family. Use it for initialization-parameter observations keyed by reporting instance and source container. For example, an application can request `cpu_count`, `sga_target`, `pga_aggregate_limit`, `max_iops`, and `max_mbps` and retain only the rows actually visible for the corresponding containers.

## Source and result contract

| Source choice | Reporting `INST_ID` | Returned scope |
| --- | --- | --- |
| `parameters.ContainerSystemLocal` | Current instance from `SYS_CONTEXT('USERENV','INSTANCE')` | Visible `V$SYSTEM_PARAMETER` rows on the executing member |
| `parameters.ContainerSystemGlobal` | `GV$SYSTEM_PARAMETER.INST_ID` | Visible rows on qualified reporting members |

Both choices project `INST_ID, CON_ID, NAME, VALUE, DISPLAY_VALUE` in that order. The numeric identity columns use integer JSON scalars. Raw values and display values use text and retain SQL NULL separately; text such as `0`, a comma-delimited list, or a human-formatted size stays unchanged. A missing parameter can produce no row, and a result missing a projected column is malformed. Row order and repeated names remain intact. SQL orders by reporting instance, source container, and binary parameter name; equal keys have source order within the observed cursor.

The library uses the numeric release and actual executing container ID already observed on the caller-admitted handle. A CDB root (ID 1) may select every visible `CON_ID` or bind exact IDs. ID 0 in a filter selects whole-CDB data, kept separate from root/seed/PDB observations. A connection outside the CDB root is limited to its exact observed nonzero current container ID. Known non-CDB scope (current ID 0) returns `ErrContainerSystemUnsupportedScope` before SQL. Names compare exactly under binary collation and instance filters apply to reported IDs. An empty name or instance filter admits all visible values for that dimension.

Oracle determines visibility through the admitted account, container state and, for a common user in the root, `CONTAINER_DATA`. Joining a container data object to a non-container data object can restrict root results to whole-CDB rows. The selectors read a single view and preserve the source ID for the consumer to match against its independent inventory. A complete cursor establishes only the observed selection. Parameter availability and mutability vary by release, container and configuration. Consumer policy owns identity joins, peer selection, default/inheritance interpretation and numeric units or rollups. Existing scalar `System`, list `Effective`, session and stored SPFILE facts have separate contracts. [Oracle 12.1 Multitenant Architecture](https://docs.oracle.com/database/121/CNCPT/cdblogic.htm) · [Oracle 19c Monitoring CDBs and PDBs](https://docs.oracle.com/en/database/oracle/oracle-database/19/multi/viewing-information-about-cdbs-and-pdbs-with-sql-plus.html) · [Oracle 26ai Multitenant Administrator's Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/multi/administering-a-cdb-with-sql-plus.html) · [Oracle 12.2 SQL Language Reference, SYS_CONTEXT](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/sqlrf/SYS_CONTEXT.html)

`ContainerSystemStatement` and `DecodeContainerSystem` share the same projection, including filters. `ReadContainerSystem` requires an admitted bounded complete reader and stable SQL reference. `ContainerSystemRows` returns the original cursor and leaves iteration, `Err`, `Close` and stream budgets with the caller. `ContainerSystemJSON` uses a separate bounded cursor and publishes no prefix. Completed Read and JSON observations set provenance complete. Direct Decode validates the supplied rows without asserting transport completion. An explicit ORA-01031 source error is marked with `ErrContainerSystemInaccessible`. ORA-00942 is marked with `ErrContainerSystemUnavailable` because it can reflect an unresolved or inaccessible object. Other query failures retain their cause under a source-labeled `*parameters.Error`. Empty success, SQL NULL, missing projection, unsupported scope/release and failed queries remain distinct. [ORA-01031](https://docs.oracle.com/en/error-help/db/ora-01031/) · [ORA-00942](https://docs.oracle.com/en/error-help/db/ora-00942/)

## PDB-only effective settings

`Client.PDBParameters` and `Client.PDBParametersJSON` use
`parameters.ContainerSystemPDBGlobal` with the version and current container
observed on the caller's handle. The facade requires access to `V$INSTANCE`
for release selection and `V$DATABASE` for connected-container admission.
Use the source with `ReadContainerSystem` when the caller already owns a
complete buffered reader and independently admitted scope.

~~~go
data, err := client.PDBParametersJSON(ctx, []int64{3},
    oracleinfo.ParameterFilter{Names: []string{"open_cursors"}})
~~~

An empty PDB ID filter from `CDB$ROOT` returns all visible rows with
`CON_ID > 2`. A direct PDB connection binds only its own
container ID. The selector excludes whole-CDB rows, `CDB$ROOT` and the CDB's
`PDB$SEED`. Visible application containers with IDs above 2 are in scope.
Exact PDB IDs, parameter names and reporting instance IDs can narrow results.
The selected columns and cursor contract are the same as for
`ContainerSystemGlobal`.

The linked article examines effective parameter values and their `CON_ID` in
`V$SYSTEM_PARAMETER`. A PDB-attributed row reports the effective setting
visible for that container. Oracle can return rows with default values too.
Neither the row's presence nor the absence of a row establishes independent
configuration, inheritance, or storage. A mounted standby can return an empty
PDB selection. The API uses Oracle's documented view and does not read the
internal `PDB_SPFILE$` table. For stored and instance-wide observations, use
the separate [parameter contracts](system-parameters.md).
[Oracle 19c Multitenant Administrator's Guide](https://docs.oracle.com/en/database/oracle/oracle-database/19/multi/viewing-information-about-cdbs-and-pdbs-with-sql-plus.html) ·
[Oracle 26ai ALTER SYSTEM inheritance and scope](https://docs.oracle.com/en/database/oracle/oracle-database/26/sqlrf/ALTER-SYSTEM.html) ·
[Article](https://www.krenger.ch/blog/parameters-of-pluggable-databases/)


## Release and qualification matrix

Oracle public documentation was checked on 2026-11-21. Both projections use the same scalar columns in the admitted numeric families. Oracle describes `INST_ID` as the additional `GV$` column for qualified instances. Observed version text is independent of product branding. The current 26ai reference covers the numeric 23 release series and explicitly documents RU 23.26.4. The archived 23 documentation path currently redirects to the 26ai reference. [Oracle 12.1 RAC Performance Views](https://docs.oracle.com/database/121/RACAD/GUID-04B6632F-EC4E-4317-9BD6-D7991C429DD5.htm) · [Oracle 26ai RAC Performance Views](https://docs.oracle.com/en/database/oracle/oracle-database/26/racad/monitoring-performance/performance-views-in-oracle-rac.html) · [Oracle 26ai Database Reference RU revisions](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/changes-this-release-oracle-database-reference.html) · [Oracle 26ai RU 23.26.4](https://docs.oracle.com/en/database/oracle/oracle-database/26/nfcoa/ru23_26_4.html)

The table lists release identifiers supplied to the selector in fixture checks. Native Oracle execution and driver/application qualification for these exact statements are tracked independently.

| Oracle release / fixture version | Oracle documentation and applicability | Static selection and decoding | Native SQL | Driver/application |
| --- | --- | --- | --- | --- |
| 9iR2 / 9.2.0.8.0 | [9.2 Database Reference](https://docs.oracle.com/cd/B10501_01/server.920/a96536/ch3218.htm), no multitenant `CON_ID` | Unsupported before SQL | No execution | No execution |
| 10gR1 / 10.1.0.5.0 | [10.1 Database Reference](https://docs.oracle.com/cd/B13789_01/server.101/b10755/dynviews_2132.htm), no multitenant `CON_ID` | Unsupported before SQL | No execution | No execution |
| 10gR2 / 10.2.0.5.0 | [10.2 Database Reference](https://docs.oracle.com/cd/B19306_01/server.102/b14237.pdf), no multitenant `CON_ID` | Unsupported before SQL | No execution | No execution |
| 11gR1 / 11.1.0.7.0 | [11.1 Database Reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320.pdf), no multitenant `CON_ID` | Unsupported before SQL | No execution | No execution |
| 11gR2 / 11.2.0.4.0 | [11.2 Database Reference](https://docs.oracle.com/cd/E18283_01/server.112/e17110/dynviews_3097.htm), no multitenant `CON_ID` | Unsupported before SQL | No execution | No execution |
| 12cR1 / 12.1.0.1.0 and 12.1.0.2.0 | [12.1 Database Reference](https://docs.oracle.com/database/121/REFRN/GUID-2F2801AE-44CB-4C86-ADBE-91CFBC0B8407.htm), scalar columns and `CON_ID` documented for CDB | Both selections checked | Pending | Pending |
| 12cR2 / 12.2.0.1.0 | [12.2 Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-SYSTEM_PARAMETER.html) | Both selections and local Rows/JSON fixture checked | Unexecuted | Existing LABPDB connection failed before SQL. No approved root entry |
| 18c / 18.0.0.0.0 | [18c Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-SYSTEM_PARAMETER.html) | Both selections checked | Pending | Pending |
| 19c / 19.32.0.0.0 | [19c Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER.html) | Both selections and error/limit fixtures checked | Pending | Pending |
| 21c / 21.0.0.0.0 | [21c Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-SYSTEM_PARAMETER.html) | Both selections checked | Pending | Pending |
| 23c/23ai / 23.4.0.0.0 and 23.6.0.0.0 | [23 Database Reference path](https://docs.oracle.com/en/database/oracle/oracle-database/23/refrn/V-SYSTEM_PARAMETER.html) redirects to current 26ai reference | Both selections checked | Pending | Pending |
| 26ai / 23.26.4.0.0 | [26ai Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER.html) and [RU notes](https://docs.oracle.com/en/database/oracle/oracle-database/26/nfcoa/ru23_26_4.html) | Both selections and global decode/reader fixture checked | 23.26.1.0.0 root and PDB | Both Rows and JSON via go-ora v2.9.0 |

On 23.26.1.0.0, the approved existing Oracle primary returned complete
observations as SYSTEM without SYSDBA, in READ WRITE CDB$ROOT and OMLAB.
The root returned 11 local rows for the exact name filter and 651 global
rows. OMLAB returned 1 local and 47 global rows. The harness used Windows
amd64, Go 1.27.4, go-oracle-session v0.15.0 and go-ora/v2 v2.9.0. It checked
native cursor iteration, column aliases and complete bounded JSON. The
buffered Reader path is covered by fixtures. Earlier application captures
on 12.2, 19.32 and 23.26.1 used different statements. Other releases, MOUNT,
other login privileges and multi-member RAC remain unqualified.

## PDB-only scope qualification

Oracle's public references were rechecked on 2026-11-27. Issue #224 uses the
same global columns listed above with the PDB predicate `p.CON_ID > 2`.
Its binary name and instance filters retain the same binds. A direct PDB
also binds its exact current container ID. The facade uses the existing
connected-architecture observation for scope admission.

| Numeric release | Exact selection and decoding | Native SQL and driver |
| --- | --- | --- |
| 9.2, 10.1, 10.2, 11.1, 11.2 | Unsupported before the PDB query | Unexecuted |
| 12.1.0.1, 12.1.0.2, 12.2.0.1, 18.4 | Reviewed columns and predicate, fixture checked | Unexecuted for the PDB-only selection |
| 19.32.0.0.0 | Fixture checked | SYSDBA primary READ WRITE, CDB root and direct PDB |
| 21.3, 23c/23ai numeric 23.9 | Reviewed columns and predicate, fixture checked | Unexecuted for the PDB-only selection |
| Oracle 26ai numeric 23.26 | Fixture checked | 23.26.1.0.0 SYSDBA primary READ WRITE roots and direct PDB, plus a MOUNTED physical standby root |

The native Windows amd64 Go 1.27.4 probe used go-oracle-session v0.25.1
compatibility V2. It compared native cursor, bounded JSON and buffered results
with independent local-view counts on each single-instance fixture. The 19c
primary returned 25 rows from the root and the direct PDB. The 26ai primaries
returned 47. An exact container and name filter returned one row in the open
scopes. The 26ai mounted standby returned a complete empty selection. Ordinary
accounts, multi-instance RAC, other RUs and consuming applications remain
unqualified for the PDB-only selection.
