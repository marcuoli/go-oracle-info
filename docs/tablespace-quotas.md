# Tablespace quotas and direct system privileges

The `users` and `grants` responsibility packages expose two separate complete
fact families. They use the existing Options, Request, Reader, Provenance and
rowset.Limits contracts. Connections, dictionary/container admission, references,
finite budgets, freshness and comparison/report policy remain caller-owned.

## Quota facts

`users.TablespaceQuota` contains Username, TablespaceName, Bytes and MaxBytes as
session.Value, plus Dropped as *session.Value. Numeric values are exact integer
text, including values beyond int64; SQL NULL stays NULL. Bytes means Oracle's
bytes charged to the user, while MaxBytes is the assigned byte limit. MaxBytes
`-1` means unlimited. Zero, NULL, a missing row and a failed observation differ.
A reduced quota can be below already charged bytes; do not infer Bytes<=MaxBytes
or round either field to a display unit. Unknown raw values are not invented
finite or unlimited entitlements.

DROPPED is documented and selected in every admitted 11g-and-later projection.
The pointer is therefore nonnil in current successful rows, including a selected
SQL NULL. Nil remains distinct from source NULL. Raw YES/NO/unknown flags and all
rows are preserved; no dropped-tablespace or account exclusion policy is applied.

The complete family is:

- TablespaceQuotasStatement(Options) (string, []any, error)
- DecodeTablespaceQuotas(session.Result, Options) (TablespaceQuotasResult, error)
- ReadTablespaceQuotas(context.Context, Reader, Request) (TablespaceQuotasResult, error)
- TablespaceQuotasRows(context.Context, rowset.Queryer, Options) (*sql.Rows, error)
- TablespaceQuotasJSON(context.Context, rowset.Queryer, Options, rowset.Limits) (json.RawMessage, error)

TablespaceQuotasResult contains Provenance and Records []TablespaceQuota. The
selected DBA_TS_QUOTAS source explicitly aliases USERNAME, TABLESPACE_NAME,
BYTES, MAX_BYTES and DROPPED, ordered by username/tablespace. Request.Options.Names
selects exact usernames using bound binary predicates. Nonempty Owners is invalid.
An empty Names list selects all visible source rows, without hidden user filters.

## Direct system-privilege facts

`grants.DirectSystemGrant` contains Grantee, Privilege and AdminOption as
session.Value, with Common and Inherited as *session.Value. DirectSystemResult
contains Provenance and Records []DirectSystemGrant. Its five paths are
DirectSystemStatement, DecodeDirectSystem, ReadDirectSystem, DirectSystemRows and
DirectSystemJSON, with the same parameter/result conventions as the quota family.

DBA_SYS_PRIVS is queried independently, with no quota or user-table join. Grantees
may be users or roles. No recursive role expansion, effective-privilege assessment,
privilege-name allowlist or quota comparison is computed. Names selects exact
bound grantees; Owners is invalid. A user with no quota rows can still have a
visible UNLIMITED TABLESPACE grant. Oracle documents that this privilege overrides
explicit quotas and is grantable only to users; callers must combine complete,
fresh, compatible source observations before assessing an entitlement.

The core GRANTEE/PRIVILEGE/ADMIN_OPTION projection applies from the admitted 11g
floor. COMMON is selected from 12. INHERITED was introduced in 12.2.0.1, but a
major-only 12 admission cannot prove that subrelease, so this API conservatively
omits it throughout 12 and selects it on known 18/19/21/23. Nil means unprojected;
a nonnil pointer with Null means selected SQL NULL. Raw flag strings, row order
and duplicates survive. These container-inheritance flags are not role expansion.

## Ownership, failures and completion

Read executes once through the caller's complete, bounded Reader, using a frozen
selection/schema/provenance. Any query, malformed late row, decoding, capacity or
post-query/decode cancellation error returns no successful Records prefix.
Successful empty selections have an empty Records slice; they do not establish
user existence or an effective grant. Reader errors, including sql.ErrNoRows,
remain errors instead of being recast as confirmed absence.

Rows returns the actual caller-native cursor. The caller owns Next/Scan/Err/Close,
context/deadlines, affinity and stream budgets. JSON consumes its own cursor via
go-oracle-session/rowset, validates all rows and publishes no bytes on any failure,
including close/terminal/cancellation errors or final envelope overflow. Quota
numeric JSON scalars remain text; no float or bounded native integer conversion.
No source fallback, credential/driver ownership, container switch, capability
cache, grant/schema mutation or new generic execution mechanism is introduced.

