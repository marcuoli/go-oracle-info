package catalog

// These fixed-view projections are admitted only after their live dictionary
// columns are observed. Similar DBA_HIST names do not establish a live schema.
const MetadataSQL = `SELECT C.TABLE_NAME AS SOURCE_NAME,
       C.COLUMN_NAME AS COLUMN_NAME,
       C.DATA_TYPE AS DATA_TYPE
FROM SYS.ALL_TAB_COLUMNS C
WHERE C.OWNER = 'SYS'
  AND C.TABLE_NAME IN ('GV_$CELL', 'V_$CELL',
    'GV_$CELL_CONFIG', 'V_$CELL_CONFIG',
    'GV_$CELL_CONFIG_INFO', 'V_$CELL_CONFIG_INFO',
    'GV_$CELL_OPEN_ALERTS', 'V_$CELL_OPEN_ALERTS')
ORDER BY C.TABLE_NAME, C.COLUMN_ID`

const CellsTemplate = `SELECT {{.Instance}} AS INST_ID,
       TO_CHAR(C.CELL_HASHVAL, 'TM9') AS CELL_HASHVAL,
       C.CELL_PATH AS CELL_PATH,
       {{.CellType}} AS CELL_TYPE
FROM {{.View}} C`

const XMLTemplate = `SELECT {{.Instance}} AS INST_ID,
       {{.CellHash}} AS CELL_HASHVAL,
       {{.CellName}} AS CELL_NAME,
       {{.SourceType}} AS SOURCE_TYPE,
       {{.Payload}} AS XML_PAYLOAD,
       {{.NativeLength}} AS XML_LENGTH
FROM {{.View}} C`

const ScalarAlertsTemplate = `SELECT {{.Instance}} AS INST_ID,
       TO_CHAR(C.CELL_HASH, 'TM9') AS CELL_HASHVAL,
       C.CELL_NAME AS CELL_NAME,
       TO_CHAR(C.BEGIN_TIME, 'YYYY-MM-DD"T"HH24:MI:SS AD', 'NLS_DATE_LANGUAGE=English NLS_CALENDAR=Gregorian') AS BEGIN_TIME,
       TO_CHAR(C.SEQ_NO, 'TM9') AS SEQ_NO,
       C.MESSAGE AS MESSAGE,
       C.STATEFUL AS STATEFUL,
       C.SEVERITY AS SEVERITY,
       {{.Container}} AS CON_ID
FROM {{.View}} C`
