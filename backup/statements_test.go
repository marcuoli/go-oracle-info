package backup

import (
	"strconv"
	"strings"
	"testing"
)

func TestJobLimitsDoNotDefaultInvalidRequests(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 10, 10000, 10001} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			text, err := JobsSQL(n)
			if n < 1 || n > 10000 {
				if err == nil || text != "" {
					t.Fatal("invalid limit returned executable SQL")
				}
				return
			}
			if err != nil || !strings.HasSuffix(text, "ROWNUM<="+strconv.Itoa(n)) || !strings.Contains(text, "END_TIME DESC NULLS LAST") {
				t.Fatal("backup sample bound/order changed", err)
			}
			if strings.Contains(text, "TZH") {
				t.Fatal("DATE source acquired an invented timezone")
			}
		})
	}
}
