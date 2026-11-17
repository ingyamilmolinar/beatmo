# Refactor Ranking (`gograph rank`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `scripts/gograph` with a `rank` subcommand. It measures the Go module correctly per build config, generates refactor candidates with explainable scores, accepts agent verdicts, and writes `build/refactor/{rank.json,rank.md,rank.html,packets/}`.

**Architecture:** A new package `scripts/gograph/rank` (inside the existing tool module) holds loader → units/metrics → clones → reachability → types/areas → candidates/score → reports. `main.go` dispatches `gograph rank …`; the old default (`go-graph.html`) is unchanged except that its metrics now honour build constraints and exclude closure bodies. The agent layer is a saved Claude Code workflow writing `build/refactor/verdicts/<id>.json`.

**Tech Stack:** Go 1.26 (bundled), `golang.org/x/tools` v0.38.0 (`go/packages`, `go/ssa`, `go/ssa/ssautil`, `go/callgraph/rta`, `cover`).

**Spec:** `docs/superpowers/specs/2026-10-19-refactor-rank-design.md`

## Global Constraints

- The tool stays its own module (`scripts/gograph/go.mod`) and never imports beatmo packages.
- Production configs: **linux** (`./...`, all mains as RTA roots) and **js/wasm** (`./cmd` + deps, `./cmd` main as root). The `test` tag is never in a production config.
- Test configs (reachability only): untagged linux `Tests:true`, and tagged `-tags test -modfile=go.test.mod` with `Tests:true`. `-fast` skips both and marks unreachable as `unsplit` (confidence ≤ medium).
- Generated files (`ast.IsGenerated`) are measured but never ranked.
- Coverage is repeatable `-cover`; a unit absent from every profile is unknown (0.5); js-only units are always unknown.
- Output must be deterministic: same tree ⇒ byte-identical `rank.json`/`rank.md`.
- Verdicts live only in `build/refactor/verdicts/` (local, gitignored). Suppressions live in committed `scripts/gograph/refactor_suppress.json`.
- The tool never edits source. Agents never edit source.
- No commits (user rule): leave changes in the working tree.

## Review Focus

- **cgo files**: compiled syntax comes from the cgo cache with `//line` directives. Metrics re-parse `GoFiles` from source and join typed data by (file, line, name); a cgo package must still produce units and fan-in. Test: fixture has no cgo, so verify on `internal/audio` in the real-repo smoke (Task 9).
- **Build-tag pair with one name**: `x_js.go` + `x_notjs.go` both declare `F`. Expect two units, each tagged with its config, not one merged unit. Test in Task 2.
- **Packages with type errors under a config** (js build of linux-only code): load errors must not abort; units are still measured, and reachability for that config is skipped for the erroring package. Test in Task 4 (fixture package with a deliberate js-only type error).
- **Huge clone groups** (18 identical 9-line wrappers): one candidate, with payoff (n−1)×lines and all members listed. No O(n²) blowup and no 17 duplicates. Test in Task 3.
- **Stale suppression or a malformed verdict file**: reported, never fatal. Test in Task 7.

---

### Task 1: Accurate base metrics (shared by go-graph and rank)

**Files:**
- Create: `scripts/gograph/rank/metrics.go`, `scripts/gograph/rank/metrics_test.go`
- Modify: `scripts/gograph/metrics.go` (honour build constraints via `build.Context.MatchFile`; cyclo excludes FuncLit bodies), `scripts/gograph/main.go` (pass GOOS/tags), `scripts/gograph/testdata/fixture_golden.json` (regenerate)
- Delete: `scripts/gograph/gograph` (stale binary); add `/scripts/gograph/gograph` to `.gitignore`

