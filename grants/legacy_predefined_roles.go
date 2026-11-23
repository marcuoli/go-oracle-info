package grants

import (
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
)

// LegacyPredefinedRoles describes the documented predefined role names for a
// legacy Oracle release. Installation options and upgrade history may change
// which roles are present. An unmatched name remains unclassified.
type LegacyPredefinedRoles struct {
	Release    string
	RoleNames  []string
	SourceURLs []string
}

// LookupLegacyPredefinedRoles selects a documentation-backed legacy role list
// using the numeric database release. Results are exact, sorted and caller-owned.
// For supported modern databases use the observed DBA_ROLES.ORACLE_MAINTAINED
// flag. Neither source identifies who granted a role.
func LookupLegacyPredefinedRoles(version string) (LegacyPredefinedRoles, bool) {
	numbers, ok := oracleversion.Parse(version)
	if !ok {
		return LegacyPredefinedRoles{}, false
	}
	switch [2]uint64{numbers[0], numbers[1]} {
	case [2]uint64{9, 2}:
		return LegacyPredefinedRoles{
			Release:   "9.2",
			RoleNames: strings.Fields(legacy92Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/cd/A97385_01/server.920/a96540/statements_912a.htm",
			},
		}, true
	case [2]uint64{10, 1}:
		return LegacyPredefinedRoles{
			Release:   "10.1",
			RoleNames: strings.Fields(legacy10Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/cd/B13789_01/network.101/b10773/admusers.htm",
			},
		}, true
	case [2]uint64{10, 2}:
		return LegacyPredefinedRoles{
			Release:   "10.2",
			RoleNames: strings.Fields(legacy10Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/cd/B19306_01/network.102/b14266/admusers.htm",
			},
		}, true
	case [2]uint64{11, 1}:
		return LegacyPredefinedRoles{
			Release:   "11.1",
			RoleNames: strings.Fields(legacy111Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/cd/B28359_01/network.111/b28531/authorization.htm",
			},
		}, true
	case [2]uint64{11, 2}:
		return LegacyPredefinedRoles{
			Release:   "11.2",
			RoleNames: strings.Fields(legacy112Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/cd/E18283_01/network.112/e16543/authorization.htm",
			},
		}, true
	case [2]uint64{12, 1}:
		if numbers[2] != 0 || numbers[3] != 1 {
			return LegacyPredefinedRoles{}, false
		}
		return LegacyPredefinedRoles{
			Release:   "12.1.0.1",
			RoleNames: strings.Fields(legacy12101Roles),
			SourceURLs: []string{
				"https://docs.oracle.com/database/121/DBSEG/authorization.htm",
				"https://docs.oracle.com/database/121/NEWFT/chapter12101.htm",
				"https://docs.oracle.com/database/121/ADMQS/E51671-10.pdf",
			},
		}, true
	default:
		return LegacyPredefinedRoles{}, false
	}
}