## Per-query release matrix

References were read on 2026-11-18 in America/Sao_Paulo (UTC 2026-11-19). D and S mean
documentation and static selection/decoding checks respectively; neither is native
qualification. Both facts reject 9/10 and unknown/future numeric majors before SQL,
preserving the existing 11g floor without inferring numeric 26 from 26ai branding.
The same row applies independently to the two queries with the projections shown.

| Release | Quota projection | Direct system projection | Documentation / static | Native SQL / Go driver |
| --- | --- | --- | --- | --- |
| 9.2 | Unsupported, no SQL | Unsupported, no SQL | Current API floor; refusal checked | Unqualified / unqualified |
| 10.1 | Unsupported, no SQL | Unsupported, no SQL | Current API floor; refusal checked | Unqualified / unqualified |
| 10.2 | Unsupported, no SQL | Unsupported, no SQL | Current API floor; refusal checked | Unqualified / unqualified |
| 11.1 | Five quota columns including DROPPED | Three core columns | D / S | Unqualified / unqualified |
| 11.2 | Five quota columns including DROPPED | Three core columns | D / S | Unqualified / unqualified |
| 12.1 | Five quota columns | Core + COMMON | D / S | Unqualified / unqualified |
| 12.2 | Five quota columns | Core + COMMON; INHERITED intentionally omitted | D / S; 12.2.0.1 boundary recorded | Unqualified / unqualified |
| 18 | Five quota columns | Core + COMMON + INHERITED | D / S | Unqualified / unqualified |
| 19 | Five quota columns | Core + COMMON + INHERITED | D / S | Exact 19.32 case below / unqualified |
| 21 | Five quota columns | Core + COMMON + INHERITED | D / S | Unqualified / unqualified |
| 23/23ai | Five quota columns | Core + COMMON + INHERITED | D / S | Observed 23.26.1 case only; other RUs unqualified / unqualified |
| 26ai, actual numeric 23.26 | Five quota columns | Core + COMMON + INHERITED | D / S | Exact 23.26.1 case below / unqualified |

The production selectors at 9900be0 executed through unmodified SQLcl 26.3.1/JDBC
with Oracle JDK 25.0.5 against existing SYSTEM/current-schema SYSTEM PDB CON_ID 3,
PRIMARY READ WRITE profiles. Fresh source identity, roles and NLS were retained.
On 19.32.0.0.0 the two sources returned 14 quota and 961 system-grant rows; on
23.26.1.0.0 they returned 16 quota and 986 system-grant rows. Separate counts matched
all four client row counts; this is not a promised atomic multi-query snapshot.
No credentials were copied and no accounts, grants, data, schema, container or
lab state were changed. No physical engineered-system hardware is needed.

These four SELECT results qualify their exact SQL/source shapes only.
**No new Go SDK reader/Oracle-driver/application path is natively qualified.**
Legacy/root/non-CDB/standby/MOUNT/other-role/RAC cases remain unperformed. Private
receipts retain exact SDK/SQL/source/result hashes, release/account/scope and the
24 independent query/release records, with static and native gaps kept separate.

## Oracle references

- [DBA_TS_QUOTAS 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TS_QUOTAS.html), corresponding 12.2/18/21/23/26 pages, and legacy [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_5062.htm), [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5068.htm), [12.1](https://docs.oracle.com/database/121/REFRN/GUID-484ABD92-CCEC-4550-9621-7B2BF301328C.htm).
- [DBA_SYS_PRIVS 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_SYS_PRIVS.html), corresponding 18/21/23/26 pages and [12.2](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/DBA_SYS_PRIVS.html), [11.1](https://docs.oracle.com/cd/B28359_01/server.111/b28320/statviews_5036.htm), [11.2](https://docs.oracle.com/cd/E11882_01/server.112/e40402/statviews_5041.htm), [12.1](https://docs.oracle.com/database/121/REFRN/GUID-384E578D-6506-4419-B943-612A7CF3465F.htm).
- [Creating User Accounts 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/dbseg/creating-user-accounts.html), quota/UNLIMITED TABLESPACE semantics and user-only grant constraint.
