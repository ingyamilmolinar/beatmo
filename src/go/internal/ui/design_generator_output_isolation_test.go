package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// generatorOutFlags is every output flag cmd/gen_design_tokens accepts. Each
// one DEFAULTS to a real, tracked, generated file (see the flag.String block
// in cmd/gen_design_tokens/main.go):
//
//	-out            -> internal/ui/design_tokens.gen.go
//	-out-components -> internal/ui/design_components.gen.go
//	-out-profile    -> internal/ui/design_profile.gen.go
//	-out-density    -> internal/ui/design_density.gen.go
//	-out-sequence   -> internal/templates/instrument_sequence.gen.go
var generatorOutFlags = []string{"-out", "-out-components", "-out-profile", "-out-density", "-out-sequence"}

// generatorCallArgs returns, for every exec.Command(...) call in the parsed
// file that invokes ./cmd/gen_design_tokens, the set of string-literal
// arguments passed to it.
//
// This walks the AST rather than grepping the source so that a generator path
// or flag name appearing inside a comment or a string literal (such as the
// ones in this very file) can never be mistaken for a real invocation.
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

// TestGeneratorInvocationsRedirectAllOutputs guards the developer's working
// tree against the design-token generator tests.
//
// Every -out* flag defaults to a REAL tracked file, so a test that omits one
// does not merely "use a default" — it hands the generator write access to a
// checked-in generated file. That is destructive in both directions:
//
//   - TestDesignTokensFreshness omitted -out-sequence and so regenerated the
//     real internal/templates/instrument_sequence.gen.go on every run.
//   - TestGeneratorRejectsSchemaViolations omitted -out-density AND
//     -out-sequence while feeding the generator a DELIBERATELY MALFORMED
//     DESIGN.md. The generator rejects that input (which is the point of the
//     test) and on the reject path leaves the two default-path outputs
//     DELETED — so a PASSING test silently removed
//     src/go/internal/ui/design_density.gen.go and
//     src/go/internal/templates/instrument_sequence.gen.go from the working
//     tree, breaking the next build with "undefined: densityValues".
//
// A test must never write outside its t.TempDir(). Redirect every output,
// including the ones it does not assert on.
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
