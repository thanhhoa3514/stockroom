package archtest

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestImports(t *testing.T) {
	const module = "github.com/thanhhoa3514/stockroom/"
	allowed := map[string][]string{
		"internal/domain":                      {},
		"internal/application":                 {"internal/domain"},
		"internal/infrastructure/sqlite":       {"internal/application", "internal/domain"},
		"internal/infrastructure/fakesupplier": {"internal/application", "internal/domain"},
		"internal/interfaces/cli":              {"internal/application"},
	}
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel("../..", path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		rules, checked := allowed[pkg]
		if !checked {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if pkg == "internal/domain" && (name == "database/sql" || name == "encoding/json" || name == "os") {
				t.Errorf("%s: forbidden import %s", rel, name)
			}
			if !strings.HasPrefix(name, module) {
				if pkg == "internal/domain" {
					imported, err := build.Default.Import(name, "", build.FindOnly)
					if err != nil || !imported.Goroot {
						t.Errorf("%s: domain may only import standard library packages, got %s", rel, name)
					}
				}
				continue
			}
			target := strings.TrimPrefix(name, module)
			ok := false
			for _, a := range rules {
				if target == a {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s: forbidden import %s", rel, name)
			}
		}
		if pkg == "internal/domain" || pkg == "internal/application" {
			full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			timeNames := map[string]bool{"time": true}
			for _, imp := range full.Imports {
				if imp.Path.Value == `"time"` && imp.Name != nil {
					if imp.Name.Name == "." {
						t.Errorf("%s: dot import of time prevents enforcing time.Now rule", rel)
					}
					timeNames[imp.Name.Name] = true
				}
			}
			ast.Inspect(full, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Now" {
					if id, ok := sel.X.(*ast.Ident); ok && timeNames[id.Name] {
						t.Errorf("%s: forbidden time.Now call", rel)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
