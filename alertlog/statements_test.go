package alertlog

import (
	"strconv"
	"strings"
	"testing"
)

func TestEntryLimitsKeepWindowBoundAndRejectIncompleteSelection(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 100, 10000, 10001} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			text, err := EntriesSQL(n)
			valid := n >= 1 && n <= 10000
			if !valid {
				if err == nil || text != "" {
					t.Fatal("invalid limit returned executable SQL")
				}
				return
			}
			if err != nil || !strings.HasSuffix(text, "ROWNUM<="+strconv.Itoa(n)) {
				t.Fatal("row bound lost", err)
			}
			if !strings.Contains(text, "NUMTODSINTERVAL(:1,'SECOND')") || !strings.Contains(text, "MESSAGE_TEXT") || !strings.Contains(text, "RECORD_ID DESC") {
				t.Fatal("bound window, complete text or deterministic ordering lost")
			}
		})
	}
}
