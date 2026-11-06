# go-graph — Interactive Go Architecture Visualizer

**Date:** 2026-07-03
**Status:** Approved design, pending implementation plan

## Purpose

Provide tooling that graphically renders the beatmo Go program — its packages,
functions, and data structures — with:

- **Size:** lines of code (LOC) per package, type, and function.
- **Depth (three metrics, all requested):**
  - **Nesting depth** — max block-nesting inside a function body.
  - **Cyclomatic complexity** — independent paths through a function.
  - **Call-graph depth** — how deep the downstream call chain reaches.
- **Multiple levels of detail** — drill down Packages → Types → Functions.
- **Imports and cross-package calls** — which public methods/functions one
  package actually calls in another, with per-edge call counts, distinct from
  raw import edges.

This is a significant upgrade over the existing `scripts/depgraph` (static DOT of
packages/types/funcs/imports), which has no LOC sizing, no depth/complexity, no
drill-down, and no true call edges (imports only). `depgraph` is left untouched.

## Non-Goals (YAGNI)

- Not a replacement for `depgraph`/`covgraph`; those keep working.
- No live/watch mode, no server — a single static HTML file opened in a browser.
- No runtime/dynamic call tracing; static analysis only.
- No cross-language (C/JS) graphing; Go only.

## Decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Output format | Interactive, self-contained HTML (offline, no CDN) |
| Depth metrics | All three: nesting depth, cyclomatic complexity, call-graph depth |
| Call-edge fidelity | Type-resolved via `go/packages` (resolves methods on imported types) |
| Default build config | Native desktop: `GOOS=linux`, no build tags |
| HTML renderer | Hand-rolled SVG + vanilla JS (no d3 vendoring) |

## Prior art evaluated

Two existing tools were cloned and analyzed for reuse:

- **kisielk/godepgraph** (MIT, stdlib-only, ~250 LOC) — package-level, import-only
  DOT/Mermaid text emitter. No AST/type analysis, no metrics, no interactivity.
  Nothing on our critical path (type-resolved calls, metrics, drill-down) exists.
  Liftable value: CLI ignore/prefix filtering ergonomics only.
- **grishy/gopkgview** (**GPL-3.0**, React Flow + elkjs, ~1.77 MB JS bundle) —
  package-level, import-only graph served by a local HTTP server. Nicer
  interactive UI (zoom/pan/hover/select) but no metrics, no type resolution, no
  packages→types→functions drill-down. GPL license makes code reuse a copyleft
  blocker; server + heavy-bundle model conflicts with our self-contained offline
  HTML goal.

**Conclusion:** neither is reusable as an engine — both lack the type-resolved
call edges, per-function metrics, and hierarchical drill-down that are the point
of this tool. We build fresh and borrow two *ideas* (not code): gopkgview's
hover-highlight / click-to-focus-neighbors UX (§ HTML), and `go/ssa` + `go/callgraph`
as an alternative call-edge construction route (§ Call edges).

## Architecture

Three components: a Go analysis engine, a JSON model, and a self-contained HTML
renderer. The tool lives in its own module so its heavy dependency does not
touch the main build.

```
scripts/gograph/           (own go.mod, requires golang.org/x/tools)
  main.go                  CLI: flags, orchestration, HTML emit
  metrics.go               AST pass: LOC, nesting depth, cyclomatic complexity
  callgraph.go             go/packages pass: type-resolved call edges
  model.go                 the JSON model types (Graph, Package, Type, Func, Edge)
  render.go                template exec → self-contained HTML
  template.html            embedded (go:embed) HTML+CSS+JS shell
  app.js                   embedded (go:embed) vanilla-JS renderer
  *_test.go                unit + golden tests
  testdata/fixturemod/     tiny multi-package module for deterministic tests
```

### Tool structure & dependency isolation

- New command **`scripts/gograph/`** with its **own `go.mod`** requiring
  `golang.org/x/tools`. This keeps `x/tools` out of `src/go/go.mod` (WASM-bloat
  sensitivity; mirrors how `depgraph` stays stdlib-only and isolated).
- The tool points `go/packages` at `src/go` via `Config.Dir`, analyzing the real
  module without being part of it.
