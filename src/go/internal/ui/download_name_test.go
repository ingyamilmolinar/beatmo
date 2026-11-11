package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// stampedNameRe matches "<slug>-YYYY-MM-DD_HHMMSS<ext>".
var stampedNameRe = regexp.MustCompile(`^[a-z0-9-]+-\d{4}-\d{2}-\d{2}_\d{6}\.[a-z.-]+$`)

// browserUniquifiers are the suffixes a download manager invents when the page
// asks for a name that already exists in the Downloads folder. Chrome writes
// " (1)", Firefox "(1)", Safari " 2" — the page cannot pin which one it gets,
// so a name that can collide is a name that renders differently per browser.
var browserUniquifiers = []string{" (1)", "(1)", " 2"}

func assertCollisionFreeName(t *testing.T, got string) {
	t.Helper()
	if !stampedNameRe.MatchString(got) {
		t.Errorf("download name %q is not timestamp-stamped; a constant name collides in Downloads and each browser spells the uniquifier differently", got)
	}
	for _, u := range browserUniquifiers {
		if strings.Contains(got, u) {
			t.Errorf("download name %q carries a browser uniquifier %q", got, u)
		}
	}
}

func TestStampedFileNameIsUniquePerSecond(t *testing.T) {
	base := time.Date(2026, 9, 13, 14, 15, 30, 0, time.UTC)
	restore := SetDownloadClockForTest(func() time.Time { return base })
	got1 := stampedFileName("beatmo-export", ".json")
	restore()
	if got1 != "beatmo-export-2026-09-13_141530.json" {
		t.Fatalf("stampedFileName = %q, want beatmo-export-2026-09-13_141530.json", got1)
	}
	restore = SetDownloadClockForTest(func() time.Time { return base.Add(time.Second) })
	got2 := stampedFileName("beatmo-export", ".json")
	restore()
	if got1 == got2 {
		t.Errorf("two saves produced the same name %q; the second collides in Downloads", got1)
	}
}

// TestExportDownloadNameNeverCollides drives the real Export path twice and
// asserts each save asks for its own filename, so the browser's collision
// resolver never runs and Chrome/Firefox/Safari all land the same name.
func TestExportDownloadNameNeverCollides(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)

	var names []string
	restoreSave := SetSaveJSONForTest(func(name string, data []byte) error {
		names = append(names, name)
		return nil
	})
	defer restoreSave()

	now := time.Date(2026, 9, 13, 14, 15, 30, 0, time.UTC)
	restoreClock := SetDownloadClockForTest(func() time.Time { return now })
	defer restoreClock()

	if err := g.drum.Export(); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	now = now.Add(time.Second)
	if err := g.drum.Export(); err != nil {
		t.Fatalf("second export failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("want 2 saves, got %d (%v)", len(names), names)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "beatmo-export-") || !strings.HasSuffix(n, ".json") {
			t.Errorf("export name = %q, want beatmo-export-<stamp>.json", n)
		}
		assertCollisionFreeName(t, n)
	}
	if names[0] == names[1] {
		t.Errorf("both exports asked for %q; the second one collides and the browser renames it", names[0])
	}
}

// TestEveryDownloadNameIsStamped is the discipline guard: a future export site
// that hands saveJSON a literal name reintroduces the collision, and the bug is
// invisible in tests because it only shows up on the *second* save, in the
// user's real Downloads folder, spelled differently per browser. Every call
// must route its name through a stamping helper.
func TestEveryDownloadNameIsStamped(t *testing.T) {
	stampers := map[string]bool{
		"stampedFileName":    true,
		"instrumentFileName": true, // stamps internally
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	checked := 0
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			continue // platform-tagged files that don't parse under this GOOS
		}
		ast.Inspect(f, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || id.Name != "saveJSON" || len(call.Args) == 0 {
				return true
			}
			checked++
			pos := fset.Position(call.Pos())
			switch arg := call.Args[0].(type) {
			case *ast.CallExpr:
				if fid, ok := arg.Fun.(*ast.Ident); ok && stampers[fid.Name] {
					return true
				}
			case *ast.Ident:
				// A local variable: require it to be assigned from a stamper in
				// the same file (cheap textual check, kept honest by the two
				// behavioural tests above).
				src, err := os.ReadFile(n)
				if err == nil && strings.Contains(string(src), arg.Name+" := stampedFileName(") {
					return true
				}
			}
			t.Errorf("%s: saveJSON name is not stamped; route it through stampedFileName so the second save cannot collide in Downloads", pos)
			return true
		})
	}
	if checked == 0 {
		t.Fatal("found no saveJSON call sites — the guard is no longer scanning anything")
	}
}
