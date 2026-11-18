package identity

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDatasetReleaseSelectionMatrix(t *testing.T) {
	cases := []struct {
		name      string
		selectSQL func(int) (string, []any, error)
	}{
		{name: "DatabaseIdentity", selectSQL: func(major int) (string, []any, error) {
			return DatabaseIdentityStatement(DatabaseIdentityOptions{OracleMajor: major})
		}},
		{name: "InstanceIdentity", selectSQL: func(major int) (string, []any, error) {
			return InstanceIdentityStatement(InstanceIdentityOptions{OracleMajor: major})
		}},
		{name: "ExecutionIdentity", selectSQL: func(major int) (string, []any, error) {
			return ExecutionIdentityStatement(ExecutionIdentityOptions{OracleMajor: major})
		}},
		{name: "NameSource", selectSQL: func(major int) (string, []any, error) {
			return NameSourceStatement(NameSourceOptions{OracleMajor: major})
		}},
		{name: "RoleSource", selectSQL: func(major int) (string, []any, error) {
			return RoleSourceStatement(RoleSourceOptions{OracleMajor: major})
		}},
		{name: "VersionSource", selectSQL: func(major int) (string, []any, error) {
			return VersionSourceStatement(VersionSourceOptions{OracleMajor: major})
		}},
		{name: "EditionSource", selectSQL: func(major int) (string, []any, error) {
			return EditionSourceStatement(EditionSourceOptions{OracleMajor: major})
		}},
		{name: "BannersSource", selectSQL: func(major int) (string, []any, error) {
			return BannersSourceStatement(BannersSourceOptions{OracleMajor: major})
		}},
		{name: "Snapshot", selectSQL: func(major int) (string, []any, error) { return SnapshotStatement(SnapshotOptions{OracleMajor: major}) }},
		{name: "ContainerSource", selectSQL: func(major int) (string, []any, error) {
			return ContainerSourceStatement(ContainerSourceOptions{OracleMajor: major})
		}},
		{name: "FullVersionSource", selectSQL: func(major int) (string, []any, error) {
			return FullVersionSourceStatement(FullVersionSourceOptions{OracleMajor: major})
		}},
		{name: "PDBOpenMode", selectSQL: func(major int) (string, []any, error) {
			return PDBOpenModeStatement(PDBOpenModeOptions{OracleMajor: major})
		}},
		{name: "ConnectedArchitecture", selectSQL: func(major int) (string, []any, error) {
			return ConnectedArchitectureStatement(ConnectedArchitectureOptions{OracleMajor: major})
		}},
		{name: "VersionBootstrap", selectSQL: func(major int) (string, []any, error) {
			return VersionBootstrapStatement(VersionBootstrapOptions{OracleMajor: major})
		}},
	}
	for _, c := range cases {
		for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
			text, args, err := c.selectSQL(major)
			if err == nil {
				encoded, _ := json.Marshal(struct {
					Package string
					Fact    string
					Major   int
					SQL     string
					Args    []any
				}{"identity", c.name, major, text, args})
				t.Log(string(encoded))
			}
			admitted := major == 19 || major == 21 || major == 23
			if admitted {
				if err != nil || text == "" {
					t.Fatal(c.name, major, text, err)
				}
			} else if !errors.Is(err, ErrUnsupportedVersion) || text != "" {
				t.Fatal(c.name, major, text, err)
			}
		}
	}
}
