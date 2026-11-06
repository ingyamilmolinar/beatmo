# Comment diet: census report and comment policy

Measured 2026-11-03 with `python3 scripts/comment_census.py` (tests: `python3 scripts/comment_census_test.py`;
the lexers agree line-for-line with `go/parser` on all 2,912 Go files and with esprima on the 200 of 207 JS
files it can tokenize). Generated (`*.gen.*`, `Code generated … DO NOT EDIT`) and vendored (`miniaudio`,
`node_modules`, `testdata`) code is excluded. "Comment lines" = lines holding only a comment; directives
(`//go:build`, `//export`, cgo preambles, `//nolint`, shebangs, eslint) are not comments.

## 1. Census

| | files | lines | code | comment-only | trailing | blocks | comment / (comment+code) |
|---|---:|---:|---:|---:|---:|---:|---:|
| Go | 2,912 | 537,133 | 398,283 | 88,194 | 5,566 | 31,939 | 18 % |
| JS | 205 | 56,582 | 40,391 | 11,312 | 407 | 3,167 | 22 % |
| C | 27 | 8,615 | 6,135 | 1,621 | 325 | 447 | 21 % |
| sh / Makefile / yaml / py | 29 | 6,144 | 4,324 | 1,103 | 69 | 292 | 20 % |
| **production** | 1,177 | 268,091 | 191,993 | **54,525** | 3,433 | 17,493 | 22 % |
| **tests** | 1,996 | 340,383 | 257,140 | **47,705** | 2,934 | 18,352 | 16 % |
| **all** | 3,173 | 608,474 | 449,133 | **102,230** | 6,367 | 35,845 | 19 % |

**One line in six in the repo is a comment-only line.**

Block sizes: 15,931 one-liners, 7,116 two-liners, 9,054 blocks of 3–5 lines, and **3,744 blocks of 6+ lines
holding 38,922 lines**. By position: doc comments above declarations hold 51,212 lines, indented (in-body /
field) comments 37,679, floating blocks 8,264, file headers 5,075.

### Largest blocks

| lines | where | what it is |
|---:|---|---|
| 88 | `internal/audio/synth_golden_pinned_test.go:255` | test doc narrating the golden's history |
| 86 | `internal/audio/synth_golden_pinned_test.go:13` | doc on the golden table |
| 62 | `src/js/recording_capture_worklet.js:1` | file header (protocol essay) |
| 58 | `src/js/audio_probe_worklet.js:1` | file header |
| 58 / 57 | `webaudio_audio_isolation_bench` / `webaudio_fx_chain_stress` `.browser.test.js:1` | test-file headers |
| 53 | `cmd/refs/diagnose_acceptance_test.go:14` | "THE acceptance gate" essay |
| 52 | `src/js/chain_per_instrument_traces.browser.test.js:119` | in-body narrative |
| 50 | `src/c/modular_stages.c:1067` | modal-drumhead physics essay |
| 50 | `internal/audio/fingerprint/thresholds.go:8` | calibration provenance |
| 48 | `internal/env/env.go:1` | package header listing every variable |
| 43 | `internal/deploy/asset_size_test.go:68` | dated size measurements ("22,606,472 B as of 2026-07-01") |

### Most-commented files (comment-only lines)

| comment | code | file |
|---:|---:|---|
| 1,196 | 3,459 | `src/js/audio.js` |
| 911 | 1,944 | `internal/ui/synth_panel_zone.go` |
| 905 | 2,277 | `internal/ui/eq_panel_zone.go` |
| 709 | 3,216 | `internal/ui/scene_catalog.go` |
| 633 | 2,204 | `src/c/modular_stages.c` |
| 612 | 1,702 | `internal/ui/drumview_overlay_inst_comp.go` |
| 513 | 1,801 | `internal/ui/game_node_sidebar.go` |
| 507 | 1,007 | `src/c/modular.c` |
| 489 | 1,388 | `internal/ui/transport_zone.go` |
| 429 | 762 | `internal/ui/game_group_menu.go` |
| 421 | 670 | `Makefile` |
| 398 | 460 | `internal/ui/drumview_tree.go` |

Densest (≥ 100 code lines): `ui/runtime_profile.go` 55 %, `ui/parity_timing_liveedit_test.go` 53 %,
`deploy/asset_size_test.go` 53 %, `audio/synth_golden_pinned_test.go` 48 %, `ui/game_struct.go` 47 %,
`ui/drumview_tree.go` 46 %.

### Most-commented packages

| comment | of which tests | code | package |
|---:|---:|---:|---|
| 58,937 | 30,050 | 255,211 | `src/go/internal/ui` |
| 9,979 | 7,068 | 33,311 | `src/js` |
| 9,378 | 4,403 | 40,355 | `src/go/internal/audio` |
| 4,802 | 1,215 | 16,005 | `src/go/internal/audio/fingerprint` |
| 4,232 | 1,258 | 19,143 | `src/go/cmd/refs` |
| 1,621 | 0 | 6,135 | `src/c` |

`internal/ui` alone holds 58 % of all comment lines.

## 2. What the comments are

A regex pass over every block's text (a block can carry several signals):

| signal | regex gist | comment lines in matching blocks |
|---|---|---:|
| cross-reference | `foo.go`, `TestXxx`, "see", "mirrors", "guard:" | 37,252 |
| history | "used to", "pre-fix", "regression", "reported", "Phase 5b", "no longer" | 14,436 |
| measurement | `22 ms`, `1.3 MB`, `+15.6 dB`, `36-40 xruns` | 10,879 |
| dated | `2026-09-19` | 3,846 |
| any of the above | | **58,737 (57 %)** |

