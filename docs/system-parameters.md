# Instance-wide parameters and qualified instance visibility

Use `parameters.ReadSystem` for scalar-row instance-wide facts from
`GV$SYSTEM_PARAMETER`. Its `Request` takes `OracleMajor`, `Filter` and an
application-owned `Reference`, just like the existing buffered parameter readers.
`SystemStatement`/`DecodeSystem` support collectors that already execute selected
SQL; `SystemRows`/`SystemJSON` accept an admitted native query handle. The root
`SystemParameters`/`SystemParametersJSON` facade obtains the version internally.
No method opens a connection or changes a container, destination or parameter.

The projection is the existing effective parameter projection **without
ORDINAL**: INST_ID, CON_ID(12+), NUM, NAME, TYPE, VALUE, DISPLAY_VALUE, ISDEFAULT,
ISSES_MODIFIABLE, ISSYS_MODIFIABLE, ISPDB_MODIFIABLE(12+), ISINSTANCE_MODIFIABLE,
ISMODIFIED, ISADJUSTED, ISDEPRECATED, ISBASIC, DESCRIPTION, UPDATE_COMMENT.
`SystemParameter` has no invented ordinal. Its `Value` preserves the complete
source text, including commas, quoting, whitespace and units. Numeric metadata
retains exact signed integer text in typed results. Flags and SQL NULL remain raw;
nil optional pointers mean absent 11g columns. No default value is inferred.

`ReadEffective`/`InitParameters` and `ReadHidden` continue using the published
`GV$SYSTEM_PARAMETER2` list-element projection with real ordinals. Stored SPFILE
clauses and legacy session `V$PARAMETER` selectors are also unchanged. Choose the
source required by the fact; do not silently substitute one for another.

`Filter.Instances` selects positive reporting IDs through binds. Empty means all
qualified visible IDs, with no connected-instance restriction. Names retain the
existing exact binary matching/filter rules. On 12+, the SQL retains whole-database
CON_ID=0 and current-container rows, not every visible PDB. All ordinary and hidden
names visible in the selected source survive unless explicitly name-filtered.

## Select the raw modification flag

`parameters.ModifiedSystem(facts)` copies rows whose non-NULL `IsModified.Text`
is exactly `MODIFIED`. NULL, unknown flags and other values stay in the original
facts. The helper preserves source/instance/container/default/NULL evidence and
copies optional values and provenance slices. A nil input Rows stays nil;
a complete nonnil empty input stays complete empty.

The flag records an ALTER SYSTEM modification. It does not distinguish
SCOPE=MEMORY from SCOPE=BOTH or prove non-persistence. No SPFILE comparison,
unit normalization, SID precedence resolution or PFILE classification runs.
The application chooses the report title and presentation.

## Enumerate visibility before reading members

`instances.Read` accepts an already-admitted complete reader and
`instances.Request{OracleMajor: major, Reference: ref}`. `Statement`/`Decode`,
borrowed native `Rows` and bounded `JSON` are also available. The root facade
provides `InstanceVisibility` and `InstanceVisibilityJSON`.

The projection is INST_ID, CON_ID(12+), INSTANCE_NUMBER, INSTANCE_NAME, HOST_NAME,
STATUS, DATABASE_STATUS, PARALLEL, BLOCKED from `GV$INSTANCE`, ordered by reporting
ID, container when present, and instance number. Mounted/blocked/unknown statuses
are retained. No connected-instance or open-PDB-only predicate narrows membership.
The 12+ container scope is CON_ID=0 plus the current container. Decode validates
positive non-NULL INST_ID, exact numeric metadata, full row shape and valid UTF8;
raw nullable fields and duplicate observations survive.

`Result.State` distinguishes `available`, successful `no_rows` and `unavailable`.
Any read/terminal/close/budget/cancellation/malformed-row error must discard that
source's prefix. Complete empty is not a privilege error. Qualified visible rows
are not a configured membership inventory or proof of member reachability:
privileges, container visibility, startup state and concurrent changes limit them.
The SDK does not synthesize missing members or mark physical RAC coverage complete.

A bounded consumer can enumerate the admitted visible IDs, retain that baseline,
and read `ReadSystem` independently for each ID. The application owns ID
aggregation, worker scheduling, finite per-source and aggregate budgets, expected
member evidence, missing/unreachable/failed/partial states, retries and report
selection. Keep healthy members when another source fails, and expose the failed
member. A successful empty parameter read is not proof an enumerated member has no
parameters. Do not fall back to the connected instance and label coverage complete.

Separate visibility and parameter queries are dynamic observations, without an
atomic snapshot guarantee. Native rows preserve the exact borrowed *sql.Rows and
caller-owned lifetime; JSON delegates bounded complete consumption to
`go-oracle-session/rowset`, including schema/provenance in the byte limit. Owned
session cursors remain in the session module. JSON errors return nil data.
No CSV/plaintext dataset serializer or generic executor is introduced.

## Oracle source review

Checked 2026-11-14, against the documented release boundaries; this is source
review, not native Oracle/RAC qualification. Consumers record actual driver,
release/RU, platform, grants, container and topology evidence separately.

- [11.2 V$SYSTEM_PARAMETER](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_3097.htm): common scalar-row fields and raw modification flag.
- [19c V$SYSTEM_PARAMETER](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER.html) and [26ai source](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER.html): instance values and modern container/modifiability fields.
- [11.2 V$INSTANCE](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_2002.htm) and [19c source](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html): selected status/identity fields.
- [Dynamic performance views](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/about-dynamic-performance-views.html): qualified global visibility, INST_ID, container controls and consistency limits.
- [ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/ALTER-SYSTEM.html): memory/SPFILE/BOTH semantics are distinct from the raw flag.