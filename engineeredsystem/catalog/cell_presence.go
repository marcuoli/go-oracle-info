package catalog

// VisibleCellCountSQL counts cell rows across the admitted visible reporting
// instances. Positive rows identify database-visible Exadata cell infrastructure;
// zero means no cells visible in this source, not proof of physical hardware
// family, per-file placement or complete RAC membership. A failed query is unknown.
const VisibleCellCountSQL = "SELECT COUNT(*) AS VALUE FROM GV$CELL"
