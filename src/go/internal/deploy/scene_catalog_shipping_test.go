package deploy

// The screenshot scene catalog is dev tooling: make wasm (what ships) builds with noscenes, make wasm-dev keeps it for
// test-browser and the screenshot targets. These keep that true when a scene file or the Makefile recipe changes.

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sceneCatalogMarker is a string only the scene catalog's data holds (its
// first scene's name); TestWasmAssetSizeBudget uses it to recognise a
// `make wasm-dev` binary. TestSceneCatalogMarkerIsTheCatalogs pins it.
const sceneCatalogMarker = "transport_idle"

// shippedWasmTags reads the tags `make wasm` builds with.
func shippedWasmTags(t *testing.T, root string) []string {
	t.Helper()
	mk := mustRead(t, filepath.Join(root, "Makefile"))
	recipe := regexp.MustCompile(`(?m)^wasm:.*\n(?:\t.*\n)*`).FindString(mk)
	if !strings.Contains(recipe, "-tags $(WASM_RELEASE_TAGS)") {
		t.Fatalf("the Makefile `wasm` recipe no longer builds with -tags $(WASM_RELEASE_TAGS):\n%s", recipe)
	}
	m := regexp.MustCompile(`(?m)^WASM_RELEASE_TAGS\s*=\s*(.*)$`).FindStringSubmatch(mk)
	if m == nil {
		t.Fatal("Makefile defines no WASM_RELEASE_TAGS")
	}
	return strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' })
}

// sceneLiteralFiles returns the internal/ui files a js/wasm build with tags
// compiles that contain a Scene{...} composite literal (a scene definition).
func sceneLiteralFiles(t *testing.T, uiDir string, tags []string) []string {
	t.Helper()
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.BuildTags = "js", "wasm", tags
	pkg, err := ctx.ImportDir(uiDir, 0)
	if err != nil {
		t.Fatalf("import %s: %v", uiDir, err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, name := range pkg.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(uiDir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok {
				if id, ok := cl.Type.(*ast.Ident); ok && id.Name == "Scene" {
					found = true
				}
				if at, ok := cl.Type.(*ast.ArrayType); ok {
					if id, ok := at.Elt.(*ast.Ident); ok && id.Name == "Scene" {
						found = true
					}
				}
			}
			return !found
		})
		if found {
			out = append(out, name)
		}
	}
	return out
}

func TestShippedWasmLeavesOutTheSceneCatalog(t *testing.T) {
	root := repoRoot(t)
	uiDir := filepath.Join(root, "src", "go", "internal", "ui")

	if got := sceneLiteralFiles(t, uiDir, shippedWasmTags(t, root)); len(got) > 0 {
		t.Errorf("the shipped WASM (`make wasm`) compiles scene definitions from %v -- "+
			"give each file `//go:build !noscenes` so the screenshot catalog stays out of production", got)
	}
	// And the guard is not vacuous: a dev build does compile the catalog.
	if got := sceneLiteralFiles(t, uiDir, nil); len(got) == 0 {
		t.Error("a js/wasm build without -tags noscenes compiles no Scene literal -- the scan is broken")
	}
}

func TestSceneCatalogMarkerIsTheCatalogs(t *testing.T) {
	root := repoRoot(t)
	uiDir := filepath.Join(root, "src", "go", "internal", "ui")
	needle := `"` + sceneCatalogMarker + `"`
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.BuildTags = "js", "wasm", shippedWasmTags(t, root)
	shipped, err := ctx.ImportDir(uiDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	inShipped := map[string]bool{}
	for _, n := range shipped.GoFiles {
		inShipped[n] = true
	}
	files, _ := filepath.Glob(filepath.Join(uiDir, "*.go"))
	var holders []string
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") || !strings.Contains(mustRead(t, p), needle) {
			continue
		}
		name := filepath.Base(p)
		if inShipped[name] {
			t.Errorf("%s is compiled into the shipped WASM and holds %s, so TestWasmAssetSizeBudget "+
				"would mistake every shipped build for a dev build -- pick another marker", name, needle)
		}
		holders = append(holders, name)
	}
	if len(holders) == 0 {
		t.Errorf("no scene catalog file holds %s any more -- update sceneCatalogMarker", needle)
	}
}
