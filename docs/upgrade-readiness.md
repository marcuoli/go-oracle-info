# Upgrade readiness facts

The application owns the assessment and chooses which admitted Oracle handles
to query. Each successful response covers its selected source and scope.
Empty observations retain their provenance and the application can report
unavailable sources separately.

## Recorded PDB plug-in violations

Use `containers.PlugInViolationsStatement`, `DecodePlugInViolations`,
`ReadPlugInViolations`, `PlugInViolationsRows`, and `PlugInViolationsJSON`.
The query inspects `PDB_PLUG_IN_VIOLATIONS`, including any recorded prospective
PDB names. It does not run a compatibility check, join against the live
container inventory, or change a violation's status.

| Numeric release | Documented projection and selector | Static coverage | Native SQL and driver coverage |
| --- | --- | --- | --- |
| 9.2, 10.1/10.2, 11.1/11.2 | Multitenant source unsupported | Rejects before query | Inapplicable |
| 12.1.0.1/12.1.0.2 | CDB root only. TIME, NAME, CAUSE, TYPE, ERROR_NUMBER, LINE, MESSAGE, STATUS, ACTION | Both patch-set selectors and absent CON_ID verified | Unqualified |
| 12.2.0.1, 18c, 19c, 21c, 23ai | Adds nullable CON_ID. Root or connected container selected by scope | Both admission predicates and nullable CON_ID verified | Unqualified |
| 26ai (23.26) | Same projection. Numeric server version is required | 23.26.1 and 23.26.4 selectors verified | 23.26.1.0.0 primary READ WRITE and physical standby READ ONLY WITH APPLY, both CDB$ROOT and SYSDBA. Rows and JSON completed through Windows amd64, Go 1.27.4, go-oracle-session v0.23.0 V2. Independent counts matched zero. The timestamp expression also returned six fractional digits for a nonzero timestamp literal. |

`Root` requires a caller-admitted root and selects visible findings, including
rows with zero or NULL `CON_ID`. `CurrentContainer` restricts rows to the
connected `CON_ID` where the view projects it. Direct PDB visibility, nonempty
native findings, ordinary accounts, RAC and MOUNT remain unqualified. A
successful empty table applies only to the admitted source and scope. Oracle
requires the connection to be able to query the selected view. Missing or
inaccessible view errors retain their classified cause.

TIME is civil Gregorian timestamp text with six fractional digits and no
timezone. LINE and ERROR_NUMBER are exact signed integer text. The optional
CON_ID is also exact text. Rows preserve the required type and the raw status,
including PENDING, RESOLVED, IGNORE, and SQL NULL. In typed results a nil ContainerID
represents the Oracle 12.1 projection, while a present value can be NULL or zero.
Native Rows transfers cursor ownership to the caller. Read validates the
complete buffered observation against raw-text and row bounds. JSON has its own
cursor and envelope-inclusive budget.

Oracle references checked 2026-11-24.

- [12.1 PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/database/121/REFRN/GUID-845E5369-CCB0-4F8D-AE09-447EF0CAC93F.htm)
- [12.2 PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [18c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [19c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [21c PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [26ai PDB_PLUG_IN_VIOLATIONS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/PDB_PLUG_IN_VIOLATIONS.html)
- [26ai upgrade troubleshooting in the target CDB root](https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/troubleshooting-oracle-database-upgrade.html)
