package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"unicode"
)

// i18nCleanFiles are source files whose user-facing labels are fully migrated to
// the i18n catalog. This ratchet keeps them clean: a new bare English label
// literal here fails the build — wrap it in i18n.T(i18n.KeyXxx) instead. Add a
// file to this list once its chrome strings are migrated.
var i18nCleanFiles = []string{
	"drumview_context_menu.go",
	"audio_panel_legend.go",
	"plain_english.go",
}

// i18nDriftAllowlist holds exact label literals intentionally left un-translated
// in a clean file (justify each). Prefer empty.
var i18nDriftAllowlist = map[string]bool{}

func hasLetterLongerThan1(s string) bool {
	if len([]rune(s)) <= 1 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func TestNoBareLabelLiteralsInCleanFiles(t *testing.T) {
	watchedCalls := map[string]bool{
		"DrawTextStyled": true, "DrawTextColorAt": true, "DrawTextColorAtScale": true,
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, f := range i18nCleanFiles {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		node, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			// label: "literal" in composite literals
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "label" {
					if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						val := strings.Trim(lit.Value, "`\"")
						if hasLetterLongerThan1(val) && !i18nDriftAllowlist[val] {
							offenders = append(offenders, f+": label: "+lit.Value)
						}
					}
				}
			}
			// DrawText*(dst, "literal", ...)
			if call, ok := n.(*ast.CallExpr); ok {
				name := ""
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if watchedCalls[name] && len(call.Args) >= 2 {
					if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						val := strings.Trim(lit.Value, "`\"")
						if hasLetterLongerThan1(val) && !i18nDriftAllowlist[val] {
							offenders = append(offenders, f+": "+name+"("+lit.Value+")")
						}
					}
				}
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("bare user-facing literals in i18n-clean files (use i18n.T or add to allowlist with justification):\n%s",
			strings.Join(offenders, "\n"))
	}
}
