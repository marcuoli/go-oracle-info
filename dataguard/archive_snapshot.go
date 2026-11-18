package dataguard

import (
	"strconv"
	"strings"
	"time"
)

// archiveSnapshot keeps tied copies at the maximum sequence of each reporter,
// container, thread and kind. Times/SCNs stay on their actual source row. RFS
// receipt is archived-log registration, not proof that unarchived SRL redo applied.
func archiveSnapshot(p selection, filter ArchiveProgressOptions) (selection, error) {
	raw := []string{"INST_ID", "RECID", "STAMP", "DEST_ID", "NAME", "THREAD#", "SEQUENCE#", "RESETLOGS_CHANGE#", "RESETLOGS_TIME", "RESETLOGS_ID", "FIRST_CHANGE#", "FIRST_TIME", "NEXT_CHANGE#", "NEXT_TIME", "COMPLETION_TIME", "REGISTRAR", "APPLIED", "ARCHIVED", "DELETED", "STATUS", "CON_ID"}
	var source []string
	for _, name := range raw {
		source = append(source, "s."+name+" AS "+name)
	}
	reporter := ""
	reset, e := civilTime(filter.ResetlogsTime)
	if e != nil {
		return selection{}, ErrInvalidRequest
	}
	seconds := reset.Hour()*3600 + reset.Minute()*60 + reset.Second()
	args := []any{filter.ResetlogsChange, seconds}
	var marks []string
	for _, id := range filter.Instances {
		args = append(args, id)
		marks = append(marks, ":"+strconv.Itoa(len(args)))
	}
	if len(marks) > 0 {
		reporter = " AND s.INST_ID IN (" + strings.Join(marks, ", ") + ")"
	}
	inner := "SELECT " + strings.Join(source, ", ") + ", k.KIND AS KIND, DENSE_RANK() OVER (PARTITION BY s.INST_ID, s.CON_ID, s.THREAD#, k.KIND ORDER BY s.SEQUENCE# DESC) AS PROGRESS_RANK FROM GV$ARCHIVED_LOG s CROSS JOIN (SELECT 'RECEIVED' AS KIND FROM DUAL UNION ALL SELECT 'APPLIED' AS KIND FROM DUAL) k WHERE s.RESETLOGS_CHANGE# = TO_NUMBER(:1) AND s.RESETLOGS_TIME = DATE '" + reset.Format("2006-01-02") + "' + :2 / 86400 AND s.REGISTRAR = 'RFS' AND (k.KIND = 'RECEIVED' OR s.APPLIED = 'YES') AND s.THREAD# IS NOT NULL AND s.SEQUENCE# IS NOT NULL" + reporter
	var projection []string
	for _, f := range p.fields {
		projection = append(projection, f.expression+" AS "+f.name)
	}
	p.sql = "SELECT " + strings.Join(projection, ", ") + " FROM (" + inner + ") v WHERE v.PROGRESS_RANK = 1 ORDER BY v.INST_ID, v.CON_ID, v.THREAD#, v.KIND, v.SEQUENCE#, v.DEST_ID, v.RECID"
	p.args = args
	return p, nil
}

func civilTime(value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02T15:04:05", value)
	if err != nil || t.Year() < 1 || t.Format("2006-01-02T15:04:05") != value {
		return time.Time{}, ErrInvalidRequest
	}
	return t, nil
}