const (
	legacy92Roles = `AQ_ADMINISTRATOR_ROLE AQ_USER_ROLE CONNECT DBA DELETE_CATALOG_ROLE EXECUTE_CATALOG_ROLE
EXP_FULL_DATABASE HS_ADMIN_ROLE IMP_FULL_DATABASE RECOVERY_CATALOG_OWNER RESOURCE
SELECT_CATALOG_ROLE SNMPAGENT`
	legacy10Roles = `AQ_ADMINISTRATOR_ROLE AQ_USER_ROLE CONNECT DBA DELETE_CATALOG_ROLE EXECUTE_CATALOG_ROLE
EXP_FULL_DATABASE HS_ADMIN_ROLE IMP_FULL_DATABASE RECOVERY_CATALOG_OWNER RESOURCE
SELECT_CATALOG_ROLE`
	legacy111Roles = `AQ_ADMINISTRATOR_ROLE AQ_USER_ROLE AUTHENTICATEDUSER CONNECT CSW_USR_ROLE CTXAPP CWM_USER
DATAPUMP_EXP_FULL_DATABASE DATAPUMP_IMP_FULL_DATABASE DBA DELETE_CATALOG_ROLE EJBCLIENT
EXECUTE_CATALOG_ROLE EXP_FULL_DATABASE GATHER_SYSTEM_STATISTICS GLOBAL_AQ_USER_ROLE HS_ADMIN_ROLE
IMP_FULL_DATABASE JAVADEBUGPRIV JAVAIDPRIV JAVASYSPRIV JAVAUSERPRIV JAVA_ADMIN JAVA_DEPLOY
JMXSERVER LBAC_DBA LOGSTDBY_ADMINISTRATOR MGMT_USER OEM_ADVISOR OEM_MONITOR OLAPI_TRACE_USER
OLAP_DBA OLAP_USER OLAP_XS_ADMIN ORDADMIN OWB$CLIENT OWB_DESIGNCENTER_VIEW OWB_USER
RECOVERY_CATALOG_OWNER RESOURCE SCHEDULER_ADMIN SELECT_CATALOG_ROLE SPATIAL_CSW_ADMIN
SPATIAL_WFS_ADMIN WFS_USR_ROLE WKUSER WM_ADMIN_ROLE XDBADMIN XDB_SET_INVOKER XDB_WEBSERVICES
XDB_WEBSERVICES_OVER_HTTP XDB_WEBSERVICES_WITH_PUBLIC`
	legacy112Roles = `ADM_PARALLEL_EXECUTE_TASK AQ_ADMINISTRATOR_ROLE AQ_USER_ROLE AUTHENTICATEDUSER CAPI_USER_ROLE
CONNECT CSW_USR_ROLE CTXAPP CWM_USER DATAPUMP_EXP_FULL_DATABASE DATAPUMP_IMP_FULL_DATABASE DBA
DELETE_CATALOG_ROLE EJBCLIENT EXECUTE_CATALOG_ROLE EXP_FULL_DATABASE GATHER_SYSTEM_STATISTICS
GLOBAL_AQ_USER_ROLE HS_ADMIN_EXECUTE_ROLE HS_ADMIN_ROLE HS_ADMIN_SELECT_ROLE IMP_FULL_DATABASE
JAVADEBUGPRIV JAVAIDPRIV JAVASYSPRIV JAVAUSERPRIV JAVA_ADMIN JAVA_DEPLOY JMXSERVER LBAC_DBA
LOGSTDBY_ADMINISTRATOR MGMT_USER OEM_ADVISOR OEM_MONITOR OLAPI_TRACE_USER OLAP_DBA OLAP_USER
OLAP_XS_ADMIN ORDADMIN OWB$CLIENT OWB_DESIGNCENTER_VIEW OWB_USER RECOVERY_CATALOG_OWNER RESOURCE
SCHEDULER_ADMIN SELECT_CATALOG_ROLE SNMPAGENT SPATIAL_CSW_ADMIN SPATIAL_WFS_ADMIN WFS_USR_ROLE
WM_ADMIN_ROLE XDBADMIN XDB_SET_INVOKER XDB_WEBSERVICES XDB_WEBSERVICES_OVER_HTTP
XDB_WEBSERVICES_WITH_PUBLIC`
	legacy12101Roles = `ADM_PARALLEL_EXECUTE_TASK AQ_ADMINISTRATOR_ROLE AQ_USER_ROLE AUDIT_ADMIN AUDIT_VIEWER
AUTHENTICATEDUSER CAPTURE_ADMIN CDB_DBA CONNECT CSW_USR_ROLE CTXAPP CWM_USER
DATAPUMP_EXP_FULL_DATABASE DATAPUMP_IMP_FULL_DATABASE DBA DELETE_CATALOG_ROLE EJBCLIENT
EM_EXPRESS_ALL EM_EXPRESS_BASIC EXECUTE_CATALOG_ROLE EXP_FULL_DATABASE GATHER_SYSTEM_STATISTICS
GLOBAL_AQ_USER_ROLE HS_ADMIN_EXECUTE_ROLE HS_ADMIN_ROLE HS_ADMIN_SELECT_ROLE IMP_FULL_DATABASE
JAVADEBUGPRIV JAVAIDPRIV JAVASYSPRIV JAVAUSERPRIV JAVA_ADMIN JAVA_DEPLOY JMXSERVER LBAC_DBA
LOGSTDBY_ADMINISTRATOR OEM_ADVISOR OEM_MONITOR OLAP_DBA OLAP_USER OLAP_XS_ADMIN ORDADMIN PDB_DBA
RECOVERY_CATALOG_OWNER RESOURCE SCHEDULER_ADMIN SELECT_CATALOG_ROLE SPATIAL_CSW_ADMIN
SPATIAL_WFS_ADMIN WFS_USR_ROLE WM_ADMIN_ROLE XDBADMIN XDB_SET_INVOKER XDB_WEBSERVICES
XDB_WEBSERVICES_OVER_HTTP XDB_WEBSERVICES_WITH_PUBLIC`
)
