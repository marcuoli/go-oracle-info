package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/redo"
	"github.com/marcuoli/go-oracle-info/services"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestAssessmentIdentityReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		release                               string
		objects, edition, container, services bool
	}{
		{"9.2.0.8.0", true, false, false, false},
		{"10.1.0.5.0", true, false, false, true},
		{"10.2.0.5.0", true, false, false, true},
		{"11.1.0.7.0", true, false, false, true},
		{"11.2.0.4.0", true, true, false, true},
		{"12.1.0.1.0", true, true, true, true},
		{"12.1.0.2.0", true, true, true, true},
		{"12.2.0.1.0", true, true, true, true},
		{"18.1.0.0.0", true, true, true, true},
		{"19.33.0.0.0", true, true, true, true},
		{"21.21.0.0.0", true, true, true, true},
		{"23.9.0.0.0", true, true, true, true},
		{"23.26.1.0.0", true, true, true, true},
		{"23.26.4.0.0", true, true, true, true},
		{"8.1.7.4.0", false, false, false, false},
		{"26.0", false, false, false, false},
		{"26ai", false, false, false, false},
	} {
		t.Run(tc.release, func(t *testing.T) {
			obj, args, err := objects.ObjectIdentitiesStatement(objects.ObjectIdentitiesOptions{OracleVersion: tc.release, Owners: []string{"SYS", "SYSTEM"}})
			if tc.objects {
				if err != nil || !strings.Contains(obj, " FROM SYS.DBA_OBJECTS v") || !strings.Contains(obj, "NLSSORT(:2, 'NLS_SORT=BINARY')") ||
					strings.Contains(obj, "'SYSTEM'") || !reflect.DeepEqual(args, []any{"SYS", "SYSTEM"}) ||
					strings.Contains(obj, " AS EDITION_NAME") != tc.edition ||
					strings.Contains(obj, " AS SOURCE_CON_ID") != tc.container {
					t.Fatalf("object projection %q args=%v err=%v", obj, args, err)
				}
			} else if obj != "" || !errors.Is(err, objects.ErrUnsupportedVersion) {
				t.Fatalf("unsupported object version selected SQL: %q %v", obj, err)
			}

			threads, err := redo.ThreadsStatement(redo.ThreadsOptions{OracleVersion: tc.release, DatabaseRole: "PRIMARY"})
			if tc.objects {
				if err != nil || !strings.Contains(threads, " FROM V$THREAD v") || !strings.Contains(threads, " AS SOURCE_INST_ID") ||
					strings.Contains(threads, " AS CON_ID") != tc.container || strings.Contains(threads, " AS SOURCE_CON_ID") != tc.container {
					t.Fatalf("thread projection %q err=%v", threads, err)
				}
			} else if threads != "" || !errors.Is(err, redo.ErrUnsupportedVersion) {
				t.Fatalf("unsupported thread version selected SQL: %q %v", threads, err)
			}

			active, err := services.ActiveServicesStatement(services.ActiveServicesOptions{OracleVersion: tc.release})
			if tc.services {
				if err != nil || !strings.Contains(active, " FROM GV$ACTIVE_SERVICES v") || !strings.Contains(active, "v.INST_ID AS INST_ID") ||
					strings.Contains(active, " AS CON_ID") != tc.container || strings.Contains(active, " AS SOURCE_CON_ID") != tc.container {
					t.Fatalf("active service projection %q err=%v", active, err)
				}
			} else if active != "" || !errors.Is(err, services.ErrUnsupportedVersion) {
				t.Fatalf("unsupported service version selected SQL: %q %v", active, err)
			}
		})
	}
}

func TestRedoThreadsRejectsPhysicalStandbyBeforeView(t *testing.T) {
	ctx := t.Context()
	options := redo.ThreadsOptions{OracleVersion: "23.26.1.0.0", DatabaseRole: "PHYSICAL STANDBY"}
	db, spy := testsql.New(t)
	ref, _ := reference.NewReference("go-oracle-info", "tests", "thread-scope", 1)
	calls := 0
	reader := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
		calls++
		return session.Result{}, nil
	})
	facts, readErr := redo.ReadThreads(ctx, reader, redo.ThreadsRequest{Options: options, Reference: ref})
	rows, rowsErr := redo.ThreadsRows(ctx, db, options)
	blob, jsonErr := redo.ThreadsJSON(ctx, db, options, rowset.Limits{})
	if facts.Rows != nil || rows != nil || blob != nil || calls != 0 || len(spy.Calls()) != 0 {
		t.Fatal("standby source opened or produced evidence")
	}
	for _, err := range []error{readErr, rowsErr, jsonErr} {
		if !errors.Is(err, redo.ErrUnsupportedScope) {
			t.Fatalf("unsupported-scope result: %v", err)
		}
	}
}

