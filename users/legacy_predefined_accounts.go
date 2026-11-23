package users

import (
	"slices"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
)

// LegacyPredefinedAccounts describes published Oracle account names for a legacy release.
// Usernames are exact dictionary names with documentation evidence. They do not
// enumerate any particular installation, and an unlisted name is unclassified.
type LegacyPredefinedAccounts struct {
	Release    string
	Usernames  []string
	SourceURLs []string
}

// LookupLegacyPredefinedAccounts looks up a numeric database version in the
// catalog. It returns caller-owned slices. Version families 9.2 through 11.2
// use release-level documentation. Oracle 12.1 requires the exact patch-set
// components because ORACLE_MAINTAINED became available in 12.1.0.2.
func LookupLegacyPredefinedAccounts(version string) (LegacyPredefinedAccounts, bool) {
	numbers, ok := oracleversion.Parse(version)
	if !ok {
		return LegacyPredefinedAccounts{}, false
	}

	var release, names string
	var sources []string
	switch [2]uint64{numbers[0], numbers[1]} {
	case [2]uint64{9, 2}:
		release = "9.2"
		names = "ANONYMOUS AURORA$JIS$UTILITY$ AURORA$ORB$UNAUTHENTICATED CTXSYS DBSNMP LBACSYS MDSYS OLAPSYS ORDPLUGINS ORDSYS OSE$HTTP$ADMIN OUTLN SYS SYSTEM WKSYS WMSYS XDB"
		sources = []string{
			"https://docs.oracle.com/cd/B10501_01/win.920/a95490/username.htm",
			"https://docs.oracle.com/cd/B10501_01/server.920/a96521/dba.htm",
		}
	case [2]uint64{10, 1}:
		release = "10.1"
		names = legacy10gAccounts
		sources = []string{"https://docs.oracle.com/cd/B12037_01/network.101/b10773/policies.htm"}
	case [2]uint64{10, 2}:
		release = "10.2"
		names = legacy10gAccounts + " TSMSYS"
		sources = []string{"https://docs.oracle.com/cd/B19306_01/network.102/b14266/policies.htm"}
	case [2]uint64{11, 1}:
		release = "11.1"
		names = legacy11gAccounts + " TSMSYS"
		sources = []string{"https://docs.oracle.com/cd/B28359_01/server.111/b28337/tdpsg_user_accounts.htm"}
	case [2]uint64{11, 2}:
		release = "11.2"
		names = legacy11gAccounts + " ORDDATA APEX_030200 APPQOSSYS OWBSYS_AUDIT"
		sources = []string{
			"https://docs.oracle.com/cd/E24693_01/server.11203/e10575/tdpsg_user_accounts.htm",
			"https://docs.oracle.com/cd/E14075_01/doc/install.112/e10840.pdf",
			"https://docs.oracle.com/cd/E25154_01/doc/owb.11203/e17130/install.htm",
		}
	case [2]uint64{12, 1}:
		if numbers[2] != 0 || numbers[3] != 1 {
			return LegacyPredefinedAccounts{}, false
		}
		release = "12.1.0.1"
		names = "ANONYMOUS APEX_040200 AUDSYS CTXSYS DBSNMP DIP LBACSYS MDDATA MDSYS OLAPSYS ORACLE_OCM ORDDATA ORDPLUGINS ORDSYS SI_INFORMTN_SCHEMA SPATIAL_CSW_ADMIN_USR SPATIAL_WFS_ADMIN_USR SYS SYSBACKUP SYSDG SYSKM SYSTEM WMSYS XDB XS$NULL"
		sources = []string{
			"https://docs.oracle.com/database/121/TDPSG/GUID-3EC7A894-D620-4497-AFB1-64EB8C33D854.htm",
			"https://docs.oracle.com/database/121/NEWFT/chapter12101.htm",
			"https://docs.oracle.com/en/database/oracle/application-express/19.2/htmig/upgrading-from-previous-Oracle-application-express-release.html",
			"https://docs.oracle.com/cd/E59726_01/install.50/e39144/overview.htm",
		}
	default:
		return LegacyPredefinedAccounts{}, false
	}
	usernames := strings.Fields(names)
	slices.Sort(usernames)
	return LegacyPredefinedAccounts{Release: release, Usernames: usernames, SourceURLs: sources}, true
}

// These are the names common to the cited tables for each pair of releases.
// Keep installation-specific additions with their own release and source.
const (
	legacy10gAccounts = "ANONYMOUS CTXSYS DBSNMP DIP DMSYS EXFSYS MDDATA MDSYS MGMT_VIEW ODM ODM_MTR OLAPSYS ORDPLUGINS ORDSYS OUTLN SI_INFORMTN_SCHEMA SYS SYSMAN SYSTEM WK_TEST WKPROXY WKSYS WMSYS XDB"
	legacy11gAccounts = "ANONYMOUS APEX_PUBLIC_USER CTXSYS DBSNMP DIP EXFSYS FLOWS_30000 FLOWS_FILES LBACSYS MDDATA MDSYS MGMT_VIEW OLAPSYS ORACLE_OCM ORDPLUGINS ORDSYS OUTLN OWBSYS SI_INFORMTN_SCHEMA SPATIAL_CSW_ADMIN_USR SPATIAL_WFS_ADMIN_USR SYS SYSMAN SYSTEM WK_TEST WKPROXY WKSYS WMSYS XDB XS$NULL"
)
