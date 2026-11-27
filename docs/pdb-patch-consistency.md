# Per-PDB SQL patch consistency

The opt-in root SQL patch reader extends the CDB registry history with
PATCH_TYPE, TARGET_VERSION and FLAGS on Oracle 18c and later admitted numeric
versions. FLAGS lets the existing evaluator distinguish JVM patches from
database release updates. The seven-column history APIs remain unchanged.
A selected SQL NULL stays NULL. A column omitted by the release selector stays
absent. The reader accepts an already-admitted handle and never changes its
connection, container or privileges.

EvaluatePDBConsistency reuses the ordinary release-level evaluator. Callers
admit the visible, open PDB and qualified reporting-member coverage from
separate evidence on the same CDB-root endpoint. A whole-database GV$INSTANCE
row with CON_ID 0 and CURRENT_CONTAINER_ID 1 retains both observed IDs. Each
PDB uses only its own CDB_REGISTRY_SQLPATCH rows and exact patch ID/UID history.
An empty inventory, rollback without an effective release, absent release
fields or inconclusive evidence stays unverified. A match covers release level
within the selected reporting scope.

## Query compatibility checked 2026-11-27

The projection uses SYS.CDB_REGISTRY_SQLPATCH and the original RAC-safe outer
NO_MERGE ordering by container, civil action time, numeric patch ID/UID and
action. The documented CDB view reads only visible open, unrestricted PDBs.
A completed root query may omit other PDBs even with SYSDBA.

| Oracle release | Documentation and static selector/decoder outcome | Native SQL | Driver/application |
| --- | --- | --- | --- |
| 9iR2 9.2.0.8 | No multitenant source. Unsupported without SQL in selector test. | No new query selected | No source |
| 10gR1 10.1.0.5 | No multitenant source. Unsupported without SQL in selector test. | No new query selected | No source |
| 10gR2 10.2.0.5 | No multitenant source. Unsupported without SQL in selector test. | No new query selected | No source |
| 11gR1 11.1.0.7 | No multitenant source. Unsupported without SQL in selector test. | No new query selected | No source |
| 11gR2 11.2.0.1–11.2.0.4 | No multitenant source. 11.2.0.4 selector test rejects without SQL. | No new query selected | No source |
| 12cR1 12.1.0.1 | Existing CDB patch selector rejects without SQL in boundary test. | No new query selected | No source |
| 12cR1 12.1.0.2 | 12.1 reference describes legacy VERSION/BUNDLE columns. Selector test preserves the seven-column statement. | No new query selected | Release consistency unavailable |
| 12cR2 12.2.0.1 | 12.2 reference describes legacy VERSION/BUNDLE columns. Tests preserve seven columns and typed absence. | No new query selected | Release consistency unavailable |
| 18c 18.1.0.0.0 | 18c documents PATCH_TYPE and TARGET_VERSION as new in 18c. Selector test enables all three fields. | Not run for this projection | Needs an admitted 18c CDB-root fixture |
| 19c 19.3.0.0.0 | 19c documents the three fields. Selector and exact/null decoding tests pass. | Executed via the driver on a SYSDBA two-member RAC primary in CDB root, READ WRITE. | Root and open PDB returned separate RU rows. PDB release matched both member binaries. |
| 19c 19.33.0.0.0 | Same 19c projection in a separate numeric-version selector case. | Executed via the driver on the paired SYSDBA RAC physical standby in CDB root, READ ONLY WITH APPLY. | The root returned its previous RU. The mounted PDB had no patch rows and stayed unverified. |
| 21c 21.3.0.0.0 | 21c documents the three fields. Selector tested. | Not run for this projection | Needs an admitted 21c CDB-root fixture |
| 23c / 23ai 23.1.0.0.0 and 23.9.0.0.0 | Oracle redirects the 23 reference to its current 26ai page. Numeric selectors tested within the existing 23-family contract. | Not run for this projection | Needs admitted 23c and 23ai root fixtures |
| 26ai numeric 23.26.1.0.0 | The current 26ai reference documents the fields. Selector chooses the modern shape for the observed 23 numeric major. A literal unsupported major 26 is rejected without SQL. | Not run for this projection | Needs an admitted 26ai CDB-root fixture |

The 19c qualification uses the normal buffered driver path. The 19c non-SYS,
direct PDB, MOUNTED CDB root, single-instance and other service-scope combinations
were not newly qualified. Direct PDB and non-CDB are outside this root selector.

Oracle references

- [12.1 DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/cd/E57425_01/121/REFRN/GUID-F6D2BC12-606C-41FE-B9E8-F3702CD32F89.htm)
- [12.2 DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_REGISTRY_SQLPATCH.html)
- [18c DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY_SQLPATCH.html)
- [19c DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY_SQLPATCH.html)
- [21c DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/DBA_REGISTRY_SQLPATCH.html)
- [26ai DBA_REGISTRY_SQLPATCH](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_REGISTRY_SQLPATCH.html)
- [19c CDB views](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/cdb_-views.html)
