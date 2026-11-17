# Refactor ranking — `gograph rank` (design)

Status: proposed 2026-10-19. Extends `scripts/gograph` (the `make go-graph` tool).

## Intent

Produce a **ranked list of the highest-value refactoring candidates** in the Go
module (`src/go`): places where complexity is real and removable, code that is
unclear or more verbose than it needs to be, and whole modules whose complexity
compounds. The point is to shrink overall complexity **without changing
behaviour**: every candidate must carry enough evidence (callers, build configs,
coverage, guarding tests) for the refactor to land with no regressions.

Automate as much as possible in deterministic software. On top of that, an
**optional agent layer** reads the top candidates the way a reviewer would. It
separates essential complexity from accidental complexity, catches false
positives, judges clarity and verbosity, and assesses whole modules. Those
verdicts feed back into the ranking.

## Why now (measured 2026-10-19)

`make go-graph` did not build: `golang.org/x/tools v0.25.0` fails under Go 1.26
(`tokeninternal` struct-layout assertion). Nothing in `make test`/CI runs the
tool, so it rotted silently. It was bumped to v0.38.0 to run it. Accuracy
problems found in the current tool:

1. **Build-tag variants merged.** The metrics pass walks every file and ignores
   `-goos`/`-tags`, while the call pass honours them. 214 function ids are
   declared more than once (`Game.SimulateClickAt` ×2, …), and their LOC is
   double-counted. `initJSGraphUI` (js-only) reports cyclo 474 with call
   depth 1.
2. **Closures counted inconsistently.** Cyclomatic counts FuncLit bodies into the
   parent while nesting skips them.
3. **Call depth is noise for ranking.** Test helpers like `Game.SimulateDrag`
   score 24. Fan-in and fan-out answer the question that matters (blast radius).
4. **Package "LOC" is a sum of declaration spans** (no comments or imports), and
   nothing labels it that way.
5. A stale 6.7 MB binary `scripts/gograph/gograph` is committed.

A throwaway probe (linux config, non-test files) measured what better signals
would show. These numbers are the calibration targets below:

| Signal | Result |
|---|---|
| Cognitive complexity | reorders the top: `Game.Update` 423, `drawGridNodes` 384, `seqScheduleTime` 294, `Import` 289, `updateBeatInfos` 283 |
| Whole-func structural clones (≥ 8 LOC) | 90 groups, ~1,564 redundant LOC (`emitProfiles`/`emitDensities` 67 LOC; 8× `clamp`, 7× `clamp01`; `toggleSettingsOverlay`/`toggleLogsPanel`/`toggleThemeOverlay`) |
| Repeated 6-statement windows (≥ 10 lines) | 154 distinct shapes |
| Unreachable (x/tools `deadcode`, linux all-mains ∩ wasm app) | 389 funcs, ~3,200 LOC; 51 are `*ForTest` |
| LCOM4 (connected method components) | **useless here**: `DrumView` = one 521-method component even with hub fields excluded |
| Per-file field exclusivity | actionable: `drumview_fx_panel.go` 34 methods / 16 exclusive fields; `synth_macro_render.go` 19 of 24; `drumview_context_menu.go` 60 methods / 16 |
| ≥ 7 params | 75 funcs |
| Data-shaped giants | `templateSpecs` 890 LOC cc=1, `ModularSynthParamDefs` 490 LOC: long, but not complex |

## Goals

- **G1 Correct base metrics.** One source of truth for syntax: the
  `packages.Load` result, per build configuration. Build-tag variants appear
  once each, tagged with their configs. Closures are their own units.
- **G2 Ranked candidates.** Each candidate has an **action**, a **payoff**, a
  **risk**, a **confidence**, and **evidence**. The score is explainable as a
  breakdown, never a bare number.
- **G3 Compounded module complexity.** Rank packages **and areas** (file-prefix
  groups inside big packages, e.g. `drumview_*`, `synth_*`, `eq_*` in
  `internal/ui`), not only functions.
