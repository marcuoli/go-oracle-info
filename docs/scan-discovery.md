# SQL-only SCAN discovery

Use `Client.Scan` for a typed observation or `Client.ScanJSON` for bounded
standard JSON. Call `services.ReadScan` if you already have a complete bounded
session reader, the observed numeric version, and two SQL references.

Both methods check the effective `REMOTE_LISTENER` and `LISTENER_NETWORKS`
values in `GV$SYSTEM_PARAMETER2`. The observation preserves raw values, SQL NULL,
list ordinals, reporting instances and available container IDs. Only listener
addresses in the `REMOTE_LISTENER` clause of a named network contribute to the
candidate. Local and forward listener addresses are ignored.

A candidate needs one consistent explicit hostname. Each observed reporting
instance and container must provide both parameters. Supported inputs include
`host:port` and TCP/TCPS address descriptors in the documented listener
parameter syntax. An empty `Name` (omitted in JSON) means unknown. IP-only
settings, mixed hostname and IP settings, unresolved aliases, conflicting
names and unrecognized syntax remain unknown. No DNS, reverse DNS, alias
resolution or Grid Infrastructure calls are part of the observation.

`GV$LISTENER_NETWORK` adds the visible remote registration rows. These rows
retain their reported address or descriptor, including literal IPs. They
provide independent registration evidence and do not establish the SCAN DNS
name. A classified privilege or view-availability rejection while opening the
borrowed registration cursor is recorded as a registration state, and
independently valid configuration remains available. The complete buffered
reader can also report failures during fetch or close. An error without
known open-stage provenance fails the observation. A successfully empty
registration cursor is reported as `no_rows`.

Use `Client.ListenerNetworks` or `services.ListenerNetworksRows` for the actual
native cursor, and their JSON companions for a separate bounded result.
`MaxRows` also limits the combined number of parameter and registration rows
returned by SCAN discovery. Both observations use the caller-admitted scope.
A successful read establishes transport completion for the selections.
Clusterware SCAN identity is established by `srvctl config scan`.

## Release checks

SQL and decoder fixture checks cover both selected projections. The parameter
source is the existing filtered effective parameter API. The registration
source selects `INST_ID`, `NETWORK`, and `VALUE` where `TYPE` is
`REMOTE LISTENER`. From 12c it also selects `CON_ID` and restricts rows to
whole-database or current-container visibility.

| Release | Selection | Native SQL with SYSDBA | Go driver and SDK |
| --- | --- | --- | --- |
| 9iR2 (9.2), 10gR1 (10.1), 10gR2 (10.2), 11gR1 (11.1) | Unsupported before SQL. Oracle introduced RAC SCAN in 11gR2. | No discovery SQL selected. | Unsupported selection tested. |
| 11gR2 (11.2) | Legacy projection without `CON_ID`. | 11.2.0.2.0, non-CDB, primary READ WRITE. | Fixture coverage. |
| 12cR1 (12.1) | Container-aware projection. | 12.1.0.2.0, CDB root, primary READ WRITE. | Fixture coverage. |
| 12cR2 (12.2) | Container-aware projection. | 12.2.0.1.0, CDB root, primary READ WRITE. | Fixture coverage. |
| 18c | Container-aware projection. | 18.4.0.0.0, CDB root, primary READ WRITE. | Fixture coverage. |
| 19c | Container-aware projection. | 19.32.0.0.0, CDB root, primary READ WRITE. | See Go driver evidence below. |
| 21c | Container-aware projection. | 21.3.0.0.0, CDB root, primary READ WRITE. | Fixture coverage. |
| 23ai | Container-aware projection. | 23.9.0.25.07, CDB root, primary READ WRITE. | Fixture coverage. |
| 26ai (numeric 23.26) | Container-aware projection. | 23.26.1.0.0, CDB roots, primary READ WRITE and physical standby READ ONLY WITH APPLY. | See Go driver evidence below. |

The native SQL checks were run on 2026-11-24. All fixtures were
single-instance and returned two parameter rows and zero remote registration
rows. Go unit fixtures exercise nonempty registration values, IP-only and mixed
inputs, multiple network descriptors, scope, and read failures. A populated
RAC registration view, ordinary accounts, MOUNT, other patch levels, and
consuming applications remain unqualified.

## Go driver checks

Go 1.27.4 on Windows amd64 and go-oracle-session v0.25.1 compatibility V2
verified the scoped borrowed cursor, bounded JSON, typed discovery, and
session-owned buffered discovery on the following SYSDBA connections:

- 19.32.0.0.0 primary READ WRITE root and direct PDB.
- 23.26.1.0.0 primary READ WRITE roots and direct PDB, and the physical standby
  READ ONLY WITH APPLY root.

Each returned two effective parameter rows, no registration rows, and an
unknown SCAN name. Other releases have SQL and decoder fixture coverage.


Oracle sources reviewed on 2026-11-24 include the *Oracle Single Client Access
Name (SCAN)* guide, the *Oracle Database Reference* listener parameters, the
*MetaSolv Solution Data Selection Tool How-to Guide* listener view example, and
the *Oracle RAC Administration and Deployment Guide* SRVCTL command reference.
Oracle's Database Reference has no schema entry for the listener view. The
release-specific view coverage above comes from the exact select projection
and Oracle fixture execution.

- [Oracle SCAN guide](https://www.oracle.com/a/otn/docs/database/oracle-scan.pdf)
- [11.2 LISTENER_NETWORKS](https://docs.oracle.com/cd/E18283_01/server.112/e17110/initparams115.htm)
- [12.2 LISTENER_NETWORKS](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/LISTENER_NETWORKS.html)
- [19c REMOTE_LISTENER](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/REMOTE_LISTENER.html)
- [21c Oracle Net Listener configuration](https://docs.oracle.com/en/database/oracle/oracle-database/21/netag/configuring-and-administering-oracle-net-listener.html)
- [26ai LISTENER_NETWORKS](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/LISTENER_NETWORKS.html)
- [Oracle listener view example](https://docs.oracle.com/en/industries/communications/metasolv-solution/8.0/data-selection-tool/migrating-network-templates.html)
- [Oracle SRVCTL SCAN commands](https://docs.oracle.com/en/database/oracle/oracle-database/26/racad/server-control-utility-reference/scan-commands.html)