Samples show the same shapes everywhere:

- **What, restated.** `// Release.` above `dv.handleEQCurveDrag(..., false)`; `// Register a zone with a handler that
  returns InputIgnored.` above exactly that; `// outlineCountOfColor counts OUTLINE (filled=false) calls matching
  col …`; trailing `ZTransport = 100 // transport zone`.
- **History that git already holds.** `// runtime_profile.go — Phase 5b of the design-system refactor. LayoutProfile
  historically mixed …` (241 comment lines, 197 code lines); `// Phase 4 — tests for the Synth tab …`; `// ── Phase-8E
  unison tests ──`.
- **Promises that have already expired.** `// … Phase 7 retires the old names; for now both coexist in the bridge.`;
  `// A future PR may extract these fields …`; `// see parityTimingCheck, added by a later task`.
- **Numbers that drift.** `// main.wasm raw size. Stripped: 22,606,472 B (21.56 MiB) as of 2026-07-01; …` (43 lines)
  sitting on a constant that has moved since.
- **A real *why* buried in a story.** `// zones holds POINTERS, not values. … Storing values here … was a latent bug:
  every RegisterZone append could reallocate … See TestDrumViewTree_SetZoneRectSurvivesLaterRegistrations.` Only
  the reason is worth keeping: `// Pointers: zoneMap aliases these entries, and append would orphan them.`

Capping every block at 3 lines alone removes 33,673 lines (19,078 production, 14,595 tests); at 2 lines, 46,471.

## 3. Policy

**The code is the spec.** A comment says *why*, in one or two sentences: a non-obvious choice, the constraint that
forced it, or what breaks if someone "simplifies" it. Never what, never how, never when.

Delete:

1. **What / how.** Restating a name, a signature, or the next line; narrating steps; section banners; a doc comment
   that is the identifier spelled as a sentence. Go's `exported` doc rule is already off (`.golangci.yml`), so a
   bare exported identifier is fine.
2. **History.** Dates, phases, tasks, batches, "used to", "pre-fix", "previously", "historically", "no longer",
   "reported", bug stories, "the old X was …". `git log -L` / `git blame` own history.
3. **State that drifts.** Measurements, sizes, timings, counts, calibration provenance, "as of", "for now", "a future
   PR", TODO without a decision behind it. The constant is the value.
4. **Cross-references.** "See `foo.go`", "mirrors X", "Guard: TestY", spec paths. They go stale on every rename,
   and grep finds the test.
5. **Commented-out code.**

Keep (rewritten to ≤ 2 sentences, ≤ 3 lines):

- The reason for a surprising choice ("Pointers: append would orphan the zoneMap aliases.").
- A hazard ("Must not take seqMu: called from Update under it.").
- An external constraint the code cannot show (a browser or OS bug, a licence or spec rule, a platform quirk).
- A contract the type cannot express: units, ranges, lock ownership.

Exempt (never counted or flagged): toolchain directives (`//go:*`, `// +build`, `//export`, `//line`, `//nolint`,
`//lint:`, cgo preambles, shebangs, eslint / `@ts-` / `global` / istanbul / c8, shellcheck, `# noqa`, `# type:`).

Both production and test code follow the same rules. In a test, the test's name is the spec, so its doc comment
survives only to say why the scenario matters.

## 4. Enforcement

`comment_census.py --check` (added by the plan) fails on:

- any comment block longer than **3** lines;
- any comment line matching the history / drift markers: `\b20\d\d-\d\d(-\d\d)?\b`, case-sensitive
  `\bPhase \d`, and case-insensitive
  `\b(used to|pre-?fix|previously|historically|no longer|for now|as of 20|a future (pr|change)|was (removed|deleted|replaced|renamed))\b`
  (`Phase` is case-sensitive because DSP comments legitimately say "phase 0").

It runs in `make test` and CI with no allowlist, so the sweep ends repo-wide clean. What versus why cannot be
linted. That part lives in the policy text (CLAUDE.md / AGENTS.md) and in review.

## 5. Targets

| | now | target |
|---|---:|---:|
| comment-only lines | 102,230 | ≤ 40,000 |
| blocks > 3 lines | 8,090 | 0 |
| history / drift markers | 3,846+ dated lines | 0 |

That is about −62,000 lines, roughly 10 % of the repo's 608 k lines, with **zero code change**: a per-file digest
of the code tokens must be identical before and after the sweep.

## 6. Decisions (recommendations marked)

1. **Max block length 3 lines** (recommended): "one or two sentences" at ~100 columns, and lintable. 2 lines is
   stricter but splits ordinary two-sentence reasons.
2. **Spec paths in code are deleted** (recommended): specs are found by grep, and a path pointer is exactly the
   cross-reference class that goes stale.
3. **`COVERED-BY-GO:` in thin JS tests stays as one line** (recommended): the JS charter uses it as the *reason* a
   browser test is a stub.
4. **Units and lock contracts are kept** (recommended): the type cannot say "seconds" or "caller holds seqMu", and
   the sweep may not rename code.

## 7. Out of scope (follow-up)

`CLAUDE.md` (1,169 lines) and `AGENTS*.md` carry the same disease: dated incidents, measurements, and test-name
pointers. Agents copy that register into code comments. After the sweep, the same policy applied to the agent docs
is the natural next diet. The plan only adds the policy and fixes the doc pointers that would dangle.
