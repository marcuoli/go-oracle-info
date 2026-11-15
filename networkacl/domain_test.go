package networkacl

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestDistinctACLSourcesAndLegacyFlags(t *testing.T) {
	cases := []struct {
		major   int
		dataset Dataset
		source  string
	}{{11, DBAHostMappings, "DBA_NETWORK_ACLS"}, {12, DBAHostMappings, "DBA_HOST_ACLS"}, {11, DBAHostPrivileges, "DBA_NETWORK_ACL_PRIVILEGES"}, {19, DBAHostPrivileges, "DBA_HOST_ACES"}, {11, DBAWalletMappings, "DBA_WALLET_ACLS"}, {19, DBAWalletMappings, "DBA_WALLET_ACLS"}, {19, DBAWalletPrivileges, "DBA_WALLET_ACES"}, {11, UserHostPrivileges, "USER_NETWORK_ACL_PRIVILEGES"}, {19, UserHostPrivileges, "USER_HOST_ACES"}, {19, UserWalletPrivileges, "USER_WALLET_ACES"}}
	for _, tc := range cases {
		p, err := selectACL(Options{OracleMajor: tc.major, Dataset: tc.dataset})
		if err != nil || p.provenance.Source != tc.source || !strings.Contains(p.sql, " FROM "+tc.source+" t") {
			t.Fatal(tc, p, err)
		}
	}
	r := session.Result{Columns: []string{"ACL", "ACLID", "PRINCIPAL", "PRIVILEGE", "GRANT_TYPE", "INVERTED_PRINCIPAL", "START_DATE", "END_DATE"}, Rows: [][]session.Value{{{Text: "/sys/acls/a.xml"}, {Text: "ABCD"}, {Text: "APP_ROLE"}, {Text: "connect"}, {Text: "false"}, {Text: "true"}, {Null: true}, {Text: "2030-01-01T00:00:00.000000000"}}}}
	got, err := Decode(r, Options{OracleMajor: 11, Dataset: DBAHostPrivileges})
	if err != nil || got.Records[0].GrantType.Text != "false" || got.Records[0].InvertedPrincipal.Text != "true" || got.Records[0].Host != nil || !got.Records[0].StartDate.Null {
		t.Fatal(got, err)
	}
	sql, _, err := Statement(Options{OracleMajor: 11, Dataset: DBAHostPrivileges})
	if err != nil || !strings.Contains(sql, "t.IS_GRANT AS GRANT_TYPE") || !strings.Contains(sql, "t.INVERT AS INVERTED_PRINCIPAL") {
		t.Fatal(sql, err)
	}
	user := session.Result{Columns: []string{"HOST", "LOWER_PORT", "UPPER_PORT", "PRIVILEGE", "STATUS"}, Rows: [][]session.Value{{{Text: "*.example.invalid"}, {Null: true}, {Null: true}, {Text: "resolve"}, {Text: "DENIED"}}}}
	u, err := Decode(user, Options{OracleMajor: 19, Dataset: UserHostPrivileges})
	if err != nil || u.Provenance.Scope != "current_user_visible_privilege_status" || u.Records[0].Principal != nil || u.Records[0].Status.Text != "DENIED" || !u.Records[0].LowerPort.Null {
		t.Fatal(u, err)
	}
	for _, dataset := range []Dataset{DBAWalletPrivileges, UserWalletPrivileges} {
		if _, _, err := Statement(Options{OracleMajor: 11, Dataset: dataset}); !errors.Is(err, ErrUnsupportedCapability) {
			t.Fatal(dataset, err)
		}
	}
	if _, _, err := Statement(Options{OracleMajor: 19, Dataset: "injected"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}
