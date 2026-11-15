package users

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestUsersReleaseNullAndSensitiveProjection(t *testing.T) {
	base := session.Result{Columns: []string{"USERNAME", "USER_ID", "ACCOUNT_STATUS", "LOCK_DATE", "EXPIRY_DATE", "DEFAULT_TABLESPACE", "TEMPORARY_TABLESPACE", "CREATED", "PROFILE", "INITIAL_RSRC_CONSUMER_GROUP", "AUTHENTICATION_TYPE", "EDITIONS_ENABLED"}, Rows: [][]session.Value{{{Text: "MiXeD"}, {Text: "42"}, {Text: "OPEN"}, {Null: true}, {Null: true}, {Text: "DATA"}, {Text: "TEMP"}, {Text: "2001-02-03T04:05:06"}, {Text: "DEFAULT"}, {Null: true}, {Text: "PASSWORD"}, {Text: "N"}}}}
	for _, major := range []int{11, 12, 19, 26} {
		result := base
		result.Columns = append([]string(nil), base.Columns...)
		result.Rows = [][]session.Value{append([]session.Value(nil), base.Rows[0]...)}
		if major >= 12 {
			result.Columns = append(result.Columns, "COMMON", "ORACLE_MAINTAINED")
			result.Rows[0] = append(result.Rows[0], session.Value{Text: "NO"}, session.Value{Text: "Y"})
		}
		if major >= 18 {
			result.Columns = append(result.Columns, "INHERITED")
			result.Rows[0] = append(result.Rows[0], session.Value{Null: true})
		}
		options := Options{OracleMajor: major, Names: []string{"MiXeD"}}
		got, err := Decode(result, options)
		if err != nil || len(got.Records) != 1 {
			t.Fatal(got, err)
		}
		u := got.Records[0]
		if u.Username.Text != "MiXeD" || u.UserID.Text != "42" || !u.LockDate.Null || u.AuthenticationType.Text != "PASSWORD" || u.Created.Text != "2001-02-03T04:05:06" {
			t.Fatal(u)
		}
		if (u.Common != nil) != (major >= 12) || (u.Inherited != nil) != (major >= 18) || (u.Inherited != nil && !u.Inherited.Null) {
			t.Fatal("absence/null lost", u)
		}
		sql, args, err := Statement(options)
		if err != nil || len(args) != 1 || args[0] != "MiXeD" || strings.Contains(sql, "MiXeD") || strings.Contains(sql, "PASSWORD") || strings.Contains(sql, "EXTERNAL_NAME") {
			t.Fatal(sql, args, err)
		}
		result.Rows[0][0].Text = "mixed"
		if _, err = Decode(result, options); !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal("scope case mismatch accepted", err)
		}
	}
	if _, _, err := Statement(Options{OracleMajor: 10}); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}
func TestFullTableInventoryPreservesExcludedCategoriesAndNullStatistics(t *testing.T) {
	columns := []string{"OWNER", "TABLE_NAME", "TABLESPACE_NAME", "STATUS", "TEMPORARY", "NESTED", "SECONDARY", "IOT_TYPE", "IOT_NAME", "PARTITIONED", "DURATION", "NUM_ROWS", "BLOCKS", "EMPTY_BLOCKS", "AVG_ROW_LEN", "SAMPLE_SIZE", "LAST_ANALYZED", "GLOBAL_STATS", "USER_STATS", "DEGREE", "INSTANCES"}
	result := session.Result{Columns: columns}
	for i, name := range []string{"TEMP_TABLE", "NESTED_TABLE", "SECONDARY_TABLE", "OVERFLOW_TABLE"} {
		row := make([]session.Value, len(columns))
		for j := range row {
			row[j] = session.Value{Null: true}
		}
		row[0] = session.Value{Text: "APP"}
		row[1] = session.Value{Text: name}
		row[4] = session.Value{Text: "N"}
		row[5] = session.Value{Text: "NO"}
		row[6] = session.Value{Text: "N"}
		switch i {
		case 0:
			row[4].Text = "Y"
		case 1:
			row[5].Text = "YES"
		case 2:
			row[6].Text = "Y"
		case 3:
			row[7] = session.Value{Text: "IOT_OVERFLOW"}
			row[11] = session.Value{Text: "9999999999999999999999999999"}
			row[16] = session.Value{Text: "2000-01-01T01:02:03"}
		}
		result.Rows = append(result.Rows, row)
	}
	options := Options{OracleMajor: 19, Owners: []string{"APP"}}
	got, err := DecodeTableMetadata(result, options)
	if err != nil || len(got.Records) != 4 || !got.Records[0].NumRows.Null || !got.Records[0].LastAnalyzed.Null || got.Records[1].Nested.Text != "YES" || got.Records[2].Secondary.Text != "Y" || got.Records[3].IOTType.Text != "IOT_OVERFLOW" || got.Records[3].NumRows.Text != "9999999999999999999999999999" {
		t.Fatal(got, err)
	}
	sql, args, err := TableMetadataStatement(options)
	if err != nil || len(args) != 1 || args[0] != "APP" || strings.Contains(sql, "COUNT(") || strings.Contains(sql, "TEMPORARY='") || strings.Contains(sql, "IOT_OVERFLOW") {
		t.Fatal(sql, args, err)
	}
	result.Rows[3][11].Text = "3.14"
	if bad, err := DecodeTableMetadata(result, options); !errors.Is(err, ErrInvalidEvidence) || bad.Records != nil {
		t.Fatal(bad, err)
	}
}
func TestTotalAccountCountIndependentAndStrict(t *testing.T) {
	options := Options{OracleMajor: 11}
	result := session.Result{Columns: []string{"ACCOUNT_COUNT"}, Rows: [][]session.Value{{{Text: "0"}}}}
	got, err := DecodeAccountCount(result, options)
	if err != nil || len(got.Records) != 1 || got.Records[0].Count.Text != "0" {
		t.Fatal(got, err)
	}
	for _, bad := range []string{"-1", "3.1", ""} {
		result.Rows[0][0].Text = bad
		if _, err = DecodeAccountCount(result, options); !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal(bad, err)
		}
	}
	result.Rows = nil
	if _, err = DecodeAccountCount(result, options); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal(err)
	}
	if _, _, err = AccountCountStatement(Options{OracleMajor: 19, Names: []string{"APP"}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("filtered total accepted", err)
	}
	db, _ := testsql.New(t, testsql.Step{Columns: []string{"ACCOUNT_COUNT"}, Values: [][]driver.Value{}})
	payload, err := AccountCountJSON(context.Background(), db, options, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || payload != nil {
		t.Fatal(payload, err)
	}
}
