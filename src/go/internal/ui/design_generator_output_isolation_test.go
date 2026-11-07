package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// Each defaults to a real tracked generated file.
var generatorOutFlags = []string{"-out", "-out-components", "-out-profile", "-out-density", "-out-sequence"}

// By AST, so a path or flag inside a comment or string (like this file's) never counts as a call.
func generatorCallArgs(f *ast.File) []map[string]bool {
	var calls []map[string]bool
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Command" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
			return true
		}
		lits := map[string]bool{}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if v, err := strconv.Unquote(lit.Value); err == nil {
				lits[v] = true
			}
		}
		if lits["./cmd/gen_design_tokens"] {
			calls = append(calls, lits)
		}
		return true
	})
	return calls
}

// Every -out* defaults to a tracked file, so a test that omits one lets the generator overwrite it, and a rejected
// malformed input once left those defaults deleted, breaking the next build. Redirect every output.
func TestGeneratorInvocationsRedirectAllOutputs(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	fset := token.NewFileSet()
	seen := 0
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, args := range generatorCallArgs(f) {
			seen++
			for _, flag := range generatorOutFlags {
				if !args[flag] {
					t.Errorf("%s: gen_design_tokens invocation omits %q, so the generator writes the real tracked file that flag defaults to; redirect it into t.TempDir()", path, flag)
				}
			}
		}
	}

	if seen == 0 {
		t.Fatal("found no ./cmd/gen_design_tokens invocations — this scanner has gone stale and is guarding nothing")
	}
}
