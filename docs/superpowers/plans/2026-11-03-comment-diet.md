# Comment Diet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut comment-only lines from 102,230 to ≤ 40,000 with zero code change, leaving only one- or two-sentence *why* comments, then keep it that way with a lint in `make test`.

**Architecture:** `scripts/comment_census.py` (already built and tested) gains three modes: `--check` (policy lint), `--digest` / `--compare` (proof that only comments changed), and `--only` / `--tests-only` (chunk selection). The repo is then swept chunk by chunk, each chunk gated by digest-equal + check-clean + its package tests. The last task wires `--check` into `make test` and CI.

**Tech Stack:** Python 3.11 stdlib (unittest, tokenize), bundled Go (`.tools/go/bin/go`, `gofmt`), node 22 (`node --check`).

**Spec:** `docs/superpowers/specs/2026-11-03-comment-diet-design.md` (census, policy §3, enforcement §4, targets §5, decisions §6).

## Global Constraints

- The sweep changes comments only. `--compare` must report no file for every chunk. A comment edit that needs a code change (a rename to carry a unit) is out of scope.
- A surviving comment is ≤ 2 sentences and ≤ 3 lines (`MAX_BLOCK_LINES = 3`), says why, and carries no history or drift marker (spec §4 regexes, verbatim).
- Never edit generated files (`*.gen.*`, `Code generated … DO NOT EDIT`), vendored code (`src/c/miniaudio.*`, `node_modules`) or `testdata/`. The census already excludes them.
- Directives are untouchable: `//go:*`, `// +build`, `//export`, `//line`, `//nolint`, `//lint:`, cgo preambles, shebangs, eslint / `@ts-` / `global` / istanbul / c8, shellcheck, `# noqa`, `# type:`.
- Use the bundled Go only. No git stash. No staging or commits: the owner commits.
- Decisions per spec §6: 3-line cap, spec paths deleted, one-line `COVERED-BY-GO:` kept, units and lock contracts kept.
- `$SCRATCH` is any directory outside the repo (digest snapshots must not land in the tree).

## Review Focus

1. **A directive or cgo preamble deleted as "a comment".** Expected: `--compare` flags the file. The digest keeps directive text as code. Pinned in Task 1 by `test_digest_sees_directive_and_cgo_edits`.
2. **A raw-text ratchet drops below its budget.** `TestTokenDiscipline` counts `color.RGBA{` in raw source, comments included, and fails when a file goes *under* budget. Expected: lower the budget in the same chunk, never restore the comment. Pinned by each sweep chunk's package-test step.
3. **The only record of a hazard is deleted with the story around it.** Expected: the reason survives as one sentence. Pinned by sweep step 3: every removed block of ≥ 4 lines containing `because|so that|otherwise|must|never|only|cannot` is condensed, not dropped, unless the code already states it.
4. **Agent docs point at a header comment the sweep trims.** Expected: `CLAUDE.md` states the gotcha itself. Pinned in Task 2 by the pointer grep coming back empty.
5. **gofmt realigns a const or struct block after trailing comments go.** The digest ignores whitespace, so it passes. Expected: `gofmt -l` is still empty. Pinned by sweep step 4.

---

### Task 1: Census gains check, digest and chunk selection

**Files:**
- Modify: `scripts/comment_census.py`
- Test: `scripts/comment_census_test.py`
- Modify: `Makefile` (two targets next to the other dev tools)

**Interfaces:**
- Produces:
  - `MAX_BLOCK_LINES = 3`
  - `HISTORY_MARKERS: list[re.Pattern]`: the three spec §4 patterns.
  - `Violation(path: str, line: int, rule: str, detail: str)` dataclass. `rule` is `"block-too-long"` or `"history-marker"`.
  - `check_file(path: str, text: str) -> list[Violation]`
  - `code_digest(text: str, lang: str) -> str`: sha256 hex of every non-comment character plus the full text of directive lines, with each whitespace run collapsed to one space and the ends stripped.
  - `Line.code_text: str`, filled by all three scanners.
  - CLI: `--only GLOB...` and `--exclude GLOB...` take one or more globs each and are repeatable (`action="extend", nargs="+"`), so a shell brace expansion is one flag; fnmatch semantics (`*` crosses `/`). Also `--tests-only`, `--check` (prints `path:line: rule: detail`, exit 1 on any violation), `--digest` (JSON `{path: digest}` on stdout), `--compare FILE` (prints each path whose digest differs from FILE or vanished, exit 1 if any).
  - Make: `comment-census` = `python3 scripts/comment_census.py $(ARGS)`; `test-comment-census` = `python3 scripts/comment_census_test.py`.

