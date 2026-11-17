package oracleinfo_test

import (
	"github.com/marcuoli/go-oracle-info/dblinks"
	"github.com/marcuoli/go-oracle-info/triggers"
	"github.com/marcuoli/go-oracle-info/users"
	"strconv"
	"strings"
	"testing"
)

func TestDictionaryDateSelectorsUseGregorianComponents(t *testing.T) {
	for _, major := range []int{11, 19, 23} {
		t.Run(strconv.Itoa(major), func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				expressions []string
				statement   func() (string, []any, error)
			}{
				{"users", []string{"t.LOCK_DATE", "t.EXPIRY_DATE", "t.CREATED"}, func() (string, []any, error) { return users.Statement(users.Options{OracleMajor: major}) }},
				{"table metadata", []string{"t.LAST_ANALYZED"}, func() (string, []any, error) { return users.TableMetadataStatement(users.Options{OracleMajor: major}) }},
				{"database links", []string{"t.CREATED"}, func() (string, []any, error) { return dblinks.Statement(dblinks.Options{OracleMajor: major}) }},
				{"triggers", []string{"o.LAST_DDL_TIME"}, func() (string, []any, error) { return triggers.Statement(triggers.Options{OracleMajor: major}) }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					sql, _, err := tc.statement()
					if err != nil {
						t.Fatal(err)
					}
					for _, expr := range tc.expressions {
						if !strings.Contains(sql, "CASE WHEN "+expr+" IS NULL THEN NULL ELSE ") {
							t.Fatalf("DATE NULL is not preserved independently of formatting: %s", expr)
						}
						for _, field := range []string{"YEAR", "MONTH", "DAY", "HOUR", "MINUTE", "SECOND"} {
							if !strings.Contains(sql, "EXTRACT("+field+" FROM CAST("+expr+" AS TIMESTAMP))") {
								t.Fatalf("%s depends on session calendar or loses %s", expr, field)
							}
						}
					}
					if !strings.Contains(sql, "'FM0000'") || !strings.Contains(sql, "'FM00'") || !strings.Contains(sql, "NLS_NUMERIC_CHARACTERS=''.,''") {
						t.Fatal("Gregorian components lost zero padding or exact numeric locale")
					}
					if strings.Contains(sql, "ALTER SESSION") || strings.Contains(sql, "AT TIME ZONE") {
						t.Fatal("DATE renderer changed caller state or invented a zone")
					}
				})
			}
		})
	}
}