func TestObjectIdentitiesExactScopeAndLimits(t *testing.T) {
	ctx := t.Context()
	options := objects.ObjectIdentitiesOptions{OracleVersion: "23.26.1.0.0", Owners: []string{"SYS", "SYSTEM"}}
	cols := []string{"OWNER", "OBJECT_NAME", "SUBOBJECT_NAME", "OBJECT_TYPE", "OBJECT_ID", "EDITION_NAME", "SOURCE_CON_ID"}
	text := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	const exact = "9007199254740993999"
	first := []session.Value{text("SYS"), text("MixedName"), null, text("PACKAGE"), text(exact), text("E1"), text("1")}
	second := []session.Value{text("SYS"), text("MixedName"), text("P1"), text("PACKAGE"), text("42"), null, text("1")}
	good := session.Result{Columns: cols, Rows: [][]session.Value{first, second}}
	got, err := objects.DecodeObjectIdentities(good, options)
	if err != nil || len(got.Rows) != 2 || got.Provenance.Complete || !got.Rows[0].SubobjectName.Null || got.Rows[0].ObjectID.Text != exact ||
		got.Rows[0].EditionName == nil || got.Rows[0].EditionName.Text != "E1" || got.Rows[1].EditionName == nil || !got.Rows[1].EditionName.Null || got.Rows[1].SubobjectName.Text != "P1" {
		t.Fatalf("exact object identities: %+v %v", got, err)
	}
	wrongOwner := append([]session.Value(nil), second...)
	wrongOwner[0] = text("sys")
	got, err = objects.DecodeObjectIdentities(session.Result{Columns: cols, Rows: [][]session.Value{first, wrongOwner}}, options)
	if got.Rows != nil || !errors.Is(err, objects.ErrInvalidEvidence) {
		t.Fatalf("late case mismatch produced evidence: %+v %v", got, err)
	}
	if _, _, err := objects.ObjectIdentitiesStatement(objects.ObjectIdentitiesOptions{OracleVersion: options.OracleVersion}); !errors.Is(err, objects.ErrInvalidRequest) {
		t.Fatalf("unbounded owner set: %v", err)
	}

	sql, args, err := objects.ObjectIdentitiesStatement(options)
	if err != nil || len(args) != 2 {
		t.Fatal("expected exact owner binds")
	}
	ref, _ := reference.NewReference("go-oracle-info", "tests", "object-identities", 1)
	reader := runtimeDictionaryReader(func(_ context.Context, stmt session.Statement) (session.Result, error) {
		if stmt.Text != sql || !reflect.DeepEqual(stmt.Args, args) || stmt.Reference != ref {
			t.Fatalf("unexpected object selection: %v", stmt)
		}
		return good, nil
	})
	request := objects.ObjectIdentitiesRequest{Options: options, Reference: ref, Limits: rowset.Limits{MaxRows: 1}}
	if got, err := objects.ReadObjectIdentities(ctx, reader, request); got.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("buffered result excluded required bound: %+v %v", got, err)
	}
	failure := errors.New("cursor tail")
	db, spy := testsql.New(t, testsql.Step{SQL: sql, Columns: cols, Values: [][]driver.Value{{"SYS", "MixedName", nil, "PACKAGE", exact, "E1", int64(1)}}, NextErr: failure})
	if blob, err := objects.ObjectIdentitiesJSON(ctx, db, options, rowset.Limits{}); blob != nil || !errors.Is(err, failure) || spy.Closed() != 1 {
		t.Fatalf("incomplete cursor published object evidence: %s %v", blob, err)
	}
}

func TestAssessmentIdentityRetainsInstanceAndContainerSemantics(t *testing.T) {
	text := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	opt := redo.ThreadsOptions{OracleVersion: "23.26.1.0.0", DatabaseRole: "PRIMARY"}
	threads, err := redo.DecodeThreads(session.Result{
		Columns: []string{"THREAD_NUMBER", "STATUS", "ENABLED", "INSTANCE_NAME", "SOURCE_INST_ID", "CON_ID", "SOURCE_CON_ID"},
		Rows:    [][]session.Value{{text("2"), text("CLOSED"), text("DISABLED"), null, text("4"), text("0"), text("1")}},
	}, opt)
	if err != nil || len(threads.Rows) != 1 || threads.Rows[0].ThreadNumber.Text != "2" || threads.Rows[0].SourceInstanceID.Text != "4" ||
		threads.Rows[0].Enabled.Text != "DISABLED" || !threads.Rows[0].InstanceName.Null ||
		threads.Rows[0].ContainerID == nil || threads.Rows[0].ContainerID.Text != "0" ||
		threads.Rows[0].SourceContainerID == nil || threads.Rows[0].SourceContainerID.Text != "1" {
		t.Fatalf("thread provenance did not retain distinct identities: %+v %v", threads, err)
	}

	serviceOptions := services.ActiveServicesOptions{OracleVersion: "23.26.1.0.0"}
	sql, err := services.ActiveServicesStatement(serviceOptions)
	if err != nil {
		t.Fatal(err)
	}
	cols := []string{"INST_ID", "NAME", "NETWORK_NAME", "CON_ID", "SOURCE_CON_ID"}
	db, spy := testsql.New(t, testsql.Step{SQL: sql, Columns: cols, Values: [][]driver.Value{{int64(1), "SYS$BACKGROUND", nil, int64(0), int64(1)}, {int64(2), "svc", "N", int64(3), int64(1)}}})
	blob, err := services.ActiveServicesJSON(t.Context(), db, serviceOptions, rowset.Limits{})
	var document struct {
		Provenance services.DatasetProvenance
		Data       struct{ Rows [][]json.RawMessage }
	}
	if err != nil || json.Unmarshal(blob, &document) != nil || !document.Provenance.Complete || spy.Closed() != 1 || len(document.Data.Rows) != 2 ||
		string(document.Data.Rows[0][1]) != "\"SYS$BACKGROUND\"" || string(document.Data.Rows[0][2]) != "null" ||
		string(document.Data.Rows[1][0]) != "2" || string(document.Data.Rows[1][3]) != "3" || string(document.Data.Rows[1][4]) != "1" {
		t.Fatalf("active service rows %s %v", blob, err)
	}
}
