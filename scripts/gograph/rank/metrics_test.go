package rank

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseFunc parses src (a file body without the package clause) and returns
// the named function declaration.
func parseFunc(t *testing.T, src, name string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package x\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fset, fd
		}
	}
	t.Fatalf("func %s not found", name)
	return nil, nil
}

// The two worked examples from the Sonar cognitive-complexity white paper,
// transliterated to Go.
func TestCognitiveSonarExamples(t *testing.T) {
	_, fd := parseFunc(t, `
func sumOfPrimes(max int) int {
	total := 0
OUT:
	for i := 1; i <= max; i++ { // +1
		for j := 2; j < i; j++ { // +2 (nesting 1)
			if i%j == 0 { // +3 (nesting 2)
				continue OUT // +1
			}
		}
		total += i
	}
	return total
}`, "sumOfPrimes")
	if got, _ := Cognitive(fd.Body, "sumOfPrimes"); got != 7 {
		t.Errorf("sumOfPrimes cognitive = %d, want 7", got)
	}

	_, fd = parseFunc(t, `
func getWords(n int) string {
	switch n { // +1
	case 1:
		return "one"
	case 2:
		return "a couple"
	case 3:
		return "a few"
	default:
		return "lots"
	}
}`, "getWords")
	if got, _ := Cognitive(fd.Body, "getWords"); got != 1 {
		t.Errorf("getWords cognitive = %d, want 1", got)
	}
}

func TestCognitiveBooleanSequencesElseAndRecursion(t *testing.T) {
	_, fd := parseFunc(t, `
func f(a, b, c, d bool, n int) int {
	if a && b && c { // if +1, one && run +1
		return 1
	} else if a || b && c { // else-if +1, || then && = 2 runs +2
		return 2
	} else { // +1
		if d { // +2 (nesting 1)
			return f(a, b, c, d, n-1) // recursion +1
		}
	}
	return 0
}`, "f")
	got, bonus := Cognitive(fd.Body, "f")
	if got != 9 {
		t.Errorf("cognitive = %d, want 9", got)
	}
	if bonus != 1 {
		t.Errorf("nesting bonus = %d, want 1", bonus)
	}
}

// A closure is its own unit: its control flow must not count toward the parent.
func TestClosureBodiesExcludedFromParent(t *testing.T) {
	_, fd := parseFunc(t, `
func reg(xs []int) {
	register(func(v int) int {
		if v > 0 {
			for i := 0; i < v; i++ {
				if i == 3 && v > 9 {
					return i
				}
			}
		}
		return 0
	})
	if len(xs) == 0 {
		return
	}
}
func register(func(int) int) {}`, "reg")
	if got, _ := Cognitive(fd.Body, "reg"); got != 1 {
		t.Errorf("parent cognitive = %d, want 1 (closure excluded)", got)
	}
	if got := Cyclomatic(fd.Body); got != 2 {
		t.Errorf("parent cyclo = %d, want 2 (closure excluded)", got)
	}
	if got := MaxNesting(fd.Body); got != 1 {
		t.Errorf("parent nesting = %d, want 1", got)
	}
}

func TestCyclomaticAndNestingMatchLegacyHelper(t *testing.T) {
	_, fd := parseFunc(t, `
func Helper(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				x += i
			}
		}
	}
	return x
}`, "Helper")
	if got := Cyclomatic(fd.Body); got != 4 {
		t.Errorf("cyclo = %d, want 4", got)
	}
	if got := MaxNesting(fd.Body); got != 3 {
		t.Errorf("nesting = %d, want 3", got)
	}
}

func TestDataRatioFlagsTables(t *testing.T) {
	fset, fd := parseFunc(t, `
type spec struct{ name string; n int }
func table() []spec {
	return []spec{
		{name: "a", n: 1},
		{name: "b", n: 2},
		{name: "c", n: 3},
		{name: "d", n: 4},
		{name: "e", n: 5},
		{name: "f", n: 6},
	}
}`, "table")
	if got := DataRatio(fset, fd.Body); got < 0.7 {
		t.Errorf("table DataRatio = %.2f, want >= 0.7", got)
	}
	fset, fd = parseFunc(t, `
func logic(x int) int {
	if x > 0 {
		x++
	}
	y := []int{x}
	for range y {
		x--
	}
	return x
}`, "logic")
	if got := DataRatio(fset, fd.Body); got > 0.3 {
		t.Errorf("logic DataRatio = %.2f, want <= 0.3", got)
	}
}

func TestSLOCSkipsBlanksAndComments(t *testing.T) {
	src := []byte("func f() {\n\t// a comment\n\n\tx := 1 // trailing\n\t/* block\n\t   still */\n\t_ = x\n}\n")
	if got := SLOC(src, 1, 8); got != 4 {
		t.Errorf("SLOC = %d, want 4 (func, x:=, _=x, })", got)
	}
}
