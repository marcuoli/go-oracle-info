package memory

import (
	"encoding/json"
	"errors"
	"testing"
)

// The release matrix records source selection, never native qualification.
func TestMemoryNativeSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		for _, scope := range []Scope{NonCDB, CDBRoot, ConnectedPDB} {
			for _, parameter := range []Parameter{SGA, PGA} {
				o := Options{OracleMajor: major, Scope: scope}
				if scope == ConnectedPDB {
					o.TargetContainerID = 3
					o.PDB.AllowInherited = true
				}
				for _, rac := range []bool{false, true} {
					p, err := sourcePlan(parameter, o, rac)
					admitted := major != 26 && (scope == NonCDB || major >= 12)
					if !admitted {
						if err == nil || p.sql != "" {
							t.Fatal(major, scope, parameter, rac, p.sql, err)
						}
						if major == 26 && !errors.Is(err, ErrUnsupportedVersion) {
							t.Fatal(err)
						}
						continue
					}
					if err != nil || p.sql == "" {
						t.Fatal(major, scope, parameter, rac, err)
					}
					// Forwarding must use the same reviewed legacy selector and bindings.
					var want string
					var args []any
					if rac {
						want, err = RACStatement(o)
					} else {
						want, err = Statement(parameter, o)
					}
					if err != nil || p.sql != want {
						t.Fatal(major, scope, parameter, rac, "selection diverged", err)
					}
					b, _ := json.Marshal(struct {
						Package, Fact string
						Major         int
						Scope         Scope
						SQL           string
						Args          []any
					}{"memory", string(parameter) + "_" + string(scope) + func() string {
						if rac {
							return "_rac"
						}
						return ""
					}(), major, scope, p.sql, args})
					t.Log(string(b))
				}
			}
		}
	}
}
