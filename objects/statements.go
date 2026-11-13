// Package objects selects dictionary object inventories and exact table counts.
// Callers retain schema authorization, collection budgets and comparison policy.
package objects

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// InvalidStatement reads invalid objects in the bound owner list. An empty list
// means all dictionary-visible owners; it does not infer application schemas.
func InvalidStatement(owners ...string) (string, []any) {
	filter, args := ownerFilter(owners)
	if filter == "" {
		filter = " WHERE STATUS='INVALID'"
	} else {
		filter += " AND STATUS='INVALID'"
	}
	return "SELECT OWNER, OBJECT_TYPE, OBJECT_NAME, STATUS FROM DBA_OBJECTS" + filter + " ORDER BY OWNER,OBJECT_TYPE,OBJECT_NAME", args
}

// InventoryStatement groups object counts by owner, type and status. Counts are
// exact decimal text; empty sources remain empty rather than synthesized zeros.
func InventoryStatement(owners ...string) (string, []any) {
	filter, args := ownerFilter(owners)
	return "SELECT OWNER, OBJECT_TYPE, STATUS, TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OBJECT_COUNT FROM DBA_OBJECTS" + filter + " GROUP BY OWNER,OBJECT_TYPE,STATUS ORDER BY OWNER,OBJECT_TYPE,STATUS", args
}

func ownerFilter(owners []string) (string, []any) {
	if len(owners) == 0 {
		return "", []any{}
	}
	placeholders, args := make([]string, len(owners)), make([]any, len(owners))
	for i, owner := range owners {
		placeholders[i], args[i] = fmt.Sprintf(":%d", i+1), owner
	}
	return " WHERE OWNER IN (" + strings.Join(placeholders, ",") + ")", args
}

// QuoteIdentifier quotes one dictionary identifier without changing its case.
// It rejects empty, invalid UTF-8, NUL and names exceeding 128 bytes. This is SQL
// escaping, not authorization to access the named object.
func QuoteIdentifier(value string) (string, error) {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", errors.New("invalid catalog identifier")
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`, nil
}

// CountStatement returns an exact decimal COUNT(*) for one safely quoted table.
// Callers must first admit the object as an ordinary table in the intended scope;
// this function cannot establish cost, cutoff consistency or access permission.
func CountStatement(owner, table string) (string, error) {
	o, err := QuoteIdentifier(owner)
	if err != nil {
		return "", err
	}
	t, err := QuoteIdentifier(table)
	if err != nil {
		return "", err
	}
	return "SELECT TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ROW_COUNT FROM " + o + "." + t, nil
}

// SchemaSQL looks up the exact bound schema (:1).
const SchemaSQL = `SELECT USERNAME FROM DBA_USERS WHERE USERNAME=:1`

// TotalsSQL counts each object status class for the bound owner (:1).
const TotalsSQL = `SELECT OBJECT_TYPE, TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS TOTAL, TO_CHAR(SUM(CASE WHEN STATUS='VALID' THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALID_COUNT, TO_CHAR(SUM(CASE WHEN STATUS='INVALID' THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS INVALID_COUNT, TO_CHAR(SUM(CASE WHEN STATUS NOT IN ('VALID','INVALID') OR STATUS IS NULL THEN 1 ELSE 0 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OTHER_COUNT FROM DBA_OBJECTS WHERE OWNER=:1 GROUP BY OBJECT_TYPE ORDER BY OBJECT_TYPE`

// TablesSQL lists ordinary visible tables for the bound owner (:1), excluding temporary, nested, secondary and IOT-overflow entries.
const TablesSQL = `SELECT TABLE_NAME FROM DBA_TABLES WHERE OWNER=:1 AND TEMPORARY='N' AND NESTED='NO' AND SECONDARY='N' AND NVL(IOT_TYPE,'HEAP')<>'IOT_OVERFLOW' ORDER BY TABLE_NAME`

// TableSQL checks the bound owner (:1) and table (:2) before an explicit count.
const TableSQL = `SELECT TABLE_NAME FROM DBA_TABLES WHERE OWNER=:1 AND TABLE_NAME=:2 AND TEMPORARY='N'`
