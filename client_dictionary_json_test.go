package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/dblinks"
	"github.com/marcuoli/go-oracle-info/directories"
	"github.com/marcuoli/go-oracle-info/grants"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/networkacl"
	"github.com/marcuoli/go-oracle-info/sequences"
	"github.com/marcuoli/go-oracle-info/sqlplan"
	"github.com/marcuoli/go-oracle-info/triggers"
	"github.com/marcuoli/go-oracle-info/users"
	"github.com/marcuoli/go-oracle-session/rowset"
)

// Literal projections and distinct rows are independent of the selected schema.
// A valid envelope alone must not hide lost/reordered rows or changed JSON kinds.
func TestDictionaryJSONPreservesSuccessfulData(t *testing.T) {
	for _, tc := range []struct {
		name, source, columns string
		rows                  [][]driver.Value
		read                  func(context.Context, rowset.Queryer) (json.RawMessage, error)
	}{
		{"sequences", "DBA_SEQUENCES", "SEQUENCE_OWNER,SEQUENCE_NAME,MIN_VALUE,MAX_VALUE,INCREMENT_BY,CYCLE_FLAG,ORDER_FLAG,CACHE_SIZE,LAST_NUMBER", [][]driver.Value{
			{"APP", "SEQ_A", "-9223372036854775809", "9999999999999999999999", "-5", "Y", "N", "100", "9007199254740993"},
			{"OTHER", "SEQ_B", nil, "8888888888888888888888", "3", "N", "Y", "0", "9007199254740995"},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return sequences.JSON(c, q, sequences.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"database links", "DBA_DB_LINKS", "OWNER,DB_LINK,USERNAME,HOST,CREATED", [][]driver.Value{
			{"PUBLIC", "REMOTE.A", nil, "(DESCRIPTION=(ADDRESS=(HOST= db.invalid )))", "2000-02-29T01:02:03"},
			{"APP", "REMOTE.B", "OTHER", " remote host \"quoted\"\n雪", nil},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return dblinks.JSON(c, q, dblinks.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"directories", "DBA_DIRECTORIES", "OWNER,DIRECTORY_NAME,DIRECTORY_PATH", [][]driver.Value{
			{"SYS", "DIR_A", "/data/A\"\\path\n雪"}, {"APP", "DIR_B", nil},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return directories.JSON(c, q, directories.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"role grants", "ROLE_TAB_PRIVS", "ROLE,OWNER,TABLE_NAME,COLUMN_NAME,PRIVILEGE,GRANTABLE,COMMON,INHERITED", [][]driver.Value{
			{"APP_ROLE", "SYS", "OBJ_A", "Case\"Column", "UPDATE", "YES", "YES", nil},
			{"READ_ROLE", "APP", "OBJ_B", nil, "SELECT", "NO", "NO", "NO"},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return grants.RoleJSON(c, q, grants.Options{OracleMajor: 19}, rowset.Limits{})
		}},
		{"SQL profiles", "DBA_SQL_PROFILES", "NAME,CATEGORY,SIGNATURE,CREATED,LAST_MODIFIED,DESCRIPTION,TYPE,STATUS,FORCE_MATCHING", [][]driver.Value{
			{"P_A", "CAT", "18446744073709551616", "2001-01-02T03:04:05.001000000", nil, "text \"雪\"\n\\value", "REGULAR", "ENABLED", "YES"},
			{"P_B", "OTHER", "9007199254740993", nil, "2026-11-17T00:00:00.123456789", "other", "MANUAL", "DISABLED", "NO"},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return sqlplan.ProfileJSON(c, q, sqlplan.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"triggers", "DBA_TRIGGERS+DBA_OBJECTS", "OWNER,TRIGGER_NAME,TRIGGER_TYPE,TRIGGERING_EVENT,TABLE_OWNER,TABLE_NAME,TRIGGER_STATUS,OBJECT_STATUS,LAST_DDL_TIME,BASE_OBJECT_TYPE,ACTION_TYPE", [][]driver.Value{
			{"SYS", "T_A", "AFTER EVENT", "LOGON", nil, nil, "ENABLED", "INVALID", "1999-12-31T23:59:59", "DATABASE", "PL/SQL"},
			{"APP", "T_B", "BEFORE EACH ROW", "INSERT", "APP", "TAB", "DISABLED", nil, nil, "TABLE", "CALL"},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return triggers.JSON(c, q, triggers.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"users", "DBA_USERS", "USERNAME,USER_ID,ACCOUNT_STATUS,LOCK_DATE,EXPIRY_DATE,DEFAULT_TABLESPACE,TEMPORARY_TABLESPACE,CREATED,PROFILE,INITIAL_RSRC_CONSUMER_GROUP,AUTHENTICATION_TYPE,EDITIONS_ENABLED", [][]driver.Value{
			{"MiXeD", "9007199254740993", "OPEN", nil, nil, "DATA", "TEMP", "2001-02-03T04:05:06", "DEFAULT", nil, "PASSWORD", "N"},
			{"OTHER", "9007199254740995", "LOCKED", "2026-01-01T01:02:03", nil, "DATA2", "TEMP2", "2003-04-05T06:07:08", "PROFILE", "GROUP", "EXTERNAL", "Y"},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return users.JSON(c, q, users.Options{OracleMajor: 11}, rowset.Limits{})
		}},
		{"ACL host mappings", "DBA_HOST_ACLS", "HOST,LOWER_PORT,UPPER_PORT,ACL,ACLID,ACL_OWNER", [][]driver.Value{
			{"*.example.invalid", nil, nil, "/sys/acls/a.xml", "ABCD", "SYS"},
			{"other.example.invalid", "80", "443", "/sys/acls/雪\".xml", "DEAD", nil},
		}, func(c context.Context, q rowset.Queryer) (json.RawMessage, error) {
			return networkacl.JSON(c, q, networkacl.Options{OracleMajor: 19, Dataset: networkacl.DBAHostMappings}, rowset.Limits{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			names := strings.Split(tc.columns, ",")
			db, state := testsql.New(t, testsql.Step{Columns: names, Values: tc.rows})
			data, err := tc.read(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Provenance struct {
					Source string `json:"source"`
				} `json:"provenance"`
				Data rowset.Table `json:"data"`
			}
			decoder := json.NewDecoder(strings.NewReader(string(data)))
			decoder.UseNumber()
			if err = decoder.Decode(&got); err != nil {
				t.Fatal(err)
			}
			expected := rowset.Table{Columns: make([]rowset.Column, len(names)), Rows: make([][]any, len(tc.rows))}
			for i, name := range names {
				expected.Columns[i] = rowset.Column{Name: name, Kind: rowset.Text}
			}
			for i, row := range tc.rows {
				expected.Rows[i] = make([]any, len(row))
				for j, value := range row {
					expected.Rows[i][j] = value
				}
			}
			if got.Provenance.Source != tc.source || !reflect.DeepEqual(got.Data, expected) {
				t.Fatalf("successful JSON data changed: got=%+v want=%+v source=%q", got.Data, expected, got.Provenance.Source)
			}
			if state.Closed() != 1 {
				t.Fatal("owned successful cursor not finalized", state.Closed())
			}
		})
	}
}