- New **`make go-graph`** target emits:
  - `build/go-graph.html` — self-contained interactive visualization.
  - `build/go-graph.json` — the raw model, reusable for CI / diffing / other UIs.
- Invocation shape (mirrors `dep-graph`):
  ```
  make go-graph [GOOS=linux] [TAGS=] [ROOT=src/go]
  ```
  Under the hood: `cd scripts/gograph && go run . -root ../../src/go
  -goos linux -tags '' -out ../../build/go-graph.html -json ../../build/go-graph.json`.

### Analysis engine (Go)

**Pass 1 — Metrics (AST, all in-module packages).** Parse every non-test `.go`
file. Per declaration:

- **Function/method LOC** — `fset.Position(decl.End()).Line - fset.Position(decl.Pos()).Line + 1`.
- **Max nesting depth** — walk the body; increment on `*ast.IfStmt`,
  `*ast.ForStmt`, `*ast.RangeStmt`, `*ast.SwitchStmt`, `*ast.TypeSwitchStmt`,
  `*ast.SelectStmt`, and `*ast.BlockStmt` inside those; track running max.
- **Cyclomatic complexity** — `1 +` count of `if`, `for`, `range`, `case`
  (per case clause, excluding default), `&&`, `||`, and boolean short-circuits.
  (Standard McCabe-ish definition; documented in code.)
- **Type LOC** — the `TypeSpec` span plus the LOC of all methods with that
  receiver. Kind ∈ {struct, interface, alias/named}.
- **Package rollups** — total LOC, file count, type/func/method counts.

**Pass 2 — Call edges (`go/packages`, type-resolved).** Load in-module packages
with `NeedName|NeedFiles|NeedSyntax|NeedTypes|NeedTypesInfo|NeedImports`. For
every `*ast.CallExpr`, resolve the callee via `TypesInfo.Uses`/`Selections`:

- Direct `pkg.Func()` → resolves to the function object and its package.
- `x.Method()` where `x`'s type belongs to another package → resolved via
  `types.Selection` to the method object and its package. **This is the case
  AST-only analysis misses; it is the core reason for type resolution.**
- Keep only edges where **caller package ≠ callee package** and callee is
  **exported** and **in-module**. Aggregate into
  `Edge{From, To, Calls: count, Methods: set-of-qualified-names}`.
- **Import edges** captured separately from `Package.Imports`, so the UI can
  show "imports but never calls" vs. "actively calls".

**Call-edge construction — two routes.** Default is the lightweight `types.Info`
selector walk described above (walk `CallExpr`, resolve via `Uses`/`Selections`),
which is sufficient for package-level aggregation and cheap. If precision on
interface/dynamic dispatch is later needed, `golang.org/x/tools/go/ssa` +
`go/callgraph` (CHA or RTA) is the more robust alternative; the implementation
plan should keep the resolver behind an interface so this can be swapped without
touching the model or renderer.

**Call-graph depth** is derived from the resolved intra+inter-package call edges:
for each function node, the longest downstream chain length (memoized DFS over
the function-level call graph; cycles bounded by a visited set so recursion does
not loop forever — depth caps at first repeat).

**Build config.** Default `GOOS=linux`, no tags (native desktop). `internal/audio`
uses cgo (miniaudio) and type-checks with the system C compiler present. A
`-goos`/`-tags` flag regenerates under `js`/`test` to capture the 61 wasm-only
files (`js_exports_*`, etc.). The default view documents that wasm-only symbols
are omitted. Packages that fail to type-check under the chosen config are
reported to stderr and skipped (metrics from Pass 1 still appear; only their
call edges are missing).

### JSON model

```jsonc
{
  "module": "github.com/ingyamilmolinar/beatmo",
  "config": { "goos": "linux", "tags": "" },
  "packages": [
    {
      "path": "internal/ui",
      "loc": 41000, "files": 210,
      "types": [
        { "name": "DrumView", "kind": "struct", "loc": 812,
          "methods": [
            { "name": "Draw", "loc": 140, "nesting": 6, "cyclo": 17, "callDepth": 5 }
          ] }
      ],
      "funcs": [
        { "name": "NewDrumView", "loc": 60, "nesting": 3, "cyclo": 5, "callDepth": 4 }
      ],
      "imports": ["internal/audio", "internal/timeline"]
    }
  ],
  "edges": [
    { "from": "internal/ui", "to": "internal/audio",
      "calls": 37, "methods": ["audio.StartRecording", "(*audio.Engine).Process"] }
  ]
}
```

