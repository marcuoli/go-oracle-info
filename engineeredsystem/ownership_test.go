package engineeredsystem

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func engineeredOwnershipProblems(file *ast.File) []string {
	var problems []string
	aliases := map[string]string{}
	for _, item := range file.Imports {
		value, _ := strconv.Unquote(item.Path.Value)
		alias := filepath.Base(value)
		if item.Name != nil {
			alias = item.Name.Name
		}
		aliases[alias] = value
		if alias == "." && (value == "database/sql" || value == "github.com/marcuoli/go-oracle-session/session") {
			problems = append(problems, "dot import hides physical transport constructors")
		}
		if strings.Contains(value, "/orascope/") || strings.Contains(value, "/oramover/") || strings.HasPrefix(value, "github.com/sijms/go-ora") || value == "database/sql/driver" {
			problems = append(problems, "application/driver import "+value)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		owner := aliases[receiver.Name]
		// Inspect references too: assigning Open to a variable must not bypass this
		// boundary. Open is the released shared-session physical acquisition API.
		if owner == "database/sql" && (selector.Sel.Name == "Open" || selector.Sel.Name == "OpenDB") || owner == "github.com/marcuoli/go-oracle-session/session" && (selector.Sel.Name == "Open" || selector.Sel.Name == "New") {
			problems = append(problems, "physical transport constructor reference")
		}
		return true
	})
	return problems
}
func TestSharedEngineeredFactsDoNotOwnApplicationOrTransport(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, problem := range engineeredOwnershipProblems(file) {
			t.Errorf("%s: %s", path, problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestSharedTransportGuardRejectsActualConstructorAndAliases(t *testing.T) {
	for _, test := range []struct {
		owner, body string
		bad         bool
	}{
		{"github.com/marcuoli/go-oracle-session/session", "func f(){s.Open(ctx,connector,options)}", true},
		{"github.com/marcuoli/go-oracle-session/session", "var acquire=s.Open", true},
		{"github.com/marcuoli/go-oracle-session/session", "var result=s.Result{}", false},
		{"database/sql", "func f(){s.Open(driver,dsn)}", true},
		{"database/sql", "var acquire=s.OpenDB", true},
		{"database/sql", "var value=s.NullString{}", false},
		{"example.com/unrelated/session", "var value=s.Open", false},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", fmt.Sprintf("package fixture\nimport s %q\n%s", test.owner, test.body), 0)
		if err != nil {
			t.Fatal(err)
		}
		problems := engineeredOwnershipProblems(file)
		if (len(problems) > 0) != test.bad {
			t.Errorf("%s %s: %v", test.owner, test.body, problems)
		}
	}
}

func TestSharedTransportGuardRejectsRestrictedDotImports(t *testing.T) {
	for _, owner := range []string{"database/sql", "github.com/marcuoli/go-oracle-session/session", "example.com/unrelated/session"} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", fmt.Sprintf("package fixture\nimport . %q\nfunc f(){Open(ctx,connector,options)}", owner), 0)
		if err != nil {
			t.Fatal(err)
		}
		bad := owner != "example.com/unrelated/session"
		if (len(engineeredOwnershipProblems(file)) > 0) != bad {
			t.Errorf("dot import ownership mismatch: %s", owner)
		}
	}
}
