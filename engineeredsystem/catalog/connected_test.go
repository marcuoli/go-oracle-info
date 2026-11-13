package catalog

import (
	"strconv"
	"strings"
	"testing"
)

func TestConnectedCellShapeAndSentinelAreExplicit(t *testing.T) {
	for _, includeType := range []bool{false, true} {
		t.Run(strconv.FormatBool(includeType), func(t *testing.T) {
			selection, err := ConnectedCells(1001, includeType)
			if err != nil || !strings.HasSuffix(selection.Text, "ROWNUM<=1001") || strings.Contains(selection.Text, "GV$") || selection.View != "V$CELL" {
				t.Fatal("scope or sentinel changed", selection, err)
			}
			if strings.Contains(selection.Text, "CELL_TYPE") != includeType || !strings.Contains(selection.Original, "{{.Limit}}") {
				t.Fatal("legacy capability or source template was hidden")
			}
		})
	}
	for _, n := range []int{-1, 0, 10001} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			selection, err := ConnectedCells(n, true)
			if err == nil || selection.Text != "" {
				t.Fatal("invalid bound returned executable SQL")
			}
		})
	}
}
