package database

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
		{name: "Configuration", selectSQL: func(major int) (string, []any, error) {
			return ConfigurationStatement(ConfigurationOptions{OracleMajor: major})
		}},
		{name: "Endian", selectSQL: func(major int) (string, []any, error) { return EndianStatement(EndianOptions{OracleMajor: major}) }},
		{name: "CharacterSet", selectSQL: func(major int) (string, []any, error) {
			return CharacterSetStatement(CharacterSetOptions{OracleMajor: major})
		}},
		{name: "CPUCount", selectSQL: func(major int) (string, []any, error) { return CPUCountStatement(CPUCountOptions{OracleMajor: major}) }},
		{name: "Products", selectSQL: func(major int) (string, []any, error) { return ProductsStatement(ProductsOptions{OracleMajor: major}) }},
		{name: "DatafileSize", selectSQL: func(major int) (string, []any, error) {
			return DatafileSizeStatement(DatafileSizeOptions{OracleMajor: major})
		}},
		{name: "Storage", selectSQL: func(major int) (string, []any, error) { return StorageStatement(StorageOptions{OracleMajor: major}) }},
		{name: "EncryptionCounts", selectSQL: func(major int) (string, []any, error) {
			return EncryptionCountsStatement(EncryptionCountsOptions{OracleMajor: major})
		}},
		{name: "SGAAllocated", selectSQL: func(major int) (string, []any, error) {
			return SGAAllocatedStatement(SGAAllocatedOptions{OracleMajor: major})
		}},
		{name: "PGAAllocated", selectSQL: func(major int) (string, []any, error) {
			return PGAAllocatedStatement(PGAAllocatedOptions{OracleMajor: major})
		}},
		{name: "PDBCount", selectSQL: func(major int) (string, []any, error) {
			return PDBCountStatement(PDBCountOptions{OracleMajor: major, Root: true})
		}},
		{name: "SessionParameters", selectSQL: func(major int) (string, []any, error) {
			return SessionParametersStatement(SessionParametersOptions{OracleMajor: major})
		}},
		{name: "SessionParametersWithDefaults", selectSQL: func(major int) (string, []any, error) {
			return SessionParametersWithDefaultsStatement(SessionParametersWithDefaultsOptions{OracleMajor: major})
		}},
		{name: "NLSInstance", selectSQL: func(major int) (string, []any, error) {
			return NLSInstanceStatement(NLSInstanceOptions{OracleMajor: major})
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
				}{"database", c.name, major, text, args})
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
