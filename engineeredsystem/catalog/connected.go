package catalog

import (
	"errors"
	sqltemplate "github.com/marcuoli/go-sqltemplate/render"
	"strconv"
)

// ConnectedCells selects a bounded V$CELL projection for the admitted connected
// instance, without metadata discovery or switching to GV$ views. Callers may
// omit CELL_TYPE only after establishing that the source lacks that column.
// Reserve a sentinel row in maxRows when truncation must be detected; a full
// returned limit must not be assumed to describe a complete inventory.
func ConnectedCells(maxRows int, includeType bool) (Command, error) {
	if maxRows < 1 || maxRows > 10000 {
		return Command{}, errors.New("row limit must be between 1 and 10000")
	}
	original := connectedCellsSQL
	if !includeType {
		original = connectedCellsLegacySQL
	}
	text, err := sqltemplate.Render("connected-cells", original,
		struct{ Limit sqltemplate.Fragment }{sqltemplate.Fragment(strconv.Itoa(maxRows))})
	return Command{ID: "connected-cells", Text: text, View: "V$CELL", Original: original}, err
}

const connectedCellsSQL = `SELECT TO_CHAR(CELL_HASHVAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CELL_HASHVAL, CELL_PATH, CELL_TYPE FROM (SELECT CELL_HASHVAL,CELL_PATH,CELL_TYPE FROM V$CELL ORDER BY CELL_HASHVAL,CELL_PATH) WHERE ROWNUM<={{.Limit}}`
const connectedCellsLegacySQL = `SELECT TO_CHAR(CELL_HASHVAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CELL_HASHVAL, CELL_PATH FROM (SELECT CELL_HASHVAL,CELL_PATH FROM V$CELL ORDER BY CELL_HASHVAL,CELL_PATH) WHERE ROWNUM<={{.Limit}}`