- **G4 Clarity and verbosity signals.** Find code that is long or repetitive
  relative to what it does, not only branchy code.
- **G5 Optional agent assessment.** Verdicts are cached and merged back into the
  ranking. Dismissed false positives stay dismissed, each with a reason.
- **G6 Deltas.** `-baseline <old rank.json>` reports what each refactor removed.
- **G7 It can't rot again.** The tool's fixture tests run in `make test`.

## Non-goals

- Automatic refactoring or codemods. The tool only reads.
- C (`src/c`) and JS (`src/js`) analysis in v1. The model leaves room for them
  (units carry a language), but v1 is Go only.
- Replacing the go-graph visualisation. It gains accurate metrics and two tabs.
- A complexity gate that fails `make test` (see "Ratchet" under my calls).

## Anti-goals (failure even if it "works")

- A top list dominated by **generated code, data tables, or test scaffolding**
  (`templateSpecs`, `*.gen.go`, `scene_catalog.go` setups). These are long but
  not complex.
- A dead-code claim that is wrong for wasm, tests, or string-named registration,
  leading to a deletion that breaks a build or a browser test.
- Non-deterministic output: same tree means byte-identical JSON and Markdown.
- The agent layer becoming *required*. The deterministic report must stand on
  its own.
- The agent editing code or the tool silently trusting an agent verdict. Agent
  output adjusts a candidate's rank and shows its reasoning. It never deletes a
  candidate without a recorded reason.

## Constraints

- Bundled Go (`.tools/go/bin/go`). The tool stays its own module
  (`scripts/gograph/go.mod`) and never imports beatmo packages.
- Layer 1 is offline and deterministic. Runtime target ≤ 60 s for the whole
  module without coverage. Measured: load ≈ 3 s per config, deadcode/RTA ≈ 6.5 s
  per config.
- Two production build configs: **linux** (all `cmd/...` mains) and **js/wasm**
  (the app `./cmd`). The `test` tag is excluded from production metrics because
  it selects stubs.
- Read-only on the repo apart from `build/refactor/` outputs and the committed
  suppression file (below).

## Architecture

```
L0 load   ── packages.Load × {linux, js/wasm} (+ tests for reachability only)
L1 metrics── units (funcs, methods, closures) · types · files · areas · packages
           · clones · reachability · optional coverage
L2 rank   ── candidate generators per action kind → score = payoff·confidence / risk
L3 assess ── (optional) agents over top-N packets → verdicts.json (cached)
L4 report ── rank.json · rank.md · HTML "Candidates" + "Modules" tabs · baseline delta
```

CLI: `gograph rank [-cover coverage/go.out] [-verdicts build/refactor/verdicts.json]
[-baseline old.json] [-top N] -out build/refactor/` (a subcommand; the current
behaviour stays the default). Make targets: `make refactor-rank` (L0–L2, L4)
and `make refactor-packets` (writes L3 inputs). The agent run itself is a
Claude Code saved workflow (see L3).

### L0 — loading

- `packages.Load` with `NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps`, once per
  config. **Metrics come from the loaded syntax**, so the metrics pass and the
  call pass can never disagree about which files exist again.
- **Unit identity** = `file:line:col` of the declaration plus a canonical name
  (`pkg.Recv.Name`, closures `pkg.Recv.Name.func1`). A unit seen in both configs
  is one unit with `configs: ["linux","js"]`. Two build-tag variants of one name
  are two units sharing a name, each with its config. Package and area totals
  count each unit once.
- **Generated files** (`^// Code generated .* DO NOT EDIT\.$`, as Go defines it)
  are measured and reported in totals but **excluded from ranking**.
- Non-test production files only, except for the reachability pass.

### L1 — metrics