- [ ] **Step 1: Write the failing tests** in a new `class Policy(unittest.TestCase)` and `class Digest(unittest.TestCase)`:
  - `test_block_of_four_lines_is_too_long`: `check_file("p/a.go", "// 1\n// 2\n// 3\n// 4\nx := 1\n")` → one `Violation("p/a.go", 1, "block-too-long", …)`. The same text with 3 lines → `[]`.
  - `test_history_markers_are_flagged`: one violation each for `// fixed 2026-09-19`, `// it used to panic`, `// Phase 5b of the refactor`, `// pre-fix this leaked`, `// for now both coexist`. No violation for `// phase 0 is the start of the cycle`, `s := "2026-09-19"`, `//go:build linux`.
  - `test_trailing_comments_are_checked_for_markers`: `x := 1 // used to be 2` → one `history-marker` at line 1.
  - `test_digest_ignores_comment_edits`: for go / js / c / sh / py, deleting, adding or rewording comments and re-indenting gives an equal digest. For example `"x := 1 // a\n"` and `"// b\nx  :=  1\n"`.
  - `test_digest_sees_code_edits`: changing a number, a string's content, or deleting a statement changes the digest.
  - `test_digest_sees_directive_and_cgo_edits`: deleting `//go:build linux`, `//export Foo`, or one `// #include "x.h"` line above `import "C"` changes the digest.
  - `test_cli_check_and_compare`: in a temp `--no-git` root, `--check` exits 1 and names `p/a.go:1` on a 4-line block. `--digest` > before.json, then edit a comment: `--compare before.json` exits 0 with no output. Edit a code token: it exits 1 and prints `p/a.go`.
  - `test_only_and_tests_only_select_files`: with `p/a.go`, `p/a_test.go`, `q/b.go`, `r/c.go`: `--only 'p/*' --tests-only --json` reports only `p/a_test.go`; `--only 'p/*' 'q/*' --exclude 'p/a_test.go' 'q/*'` reports only `p/a.go`.

- [ ] **Step 2: Run them, expect failures**

  Run: `python3 scripts/comment_census_test.py Policy Digest CLI`
  Expected: `AttributeError: module 'comment_census' has no attribute 'check_file'` (and siblings).

- [ ] **Step 3: Implement.** In `_scan_clike`, append each character to `lines[li].code_text` whenever the state is not `line` or `block`. `_scan_hash` takes the text before the comment start. `_scan_py` takes token strings. `check_file` reuses `analyze` (blocks) plus `scan` (markers on every comment line that is not a directive).

- [ ] **Step 4: Run the whole suite, expect green**

  Run: `python3 scripts/comment_census_test.py`
  Expected: `OK`, including both oracle tests.

- [ ] **Step 5: Baseline.** Run `python3 scripts/comment_census.py --digest > "$SCRATCH/digest-baseline.json"` and `make comment-census ARGS="--check" | wc -l`. Record the violation count in this checkbox: it is the number the sweep drives to 0.

### Task 2: Policy in the agent docs, dangling pointers removed

**Files:**
- Modify: `CLAUDE.md` (new `## Comments` section directly after "Visual Verification of UI Work", plus one Quick Start row for `make comment-census`)
- Modify: `AGENTS.md` (same section, same text)

