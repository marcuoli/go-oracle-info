# Native fact client and parameter datasets

Use a caller-admitted native database/sql handle:

```go
import info "github.com/marcuoli/go-oracle-info"

facts := info.New(existingConn)
version, err := facts.Version(ctx)
if err != nil {
    return err
}
// Check version.State and the precision required by the application.

data, err := facts.InitParametersJSON(ctx, info.ParameterFilter{
    Names: []string{"processes", "control_files"},
    Instances: []int64{1, 2},
})
```

New stores the handle; it opens, closes and configures nothing. Prefer an
already-admitted session-affine sql.Conn or sql.Tx. A pool/custom Queryer must
provide caller-proven compatible targets and supported cursor concurrency.
The facade adds no retry or recovery and holds no lease/capability cache.
The supplied handle retains its native database/sql/driver behavior. Generic execution, native-row consumption and bounded JSON are delegated directly to go-oracle-session/rowset; this module owns Oracle fact selection, filters, schema and provenance.

Version, Architecture and Edition reuse the existing independent identity
observations, with actual sources, availability, precision, fallback and safe
error causes. Every parameter call independently reads the version through those
contracts before selecting its source. On18+ that reads VERSION then VERSION_FULL.
Applications supply neither major versions, SQL nor reference wiring. Failure
stops the sequence, including a failed modern version with a retained legacy
value. Successful legacy precision can establish a release capability without
claiming an RU. External session changes or different pool members can invalidate
cross-call consistency; no snapshot or session pinning is invented.

## Native rows versus JSON

InitParameters and SPFileParameters return the exact *sql.Rows obtained from
the caller's QueryContext. No cursor is synthesized, wrapped or converted from
session.Stream, and no internal defer-cancel invalidates it on return.
The caller must iterate Next/Scan, inspect Err, close on every path and check
Close errors, and supply deadlines/stream budgets. Close the admitted handle in
its owner. Keep its cursor concurrency constraints; these APIs do not serialize
an escaping cursor with later calls.

The JSON companions accept zero/one JSONLimits value. Defaults are10000 rows
and8MiB; maximums1million rows/64MiB. There is no unbounded JSON mode.
Each JSON call executes the same selected SQL/filter as the native counterpart
on a separate cursor. It consumes and finalizes that cursor itself. No SQL row
cap conceals a truncated prefix. Oversized data returns an error and nil JSON.
Query/metadata/Scan/value/terminal/Close/cancellation/encoding failures likewise
return nil JSON, never a partial-success dataset.

JSON uses encoding/json with an explicit small ordered contract:

```json
{
  "provenance": {
    "source": "GV$SPPARAMETER",
    "oracle_major": 23,
    "scope": "current_container_and_whole_database_rows",
    "instances": [1],
    "sids": ["instance_a", "*"],
    "sid_policy": "selected_sids_plus_wildcard"
  },
  "data": {
    "columns": [
      {"name": "INST_ID", "kind": "integer"},
      {"name": "CON_ID", "kind": "integer"}
    ],
    "rows": []
  }
}
```

The abbreviated columns above illustrate the envelope, not the full projection.
Column names/casing, column and row positions, repeated rows and multivalue
ORDINAL remain intact. SQL NULL is null, text remains exact text (including
whitespace/commas/unknown flags), and IDs/ordinals are exact JSON integers.
System TYPE is an integer; SPFILE TYPE is text because Oracle declares it
VARCHAR2(11). Declared integer fields use exact signed64-bit values; overflow fails. No float-to-exact-number conversion is accepted. Invalid UTF8
fails instead of receiving replacement characters. Names/IDs/SIDs are filters,
not implicit policy or inferred defaults.

The byte limit includes actual standard-JSON escaping, column schema and
provenance. No CSV or plaintext result serialization is provided. Only genuine
DBMS_OUTPUT may be textual output; typed scalar/string values and SQL source
strings are not formatted resultsets. Keep diagnostics separate from result data.

Errors have static source/operation messages and preserve available causes via
errors.Is/As/Unwrap for deliberate private diagnostics. database/sql may hide an
independent driver Close error when Next already failed; the operation still
fails and publishes no JSON. The SDK does not install a tracking driver to
recover hidden causes. A driver ignoring cancellation/Close still needs the
caller's transport disposal/cleanup policy; JSON limits do not replace that.

## Effective initialization parameters

InitParameters reads GV$SYSTEM_PARAMETER2, the instance-effective source with
one row per list element. It is not current-session V$PARAMETER and not stored
SPFILE configuration. Existing database.ParametersSQL/ParametersWithDefaultsSQL
remain unchanged. The former go-oracle-info/rowset package is removed; direct generic dataset users import go-oracle-session/rowset.

ParameterFilter.Names and Instances are optional. Empty lists mean all names
and reporting instance IDs visible in that admitted view. Names are exact
case-preserving matches (Oracle normally exposes lowercase names); the API
does not trim/uppercase returned data. Filters are bound values; binary
comparisons prevent linguistic settings broadening character admission.
IDs must be positive. Up to1000 values fit each Oracle IN list. Names/SIDs
accept up to80 UTF8 bytes, without empty, control or padded values.

Effective column order:
INST_ID, CON_ID(12+), NUM, NAME, TYPE, ORDINAL, VALUE, DISPLAY_VALUE,
ISDEFAULT, ISSES_MODIFIABLE, ISSYS_MODIFIABLE, ISPDB_MODIFIABLE(12+),
ISINSTANCE_MODIFIABLE, ISMODIFIED, ISADJUSTED, ISDEPRECATED, ISBASIC,
DESCRIPTION, UPDATE_COMMENT.

INST_ID, CON_ID, NUM, TYPE and ORDINAL are integers; other columns are text.
On11g the unsupported CON_ID and ISPDB_MODIFIABLE columns are omitted.

