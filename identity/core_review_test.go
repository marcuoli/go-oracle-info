package identity

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func TestDatabaseInfoSCNRemainsUnsignedExactText(t *testing.T) {
	options := DatabaseInfoOptions{OracleMajor: 19}
	for _, value := range []string{"9223372036854775808", "18446744073709551615", "1.5", "-1", "garbage"} {
		t.Run(value, func(t *testing.T) {
			source := databaseInfoFixture(false)
			source.Rows[0][14].Text = value
			valid := value == "9223372036854775808" || value == "18446744073709551615"
			decoded, err := DecodeDatabaseInfo(source, options)
			if valid && (err != nil || len(decoded.Rows) != 1 || decoded.Rows[0].CurrentSCN.Text != value) {
				t.Fatalf("exact SCN rejected: %+v %v", decoded, err)
			}
			if !valid && (decoded.Rows != nil || !errors.Is(err, ErrInvalidEvidence)) {
				t.Fatalf("invalid SCN admitted: %+v %v", decoded, err)
			}
			values := make([]driver.Value, len(source.Columns))
			for i, v := range source.Rows[0] {
				if !v.Null {
					values[i] = v.Text
				}
			}
			db, state := testsql.New(t, testsql.Step{Columns: source.Columns, Values: [][]driver.Value{values}})
			payload, err := DatabaseInfoJSON(t.Context(), db, options, rowset.Limits{})
			if valid {
				if err != nil || !json.Valid(payload) {
					t.Fatalf("exact JSON SCN rejected: %s %v", payload, err)
				}
				var envelope struct {
					Data rowset.Table `json:"data"`
				}
				if err = json.Unmarshal(payload, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Data.Rows[0][14] != value {
					t.Fatal("SCN changed type or precision", envelope.Data.Rows[0][14])
				}
			} else if payload != nil || !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("JSON bypassed SCN validation: %s %v", payload, err)
			}
			if state.Closed() != 1 {
				t.Fatal("cursor not finalized", state.Closed())
			}
		})
	}
}
