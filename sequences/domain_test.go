package sequences

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestSequenceCachePrecisionVersionAndScope(t *testing.T) {
	base := session.Result{Columns: []string{"SEQUENCE_OWNER", "SEQUENCE_NAME", "MIN_VALUE", "MAX_VALUE", "INCREMENT_BY", "CYCLE_FLAG", "ORDER_FLAG", "CACHE_SIZE", "LAST_NUMBER"}, Rows: [][]session.Value{{{Text: "APP"}, {Text: "SEQ"}, {Text: "-999999999999999999999999999"}, {Text: "9999999999999999999999999999"}, {Text: "-5"}, {Text: "Y"}, {Text: "N"}, {Text: "100"}, {Text: "1234567890123456789012345678"}}}}
	for _, major := range []int{11, 12, 18, 19, 26} {
		result := base
		result.Columns = append([]string(nil), base.Columns...)
		result.Rows = [][]session.Value{append([]session.Value(nil), base.Rows[0]...)}
		if major >= 12 {
			result.Columns = append(result.Columns, "SESSION_FLAG", "KEEP_VALUE")
			result.Rows[0] = append(result.Rows[0], session.Value{Text: "Y"}, session.Value{Null: true})
		}
		if major >= 18 {
			result.Columns = append(result.Columns, "SCALE_FLAG", "EXTEND_FLAG")
			result.Rows[0] = append(result.Rows[0], session.Value{Text: "N"}, session.Value{Null: true})
		}
		if major >= 19 {
			result.Columns = append(result.Columns, "SHARDED_FLAG")
			result.Rows[0] = append(result.Rows[0], session.Value{Text: "future"})
		}
		o := Options{OracleMajor: major, Owners: []string{"APP"}, Names: []string{"SEQ"}}
		got, err := Decode(result, o)
		if err != nil || got.Records[0].LastNumber.Text != "1234567890123456789012345678" || got.Records[0].IncrementBy.Text != "-5" || got.Records[0].CacheSize.Text != "100" {
			t.Fatal(got, err)
		}
		if (got.Records[0].SessionFlag != nil) != (major >= 12) || (got.Records[0].ShardedFlag != nil) != (major >= 19) {
			t.Fatal(got)
		}
		sql, args, err := Statement(o)
		if err != nil || len(args) != 2 || strings.Contains(sql, "NEXTVAL") || strings.Contains(sql, "CURRVAL") || strings.Contains(sql, "WHERE OWNER") {
			t.Fatal(sql, args, err)
		}
		result.Rows[0][0].Text = "OTHER"
		if _, err = Decode(result, o); !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal(err)
		}
	}
}