## Stored SPFILE clauses

SPFileParameters reads GV$SPPARAMETER. Its Instances filter is the reporting
INST_ID, not the SID to which a stored setting applies. SIDs is a separate filter.
With no SIDs filter every visible stored clause survives, including dormant SIDs
that have no running instance and wildcard defaults. Named SIDs automatically
retain '*' alongside overrides. The1000-value budget includes that wildcard.
No join to GV$INSTANCE, ISSPECIFIED=TRUE filter, precedence merge, effective-value
resolution or deduplication removes independent clauses.

Stored column order:
INST_ID, CON_ID(12+), SID, NAME, TYPE, ORDINAL, VALUE, DISPLAY_VALUE,
ISSPECIFIED, UPDATE_COMMENT.

Only INST_ID, CON_ID and ORDINAL are integers. TYPE and flags remain text.
If no SPFILE was used, Oracle documents FALSE ISSPECIFIED, NULL VALUE and
ORDINAL0; preserve those observations. Empty/NULL/unspecified rows do not
establish an empty valid persisted configuration. This view is not proof of
complete physical file contents, hidden parameters or SID applicability.

## Container and coverage boundaries

On12+, both statements explicitly select CON_ID0 whole-database rows plus the
current SYS_CONTEXT CON_ID, retaining CON_ID in output. Other PDB rows that
root CONTAINER_DATA could expose are excluded. This policy does not synthesize
inheritance or claim the selected whole-database values are effective PDB values.
11g uses the non-CDB schema. No container switch/discovery, credentials, grants,
file access or hidden X$ parameter read occurs.

All visible reporting instances is not proven RAC membership/completeness.
Views are dynamic and not read-consistent snapshots; separate rows/JSON calls
can see different observations. Source names, version selection and filters
describe selected view rows only, never cross-container or SPFILE-file coverage.
Native grants/open-state/driver/platform qualification must name the actual
observed configuration; fixtures alone do not qualify native transport.

## Functional extension pattern

Advanced consumers can use parameters.EffectiveRows/EffectiveJSON and
SPFileRows/SPFileJSON with an already-known major, or inspect the corresponding
Statement builders. New fact families should add small domain functions and
facade methods, reuse the native Queryer/ordered rowset contract and standard JSON,
and retain their own source semantics. Do not introduce a query registry/ORM or
convert existing consumers wholesale.

The released go-oracle-session v0.6.0 requires Go 1.27.4. Use its rowset package directly for generic borrowed-native queries and
ordered bounded datasets. Its helpers own only consumption/finalization of rows
already supplied by the caller. The owned Session.Stream API retains its private
cursor and lease lifecycle; no Session cursor is cast to sql.Rows or detached
from its lease. No second transport or cancellation watcher is introduced here.
There is no go-oracle-info/rowset compatibility package. The facade's Queryer and
JSONLimits conveniences refer directly to the session module's functional types.

## Oracle references

Checked2026-11-14:
- Effective/current-session distinction and multivalue ordinals:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER2.html
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PARAMETER.html
- Stored SID/TYPE/NULL/ISSPECIFIED/ORDINAL:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SPPARAMETER.html
- Global reporting INST_ID, CONTAINER_DATA and dynamic read consistency:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/about-dynamic-performance-views.html
- Legacy column boundaries:
  https://docs.oracle.com/cd/E11882_01/server.112/e40402/dynviews_3098.htm
  https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_3042.htm
- Current source comparison:
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER2.html
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SPPARAMETER.html
- SPFILE wildcard/override context (no merge performed here):
  https://docs.oracle.com/en/database/oracle/oracle-database/19/admin/creating-and-configuring-an-oracle-database.html

## Visible hidden parameters and buffered readers

`HiddenStatement`, `HiddenRows` and `HiddenJSON` restrict the same effective
projection to literal leading-underscore names. The root facade exposes
`HiddenParameters` and `HiddenParametersJSON`. `Provenance.NameClass` records
`leading_underscore_only`. This is a visible/configured view subset, including
raw/default-state facts, not a complete unset internal catalog. No X$ table,
setting change or fallback is used. Explicit names intersect that prefix and
cannot admit ordinary names. A missing name remains absent/unavailable.

The application selects non-default plus explicit rows, retaining a peer default
when needed for comparison. This policy does not enter the SQL or decoder. Align
actual instance/container/name/ordinal identity; do not collapse list rows or
infer RAC completeness. Existing effective and stored methods are unchanged.

For an existing complete buffered reader, `ReadEffective`, `ReadHidden` and
`ReadSPFile` take a known source version, filter and valid caller reference.
Matching `Decode...` functions validate complete `session.Result` metadata,
exact integer fields and all rows before publishing typed records. Raw fields
are `session.Value`; optional 11g-absent columns use nil pointers, distinct from
projected SQL NULL. A failed read or malformed late row returns no successful
prefix. Reader ownership, finite budgets, terminal errors and transport recovery
remain with the caller. See the [developer guide](developer-guide.md).

Oracle-published hidden-name guidance checked 2026-11-14:

- [Oracle 11.2 Upgrade Guide, Upgrading to the New Release](https://docs.oracle.com/cd/E18283_01/server.112/e17222/upgrade.htm): leading-underscore selection of existing hidden parameters under admitted SYSDBA.
- [Oracle 12.1 Upgrade Guide, Preparing to Upgrade Oracle Database](https://docs.oracle.com/database/121/UPGRD/preup.htm): the same public-view selection for currently set parameters. The former E16655 URL redirects here.

The instance-effective source remains the documented `GV$SYSTEM_PARAMETER2`
contract above; the upgrade examples do not establish all hidden defaults,
current-session equivalence or native cross-instance/container acceptance.