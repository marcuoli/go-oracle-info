package users_test

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/users"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestPDBAccountsBindAndPreserveOwnerFlags(t *testing.T) {
	for _, tc := range []struct {
		version   string
		supported bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false}, {"12.1.0.1.0", false},
		{"12.1.0.2.0", true}, {"12.2.0.1.0", true}, {"18.0.0.0.0", true},
		{"19.32.0.0.0", true}, {"21.0.0.0.0", true}, {"23.9.0.0.0", true},
		{"23.26.1.0.0", true}, {"26ai", false}, {"26.0.0.0.0", false}, {"22.0.0.0.0", false}, {"12.2.0.0.0", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			sql, args, err := users.PDBAccountsStatement(users.PDBAccountsOptions{OracleVersion: tc.version, ContainerID: 7})
			if !tc.supported {
				if err == nil || sql != "" || args != nil {
					t.Fatal("unavailable maintenance flag was selected")
				}
				return
			}
			if err != nil || len(args) != 1 || args[0] != 7 || !strings.Contains(sql, "u.CON_ID = :1") ||
				strings.Contains(sql, "ORACLE_MAINTAINED =") {
				t.Fatal("the statement discarded the scope bind or filtered out account evidence")
			}
		})
	}
	options := users.PDBAccountsOptions{OracleVersion: "23.26.1.0.0", ContainerID: 7}
	result := session.Result{
		Columns: []string{"ORACLE_MAINTAINED", "USERNAME"},
		Rows:    [][]session.Value{{{Text: "Y"}, {Text: "SYS"}}, {{Text: "N"}, {Text: "CaseSensitive"}}, {{Null: true}, {Text: "UnknownFlag"}}},
	}
	accounts, err := users.DecodePDBAccounts(result, options)
	if err != nil || len(accounts) != 3 || accounts[0].OracleMaintained.Text != "Y" ||
		accounts[1].Username.Text != "CaseSensitive" || accounts[1].OracleMaintained.Text != "N" || !accounts[2].OracleMaintained.Null {
		t.Fatalf("account evidence changed: %+v, %v", accounts, err)
	}
	result.Rows = nil
	accounts, err = users.DecodePDBAccounts(result, options)
	if err != nil || accounts == nil || len(accounts) != 0 {
		t.Fatalf("successful empty dictionary changed: %+v, %v", accounts, err)
	}
}