**Interfaces — Produces:**
- `rank.Cyclomatic(body ast.Node) int` (McCabe; FuncLit bodies skipped)
- `rank.Cognitive(body *ast.BlockStmt, selfName string) (total, nestingBonus int)` (Sonar: structures +1+nesting; `else`/`else if` +1; each run of same `&&`/`||` +1; labelled branch/goto +1; direct recursion on `selfName` +1; FuncLit bodies skipped, but a FuncLit increases nesting for nothing since it's its own unit)
- `rank.MaxNesting(body *ast.BlockStmt) int`
- `rank.SLOC(src []byte, startLine, endLine int) int` (non-blank, non-comment-only lines)
- `rank.DataRatio(fset *token.FileSet, body *ast.BlockStmt) float64` (lines covered by composite literals ÷ body lines)

- [ ] Tests: the Sonar spec examples, with `sumOfPrimes` = 7 and `getWords` (switch) = 1; a closure-heavy func whose parent cognitive excludes the closure; `Helper` from the fixture keeps cyclo 4 and nesting 3; an 80 %-literal table function has DataRatio ≥ 0.7.
- [ ] Implement; `go test ./...` in `scripts/gograph` passes; regenerate the golden with `-update` and review the diff (only build-tag and closure effects).

### Task 2: Loader + units (multi-config union)

**Files:** Create `rank/load.go`, `rank/units.go`, `rank/units_test.go`; extend fixture `testdata/rankmod/` (new module `example.com/rankmod`: `app/` main, `core/` lib with `plat_js.go`/`plat_notjs.go` pair, a `zz_gen.go` generated file, a data table, a god type over 3 files, clones, a dead func, a test-only func + `_test.go`, a `test`-tagged helper file).

**Interfaces — Produces:**
```go
type BuildConfig struct{ Name, GOOS, GOARCH, Tags, ModFile string; Patterns []string; Tests bool }
type Config struct{ Root, Module string; Prod, TestCfgs []BuildConfig; Covers []string; Fast bool }
type Unit struct {
    ID, Pkg, File, Name, Recv, Kind, Parent string // Kind: func|method|closure
    Line, EndLine int
    Exported, Generated, Data, JSOnly bool
    Configs []string
    LOC, SLOC, Cyclo, Cognitive, NestBonus, Nesting, Params, Results, FanIn, FanOut int
    DataRatio float64
    Verb Verbosity
    Coverage *float64 `json:",omitempty"`
    Reach string // prod|test-only|dead|unsplit|""
}
type Model struct{ Units []*Unit; Files []*File; Types []*TypeStat; Areas, Pkgs []*AreaStat; Clones []*CloneGroup; Totals Totals }
func Load(cfg Config) (*Loaded, error)          // one packages.Load per config, errors tolerated
func CollectUnits(l *Loaded) []*Unit           // re-parse GoFiles per config, union by ID
```
- Unit ID = `relfile:line:name` for decls, `relfile:line:col` for closures; closure Name = `Parent.funcN` (N in source order). `JSOnly` = configs == [js].
- Fan-in/out: walk typed syntax of each non-test package variant; map call position → innermost unit by (file, line range); map callee `*types.Func` → unit by (decl file, line, name). Interface methods map to nothing (counted in FanOut as unresolved-dynamic, not in FanIn).

- [ ] Test: in the fixture, `core.Plat` appears as two units with configs `[js]` and `[linux]`; the generated unit has `Generated=true`; the closure unit exists with Parent set; FanIn of a helper called from 3 units = 3.
- [ ] Implement and pass.

### Task 3: Clones

**Files:** `rank/clones.go`, `rank/clones_test.go`

**Interfaces — Produces:** `type CloneGroup struct{ ID, Tier string; Lines int; Members []CloneLoc; CrossPkg bool }`; `type CloneLoc struct{ File string; Line, EndLine int; Unit string }`; `func FindClones(units []*Unit, files map[string]*ast.File, fset) []*CloneGroup`
- Exact tier: consistent identifier renaming (first-occurrence index within the region), literal values kept. Structural tier: identifiers and literal values abstracted. A group found in both tiers is reported once, as exact.
- Whole units ≥ 8 lines; statement windows of 6 statements spanning ≥ 10 lines, merged to maximal regions (a group is skipped when every member's previous window forms the same shifted group); members overlapping each other are dropped; window groups whose every member lies inside one whole-unit clone group are dropped.
- Intra-unit repetition (3-statement windows repeated inside one unit, non-overlapping) feeds `Unit.Verb.RepeatedLines`.

- [ ] Tests: the fixture's two 12-line copies with renamed locals → one exact group; two same-shape switch tables with different strings → structural only; 5 identical wrappers → one group, 5 members; a 14-statement copy → one maximal region, not 9.
- [ ] Implement and pass.

### Task 4: Reachability (RTA)

**Files:** `rank/reach.go`, `rank/reach_test.go`
**Interfaces — Produces:** `func Reachability(l *Loaded, units []*Unit)` sets `Unit.Reach` for func/method units (closures inherit the parent's).
- Per config: `ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)`, `prog.Build()`, roots = `init`+`main` of each main package in that config's roots (prod: non-test mains; test: `.test` mains), `rta.Analyze(roots, false)`. Key reached funcs by (file, line, name) via `fn.Origin()` when generic; skip synthetic (no pos).
- Prod-reached in any prod config containing the unit ⇒ `prod`; else reached by any test config ⇒ `test-only`; else `dead` (`unsplit` in `-fast`). Packages with errors are excluded from SSA for that config.

- [ ] Test: fixture `dead` func ⇒ `dead`; `helperForTest` used only by `_test.go` ⇒ `test-only`; a func only called from the `test`-tagged non-test file ⇒ `test-only`; js-only func reached from the app main under js ⇒ `prod`.
- [ ] Implement and pass.

### Task 5: Types, files, areas, packages, coverage, verbosity

**Files:** `rank/structure.go`, `rank/coverage.go`, `rank/verbosity.go`, tests.
**Interfaces — Produces:**
- `type TypeStat struct{ Pkg, Name string; Fields, Methods, FilesSpanned int; PerFile []FileReadiness }`; `type FileReadiness struct{ File string; Methods, FieldsUsed, Exclusive int; MethodCog int }`
- `type File struct{ Path, Pkg string; Lines, Units, TopUnitLOC int; Generated bool }`
- `type AreaStat struct{ Name, Pkg string; Files, Units, SumCog, Over15, Over30, Over60, CloneLOC, DeadLOC, ExtCallees, ExtCallers int; Gini, Coupling, Compound float64 }`; area = first `_` token of the filename, in packages with ≥ 15 files (else the package is one area); a prefix group > 25 files splits by its first two tokens when each subgroup has ≥ 3 files.
- `Compound = Σ cogᵢ^1.3 × (1 + min(1, Coupling/4)) + CloneLOC + DeadLOC`, with `Coupling = (ExtCallees+ExtCallers)/Units`.
- `type Verbosity struct{ RepeatedLines, SimilarCases, MaxChain, MaxCondOps int }`
- `func ApplyCoverage(units []*Unit, profiles []string, root, module string) error` (`cover.ParseProfiles`; block → units by file+line; covered stmts ÷ stmts; merge = any profile).

- [ ] Tests: the fixture god type's file with 9 exclusive fields reports Exclusive=9; the area split rule on a synthetic file list; Gini of [0,0,10] = 2/3; coverage for a unit covered in profile B but not A counts as covered; a unit absent everywhere stays nil.
- [ ] Implement and pass.

### Task 6: Candidates + scoring + weights

**Files:** `rank/weights.go` (one table of every threshold and weight), `rank/candidates.go`, `rank/candidates_test.go`
**Interfaces — Produces:**
```go
type Candidate struct {
  ID, Kind, Target, Area string; Units []string
  Payoff, Confidence, Risk, Score float64
  Breakdown []string; Notes []string; Evidence map[string]any
  Agent *Verdict `json:",omitempty"`; Suppressed string `json:",omitempty"`
}
func Generate(m *Model) []*Candidate
```
- Kinds: `delete-unreachable`, `move-test-only`, `dedupe-clone`, `consolidate-utility`, `split-function` / `flatten` / `simplify-verbose` (one candidate per unit, kind = dominant payoff, others as notes, payoff = max + 0.5·rest), `extract-struct`, `split-file`, `restructure-area`, `data-to-asset`. Generated and data units never yield a complexity kind.
- `risk = 1 + 0.35·ln(1+fanIn) + (1−cov) + 0.5·[jsOnly] + 0.3·[exported]`, cov = 0.5 when unknown; `Score = Payoff·Confidence/Risk`, rounded to 3 decimals; ID = first 12 hex of sha256(kind|target|content hash).

- [ ] Tests: a data table ⇒ only `data-to-asset`; a generated unit ⇒ nothing; the fixture's deep function ⇒ `split-function` with a `flatten` note; ordering is deterministic across two runs.
- [ ] Implement and pass.

### Task 7: Suppressions, verdicts, baseline, reports, packets

**Files:** `rank/suppress.go`, `rank/verdicts.go`, `rank/report.go` (json + md), `rank/html.go` + `rank/rank.html` (embedded), `rank/packets.go`, `rank/baseline.go`, tests; `scripts/gograph/refactor_suppress.json` (empty list).
**Interfaces — Produces:**
- `type Suppression struct{ Target, Kind, Reason, Date string }`; matching = exact target or `path.Match` glob; unmatched ⇒ listed as stale.
- `type Verdict struct{ ID string; Real bool; Kind string; EssentialVsAccidental float64; Clarity, Verbosity int; Proposal, Effort, RegressionRisk string; GuardingTests, Blockers []string; SuppressReason string }`; read from `verdicts/*.json`; malformed ⇒ warning listed in report. Agent factor = `Real ? (0.5+EvA)·{L:1,M:.85,H:.6} : 0.1`; disagreement (`!Real` or kind changed) ⇒ "contested" section.
- `rank.md`: totals, top N unified, per-kind top 10, areas table, god types, contested, suppressed, stale suppressions, baseline delta.
- Packets: top N candidates without a cached verdict ⇒ `packets/<id>.json` (candidate, metrics, source excerpt ≤ 400 lines, callers/callees ≤ 30, test files mentioning the name, coverage, reach).

- [ ] Tests: a stale suppression is reported; a malformed verdict file is a warning; `Real=false` moves the candidate to contested and multiplies by 0.1; the baseline delta reports a resolved candidate.
- [ ] Implement and pass.

### Task 8: CLI, Makefile, CI, workflow

**Files:** Modify `scripts/gograph/main.go` (`rank` subcommand with flags `-root -module -out -cover (repeatable) -baseline -verdicts -suppress -top -fast -packets`), `Makefile` (`refactor-rank`, `refactor-packets`, `gograph-test`; `test:` runs the tool's tests), `.github/workflows/ci.yml` (step "gograph tool tests"); create `.claude/workflows/refactor-assess.js`.
- Workflow: reads `build/refactor/packets/*.json`; batches of 5 per agent (≤ 8 agents) + one module agent per top-3 `restructure-area` + a verifier for every confirmed delete/dedupe; agents write `build/refactor/verdicts/<id>.json` matching the schema; agents never edit source.

- [ ] `make refactor-rank` runs end to end on the repo; `make test` includes `gograph-test` (fast).

### Task 9: Real-repo calibration and manual verification

- [ ] Run `make refactor-rank` (with `-cover coverage/go.out` if present) and check the spec's calibration targets: no generated/data/scene-catalog unit in the top 20; `Game.Update`, `drawGridNodes`, `seqScheduleTime`, `Import`, `updateBeatInfos` are `split-function` in the top 30; `emitProfiles`/`emitDensities` and `clamp`/`clamp01` appear as clones; `drumview_fx_panel.go` is `extract-struct`; unreachable ≈ 389 ± 10 % (now split); 0 merged build-tag ids; two runs byte-identical.
- [ ] Read the code behind the top 25 candidates and a sample of dead/clone findings, and tune `weights.go` where the ranking disagrees with reading the code. Record what changed and why in the spec's appendix.
