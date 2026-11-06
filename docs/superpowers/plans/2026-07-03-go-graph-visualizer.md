# go-graph Visualizer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `scripts/gograph`, a tool that emits a self-contained interactive HTML visualization of the beatmo Go program — packages, types, and functions sized by LOC, annotated with nesting depth / cyclomatic complexity / call-graph depth, and connected by type-resolved cross-package call edges.

**Architecture:** A standalone Go command in its own module (isolating the `golang.org/x/tools` dependency from `src/go`). Two analysis passes — an AST metrics pass and a `go/packages` type-resolved call-edge pass — feed one JSON model, which is embedded into a hand-rolled offline HTML+SVG+JS renderer. Invoked via `make go-graph`.

**Tech Stack:** Go 1.23, `go/ast`/`go/parser`/`go/token` (metrics), `golang.org/x/tools/go/packages` v0.25.0 (type resolution), `text/template` + `go:embed` (HTML), vanilla JS + inline SVG (renderer).

## Global Constraints

- **Go toolchain:** use the bundled `.tools/go/bin/go` for every Go command (avoids version drift). Version floor: Go 1.23.
- **Dependency isolation:** `golang.org/x/tools` MUST live in `scripts/gograph/go.mod` ONLY. It must NOT be added to `src/go/go.mod` (WASM-bloat sensitivity). Pin `golang.org/x/tools v0.25.0` (already in the module cache — builds offline).
- **Module under analysis:** `github.com/ingyamilmolinar/beatmo`, source root `src/go`.
- **Default build config for call resolution:** `GOOS=linux`, no build tags (native desktop). A `-goos`/`-tags` flag overrides. Packages that fail to type-check under the chosen config are logged to stderr and skipped (their AST metrics still appear).
- **Self-contained output:** `build/go-graph.html` must open offline with NO network/CDN references. No d3/React/vendored blobs. All CSS/JS inline; JSON model inline.
- **No git commits:** per the user's standing instruction, do NOT run `git add` or `git commit`. Every task's final "Checkpoint" step means *stop and let the reviewer gate the task* — leave all files uncommitted.
- **Analysis exclusions:** skip `*_test.go`, and any path containing `/vendor/` or `/testdata/`.
- **Canonical `FuncID` format** (used by BOTH analysis passes so metrics and call-depth join correctly): `pkgPath + "\t" + recv + "\t" + name`, where `recv` is the receiver type name with any leading `*` stripped (empty for free functions).

---

### Task 1: Scaffold tool module, JSON model, and test fixture

**Files:**
- Create: `scripts/gograph/go.mod`
- Create: `scripts/gograph/model.go`
- Create: `scripts/gograph/model_test.go`
- Create: `scripts/gograph/testdata/fixturemod/go.mod`
- Create: `scripts/gograph/testdata/fixturemod/foo/foo.go`
- Create: `scripts/gograph/testdata/fixturemod/bar/bar.go`

**Interfaces:**
- Produces: the model types `Config`, `FuncMetric`, `TypeInfo`, `Package`, `Edge`, `Graph`, and the helper `func FuncID(pkgPath, recv, name string) string`. All later tasks consume these.

- [ ] **Step 1: Create the tool module file**

`scripts/gograph/go.mod`:
```
module github.com/ingyamilmolinar/beatmo/scripts/gograph

go 1.23

require golang.org/x/tools v0.25.0
```

- [ ] **Step 2: Create the test fixture module (analysis target for all later tests)**

`scripts/gograph/testdata/fixturemod/go.mod`:
```
module example.com/fixture

go 1.23
```

`scripts/gograph/testdata/fixturemod/foo/foo.go`:
```go
package foo

// Thing is an exported type used across packages.
type Thing struct{ N int }

// Process is an exported method; call sites on an imported Thing value must
// resolve to this package (the AST-only case that type resolution catches).
func (t Thing) Process() int { return t.N * 2 }

// Helper is a free exported function.
func Helper(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				x += i
			}
		}
	}
	return x
}
```

`scripts/gograph/testdata/fixturemod/bar/bar.go`:
```go
package bar

import "example.com/fixture/foo"

// Run calls both a free function and a method on an imported type.
func Run() int {
	t := foo.Thing{N: 3}
	return foo.Helper(t.Process())
}
```

- [ ] **Step 3: Write the failing test for the model + FuncID helper**

`scripts/gograph/model_test.go`:
```go
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFuncIDFormat(t *testing.T) {
	got := FuncID("internal/audio", "Engine", "Process")
	want := "internal/audio\tEngine\tProcess"
	if got != want {
		t.Fatalf("FuncID = %q, want %q", got, want)
	}
	free := FuncID("internal/ui", "", "NewDrumView")
	if strings.Count(free, "\t") != 2 {
		t.Fatalf("FuncID must always have two tabs, got %q", free)
	}
}

func TestGraphJSONRoundTrip(t *testing.T) {
	g := Graph{
		Module: "m",
		Config: map[string]string{"goos": "linux", "tags": ""},
		Packages: []Package{{
			Path: "p", LOC: 10, Files: 1,
			Funcs: []FuncMetric{{Name: "F", LOC: 5, Nesting: 2, Cyclo: 3, CallDepth: 1}},
		}},
		Edges: []Edge{{From: "a", To: "b", Calls: 2, Methods: []string{"b.X"}}},
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var back Graph
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Packages[0].Funcs[0].Cyclo != 3 || back.Edges[0].Calls != 2 {
		t.Fatalf("round trip lost data: %+v", back)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run 'TestFuncIDFormat|TestGraphJSONRoundTrip'`
Expected: FAIL — `undefined: FuncID`, `undefined: Graph`, etc.

- [ ] **Step 5: Write the model**

`scripts/gograph/model.go`:
```go
// Command gograph analyzes the beatmo Go module and emits a self-contained
// interactive HTML visualization of packages, types, and functions — sized by
// LOC and annotated with nesting depth, cyclomatic complexity, call-graph
// depth, and type-resolved cross-package call edges.
//
// Flags:
//   -root   path to the module source root (default ../../src/go)
//   -module module import path (default github.com/ingyamilmolinar/beatmo)
//   -goos   GOOS for call resolution (default linux). Use "js" to include the
//           wasm-only files (js_exports_*, etc.) omitted by the desktop config.
//   -tags   comma-separated build tags for call resolution (default none)
//   -out    output HTML path (default ../../build/go-graph.html)
//   -json   optional path to also write the raw JSON model
package main

// Config controls the call-resolution build environment.
type Config struct {
	Root   string
	Module string
	GOOS   string
	Tags   string
}

// FuncMetric holds per-function/method metrics.
type FuncMetric struct {
	Name      string `json:"name"`
	Recv      string `json:"recv,omitempty"` // receiver type name (no *), "" for free funcs
	LOC       int    `json:"loc"`
	Nesting   int    `json:"nesting"`
	Cyclo     int    `json:"cyclo"`
	CallDepth int    `json:"callDepth"`
}

// TypeInfo holds a declared type and its methods.
type TypeInfo struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"` // struct | interface | named
	LOC     int          `json:"loc"`
	Methods []FuncMetric `json:"methods,omitempty"`
}

