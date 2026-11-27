# RAC member and network evidence

The admitted session can observe several distinct sources. Keep each result with
its own reporting instance, container, capture time and source.

| Observation | API or owner | Meaning |
| --- | --- | --- |
| Visible database members | `instances.ReadVisibility`, `ReadMemberStartup`, `ReadVersions` | Qualified `GV$INSTANCE` rows. A missing row leaves visibility or coverage unknown. |
| Database and container identity | `identity.ReadLogicalDatabase`, the admitted container/instance observations, and `database.ReadConfiguration` | The logical database identifiers and the raw `CLUSTER_DATABASE` parameter support a caller's admission policy. They do not identify a network endpoint. |
| Active RAC interconnects | `instances.ReadClusterInterconnects` | Database-observed interfaces being used for cluster traffic. |
| SCAN-related database settings and listener registrations | `services.ReadScan`, `services.ReadListenerNetworks` | Independently qualified [SQL settings and remote registrations](scan-discovery.md), which may contain hostnames, aliases, descriptors, or IP text. |
| Configured nodes, node VIPs and SCAN VIPs | Oracle Clusterware configuration from the operator | Oracle documents OLSNODES, SRVCTL VIP and SRVCTL SCAN as configuration sources. This library only consumes an admitted database handle. |
| Client DNS and the final TCP peer | Client network and connection owner | DNS responses describe that resolver's view. The socket peer is observed by the caller for each connection after any redirect. |

An interconnect row with `IS_PUBLIC=YES` describes the interface reported by
Oracle. It cannot identify a client-facing node VIP or SCAN VIP. A reported
`SOURCE` mentioning the Oracle Cluster Registry describes where Oracle found
the interface. The database views do not enumerate all configured Clusterware
members or prove that every expected member was reachable. Supply any expected
member set from a qualified independent source and compare it at the caller.

## Runtime interconnect fact

`ClusterInterconnectsStatement`, `DecodeClusterInterconnects`,
`ReadClusterInterconnects`, `ClusterInterconnectsRows` and
`ClusterInterconnectsJSON` share one simple `SYS.GV_$CLUSTER_INTERCONNECTS`
projection. The facade exposes `Client.ClusterInterconnects` and
`Client.ClusterInterconnectsJSON` over the caller's native handle.

Columns are `INST_ID`, `NAME`, `IP_ADDRESS`, `IS_PUBLIC`, `SOURCE`,
and from 12.1, `CON_ID`. The result retains raw nullable text, including
unknown flag values and unparsed addresses. An omitted container column before
12.1 is distinct from a projected NULL. `INST_ID` is a positive reporting
instance ID. Reporting and container IDs preserve their exact numeric text in
the typed result. The query has no join, sort, scope substitution or fallback.
The decoder retains cursor order and duplicate observations.

Use a root or non-CDB session with existing permission to read the selected
view for database-wide visibility. PDB sessions, MOUNT, and other privilege
levels have independently scoped access and visibility. The API does not change
that admission. A successful empty result is complete for the selected source.
Only Read and JSON can assert complete transport. Known borrowed cursor-open
privilege and source errors retain their classifications. Buffered reader,
iteration, closure, and cancellation failures retain their original causes.
No failed prefix is returned.

## Release checks

The static SQL and decoder matrix exercises the exact common projection,
including each supported legacy and multitenant branch. The current 26ai
reference also describes `PNAME` and `TRANSPORT`. The common projection
does not require either column.

| Oracle release | Oracle documentation and selected columns | Static SQL/decoder | Native SQL and Go driver |
| --- | --- | --- | --- |
| 9iR2 (9.2), 10gR1 (10.1) | The 10.2 reference lists the source as new. Unsupported before executing. | Unsupported selection and JSON pre-query checks | No SQL selected |
| 10gR2 (10.2) | Introduced in 10.2. Core five columns. | Tested | Unqualified |
| 11gR1 (11.1), 11gR2 (11.2) | Oracle references list the same core columns. | Tested in both families | Unqualified |
| 12cR1 (12.1), 12cR2 (12.2) | Oracle references also list `CON_ID`. | Tested in both families | Unqualified |
| 18c | Core plus `CON_ID`. | Tested | Unqualified |
| 19c | Core plus `CON_ID`. | Tested at 19.3 and 19.33 | See the two qualified SYSDBA root cases below |
| 21c | Core plus `CON_ID`. | Tested | Unqualified |
| 23ai | Core plus `CON_ID` selected by continuity. Oracle redirects the online 23 reference to 26ai. | Tested with 23.9 numeric version | Unqualified |
| 26ai (numeric 23.26) | Core plus `CON_ID` selected from the current reference. | Tested with 23.26.1 numeric version | Unqualified |

## Native checks

On 2026-11-27, a read-only probe used the exact new selector through Go 1.27.4
on Windows amd64 and released go-oracle-session v0.25.2 with go-ora V2.
It ran as SYSDBA on a 19.3.0.0.0 RAC primary in READ WRITE and a 19.33.0.0.0
RAC physical standby in READ ONLY WITH APPLY, both in CDB$ROOT.

Each scope reported database instances 1 and 2. The owned buffered read
returned two qualified interconnect rows, one per visible reporting member.
An independent select through the documented GV$ synonym matched the full
six-column row multiset on both sides, including raw address and source
values. The existing SCAN reader independently returned four effective
parameter observations, six remote registration rows, and a configured
hostname candidate on each side. Values identifying network endpoints
remain private. These checks qualify the typed buffered read and existing
SCAN reader for the two reported scopes. The borrowed cursor and JSON
contracts have fixture coverage. Other releases, PDBs, MOUNT, and ordinary
accounts remain unqualified for this fact.


Oracle sources checked on 2026-11-27:

- [10.2 new dynamic performance views](https://docs.oracle.com/cd/B19306_01/server.102/b14237/whatsnew.htm) and [10.2 column reference](https://docs.oracle.com/cd/B19306_01/server.102/b14237/dynviews_1065.htm).
- [11.1 column reference](https://docs.oracle.com/cd/B28359_01/server.111/b28320/dynviews_1074.htm) and [11.2 column reference](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_1083.htm).
- [12.1 column reference](https://docs.oracle.com/database/121/REFRN/GUID-96FEF40A-19C2-40CB-8666-E4F8D96343C7.htm) and [12.2 column reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-CLUSTER_INTERCONNECTS.html).
- [18c column reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-CLUSTER_INTERCONNECTS.html) and [19c column reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-CLUSTER_INTERCONNECTS.html).
- [21c column reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-CLUSTER_INTERCONNECTS.html) and [26ai column reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-CLUSTER_INTERCONNECTS.html).
- [GV$ visibility and simple dynamic-view queries](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/about-dynamic-performance-views.html) and [Oracle RAC interconnect guidance](https://docs.oracle.com/en/database/oracle/oracle-database/26/racad/monitoring-performance/verifying-the-interconnect-settings-for-oracle-rac.html).
- [OLS NODES configuration reference](https://docs.oracle.com/en/database/oracle/oracle-database/18/cwadd/olsnodes-command-reference.html), [19c SRVCTL command reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/racad/server-control-utility-reference.html), and [SRVCTL SCAN configuration](https://docs.oracle.com/en/database/oracle/oracle-database/26/racad/server-control-utility-reference/scan-commands.html).