- [ ] **Step 1: List the pointers.**

  ```bash
  grep -nE 'See `[^`]+`( header| JSDoc)|header comment' CLAUDE.md AGENTS*.md
  ```

  Expected today: the Critical Gotchas lines for `input.go`, `touch.go`, `predictor.go`, `stub.go`, `game_parity_state.go`, `engine_stop.go`, `real_input_test_helpers.js` and `testutil_test.go`.
- [ ] **Step 2: Rewrite each pointer** so the doc line states the fact in at most one sentence and drops "See X header".
- [ ] **Step 3: Add the section.** Spec §3 in ≤ 12 lines: the code is the spec; why only, ≤ 2 sentences / 3 lines; the five delete classes; the four keep classes; `make comment-census ARGS=--check` must pass.
- [ ] **Step 4: Verify.** The Step 1 grep prints nothing.

### Sweep procedure (Tasks 3–11)

Each sweep task names its selection `SEL` (the `--only` / `--exclude` / `--tests-only` / `--no-tests` flags). For a chunk:

1. `python3 scripts/comment_census.py $SEL --digest > "$SCRATCH/before-<task>.json"` and `python3 scripts/comment_census.py $SEL --top 60` for the work order (largest `comment` first).
2. Edit each file with the Edit tool following spec §3. Delete what / how / history / drift / cross-reference / commented-out code. Rewrite surviving reasons to ≤ 2 sentences. Trailing comments get the same treatment. Never touch a code token.
3. Before removing any block of ≥ 4 lines containing `because|so that|otherwise|must|never|only|cannot`, either keep its reason as one sentence or confirm the code already states it.
4. `.tools/go/bin/gofmt -l <changed .go files>` prints nothing. Run `gofmt -w` if it does.
5. `python3 scripts/comment_census.py $SEL --compare "$SCRATCH/before-<task>.json"` prints nothing, exit 0.
6. `python3 scripts/comment_census.py $SEL --check` prints nothing, exit 0.
7. Build and test what the chunk touched:
   - Go: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod <pkgs>` and `../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
   - JS: `node --check <file>` per changed `.js` / `.mjs`.
   - C: `make` the C library target the Go build links.
   A ratchet that fails because a count went *down* gets its budget lowered here (Review Focus 2).
8. Write the chunk's before → after comment-only lines into its checkbox (from `--json` `totals.comment_lines`).

### Task 3: `internal/ui` production — zones, layout, grid, game, menus, input

`SEL = --no-tests --only 'src/go/internal/ui/'{drumview,row,timeline,transport,layout,zone,grid,game,node,sidebar,popup,menu,overlay,portal,input,touch,mobile,keyboard,hit}'*'` (13,435 lines today; `drumview_tree.go`, `game_node_sidebar.go`, `transport_zone.go`, `game_group_menu.go` lead).

- [ ] Steps 1–8 of the sweep procedure. Packages for step 7: `./internal/ui/...`.

### Task 4: `internal/ui` production — synth, EQ, chain, render, audio panel

`SEL = --no-tests --only 'src/go/internal/ui/'{synth,eq,chain,render,audio,sampler,scope,spectrum,fx,insert,wave,levels,meter,panel,param,knob,dial,soundview}'*'` (6,779; `synth_panel_zone.go` 911 and `eq_panel_zone.go` 905 lead).

- [ ] Steps 1–8. Packages: `./internal/ui/...`.

### Task 5: `internal/ui` production — the rest

`SEL = --no-tests --only 'src/go/internal/ui/*'` plus `--exclude` with every Task 3 and Task 4 glob (8,673; `scene_catalog.go` 709, `runtime_profile.go` 55 % lead).

- [ ] Steps 1–8. Packages: `./internal/ui/...`.

### Task 6: `internal/ui` tests — Task 3's prefixes

`SEL` = Task 3's `--only` globs with `--tests-only` (9,611).

- [ ] Steps 1–8. Packages: `./internal/ui/...`.

### Task 7: `internal/ui` tests — Task 4's prefixes

`SEL` = Task 4's `--only` globs with `--tests-only` (6,749).

- [ ] Steps 1–8. Packages: `./internal/ui/...`.

### Task 8: `internal/ui` tests — the rest

`SEL` = Task 5's selection with `--tests-only` (13,690).

- [ ] Steps 1–8, then the whole ui suite once: `go test -tags test -modfile=go.test.mod ./internal/ui/...`.

### Task 9: `internal/audio` and its subpackages

`SEL = --only 'src/go/internal/audio/*'` (about 15,700, tests included; `synth_golden_pinned_test.go`'s 88- and 86-line blocks and `fingerprint/thresholds.go` lead).

- [ ] Steps 1–8. Packages: `./internal/audio/...`. Then the real build, `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/...`, because the `!test` guards (`TestEveryDialIsMeasurable`, `TestRenderAsShippedMatchesPlayback`, …) are invisible to `-tags test`.

### Task 10: Browser JS and C DSP

`SEL = --only 'src/js/*' --only 'src/c/*'` (11,312 + 1,621; `audio.js` 1,196 leads; most JS test files open with a 34–62-line header).

- [ ] Steps 1–8. Shrink each `COVERED-BY-GO:` block to one line (spec §6.3). Then `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` and `make js-audit`.

### Task 11: Everything else — `cmd/`, `core/`, remaining `internal/`, scripts and build files

`SEL = --exclude 'src/go/internal/ui/*' --exclude 'src/go/internal/audio/*' --exclude 'src/js/*' --exclude 'src/c/*'` (about 14,000: `cmd/refs` 4,232, `cmd/gen-showcase`, `core/engine`, `internal/timeline`, `internal/deploy`, `internal/env`, `Makefile` 421, `scripts/*.sh`, `ci.yml`).

- [ ] Steps 1–8. Packages: `./cmd/... ./core/... ./internal/...`, plus `make -n test` to prove the Makefile still parses.

### Task 12: Lock it in

**Files:**
- Modify: `Makefile` (`test:` runs `test-comment-census` and `comment-census ARGS=--check`)
- Modify: `.github/workflows/ci.yml` (same two targets, as their own step)
- Modify: `docs/superpowers/specs/2026-11-03-comment-diet-design.md` §5 (add an "achieved" column)

- [ ] **Step 1:** `python3 scripts/comment_census.py --check` prints nothing repo-wide. `--compare "$SCRATCH/digest-baseline.json"` lists only files edited outside this plan since Task 1, and each listed file is explained.
- [ ] **Step 2:** Wire both targets into `make test` and CI. The CI ↔ Makefile drift guard (`internal/deploy/ci_make_target_drift_test.go`) must stay green: `go test -tags test -modfile=go.test.mod ./internal/deploy/`.
- [ ] **Step 3:** Full verification: `make test`, `make test-real`, `make js-audit`. Report any failure by name.
- [ ] **Step 4:** `python3 scripts/comment_census.py --json` → fill in §5 achieved numbers (target ≤ 40,000 comment-only lines, 0 blocks > 3 lines, 0 markers).
