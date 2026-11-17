package instances_test

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
	"testing"
)

func versionFixture(major int) session.Result {
	names := []string{"INST_ID", "INSTANCE_NUMBER", "INSTANCE_NAME", "HOST_NAME", "STATUS", "VERSION"}
	row := []session.Value{{Text: "7"}, {Text: "7"}, {Text: "node7"}, {Null: true}, {Text: "MOUNTED"}, {Text: "19.0.0.0.0"}}
	names = append(names, "SESSION_USER", "ISDBA")
	row = append(row, session.Value{Text: "SYS"}, session.Value{Text: "TRUE"})
	if major >= 12 {
		names = append(names, "CURRENT_CONTAINER_ID")
		row = append(row, session.Value{Text: "1"})
	}
	if major >= 23 {
		names = append(names, "CLOUD_SERVICE")
		row = append(row, session.Value{Null: true})
	}
	if major >= 12 {
		names = append(names, "CON_ID")
		row = append(row, session.Value{Text: "0"})
	}
	if major >= 18 {
		names = append(names, "VERSION_FULL")
		row = append(row, session.Value{Text: "19.32.0.0.0"})
	}
	return session.Result{Columns: names, Rows: [][]session.Value{row}}
}
func TestVersionsPreserveEveryVisibleMemberAndPrecision(t *testing.T) {
	for _, major := range []int{11, 12, 18, 19, 21, 23} {
		sql, err := instances.VersionsStatement(major)
		if err != nil || !strings.Contains(sql, "FROM GV$INSTANCE i") || strings.Contains(sql, "i.INST_ID =") || strings.Contains(sql, "STATUS =") {
			t.Fatal(sql, err)
		}
		if strings.Contains(sql, "VERSION_FULL") != (major >= 18) {
			t.Fatal(sql)
		}
		r := versionFixture(major)
		r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
		r.Rows[1][0].Text = "8"
		got, err := instances.DecodeVersions(r, major)
		if err != nil || len(got.Rows) != 2 || got.Rows[1].InstanceID.Text != "8" || got.Rows[0].Status.Text != "MOUNTED" || !got.Rows[0].HostName.Null {
			t.Fatal(got, err)
		}
		if (got.Rows[0].VersionFull != nil) != (major >= 18) || (got.Rows[0].ContainerID != nil) != (major >= 12) {
			t.Fatal("source absence lost", got)
		}
		if (got.Precision == instances.FullPrecision) != (major >= 18) {
			t.Fatal(got.Precision)
		}
	}
}
func TestVersionsRejectLateMalformedRowsAndFailedPrefix(t *testing.T) {
	for _, name := range []string{"INST_ID", "VERSION_FULL", "CON_ID"} {
		r := versionFixture(19)
		r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
		v := session.Value{Text: string([]byte{255})}
		if name == "INST_ID" {
			v = session.Value{Text: "0"}
		}
		change(&r, name, v)
		got, err := instances.DecodeVersions(r, 19)
		if !errors.Is(err, instances.ErrInvalidEvidence) || got.Rows != nil || got.State != instances.Unavailable {
			t.Fatal(got, err)
		}
	}
	ref, err := reference.NewReference("unit", "instances", "versions", 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader := readerFunc(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return versionFixture(19), nil
	})
	got, err := instances.ReadVersions(ctx, reader, instances.Request{OracleMajor: 19, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
}
