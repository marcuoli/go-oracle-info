package passwordfile_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/passwordfile"
)

func TestStatementVersionAndAdmittedColumn(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"11.2.0.2.0", "12", "12.1.0.2.0", "", "12.2x", "12.2", "12.2.0", "12.2.0.0", "12.2.0.0.0", "12.2.", "19.0.0.0.0.1", "19.-1"} {
		t.Run(version, func(t *testing.T) {
			sql, err := passwordfile.Statement(passwordfile.Options{OracleVersion: version})
			if !errors.Is(err, passwordfile.ErrUnsupportedVersion) || sql != "" {
				t.Fatal(sql, err)
			}
		})
	}
	for _, version := range []string{"12.2.0.1", "12.2.0.1.0", "12.2.0.2", "12.2.1", "12.3", "19.32.0.0.0", "23.26.1.0.0"} {
		sql, err := passwordfile.Statement(passwordfile.Options{OracleVersion: version})
		if err != nil || sql != "SELECT p.FILE_NAME AS FILE_NAME, p.FORMAT AS FORMAT, p.IS_ASM AS IS_ASM, p.CON_ID AS CON_ID FROM V$PASSWORDFILE_INFO p" {
			t.Fatal(sql, err)
		}
		if strings.Contains(sql, "IS_EXC") {
			t.Fatal("column guessed from version")
		}
	}
	sql, err := passwordfile.Statement(passwordfile.Options{OracleVersion: "23.26.1.0.0", IncludeExascale: true})
	if err != nil || !strings.Contains(sql, "p.IS_EXC AS IS_EXC, p.CON_ID AS CON_ID") {
		t.Fatal(sql, err)
	}
	for _, bad := range []string{"PWFILE_USERS", "PASSWORD=", "ALTER", "FLUSH", "X$", "DBA_USERS"} {
		if strings.Contains(sql, bad) {
			t.Fatal("metadata query widened", sql)
		}
	}
}

func TestDocumentedDefaultOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, version string
		extra         bool
	}{{"12.2", "12.2.0.1.0", false}, {"19", "19.32.0.0.0", false}, {"21", "21.0.0.0.0", true}, {"26ai-version23", "23.26.1.0.0", true}} {
		t.Run(tc.name, func(t *testing.T) {
			options, err := passwordfile.DefaultOptions(tc.version)
			if err != nil || options.OracleVersion != tc.version || options.IncludeExascale != tc.extra {
				t.Fatal(options, err)
			}
			sql, err := passwordfile.Statement(options)
			if err != nil || strings.Contains(sql, "p.IS_EXC AS IS_EXC") != tc.extra {
				t.Fatal(sql, err)
			}
		})
	}
	if options, err := passwordfile.DefaultOptions("12.1.0.2.0"); !errors.Is(err, passwordfile.ErrUnsupportedVersion) || options.OracleVersion != "" {
		t.Fatal(options, err)
	}
}

func TestDefaultOptionsRequiresEstablishedMinimum(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"12.1.0.2.0", "12.2", "12.2.0", "12.2.0.0", "12.2.0.0.0"} {
		options, err := passwordfile.DefaultOptions(version)
		if !errors.Is(err, passwordfile.ErrUnsupportedVersion) || options.OracleVersion != "" {
			t.Fatal(version, options, err)
		}
	}
	for _, version := range []string{"12.2.0.1", "12.2.0.2", "12.2.1", "12.3"} {
		options, err := passwordfile.DefaultOptions(version)
		if err != nil || options.IncludeExascale || options.OracleVersion != version {
			t.Fatal(version, options, err)
		}
	}
}