### Interactive HTML (self-contained, offline)

One HTML file, no CDN/network, hand-rolled SVG + vanilla JS (~150 lines). Embeds
the JSON model inline (`<script type="application/json">`). Controls:

- **Detail-level control (global):** Packages → +Types → +Functions. Matches the
  "multiple levels of detail" requirement without regenerating.
- **Package level:** packages as nodes; **node area ∝ package LOC**; import edges
  (thin) and call edges (weight ∝ call count). Hand-rolled force layout — ~30
  nodes, dependency-free and deterministic seed for stable layout.
- **Expand a package:** treemap of its types & top-level funcs, boxes sized by
  LOC (deterministic squarified treemap).
- **Select a func/type:** side panel showing LOC, **nesting depth**,
  **cyclomatic complexity**, **call-graph depth**, and cross-package
  callers/callees (from resolved edges).
- **Zoom/pan** (SVG viewBox) + **text filter** (dim non-matching nodes).
- **Hover-highlight & click-to-focus** (borrowed from gopkgview's UX): hovering a
  node highlights its connected nodes/edges; clicking focuses on a node and its
  direct callers/callees, dimming the rest. Cheap to add to the hand-rolled SVG.
- **Color encoding:** hue by depth/complexity hotspot (e.g., high cyclo or deep
  nesting → warm), so hotspots pop at every zoom level.

Layout is computed in JS from the emitted metrics; Go emits the model only. A
deterministic seed keeps the force layout reproducible for screenshots/tests.

### Testing

Per the JS-test charter, logic lives in Go:

- **`testdata/fixturemod/`** — a tiny 2–3 package module with hand-known
  metrics: a function with known nesting depth, a known cyclomatic count, a
  method-on-imported-type call, and a direct `pkg.Func()` call.
- **Metrics unit tests** — assert LOC / nesting / cyclomatic values on the
  fixture's named functions.
- **Call-edge test** — assert the fixture's method-on-imported-type call
  produces a `From→To` edge with the expected qualified method name (the case
  AST-only would miss), and that same-package calls produce no edge.
- **Call-graph-depth test** — assert a known A→B→C chain yields callDepth 3.
- **Golden JSON test** — pin the emitted model shape for the fixture module
  (regenerate flag, e.g. `-update`).
- **HTML smoke test** — assert the emitted HTML parses, contains the embedded
  JSON, and references the expected control IDs. JS interaction logic itself is
  not unit-tested (kept minimal, logic is in Go).

## Interfaces / boundaries

- `metrics.go`: `func AnalyzeMetrics(root string) (map[string]*PkgMetrics, error)`
  — pure AST, no type checking, no network. Independently testable.
- `callgraph.go`: `func ResolveEdges(cfg Config) ([]Edge, map[FuncID]int, error)`
  — returns cross-package edges and per-function call depth. Independently
  testable against the fixture module.
- `model.go`: assembles `Graph` from the two passes. Pure data.
- `render.go`: `func RenderHTML(g *Graph) ([]byte, error)` — template exec only;
  no analysis. Testable via the smoke test.

Each unit has one purpose, a typed interface, and testable-in-isolation inputs.

## Documentation

- Add a `make go-graph` row to CLAUDE.md's Quick Start table.
- Header doc comment in `main.go` describing flags, build-config caveat
  (wasm-only omission), and the JSON model.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| cgo (`internal/audio`) type-check flakiness | C compiler is present (C DSP already builds); report+skip on failure, keep AST metrics. |
| wasm-only symbols missing in default view | Documented; `-goos js` flag to regenerate under wasm config. |
| Force layout non-determinism | Fixed PRNG seed; deterministic treemap for intra-package. |
| Adding `x/tools` bloats main module | Isolated in `scripts/gograph/go.mod`; never touches `src/go/go.mod`. |
| Large graph readability (~50 packages) | Detail-level control + LOC-area sizing + filter keep it legible. |

## Out-of-scope follow-ups (not this spec)

- CI diffing of `go-graph.json` between branches.
- Merging C-DSP and JS boundaries into the same view.
