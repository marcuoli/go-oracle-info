# Engineered-system statement object diagnostics

Each existing catalog options type provides `StatementObjects(executionErr)`.
Provide the same admitted options, including the unchanged capability schema, and the original execution error from the selected
statement, or nil after success. The returned list describes every direct
relation that the reviewed statement references. Metadata, Cells,
ConnectedCells, VisibleCellCount, Configuration and Alerts currently reference
one relation each. A capability fallback reports the selected local view.
The statement family, version gates and native and JSON result formats
retain their existing contracts.

An explicitly qualified SYS view reports its owner, dictionary name and
kind. Unqualified `V$CELL` and `GV$CELL` retain their SQL spelling and omit
owner and kind because the actual resolved object is connection-dependent.
Reporting uses the declared source identity and supplied error only.

`ProblemCode` is present only when go-oracle-session's diagnostic layer
matches Oracle's exact named local error to a reviewed object. A bare
ORA-00942, unrelated error, privilege rejection without an object name
or remote stack returns the selected objects without attribution. A
named ORA-00942 indicates an Oracle-reported problem with access or existence.
Object existence and the required grants need independent evidence.
The original error remains available to the caller. Catalog diagnostics
reuse the existing statement compatibility and source qualification.