**Per unit** (func, method, closure; a closure's metrics are excluded from its
parent's):

| Metric | Notes |
|---|---|
| `loc`, `sloc` | span; non-blank, non-comment lines |
| `cyclo` | McCabe (as today, but closures excluded) |
| `cognitive` | Sonar cognitive complexity: structures +1+nesting, `else`/`else if` +1, each run of mixed `&&`/`\|\|` +1, labelled `break`/`continue`/`goto` +1, recursion +1. The probe's approximation is the starting point; pin it against the spec's worked examples |
| `nesting` | max depth (as today) |
| `params`, `results` | |
| `fanIn`, `fanOut` | distinct in-module callers / callees, type-resolved. Interface calls attribute to the interface method; RTA (below) supplies concrete targets for reachability |
| `callDepth` | kept for the visualisation only, **not** used in ranking |
| `dataRatio` | share of the body's lines inside composite literals. Units with `cyclo ≤ 3` and `dataRatio ≥ 0.7` are classified **data**: they leave complexity rankings and can only produce the low-priority `data-to-asset` action |
| verbosity signals | see below |
| `coverage` | optional, statement coverage via `golang.org/x/tools/cover` (the parsing `scripts/covgraph` hand-rolls). `-cover` is **repeatable**: profiles are merged per block, and a block counts as covered if any profile covers it. This matters because `make coverage-go` runs `-tags test` (stubbed ebiten and C), so the `!test` files are absent from it; an untagged profile (e.g. the real-C `internal/audio` run) can be added. A unit **absent from every profile counts as unknown (0.5), never 0 %**. Js-only units are always unknown |

**Verbosity and clarity signals** (G4). These are deterministic proxies; the
agent layer judges them for real:

- **intra-unit repetition**: statement windows repeated *within* the same unit
  (same shape, ≥ 3 statements, ≥ 2 occurrences). This is the "copy-paste inside
  a long function" smell.
- **similar case bodies**: `switch` arms whose bodies share a shape (could be a
  table or a helper).
- **selector-chain depth**: max and mean depth of chains like
  `g.drum.Rows[i].X.Y` (reaching through objects).
- **condition weight**: max operators in one boolean condition.
- **length vs logic**: `sloc / cognitive` very high (long but simple) means
  verbose. Very low (short but dense) means unclear.

**Clones** (cross-unit):

- Two tiers. **Exact**: identifiers renamed canonically, literals kept, so it
  catches true copy-paste. **Structural**: literals abstracted. This tier is
  lower confidence and catches same-shape switch tables that hold different data
  (the probe's `Level.String` vs `StageLabel` false positive), so it is never
  ranked above an exact clone of equal size.
- Granularity: whole units ≥ 8 lines, plus statement windows of ≥ 6 statements
  and ≥ 10 lines. Overlapping windows **merge into maximal regions** before
  reporting, so one long clone is one finding, not twenty.
- A cross-package clone group becomes a `consolidate-utility` candidate
  (`clamp`/`clamp01` ×15).

**Reachability** (RTA over SSA, `golang.org/x/tools/go/callgraph/rta`, the
algorithm `cmd/deadcode` uses, called as a library):

- Roots: linux = every `main` under `cmd/...`; js/wasm = `./cmd`. A unit is
  **unreachable** only if unreachable in *both* configs.
- A third pass with test mains as roots splits that set into **dead** (reached
  by nothing) and **test-only** (reached only from tests: belongs in `_test.go`
  or `export_test.go`, or is a seam that should be documented as such).
  **Test configs (my call): the union of BOTH** the tagged build (`-tags test
  -modfile=go.test.mod`, where ~721 of ~850 test files live) and the untagged
  build (the ~120 `!test` real-build suites, e.g. `export_import_master_audio_*`
  and the real-C `internal/audio` tests). A unit is **dead** only if neither
  production config and neither test config reaches it. Using one test config
  alone would label code that only the other config's tests use as dead. Callers
  in non-`_test.go` files tagged `test` count as test callers.
- Non-test files tagged `test` (26 of them) are test code living in production
  directories. They are excluded from production metrics, which follows from the
  production configs. Files tagged `test || js` are production code through the
  js config.
- The test-reachability pass costs two more loads plus SSA over the
  `internal/ui` test package, so it gets its own budget (≤ 120 s more).
  `-fast` skips it, and every unreachable unit is then reported as
  `unreachable (unsplit)` with confidence capped at medium.
- Known blind spots, which become evidence for the agent rather than
  suppressions: string-named registration (`js.FuncOf` / `js.Global().Set`
  names, scene catalog by name, `reflect`, `//go:linkname`, `go:generate`
  tools). Units in files that register JS exports get confidence capped at
  "medium".

**Per type**: fields, methods, files spanned, and **per-file extraction
readiness** = for each file holding methods of the type, its fields used only
by that file's methods (exclusive) vs shared. LCOM4 is **not** used (measured
useless here).

**Per file**: lines, unit count, top unit's share, generated flag.

**Per area and package (compounded complexity, G3).** An **area** is a group of
files sharing a prefix inside one package (the `filePrefix`/`buildGroups` idea
from `scripts/covgraph`: `drumview_`, `synth_`, `eq_panel_`, `game_`, …). A
package with fewer than ~15 files is its own single area. Each area and package
gets:

- `Σ cognitive`, and the count of units above the cognitive thresholds (15 / 30 / 60)
- **concentration**: Gini of unit cognitive (is the complexity spread out, or
  sitting in a few monsters?)
- **coupling**: distinct callees outside the area, distinct callers into it,
  and how many fields of a god type it touches (an area reaching into 100
  `DrumView` fields is entangled)
- clone LOC and unreachable LOC inside it
- **compound score** = superlinear in unit cognitive (`Σ cogᵢ^1.3`, so one
  cognitive-300 function outweighs twenty cognitive-15 ones) × (1 + normalised
  coupling) + clone LOC + unreachable LOC. The exponent and weights live in one
  table (`weights.go`) and are tuned against the calibration targets below.

### L2 — candidates and scoring

One generator per **action kind**; each emits candidates with a stable `id` =
hash(kind, canonical target, content hash of the involved code). The id stays
stable while the code is unchanged and moves when it changes, which is what
verdict caching needs.

| Action | Generated from | Payoff (complexity removed) | Risk inputs |
|---|---|---|---|
| `delete-unreachable` | RTA dead set | its LOC + cognitive | JS-registration file, exported, configs |
| `move-test-only` | RTA test-only set | LOC moved out of production | low |
| `dedupe-clone` | exact/structural clone groups, same package | (n−1) × region LOC | n call sites, fan-in, tier |
| `consolidate-utility` | cross-package clone groups | (n−1) × LOC | packages touched |
| `split-function` | cognitive ≥ 30 or sloc ≥ 150 | cognitive above 15 for the unit | fanIn, coverage, configs |
| `flatten` | nesting ≥ 5, or cognitive ≫ cyclo | nesting-driven share of cognitive | coverage |
| `simplify-verbose` | verbosity signals | repeated LOC + chain/condition excess | coverage |
| `extract-struct` | type × file readiness (exclusive ≥ 8 fields) | fields + methods moved off the god type | shared-field count, files |
| `split-file` | file lines ≥ 1,500 | lines above 800 | low (mechanical) |
| `restructure-area` | top compound areas | the area's compound score | coupling |
| `data-to-asset` | data units ≥ 200 LOC | LOC | low, and ranked last by design |

**Score** = `payoff × confidence / risk`. In this formula:

- `risk = 1 + w₁·log(1+fanIn) + w₂·(1−coverage) + w₃·[js-only] + w₄·[exported]`.
  Coverage is treated as 0.5 when no profile is given. Js-only code is risky
  because the desktop tests don't compile it.
- `confidence` ∈ {high 1.0, medium 0.6, low 0.3} per generator rule, then
  multiplied by the agent factor when a verdict exists.

Output: **one unified ranked list** plus per-action and per-area lists. Every
row shows its breakdown (`payoff 412 = cognitive 387 + …; risk 2.1 = fanIn 14 →
+0.9, coverage 61% → +0.4`). Ties break by id for determinism.

**Suppressions**: `scripts/gograph/refactor_suppress.json` (committed): a list
of `{id-or-target-pattern, kind, reason, date}`. It follows the
`synth_dial_known_weak.json` ratchet style: a suppression whose target no longer
exists is reported as stale. Agents propose entries; a human (or the user's
instruction) accepts them.

### L3 — agent assessment (optional)

**Packets.** `make refactor-packets TOP=40` writes
`build/refactor/packets/<id>.json`. Each packet holds the candidate, its metrics
and breakdown, the code (unit, clone regions, or area file list with sizes),
callers and callees (names + file:line), configs, coverage, the test files in
the package that mention the unit's name, and any reachability caveats. The
packet is self-contained, but the agent may read the repo.

**Driver.** A Claude Code **saved workflow** `.claude/workflows/refactor-assess.js`.
The user starts it by name ("run the refactor-assess workflow"):

- Candidates in batches of ~5 per agent (≤ 8 agents, within the session's
  default workflow size), most-ranked first. Only candidates whose
  `id` has no cached verdict are sent, so a re-run after a refactor reassesses
  just what changed.
- **Module agents**: one per top-3 `restructure-area` candidate. Each reads the
  area and proposes a concrete restructuring (which types and files, what moves
  where, which seams already exist). This is the "entire modules" assessment.
- **Verifier pass** for every `delete-unreachable` and `dedupe-clone` the
  candidate agents confirm: an adversarial agent tries to show the deletion or
  merge would break something (string-named use, wasm-only caller, a test
  asserting distinct behaviour, a subtly different clone).
- Agents never edit files. The workflow writes `build/refactor/verdicts.json`.
  **Verdicts stay local (user decision 2026-10-19)**: `build/` is gitignored, so
  verdicts are a per-checkout cache. They are not committed, and a fresh clone
  starts with none. Only accepted suppressions are committed.

**Verdict schema** (per candidate):

```json
{
  "id": "…", "real": true,
  "kind": "split-function",            // may re-classify
  "essential_vs_accidental": 0.3,      // 0 = all essential, 1 = all accidental
  "clarity": 2, "verbosity": 4,        // 1–5
  "proposal": "Split Update into input / sequencer / UI phases; the three already share no locals past line 412.",
  "effort": "M",                       // S | M | L
  "regression_risk": "M",
  "guarding_tests": ["TestLane…", "…"],
  "blockers": ["js export by string name in js_exports_graph_ui.go:88"],
  "suppress_reason": null              // set ⇒ proposed suppression
}
```

**Merge.** `gograph rank -verdicts …` multiplies confidence by an agent factor
derived from `real`, `essential_vs_accidental`, and `regression_risk`. It shows
agent and tool disagreements in a "contested" section instead of hiding them,
and lists proposed suppressions for acceptance.

### L4 — reports

- `build/refactor/rank.json`: totals, units, areas, candidates, verdicts.
  Deterministic and sorted.
- `build/refactor/rank.md`: a summary of totals (Σ cognitive, clone LOC,
  unreachable LOC, units over thresholds, largest god types), then the top-N
  unified list with breakdowns, the area table, and contested/suppressed
  sections. It should be readable in a terminal.
- HTML (`build/go-graph.html`): a **Candidates** tab (sortable, filter by action
  and area, click to open the unit in the existing drill-down) and a **Modules**
  tab (area treemap sized by compound score).
- `-baseline old.json` adds a **delta** section: per-total change, candidates
  resolved, new candidates. This is how a refactor shows it removed complexity.

## Testing

- Extend `testdata/fixturemod` with one deliberate specimen per metric, each
  with a hand-computed answer: Sonar cognitive worked examples, a closure-heavy
  func, a build-tagged pair (`_js.go` + `!js`), a generated file, a data table,
  exact and structural clones (including the same-shape-different-data
  false-positive pair), an unreachable func, a test-only func, a string-registered
  func, a god type with one extractable file, an area. A golden `rank.json`
  covers the fixture, and runs twice are byte-identical.
- The fixture tests join `make test` (`go test ./...` in `scripts/gograph`, well
  under 5 s) **and get their own CI step** (CI runs untagged `go test` in
  `src/go`, not `make test`). They also catch the next x/tools/Go
  incompatibility the day it happens.
- A real-repo smoke run (`make refactor-rank`), not in `make test`, checks the
  **calibration targets**:
  - No generated, data-classified, or `scene_catalog.go` unit in the top 20
    unified candidates.
  - `Game.Update`, `drawGridNodes`, `seqScheduleTime`, `Import`, and
    `updateBeatInfos` all rank as `split-function` in the top 30.
  - `emitProfiles`/`emitDensities` and the `clamp`/`clamp01` family appear as
    clone candidates.
  - `drumview_fx_panel.go` appears as `extract-struct` on `DrumView`.
  - Unreachable totals stay within ±10 % of the probe (389 funcs), now split
    dead vs test-only.
  - Build-tag duplicates: 0 merged ids.
- The workflow is exercised once by hand on the top 10. Its verdict schema is
  validated by the merge step, which rejects malformed verdicts with a message.

## Housekeeping (part of the work)

- Keep the x/tools v0.38.0 bump (`go 1.24.0` in the tool's go.mod).
- Delete the committed `scripts/gograph/gograph` binary and ignore it.
- Fix the existing go-graph metrics with L0 (build-tag union, closure units),
  so the visualisation stops showing merged variants.

## Decided by the user

- Write it up as a design, automate as much as possible in software, and use
  agents for extra ranking and assessment of candidates: true complexity,
  unclear or verbose code, and entire modules (compounded complexity).
- No regressions: refactors driven by this list must keep every test green. The
  tool itself only reads.
- Agent verdicts are kept only on the local machine (`build/refactor/`), never
  committed.

## My calls (please check)

1. **Extend `scripts/gograph`** with a `rank` subcommand rather than a new tool.
   The loader and metrics are shared, and the visualisation gets the fixes.
2. **Agent driver = a Claude Code saved workflow**, not a script calling the
   Anthropic API. Agents can then read the repo, grep callers, and run things,
   and no API key handling is needed. The packet/verdict files are
   driver-neutral, so an API driver can be added later.
3. **No `make test` complexity gate (ratchet) in v1.** Deltas via `-baseline`
   only. A gate would fail unrelated work in a tree under this much active
   development. It is easy to add once totals are trusted.
4. **Suppressions are committed** (`refactor_suppress.json`) with reasons, so a
   dismissed false positive never returns.
5. **Areas = file-prefix groups** inside packages ≥ 15 files, not a clustering
   algorithm. The probe showed method/field clustering does not separate the
   god types.
6. **Test reachability = tagged ∪ untagged test builds**, with a separate
   ≤ 120 s budget and a `-fast` escape (see Reachability).
7. **Coverage: repeatable `-cover`, missing = unknown (0.5)**, never 0 %.
8. **CI step** for the tool's fixture tests, in addition to `make test`.
9. **Production code only** in rankings. Tests are used for reachability and
   guarding-test evidence, but test code itself is not ranked.

## Left to the builder

Exact weights and thresholds (tuned against the calibration targets), clone
window sizes, the area-prefix rule details, the HTML tab layout, the Markdown
formatting, and the agent prompts (within the schema and roles above).

## Appendix — implementation notes and calibration (2026-10-19)

Built as planned (`docs/superpowers/plans/2026-10-19-refactor-rank.md`):
package `scripts/gograph/rank`, `gograph rank`, `make refactor-rank` /
`refactor-packets` / `gograph-test`, a CI step, and the saved workflow
`.claude/workflows/refactor-assess.js`. On this repo a full run takes about
34 s and peaks at about 4.5 GB RSS (SSA over the `internal/ui` test package
dominates). `-fast` skips the test configs.

**Deviations from the design, and why**

- **HTML**: the report is a standalone `build/refactor/rank.html` (tabs
  Candidates / Modules / Units / Types) rather than tabs inside
  `go-graph.html`. The two commands run separately, and coupling them would
  make every go-graph run pay for RTA.
- **Verdicts are a directory** (`build/refactor/verdicts/<id>.json`, one file
  per candidate, or a JSON array). Agents running in parallel then never write
  the same file.
- **Clone tiers are three, not two.** *exact* renames only the unit's own
  locals, *renamed* renames every identifier (copies that touch different
  fields or functions, such as three toggles over three overlays: medium
  confidence), and *structural* abstracts identifiers and literals. Renaming
  every identifier in the exact tier made any run of same-shaped assignments
  "repeated code": `rebuildThemeAliases`, a table of distinct
  `colX = WithAlpha(genColorY, …)` lines, scored 220.
- **Dev tooling is discounted** (`Weights.ToolShare` 0.75 on candidates
  entirely under `cmd/…`; the app package `cmd` itself is product). It is
  shown in the breakdown.

**False-positive classes found on the real repo and fixed (each has a fixture test)**

1. SSA names explicit `init` functions `init#1`, `init#2`…, so every
   registration `init` (effect registry, descriptor registries) looked dead
   (+2,300 lines). Fixed by mapping `init#N` → `init`.
2. **Build-tag stubs.** `softKeyboardInit` in `softkeyboard_stub.go`
   (`!js || test`) has no linux caller, but the `js && test` build compiles it
   against `js_exports_init.go`. A dead function whose same-named sibling in
   another file is live is now `reach: variant`, never a deletion candidate.
3. **Builds the analysis never compiles.** A reference by name from a file no
   analysed config compiles drops confidence to low and names the file
   (`UnanalyzedRefs`). So does a reference from live code, which means the
   function is used as a value that is never called.
4. **Stale coverage.** `coverage/go.out` was four months old, and its line
   ranges no longer pointed at the same code. A profile is ignored for any
   source file newer than it, with a warning.
5. **`extract-struct` on single-file types** (`EQPanelZone ← eq_panel_zone.go`
   held 91 of the type's 93 methods). A file must hold at most
   `ExtractMaxShare` (½) of the type's methods to be a slice that could move
   out.
6. **`split-file` overweighted** (`scene_catalog.go`, 127 small independent
   setups, was #9). The per-line payoff went from 0.05 to 0.02, and a file
   whose largest unit is ≥ ½ of it is left to `split-function`.
7. **Closure lines counted toward the parent.** `split-function` now uses
   `OwnSLOC` (SLOC minus nested closures, which are units of their own).

**Calibration targets**: met. The top 7 are `drawGridNodes`,
`seqScheduleTime`, `Update`, `updateBeatInfos`, `Import`, `refreshDrumRow` and
`parityScan`. No generated, data or `scene_catalog` unit is in the top 20
(`scene_catalog.go` as `split-file` is #36). `emitProfiles`/`emitDensities`
and the `clamp`/`clamp01` family are clone candidates. `drumview_fx_panel.go`
is `extract-struct` #20. There are 0 merged IDs, and two runs are
byte-identical. go-graph duplicated IDs went from 214 to 3, and those 3 are
legitimate multiple `init`s. The probe's "389 unreachable" figure was an
intersection of two deadcode outputs that silently dropped every
single-config function, so it is not a valid target. The tool reports 98 dead
funcs (733 lines) and 456 test-only funcs (5,123 lines) once the false-positive
classes above are removed.