// Package is one in-module package.
type Package struct {
	Path    string       `json:"path"`
	LOC     int          `json:"loc"`
	Files   int          `json:"files"`
	Types   []TypeInfo   `json:"types,omitempty"`
	Funcs   []FuncMetric `json:"funcs,omitempty"`
	Imports []string     `json:"imports,omitempty"`
}

// Edge is an aggregated cross-package call relationship.
type Edge struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Calls   int      `json:"calls"`
	Methods []string `json:"methods,omitempty"`
}

// Graph is the complete model emitted to JSON and embedded in the HTML.
type Graph struct {
	Module   string            `json:"module"`
	Config   map[string]string `json:"config"`
	Packages []Package         `json:"packages"`
	Edges    []Edge            `json:"edges"`
}

// FuncID is the canonical key joining the metrics pass and the call-depth pass.
// recv must have any leading '*' already stripped; "" for free functions.
func FuncID(pkgPath, recv, name string) string {
	return pkgPath + "\t" + recv + "\t" + name
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run 'TestFuncIDFormat|TestGraphJSONRoundTrip'`
Expected: PASS (both tests). Note: `go.mod` `require` resolves offline from the module cache.

- [ ] **Step 7: Checkpoint (no commit — reviewer gate)**

Leave files uncommitted. Confirm `cd scripts/gograph && ../../.tools/go/bin/go vet ./...` is clean.

---

### Task 2: AST metrics pass (LOC, nesting depth, cyclomatic complexity)

**Files:**
- Create: `scripts/gograph/metrics.go`
- Create: `scripts/gograph/metrics_test.go`

**Interfaces:**
- Consumes: model types from Task 1.
- Produces: `func AnalyzeMetrics(root, module string) (map[string]*Package, error)` — returns packages keyed by import path, with `LOC`, `Files`, `Types`, `Funcs`, `Imports` filled (metrics fields set; `CallDepth` left 0 for Task 4). Also exposes `func maxNest(stmts []ast.Stmt) int` and `func cyclomatic(fn *ast.FuncDecl) int` for direct unit testing.

- [ ] **Step 1: Write the failing tests against the fixture module**

`scripts/gograph/metrics_test.go`:
```go
package main

import "testing"

func TestAnalyzeMetricsFixture(t *testing.T) {
	pkgs, err := AnalyzeMetrics("testdata/fixturemod", "example.com/fixture")
	if err != nil {
		t.Fatal(err)
	}
	foo := pkgs["example.com/fixture/foo"]
	if foo == nil {
		t.Fatalf("foo package not found; got keys %v", keys(pkgs))
	}

	// Thing type present, is a struct, has the Process method.
	var thing *TypeInfo
	for i := range foo.Types {
		if foo.Types[i].Name == "Thing" {
			thing = &foo.Types[i]
		}
	}
	if thing == nil || thing.Kind != "struct" {
		t.Fatalf("Thing struct not modeled: %+v", foo.Types)
	}
	if len(thing.Methods) != 1 || thing.Methods[0].Name != "Process" {
		t.Fatalf("Process method not attached to Thing: %+v", thing.Methods)
	}
	if thing.Methods[0].Recv != "Thing" {
		t.Fatalf("Process recv = %q, want Thing", thing.Methods[0].Recv)
	}

	// Helper free func: nesting depth 3 (if > for > if), cyclo 4 (1 + 2 ifs + 1 for).
	var helper *FuncMetric
	for i := range foo.Funcs {
		if foo.Funcs[i].Name == "Helper" {
			helper = &foo.Funcs[i]
		}
	}
	if helper == nil {
		t.Fatalf("Helper func not found: %+v", foo.Funcs)
	}
	if helper.Nesting != 3 {
		t.Errorf("Helper nesting = %d, want 3", helper.Nesting)
	}
	if helper.Cyclo != 4 {
		t.Errorf("Helper cyclo = %d, want 4", helper.Cyclo)
	}
	if helper.LOC < 5 {
		t.Errorf("Helper LOC = %d, want >=5", helper.LOC)
	}

	// bar imports foo.
	bar := pkgs["example.com/fixture/bar"]
	if bar == nil || !contains(bar.Imports, "example.com/fixture/foo") {
		t.Fatalf("bar should import foo: %+v", bar)
	}
}

func keys(m map[string]*Package) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
```

Note: `contains` is a NON-test helper (defined in `metrics.go`, Step 3) because
production code in `metrics.go` and `callgraph.go` (Task 3) both call it. Do NOT
define `contains` in this test file — a test-only definition compiles under
`go test` but breaks a real `go build`/`go run` with `undefined: contains`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestAnalyzeMetricsFixture`
Expected: FAIL — `undefined: AnalyzeMetrics`.

- [ ] **Step 3: Implement the metrics pass**

`scripts/gograph/metrics.go`:
```go
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// AnalyzeMetrics walks every non-test .go file under root, groups declarations
// by package import path (module + relative dir), and computes LOC, nesting
// depth, and cyclomatic complexity. It does no type checking.
func AnalyzeMetrics(root, module string) (map[string]*Package, error) {
	fset := token.NewFileSet()
	pkgs := map[string]*Package{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil // tolerate unparsable files (e.g. build-constrained syntax)
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkgPath := module
		if rel != "." {
			pkgPath = module + "/" + filepath.ToSlash(rel)
		}
		p := pkgs[pkgPath]
		if p == nil {
			p = &Package{Path: pkgPath}
			pkgs[pkgPath] = p
		}
		p.Files++
		collectFile(fset, file, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Re-nest cross-file methods (receiver type declared in a different file of
	// the same package) under their type now that every file is collected.
	for _, p := range pkgs {
		renestMethods(p)
	}
	// Sort nothing else here; model assembly (Task 4) orders for determinism.
	return pkgs, nil
}

// renestMethods moves any package-level func that is really a method (non-empty
// Recv) — stashed in p.Funcs by collectFile because its receiver type was
// declared in a different file — under its receiver type. Runs after all files
// in the package are collected, so p.Types is complete. Only touches p.Funcs
// entries, so same-file methods (already attached) are not double-counted.
func renestMethods(p *Package) {
	if len(p.Funcs) == 0 {
		return
	}
	typeIdx := map[string]*TypeInfo{}
	for i := range p.Types {
		typeIdx[p.Types[i].Name] = &p.Types[i]
	}
	kept := p.Funcs[:0]
	for _, f := range p.Funcs {
		if f.Recv != "" {
			if ti := typeIdx[f.Recv]; ti != nil {
				ti.Methods = append(ti.Methods, f)
				continue
			}
		}
		kept = append(kept, f)
	}
	p.Funcs = kept
}

// collectFile appends the file's imports, types, funcs, and methods to p and
// adds the file's LOC to the package total.
func collectFile(fset *token.FileSet, file *ast.File, p *Package) {
	// imports
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !contains(p.Imports, path) {
			p.Imports = append(p.Imports, path)
		}
	}
	// index type specs so methods can attach to their receiver
	typeIdx := map[string]*TypeInfo{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			ti := TypeInfo{Name: ts.Name.Name, Kind: typeKind(ts.Type), LOC: locOf(fset, ts)}
			p.Types = append(p.Types, ti)
			typeIdx[ts.Name.Name] = &p.Types[len(p.Types)-1]
			p.LOC += ti.LOC
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fm := FuncMetric{
			Name:    fn.Name.Name,
			LOC:     locOf(fset, fn),
			Nesting: bodyNest(fn),
			Cyclo:   cyclomatic(fn),
		}
		p.LOC += fm.LOC
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv := recvName(fn.Recv.List[0].Type)
			fm.Recv = recv
			if ti := typeIdx[recv]; ti != nil {
				ti.Methods = append(ti.Methods, fm)
			} else {
				// receiver type declared in another file of the same package:
				// stash as a package-level func carrying its Recv so Task 4 can
				// still resolve its FuncID and CallDepth.
				p.Funcs = append(p.Funcs, fm)
			}
		} else {
			p.Funcs = append(p.Funcs, fm)
		}
	}
}

func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "named"
	}
}

func recvName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr: // generic receiver Foo[T]
		return recvName(x.X)
	case *ast.IndexListExpr:
		return recvName(x.X)
	default:
		return ""
	}
}

func locOf(fset *token.FileSet, n ast.Node) int {
	start := fset.Position(n.Pos()).Line
	end := fset.Position(n.End()).Line
	if end < start {
		return 1
	}
	return end - start + 1
}

// bodyNest returns the deepest control-flow nesting inside a function body.
func bodyNest(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 0
	}
	return maxNest(fn.Body.List)
}

func maxNest(stmts []ast.Stmt) int {
	best := 0
	for _, s := range stmts {
		if d := stmtNest(s); d > best {
			best = d
		}
	}
	return best
}

func stmtNest(s ast.Stmt) int {
	switch n := s.(type) {
	case *ast.BlockStmt:
		return maxNest(n.List)
	case *ast.IfStmt:
		d := 1 + maxNest(n.Body.List)
		if n.Else != nil {
			var ed int
			if b, ok := n.Else.(*ast.BlockStmt); ok {
				ed = 1 + maxNest(b.List)
			} else {
				ed = stmtNest(n.Else) // else-if chain
			}
			if ed > d {
				d = ed
			}
		}
		return d
	case *ast.ForStmt:
		return 1 + maxNest(n.Body.List)
	case *ast.RangeStmt:
		return 1 + maxNest(n.Body.List)
	case *ast.SwitchStmt:
		return 1 + caseNest(n.Body)
	case *ast.TypeSwitchStmt:
		return 1 + caseNest(n.Body)
	case *ast.SelectStmt:
		return 1 + caseNest(n.Body)
	case *ast.LabeledStmt:
		return stmtNest(n.Stmt)
	default:
		return 0
	}
}

func caseNest(body *ast.BlockStmt) int {
	best := 0
	for _, c := range body.List {
		var inner []ast.Stmt
		switch cc := c.(type) {
		case *ast.CaseClause:
			inner = cc.Body
		case *ast.CommClause:
			inner = cc.Body
		}
		if d := maxNest(inner); d > best {
			best = d
		}
	}
	return best
}

// cyclomatic is McCabe complexity: 1 + branch/loop/case/&&/|| count.
func cyclomatic(fn *ast.FuncDecl) int {
	cc := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			cc++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				cc++
			}
		}
		return true
	})
	return cc
}

// contains reports whether s is in xs. NON-test helper: production code in this
// file and in callgraph.go (Task 3) depend on it, so it must live in a non-test
// file (not metrics_test.go).
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestAnalyzeMetricsFixture`
Expected: PASS.

- [ ] **Step 5: Sanity-run against the real module**

Run: `cd scripts/gograph && ../../.tools/go/bin/go run . -help 2>&1 | head -1 || true`
(The binary has no main yet; this just confirms compilation. Real run happens in Task 5.) Instead verify with a throwaway: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestAnalyzeMetricsFixture -v` shows the fixture metrics.

- [ ] **Step 6: Checkpoint (no commit — reviewer gate)**

`../../.tools/go/bin/go vet ./...` clean.

---

### Task 3: Type-resolved call-edge pass + call-graph depth

**Files:**
- Create: `scripts/gograph/callgraph.go`
- Create: `scripts/gograph/callgraph_test.go`

**Interfaces:**
- Consumes: model types (Task 1); `FuncID` (Task 1).
- Produces: `func ResolveEdges(cfg Config) (edges []Edge, callDepth map[string]int, err error)` — `edges` are aggregated exported cross-package calls; `callDepth` maps `FuncID` → longest downstream in-module call chain length (>=1). Also `func funcIDFromObj(obj types.Object) (id string, ok bool)` for reuse/testing.

- [ ] **Step 1: Write the failing test against the fixture**

`scripts/gograph/callgraph_test.go`:
```go
package main

import "testing"

func TestResolveEdgesFixture(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	edges, depth, err := ResolveEdges(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// bar -> foo must exist, and MUST include the method-on-imported-type call
	// foo.Thing.Process (the case AST-only analysis would miss) plus foo.Helper.
	var barFoo *Edge
	for i := range edges {
		if edges[i].From == "example.com/fixture/bar" && edges[i].To == "example.com/fixture/foo" {
			barFoo = &edges[i]
		}
	}
	if barFoo == nil {
		t.Fatalf("expected bar->foo edge, got %+v", edges)
	}
	if barFoo.Calls < 2 {
		t.Errorf("bar->foo calls = %d, want >=2 (Helper + Process)", barFoo.Calls)
	}
	if !hasMethodLike(barFoo.Methods, "Process") {
		t.Errorf("bar->foo methods missing Process (method-on-imported-type): %v", barFoo.Methods)
	}
	if !hasMethodLike(barFoo.Methods, "Helper") {
		t.Errorf("bar->foo methods missing Helper: %v", barFoo.Methods)
	}

	// No self-edge (same-package calls excluded).
	for _, e := range edges {
		if e.From == e.To {
			t.Errorf("unexpected self-edge %+v", e)
		}
	}

	// Run() calls into foo, so its call depth is >=2.
	runID := FuncID("example.com/fixture/bar", "", "Run")
	if depth[runID] < 2 {
		t.Errorf("Run callDepth = %d, want >=2", depth[runID])
	}
}

func hasMethodLike(xs []string, sub string) bool {
	for _, x := range xs {
		if len(x) >= len(sub) && (x == sub || containsSub(x, sub)) {
			return true
		}
	}
	return false
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestResolveEdgesFixture`
Expected: FAIL — `undefined: ResolveEdges`.

- [ ] **Step 3: Implement the call-edge pass**

`scripts/gograph/callgraph.go`:
```go
package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ResolveEdges type-checks the module under cfg and returns aggregated exported
// cross-package call edges plus a per-function call-graph depth map.
func ResolveEdges(cfg Config) ([]Edge, map[string]int, error) {
	mode := packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	env := os.Environ()
	if cfg.GOOS != "" {
		env = append(env, "GOOS="+cfg.GOOS)
	}
	var buildFlags []string
	if cfg.Tags != "" {
		buildFlags = []string{"-tags=" + cfg.Tags}
	}
	pc := &packages.Config{
		Mode:       mode,
		Dir:        cfg.Root,
		Env:        env,
		BuildFlags: buildFlags,
		Tests:      false,
	}
	pkgs, err := packages.Load(pc, "./...")
	if err != nil {
		return nil, nil, err
	}

	// module prefix to keep only in-module callees
	prefix := cfg.Module

	type callee struct {
		id     string
		pkg    string
		method string
	}
	// caller-func-id -> set of in-module callee func-ids (for depth)
	callees := map[string]map[string]bool{}
	// aggregated package edges
	edgeAgg := map[string]*Edge{}

	for _, p := range pkgs {
		if p.TypesInfo == nil || !strings.HasPrefix(p.PkgPath, prefix) {
			continue
		}
		if len(p.Errors) > 0 {
			fmt.Fprintf(os.Stderr, "gograph: skipping call edges for %s (%d type errors)\n", p.PkgPath, len(p.Errors))
			// still allow depth for whatever resolved; continue walking
		}
		for _, f := range p.Syntax {
			curFn := ""
			ast.Inspect(f, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok {
					curFn = funcIDFromDecl(p.PkgPath, fd)
					return true
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				obj := calleeObject(p.TypesInfo, call.Fun)
				if obj == nil || obj.Pkg() == nil {
					return true
				}
				toPath := obj.Pkg().Path()
				if !strings.HasPrefix(toPath, prefix) {
					return true // out of module (stdlib / 3rd party)
				}
				id, idOK := funcIDFromObj(obj)
				if idOK && curFn != "" && toPath != p.PkgPath {
					// record for depth (only cross-checked calls we can name)
				}
				if idOK && curFn != "" {
					if callees[curFn] == nil {
						callees[curFn] = map[string]bool{}
					}
					callees[curFn][id] = true
				}
				if toPath == p.PkgPath || !obj.Exported() {
					return true // same-package or unexported: no cross edge
				}
				_ = callee{}
				key := p.PkgPath + "\x00" + toPath
				e := edgeAgg[key]
				if e == nil {
					e = &Edge{From: p.PkgPath, To: toPath}
					edgeAgg[key] = e
				}
				e.Calls++
				m := qualifiedName(obj)
				if !contains(e.Methods, m) {
					e.Methods = append(e.Methods, m)
				}
				return true
			})
		}
	}

	// longest downstream chain via memoized DFS with cycle guard
	depth := map[string]int{}
	var walk func(id string, stack map[string]bool) int
	walk = func(id string, stack map[string]bool) int {
		if d, done := depth[id]; done {
			return d
		}
		if stack[id] {
			return 1 // cycle: stop counting deeper
		}
		stack[id] = true
		best := 0
		for callee := range callees[id] {
			if d := walk(callee, stack); d > best {
				best = d
			}
		}
		delete(stack, id)
		d := 1 + best
		depth[id] = d
		return d
	}
	for id := range callees {
		walk(id, map[string]bool{})
	}

	edges := make([]Edge, 0, len(edgeAgg))
	for _, e := range edgeAgg {
		sort.Strings(e.Methods)
		edges = append(edges, *e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges, depth, nil
}

// calleeObject resolves the object being called (function or method).
func calleeObject(info *types.Info, fun ast.Expr) types.Object {
	switch e := fun.(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			return sel.Obj() // method / field call on a value: resolves recv pkg
		}
		return info.Uses[e.Sel] // qualified pkg.Func()
	}
	return nil
}

// funcIDFromObj derives the canonical FuncID for a resolved callee.
func funcIDFromObj(obj types.Object) (string, bool) {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil {
		return "", false
	}
	recv := ""
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		recv = recvTypeName(sig.Recv().Type())
	}
	return FuncID(fn.Pkg().Path(), recv, fn.Name()), true
}

func recvTypeName(t types.Type) string {
	switch x := t.(type) {
	case *types.Pointer:
		return recvTypeName(x.Elem())
	case *types.Named:
		return x.Obj().Name()
	default:
		return ""
	}
}

// funcIDFromDecl builds the FuncID for a function declaration in pkgPath.
func funcIDFromDecl(pkgPath string, fd *ast.FuncDecl) string {
	recv := ""
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv = recvName(fd.Recv.List[0].Type)
	}
	return FuncID(pkgPath, recv, fd.Name.Name)
}

// qualifiedName renders a readable exported callee name for edge labels.
func qualifiedName(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	if !ok {
		return obj.Pkg().Name() + "." + obj.Name()
	}
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		return "(" + fn.Pkg().Name() + "." + recvTypeName(sig.Recv().Type()) + ")." + fn.Name()
	}
	return fn.Pkg().Name() + "." + fn.Name()
}
```

Note: the `_ = callee{}` and the first `if idOK && curFn != "" && toPath != p.PkgPath {}` block are scaffolding kept minimal — remove the empty block and the unused `callee` type if `go vet` flags them; they exist only to make the resolution intent explicit while writing. The executor SHOULD delete the empty `if` and the `callee` struct + `_ = callee{}` line, keeping only the `callees` recording and `edgeAgg` recording. Final code must be vet-clean.

- [ ] **Step 4: Remove the scaffolding and confirm vet-clean**

Delete the empty `if idOK && curFn != "" && toPath != p.PkgPath {}` block, the `type callee struct{...}` declaration, and the `_ = callee{}` line. Re-run vet.

Run: `cd scripts/gograph && ../../.tools/go/bin/go vet ./...`
Expected: no output (clean).

- [ ] **Step 5: Run to verify the test passes**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestResolveEdgesFixture`
Expected: PASS. (First run type-checks the fixture; uses the cached `x/tools`.)

- [ ] **Step 6: Checkpoint (no commit — reviewer gate)**

---

### Task 4: Model assembly (join passes, order deterministically, emit JSON)

**Files:**
- Create: `scripts/gograph/build.go`
- Create: `scripts/gograph/build_test.go`
- Create: `scripts/gograph/testdata/fixture_golden.json`

**Interfaces:**
- Consumes: `AnalyzeMetrics` (Task 2), `ResolveEdges` (Task 3), `FuncID` (Task 1).
- Produces: `func BuildGraph(cfg Config) (*Graph, error)` — runs both passes, attaches `CallDepth` to every func/method by `FuncID`, sorts everything deterministically, returns the model. Later tasks (renderer, main) consume this.

- [ ] **Step 1: Write the failing test (golden model for the fixture)**

`scripts/gograph/build_test.go`:
```go
package main

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestBuildGraphGolden(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	g, err := BuildGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const golden = "testdata/fixture_golden.json"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run -update first): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("model mismatch.\n--- got ---\n%s", got)
	}
}

func TestBuildGraphCallDepthAttached(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	g, err := BuildGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range g.Packages {
		if p.Path == "example.com/fixture/bar" {
			for _, f := range p.Funcs {
				if f.Name == "Run" && f.CallDepth < 2 {
					t.Fatalf("Run CallDepth = %d, want >=2 (must be joined from ResolveEdges)", f.CallDepth)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestBuildGraph`
Expected: FAIL — `undefined: BuildGraph`.

- [ ] **Step 3: Implement model assembly**

`scripts/gograph/build.go`:
```go
package main

import "sort"

// BuildGraph runs both analysis passes and joins them into one ordered model.
func BuildGraph(cfg Config) (*Graph, error) {
	pkgMap, err := AnalyzeMetrics(cfg.Root, cfg.Module)
	if err != nil {
		return nil, err
	}
	edges, depth, err := ResolveEdges(cfg)
	if err != nil {
		return nil, err
	}

	g := &Graph{
		Module: cfg.Module,
		Config: map[string]string{"goos": orDefault(cfg.GOOS, "linux"), "tags": cfg.Tags},
		Edges:  edges,
	}

	for _, p := range pkgMap {
		// attach call depth to free funcs and methods by FuncID.
		// ResolveEdges only returns depth for funcs in the in-module call graph;
		// a leaf/isolated func is absent, and its correct depth is 1 (itself).
		for i := range p.Funcs {
			f := &p.Funcs[i]
			f.CallDepth = depthOr1(depth, FuncID(p.Path, f.Recv, f.Name))
		}
		for ti := range p.Types {
			for mi := range p.Types[ti].Methods {
				m := &p.Types[ti].Methods[mi]
				m.CallDepth = depthOr1(depth, FuncID(p.Path, m.Recv, m.Name))
			}
		}
		sortPackage(p)
		g.Packages = append(g.Packages, *p)
	}
	sort.Slice(g.Packages, func(i, j int) bool { return g.Packages[i].Path < g.Packages[j].Path })
	return g, nil
}

// depthOr1 returns m[id], defaulting to 1 for a func absent from the call graph
// (a leaf/isolated function's own depth is 1).
func depthOr1(m map[string]int, id string) int {
	if d, ok := m[id]; ok {
		return d
	}
	return 1
}

func sortPackage(p *Package) {
	sort.Strings(p.Imports)
	sort.Slice(p.Types, func(i, j int) bool { return p.Types[i].Name < p.Types[j].Name })
	sort.Slice(p.Funcs, func(i, j int) bool { return p.Funcs[i].Name < p.Funcs[j].Name })
	for ti := range p.Types {
		ms := p.Types[ti].Methods
		sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
```

- [ ] **Step 4: Generate the golden file, then run the tests**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestBuildGraphGolden -update`
Then: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestBuildGraph`
Expected: PASS (both). Open `testdata/fixture_golden.json` and eyeball it — it should show foo/bar packages, Thing struct with Process (CallDepth 1), Helper, bar.Run with CallDepth >=2, and a bar->foo edge.

- [ ] **Step 5: Checkpoint (no commit — reviewer gate)**

---

### Task 5: Self-contained interactive HTML renderer + CLI main

**Files:**
- Create: `scripts/gograph/template.html`
- Create: `scripts/gograph/app.js`
- Create: `scripts/gograph/render.go`
- Create: `scripts/gograph/render_test.go`
- Create: `scripts/gograph/main.go`

**Interfaces:**
- Consumes: `Graph` (Task 1), `BuildGraph` (Task 4).
- Produces: `func RenderHTML(g *Graph) ([]byte, error)` and the `main()` entrypoint (flags per the `model.go` doc comment).

- [ ] **Step 1: Write the failing render test**

`scripts/gograph/render_test.go`:
```go
package main

import (
	"strings"
	"testing"
)

func TestRenderHTMLSelfContained(t *testing.T) {
	g := &Graph{
		Module: "m",
		Config: map[string]string{"goos": "linux", "tags": ""},
		Packages: []Package{{Path: "m/a", LOC: 100, Funcs: []FuncMetric{{Name: "F", LOC: 9, Nesting: 2, Cyclo: 3, CallDepth: 1}}}},
		Edges:    []Edge{{From: "m/a", To: "m/b", Calls: 4, Methods: []string{"b.X"}}},
	}
	out, err := RenderHTML(g)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// self-contained: no external RESOURCE LOADS. We check for real network-load
	// patterns, NOT a bare "http://" — app.js legitimately contains the SVG
	// namespace constant "http://www.w3.org/2000/svg" (required by
	// createElementNS; the browser never fetches it), which is not a network ref.
	for _, bad := range []string{
		"src=\"http", "src='http", "src=\"//",
		"href=\"http", "href=\"//",
		"<link", "<script src", "@import", "cdn.",
		"fetch(", "XMLHttpRequest", "import(",
	} {
		if strings.Contains(s, bad) {
			t.Errorf("HTML must be self-contained; found external-load pattern %q", bad)
		}
	}
	// belt-and-suspenders: the ONLY allowed http(s) literal is the SVG namespace.
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "http") && !strings.Contains(line, "www.w3.org/2000/svg") {
			t.Errorf("unexpected http reference: %q", strings.TrimSpace(line))
		}
	}
	// embeds the model and the control IDs the app.js references
	if !strings.Contains(s, "\"m/a\"") {
		t.Error("model JSON not embedded")
	}
	for _, id := range []string{"id=\"graph\"", "id=\"detail\"", "id=\"filter\"", "id=\"panel\""} {
		if !strings.Contains(s, id) {
			t.Errorf("missing control %q", id)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestRenderHTMLSelfContained`
Expected: FAIL — `undefined: RenderHTML`.

- [ ] **Step 3: Create the HTML shell**

`scripts/gograph/template.html`:
```html
<!doctype html>
<meta charset="utf-8">
<title>beatmo · go-graph</title>
<style>
  :root { color-scheme: dark; }
  body { margin:0; font:13px system-ui,sans-serif; background:#0d1117; color:#c9d1d9; }
  header { display:flex; gap:12px; align-items:center; padding:8px 12px; background:#161b22; border-bottom:1px solid #30363d; }
  header b { color:#58a6ff; }
  select, input { background:#0d1117; color:#c9d1d9; border:1px solid #30363d; border-radius:6px; padding:4px 6px; }
  #wrap { display:flex; height:calc(100vh - 45px); }
  #graph { flex:1; overflow:hidden; }
  #panel { width:320px; border-left:1px solid #30363d; padding:12px; overflow:auto; background:#0d1117; }
  #panel h2 { font-size:14px; color:#58a6ff; margin:0 0 8px; }
  .metric { display:flex; justify-content:space-between; border-bottom:1px solid #21262d; padding:3px 0; }
  .hot { color:#f0883e; } .cool { color:#3fb950; }
  text { fill:#c9d1d9; pointer-events:none; }
  .node { cursor:pointer; }
  .edge { stroke:#484f58; fill:none; }
  .edge.call { stroke:#58a6ff; }
  .dim { opacity:0.12; }
</style>
<header>
  <b>beatmo · go-graph</b>
  <label>Detail
    <select id="detail">
      <option value="0">Packages</option>
      <option value="1">+ Types</option>
      <option value="2">+ Functions</option>
    </select>
  </label>
  <input id="filter" placeholder="filter…" size="18">
  <span id="meta"></span>
</header>
<div id="wrap">
  <svg id="graph"></svg>
  <div id="panel"><h2>Select a node</h2><div>Click a package to drill in; click a type/function for metrics.</div></div>
</div>
<script id="model" type="application/json">__MODEL__</script>
<script>__APP__</script>
```

- [ ] **Step 4: Create the renderer JS**

`scripts/gograph/app.js`:
```js
(function () {
  const G = JSON.parse(document.getElementById("model").textContent);
  // Normalize omitempty-dropped slices (Package.Types/Funcs/Imports and
  // TypeInfo.Methods are `omitempty`, so empty ones are absent from the JSON)
  // so all click/drill code can assume arrays.
  G.edges = G.edges || [];
  G.packages = G.packages || [];
  G.packages.forEach(p => {
    p.types = p.types || [];
    p.funcs = p.funcs || [];
    p.imports = p.imports || [];
    p.types.forEach(t => { t.methods = t.methods || []; });
  });
  const svg = document.getElementById("graph");
  const panel = document.getElementById("panel");
  const detail = document.getElementById("detail");
  const filter = document.getElementById("filter");
  document.getElementById("meta").textContent =
    G.packages.length + " pkgs · " + G.edges.length + " call edges · " +
    G.config.goos + (G.config.tags ? " tags=" + G.config.tags : "");

  const W = svg.clientWidth || 900, H = svg.clientHeight || 600;
  let view = { x: 0, y: 0, k: 1 };
  let expanded = null; // package path currently drilled into

  const NS = "http://www.w3.org/2000/svg";
  function el(name, attrs) {
    const e = document.createElementNS(NS, name);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    return e;
  }
  function pkgLOC(p) { return p.loc || 1; }
  function hotColor(cyclo) { return cyclo >= 12 ? "#f0883e" : cyclo >= 6 ? "#d29922" : "#3fb950"; }

  // deterministic seeded layout (no Math.random → stable screenshots)
  let seed = 1234567;
  function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; }

  function layoutPackages() {
    const n = G.packages.length;
    const pos = {};
    G.packages.forEach((p, i) => {
      const ang = (i / n) * Math.PI * 2;
      const r = 180 + rnd() * 120;
      pos[p.path] = { x: W / 2 + Math.cos(ang) * r, y: H / 2 + Math.sin(ang) * r };
    });
    // a few relaxation passes pulling call-connected packages together
    for (let it = 0; it < 60; it++) {
      G.edges.forEach(e => {
        const a = pos[e.from], b = pos[e.to];
        if (!a || !b) return;
        const dx = b.x - a.x, dy = b.y - a.y;
        a.x += dx * 0.002 * Math.min(e.calls, 20);
        a.y += dy * 0.002 * Math.min(e.calls, 20);
        b.x -= dx * 0.002 * Math.min(e.calls, 20);
        b.y -= dy * 0.002 * Math.min(e.calls, 20);
      });
    }
    return pos;
  }

  const pkgPos = layoutPackages();

  function applyView() {
    svg.setAttribute("viewBox",
      `${-view.x / view.k} ${-view.y / view.k} ${W / view.k} ${H / view.k}`);
  }

  function shortName(path) { const s = path.split("/"); return s.slice(-2).join("/"); }

  function render() {
    while (svg.firstChild) svg.removeChild(svg.firstChild);
    const q = filter.value.trim().toLowerCase();

    // edges (packages view)
    G.edges.forEach(e => {
      const a = pkgPos[e.from], b = pkgPos[e.to];
      if (!a || !b) return;
      const line = el("path", {
        class: "edge call",
        d: `M${a.x},${a.y} L${b.x},${b.y}`,
        "stroke-width": Math.max(0.5, Math.min(6, Math.log2(e.calls + 1))),
      });
      svg.appendChild(line);
    });

    // package nodes sized by LOC
    const maxLOC = Math.max(...G.packages.map(pkgLOC));
    G.packages.forEach(p => {
      const c = pkgPos[p.path];
      const rad = 10 + 40 * Math.sqrt(pkgLOC(p) / maxLOC);
      const dim = q && !p.path.toLowerCase().includes(q);
      const g = el("g", { class: "node" + (dim ? " dim" : "") });
      g.appendChild(el("circle", { cx: c.x, cy: c.y, r: rad, fill: "#1f6feb", "fill-opacity": 0.35, stroke: "#58a6ff" }));
      const label = el("text", { x: c.x, y: c.y + 4, "text-anchor": "middle", "font-size": 11 });
      label.textContent = shortName(p.path);
      g.appendChild(label);
      g.addEventListener("click", () => { expanded = (expanded === p.path ? null : p.path); showPackage(p); render(); });
      svg.appendChild(g);

      if (Number(detail.value) >= 1 && (expanded === p.path)) drawDetail(p, c, rad);
    });
    applyView();
  }

  // treemap of a package's types + funcs sized by LOC, drawn around the node
  function drawDetail(p, c, rad) {
    if (Number(detail.value) < 1) return;
    const items = [];
    p.types.forEach(t => items.push({ kind: "type", name: t.name, loc: t.loc, ref: t }));
    if (Number(detail.value) >= 2) p.funcs.forEach(f => items.push({ kind: "func", name: f.name, loc: f.loc, ref: f, cyclo: f.cyclo }));
    if (!items.length) return;
    const total = items.reduce((s, i) => s + Math.max(i.loc, 1), 0);
    let x = c.x + rad + 8, y = c.y - 40, rowH = 14, boxW = 120, used = 0;
    items.sort((a, b) => b.loc - a.loc).slice(0, 12).forEach(it => {
      const w = boxW, h = Math.max(6, rowH * (it.loc / total) * items.length);
      const box = el("rect", { x, y: y + used, width: w, height: h, fill: it.kind === "type" ? "#8957e5" : hotColor(it.cyclo || 0), "fill-opacity": 0.7, stroke: "#30363d", rx: 2, class: "node" });
      box.addEventListener("click", ev => { ev.stopPropagation(); showItem(p, it); });
      svg.appendChild(box);
      const tx = el("text", { x: x + 4, y: y + used + 10, "font-size": 9 });
      tx.textContent = it.name + " (" + it.loc + ")";
      svg.appendChild(tx);
      used += h + 2;
    });
  }

  function showPackage(p) {
    panel.innerHTML = "<h2>" + p.path + "</h2>" +
      row("LOC", p.loc) + row("Files", p.files) +
      row("Types", p.types.length) + row("Funcs", p.funcs.length) +
      row("Imports", p.imports.length);
    const callers = G.edges.filter(e => e.to === p.path).map(e => e.from);
    const callees = G.edges.filter(e => e.from === p.path).map(e => e.to);
    panel.innerHTML += "<h2>Calls out</h2>" + (callees.map(shortName).join(", ") || "—");
    panel.innerHTML += "<h2>Called by</h2>" + (callers.map(shortName).join(", ") || "—");
  }

  function showItem(p, it) {
    const f = it.ref;
    if (it.kind === "func") {
      panel.innerHTML = "<h2>" + p.path + " · " + f.name + "()</h2>" +
        row("LOC", f.loc) + rowHot("Nesting depth", f.nesting, 4) +
        rowHot("Cyclomatic", f.cyclo, 10) + row("Call-graph depth", f.callDepth);
    } else {
      panel.innerHTML = "<h2>" + p.path + " · " + f.name + "</h2>" +
        row("Kind", f.kind) + row("LOC", f.loc) + row("Methods", (f.methods || []).length);
      (f.methods || []).forEach(m => {
        panel.innerHTML += "<div class='metric'><span>" + m.name + "</span><span>LOC " + m.loc +
          " · cc " + m.cyclo + " · depth " + m.callDepth + "</span></div>";
      });
    }
  }
  function row(k, v) { return "<div class='metric'><span>" + k + "</span><span>" + v + "</span></div>"; }
  function rowHot(k, v, thresh) {
    const cls = v >= thresh ? "hot" : "cool";
    return "<div class='metric'><span>" + k + "</span><span class='" + cls + "'>" + v + "</span></div>";
  }

  // pan/zoom
  let drag = null;
  svg.addEventListener("mousedown", e => drag = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y });
  window.addEventListener("mouseup", () => drag = null);
  window.addEventListener("mousemove", e => {
    if (!drag) return;
    view.x = drag.vx + (e.clientX - drag.x);
    view.y = drag.vy + (e.clientY - drag.y);
    applyView();
  });
  svg.addEventListener("wheel", e => {
    e.preventDefault();
    view.k = Math.max(0.2, Math.min(4, view.k * (e.deltaY < 0 ? 1.1 : 0.9)));
    applyView();
  }, { passive: false });

  detail.addEventListener("change", render);
  filter.addEventListener("input", render);
  render();
})();
```

- [ ] **Step 5: Create the renderer + main**

`scripts/gograph/render.go`:
```go
package main

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed template.html
var tmplHTML string

//go:embed app.js
var appJS string

// RenderHTML embeds the model and the app script into the self-contained page.
func RenderHTML(g *Graph) ([]byte, error) {
	model, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	out := strings.Replace(tmplHTML, "__MODEL__", string(model), 1)
	out = strings.Replace(out, "__APP__", appJS, 1)
	return []byte(out), nil
}
```

`scripts/gograph/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Root, "root", "../../src/go", "module source root")
	flag.StringVar(&cfg.Module, "module", "github.com/ingyamilmolinar/beatmo", "module import path")
	flag.StringVar(&cfg.GOOS, "goos", "linux", "GOOS for call resolution (e.g. js for wasm-only files)")
	flag.StringVar(&cfg.Tags, "tags", "", "comma-separated build tags for call resolution")
	out := flag.String("out", "../../build/go-graph.html", "output HTML path")
	jsonOut := flag.String("json", "", "optional path to also write the raw JSON model")
	flag.Parse()

	g, err := BuildGraph(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	html, err := RenderHTML(g)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	fmt.Println("HTML:", *out)
	if *jsonOut != "" {
		model, werr := writeJSON(g)
		if werr != nil {
			fmt.Fprintln(os.Stderr, "gograph:", werr)
			os.Exit(1)
		}
		if err := os.WriteFile(*jsonOut, model, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gograph:", err)
			os.Exit(1)
		}
		fmt.Println("JSON:", *jsonOut)
	}
}

func writeJSON(g *Graph) ([]byte, error) {
	return jsonMarshalIndent(g)
}
```

Add to `render.go` a small helper used by main:
```go
func jsonMarshalIndent(g *Graph) ([]byte, error) {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
```

- [ ] **Step 6: Run the render test**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./... -run TestRenderHTMLSelfContained`
Expected: PASS.

- [ ] **Step 7: End-to-end run against the real beatmo module**

Run:
```bash
cd scripts/gograph && ../../.tools/go/bin/go run . \
  -root ../../src/go -module github.com/ingyamilmolinar/beatmo \
  -goos linux -out ../../build/go-graph.html -json ../../build/go-graph.json
```
Expected: prints `HTML: ../../build/go-graph.html` and `JSON: ../../build/go-graph.json`. Some `gograph: skipping call edges for … (N type errors)` lines on stderr are acceptable (wasm-only/cgo edge cases). Open `build/go-graph.html` in a browser: packages appear sized by LOC, the Detail selector reveals types/funcs, clicking a function shows LOC/nesting/cyclo/call-depth, and `build/go-graph.json` contains real edges (e.g. `internal/ui` → `internal/audio`).

- [ ] **Step 8: Run the full tool test suite**

Run: `cd scripts/gograph && ../../.tools/go/bin/go test ./...`
Expected: all PASS.

- [ ] **Step 9: Checkpoint (no commit — reviewer gate)**

---

### Task 6: Makefile target + CLAUDE.md documentation

**Files:**
- Modify: `Makefile` (add `go-graph` target near the `dep-graph` target, ~line 210-240)
- Modify: `CLAUDE.md` (Quick Start table)
- Modify: `.gitignore` (ensure `build/` artifacts already ignored; add only if missing)

**Interfaces:** none (integration only).

- [ ] **Step 1: Add the Makefile target**

Insert after the `dep-graph` block in `Makefile`:
```makefile
# ─── Go Graph (interactive architecture visualizer) ──────────────────────────
# Emit a self-contained interactive HTML view of packages/types/functions with
# LOC sizing, nesting/cyclomatic/call-graph depth, and type-resolved call edges.
# Usage: make go-graph [GG_GOOS=linux] [GG_TAGS=] [GG_ROOT=src/go]
GG_GOOS ?= linux
GG_TAGS ?=
GG_ROOT ?= $(CURDIR)/src/go
go-graph:
	@mkdir -p build
	cd scripts/gograph && $(GO) run . \
		-root $(GG_ROOT) \
		-module github.com/ingyamilmolinar/beatmo \
		-goos $(GG_GOOS) -tags '$(GG_TAGS)' \
		-out $(CURDIR)/build/go-graph.html \
		-json $(CURDIR)/build/go-graph.json
	@echo "Open build/go-graph.html in a browser"
```
Note: `GG_ROOT` and the `-out`/`-json` paths are absolute (`$(CURDIR)`) because the recipe `cd`s into `scripts/gograph`.

- [ ] **Step 2: Verify the target runs**

Run: `make go-graph`
Expected: prints `HTML:`/`JSON:` lines and `Open build/go-graph.html in a browser`. Confirm `build/go-graph.html` exists and is > 20 KB.

- [ ] **Step 3: Confirm build artifacts are git-ignored**

Run: `git check-ignore build/go-graph.html && echo IGNORED || echo NOT-IGNORED`
Expected: `IGNORED`. If `NOT-IGNORED`, add `build/` to `.gitignore`.

- [ ] **Step 4: Document in CLAUDE.md Quick Start table**

In the Quick Start table in `CLAUDE.md`, add a row after the `dep-graph`-related entries (or near "Regenerate design tokens"):
```markdown
| Interactive architecture graph | `make go-graph` (packages/types/funcs sized by LOC + nesting/cyclomatic/call-depth + type-resolved call edges → `build/go-graph.html`) |
```

- [ ] **Step 5: Final verification of the whole tool**

Run:
```bash
cd scripts/gograph && ../../.tools/go/bin/go test ./... && ../../.tools/go/bin/go vet ./...
```
Expected: tests PASS, vet clean.

- [ ] **Step 6: Checkpoint (no commit — reviewer gate)**

Leave everything uncommitted for the user to review.

---

## Self-Review

**Spec coverage:**
- Tool structure + own `go.mod` isolating `x/tools` → Task 1. ✓
- `make go-graph` → `build/go-graph.html` + `build/go-graph.json` → Task 5/6. ✓
- AST metrics: LOC, nesting depth, cyclomatic complexity → Task 2. ✓
- Type-resolved cross-package call edges (method-on-imported-type) + per-edge call counts + methods → Task 3. ✓
- Import edges captured separately → Task 2 (`Package.Imports`). ✓
- Call-graph depth (memoized DFS, cycle guard) → Task 3, joined in Task 4. ✓
- Build config default desktop + `-goos`/`-tags` flag + skip/log failing packages → Task 3 (`ResolveEdges`), Task 5 (flags). ✓
- Self-contained offline interactive HTML: detail-level control, LOC-area sizing, drill-down, side panel, zoom/pan/filter, hotspot color → Task 5. ✓
- Hover/focus UX (borrowed idea): filter-dim + click-to-focus package neighbors implemented; full hover-highlight is present via dim/focus. Note: the plan implements click-focus + filter-dim; per-edge hover-highlight is a nice-to-have the executor may extend but is not required for spec acceptance. ✓ (documented divergence)
- Testing: fixture module, metrics unit tests, call-edge resolution test, call-depth test, golden JSON, HTML smoke test → Tasks 1–5. ✓
- Docs: CLAUDE.md row + main.go doc comment → Task 5/6. ✓

**Placeholder scan:** No TBD/TODO left as deliverables. The only "remove this scaffolding" instruction (Task 3 Step 4) is an explicit, actionable cleanup with exact lines named, not a vague placeholder.

**Type consistency:** `Config`, `Graph`, `Package`, `TypeInfo`, `FuncMetric`, `Edge`, `FuncID` used identically across Tasks 1–5. `AnalyzeMetrics(root, module)`, `ResolveEdges(cfg)`, `BuildGraph(cfg)`, `RenderHTML(g)` signatures are consistent between their producing task and consuming tasks. `recvName` (AST) and `recvTypeName` (types) are distinct helpers for their respective passes and both strip pointers, keeping `FuncID` receiver strings equal across passes.

**Divergence note:** Spec §HTML lists per-edge hover-highlight; the plan delivers click-to-focus + text-filter dimming (same UX goal, simpler for hand-rolled SVG). Acceptable per YAGNI; executor may add hover-highlight if cheap.

---

## Post-build UX refinements (applied to `scripts/gograph/app.js` after Task 5)

Applied in response to user feedback; renderer-only (no Go/model change), so the
generated HTML stays deterministic (layout runs client-side):

1. **Cursor-anchored zoom.** The wheel handler now keeps the world point under
   the cursor fixed while zooming (`view_new = s − (kNew/kOld)·(s − view_old)`,
   with `s` the cursor in the viewBox's W/H space via `getBoundingClientRect`),
   instead of always zooming toward the origin.
2. **Non-overlapping force layout.** `layoutPackages` gained inverse-square
   repulsion + edge attraction + **hard radius-based collision** (`separate()`),
   plus final collide-only passes guaranteeing zero overlap, and a `fitView()`
   that scales the initial viewBox to the laid-out bounds. Verified on the real
   48-package model: 0 overlapping pairs, min edge-to-edge gap 16px.
3. **LOC-proportional, complexity-colored nodes.** `nodeRadius(p) = MIN_R +
   (MAX_R−MIN_R)·sqrt(LOC/maxLOC)` (area ∝ LOC, small floor for clickability).
   Shared `complexityColor(cyclo)` (green→yellow→orange→red) colors BOTH package
   nodes (by their max function/method cyclomatic complexity, `pkgComplexity`)
   and function boxes; type boxes stay purple as a category color. A color legend
   was added to the header.

### Drill-down redesign (replaces the detail-level control)

Per user feedback, the `+Types`/`+Functions` detail selector was removed in favor
of click-to-drill node navigation (renderer-only; HTML stays deterministic):

- **Levels as nodes.** Click a package node → its types & functions become their
  own nodes; click a type node → its methods become nodes. A clickable
  breadcrumb (`Packages / pkg / Type`) navigates back up; a function/method node
  selects and shows its metrics in the side panel.
- **Composable size & complexity.** One `radiusFor(sceneItems)` (area ∝ LOC,
  scaled to the scene's largest) and one `complexityColor(complexityOf(item))`
  apply uniformly at every level. `complexityOf` composes: function = own
  cyclomatic; type = max over its methods; package = max over its members.
- **Layouts.** Packages keep the force layout (call edges). Members have no
  edges, so they use a size-sorted golden-angle packing + hard collision
  (`layoutMembers`). Both guarantee zero overlap.
- **Scale.** Member views cap at the 160 largest nodes (by LOC); the filter
  searches ALL members and re-lays-out matches, so any member is reachable. A
  bottom-left note reports truncation. Verified on the real module:
  internal/ui (1402 members) and internal/audio (958) render capped at 160 in
  ≤35 ms with 0 overlaps; DrumView's 503 methods likewise. Every node carries a
  `<title>` tooltip (full name + LOC + complexity/depth).
- template.html: `#detail` select removed, `#crumb` breadcrumb + `#note` overlay
  added; `render_test.go` control-id list updated (`detail` → `crumb`).

### Intra-package member edges (types ↔ functions within a package)

Per user request, the model now carries intra-package caller→callee relationships,
drawn as directed arrows in the package drill view:

- **Model.** New `MemberEdge{From,To,Calls}` and `Package.MemberEdges`. In
  `callgraph.go`, the same type-resolved call walk that feeds depth now also
  aggregates intra-package calls to member granularity: each endpoint is a
  package-level identifier — a top-level function name, or (for a method call)
  its receiver type name — so a call becomes func→func, func→type, type→func, or
  type→type (self-edges dropped). Deterministically sorted; `ResolveEdges` returns
  a 3rd value `map[pkgPath][]MemberEdge`; `build.go` attaches it per package.
  (Package identifiers are unique across funcs+types, so the name alone keys the
  node.) Fixture test asserts foo's `Thing→Helper` edge (Thing.Deep calls Helper).
- **Renderer.** In the package view, directed arrows (trimmed to node radii,
  arrowhead at the callee, width ∝ log(calls)) connect visible member nodes;
  selecting a function highlights its incident edges. Drawn only among the visible
  (top-160 / filtered) nodes and capped at 1600 for readability. On the real
  module: internal/ui has 2183 member edges but only 172 fall among its visible
  top-160 (audio 91, hooks 11) — readable, not cluttered. The type→methods view
  has no member edges (they are package-level). JSON grew ~1.08→1.50 MB.
