# Conditional parameter restart persistence observations

`parameters.EvaluatePersistence(ctx, request)` compares existing complete
`EffectiveResult` (GV$SYSTEM_PARAMETER2) and `SPFileResult` (GV$SPPARAMETER).
It reads no database, startup file or host configuration and changes no inputs.
Use the existing raw readers/native rows/JSON APIs for collection. Computed
findings have no synthetic native cursor or connection-owning facade.

## Caller admission

Provide `PersistenceRequest{At, MaxAge, Members}` with an explicit comparison time
and positive maximum observation age. Each `PersistenceMember` supplies reporting
`InstanceID`, observed `SID`, `SIDVerified`, admitted `Scope`, `Startup`, both raw
results and `EffectiveEvidence`/`StoredEvidence`. Evidence fields are
`Complete` and `ObservedAt`; complete means successful EOF/terminal/close handling
and complete selected-member coverage, not merely successful pure decoding.

`PersistenceStartup` supplies `Kind`, raw `SPFilePath`, `Verified` and `ObservedAt`.
A current SPFILE parameter observation can support admission; verification still
belongs to the caller and does not establish future file/operator choices. PFILE,
unverified/empty startup evidence and PDB dictionary persistence/inheritance stay
unknown. No files are opened or settings changed to resolve these cases.

`SIDVerified` must admit the stored SID identity. INSTANCE_NAME defaults to the
SID but is not an unconditional identity proof. With an unverified SID, a
parameter can still use a specified wildcard if the complete unfiltered source
has no potentially specified named override for that parameter. Otherwise its
applicability is unknown. Verified matching SID clauses precede wildcard clauses;
dormant clauses remain in the original raw dataset. Reporting INST_ID is never SID.

Both source provenances must match reviewed source/version/container selection.
Name/SID/hidden-class filtering, incomplete/empty/failed sources, stale or future
observations and contradictory member coverage cannot establish a match or
explicit difference. Each failed member remains represented independently.
Malformed request identity/encoding and context cancellation return no report.
These are dynamic observations without a common snapshot guarantee.

## Findings and comparison boundaries

`PersistenceReport.Members` retains member identity, copied source provenance,
a stable member `Reason` when admission fails, and `Findings`. Each finding has
`Name`, actual nullable/absent `ContainerID`, `State`, stable `Reason`, selected
`ApplicableSID`, and owned raw `Effective`/`Stored` row snapshots. Source inputs
and optional pointers/provenance slices are never mutated or aliased by output.
The caller retains original full datasets, including nonselected/dormant clauses.

| State | Meaning |
| --- | --- |
| `matches` | Explicit applicable stored values compare equal under the admitted rules. |
| `explicit_stored_difference` | Explicit stored literal values differ; replacement is conditional on the same admitted startup configuration remaining applicable. |
| `not_explicitly_stored_default_unresolved` | No applicable explicit definition is established; the restart default is unresolved. This is not confirmed loss. |
| `unknown` | Admission, scope, adjustment, shape, type or value equivalence is unresolved. |

The effective view's documented numeric TYPE selects comparison rules for the
same named/versioned parameter. SPFILE TYPE is retained as opaque VARCHAR2
metadata; no undocumented textual/numeric label mapping is invented. Booleans
accept only ASCII true/false spelling. Integer literals use exact `big.Int` math;
big integers additionally recognize the reviewed K/KB, M/MB and G/GB byte units.
No float rounding, expression evaluation, environment substitution or unsupported
suffix conversion occurs. Other values remain unknown rather than guessed.

String/file values match only byte-for-byte. Different strings, including
case-only/path/quoting differences, need parameter/platform-specific equivalence
and remain unknown. No generic case folding, splitting comma-containing values
or parameter-specific normalization is applied. Ordered list rows retain actual
ordinal identities; mismatched/duplicate ordinals or ambiguous list shapes stay
unknown. Scalar numeric/boolean ordinal numbers are not treated as list identity.
Adjusted or unavailable values cannot establish a persistence conclusion.

For an admitted Oracle 12c-or-later CDB-root comparison only, whole-CDB CON_ID 0 observations and
root CON_ID 1 definitions can apply to the same root instance/name. They share
one logical comparison group while every effective/stored raw CON_ID remains
unchanged; Finding.ContainerID retains the effective row's ID. Multiple effective
0/1 scopes or competing potentially applicable stored 0/1 clauses remain unknown:
no precedence between container scopes is invented. Dormant unrelated verified
SID clauses do not override that member's root evidence. PDB, non-CDB, unverified
and other container scopes gain no mapping. Legacy 11g absent IDs remain absent.
Defaults are
not inferred from missing definitions, ISDEFAULT, ISSPECIFIED or ISMODIFIED.
`ModifiedSystem` keeps its published exact-flag selection unchanged.

## Standard bounded JSON

```go
report, err := parameters.EvaluatePersistence(ctx, request)
// Retain every member and inspect State/Reason before selecting report rows.
payload, err := parameters.PersistenceJSON(ctx, request,
    rowset.Limits{MaxRows: 20000, MaxBytes: 8 << 20})
// Check err before publishing payload. This independently evaluates the inputs.
```

`PersistenceJSON` resolves existing session-rowset finite limits. Input members
and source rows are bounded before allocating snapshots; output members, findings
and retained effective/stored rows are also counted against MaxRows. MaxBytes
includes the full standard JSON report. Invalid limits, context cancellation,
capacity or encoding errors return nil JSON. Diagnostics are separate; no
CSV/plaintext query-result output is introduced.

## Source review and qualification

Oracle-published supporting content checked 2026-11-15:

- [SYSTEM_PARAMETER2](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SYSTEM_PARAMETER2.html): instance/list rows, numeric type codes, adjustment and actual ordinals.
- [SPPARAMETER](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-SPPARAMETER.html): stored SID/ISSPECIFIED/NULL/ordinal fields; TYPE is VARCHAR2 without an enumerated label mapping.
- [ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/ALTER-SYSTEM.html): SID precedence, MEMORY/SPFILE/BOTH, PDB inheritance and internal adjustment; no ALTER operation is executed.
- [Parameter descriptions](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/reading-the-parameter-descriptions.html): Boolean/integer/big-integer categories and documented units.
- [Parameter files](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/parameter-files.html): string/file syntax and platform-sensitive filenames.
- [INSTANCE_NAME](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/INSTANCE_NAME.html): the SID is a default, not an unconditional equality guarantee.
- [SPFILE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/SPFILE.html): name of the current SPFILE; a client PFILE may reference it.

This is conditional comparison of supplied evidence, not all-version/platform,
physical RAC, startup success or future restart certification. Source and tests
remain queued under issue50; independent consumer/native qualification and every
integration/publication/source-cleanup gate remain separate and open.
Root applicability source review, 2026-11-15: [SYSTEM_PARAMETER2 26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SYSTEM_PARAMETER2.html) and [SPPARAMETER 26ai](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-SPPARAMETER.html) retain the same whole-CDB (0) and root (1) meanings as 19c. [ALTER SYSTEM](https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/ALTER-SYSTEM.html) describes root versus whole-CDB parameter scope. The qualified root grouping follows those scopes and the supplied native root observation shapes; it establishes no PDB inheritance or future startup guarantee.
