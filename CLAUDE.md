# Beatmo

## Overview

**Beatmo** is a graph-driven grid sequencer—a drum machine where nodes on a 2D grid form a directed graph that drives beat patterns in real time. Each node represents a potential drum hit; edges define traversal paths that determine when instruments fire.

**Platforms**: Desktop (Ebiten/Go) and Browser (WASM + WebAudio)

**Toolchain**: Go 1.26, JavaScript/WebAudio, miniaudio C DSP, Emscripten

---

## ⚠️ Visual Verification of UI Work (MANDATORY)

**Every UI change — a new feature, a restyle, or a UI bug fix — MUST be verified by rendering a real screenshot scene and looking at the PNG before it is considered done.** Green Go tests are NOT sufficient for UI: the ebitenstub/`-tags test` path does not prove that text, layout, colors, or button/keycap chrome actually render. Shipping UI on unit tests alone has repeatedly left features "unbaked" — e.g. buttons whose labels were completely blank while every unit test passed, a dead close button, and a badge counting the wrong thing.

The loop for any UI feature or UI bug:

1. **Scene**: add (or reuse) a scene in `internal/ui/scene_catalog.go` whose `Setup` drives the UI into the exact state under change — open the panel/menu/dialog/picker and seed the data it needs. Mirror the one-liner style: `{Name: "logs_panel", Setup: func(g *Game){ g.drum.notifyError("boom"); g.toggleLogsPanel() }}`. Keep the scene in the catalog afterwards as a regression surface.
2. **Capture**: `CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` then `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>` (or `make screenshots-all SCENES=<name>` for the batch harness; `MOBILE=1` for the mobile pass).
3. **Look**: open the PNG and confirm the change actually renders — text present and legible, layout sane, no blank controls, no overlaps, correct colors/active states. Fix and **re-capture until it looks right**.
4. Verify both platforms/orientations when relevant (desktop + `MOBILE=1`).
5. **Isolate prefs when a scene persists user state** (a Theme editor pick writes `prefs.json`): run the capture with `XDG_CONFIG_HOME=<scratch dir>` so a screenshot sweep never edits the developer's real `~/.config/beatmo/prefs.json` (a 2026-09-06 sweep left the real profile on the last preset it captured). For runtime-vs-boot theme parity use `TestRuntimeThemePickMatchesStartup` (`theme_runtime_parity_test.go`) before trusting a capture.

**Never claim a UI change is complete, and never leave the tree, without a screenshot you have actually opened and inspected.** Automated `scene_crop_test.go` / `audio_panel_render_pixels_test.go` pixel checks complement this but do not replace a human/agent looking at the rendered surface.

## Comments — the code is the spec

A comment says **why**, in one or two sentences (at most 3 lines): a non-obvious choice, the constraint that forced it, or what breaks if someone "simplifies" it. Never what or how.

- **Delete:** restated names or next lines, step narration, banners; history (dates, phases, "it used to", bug stories — git has it); drifting state (measurements, sizes, counts, "as of", "for now"); cross-references ("see foo.go", "Guard: TestX", spec paths); commented-out code.
- **Keep:** the reason for a surprising choice; a hazard ("must not take seqMu: called under it"); an external constraint the code cannot show (a browser bug, a licence rule); a contract the type cannot express (units, ranges, lock ownership).
- Tests follow the same rules: the test name is the spec.
- Directives are not comments (`//go:*`, `//export`, cgo preambles, `//nolint`, eslint, shebangs).
- `make comment-census ARGS=--check` must pass (blocks ≤ 3 lines, no history markers); `--digest` / `--compare` prove a comment edit changed no code.

---

## Quick Start

| Task | Command |
|------|---------|
| Install deps | `sudo make dependencies` |
| Comment census / policy lint | `make comment-census` (`ARGS="--check"`, `"--top 60"`, `"--json"`, `"--only src/go/internal/ui/*"`) |
| Desktop run | `make run` |
| Debug run | `make run-debug` |
| Fast Go tests | `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...` |
| Full pipeline | `make test` (runs `make ci-build` first: the CI Build job's exact commands) |
| Check host tools vs the CI pins | `make doctor` (`ci/versions.env` is the ONE place a tool version is pinned: Debian digest, Go, emsdk, golangci-lint, Node, Playwright, Chrome) |
| Run any target in the CI image | `make dev TARGET=<target> [KIND=build\|browser]` (podman or docker; the exact image CI uses, `ci/images/*.Dockerfile`, tag = content hash) |
| Lint (what CI's Lint job runs) | `make lint` = golangci-lint (Go version from go.mod — never pin `run.go` in `.golangci.yml`) + `scripts/ci/lint-unused.py` (the `unused` linter judged across linux, linux `-tags test`, js and js release: a symbol is dead only if every build that compiles its file finds it unused) + eslint over git-tracked JS + cppcheck |
| Reproduce one CI shard | `make test-go-shard SHARD=2/4` / `make test-real-go-shard SHARD=2/4` (`scripts/ci/go-shard.sh plan 2/4` prints what it runs) |
| JS-bridge audit (no typed id/enum reaches `syscall/js`) | `make js-audit` |
| Real Ebiten tests | `make test-real` |
| Single browser test | `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js` |
| Cross-platform parity | `make test-parity` |
| Build WASM | `make wasm` = the SHIPPED binary (`-tags noscenes`: no screenshot scene catalog — deploy + `TestWasmAssetSizeBudget`); `make wasm-dev` = same with the catalog, for anything that calls `runScene`/`listScenes` (`test-browser`/`wasm-all`, the screenshot targets). The scene TYPE + lookups live in `scene_registry.go` (always built); scene DATA files carry `//go:build !noscenes` (guard `TestShippedWasmLeavesOutTheSceneCatalog`) |
| Run the site as beatmo.io serves it (landing demo on every visit) | `make serve-landing` → http://localhost:8090 (hashed wasm stamped into `index.html`, compression, video Range; `/?landing=1` always shows the demo, `/?landing=0` never; `BEATMO_LANDING_HOST=0.0.0.0` to open it from a phone) |
| Re-record the landing page videos | `make showcase-video` (`CUTS=desktop` for one; manual by design — see § Landing page) |
| Landing behaviour (browser, real Chrome) | `GO=$(pwd)/.tools/go/bin/go node src/js/landing_behavior.browser.test.js` (`WASM_FILE=` to serve a prebuilt wasm, `LANDING_SHOTS=<dir>` for screenshots of each layout) |
| Landing playback on phones (browser, real Chrome, NO evaluate gestures) | `GO=$(pwd)/.tools/go/bin/go node src/js/landing_playback.browser.test.js` (`LANDING_PLATFORMS=android,iphone,desktop` default, `iphone-webkit` opt-in; `LANDING_RECORD=<dir>` records the whole loop per platform to `<dir>/<platform>.webm`; `LANDING_ROOT=<dir>` A/Bs another copy of `src/js`) |
| Landing cold-load A/B (throttled) | `BASELINE_INDEX=<old index.html> WASM_FILE=<stripped main.wasm> node src/js/landing_load.browser.test.js` (`LANDING_PROFILE=4g\|slow4g\|desktop`, `LANDING_ARMS=baseline,landing,skip,nolanding,videolate,nocrystal`, `LANDING_ENCODING=br`) |
| Import-cold bench (browser) | `make bench-import-cold` (load circuit → play immediately; → `bench-results/import_cold.json`) |
| Audio-lane A/B (browser) | `make bench-lane-ab` (baseline vs legacy vs lane, interleaved, load-gated → `bench-results/audio_lane_ab_<date>.md`) |
| Live-edit budget (browser, real pointer input during playback) | `GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_live_edit_budget.browser.test.js` (FX add / FX slider hold + drag / EQ band hold + drag on the startup demo; HARD gates on WebAudio graph churn (`window.audioGraphStats()`) and silent notes; main-thread cost reported, `AUDIO_PROBE_STRICT=1` enforces; `TEST_PLATFORM=chrome-local TEST_HEADED=1` = real Chrome with a real GPU, `LIVE_EDIT_CPU_THROTTLE=4` = a slower machine, `LIVE_EDIT_SHOTS=<dir>` = a PNG per phase) |
| Audio-isolation bench (browser) | `make bench-isolation` (UI loads × what is HEARD, via the playback probe → `bench-results/audio_isolation.json`; `AUDIO_PROBE_STRICT=1` enforces lateness) |
| Capture UI screenshots | `make screenshot` (desktop + browser; output → `screenshots/`) |
| Capture all UI scenes | `make screenshots-all` (all scenes; `SCENES=name` to filter, `MOBILE=1` for mobile pass) |
| Capture cropped UI subjects | `make screenshots-all SCENES=$(./tmp/beatmo_screenshot -list-scenes \| grep '^crop_' \| paste -sd ,)` (only `crop_*` subject-cropped scenes) |
| Regenerate design tokens | `make gen-design-tokens` (DESIGN.md → `internal/ui/design_*.gen.go`) |
| Regenerate built-in templates | `make templates` (committed scores → `internal/assets/templates/*.json`) |
| Rebuild the instrument pack | `make asset-pack` (the readable documents in `internal/assets/instruments/*.json` → the embedded `internal/assets/instrumentpack/instruments.pack.json.gz`; drift-tested by `TestAssetPackIsCurrent`) |
| Edit the startup demo | edit `internal/assets/startup_demo.json` directly (or export a circuit from the app over it) — the launch circuit comes ONLY from that file (no code-built fallback, no post-import recolor). No test pins its contents; `TestStartupDemoLoads` only checks it imports. Circuit behaviour is tested on purpose-built circuits (`newMelodicCircuitGame`) and `test_fixture_demo.json`, never on the demo |
| Regenerate instrument fundamentals | `make gen-fundamentals` (renders + measures every instrument → `internal/audio/instrument_fundamentals.gen.go`) |
| Regenerate which Synth dials each instrument shows | `make gen-dial-relevance` (re-measures every shipped sound — a dial whose 0→100 sweep is inaudible or invisible is hidden → `internal/assets/synth_dial_relevance.gen.json`; run after a tuning pass, a new instrument, a dial edit or an engine change — `TestDialRelevanceTableIsCurrent` fails until you do) |
| Report what each instrument's note resolves to | `make fundamentals-report` (per instrument: base Hz, which source won, detector confidence) |
| Verify reference scores upstream | `make scores-verify` (re-fetch each score, compare SHA256) |
| Regenerate score attribution notice | `make scores-notice` |
| Install pre-commit hook | `make install-hooks` (rejects commits that add/modify ANY audio file, and commits with stale `*.gen.go`) |
| Fetch reference audio | `make fetch-refs [TIER=instrument\|song]` |
| Reference coverage report | `make refs-doctor` |
| Re-derive corpus calibrations (thresholds, organic range, f0 confidence, audition candidates) | `make refs-check` (`CHECKS=a,b`, `V=1`; against the local refs — never part of `go test`) |
| Audit instrument families vs their synth counterparts | `make refs-families` (drives off `roster.All`, not the catalogue: family → members, the reference each is auditioned against, its state/f0, the last ear verdict, and `NO REF` / `via <base>` for registered ids with no reference of their own; `INST=<family-or-instrument>` for one) |
| Point an instrument at your own reference WAV | `make refs-adopt WAV=snare-lofi.wav [INST=] [NOTE=] [ID=] [F0=] [START_SEC=] [MAX_SEC=]` — any recording will do (reference audio is local dev tooling, never committed or shipped, so there is no licence); drop the file in `$BEATMO_WAV_REFS/sources`; the **filename names the instrument** (`snare-lofi.wav`, `kick-deep_take3.wav`), `INST=` only when it can't |
| Adopt a whole folder of takes at once | `make refs-adopt-all [NOTE=] [MAX_SEC=]` (adopts every resolvable file in `$BEATMO_WAV_REFS/sources`, reports the rest) |
| Play isolated single-property probe tones (calibrate a threshold by ear) | `make probe-sounds` (`SET=<name>` for one; `go run ./cmd/refs probe` prints the measured axis value per step) |
| Check the perceptual axes against the listener's own WORDS | `cd src/go && ../../.tools/go/bin/go run ./cmd/refs validate -terms` (does the axis claiming to measure "airy"/"warm"/"wooden" fire on the families where that word was written? plus a discovery ranking of which axis actually tracks each word) |
| Grade every analysis metric against the ear verdicts | `cd src/go && ../../.tools/go/bin/go run ./cmd/refs validate` (AUC per descriptor vs `ear_verdicts.jsonl`; `PREDICTIVE` = trust it when tuning, `ANTI` = it points backwards, `SATURATED` = fires on ~everything so it cannot rank) |
| Reference pitch-reliability report | `cd src/go && ../../.tools/go/bin/go run ./cmd/refs doctor -pitch` (which refs have a trustworthy f0; `SUPPRESSED` = harmonic axes report "not measurable" there) |
| A/B listen: real vs synthesized | `make ab INST=<instrument>` |
| List instruments comparable via `make ab` | `make ab-list` |
| Ear-listening session over every instrument family | `make ab-all` (verdicts → `src/go/internal/audio/testdata/ear_verdicts.jsonl`; `RESUME=false` to start over, `INST=<family>` for just one) — every option in a family is level-matched against **one shared** reference clip, so the listen grades timbre and not loudness |
| Clone a struck/modal reference into a candidate instrument | `make clone INST=<instrument> [REF=] [WAV=] [F0=] [AS=<newid> \| INTO=<id>] [NO_REFINE=1] [SEED=] [GENS=]` (analysis: sub-band ESPRIT → matching pursuit → one joint amplitude fit, strike, decay law, residual noise bands; then `clone.Invert` → a document on the MODAL / STRIKE / NOISE stages that reproduces the measurement, and seeded parallel CMA-ES refinement against the reference through `RenderAsShipped` → `$BEATMO_WAV_REFS/clones/<inst>/` candidate.json, meta.json, report.md/json, reference/current/candidate WAVs; refuses clipped / onset-less / short / non-modal input; spec `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md`) |
| Clone the slice-1 benchmark set | `make clone-bench [CLONE_BENCH="a b"] [GENS=]` (local corpus only; ~25–50 s each) |
| Listen to a clone, then ship it | `make ab-session FAMILIES=<inst> CLONE=1` (candidate K + a y/n/p ship prompt, recorded with the document's `doc_sha`), then `make clone-adopt INST=<inst>` (writes `internal/assets/instruments/<id>.json` ONLY on the newest `y` for that exact document; prints the asset-pack / generate / measure-loudness / gen-fundamentals follow-ups) |
| Essence-tune an instrument into a candidate (no shipped file touched) | `make tune INST=<id>[,<id>] [GENS=40] [SEED=1] [HARMONICS=N] [TUNE_WAVS=<glob>]` or `make tune-picks [PICKS=a,b]` (`refs tune`: CMA-ES over the document's OWN continuous values -- partial gains, noise layer, filter, envelopes, FINISH, string/bow -- each in a trust box around its shipped value, minimising the mean RAW essence distance over every recording of the instrument (catalogue + `TUNE_WAVS` files whose name says it), each played the way `refs match` plays it; then a pruning pass reverts every move worth < 1 % together; then T1: shipped vs candidate rank among the whole roster per recording. Picks = `src/go/cmd/refs/testdata/tune_picks.json`: the listener's own note + the map gap per instrument, optional `add_harmonics` (raised from silence), `only` (restrict the search), `set` (edits from the note, e.g. `render.seconds`), `skip` (catalogued recordings filed under the instrument that are not it -- `snare-vcsl` is a snares-OFF hit and dragged the snare toward a tom), `from` (start from ANOTHER instrument's document when the shipped voice is the wrong instrument altogether -- the rimshot was two tuned tones ranked #33 on its recording, so its pick starts from `snare-fat`; the candidate keeps the picked id, and `meta.json` records each recording's `search_pitch` so ab-tuned plays T there and C at its own pitch). Writes `$BEATMO_WAV_REFS/tuned/<inst>/` candidate.json + meta.json + report.md; `-score <doc|dir>` = T1 only. Spec `docs/superpowers/specs/2026-10-19-organic-instrument-fidelity-design.md`) |
| A/B by ear: reference vs current vs tuned candidate, then ship | `make ab-tuned [FAMILIES=a,b]` (`refs session -tuned`: R, C, then candidate T played EXACTLY as `refs tune` scored them -- the recording loaded like `refs match` loads it, both voices HELD as long as it at the search pitch (`session_tuned.go`; the generic compare render plays every voice at 120 BPM, a 1 s oboe against a 2 s note, and was what made the first round sound chopped) -- the RECORDING at its own level (gain 1; `tunedLevelGains` brings the voices to it -- the compare plan met at the average and raised a quiet recording +15.6 dB with its hiss, to a 0.99 peak the DAC clipped between samples), every clip within -1 dBTP (`truePeak`, 4x sinc), each clip checked against the listening contract (`clipFaults`/`holdFault`: silence, clipping, true-peak overs, late start, dropout, end click, a voice cut while the recording holds; a fault prints a WARNING), 0.6 s between clips (`gappedPlayer`), blocking players with a 50 ms buffer (plain paplay drove the USB DAC to 36-40 xruns per round); rated + words from ONE word salad shared by every instrument (`session_questions.go` `wordSalad`; `DEEP=1` / `-deep` also rates the instrument's own `familyDeepWords` 1-5, "1 not X enough · 3 about right · 5 too X", stored turned to each word's axis) + a y/n/p ship gate; RESUMES by default at the first family with no record for EXACTLY its candidate on disk (`reviewedSession`; a re-tuned candidate is pending again, a deep round counts only deep records, `FAMILIES=` always asks, `RESUME=false` asks all; answers are appended, never rewritten), and a family's draft of OTHER candidates (the Sept ab-session snare draft) is never resumed, overwritten or removed (`draftFile`) pinned to the candidate's `doc_sha` → `ear_sessions.jsonl`; guards `TestAbTunedPlaysEveryPickAsItWasTuned` (real build: every pick, the PLAYED files == the scored renders) + `TestCompareRenderBreaksTheHoldContract`), then `make tune-adopt INST=<id>` (`clone adopt -kind tuned`: writes `internal/assets/instruments/<id>.json` ONLY on the newest `y` for that exact document, keeping the shipped `render.amplitude` and no `origin`, and REFUSES a candidate whose `render.seconds` differs from the shipped document's unless its pick `set` it -- a candidate tuned before a length change would undo it; prints the follow-ups: asset-pack, generate, measure-loudness, gen-fundamentals, templates, gen-dial-relevance, wav-match-calibrate) |
| Match your own WAVs to the closest shipped instrument | `make wav-match [WAVS='~/Downloads/*.wav'] [TOP=3] [FORCE=1]` (`refs match`: converts each file like a fetched ref, measures pitch (sub-octave AND sub-twelfth guard: YIN locks onto the 2nd or 3rd harmonic of a weak fundamental; no note when the strong partials are not one harmonic series or none of harmonics 1..5 is within 6 dB of the loudest -- a LOUD low harmonic, not the loudest: a reed's formant peaks higher), plays EVERY instrument the way a player would imitate it -- at the recording's note (a non-melodic voice only within an octave of its natural pitch; anything unpitched pitch-searched within an octave), held as long as the recording sounds (`synthmatch.HeldSeconds`), onset-trimmed like the ref -- and ranks by ESSENCE (`fingerprint.AnalyzeEssence`: colour / its evolution / noise texture / decay profile / attack, loudness across frequency, ENERGY across time, no pitch input -- ERB colour resolves the harmonics; minus each voice's hubness from the committed corpus calibration `cmd/refs/testdata/match_hubness.json`). The table's LABELLED column is what the catalogue/filename says the recording is and where that family ranked. Two-phase: every voice is SCOUTED (its fixed plan -- the note, plus a non-melodic voice's octaves -- or every 6 st of a search), and only the 30 closest + whatever the recording is labelled as SETTLE the full pitch plan (`rankRoster`; `-calibrate` settles everyone); measured voice frames persist in `matches/frames.json` per refs binary -- 0.57x the renders, same grades (spec § "where the time goes": profile, what was cut, what not to retry). Spec + what each old metric got wrong + which terms are signal (§ "what is signal": the legacy timbre/full/perc/fused distances, a harmonic-profile term and a harmonic sieve were measured and removed): `docs/superpowers/specs/2026-10-17-wav-match-essence-design.md`. Cached by file hash + roster sound hash + calibration in `$BEATMO_WAV_REFS/matches/`; never touches `catalogue.json`. `make wav-match-validate` grades it leave-one-out on the catalogued refs (own family #1/top-3/MRR + broad KIND); `make wav-match-calibrate` re-derives the hubness after a retune (also `refs check -only match-hubness`); `TestEssenceSelfIdentifiesUnderNuisance` (real build) = a voice re-played at another pitch/tempo/level in a room must still rank itself top 3) |
| Map recordings ↔ instruments (matches + gaps) | `make wav-match-map [MAP_WAVS='~/Downloads/*.wav,catalogue']` (`refs match -map`; `-wavs` takes comma-separated globs, `catalogue` = every catalogued instrument reference present locally) → `$BEATMO_WAV_REFS/matches/MAP.md`: per recording its top 3 vs what the catalogue/filename says it is; per roster instrument MATCHED / FIDELITY GAP (recordings of it exist, the synth ranks lower — names the losing essence term and whether the voice is darker/brighter, noisier/cleaner, dies faster/holds longer, via `fingerprint.Essence.Traits`) / STAND-IN / UNCOVERED. `refs match -explain inst,inst -wavs <file>` prints every essence term of the top N + those instruments for one recording |
| Prove the matcher ignores nuisances | `make wav-match-nuisance [CASES=a,b\|all]` (`refs match -nuisance`: re-plays shipped voices with ONE nuisance each — pitch ±5, a note cut short (half its shipped length / exactly its shipped length), room, reverb, bright/dark mic, then all at once — and reports each voice's self-rank against the whole roster; a nuisance that moves ranks is leaking into the essence distance) |
| Listen + approve/reject those matches with notes | `make ab-match [ONLY=<filename text>] [RESUME=false]` (REF then MATCH on a blocking player; `y`/`p`/`n` + a note about the synth; a rejection plays the runners-up and asks if one is closer → `src/go/internal/audio/testdata/wav_match_verdicts.jsonl`, one row per recording, pinned to the instrument's sound hash) |
| Second pass: capture WHY a family sounds wrong | `make ab-notes` (revisits `ab-all` verdicts and prompts for prose notes only, in place; `ONLY=n,p` filters verdict classes, `REANNOTATE=true` to redo existing notes, `INST=<family>` for just one) |
| Headless stubbed tests | `make test-xvfb` |
| List browser tests | `make test-browser-list` |
| Go coverage | `make coverage-go` |
| C coverage (gcov, own DSP files) | `make coverage-c` (annotated gaps → `coverage/c/*.c.gcov`; bridge Go coverage → `coverage/go-native.out`) |
| Browser coverage (Go+JS) | `make coverage-browser` |
| JS source coverage | `make coverage-js` |
| All coverage | `make coverage` |
| Coverage HTML reports | `make coverage-report` |
| Coverage graph (DOT/SVG) | `make coverage-graph` |
| Interactive architecture graph | `make go-graph` (packages/types/funcs sized by LOC + nesting/cyclomatic/call-depth + type-resolved call edges → `build/go-graph.html`) |
| Rank refactoring candidates | `make refactor-rank` (`gograph rank`: per-build-config metrics (linux + js/wasm), cognitive complexity, 3-tier clones, RTA dead vs test-only over the tagged AND untagged test builds, god-type extraction readiness, compounded area complexity → `build/refactor/{rank.md,rank.json,rank.html}`; `RANK_FAST=1`, `RANK_BASELINE=old/rank.json` for a delta; existing `coverage/*.out` profiles are used unless older than the source). Agent pass: `make refactor-packets` then ask Claude to "run the refactor-assess workflow" (verdicts → `build/refactor/verdicts/`, local only), then `make refactor-rank` again to merge. Dismiss a false positive for good in `scripts/gograph/refactor_suppress.json` (with a reason). Spec `docs/superpowers/specs/2026-10-19-refactor-rank-design.md` |

**Bundled Go**: `.tools/go/bin/go` — always use this to avoid version drift.

---

## Architecture

### Rendering Pipeline

```
Predictor buffers (source of truth)
    ↓
Timeline service (immutable history)
    ↓
preview.BuildRowWindow (pure function, single precedence table)
    ↓
DrumRow.Steps / CellTypes (render inputs)
    ↓
Row sprite cache (GPU texture)
    ↓
Rows layer composite
    ↓
Screen blit + highlights + UI controls
```

### Key Code Paths

**Audio scheduling**: `game_sequencer_schedule.go` → advances `seqNextIdxs[row]`, emits highlight events to `hlCh`

**UI applies highlights**: `game_sequencer_highlight.go` → `applySequencerHighlight()` advances `nextBeatIdxs[row]`, records timeline commits

**Slate rebuild**: `game_refresh_drum_row.go` → `refreshDrumRow()` ensures predictor horizon, builds Steps/CellTypes via `preview.BuildRowWindow`, marks caches dirty

**Audio lane** (what is HEARD): `audio_lane.go` (reconciler) ← `audio_lane_source.go` (fingerprints) + `audio_lane_note.go` (pure `laneNoteAt`) → `audio_lane_sink.go` (`flushLaneOps` → `audio.PlayBatch` with `VoiceID` / `CancelVoices` / `StopLaneAt`); wired in `audio_lane_wire.go` (`laneTick` deferred in `seqScheduleTime`, `kickLane`)

**Graph changes**: `game_graph_update_beat_infos.go` → `updateBeatInfos()` recomputes paths, calls `predictor.SetPaths()`, seeds timeline history

---

## Key Concepts

### Predictor as Source of Truth

The engine-owned Predictor is the authoritative answer for "what should fire?" The UI reads via `VisibleAt`/`TriggeredAt`; it never mutates predictor state directly.

### Timeline Immutability

Once a beat is played, its rendered state is frozen:
- **The past renders from the played snapshot, never from live state.** `timeline.Service`'s played tier (`internal/timeline/played.go`, 1 byte/cell/row, first-write-wins, whole-session retention) records what each cell SHOWED as the playhead passed it; `preview.BuildRowWindow` renders a past cell that has one verbatim — no predictor, no live path type, no commit, no previous window.
- The freeze (`freezePlayedThroughPlayhead`, `game_played_snapshot.go`; rule `preview.CrossingCell`) must run BEFORE anything can change what an unfrozen cell would show: `setPrimaryStep` (playhead advance), top of `refreshDrumRow` and `updateBeatInfos`, and `bumpParityGen` (every structural mutation). Never while importing/restoring — the document is half-built.
- Commits (`CommitKindPlayback` / `CommitKindImport`) are scheduler/parity provenance, NOT the render's immutability guarantee: highlights are lossy (gated rows emit none, `applySequencerHighlight` drops late ones, off/gap cells are never highlighted), and before the played tier every such cell rendered from the live predictor, so editing the circuit rewrote it.
- Rows are index-keyed: `DrumView.DeleteRow` → `onDrumRowRemoved` shifts the timeline (`Service.RemoveRow`) and every per-row store synchronously. Undo/redo (a re-import) carries played history across IN MEMORY, keyed by row origin node (`undoRestore`) — never through the document bytes; a backward seek truncates it (`RewindTo`) and the replayed stretch re-records.
- **A backward seek abandons the pass after it**: `RewindTo` moves every Playback/Import commit at/after the target — hot tiers AND entries already aged into the scroll-back archive — to the `superseded` store (retained, never read by the live lookup). It used to land in the archive, which `valueLocked` falls back to, so after an edit + rewind the abandoned pass answered for unreplayed cells, parity compared against it and `CrossingCell` froze it into the drawn past. A seek also advances `parityGen` (`parityOnSeek`) so parity drops every observation of the superseded pass. Guards: `TestRewindToSupersededPassIsNotLiveHistory`, `TestBackwardSeekAfterEditDoesNotCompareAbandonedPass`, `TestSeekWhilePlayingLeavesNoStaleParity`; follow-up plan `docs/superpowers/plans/2026-10-04-parity-sources-of-truth.md`.
- **Playback history is ephemeral session state and never part of the project JSON**, in either direction: the format holds the document only (no played cells, commits, playhead, solo/mute), and an imported file starts with no history. Guard: `TestExportImportCarryNoPlaybackHistory` (export is byte-identical before/after a played, soloed session; import leaves the timeline empty).
- Guard: `drum_past_immutable_solo_delete_test.go` (solo × delete × row removal × undo, real `Game.Update` frames); scene `solo_then_delete_past_immutable`.

### Concurrency Model

- **UI thread**: `Update()` + `Draw()`
- **Background sequencer**: Audio scheduling via goroutine
- **Audio thread** (desktop): Sample generation via Oto
- **WebAudio thread** (browser): Sample playback
- **Guard**: `seqMu` protects structural edits (row add/delete, path changes)

### Async / Resource-Constrained Goroutines (`internal/async/`)

`async.Pool` + `async.Registry` are the **single canonical API** for any background work that isn't a real-time loop. Spawn raw `go func()` only for the audio/sequencer/BPM real-time loops where pool dispatch latency is unacceptable — every other case goes through here.

**Building blocks:**
- `async.Pool` — bounded workers + bounded queue. `Submit` is non-blocking (returns `ErrBackpressure` on saturation, never blocks the caller); `SubmitBlocking` respects `context.Context`. Workers recover panics. `Close` drains the queue.
- `async.Registry` — process-wide named-pool registry sharing a global worker budget (default `max(6, NumCPU-4)`, reserving threads for Ebiten/oto). `DefaultRegistry()` is the singleton. Use `Get(name, opts)` for first-call lazy creation; subsequent calls reuse. `Release(name)` frees the budget for transient/test pools.
- `async.Scheduler` — heap-backed deadline dispatcher. One timer goroutine no matter how many entries are pending; due jobs are non-blocking-submitted to the underlying pool. Use instead of `go func() { time.Sleep(d); fn() }` whenever pending count could grow with workload.
- `async.Go(name, fn)` — fire-and-forget convenience wrapper; lazy-creates a 1-worker / queue-8 pool in `DefaultRegistry`. Use for one-shot, low-frequency tasks (file dialogs, init hand-offs).

**Standing pools (production):** the registry's default budget floor (`minDefaultBudget` in `internal/async/registry.go`) **must cover this whole table** — see the ⚠ below.
| Name | Owner | Workers / Queue |
|---|---|---|
| `recording.lifecycle` | `internal/audio/recording_lifecycle_pool.go` | 1 / 8 — slow file-I/O finalize |
| `eventstream.persist` | `internal/eventstream/sink.go` | 1 / 8 — JSONL writer |
| `hooks.fanout` | `internal/hooks/bus.go` | 2 / 256 — pub/sub delivery |
| `eventlogger.format` | `internal/eventlogger/logger.go` | 1 / 8 — INFO narrative formatting |
| `audio.timers` | `internal/ui/game_new.go` | 1 / 64 — backs `Game.audioScheduler` for future-`when` playFn dispatch |
| `audio.prewarm` | `internal/ui/voice_prewarmer.go` | 4 / 64 — predictive voice render + gated import warm-up |
| `synth.preview` | `internal/ui/synth_graph_pipeline.go` | 1 / 8 — Synth-tab graph renders (`newSynthPreviewPool`) |
| `ui.dialog` | `internal/ui/select_json_async_desktop.go`, `drumview_ctor.go` | 1 / 8 (defaults) — file pickers (JSON import + WAV upload) |
| `userprefs.persist` | `internal/userprefs/store_notjs.go` | 1 / 8 — favorites file writer (atomic-rename, coalescing latest-write-wins) |

**Recipe** (mirror `recording_lifecycle_pool.go`'s `newLifecyclePool`): acquire a named pool from `DefaultRegistry().Get(...)` with a private-pool fallback if the registry budget is exhausted, hold the `*Pool` for the lifetime of the subsystem, `Submit` jobs that don't need to block.

⚠ **Adding a standing pool = 3 edits, not 1.** Add the row above, add it to `standingFootprint` in `internal/async/registry_footprint_test.go`, and bump `minDefaultBudget` to the new total. A floor below the footprint saves **nothing** — every caller with a fallback allocates the same workers in a private pool anyway, just outside the budget — while breaking the callers that have none. Precedent (2026-07): the synth overhaul added `audio.prewarm` (2), `synth.preview` (1) and `eventlogger.format` (1) without bumping the floor of 6. On js/wasm (`NumCPU()==1`) and on any host with ≤10 cores the default is the floor, so the budget ran out and `ui.dialog` could not be created — WAV upload silently failed — while `recording.lifecycle`'s `MustGet` **panicked the whole app** the moment the user tapped Record. Never use `MustGet` for a pool reached from a user action; it is startup-only.

**Test discipline:** every test Game must be closed (`t.Cleanup(g.CloseForTest)`): `Game.Close` is what releases the process-global registrations (`hooks` record-stop subscription, `i18n`/theme listeners, and the test-only `testGames` registry via `unregisterGameForTest`) — before 2026-09-06 the registry kept every closed Game alive until TestMain and the ui suite ran out of memory at ~12 GB. Every test that allocates a transient named pool must `t.Cleanup(func(){ async.DefaultRegistry().Release(name) })`. Packages with goroutine-spawning code use `goleak.VerifyTestMain(m, goleak.IgnoreCurrent())` in `main_test.go` (async, hooks, eventstream, audio, engine), pre-warming long-lived pools before the baseline so only test-introduced leaks trip the check.

**Predictor background:** `Predictor.StartBackground` is single-shot via an atomic CAS gate (`bgRunning`). Concurrent or repeat calls update `targetFn` but spawn at most one worker; `StopBackground` clears the gate so a fresh start works.

**WASM recording — off-thread (NOT a Go pool):** Browser recording uses an AudioWorkletProcessor (`src/js/recording_capture_worklet.js`) on the audio rendering thread + a Web Worker (`src/js/recording_encoder_worker.js`) on its own thread. Capture posts transferable Float32Array batches directly to the worker via a MessageChannel; the main thread is out of the data path. WAV encoding + zip bundling happen entirely in the worker. Hard caps: `BYTES_PER_CHANNEL_MAX=128 MB`, `RECORDING_DURATION_MAX_SEC=1800`, per-channel outbox depth=8 (drop on overflow, counter surfaced via `perfStats().recordingDrops`). The Go-WASM `audio.StopRecording` returns immediately with metadata + an "expected" zip filename; a background goroutine awaits the worker's Blob and triggers the download via `EventRecordStop`. End-to-end test: `src/js/recording_lifecycle.browser.test.js` (formerly `recording.browser.test.js`).

### Landing page (beatmo.io first visit)

Spec `docs/superpowers/specs/2026-10-23-landing-page-design.md` (revision 2, with the measured baseline); plan `docs/superpowers/plans/2026-10-23-landing-page.md`.

- **Boot order is game-first.** The first `<head>` script in `index.html` starts `fetch("main.wasm")` (the boot module instantiates from `window.__beatmoWasm`), then decides landing vs skip. Nothing the landing loads is fetched ahead of the wasm; its video loads only after `__beatmoBoot.wasmInstantiated` (the game is CPU-bound from there). Measured (4G, 4× CPU, cold): landing/baseline TTI 1.009, wasm fetch start 398 → 124 ms. **Every server for the page must answer concurrently**: nothing reads that early wasm response until audio.js's imports load, Chromium stops reading an unread body after a few MB, and a one-connection-at-a-time server then never serves those imports — `make serve` (then a single-threaded `socketserver.TCPServer`) hung on "Loading..." in Chrome with no console error, while Firefox/WebKit and every Node-served browser test booted. Guards: `TestDevServerServesWhileAnotherResponseIsUnread` (`internal/deploy`), `dev_server_boot.browser.test.js` (boots through the real `serve_nocache.py` in all three engines).
- **When it shows:** on a DEPLOYED page (the deploy stamps `main.<sha16>.wasm` into `index.html`) on EVERY visit, until the visitor ticks "Do not show tutorial demo again" (user decision 2026-10-24): `localStorage["beatmo.landing.optOut"] = "1"`, written by `landing.js`, read by the head script. There is no automatic "already seen it" skip. Dev, `serve-lan` and every browser test see the plain game unless `?landing=1`; `?landing=0` forces it off. `make serve-landing` (`landing_server.js`) serves a stamped page locally, so the demo behaves there exactly as on beatmo.io.
- **Files:** shell markup + CSS inline in `index.html`; `landing.js` (overlay, cues, readiness poll of `__beatmoBoot.firstFrame` + `warmupProgress().ready` — no Go export, Play → `startPlay()`); `landing_crystal.js` (pure JS crystal in a module Worker with OffscreenCanvas, main-thread fallback; pure-Node test `landing_crystal.browser.test.js`); `landing/<cut>.{mp4,webp,vtt}` (H.264 CRF 23, ~1.3 / 0.8 MB, uploaded uncompressed for Range requests; budgets in `asset_size_test.go`). Keys go to the landing via a capture-phase guard in the head script while `data-landing` is set.
- **The game canvas is `window.__beatmoGameCanvas()`, never the page's first `<canvas>`** (defined in the first head script). The crystal canvas is static markup, so it precedes the canvas Ebiten appends: every `querySelector('canvas')` in `index.html` bound the phone soft-keyboard focus rects and the native `<input>` touch listeners to the crystal (found 2026-10-28 via CDP `getEventListeners`: the game canvas carried only Ebiten's own, with or without the landing shown), and every in-page test helper sent its synthetic clicks there (`e2e_transport` Play never flipped). A landing that is not shown is removed right where it is parsed (as `handOver` removes it after Play), so on a page without it the game canvas is the only one. Guard: `TestPageScriptsNeverTakeTheFirstCanvas` (`internal/deploy`).
- **The recordings come from the real game:** `-tour desktop|phone [-record dir]` (`internal/ui/tour.go`, `tour_steps.go`, `tour_checks.go`, `!js`) drives the techno template through real input only (the `input.go` function vars, hover-before-press like a real pointer), pins English, records the master bus through the Rec path and pipes frames to ffmpeg at a constant 25 fps of the recording clock (a slot with no new draw holds the PREVIOUS frame — never a later frame early); the audio offset is measured from the WAV's first onset vs the transport start (sync within 1 ms, checked with `silencedetect` on the MP4's timestamps). Recording runs with vsync OFF: GNOME/XWayland throttles a window it considers hidden to ~1 frame/s (2026-10-24: 81–94% repeated frames with vsync on, 0.1–2.9% off). `showcase_video.sh` publishes into `src/js/landing/` only a clean take (no step errors, ≤ 5% repeated frames, no freeze > 12 frames); use a real display (xvfb's software GL repeats 24–52% on the Synth tab / theme switches).
- **Every tour step declares the state it exists to show** (`tourStep.reached`, `tour_checks.go`) and the runner checks invariants after EVERY step (the circuit's node/edge count never changes — a stray tap would add a node; the song keeps playing once started). A failure names the step, in `report.json` and in the tests. The tour's "tap outside" picks a grid point clear of the open menus, the corner buttons, every node and the splitter's grab strip — a tap on that strip creates a node (it did, in the first phone recording). Guards: `TestShowcaseTourReachesEveryStep` (both cuts through real `Game.Update`: every step's check, the invariants, the end state, the cue file the recorder writes, no drawn string says "tetris" — `textSpriteHook`), `TestEveryTourStepDeclaresItsState`, `TestTourReportsEveryKindOfFailure` (a missed state, a stray tap, a non-converging repeat and a failing action are each reported by name), `TestTourFrameSlotsAreConstantRate`, `TestFirstOnsetSeconds`, `TestLandingMediaMatchesTheTourAndThePage` (the committed VTT matches the CURRENT script; MP4 faststart, H.264+AAC, 25 fps, as long as its cues; poster = video size; aspect = what the CSS reserves), `TestLandingAssetsNeverSayTetris`.
- **The landing page itself** is tested end to end by `landing_behavior.browser.test.js` (real Chrome — Playwright's bundled Chromium has no H.264; served by `landing_server.js` like production): phases → Play, video only after the wasm download, cues → chips/captions, chapter seeking, keys never reaching the game while the landing is up (and reaching it after Play — the handover focuses the game canvas), the opt-out (Space/Enter on the focused box never untick it), every-visit, the phone layout at three sizes (nothing off-screen or overlapping, caption included), and the fallbacks (no H.264, Save-Data, no OffscreenCanvas, throwing storage, `?landing=0`, an unstamped dev page). The crystal's growth AND its worker message protocol: `landing_crystal.browser.test.js` (pure Node, fake canvas). Cold-load TTI: `landing_load.browser.test.js`. Playback on phones with no gesture leaking in (autoplay, first touch, "Tap for sound", autoplay refused, Save-Data, a touch plants, Play → the game plays with a running AudioContext): `landing_playback.browser.test.js` (fails 25 of 84 checks on the pre-fix page).
- **Sound and play come from the tap's `click` / the `keydown`, never `pointerdown`** (`wantSound`, landing.js). On a touch screen pointerdown fires BEFORE the tap counts as a user gesture (activation lands on pointerup / touchend); unmuting the autoplaying video there made the browser PAUSE it, and an "already unmuted" flag kept every later tap from resuming it — on phones the recording froze on the first touch and never made a sound (2026-10-24). Every tap / key now plays a paused recording (autoplay refused: iOS Low Power Mode, blocked autoplay — the chip reads "Tap to play"), Save-Data loads it on that tap, and index.html's canvas `touchstart` `preventDefault` skips `#lc-crystal` (it cancelled the tap's click: no plant, no sound). **Playwright's `page.evaluate` / `waitForFunction` run WITH a user gesture** (Chromium `userGesture: true`, WebKit `emulateUserGesture`), so one state read gives the page sticky activation and waives every autoplay rule — which is how every evaluate-driven test passed on a page that was broken on every phone. `landing_playback.browser.test.js` therefore reads state only from console messages an init script logs, and uses real touch only. Playwright's Linux WebKit is no stand-in for iOS video (refuses muted autoplay of a video with sound, crashed 2 of 4 runs, stalled gesture-started playback at 0.11 s) — the default "iphone" platform is an iPhone 13 screen + touch on Chrome with autoplay refused like Low Power Mode. BrowserStack Automate minutes were exhausted on 2026-10-24, so no real device has run it.
- **Never "Tetris" on the landing** (user decision): the template is found by its file stem only; the in-game title is out of scope.

### Runtime theme (Theme editor)

The UI palette is themeable at runtime — **rebuild-in-place**, zero per-frame cost:

- **Live palette = the generated `genColor*` vars.** `ApplyTheme(doc)` (`theme_runtime.go`) resolves a `theme.Doc` to a full token map (`theme_derive.go`), overwrites the vars by name (`genColorByName`), then re-runs the two derivations that otherwise happen once at init — **in this order**: `rebuildComponentSpecs()` (generated builder in `design_components.gen.go`; specs reference token idents, never literal hex) then `rebuildThemeAliases()` (`theme.go`; the former `var` block is now a function). Buttons resolve `Spec(id)` at draw, so all spec buttons re-theme with no per-site change.
- **Every package-level color derivation lives in `rebuildThemeAliases`.** A `var x = WithAlpha(genColor…)` anywhere else is evaluated once and goes stale after a theme change. Guard: `TestNoTopLevelDerivedColorsOutsideTheme` (`theme_rebuild_discipline_test.go`).
- **Cache chokepoint:** `Game.OnThemeChanged` (`game_theme.go`, registered via `OnThemeChange` in `New`) mirrors `OnLocaleChanged`: grid tile / node layer + sprites / edge cache, row sprites, label caches, panel zones, synth TraceSprites. Repaint proof: `TestThemeApplyRepaintsEverySurface`.
- **Every cache that BAKES a color keys on `ThemeGeneration()`** — in its hash/key/validity check, not only via `OnThemeChanged` (a zone `Invalidate()` re-lays out but does NOT drop a sprite whose hash is unchanged). Done in: transport toolbar hash, grid tile + composed grid cache, timeline ribbon, drum `bg()`, `TraceSprite` (checked inside `Draw`, so every adopter is covered), Chain trace cache, the package-level wheel sprites. Package-level sprite caches either key on the COLOR (pixel/corner/rounded caches) or are colorless masks tinted at draw (icons, text) — anything else must be theme-scoped. Never infer UI state from style identity (`btn.Style == PopupButtonStyle`): `ApplyTheme` rebuilds those styles, so the comparison silently fails after a theme change.
- **Theme guards run on EVERY scene** (`theme_drift_test.go`): `TestRuntimeThemeSwitchMatchesBootEveryScene` (runtime switch == boot with the theme, real cache path, a "loud" theme moving all nine categories, desktop + mobile) and `TestNoColorRendersOutsideTheTheme` (a color unchanged by BOTH a different theme and a shifted instrument palette is hard-coded unless it is in `themeFixedTokens`; reports `file:line` of the call site). They depend on the ebitestub rendering faithfully — `SubImage` is a pixel-sharing view and `ColorScale` tints are applied (`internal/ebitestub/ebiten`); before that every zone drawn through a clipped sub-image was discarded and pixel tests could not see buttons at all. A scene that shows the Synth tab must land its async graph renders synchronously (`settleSynthTab`) or two captures differ.
- **Beds vs controls:** area fills that merely SHARE a control surface's hex (the row-rack column, the mobile toolbar, the add-row strip) have their own Background-category tokens (`rack-bed`, `toolbar-bed` = surface-1's hex) so a background theme moves the whole drum-view bed together with the canvas, stripes and timeline; `surface-1/2/3` stay control surfaces. Never point a bed alias at `colSurfaceN` (guard `TestDrumBedsFollowBackgroundCategory`; attribute a suspect fill with a `drawRect` interceptor + `runtime.Callers`).
- **One control body:** every pressable control paints the spec keycap body (`Render`/`ButtonStyle.Draw`, surface-2 tone) — including the volume buttons (`drawVolumeButton` paints `button-secondary` under the glyph) and text inputs (`TextInputStyle.Draw` paints the `input-field` spec fill; the old recessed surface-1 slot was reverted 2026-09-05). Guard `TestVolumeButtonsAndInputsShareKeycapBody`. To check a control's body, sample the PNG (`px.go`-style decode) rather than trusting the eye — the beat counter, timeline +/- chips, sticky pills and row labels already measured identical.
- **Data canvases use the Visualizers family, never the chrome accent:** synth signature/focus graphs (`colConceptFill/Stroke`), the sampler waveform, wave/scope traces. In the default design `primary` and `viz-wave-a` are both gold, so an accent-colored graph is invisible until a theme separates Accent from Visualizers (the Ocean startup theme showed half the synth cards orange, half mint).
- **Styles and colors copied at construction resolve LIVE:** `ButtonStyle`/`TextInputStyle` carry a comparable `live` tag (spec id) re-read at draw; captured token colors (`b.IconColor = colTextSecondary`, profile fields) go through `liveCaptured`, which maps a value a chrome token has held to that token's current value; `Game.OnThemeChanged` drops the cached layout profile. Guard `TestCapturedStylesAndColorsFollowLaterTheme` / `TestProfileRebuiltOnThemeChange`. Verify a theme with a STARTUP config (`BEATMO_DEMO_CONFIG=<project with a theme block>`), not only mid-session picks — construction-time captures only show up that way.
- **Presets first:** `theme_presets.go` holds the six first-party themes (`vice-city` default = shipped palette + hot-pink-400 focus ring, `ocean`, `midnight`, `ember`, `neon-mint`, `orchid`); `theme.Doc.Preset` resolves BEFORE the per-category picks (`TestPresetResolutionOrder`); the editor lists presets as menu rows with a five-swatch strip and hides the nine chips under a collapsed "Adjust colors" expander (`TestThemeOverlayPresetRowsRealClicks`). A preset pick clears that scope's categories/tokens.
- **Model:** `theme.Doc{Categories: id→swatch NAME, Tokens: token→#hex}` (`internal/theme`, no deps). Nine categories (`theme_categories.go`, first token = the chip's principal color; guard `TestThemeCategoryTableComplete`); each member token keeps its HSL offset from the category's anchor, dark/text categories keep absolute lightness, achromatic tokens stay, semantic reds/greens are fixed. The default is the identity (`TestThemeDefaultIsIdentity`) — that is what keeps the DESIGN.md drift/discipline tests meaningful.
- **Precedence (per category and per token):** override && user → user; else circuit; else user; else default (`effectiveTheme`, `theme_state.go`; `TestThemePrecedenceTable`). `recomputeEffectiveTheme` applies only when the resolved doc changed.
- **Persistence:** user theme + `override_circuits` in prefs (`userprefs.ThemeStore`, section `theme`), seeded before frame 1 by `ApplyStoredTheme` (`cmd/beatmo.go`, both platforms) via `ThemeSaveSink` (`theme_sink.go`). Circuit theme = optional `theme` block in `beatmo.json` (`projectFile.Theme`, replace-not-merge on import, never in built-in templates — `TestTemplatesCarryNoTheme`); circuit picks are undoable (`EventCircuitThemeChanged`, `ScopeDocument`; the snapshot journal carries the block).
- **Events:** `EventThemeChanged` (session, tag `theme`) and `EventCircuitThemeChanged` (document, tag `theme`), payload `hooks.ThemePayload`.
- **UI:** fourth top-right grid button (`IconPalette`, `gridThemeInputHandler`, part of the reserved cluster) → `ThemeOverlay` (`theme_overlay.go`; scope pills, override toggle, nine rows with chip + reset, two-tap Reset all). Unlike Settings/Logs it is **window-bounded** and re-painted above the pane dividers (`drawThemeOverlaysAboveDivider`). A chip opens the shared swatch grid (`theme_picker.go`) as a must-answer entry STACKED on the (non-modal) panel — only the modal filter makes a tap on the panel beneath dismiss the picker instead of acting on the rows; a pick closes the picker, an outside tap cancels it. Scenes: `theme_editor`, `theme_editor_picker`, `theme_applied_orchid`, `theme_applied_background_only`, `crop_theme_editor`. Tests: `theme_*_test.go` (real input through `Game.Update`).

### Menus — one frame, one behavior (MenuCard)

Every floating menu draws its frame through `MenuCard` (`menu_card.go`) — a value type built ONLY from existing primitives (`drawPanel`/bottom sheet, `drawMenuHeaderBandAccent`, `drawMenuTitle`, `newPopupCloseButton`, `buttonHitAdapter`) — and opens through `menuCardEntry` (non-modal + scrim; `mustAnswer` only for a picker stacked on an open menu). Owners keep their own type, body and input handling (component-tree isolation: each overlay publishes its own HitAreas, GridTree layers stay GridTree layers). Zone-style overlays use `card.ControlAreas` + `card.CatchAll` (buttons first, catch-all LAST); comp-style overlays route presses through `card.HandleInput`. Menus that already track their own close × hand it over with `SetCloseButton`; docked panels use `DrawBand`. Guards: `TestEveryPortalMenuUsesMenuCard` (AST scan of every `PortalEntry`/`menuCardEntry` site; exemptions carry a reason) and `TestEveryMenuClosesTheSameWay` (real input: × / Esc / outside tap per menu — add a row when adding a menu). The portal lays an overlay out ONLY on open and republishes HitAreas every frame, so an overlay whose row set changes (the Theme editor's expander) replays its own Layout.

**Tap outside ANY menu / pop-up = close, and nothing else** (`popup_outside_press.go`). While a portal menu, the node sidebar or the group menu is open, `popupOutsidePressHandler` (grid `InputDispatcher`, z=250: above sidebar/group menu, below the top-right cluster) OWNS every grid press outside it: grid-local pop-ups close on that press, portal menus close via the tree's click-outside, and the dispatcher's press-owner latch (`GestureOwnedByHandler`) keeps the release-time `GestureTap` / long-press off the canvas — so no node is created or selected. Mobile has no close × by design; this is its close. Pass-throughs (pre-existing, never create nodes): Shift+press (connect/marquee), group-badge taps (switch menu), the splitter strip. **A capture that declines a NEW press edge is released and the press dispatched normally** (`InputDispatcher.Dispatch`): DrumView grabs capture on a mere HOVER over the drum pane while any dropdown is open, and that stale capture used to swallow every grid press — the outside tap got no owner and created a node (only visible in the real binary, where the X cursor rests on the drum pane). Guards: `menu_outside_tap_grid_mobile_test.go` (every popup × mobile touch / desktop mouse / desktop-layout touch / pointer-rested-on-drum-pane); scene `menu_outside_tap_grid` (real touch: Theme button → outside tap). Add new grid-local pop-ups to `gridLocalPopupOpen` + `gridPopupCases`.

### Value entry — one wheel for numbers, one TextField for text

Numbers are never typed. Every numeric control is the shared **`ValueWheel`** (`mobile_wheel_popup.go` / `mobile_wheel_render.go`) over a **`WheelModel`** (`wheel_model.go`): `knobModel` adapts synth/sampler/EQ knobs (byte-identical to the old wheel — `TestWheelCharacterizationTape`), `NumberModel` serves plain numbers (grid = multiples of the step, optional `Wrap` for clock fields). Hosts: `MobileWheelPopup` (popup + transactional Accept/Cancel — Sampler, EQ, BPM via `dv.openBPMWheel`, node values via `g.openNodeValueWheel`) or inline (`NavigationPanel`'s go-to wheel, the logs HH:MM picker). Live writes per detent, ONE commit on close (`commitBPM`, `commitNodeParams`). The go-to wheel PREVIEWS (timeline ghost, `TimelineZone.SetGotoGhost`) and seeks only on Go/Enter. A centre click confirms on the popup wheels and the go-to wheel, which also own Up/Down (value) and Left/Right (step) while open; the logs HH:MM pair is pointer-only. Esc on a node wheel restores the opening params verbatim (`WheelBinding.OnCancel`) — never re-derived through the model. `ParamValueEditor` and every typed numeric path were deleted (2026-10-03, `docs/superpowers/specs/2026-10-03-value-wheel-and-text-field-design.md`).

Text is typed through **`NewTextField(dv.textFields, TextFieldSpec{ID, Owner, ...}, maxLen)`** (`textfield.go`). A field is LIVE while its owner calls `Update()` (or its native input is open) and `syncNativeGestures` then arms its desktop soft-keyboard proxy / mobile native `<input>` automatically — never list a text box in `native_gesture.go` by hand (that is how Save As and the FX search shipped with no phone keyboard). Guards: `TestNoNewTextInputOutsideTextField`, `TestTextFieldIDsAreUnique`, `TestSaveAsAndFXSearchHaveNativeCandidates`. Browser coverage of the native path uses the instrument search (`mobile_native_input` / `mobile_text_input`).

### Parity Verification

Built-in invariant checks ensure UI slate matches predictor state:
- `parityCheck` — Steps vs predictor mismatch
- `parityScan` — Highlight events vs scheduler decisions
- `parityAudio` — Past audio events vs expected triggers
- Grace periods prevent false positives during tight loops

### Browser audio isolation — UI must not reach the audio (measure with the playback probe)

On WASM, Go runs on the page's single main thread: Ebiten's Update/Draw, the sequencer goroutines, the Go GC, every `syscall/js` call and the browser's own WebGL raster all share it. WebAudio renders on its own thread, so a note is safe once `BufferSource.start(when)` has been called with `when` in the future — **everything before that call is exposed to the UI**, and so is any audio-signal work that runs on the main thread.

**Measure what is HEARD, not what was asked for — the playback probe.** `window.startAudioProbe()` / `stopAudioProbe()` (audio.js, opt-in, zero cost while off) tap every channel bus and the final output with the `beatmo-audio-probe` AudioWorklet (`audio_probe_worklet.js`, audio thread: onsets, per-note level, clicks, hard cuts, clipping, a coarse envelope, wall-vs-audio clock drift). Each sequencer note carries `grid` (predictor trigger time, no lookahead) and `ideal` (scheduler request before any clamp) from Go (`audio.BatchParam.Grid/Ideal`, threaded through `soundReq.grid` → `playSoundsBatchFlat`'s trailing args). `audio_probe_analysis.js` (pure; Node benches and the page run the same code) matches onsets to notes and reports lag vs grid / vs request, late counts, IOI (rhythm) error, lookahead shifts, JS-queue wait, notes that *arrived* in audio.js already late, silent (`missed`) vs `masked` vs `dropped` notes, clicks/cuts, and audio-thread underruns (a permanent rise of wall−audio drift). On real hardware: `await startAudioProbe()`, play, `await audioProbeReport()` in the console. Bench glue: `audio_probe_bench_helpers.js` (`startPlaybackWhenReady`, `startPlaybackProbe`, `stopPlaybackProbe`, `probeSummaryFields`, `probeGateFailures`). Analysis guard: `audio_probe_analysis.browser.test.js` (pure Node, synthetic logs with known answers).

**Three couplings found with it and removed (2026-09-30):**
- **start() waited for the frame to end.** `enqueueAudioEvents` flushed on a microtask, which cannot run until Go returns to the event loop — the end of the whole Ebiten frame. Notes arrived with +73..+122 ms of lead and still started 74–134 ms late. The Go batch bridges now flush **synchronously** (`enqueueAudioEvents(events, true)`, no budget — a batch is ≤32 due-now events). Contract gate: probe `jsQueueMs` p99 ≤ one 60 Hz frame (`PROBE_JSQUEUE_MAX_MS` = 16; render-cache hits only — cold misses are reported separately as `renderWaitMs`). A/B knob: `window.__beatmoAudioFlushMode = 'microtask'` restores the legacy flush (isolation bench `stall-150-microtask`: jsQueue p99 115.7 ms vs 0.5 ms synchronous). Side effect to expect: `perfStats().audioCallAvg` now includes starting the voices (A/B +~0.15 ms/event, update time unchanged) — `stress_complex`/`perf_e2e` call-time gates were re-calibrated for it; don't read a higher call time as a regression of the bridge.
- **Insert effects ran on the main thread.** bitcrusher/gate/pitchshift/transient/phaser/autowah were ScriptProcessorNodes; a stalled frame starved them. Under a 150 ms/500 ms main-thread stall they left **27/122 notes silent** on their channels; on an AudioWorklet **0**. Since 2026-10-27 EVERY insert effect (and the EQ and the master soft clip) is desktop's C running in the `beatmo-dsp` worklet (§ One audio pipeline below); the main-thread ScriptProcessor host is only the fallback where AudioWorklet does not exist (an insecure origin). `window.insertEffectHosts(id)` reports the live host; `window.__beatmoDSPHost = 'scriptprocessor'` (set before the AudioContext exists) forces the fallback.
- **The audio cushion followed UI timings.** `runtimeAudioLookahead` adds up to +30 ms when draw/update run slow, and every change shifts all later notes — a UI hovering around a threshold made the rhythm wobble. It now **ratchets** per transport generation (`g.audioGen`: never shrinks mid-play, resets on Stop/Pause/seek). Guard: `TestRuntimeAudioLookaheadNeverShrinksMidPlay`.

**Lateness metrics are honest now — and were vacuous before.** audio.js observed schedule lead AFTER its 4 ms floor clamp and Go's Stage C after its 12 ms dispatch floor, so a late note read as lead=+4/+12 ms: `overdue`, `smallLeadCount`, `minLead` could never fail on WASM and every bench gate built on them passed by construction. Both now observe the **requested** time (`requestedWhen` in `processAudioEvent`, `requested` in `audioLoop`). In headless Chromium the honest numbers are dominated by software-GL raster on the renderer main thread (400–600 ms frames, CPU profile ~88% `(program)`) starving the Go sequencer, so the lateness gates in `webaudio_timing_stress` / `bench_startup` / `perf_e2e` / `stress_complex` **report by default and fail with `AUDIO_PROBE_STRICT=1`**; the structural probe gates (no silent / dropped notes, `jsQueue` ≤ one frame) are hard. Don't "fix" a lateness failure by clamping before observing again.

**The audio lane — the sequencer no longer gates the audio (2026-10-02).** The Go sequencer used to commit a beat only when the wall clock reached it and play it `lookahead` (60–150 ms) later, so any main-thread stall longer than that made notes *arrive* late. Now two lanes run off the same predictor: the **UI lane** (`seqScheduleTime`) still advances highlights / playhead / parity at grid time, and the **audio lane** (`audio_lane.go`, ticked at the end of every sequencer pass under `seqMu`) keeps every note of the next `AudioHorizonSec` (browser 1.0 s, desktop 0.25 s) **committed to the engine** — `BufferSource.start(when)` / mixer voice already scheduled — so a UI stall shorter than H is inaudible. Real-time edits stay correct by **reconciling, not invalidating**: a pure `laneNoteAt(row, abs)` (`audio_lane_note.go`) says what a cell sounds like under current state; per-row and global fingerprints (`audio_lane_source.go`: path snapshot, predictor `Epoch`, BPM/div, clock anchor, solo, row instrument/volume/mute + `audio.VoiceRevision`) pick the rows to re-derive; the diff against the committed set cancels / replaces / adds voices by **voice id** (`vid > 0`; previews stay `vid 0` and are never touched). Edit semantics: an edit reaches the next note that has **not started** (`CancelGuardSec` — a note that close to its start counts as started). Mute stops are sent just in time (`laneStopLeadSec` 0.15 s) because a sent `StopAt` can't be retracted. Late policy: a note discovered late plays if within `LateToleranceSec`, else it is dropped and counted.
- **Engines.** audio.js: `cancelLaneVoices` / `stopLaneAt` / `cancelAllLaneFuture` + tombstones for voices still queued or awaiting a render; desktop mixer: `CancelVoices` / `StopLaneAt` / `CancelAllLaneFuture` (`lane_mixer.go`, removes only voices whose start is still ahead). A cancel before start is sample-identical to never scheduling it.
- **A lane voice never waits on the main thread.** On a STALE render record (a knob was just edited) it schedules the stale buffer at once and `laneSwapFresh` re-starts it on the fresh render, same vid + start, if that lands first — the old stale-while-revalidate race replayed from a main-thread timer, and an edit storm bunched 4 notes 160–415 ms late into one burst with clicks. Bridge edits (outside `Update`) **kick** the lane (`jsFn` → `afterJSExport` → `kickLane` → `sequencerLoop` runs `laneKickPass`) so the reconcile happens before control returns to JS — otherwise a stall right after the edit left notes inside it stale.
- **A note has its own gain node, nothing shared.** The per-volume bus pool (`getBus`, 1/16-quantised volumes, LRU eviction) is gone (2026-10-27): it rounded every note's volume by up to ±1.9 dB, silenced anything under 1/32, and its eviction could cut a ringing voice. Each note's GainNode = headroom × its exact volume; a stop ramps that gain.
- **A seek moves abs↔time without a new transport generation**: every reconcile also cancels retractable commitments whose abs lies outside the re-walked range (otherwise the old timeline played on for a whole horizon), and an already-played entry whose cell now maps to a FUTURE time is history, forgotten so the cell re-commits (a backward seek — every undo/redo while playing — otherwise replayed silence). Guards: `TestLaneSeekCancelsTheOldTimeline`, `TestLaneBackwardSeekReplaysCells`.
- **Replace is atomic.** The lane replaces a changed note with `audioSink.Replace(old, new, note)`; `flushLaneOps` cancels a run of old voices in one `audio.CancelVoices` call, which RETURNS the vids the engine refused because they had started (its clock can run ahead of the lane's: the desktop mixer renders a Read chunk ahead), and drops those replacements — never the old and the new voice together. Guards: `TestLaneReplaceRefusedByEngineDoesNotDouble`, `TestFlushLaneOpsAppliesOpsInOrderAndReplacesAtomically`.
- **A knob drag re-commits the NEAR window only** (`LaneVoiceNearSec`, browser 0.25 s; desktop's whole horizon is 0.25 s). A voice-only change (`VoiceRevOnly`, rate-limited to `LaneVoiceRecommitMinSec`) re-derives `[guard, now+near]` at once; the rest of the horizon, committed under the old voice, is re-derived as it comes within the window (`voiceStale`/`voiceDone`, `advanceVoiceStale`), so every note is re-derived before it starts — on a stall longer than the window a far note may start with the old timbre, on time. Any other change (volume, mute, path) and every `GlobalKey` move (tempo, seek) still reconcile the whole window. Before (2026-10-28): every drag tick replaced the whole second of notes — ~3000 WebAudio voice starts/s in `webaudio_all_knob_bpm_stress`, near-due replacements queued 3–24 ms behind far ones and started up to 16 ms late. **audio.js processes its queue soonest-start first** (`orderQueueByStart`, stable; an ordered queue costs one pass) for the same reason. Guards: `TestLaneVoiceDragRecommitsNearWindowFirst`, `TestLaneVoiceEditReachesNextNoteAtOnce`.
- **No audio-clock anchor at Play** (the clock still read 0) is repaired by the lane once the clock runs (`TestLaneAnchorsWhenPlayStartedBeforeTheAudioClock`) — the lane maps every note through that anchor.
- **Results** (`make bench-lane-ab`, `bench-results/audio_lane_ab_after_2026-10-02.md`, same machine, interleaved, load ≤ 15): lane `arrivedLate` 0 and late>20 ms 0 in idle / stall-150 / stall-300 / tab-synth / pan-storm / edit-mute / edit-vol, vs legacy 25–69 / 17–49; IOI error p95 5–9.5 ms vs 54–287 ms; 0 stale edit notes; Update/Draw unchanged. Findings: `bench-results/audio_lane_findings_2026-10-02.md`.
- **Tests:** `audio_lane_test.go` (reconciler vs a scripted grid), `audio_lane_source_test.go` (`TestLaneKeysCoverEveryEditClass`: no edit class changes a note without moving a key), `audio_lane_game_test.go` (real `Game.Update`: stall, stop, BPM, mute, seek, kick), `audio_lane_oracle_test.go` (`TestLaneHeardEqualsLegacyDispatched`: the lane plays exactly the notes legacy dispatched, cell by cell, under seeded edits), `audio_lane_cancel.browser.test.js` (engine cancel/stop/tombstone/stale-swap/bus-eviction, probe-verified), `audio_lane_e2e.browser.test.js` (the real app with the lane: stall-300, edit storm, stop — hard gates; `E2E_LANE=0` runs the same phases on legacy for comparison). `BEATMO_AUDIO_LANE=0|1` and `BEATMO_AUDIO_HORIZON_SEC` override the profile (`window.__beatmoEnv` in benches). The lane is OFF under `go test` unless a test enables it (`enableAudioLaneForTest`) — legacy tests assert synchronous `playFn` dispatch.

**Gotchas.** (1) `startPlay()` right after loading/building a circuit is **ignored by the warm-up gate** — `webaudio_three_stage_latency` measured only `null` for that reason; use `startPlaybackWhenReady`. (2) Timing benches are meaningless on an oversubscribed machine (a load-38 run showed 22 audio-thread underruns that were CPU starvation, not a regression) — check `uptime` first. (3) Other sessions rebuild `src/js/main.wasm`; for controlled runs build your own and serve it (`WASM_FILE=<path>` on the isolation bench). (4) A zero-output AudioWorkletNode is processed without a destination connection (Chromium automatic pull); `performance` is absent in AudioWorkletGlobalScope — use `Date.now()`. (5) Chromium never calls `process()` again after it returns `false`, and a silent input looks exactly like a disconnected one — worklet effects stay alive and are disposed by message (`fx.dispose`). (6) **A per-frame WASM timing is bimodal: V8's tier, not the code.** The 30 MB module starts in Liftoff and tiers hot functions up in the background; whether a 1 s window sees optimized code is V8's call (`webaudio_perf` `updateAvgMS`: 0.45–0.64 ms with `--js-flags=--no-liftoff`, 0.68–0.96 ms under default flags on the same build, and a 10 s warm-up did not converge — 2026-10-28). A/B a per-frame change interleaved and with `--no-liftoff` (or many runs); a median of a few default runs cannot see a 15% change. Per-frame layout work skipped when unchanged: `recalcButtons`' forced audio-panel / timeline layouts are gated (`drumview_layout_gate.go`: the timeline on its exact inputs; the panel on `eqLayoutKey`, panel input last frame, a bridge call (`jsExportSeq`), and at least every `eqPanelRelayoutEvery` frames; Synth/Sampler/Chain still every frame). Guard `TestIdleFramesSkipForcedPanelLayouts`.

**Live edits retune the audio graph, never rebuild it (2026-10-25).** Reported: startup demo playing, kick FX → add a Pitch Shifter → touch its slider → "the audio lagged a lot and then stopped". Two defects stacked: (a) a held control wrote the audio layer on EVERY input tick, moved or not — FX slider Phase 0 (`handleFXPanelInput`) and the EQ curve handle's `OnDrag` (both bands and HP/LP); (b) every insert-FX param write pushed the channel's whole slot list to audio.js, whose `updateInsertEffects` tears down and rebuilds every effect (a fresh AudioWorkletNode with an EMPTY pitch-shift delay line) and rewires the channel; `setChannelEQ` likewise built ten fresh biquads + a rewire per edit. Measured in real Chrome: a slider held still = 280 chain rebuilds / 560 AudioWorkletNodes in 4 s and 10 silent kick notes; at 4× CPU throttle the main thread went from ~320 to ~700 ms/s of long tasks. Now: a held control writes only on a value change; `audio.SetInsertEffectParam` → `platformInsertEffectParamChanged(id, uid, name, value)` → JS `retuneInsertEffect` retunes that ONE live C effect in the `beatmo-dsp` worklet (`ifx_set`, state kept — the desktop `SetParam` contract); a chain edit is one message that reconciles C instances by uid (survivors keep their state, as desktop keeps its Go processors); an EQ edit of the same shape swaps coefficients and keeps the filter state (`eq_update`, both platforms). The graph is rewired only when a channel gains its first fx / eq node. Counters: `audio.InsertFXPushCounts()` / `audio.ChannelEQWriteCount()` (Go, every platform) and `window.audioGraphStats()` (JS: chain pushes, param updates, DSP nodes built, EQ updates, channel rewires). Guards: `TestInsertEffectParamEditNeverRebuildsTheChain` (audio), `TestHeldFXSliderWritesAudioOnlyOnValueChange` + `TestHeldEQHandleWritesAudioOnlyOnValueChange` (ui, real `Game.Update`), `TestSetKeepsState` + `TestEQEditKeepsFilterState` (C, native), `webaudio_live_edit_budget.browser.test.js`. A pitch shifter is a delay line (latency up to its window): audio.js records each note's insert-chain latency (`starts[].fxLatency`, `insertChainLatencySec`) and the probe matches the onset that much later — heard-late, never "silent" (`audio_probe_analysis.browser.test.js` case 5b).

Bench: `make bench-lane-ab` (`scripts/audio_lane_ab.mjs`: baseline / legacy / lane arms interleaved over the isolation scenarios + `stall-300` / `edit-mute` / `edit-vol`, runs at load > 15 discarded; `AB_SCRATCH=<dir with baseline_site/ and lane.wasm>`, `AB_DUMP` for raw probe logs). `make bench-isolation` (`webaudio_audio_isolation_bench.browser.test.js` → `bench-results/audio_isolation.json`; scenarios idle / stall-50 / stall-150 / tab-synth / tab-chain / zoom-storm / pan-storm / fx-worklet). `webaudio_bench_startup` writes `probe*` fields into `browser.json` and `bench-compare.js` renders a "Browser Playback Probe" table in REPORT.md.

### Event Notification (Two-Tier)

Two distinct mechanisms for "X changed, notify Y":

1. **Async `hooks.Bus`** (`internal/hooks/`) — fire-and-forget pub/sub for *external* observers (telemetry, eventstream, future cross-package consumers). 4-worker pool, panics swallowed, never blocks publisher. Subscribers run **off the UI goroutine** with no ordering guarantees. Helpers live in `internal/ui/event_helpers.go` (`emitNodeAdded`, `emitRowInstrumentChange`, etc.). Use this for anything an external package or test sink might want to observe.

2. **Sync direct calls** for intra-component handoffs that must be visible before the next `Draw()`. Pattern: a private method like `dv.onRowInstrumentChanged(row, oldID, newID)` invoked at every mutation site. Example: `SetInstrument` and the rename closures in `js_exports_graph_ui.go` both call `onRowInstrumentChanged` so the EQ panel's `eqActiveChannel` follows the row when the row's instrument id changes (see `drumview_audio_eq.go`).

**Rules:**
- `hooks.Bus` is **not** a substitute for sync delivery — handlers run on a worker pool, not the publisher's goroutine. Never use it for UI state that must be coherent before the next frame.
- Intra-struct notifications (publisher and consumer in the same struct) use direct method calls, not subscriber slices. A `[]func` subscriber API is justified only across real component boundaries with multiple consumers.
- Sync handlers are reachable from `Game.Update` under `seqMu`; they must not call back into Game paths that re-acquire `seqMu` (deadlock).
- When a state field is mutated at multiple sites (e.g., `Rows[i].Instrument` is changed by `SetInstrument` *and* by the rename flow), every site must call the sync notifier — otherwise some paths leave dependents stale.

### Event log tags — single source of truth

The INFO bracket tag for every `hooks.Kind` lives in `ActionMeta.Tag` inside `hooks.ActionRegistry` (`internal/hooks/actions.go`) — the same table that holds the human label and scope. `internal/eventlogger` reads `hooks.MetaFor(e.Kind).Tag`; formatters return the message string only (no embedded `[tag]`). All tags are lowercase across **all** log levels and packages.

Guards:
- `TestEveryActionHasLowercaseTag` (`internal/hooks`) — every registry entry has a non-empty, all-lowercase `Tag`.
- `TestEveryKindClassified` (`internal/hooks`) — every `hooks.Kind` const has a registry entry.
- `TestNoUppercaseInfoTags` (`internal/log/tag_case_test.go`) — no UPPERCASE bracket tag appears anywhere in the codebase.
- `TestHooks_NumNonVerboseUpdated` (`internal/hooks`) — `hooks.NumNonVerbose` (currently **76**) is validated as self-deriving from the registry; the test fails when a new Kind is added without updating the non-verbose count.

**Adding a new `hooks.Kind`** requires: a `KindAll` entry, an `ActionRegistry` entry with a lowercase `Tag`, a formatter in `internal/eventlogger`, and (if the Kind is non-verbose) verification that `NumNonVerbose` still matches.

**Full tag taxonomy (all lowercase):**

| Tag | Covers |
|---|---|
| `play` | play start/stop, pause, resume |
| `seek` | transport seek |
| `bpm` | BPM change |
| `transport` | subdiv, length |
| `record` | recording start/stop/dropped |
| `import` / `export` | project I/O |
| `scene` / `uistate` | scene apply; UI-state apply |
| `node` / `edge` | graph edits |
| `row` | row add/delete/instrument/mute/solo/volume/color |
| `master` | master volume |
| `eq` | EQ band + HPF/LPF filter |
| `fx` | insert-effect add/remove/param/move/toggle |
| `synth` | synth committed/reset, live synth param, instrument rename |
| `sampler` | sample saved/created/reset/edit, WAV load |
| `recipe` / `kit` | recipe lifecycle; kit apply |
| `favorite` | favorite toggle |
| `panel` | audio-panel state + tab switch (was `uistate`) |
| `camera` | pan / zoom — **one line per finished gesture** (promoted out of verbose) |
| `drag` | per-frame drag telemetry — stays verbose (`EventDragProgress`) |
| `lang` | UI language change |
| `theme` | theme editor: profile pick/reset/override (`theme.changed`) and circuit pick/reset (`theme.circuit.changed`, undoable) |
| `warmup` | playback readiness gate: circuit warm-up started / complete |
| `undo` / `redo` | undo / redo |
| `ui` | generic button/pill/toggle tap (Layer B) |
| `popup` | menu/popup/dialog open & close (Layer B) |
| `scroll` | scroll gesture end, coalesced (Layer B) |
| `search` | search query, coalesced (Layer B) |
| `input` | generic text-input commit (Layer B) |
| `view` | mobile view-mode switch (Layer B) |

### Layer B — Interaction telemetry

A parallel set of `hooks.Kind`s (scope `ScopeInteraction`, never undoable) describing **what the user touched**, flowing through the same `hooks.Bus → eventlogger` pipeline and appearing inline in the narrative and JSONL stream.

| Kind | Tag | Emit site |
|---|---|---|
| `EventUITap` | `ui` | `Button.applyPress` on press edge (`held==1`) — covers every button/pill/toggle/keycap; NOT coalesced |
| `EventPopupOpened` | `popup` | `portal.go Open` |
| `EventPopupClosed` | `popup` | `portal.go Close` / `CloseTop` / `CleanupClosed` (includes Esc + click-outside) |
| `EventScroll` | `scroll` | scroll-gesture end / wheel end — **coalesced** (keyed by Kind; last surface wins within the window) |
| `EventSearchChanged` | `search` | inst-menu search rebuild — **coalesced** |
| `EventTextCommitted` | `input` | generic text-input commit not already covered by a domain event (e.g. BPM, rename) |
| `EventViewModeChanged` | `view` | `SegmentedControl` view-switch callback |

Layer A also received three new non-verbose kinds: `EventLanguageChanged` (`lang`), `EventUndo` (`undo`), `EventRedo` (`redo`).

**Anti-pollution rules:**
- **One line per discrete action / gesture, never per-frame.** High-rate emit sites fire only at gesture boundary (release / commit), or route through the `eventlogger` coalescer (debounce-trailing, keyed by `Kind`).
- **`Button.applyPress` logs only on the press edge** (`held==1`), not on repeat-tick or release — a held auto-repeat button produces one tap line.
- **Per-surface kinds, not a single generic kind.** The coalescer collapses by `Kind`; a single generic interaction kind would wrongly collapse unrelated interactions. Only `EventScroll` and `EventSearchChanged` are added to `coalesceKinds`; discrete events (`EventUITap`, `EventPopupOpened`, `EventPopupClosed`) are not (coalescing by Kind would silently drop distinct taps).
- **Accepted duplication:** an action button emits `[ui] tap "<label>"` AND its domain effect line (e.g., `[node] add node`). This is intentional (intent vs. effect) and is bounded to two lines per gesture.

### Screenshot Subjects (subject-cropped captures)

The screenshot harness can crop a captured PNG to a single UI surface ("subject") instead of the full screen. One Subject→Rect registry in `internal/ui/screenshot_subjects.go` is the single source of truth: the desktop binary uses it to PNG-sub-image-encode; the Playwright browser runner reads the same rect via the `subjectRectJS` export and passes it to `page.screenshot({clip})`. Both platforms therefore produce pixel-identical crops.

- **Adding a subject**: declare a const in `screenshot_subjects.go`, return its bounds from `(*Game).SubjectRect`, and expose a public bounds accessor on the owning component (never reach into private fields from `screenshot_subjects.go`).
- **Adding a cropped scene**: name it with the `crop_` prefix, set the `Subject` field on the catalog entry. `scene_crop_test.go` will exercise it automatically — every `crop_*` scene's setup must produce a visible Subject rect that fits inside the framebuffer (and is strictly smaller than the full screen).
- **Triggering Scope state** from a crop scene's `Setup`: use the `(*ScopePanelZone).SetFrozen / SetAutoGain / SetTraceVisible / SetWindowMs / SetYGain` setters. They share code with the freeze/AG/swatch/wheel handlers, so scenes can drive the same visual state as a real user click without synthesizing input events. Reach the zone via the `scopeZoneOf(g)` helper in `scene_catalog.go`.
- **Listing**: `./beatmo -list-scene-subjects` prints `name<TAB>subject` for every catalog entry; `screenshots-all`'s `manifest.json` records each PNG's `subject` field.

---

## UI Layout — DrumView Zones & Audio Panel

The whole UI lives under one `DrumView`. Every pixel inside `dv.Bounds` originates in a Zone or Layer registered with `DrumViewTree` (`drumview_tree.go`) — `(*DrumView).Draw` is dispatch-only ([[feedback_drumview_layer_pipeline]]). The biggest single Zone is the **EQ panel zone** at the bottom of the screen, which hosts the six audio-analysis tabs.

### Grid pane — GridTree (top pane, sibling to DrumViewTree)

The top **grid/graph pane** renders through its own **`GridTree`** (`grid_tree.go`) — a slim sibling to `DrumViewTree`. It is a **draw-only z-ordered Layer stack**: it owns z-ordered compositing for the grid pane and nothing else (no input dispatch, no `HitIndex`/`Zone` arbitration, no `OverlayPortal`, no keyboard focus). `(*Game).drawGridPane` is **dispatch-only** (guarded by `grid_render_pipeline_discipline_test.go`): it computes per-frame setup into `g.gridDrawCtx` + `g.gridDrawScreen`, then calls `gridTree.SetBounds/Draw`. Every former draw block lives in a `(*Game).drawGrid*` method (`grid_pane_draw.go`) wrapped in a thin `gridLayer` adapter registered in `registerGridTree` (`grid_pane_layers.go`), in ascending grid-local z (`GZ*` constants, 0–70). The z-order table is pinned by `grid_layer_zorder_test.go`. The single Draw dispatch site is pinned by `grid_draw_dispatch_discipline_test.go` (`TestGridTreeDrawHasSingleDispatchSite`) — because the `drawGrid*` layers read the shared `g.gridDrawCtx`/`g.gridDrawScreen` that only `drawGridPane` populates, a second `gridTree.Draw` caller would render against a stale context.

- **Why it exists**: the coordinate `(i,j)` badge used to render *on top of* the node sidebar because `drawGridPane` was a flat 1100-line sequence where z-order = statement order. Now the badge is a z=40 Layer and the sidebar a z=60 Layer, so ascending-z dispatch composites the sidebar over the badge. Regression guard: `grid_coord_badge_zorder_test.go` (structural z + runtime draw-order interception).
- **Clip policy**: `gridLayer` carries a `clip bool`. `GridTree.Draw` clips `ClipToBounds()==true` layers to `t.bounds` via a cached bounds-subimage (== the original `top = screen.SubImage(gridRect)`); content layers are clip=true, while the sidebar + cursor-label are clip=false (they drew to the unclipped `screen` originally — keep so the ebitenstub backend's `screen.At()` pixel reads still see them).
- **Input is NOT tree-routed (by design)**: `Game.Update` drives grid input through the existing path (`inputDispatcher` + `handleTapInGrid` + `cam.HandleMouse`), which already has working z-prioritization and no current click-through bug. `GridTree` deliberately carries **no** input machinery — migrating node hit-testing into a tree would require a gesture-model rewrite (`handleTapInGrid` is tap-gesture/release-driven, no direct node-drag) that risks pan-from-node / double-fire regressions for little gain, so it was not done. The original design's Phase 3 (input arbitration) is specced in `docs/superpowers/plans/2026-06-12-grid-tree-zaxis.md` if ever revisited; the `Zone`/`HitIndex` half is reintroduced deliberately at that point rather than carried dormant.

### Where to look first

| Surface | Owning file |
|---|---|
| Top transport bar (Play / BPM / Rec) | `transport_zone.go` |
| Row strip (drum rows + grid + sliders) | `row_rack_zone.go` |
| Graph / grid pane (nodes + edges) | `game_draw_*.go`, `drumview_graph_*.go` |
| Timeline ribbon | `timeline_zone.go` |
| **Audio panel** (bottom) — six tabs | `eq_panel_zone.go` ← dispatcher |
| Sticky-bar chrome (channel pill, tab pills, freeze, …) | `audio_sticky_bar.go` |
| Per-row FX overlay (insert effects) | `insert_effects_*.go` |
| Overlay portals (dialogs, menus, tooltips) | `overlay_portal*.go` |
| **Menu frame** (title band, close ×, caption, footer) shared by every floating menu | `menu_card.go` ← `MenuCard` + `menuCardEntry` |
| Sidebar (mobile) | `sidebar_zone.go` |

### Audio panel — tab dispatch

`EQPanelZone` is a switch on `PanelTabState.ActiveTab()` (enum in `panel_tab.go`). The switch lives in `(*EQPanelZone).Draw` (`eq_panel_zone.go`) — every tab is a separate `case`. Tab transitions route through `onTab` (sticky-bar closure) → `EnsureAnalyzersForTab(tab, channel)` (`audio_panel_dispatcher.go`) so the right analyzer is enabled **before the next frame draws**. Programmatic transitions (`SetActiveTab`, scene catalog, JS export) go through the same dispatcher hook.

The **sticky bar** (`audio_sticky_bar.go`) owns: channel pill, tab pills, freeze, freq-scale, close, and the per-tab pills shown only on certain tabs. The bar maintains its own activeTab via `SetActiveTab` (parent calls before Layout) so Layout can decide whether to claim space for tab-specific chrome. **Each pill's `OnClick` is bound in `(*EQPanelZone).initButtons`** — that's the wiring chokepoint.

### Per-tab map

| Tab | Renderer | Key files | Tab-only chrome | Notes |
|---|---|---|---|---|
| **Wave** | `drawAnalyzerWaveform` | `render_waveform.go` | (none — always-on chrome only) | Reads `analyzer.State.Master` (or `state.Detail` when a per-row channel is picked). Cursor crosshair via `drawWaveformCursor`. |
| **Spectrum** | `drawAnalyzerSpectrumWithScale` + `drawSpectrumCurve` | `render_spectrum.go` | Slope pills (0/3/4.5 dB/oct), Pre toggle, Reset Hold | 10 ISO bars over a hi-res FFT curve underlay. Slope tilt = `SetSpectrumSlope`. Cursor chip = `formatHzShort + hzToNote + dB`. Pre overlay reads `audio.PreEQAnalyzerSnapshot` and paints through `drawPreEQOverlayFromSnapshot`. |
| **Levels** | `drawLevelsMultiChannel` | `render_meters.go` | Clear Clips, K-20 | Per-channel vertical strips + Master + aggregate column (HEADROOM big number, CLIPS (10s), LOUDEST, LUFS-S). Latches via `MultiLevelsLatch.Get(id)` keyed by inst id + "main"; Clear Clips → `MultiLevelsLatch.Clear()` + `audio.ResetClipsWindow()`. |
| **EQ** | `drawSpectrumBars` + `drawEQCurve` + sliders | `eq_panel_zone.go` (in-file), `render_eq_curve.go` | HPF/LPF buttons (inside content rect), per-band dB inputs | The only tab with mouse-draggable curve handles + band mute buttons. Curve cache in `z.curveCache`. EQ writes go through `applyMasterEQ` / `applyRowEQ` in `drumview_audio_eq.go`. |
| **Chain** (`TabScope`) | `ChainPanelZone` | `chain_panel_zone.go` + `chain_stage_card.go` + `render_chain.go` | Display-mode pills (OVR/SPL/DIF), AG, freeze, close | 5 stage cards in a left column (Synth/AntiPop/FX/EQ/Master). Each card carries a meter (`drawStageMeter`), an A/B chip, a mini-waveform when it's a tap (`drawChainStageMiniWaveform`), and FX badge count on the FX card (`drawChainFXBadgeCount` reads `audio.GetInsertEffects`). Trace area renders A vs B; trigger marker at 10% via `drawChainTriggerMarker`. |
| **Synth** | `(*synthPanel).drawSynthTab` | `synth_panel_zone.go` (header, build, hit areas) + `synth_macro_groups.go` (category tables) + `synth_macro_render.go` (root cards / drilled view) + `synth_pipeline_strip.go` + `synth_graph_pane.go` | Preview / Save As (Sampler-style header: `SYNTH · <display name>` left, actions right; mobile keeps them on the sticky bar) | ONE flow for every synth instrument (spec `docs/superpowers/specs/2026-09-05-synth-tab-overview-and-signal-graph-design.md`). **Root** = the PIPELINE STRIP (only ON stages, signal order, text pills; tap a pill to drill into that stage; a trailing `+ Add` pill lists OFF stages via the shared action menu) above CATEGORY cards (`buildMacroGroups`: melodic table Sound/Brightness/Attack/Length/Width/Human Feel/Movement/Body/Warmth/Pitch Drop/Voice/Flam, percussion table Punch/Boom/Pitch Drop/Tone/Length/Source/Grit — chosen by `synthCategoryTableFor`; HIDE-UNUSED via each entry's `active` predicate; every visible non-gain param is claimed by exactly one category — `TestMacroGroups_EveryInstrumentHasCategories`). Each card = label + one-liner + its macro dials (no signature graph: the per-card and per-stage thumbnails were removed with the modelled graphs, 2026-10-25). **Drilled** (category or stage, `synthDrillTarget`) = Back (icon button) · title · one-liner · the stage enable pill when the target maps to one pilled stage, then the target's ESSENTIALS as ParamCards (`ParamDef.Tier`) and a `More · n` divider (the reused Advanced divider) hiding the fine-tuning params. Bypassed stage = dim cards + scrim, still interactive. Right pane: the RENDERED-AUDIO GRAPH (§ Synth graphs below) — the overview at the root, the focused dial's / selected knob's view otherwise. Mobile: same screens stacked, graph band above the controls. Stage semantics still live on `instEditorSections` (`synthStageEnableState` / `applySynthStageToggle`); `setSelectedSynthSection` / `SelectSynthSectionByLabel` (the `selectSynthSection` JS export) now DRILL into the stage. All Buttons/grids/menus are the shared ones (`Button`+`buttonHitAdapter` via `instEditorBtns`, one `ControlGrid`, `openActionMenu`, `TraceSprite`). |

### Developer tools gate + Synth graphs from rendered audio (spec `docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md`, plan `docs/superpowers/plans/2026-10-24-synth-graphs-and-devtools.md`)

- **One switch: `devToolsOn()`** (`devtools.go`) = `devToolsBuild && !devToolsForcedOff`. `devToolsBuild` is a `var` set by `devtools_build_dev.go` (`!release`) / `devtools_build_release.go` (`release`) — the developer surfaces stay COMPILED IN, only hidden (user decision 2026-10-24). `make wasm` builds with `WASM_RELEASE_TAGS = noscenes,release` (comma: `-tags` takes ONE argument; guard `TestShippedWasmIsAReleaseBuild`). Preview the production UI in a developer build with `-devtools=false` (desktop, screenshots) or `?devtools=0` (dev WASM); tests use `forceProductionUIForTest(t)` and must build the Game AFTER it (the tab strip and bottom nav are built from the available lists at construction).
- **Hidden in production:** the Chain tab (`AllPanelTabs` / `bottomNavModes` omit it; `PanelTabState.SetActiveTab`, `EQPanelZone.SetActiveTab`, `setViewMode`, `SetChainVisible`, `SetActiveEQTab("chain")` refuse it — the current tab stays), and the Synth tab's low-level surfaces: pipeline strip, `+ Add`, every drill (`setSynthDrillTarget` refuses non-root targets; `SelectSynthSectionByLabel` returns false), `More`, stage pills. Guards `TestProductionHidesTheChainTab`, `TestProductionSynthHasNoLowLevelSurfaces` + the non-vacuous developer twins.
- **Production Synth tab:** compact cards (title + KNOB-style dials, `synthCompactCards`, density token `synthKnobD`) in a FLOW layout (`synth_macro_flow.go`: each card as wide as its dials, rows justified, riding `ControlGrid` one row per cell); cards with no live dial are hidden (`productionMacroCards`; every instrument keeps ≥ 4 dials on ≥ 2 cards — `TestEveryInstrumentKeepsProductionDials`; all fit two rows at 1280×720 — `TestProductionCardsFitTwoRows`); a card tap FOCUSES its first dial (`synth_focus.go`, focus validated on read so an instrument switch never carries it over); the graph pane is 40 % of the tab (min 320), on a phone a band above the cards. Developer builds keep today's card look and tap-to-drill; dial focus works in both.
- **Graphs are measured from the played voice, never modelled.** `audio.RenderDocForGraph(id, doc, pitch)` renders a document exactly as a trigger plays it, Sampler edit included (desktop: the voice-cache render + `ApplySampleEditToBuffer`; browser: `window.renderDocPCM` through the playback render worker (`voice_post.js`) + `postProcessVoiceRender`, waiting while any playback render is queued; `-tags test`: a toy voice for plumbing only). Guards `TestGraphRenderIsThePlayedVoice` (bit-identical, real build) + `synth_graph_render.browser.test.js`. `internal/soundview` (pure, wasm-safe, imports only `internal/wave`) analyses it: envelope / audible length / attack / onsets, multi-resolution YIN pitch, one STFT (hop size/2) for spectrum (+ `RefDB`, its absolute level) + spectrogram + centroid + tone/noise split, harmonics, a 1 ms onset curve + `ClickDB`, `Pitched()`, a period-aligned wave window (known-answer tests); pitch and the STFT stop at the audible end (`analysedLen`). `synth_graph_pipeline.go` renders slots NOW / LO / HI on the `synth.preview` pool (latest-wins, stale-while-revalidate, LRU 16, `pump` resubmits a refused job); its cache is keyed by CONTENT (`cacheKey` = `audio.GraphRenderKey`: sound signature + pitch + Sampler edit), so a sound is rendered once whichever slot or revision asks (`TestTheSameSoundIsRenderedOnce`); scenes settle it synchronously (`settleSynthGraphNow`, called from `settleSynthTab` + `applyMacroSceneState`) or two captures differ.
- **Views** (`synth_views.go`, geometry `synth_view_plot.go`, paint `synth_view_draw.go`): overview (a smooth loudness band + summary), loudness, the first moment (loudness at 1 ms, clicks read off it), pitch, colour (ONE level scale across NOW and every ghost — `TestToneColourSharesOneLevelScale`), harmonics, BRIGHTNESS over time (a line per sound, axis fitted to the sounds on screen — no heat map), tone-vs-noise, wave close-up (an unpitched note falls back to colour). **Two readouts** (spec `docs/superpowers/specs/2026-10-25-synth-dial-relevance-and-cheap-graphs-design.md` §3.3): WORDS by default (`synth_view_words.go`: "Short · Warm · Clean tone", "brighter than before" — no digit, unit or note name, `TestWordReadoutsHaveNoNumbersOrUnits`) and NUMBERS behind the **Details** toggle (`synth_graph_details.go`, persisted as `AudioPanelStateDoc.SynthGraphDetails` through `SetAudioPanelStateSink`; value tick labels only in NUMBERS mode; "(was …)" carries one number). Labels are placed clear of each other and of the axis words (`placeLabels`, `TestGraphLabelsNeverOverlap`). Every view reads only the analyses it warms (`TestViewsReadOnlyTheAnalysesTheyWarm` — the plot is built on the browser's one thread). A dial whose named view does not move on an instrument is shown through the view that does (`chooseDialView`, measured on the two end renders). A dial names its view and end words in `internal/assets/synth_dials.json` (`view`, `lo`, `hi`; validated by `synthdial.Load`, keys in i18n — `TestEveryDialNamesAViewAndEndWords`); a raw knob resolves through `synthParamViews` with NO fallback (`TestEveryVisibleParamHasAnExplicitView`). Every view: true scale, axes from the renders on screen (fixed while a control moves — `TestAxesHoldStillWhileTheControlMoves`), the control's ends as labelled ghosts, a BEFORE ghost from the gesture start (`captureGraphBefore`) with difference shading, and one measured readout. Developer builds: the view title is a picker (`synthGraphPickHitAreas`) cycling every view.
- **Trigger GLOW replaces the playhead** (`envelopeGlow`): full for 100 ms, then the note's own loudness until 40 dB down or 1.5 s; only for an AUDIBLE row (`rowAudible` at the highlight drain); its clock is `synth_graph_glow.go`. Guards `TestTriggerGlowsInsteadOfSweeping`, `TestGlowFollowsTheNoteEnvelope`, `TestTriggerFromAMutedRowDoesNotGlow`. Scenes `crop_synth_view_*`.
- **The old graphs are gone (user, 2026-10-25: "remove the old graphs"; spec §4.8 reinstated).** Deleted: the pure-Go voice model (`audio.RenderInstrumentPreview*`, `preview_render.go`), the concept renderers and their focus table, the "Your sound" mirror / full note, the per-knob ghost, the morph + playhead, the card signature graphs (`macroGraphKind`), the pipeline-strip stage thumbnails, the "Legacy" view, the `synthMirrorPCMLen`/`synthMirrorPCMChecksum` exports (+ `synth_knob_viz.browser.test.js`) and the helpers only they used (`ModularFilterResponse`, `GetChannelEQResponse`, `ModularFamilyStages`, the `synth-graph-morph` token). Guards `TestOldSynthGraphsAreGone` (no production identifier of the deleted code, module-wide) + `TestSynthGraphsDrawOnlyRenderedAudio` (a `synth_graph_*` / `synth_view*` file may use only the `audio` names that render or describe the document — `RenderDocForGraph` is the one PCM source). The render-parity tool's knob extremes are its `engine:` cases (the real C engine); its preview-model cases went with the model.
- **Focus, freshness, cost.** Esc (rung before Stop in `handleGlobalShortcuts`, `clearSynthFocusIfShown`) or a tap on empty card space (`synthFocusClearAdapter`, lowest z) drops the focus back to the overview. The pane draws NOW only for the instrument on screen and a BEFORE only for the control focused; end ghosts are content-addressed (`synthGraphEnds` memo), so a document edit re-renders them. The worker warms only the analyses the view needs (`synthViewNeeds` → `Analysis.Warm(need, yield)`); the STFT and pitch loops yield every few frames (`yieldEvery`; the STFT is the power of two nearest 43 ms — 2048 at 44.1/48 kHz), keeping the longest main-thread stretch on WASM near 3 ms. Budget: `TestGraphAnalysisStaysInBudget` (every view's analysis of a 1 s note ≤ 15 ms, fastest of 7; 0.3–8.8 ms on 2026-10-25). The glow is BAKED once per picture + theme and blitted with a ColorScale (`glowLayer`, `TestGlowIsBakedOncePerPicture`). Browser proof: the isolation bench's `synth-graphs` scenario (`ISOLATION_SCENARIOS=idle,tab-synth,synth-graphs ISOLATION_QUERY=devtools=0`: in-page pointer presses + drags on the dials via `synthDialRects()` and instrument switches) — 2026-10-25, 20 s / 32 gestures: no note missed or > 20 ms late, jsQueue p99 0.8 ms, FPS = the tab merely open.
- **Honesty guards (real build), no exception lists:** `TestEveryControlMovesItsGraph` — every dial SHOWN on every roster instrument moves the view the graph uses for it (`chooseDialView`) by ≥ 3 px between its 0 and 100 renders; raw low-level knobs are not covered. `TestSoundviewAgreesWithFingerprint` pins soundview to `fingerprint` by Spearman ρ over the roster (attack, audible length, centroid, noise vs flatness).
- **Landing videos still show the Chain tab**: the tour (`tour_steps.go` `tab-chain` / `nav-chain`) is unchanged until the media is re-recorded on a real display (`make showcase-video`) — slice 4 of the plan.

### Synth macro dials — ABSOLUTE, a pure function of the document

Each root category card carries up to six **macro dials** (specs `docs/superpowers/specs/2026-10-09-synth-macro-dials-design.md` + `2026-10-13-absolute-synth-dials-design.md`, which supersedes the first one's relative model). A dial is pure METADATA in `internal/assets/synth_dials.json`: a list of `{param, from, to, space, weight, requires?, when?}` terms — `from`/`to` = the param's ABSOLUTE value at dial 0 / 100 (`to < from` = falls as the dial rises; omitted = the param range; clipped to each instrument's own range, which differs per family), `space` `linear`/`log`/`magnitude`, `weight` = influence on the reading. `internal/synthdial` (pure) owns `Load`, `SpanFor`, `LiveTerm.Coord/Value`, `Resolved.Reading` and `Resolved.Turn`; the UI (`synth_dials.go`, `synth_dial_render.go`, `synth_dial_input.go`) only resolves, draws and routes input.
- **The reading is a pure function of the document**: `Reading = Σ wᵢ·clamp(uᵢ,0,1)/Σ wᵢ` over the live terms, shown as 0…100. Same sound → same knob after a restart, import, undo/redo or raw knob edit — there is NO dial state anywhere (the old anchor / self-check / one-sided `Reach` / `jnd` are gone). Guard: `TestSynthDialReadingIsAFunctionOfTheDocument`.
- **Turning = proportional shift**: `Turn(start, target)` moves every term the same fraction of its remaining travel toward the end turned to (each term keeps its offset → the instrument keeps its character), lands EXACTLY on the target, endpoints exact, target == reading → start verbatim, a term already past that end stays. The only state is the GESTURE (`synthDialGesture`: press / wheel-open captures the start values; one `SetInstrumentParamsLive` per detent; release/Accept = ONE `commitInstrumentParams`; Esc = start verbatim; a ±1 magnet at the press reading). A drag FREEZES the card's dial set (`synthDialActive`) — Length moving `amp_decay` flips Hold's `settles` gate (`TestDialRowFrozenDuringGesture`); a drag whose capture is dropped without a release (Esc, view switch) is cancelled by `healSynthDialGesture` (`TestEscMidDragCancelsTheDialGesture`). The voice length (render.seconds) feeds the caps and `settles`, so a reading is a function of the document alone — never of the tempo.
- **`rest: "start"`** marks a dial whose 0 is OFF (Drive, Weight, Edge = Butterworth Q, Fade-in = 5 ms …): a value at or past the off end reads 0. Calibration (`TestDialCalibrationSpread`): non-resting dials put the shipped median in [20, 80] with ≤ 25 % within 5 of an end a wider span could leave. Rules (`synth_dials_discipline_test.go`): one param → one dial per table, no picker/tuning, **no gate reads its own dial** (`synthDialConditionReads`), spans inside the widest range, shipped values inside spans, cap 6.
- **Engine mirrors** (like `synthDialConditions`: `ensemble`, `release`, `settles`, `pitchenv`, `morphs`, `filtenv` — the LFO cutoff path ignores the filter envelope). There are NO per-instrument range caps: **one knob, one meaning** — a dial position writes the same param values on every instrument (spans are the dial data only; `TestDialSpansAreTheSameOnEveryInstrument`; the bass family's `env_fast_rate` range was widened to hold Length's span). The release cap that motivated the old Tail cap was an engine defect, fixed in C: a release longer than half the voice ramps over the half it is given (byte-identical when not capped; `TestALongReleaseFadesOutInsideTheNote`). No dial end may silence an instrument (`TestNoDialEndSilencesItsInstrument`: Drive's `gen1_kick_sat`/`gen1_sat_k` start at 0.1 — a GAIN in several kick variants —, Shimmer no longer drives `gen1_cym_tone_m` — the modal plate's whole level —, Breath starts at 0.05).
- **Relevance — the DSP decides which dials show** (`synth_dial_relevance.go`, spec 2026-10-25 §3.2): a dial is shown when its 0→100 sweep is AUDIBLE (`soundview.CompareNotes`, the one audibility oracle) AND some view shows it (≥ 3 px); otherwise it is hidden, in every build. Judged on the instrument as the Synth tab first saw it in the open circuit (the ANCHOR — edits never re-hide or re-show dials; a new circuit re-anchors, `audio.CircuitGeneration`), keyed by `audio.SoundSignature`. Shipped sounds (and the startup demo's, as a cache) come from `internal/assets/synth_dial_relevance.gen.json` (`make gen-dial-relevance`; `TestDialRelevanceTableIsCurrent` fails with that command when a tuning pass made it stale); any other sound is measured in the background on the graph worker, one probe per job (`relevanceStep`). There is no known-weak list anywhere.
- **Layout**: rows of as many cells as fit (≤ 4: desktop cards 3, phone cards 4, so a 4-dial card keeps one row and a phone still shows two cards); uniform card height = the most rows any card needs. Scenes `crop_synth_dials_root` / `_perc_root` / `_turned` (absolute 85/60/30/20) / `_four` / `_voice`, `synth_dials_popup`.
- **Measurability contract** (`TestEveryDialIsMeasurable`, `!test`, the PLAYED voice through `RenderAsPlayed`): every SHOWN dial's ends ≥ 10 units from the shipped reading move a term ≥ 5 % of its span, and its 0↔100 sweep is audible on the played voice (independently of the graph render the verdicts were measured on); dials reached only by switching a stage on are held to the param half (their sound is measured at runtime). Audible = band envelope ≥ 1 dB over the note / ≥ 2 dB over ~70 ms / ONE third-octave band ≥ 3 dB / partials ≥ 5 ¢ (`soundview.CompareNotes`). `TestEveryDialMovesItsMetric` judges directions on the played voice too (no exceptions).

### Input routing

Pointer / touch events flow through a single dispatcher (`drumview_tree.go:330-447`). Each frame:

1. Every Zone publishes its `HitArea`s (`Rect` + `ZIndex` + `Handler` + optional `Touch` / `ClipRect`).
2. `HitIndex.At(x, y)` returns all hits containing the point, sorted by (exact-rect containment, then z-index descending).
3. The dispatcher iterates hits in z-order and calls `Handler.OnPress`. The first handler that returns `InputCaptured` (drag start) or `InputConsumed` (one-shot) stops the iteration; `InputIgnored` falls through to the next.
4. While a capture is active, every subsequent `OnDrag` goes straight to the captured handler **without re-testing hit areas** — the handler self-polices via cursor bounds on each move (see `timelineScrubHitAdapter.OnDrag` for the canonical example).
5. `OnRelease` clears the capture and normal dispatch resumes.

**The opaque-to-z contract.** Every Zone that renders a visible surface registers a full-bounds catch-all hit area at its nominal z via `NewInputCaptureHitArea(zone.Rect(), z, tag)`. Per-control hit areas inside the same zone sit at `nominal-z + 1` / `+2` and win on overlap. The catch-all guarantees a tap inside the zone's visible rect never falls through to a lower-z sibling. Discipline test: `zone_input_isolation_discipline_test.go`. **Pre-isolation bug**: a drag on a synth knob also dragged the timeline because chrome whitespace fell through to the timeline's grid-drag at z=110.

**Touch expansion**: to meet the 44-px touch-min on mobile without leaking, use `HitArea.Touch = true` + `ClipRect = visible-bar.Inset(-skirt)`. Don't physically enlarge the visible `Rect` past the zone's bounds — that's what made the timeline scrub leak before Phase 3.

### Keyboard routing — two claim tiers, never a denylist

Keyboard input is not spatial, so the HitIndex cannot arbitrate it. Two tiers answer "who gets this key?", and a surface declares itself into one of them — never as an inline `isKeyPressed` check at a shortcut site.

| Tier | Interface | Means | Examples |
|---|---|---|---|
| **Total** | `KeyboardClaimant.ClaimsKeyboard()` (`keyboard_focus.go`) | "every key is mine" — right for a focused text field | rename field, inst-menu search, Save As dialog, note-matrix picker, theme overlay |
| **Partial** | `KeyboardKeyClaimant.ClaimedKeys()` + `HandleClaimedKeys()` (`keyboard_key_claim.go`) | "these keys are mine, the rest pass through" — right for a value control | Sampler/EQ precision wheel (`MobileWheelPopup`), row/master volume (`SliderPopup`), armed Synth control (`armedControlKeys`) |

`handleGlobalShortcuts` runs them in order: Esc → Enter → **total gate** (`Game.KeyboardClaimed()` → return) → `g.dispatchClaimedKeys()` → `keyboardRouter.dispatchGated(blocked)`. The partial claimants act FIRST and report what they consumed; `suppressKeys` (`input.go`) masks exactly those keys for the gated nodes, **per key, not per node** — so a wheel popup owning the arrows still leaves the grid node's `[ ]` zoom and `0` reset working, and Space still plays.

- **Claim and handle are ONE interface on purpose.** A claim with no handler is a dead key; a handler with no claim is the bug the contract exists to stop — the Sampler Pitch wheel did neither, so an arrow press panned the camera instead of moving the value.
- **Arrow keys must be declared in BOTH spellings** (`KeyArrowUp` *and* `KeyUp`): real Ebiten aliases them, the `-tags test` ebitestub declares them distinct. Use the shared `keysStepUp/Down/Left/Right` slices + `anyKeyJustPressed`.
- **Only one site may read an arrow key.** `isKeyJustPressed` is non-consuming, so a control that reads the arrows in its own `Update` *and* through the claimant steps twice per press. Guard: `TestArrowKeysReadOnlyAtSanctionedSites` (`popup_control_keys_discipline_test.go`) allowlists the four sanctioned files.
- A closed control must claim nothing (`TestClosedPopupControlsClaimNoKeys`) — a claim outliving its popup kills arrow-pan for the rest of the session.
- Value-control semantics are uniform across tabs: **Up/Down = value, Left/Right = step resolution (Right = coarser)**. Enter accepts, Esc cancels (via `portalEnterHandler` / `portalEscapeHandler`, which run before the total gate).
- Tests: `popup_control_keys_test.go` (real key presses through `Game.handleGlobalShortcuts`), `synth_armed_keys_test.go`, `keyboard_focus_discipline_test.go`. Scene: `sampler_wheel_keyboard_step`.

**Tab / view-mode switching is a single visibility predicate.** When the mobile bottom-nav switches `viewMode` (Pads / EQ / Wave / Spectrum / Lvl / Chn / Syn), the only thing that needs to change is each zone's `visible()` return value. The tree picks that up on the next `Update()` Layout phase: invisible zones get their HitAreas cleared from the index AND their Draw skipped. Adding a new mobile view (e.g. a hypothetical "Pads2") means: (a) define the `viewMode` enum value, (b) wire the segmented control to call `setViewMode(viewModeNewMode)`, (c) for every zone that has a presence in this new mode, extend its `visible()` predicate to return `true` when `dv.currentViewMode == viewModeNewMode`. No code in the dispatcher changes; no per-mode `if-else` ladders elsewhere. The tree is the single chokepoint. Regression test: `pads_input_regression_test.go`. Discipline test: `TestZoneInvisibleEqualsNoInput`.

### Cross-tab helpers

- **`?` legend chip + chevron expander** (every tab) — `audio_panel_legend.go` + `audio_sticky_bar.go` (`LegendBtn`, `ExpanderBtn`). Click handlers bound in `(*EQPanelZone).initButtons`. Legend text per tab lives in `audioPanelLegendText[PanelTab]` map; add an entry when adding a tab.
- **`PlainEnglish(label)`** — `plain_english.go`. Maps jargon ("Cutoff", "Attack", "OVR", …) to kid-readable glosses ("bright or dull", "how quickly it starts", "show A and B together"). Use anywhere a knob/button label needs an inline subtitle.
- **Audio-panel state persistence** — `userprefs/store.go` exposes `AudioPanelStore` (Spectrum slope, Pre overlay, K-20 view, Chain A/B taps). Gated by `userprefs.UseAudioPanelStateV3` — flip false in tests to verify legacy-prefs handling.
### Audio-tab source of truth — layered byte-exact capture (`internal/audiotap`)

The programmatic replacement for "look at the screenshot". `CaptureAudioTabTap` (`internal/ui/audio_tab_tap.go`) walks one channel from the analyser data's SHAPE, through the analysis VALUES, to the exact numbers a tab will DRAW — calling the renderers' own functions (`spectrumBandDB`, `audioDBWindow`, `audioDBNorm`, `eqBandLevels`, `autoGainForPeak`), never a parallel reimplementation. Each layer is digested separately.

**Why layered, not one hash.** The 2026-09-19 blank-Spectrum defect had correct audio, a correct renderer, and an empty plot: only the SHAPE between them was wrong. A single end-to-end digest goes red and says nothing; a layered one leads with `** a SHAPE section moved — fix that first; the rest is fallout`, which is the diagnosis that took three manual rounds to reach. Section names are `shape.*` / `analysis.*` / `render.*` and sort outward from the audio.

**Two digests per section, because there are two honest questions.** `digest` is sha256 over raw IEEE-754 bits — exact, used WITHIN a platform (determinism + the committed golden). `digest_q` snaps to `audiotap.QuantumStep` (1e-6) and is used ACROSS platforms, because raw bits are **not** reproducible between amd64 and wasm downstream of a transcendental: measured on this repo's own fixture, `math.Exp(-1000/90)` is `0.034329453106454272` natively and `0.03432945310645428` under `GOOS=js` — one ULP, amd64 uses FMA and wasm does not, reaching ~8e-15 dB. The quantum sits nine orders above that noise and four below anything a pixel can show. Integer/bool sections (`shape.*`) carry an identical `digest` and `digest_q`, so for them the cross-platform check is still exact-bits (`TestIntegerSectionsAreExactInBothDigests`).

| Layer | File | Runs under | Catches |
|---|---|---|---|
| Framework | `internal/audiotap/tap_test.go` | `-tags test` | The digests actually discriminate; 1 ULP absorbed cross-platform but NOT within a platform; a length change reports as SHAPE |
| Derivation + golden | `audio_tab_tap_test.go` | `-tags test` | Determinism, band coverage at every shipped sample rate, drift from `testdata/audio_tab_tap_golden.json`. `TestAudioTabTapDetectsAResolutionChange` proves the capture is not blind to the defect class it exists for |
| Cross-platform | `xplat_audio_tab_tap.browser.test.js` | browser | The browser, running the SAME Go code over the SAME canonical fixture, reproduces the native golden section by section — plus `audioTabTapLiveShape()`, which measures what the LIVE analyser bridge really delivers and asserts every display band holds a bin |
| Real audio e2e | `audio_tab_tap_realaudio_test.go` | `make test-real` (`!test`) | Shipped synth → real offline mixer → the real `wave.FFTObserver` → real render derivations. Per-channel determinism, band coverage on real analysis, and that four instruments produce four DIFFERENT digests (a capture that collides is reading something other than the audio) |

- **Regenerating the golden**: `AUDIO_TAP_GOLDEN_UPDATE=1 go test -tags test -run TestAudioTabTapMatchesGolden ./internal/ui/`, then re-run the browser half — both read the same file.
- **The canonical fixture is deliberately BASS-HEAVY** (`CanonicalChannelMetrics`). Every pre-existing fixture decayed from bin 0, so the mid and high bands always had data and the hole at the bottom was invisible for months.
- **Adding a tab**: extend `CaptureAudioTabTap` with a `render.<tab>.*` section derived through that tab's own functions, and regenerate the golden. A tab with no section in the capture is a tab no digest protects.

- **Spectrum bin width is a display invariant, not a tuning knob** — `analyzer.SpectrumFFTSize` (2048) is the analysis FFT length on **both** platforms: the desktop service configures its observer with it (`audio/engine.go`) and the browser bridge asks the AnalyserNode for it (`audio.BridgeAnalyserFFTSize`). The Spectrum tab aggregates bins into fixed frequency bands, and `spectrumBandDB` reports a band holding **no bin** at the reference floor — indistinguishable from silence. The narrowest ISO band is 24 Hz wide (20–44 Hz), so `SampleRate/FFTSize` must stay under that at every rate the product runs at; 1024 gives 43–47 Hz and does not. Two defects came out of that: the browser compounded it with a 64-bin linear downsample (375 Hz per bin at 48 kHz) and every band below ~375 Hz rendered empty — **a kick's whole spectrum, so the plot was blank** — while desktop at 43 Hz/bin looked correct in every screenshot; and desktop itself left the 31 Hz band empty whenever the engine ran at 48 kHz. Guards: `spectrum_bin_resolution_test.go` (`TestSpectrumBinsCoverEveryDisplayBand` feeds a **uniformly loud** spectrum at every shipped `(binCount, sampleRate)` shape — every existing fixture decayed from bin 0, so the mid/high bands always had data and the hole at the bottom was invisible) and `xplat_spectrum_channel_data.browser.test.js` (pixels, **bass third** of the plot — master is broadband, so a full-width probe passes while the bass is empty).
- **Go asks JS for the bin count; never take the JS default** — `channelAnalyzerSnapshot(id, bins)` groups bins **linearly**, so any downsample spends its loss where the display bands are narrowest. `analyzer_wasm.go` passes `audio.BridgeSpectrumBins` (== the analyser's own bin count, i.e. no downsample); it used to invoke with no `bins` argument at all and silently inherit JS's 64. One AnalyserNode serves both domains, so the time-domain export is capped separately (`audio.BridgeWaveformSamples`, JS `WAVE_EXPORT_DEFAULT`) — the scope taps, Wave tab and per-stage traces keep the exact window they had, and raising the FFT costs them nothing.
- **Only Master and Detail carry slice data** — every row in an `analyzer.State` is an `InstrumentMetrics` (two levels, two flags), so `buildAnalyzerStateFromSnapshotsImpl` reads rows through the scalar-only `audio.ChannelAnalyzerMetrics` and pulls a full snapshot for **master + the detail channel only**. It used to fetch a full Spectrum+Waveform per row and discard both — affordable at 64 bins, not at full resolution, on the path that drove a production OOM once. Guard: `TestAnalyzerStateFetchesSliceDataOnlyForMasterAndDetail`.
- **Master analyzer wiring** — never bypass `EnsureAnalyzersForTab`. If you add a new tab that needs analyzer data, extend `tabReadsMasterAnalyzer` in `audio_panel_dispatcher.go`. The pre-Phase-0 bug (flat output on tab switch) lived in code paths that enabled the analyzer based on EQ channel selection, not tab activation — don't reintroduce that coupling.

### Adding a new audio-panel feature — recipe

1. **New tab pill chrome**: add fields to `AudioStickyBar`, init in `NewAudioStickyBar`, layout under the `b.activeTab == TabXxx` block, draw + hit-area register, expose accessor, bind `OnClick` in `(*EQPanelZone).initButtons`. Tests live in `audio_sticky_bar_phase1_test.go`.
2. **New render data**: add fields to `analyzer.State` (or `ChannelMetrics`); populate in `analyzer/service.go`'s `processTick`. If the value lives in the `audio` package (LUFS, clip-window counter), wire a getter callback through `analyzer.Config` so `analyzer` doesn't import `audio` ([[feedback_event_source_attribution]] applies to logging too — keep edges clean).
3. **New tab-specific control that mutates audio state**: bind the pill's `OnClick` to a tiny lambda in `initButtons`; never reach back into Game from the sticky bar.
4. **Persisting tab state across sessions**: add a field to `userprefs.AudioPanelStateDoc`, gate behind `UseAudioPanelStateV3`, add a round-trip test in `userprefs/audio_panel_state_test.go`.
5. **Alloc budget**: if your change adds per-frame draws on Levels (which has the tightest cap), update `perTabAllocBudget[TabMeters]` in `eq_panel_draw_alloc_discipline_test.go` with a one-line rationale citing the new render code.

### Live apply vs. gesture commit — a control must be AUDIBLE before it is committed

Every value control has two obligations, and they are **separate**:

| | Where | Fires | Does |
|---|---|---|---|
| **Live apply** | the edit site (`OnChange`, the drag handler, the arrow-key claimant) | on every value change | writes to `internal/audio` so the next trigger sounds different |
| **Gesture commit** | `drumview_release_commit.go`'s `commit*` | once, at release / Accept / tap-away | `emit*` + `recordUndoStep` — **one** undo step and **one** log line per gesture |

The commit must never be the only path to audio. Read `drumview_release_commit.go` as the checklist: `commitEQBand`, `commitMainVolume`, `commitRowVolume`, `commitInstrumentParams`, `commitEQFilter`, `commitEQBandMute` are all emit+record only, because their live write already happened at the edit site (`applyBandGainLive`, `audio.SetMainVolume`, `DrumRow.Volume`, `audio.SetInstrumentParam`, `applyEQ`). `commitSamplerEdit` was the exception — it carried `audio.SetSampleEdit` itself — so turning a Sampler knob moved the knob on screen while the audio layer kept the old chop until Enter / tap-away / the 600 ms wheel debounce / drag release. Now `applySamplerEditLive` runs per change and the commit only announces.

- **Splitting an audio setter that publishes:** `audio.ApplySampleEditLive` applies without announcing; `audio.SetSampleEdit` is that plus the publish (kept for import / JS bridge / undo replay, which ARE the whole gesture). When you move a publish out of a setter, **move it to the commit site explicitly** — the setter's no-op-on-identical-value skip will otherwise swallow it once the live path has already stored the value, and the narrative line disappears. Guard: `TestSamplerLiveApplyStillAnnouncesOncePerGesture` (three detents announce zero times, gesture end announces once).
- **Live ≠ audition.** Applying live means the NEXT trigger sounds different; it never plays a note on its own (see [[feedback_synth_no_auto_audition]] / [[feedback_next_trigger_param_deferral]]).
- **A live apply writes to a PROCESS-GLOBAL store**, so any test that merely turns the control now leaks into the next test. `samplerCardGame` clears `audio.ClearSampleEdit` in `t.Cleanup` for exactly this reason — a wheel-nudge test made `TestImportClearsStaleSampleEdit` export a project already carrying the edit it asserted import had cleared.
- Tests: `popup_control_live_audio_test.go` asserts against the **audio layer's** state for every control family, never the UI's copy — a knob that moved its own `Value` while the audio layer stayed put is precisely the bug.

---

## Testing

### Go Tests (240+ files)

**Fast Path** (stubbed Ebiten, no X11):
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```

**Real Ebiten** (needs X11):
```bash
cd src/go && xvfb-run -a go test ./...
```

**Key Categories**:
- **Predictor/audio**: `drum_sync_invariant_test.go`, `mute_node_audio_test.go`, `probability_prediction_consistency_test.go`
- **DrumView caching**: `drum_rows_layer_cache_test.go`, `drum_row_cache_test.go`
- **Import/export**: `import_validation_test.go`, `import_button_flow_test.go`
- **Parity**: `parity_*_test.go` (25+ files)
- **Performance**: `game_draw_throttle_test.go`, `perf_test.go`

### Playwright Browser Tests

```bash
GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js
```

**Key Suites** (after the JS test suite reduction — see § JS Test Charter below):
- **Bridge plumbing**: `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue; Go-side drift guard: `internal/ui/js_exports_catalogue_drift_test.go`), `wasm_bridge_input_sanity.browser.test.js`, `wasm_bridge_error_paths.browser.test.js` (malformed/out-of-range/wrong-typed args)
- **Cross-platform parity**: `xplat_parity.browser.test.js`, `xplat_audio_compare.browser.test.js`, `xplat_audio_tab_tap.browser.test.js` (the layered audio-tab capture: the browser must reproduce the native golden section by section, plus a LIVE analyser-shape probe — see § Audio-tab source of truth), `xplat_spectrum_channel_data.browser.test.js` (the Spectrum tab's per-instrument channel really renders BARS in the browser — pixels via `captureScreenshot`+`extractRegion`, not `getImageData`: Ebiten draws through WebGL with no `preserveDrawingBuffer`, so a canvas read returns an empty buffer and "detects" the bug everywhere, master included), `xplat_output_capture.browser.test.js`, `xplat_browser_export_import.browser.test.js` (cross-ENGINE: same edit in Chromium/Firefox/WebKit must export byte-identical project JSON, each engine must import the others' files and re-export them unchanged, and the WASM synth voice must render identically; the master mix is compared with desktop by `xplat_audio_stages`), `xplat_audio_stages.browser.test.js` (§ One audio pipeline: every stage after the voice and whole circuits, per sample against desktop, in Chromium, Firefox and WebKit)
- **WebAudio**: `webaudio_smoke.browser.test.js`, `webaudio_perf.browser.test.js`, `webaudio_perf_e2e.browser.test.js`, `webaudio_pan_stress.browser.test.js`, `webaudio_bpm_stress.browser.test.js`, `webaudio_drums_consistency.browser.test.js`
- **Load / startup**: `wasm_load_startup.browser.test.js` — gzip-served boot waterfall + time-to-interactivity (TTI = `window.__beatmoBoot.firstFrame`, the first painted frame after `setSimpleDraw(false)`). Tight deterministic gate on compressed transfer size; CI-safe ceilings on wall-clock TTI/instantiate (override `WASM_LOAD_TTI_MAX_MS` / `WASM_LOAD_INSTANTIATE_MAX_MS` / `WASM_LOAD_TRANSFER_MAX_BYTES`). Asset-size budgets (raw + gzip, enforces `-s -w` strip + gzip serving) live Go-side in `internal/deploy/asset_size_test.go` (`TestWasmAssetSizeBudget`, skips when `main.wasm` absent).
- **Heard-audio / isolation**: `webaudio_audio_isolation_bench.browser.test.js` (UI-load matrix × playback probe), `audio_probe_analysis.browser.test.js` (pure Node: the probe analysis against synthetic logs) — see § Browser audio isolation
- **Worklets / workers**: `worklet_insert_effects.browser.test.js`, `recording_lifecycle.browser.test.js`, `recording_capture.browser.test.js`, `recording_capture_backpressure.browser.test.js` (bounded-outbox drop/ack protocol; no WASM build needed)
- **Real input / e2e**: `e2e_workflow.browser.test.js`, `e2e_real_input_circuit.browser.test.js`, `e2e_real_input_edge.browser.test.js`, `e2e_real_input_live_edit.browser.test.js`, `e2e_node_click.browser.test.js`, `e2e_drum_row_controls.browser.test.js`, `e2e_transport.browser.test.js`, `e2e_timeline_seek.browser.test.js`
- **Touch / mobile**: `touch_gestures.browser.test.js`, `touch_integration.browser.test.js`, `touch_pinch_no_node.browser.test.js`, `touch_device_matrix.browser.test.js`, `touch_dropdown_scroll.browser.test.js`, `mobile_audio.browser.test.js`, `mobile_audio_unlock.browser.test.js`, `mobile_speaker_routing.browser.test.js`, `mobile_file_picker.browser.test.js`, `mobile_native_input.browser.test.js`, `mobile_text_input.browser.test.js`
- **Visual regression**: `visual_regression.browser.test.js`, `visual_device_parity.browser.test.js`, `visual_mobile_parity.browser.test.js`
- **Browser platform**: `browser_media_session.browser.test.js`

**Test logging**: Default silent. Enable with `TEST_LOG=1`, set `TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR`.

### JS Test Charter — what the JS suite owns

**Logic correctness lives in Go.** The JS browser suite owns *only* the Go↔WebAudio/Worklet/Worker/real-input boundaries that Go cannot reach. New tests must justify themselves against this charter or they belong in `internal/ui/` instead.

The JS suite is responsible for, and ONLY for:

1. **WebAudio behavior** — `AudioContext` sample rate, `decodeAudioData`, gain/pan node graphs, hard limiter clipping, anti-pop fade timing, scheduling lead/lag at `AudioContext.currentTime`.
2. **AudioWorklet message protocol** — `recording_capture_worklet.js` configure/setParam/data-batch flows; backpressure & port transfer.
3. **Web Worker boundary** — `recording_encoder_worker.js` (WAV/FLAC/ZIP encoding off-thread; Blob/zip download e2e).
4. **WASM↔JS bridge plumbing** — every Go-registered JS export is callable, returns the expected shape, doesn't throw. Tracked in **one** place: `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue).
5. **Cross-platform parity** — WASM-compiled Go produces bit-identical output to native Go for the same inputs (predictor goldens, audio rendering goldens). `xplat_parity.browser.test.js` and `xplat_audio_compare.browser.test.js`.
6. **Real input dispatch through the canvas** — touch, pointer, multi-touch via Playwright/CDP; mobile audio unlock gestures; iOS speaker routing; mobile viewport.
7. **Visual regression** — Playwright canvas screenshot diffing where pixel-level browser rendering matters.
8. **Browser-only platform features** — Media Session API, file picker dialogs, mobile orientation events, browser storage.

**What the JS suite must NOT do (use Go instead):**
- Re-test predictor/timeline/graph/scheduler logic — Go's `core/engine`, `core/model`, `internal/timeline`, `internal/ui` cover it directly with deterministic stubs.
- Re-test UI state machines (dropdowns, sidebar, scroll, orientation, layout) — Go's ebitenstub harness covers them; see `dropdown_*_test.go`, `widget_layout_test.go`, `popup_close_test.go`, `responsive_layout_test.go`.
- Re-test DSP correctness (biquad, compressor, insert effects, EQ) — Go's `internal/audio` (88% native coverage) owns it.
- Re-test JSON import/export round-trips — Go's `import_export_logic_test.go` and friends own it.

**When adding a new JS export in Go:** add an entry to `src/js/wasm_bridge_smoke.browser.test.js`'s catalogue (existence at minimum, callable if safe args exist). Do NOT create a new dedicated `*.browser.test.js` file just to verify the export got registered.

**The bridge seam fails OPEN, so two guards watch it** (`js_bridge_contract_discipline_test.go`):
- `TestBridgeCallsPassEveryDeclaredArgument` — for the functions in `noDefaultBridgeFns`, Go must pass **every** parameter audio.js declares. A JS default is invisible from Go and is a second source of truth nothing reconciles: `channelAnalyzerSnapshot(id)` silently inherited JS's `bins = 64` and emptied the Spectrum tab's bass bands in the browser. Nothing failed, because the call compiles and returns the right shape.
- `TestTypeofGuardsNameRealExports` — a `typeof X === "function"` guard in a browser test must name a real export. `rowInstrumentIDs()` is a **Go test helper**, never a JS export; guarded that way with a hardcoded fallback beside it, the guard swallowed the missing export and the fallback probed an instrument the circuit did not contain, reporting an empty channel as a dead analyser. The same guard found `window.stop()` used as a fallback for `stopPlay` — that is the DOM method that **aborts page loads**.

**`getChannelChain` fails closed on a missing or empty id.** The master bus is `'main'` and only `'main'` (Go's `channel.ID` never sends `''`); `undefined`/`null`/`''` used to map to master — so a probe asking for a nonexistent channel read MASTER and every tap agreed with every other, which is exactly what a correct reading looks like.

**Each analyser tap owns its output buffers** (`chain._taps[analyserKey]`). The three taps on a chain shared one `_snapResult` and one pair of Float32Arrays; it was safe only because the Go bridge copies bytes out before the next call — an invariant nothing stated. It stopped being theoretical once the taps got different `fftSize`s.

**`bins === 0` is the scalar-only contract.** `ChannelAnalyzerMetrics` passes it so JS skips the frequency read, the downsample and the wave copy entirely, instead of computing a spectrum for a caller that only reads two numbers off it.

**When reducing a JS test:** the recipe is:
1. Read both the JS test and the candidate Go test that covers the same scenario. Verify the Go test is strictly larger (or equal) in coverage.
2. If Go is partial, write the missing Go test FIRST (in `internal/ui/` or `internal/audio/`).
3. Either delete the JS test outright (preferred when Go fully covers it), or replace its body with a 5–10-line bridge-plumbing assertion using `bridge_smoke_helpers.js` (`assertExportExists`, `callExport`, `assertReturnShape`). Cite the Go test(s) in a top-of-file `COVERED-BY-GO:` block.
4. The bulk-reduction runner `reduced_bridge_smoke.js` was retired after Batch 12; if you need the same shape of stub, write it inline against `bridge_smoke_helpers.js` directly.

**Reference helpers:** `src/js/bridge_smoke_helpers.js` is the canonical source of bridge-plumbing assertions, used by `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue). For the historical reduction pattern, see git history before the Batch 12 cleanup.

---

## Debugging

### Log levels

The `internal/log` package enforces this contract — drift is caught by `internal/log/forbidden_at_info_test.go` (regression guard) and `internal/eventlogger/coverage_test.go` (every hooks.Kind has a formatter).

| Level | Rule |
|-------|------|
| **INFO** | One line per **user action** or **major component lifecycle event**. Never per-frame, never "ignored X because Y", never internal-mechanism breakdowns. Most user-action narrative is emitted by `internal/eventlogger/`, which subscribes to `hooks.Bus` and renders one human-readable line per published event. Interaction telemetry (button taps, popup open/close, scroll, search, text commit, view switch) flows through the same pipeline via Layer-B kinds. |
| **WARN** | Degraded-but-functional. Slow paths, parity mismatches when non-fatal, dropped frames, fallbacks taken. Has its own `LevelWarn` threshold — silenceable independent of INFO. |
| **ERROR** | Failure: an operation could not complete. |
| **DEBUG** | Mechanism detail useful for diagnosing a specific subsystem. May fire per event or per pool job, but never per frame. |
| **TRACE** | Firehose. Per-frame, per-cell, per-tick. |

**Output format**: `15:04:05.000 LEVEL [tag] message`. The tag is the first bracketed token of the format string (extracted automatically); when no tag is present the bracketed section is omitted.

**One pipeline, no stdlib `log`.** Production code never imports the standard library's `log` (guard `TestNoStdlibLogInProduction`, `internal/log`): it ignored `-log` and printed into every test run (two `[AUDIO CAPTURE]` lines per offline render). Packages without an injected `*Logger` (`internal/audio`) use the package-level `log.Debugf/Warnf/…`, which go to `log.Default()` — the binary points it at its `-log` logger (`game_log.SetDefault`, `cmd/beatmo.go`). The env-gated audio diagnostics (`DEBUG_MIXER`, `DEBUG_CHANNEL`, `TEST_RAW_VOICE`, `BEATMO_SYNTH_DISPATCH_DEBUG`, `BEATMO_PREWARM_DEBUG`) now also need the level: per-block ones are TRACE (`-log TRACE`), per-voice ones DEBUG.

**Tests: silent, with a failure tail.** Under `go test` (no `BEATMO_TEST_LOG=1` / `-v`) a logger's lines go, at its own level, to one bounded process-wide ring (`internal/log/test_tail.go`) instead of the console. A test that calls `game_log.TailOnFailure(t)` (idempotent — helpers call it freely) prints the latest 40 lines logged while it ran, only if it fails; the output-capture render helpers already do. Test tables and measurements go through `t.Log`, never `fmt.Printf`.

**Tag source of truth:** the bracket tag for every `hooks.Bus` event is defined by `ActionMeta.Tag` in `hooks.ActionRegistry` (`internal/hooks/actions.go`). `internal/eventlogger` reads it from the registry; formatters return only the message. All bracket tags are **lowercase** at every log level and in every package — enforced by `internal/log/tag_case_test.go::TestNoUppercaseInfoTags`. See the "Event log tags" subsection above for the full taxonomy and anti-pollution rules.

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `PPROF=1` | Start pprof server on `localhost:6060` |
| `PERF_LOG=1` | Emit perf logs every ~2s |
| `TIMELINE_TRACE=1` | Trace timeline masking logic |
| `PARITY_FATAL=0\|false` | Disable parity panics |
| `PARITY_WATCH=log\|panic` | Parity mode (desktop) |
| `PARITY_WASM_FATAL=1\|true\|panic` | Enable parity panics on WASM |
| `TEST_LOG=1` | Enable test logging |
| `BEATMO_TEST_LOG=1` | Enable in-process test logger output (alias for the package's runtime gate) |
| `BEATMO_TEST_LOG_LEVEL=TRACE\|DEBUG\|INFO\|WARN\|ERROR\|NONE` | Override log level when test logging is enabled |
| `BEATMO_INFO_LOG=off` | Disable the hooks.Bus → INFO narrative consumer (default: on) |
| `BEATMO_EVENT_LOG=<path>` | Write the full hooks.Bus event stream to a JSONL file (separate from INFO narrative) |
| `BEATMO_EVENT_LOG_VERBOSE=1` | Include verbose kinds (drag-progress / `EventDragProgress`) in BOTH the INFO narrative and the JSONL stream |
| `DEBUG_GEOM=1` | Verbose geometry logs |
| `SCOPE_EXPORT=1` | Enable scope export flight recorder (JSONL) |
| `SCOPE_EXPORT_PATH=<path>` | Output file (default `scope_export.jsonl`) |
| `SCOPE_EXPORT_INTERVAL=<secs>` | Snapshot interval (default `2`) |

### JS Exports (WASM)

**State Inspection**:
- `dumpRowState(row)` — Row timeline + predictor state
- `dumpTimelineSegments()` — Full timeline snapshot
- `visibleAt(row, abs)`, `triggeredAt(row, abs)` — Predictor queries
- `rowWindow(row)` — Current row window

**Graph Manipulation**:
- `addNode(i, j, type)`, `deleteNodeGrid(i, j)`
- `addEdgeGrid(i1, j1, i2, j2)`
- `setNodeLogicGrid(i, j, kind, n, p)`

**Playback**: `startPlay()`, `stop()`

**Performance**: `resetPerfStats()`, `perfStats()`, `resetAudioScheduleMetrics()`, `getAudioScheduleMetrics()`

---

## Critical Gotchas

Each gotcha has full documentation in the referenced source file.

- **Every entity has ONE typed name; closed sets are Go enums** (plans `docs/superpowers/plans/2026-10-24-typed-entities-phase1.md` + `…-closed-enums.md`).
  - **Instrument id** = `instrument.ID` (open set, `internal/instrument/id.go`). Built-ins are the generated constants `roster.Oboe` … (`internal/instrument/roster`, written by `go generate ./internal/audio/` from `index.json`; `roster.All` is THE roster list, `roster.Has(id)` membership — there is no `audio.BuiltinInstrumentIDs`). Every other shape is minted only in package `instrument`: `UserID(name, hash)` (`user.<slug>.<hash>`), `UserSampleID(sum)` (`user.sample.<hash>`), `Instance(base, n)` (`<base>--N`, read back with `id.Base()` / `id.IsInstance()`), `SampleID(relPath)` (catalog WAVs; never contains `--`). The roster generator rejects an id with an empty dash part or `main`, so a built-in can never look minted (`TestNoRosterIDLooksMinted`). Data (JSON, JS-bridge args, flags, prefs, file names) goes through `instrument.ParseID` ONCE where it is read (non-empty, no whitespace, not `main`); JS exports use `args.Instrument(i)`. A WAV the user uploads is a user sample named by its typed text (`wavInstrumentID`) — never the built-in its name matches.
  - **Mixer channel** = `channel.ID`, an OPAQUE struct (`internal/channel`): `channel.Master` (wire `"main"`) or `channel.Of(inst)`; data goes through `channel.Parse` (JS: `args.Channel(i)` / `ChannelOr`). The zero ID is **no channel, never the master bus** — `""` is not a spelling of master anywhere (Go or audio.js); a zero channel is refused by the UI setters, dropped by the mixer (`ensureChannel` hands back a detached channel) and never sent to JS. Get the instrument back with `ch.Instrument()`; `ch.String()` is the wire spelling.
  - **Closed sets are `type X int` enums over one `enum.Names` table** (`internal/enum`): `String`, `MarshalText`, strict `UnmarshalText` (an unknown spelling is a decode error, so an import with one is rejected before anything resets), `ParseX`. Index 0 named `""` is the enum's "none". Converted: `instrument.Role`, `audio.Category` (the instrument menu keys tabs by it; `Label()` is the display text), `model.NodeType` / `LogicKind` / `GrooveKind` / `GroupParam` (the retired logic aliases `none`/`prev_fired`/`every_n_loops`/`skip_if_prev_*` and case-folding are gone), the project row `kind` (`kindBuiltin`/`kindSample`, checked against what the row carries; a built-in is a sample only through embedded PCM, never a WAV path — `TestImportNeverReplacesABuiltinWithAWAV`), bundle `BundleKind` / `WavSourceKind`, catalog `SoundSource`, hook payload `SeekOrigin` / `EQFilter` / `SendKind` / `ThemeScope`, `uistate.ViewMode`. `audio.EffectType` stays a named string but decodes strictly against `audio.EffectTypes` (`TestEveryEffectTypeIsRegistered`). View-mode slugs have one spelling each (no `meters`/`scope`/`rows` aliases). **`enum.Names` has NO methods** — its operations are generic functions (`enum.String(roleNames, r)`, `enum.Unmarshal(roleNames, b, r)`, …): Ebiten links `text/template`, whose `MethodByName` makes the linker keep every exported method of any type that reaches an interface, so methods on `Names` cost a dead wrapper per method per enum in main.wasm (58 KB over 15 enums). Same dictionary call, same type safety. Guard `TestNamesHasNoMethods`; diagnose any such retention with `go build -ldflags=-dumpdep` (`<ReflectMethod>` / `<UsedInIface>` edges).
  - **UI element ids are typed too:** the node sidebar keys rects/buttons by `sbID` (`game_node_sidebar_ids.go`: fixed elements + constructors `sbSectionHeader`, `sbMinus/Value/Plus(field)`, `sbLogicOption`, `sbGroupChip`, `sbNoteCell`, …) and the group menu by `gmID` — never a concatenated or `Sscanf`-parsed string. `String()` keeps the old spelling for logs, hover ids and the JS rect probes (`laidOutID`).
  - **A built-in's menu category and kit role are data** (`builtinProperties`, keyed by roster constants; `TestBuiltinPropertiesCoverTheRoster`); a non-built-in is classified by its DISPLAY name (`categoryFromName(InstrumentDisplayName(id))`). The DESIGN.md `instrumentDefaults` colours are generated as `roster` constants.
  - **`syscall/js` panics on any named type** (`js.ValueOf(id)` / `obj.Set("k", role)` compile and crash in the browser only): `jsFn` passes results through `jsSafe` (named basics → plain, `MarshalText` structs → their text); a direct `Set`/`Call`/`Invoke`/`ValueOf` needs `string(id)` / `x.String()`. **`make js-audit`** (`scripts/jsaudit`, run by `make test` and CI) type-checks the js/wasm build and fails on any such value — it caught `obj.Set("logicKind", LogicKind)` on its first run. `case string` / `.(string)` on an `any` silently stops matching a typed id.
- **Instrument documents may carry TABLES** (`Stage.Table`, declared by a stage's `TableDecl`; the WAV-clone `modal` / `noiseband` stages): rows flatten to dense ABI arrays (`modal_freq[i]`) + a row count, are written ONLY through `instrument.SetTable`, and are never knobs — `modularFields` rows with `Rows`/`Derived` get no ParamDef (`IsTableField`), and the JS ABI lists them in `MODULAR_PARAM_TABLES`, not `MODULAR_PARAM_INDEX`. A table is model parameters only: ≤ 1,024 values per document, every column physically bounded (`TestTablesAreModelParametersOnly`). Any code that rebuilds a `Stage` must carry its `Table` (`withModule` once dropped it on every knob edit — `TestSetValueKeepsTheStageTable`). The table stages are TABLE-GATED in the Synth tab (`tableGatedStages`): with no rows they have no knobs at all (`InstrumentParamDefs` moves them to the hidden group — one ParamDef per ABI field stays the invariant), and `audio.ModalModes` is the Go mirror of the bank (`TestModalModesMatchTheEngine`). See `AGENTS.synth.md` "WAV-clone stages".
- **A note is a fixed time, the same at every tempo and on both platforms** (user decision 2026-10-27: "the same circuit sounds exactly the same at any tempo in both platforms. This is paramount."). A voice is its document's `render.seconds` long — there is no `render.beats`, and nothing between the document and the speaker reads the BPM: the voice cache key, the graph render key, the Synth dials' `settles`/caps and the analysis renders carry no tempo, and a BPM change re-renders nothing (no warm-up gate on a tempo change). The frame count is `audio.VoiceFrames` = audio.js's `Math.max(1, Math.floor(sr * seconds))`. The shipped lengths are what desktop played at 120 BPM before (beats × 0.5 s); `render.amplitude` was then re-measured over those lengths (`make measure-loudness` measured over `render.seconds`, the old WEB length, so 97 stored levels were off for desktop by up to 3 dB). A length or level change moves the sound hash, so it needs `make measure-loudness`, `make templates` (pins), `make gen-dial-relevance` and `make wav-match-calibrate` (hubness entries apply only while the hash matches — a stale key silently drops the correction; conga fell to #5 in `TestEssenceSelfIdentifiesUnderNuisance`). The browser post-processes a voice exactly like desktop (`voice_post.js` ↔ `renderDocBlock` + `normalizeAndScaleAmp`): the document's voice FX through the SAME C insert effects (`fx_host.c`, complete params from `VoiceFXCalls`, `voice_fx.go`; a document may carry only `fx.filter` / `fx.distortion` / `fx.bitcrusher`), the gate tail, peak-normalise × amplitude in float32, the end fade — in the render worker and the main-thread fallback alike. Guards: `TestVoiceIsTheSameAtEveryTempo` + `TestTempoChangeReRendersNothing` (`internal/audio`, real build), `TestTempoChangeNeverGatesPlayback`, `TestDocumentsCarryOnlyVoiceFXBothPlatformsRun`, `voice_tempo_independence.browser.test.js` (every shipped voice: identical at 60 / 240 BPM in the browser; the pushed document = desktop's; the browser render = desktop's played voice, same frame count, every sample within 1e-4 — 139 of 150 bit-identical, worst 5.6e-6 on 2026-10-27, the rest is libm/compiler rounding in the C). What a voice becomes in the mix — pitched/sized drum, gated melodic note, volume, fade-in, start sample, stop — is identical too (§ One audio pipeline).
- **One audio pipeline: every stage after the voice is the same computation on both platforms** (user 2026-10-27: "Let's make everything consistent!"; spec `docs/superpowers/specs/2026-10-27-one-audio-pipeline-design.md`). Both run at **48 kHz** (`audio.EngineSampleRate`; `new AudioContext({sampleRate: 48000})`, honoured by Chromium, Firefox and WebKit). The browser runs desktop's C for the insert effects (`fx_host.c` — the ONE table of each effect's init-argument order, buffer size and state; Go sends complete params), the EQ (`mix_dsp.c` running Go's `BuildEQProgram`: high/low-pass in series around the graphic section, a Linkwitz–Riley band split when a band is muted) and the master soft clip (identity to 0.9, `T + (1−T)·tanh((|x|−T)/(1−T))` above — continuous; the old `T·tanh(x/T)` jumped at the knee) inside the `beatmo-dsp` AudioWorklet (`dsp_worklet.js` + `dsp_host.js`; one `drums.single.js` instance per AudioContext; readiness by message — top-level await does not delay `addModule` in any engine). A note is desktop's played voice (`played_voice.js` mirrors `playedVoice`: drums resampled at k·step — WebAudio's resampler is never used, Firefox's is a sinc — melodic notes rendered at their EXACT pitch and gated by Duration, desktop's 5 ms fade-in baked in), played at rate 1 from a frame through ONE gain = headroom × its exact volume. Desktop fixes that came with it: a voice starts on its own sample (it waited for the next 64-frame block), `resampleVoice` no longer drops a sample per block (its end "probe" consumed one), stops are linear from the exact sample (`RequestStopAt`), a channel with an effect or EQ runs EVERY block (tails used to stop dead when its last voice ended), offline renders build effects at the engine rate (they built them at 44.1 kHz), the offline render plays each note through `playedVoice` (it ignored Duration and drum pitch), WAVs are decoded at their own rate and converted by ONE Go windowed-sinc `ResampleToRate` on both platforms (desktop installed SILENCE for a foreign rate; embedded 48 kHz PCM played 1.47 st low), the master compressor is deleted (dead on both after the first master EQ write), output capture + recording tap the master AFTER the soft clip. Insert slots carry instance uids (`InsertEffectInstance`): structural edits keep surviving instances' state on both platforms. Every value both sides use is in `chain_spec.go` → `chain_spec.gen.js` (`TestChainSpecGenFileIsCurrent`). **Guards:** `xplat_audio_stages.browser.test.js` (desktop `cmd/export_audio --stages` + `internal/ui` `TestWriteXplatCircuitReference` rendered fresh vs audio.js's real graph on an OfflineAudioContext via `window.__offlineRenderForTest`: played voices, Sampler edits, lane stops, all 18 effects at default/min/mid/max, every EQ shape, the master, and whole circuits imported through Go — every tap ≤ 1e-5, the only allowance one bitcrusher quantisation step on ≤ 0.1 % of samples; Chromium + Firefox + WebKit, `XPLAT_ENGINES`), `chain_spec_parity.browser.test.js` (live wiring, both DSP hosts), native `TestEveryEffectIsBlockSizeInvariant`, `TestEQIsBlockSizeInvariant`, `TestEQMatchesItsResponse`, `TestHPFAndMutedBandKeepEveryFilter`, `TestSoftClipIsContinuousAndBounded`, `TestMixerVoiceIsThePlayedVoice`, `TestLaneStopLandsOnItsSample`, `TestEffectTailsRingWithoutVoices`, `TestForeignRateSampleIsResampled`, `TestEngineRunsAt48k`. The kick fingerprint guards analyse at 44.1 kHz (its band windows are rate-sensitive though the sound is not) after proving the 48 kHz render inaudibly different (`kickGuardRender`). **The reference toolchain stays at the CORPUS rate, `wavref.Rate` (44.1 kHz), never the engine's**: every recording is converted to it and READ at it (`audio.DecodeWAVAtRate(path, wavref.Rate)`; `DecodeWAVToPCM` is the engine rate, for the Sampler), `refs match`/`tune`/`compare`/`diagnose`/`organic`/`-nuisance` render the synth at it, and the committed calibrations (thresholds, organic range, match hubness) were measured there. When those tools inherited the engine rate, the move to 48 kHz cost `TestEssenceSelfIdentifiesUnderNuisance` conga (#4) and kalimba (#83), and every recording came back resampled to 48 kHz (`refs check` hubness drifted on 30+ voices, `refs verify` would hash changed PCM). The `fingerprint` descriptors are calibrated at that rate too and several count samples (`DetectF0Detailed`'s YIN frames are a fixed 4096 samples, `noise.fraction` notches ±1 bin of a fixed FFT), so a test that judges the 48 kHz PLAYED voice with them converts it first (`audio.ResampleToRate(…, wavref.Rate)`: `TestEveryDialMovesItsMetric`, `TestAveMariaVoiceSoundsAtTheNotatedPitch` — at 48 kHz the soprano's Breath read backwards and an Ave Maria half note +32 cents for a note that is −5). Making the YIN frames a time was tried and reverted: it nudged kick-boom-bap's WRONG 53 Hz tail reading (its body is 45.5 Hz, under YIN's 50 Hz floor) past the 1-semitone floor guard, flipping its note name; kick-airy/brutal/wood already show a "measured" note ~6 st above the body their lowest partial sits at.
- **Analysis tools render through `audio.RenderAsShipped`, never the raw C render**: `RenderAsShipped(doc, sr, pitch)` is what a trigger plays (document FX, gate tail, peak-normalise × the DOCUMENT's amplitude, percussion pitched by resampling) and is pinned byte-identical to `RenderAsPlayed` for every shipped instrument at pitch 0 and +7 (`TestRenderAsShippedMatchesPlayback`). `refs compare`/`diagnose` use it via `synthmatch.RenderShipped`. `RenderInstrumentWithFields` stays the RAW C render on purpose for `cmd/export_audio` (the cross-platform reference compares raw C on both sides), `make measure-loudness` (it computes `render.amplitude`, so it must not see it applied), `gen-fundamentals` and the organic calibration.
- **C lazy tables must initialise through `beatmo_once` (`src/c/once.h`)**: the voice prewarm pool renders on several threads from the moment a circuit loads, and the wavetable generators `memset` then SUM harmonics into a shared table — a plain `if (!ready)` flag let two cold renders build it at once and froze a corrupted saw/square/glottal/FM table for the whole session (every one of 10 cold launches corrupted 5 of 8 voices). Any new lazily built C state uses the same primitive. Guard: `TestColdConcurrentRendersMatchSerial` (`internal/audio`, child processes start cold).
- **seqMu deadlock**: `Game.Update()` holds `seqMu` during `drum.Update()` — callbacks must NOT re-acquire it.
- **Input function variables**: `cursorPosition`/`isMouseButtonPressed` (`input.go`) are function variables that map a single touch onto the mouse; never call `ebiten.CursorPosition()` in UI code, and call `resetTouchOverride()` alongside `SetInputForTest`.
- **Touch override is frame-scoped**: `updateTouchOverride()` runs once per frame, after `globalTouchState.Update()` and before any cursor read; reordering `game_update.go` must keep that sequence.
- **Tap injection is 2-frame**: taps fire after the touch ends; `injectTouchTap` (`touch.go`) synthesizes the press, then the release on the next frame.
- **Mute gate**: the predictor gates only the mute subdivision; audio resumes immediately after.
- **Test stub EQ**: under the `test` tag, `NewEQProcessor` returns `gainProcessor`, not a real biquad.
- **Parity defaults**: WASM is log-only unless `PARITY_WASM_FATAL=1` (`game_parity_state.go`). **Tests are strict**: every `go test` Game starts in panic mode for both the logical checker (`testParityWatchDefault`, `testutil_test.go`) and the timing gate; a stopped transport is never scanned, under test or in production (`parityScanStoppedForTest` is the one explicit opt-in). Only checker unit tests that manufacture a mismatch on purpose opt down to log mode on their own Game. Browser tests arm WASM panics with `GOFLAGS=-tags=parityfatal` (`parity_fatal_tag.go`; every WASM build inherits `GOFLAGS`, e.g. `GOFLAGS=-tags=parityfatal make test-browser`).
- **3-phase mixer** (`engine_stop.go`): desktop renders voices into per-instrument buffers, then runs each channel's EQ on that sum, then the master; a biquad is stateful, so it must see one continuous summed stream, never individual voices.
- **Audio-panel tab switching MUST go through the dispatcher**: `EnsureAnalyzersForTab(tab, channel)` in `audio_panel_dispatcher.go` is the single chokepoint that enables the master analyzer (and any per-row analyzer) for the active tab. The pre-Phase-0 bug was that the analyzer was only enabled by EQ-channel selection, so opening Spectrum/Levels/Chain without touching the channel picker drew flat output. Every tab-change path now calls the dispatcher: `onTab` closure (user click), `SetActiveTab` (programmatic/scene catalog), and `NewEQPanelZone` (initial frame). Don't add a fourth path. Regression test: `audio_panel_dispatcher_test.go`.
- **Zone input isolation MUST be opaque to z**: every Zone that renders an opaque visual surface registers a full-bounds catch-all `HitArea` at its nominal z (the z the Zone is registered with in `DrumViewTree`). Per-control hit areas inside the same zone sit at `nominal-z + 1` / `+2` and win on overlap (`HitIndex.At` sorts exact-rect hits then z descending). Lower zones never see input inside the catch-all's rect. Use `NewInputCaptureHitArea(zone.Rect(), z, tag)` from `input_capture.go`. **Don't expand a zone's hit-area Rect geometrically past its visible bounds to meet touch-min** — use `HitArea.Touch + ClipRect` so the expansion is bounded and can't reach a sibling zone (Phase 3 of the input-isolation pass replaced the timeline scrub's downward `scrubRect.Max.Y += grow` with this pattern). **Captured drag handlers MUST re-check the cursor against their own bounds on each `OnDrag`** — the tree (`drumview_tree.go:357-368`) routes drag events to a captured handler without re-testing hit areas, so the handler self-polices and releases capture when the cursor leaves its lane (`timelineScrubHitAdapter.OnDrag` is the canonical example). Discipline test: `zone_input_isolation_discipline_test.go` — enumerates every opaque zone and asserts the catch-all is present. Pre-isolation, a tap in audio-panel chrome whitespace fell through to the timeline at z=110, making a drag on a synth knob also scrub the timeline.
- **`RegisterZoneVisible`'s predicate gates BOTH Draw AND HitAreas**: register a zone with a visibility predicate exactly once (`tree.RegisterZoneVisible(zone, z, func() bool { … })`); the tree consults it in two places — `zoneAsLayer.Visible()` for Draw and the `Update()` Layout phase for the HitIndex. When `visible()` flips from true → false, the tree publishes `nil` HitAreas for that zone so the dispatcher never reaches them. Pre-fix the gate covered Draw only, so a hidden audio panel's `eq-panel-capture` catch-all swallowed taps destined for the row rack beneath (the "Pads tab unresponsive after EQ→Pads transition" regression). Belt-and-suspenders: each opaque zone also exposes a self-defense `IsHiddenForInput` callback on its callbacks struct (see `EQCallbacks` / `ChainCallbacks`), wired by DrumView to the same predicate; the zone's `HitAreas()` returns `nil` when the predicate fires. Discipline tests: `TestZoneInvisibleEqualsNoInput` + `TestZoneInvisibleNotInHitIndex` in `zone_input_isolation_discipline_test.go`. Regression test: `pads_input_regression_test.go`.
- **A template is a melody layer + an optional GROOVE layer, and all notated timing is delay/rush on the WRITTEN step** (`cmd/gen-showcase/score_import.go`, design `docs/superpowers/specs/2026-10-02-groove-templates-design.md`). Rules that each broke something when ignored:
  - **One grid unit per step; exported `subdiv` = steps per beat (4 or 8).** The engine's groove window is ONE grid unit (`nodeNudgeSecondsAt`); at the old 4 units/step a 100% delay was a quarter of a sixteenth and swing was unwritable. On screen nothing moves. Guard `TestGrooveWindowIsOneStep`.
  - **A node's groove is its own** — the scheduler used to lend the nearest grooved node's offset to every following ungrooved node for two beats, so one swung offbeat dragged the next downbeat. Guard `TestGrooveAppliesOnlyToItsOwnNode`.
  - **Performed time = written step + offset** from swing (`<swing>`/"Swing" words, piecewise warp), tuplet remainder, grace (one step back per grace), fermata/tenuto/rit./accel., `<arpeggiate>` (compressed to fit), breath/caesura. Offsets are in BEATS, clamped to one step and COUNTED (`GrooveClamped` must be 0). Guards `timing_test.go`, `TestSwingDelaysExactlyTheSwungPositions`, `TestTrapRollPlaysAsTriplets`.
  - **Loops start on a DOWNBEAT and last the notated length exactly**; a leading pickup bar wraps to the end. The first selected bar must sound (`TestExcerptOpensWithSound`). A groove score loops to fill and must divide the melody's length; its swing swings the whole band.
  - **Percussion parts become one row per Beatmo instrument** via `gmKit` (GM key → instrument, `PartMap.Kit` overrides); same-instrument pieces share a row (tom heights = node pitch), a same-step clash spills to `<inst> ·2`. Drums are pitched by RESAMPLING in the engine (only melodic recipes re-render at pitch), so the sounding-pitch test mirrors that (`renderAsPlayed`). Timpani = `SoundingPitch` on `tom`. Every percussion part sounding in an excerpt must be mapped (`TestEveryPercussionPartInRangeIsMapped`).
  - **Groove templates = famous 70s+ break (rhythm pattern) + ORIGINAL Beatmo tune**, because 70s+ melodies are copyrighted and cannot ship. Breaks are Beatmo's CC0 encodings of drum-patterns.com step grids (`rhythm`, cited, fingerprinted by `TestGrooveScoresMatchTheirIdiom`); tunes are `beatmo-*.musicxml` (`original`, composer `Beatmo`). Templates are titled by the REAL song ("Artist — Song", `displayTitles`) — the beat is that song's, the tune is original; a genre pattern that is no one song's beat is titled by genre, never by an invented or borrowed song name. Picks lean to the critically canonised (RYM-style: Eddie Bo, The Meters, Purdie, Fela, Devo, Skinny Puppy, Telex, Mantronix, Adonis, Aphex Twin, SOPHIE…), each from a drum-patterns.com grid we parsed and verified bar by bar; a transcription that is not the famous groove (their Fool in the Rain is the samba break) is rejected, not shipped under the name. Rows: chord parts ≤ 2 voices, same-family drums share a row (toms, congas), cap 12 (`TestTemplatesAreReasonablySized`). Kit flavours are the `kit*` maps in `template_specs.go`. Batch tooling (grid parser, tune DSL) is scratch-side, not in the repo. The one famous melody is Korobeiniki/Tetris (PD folk song, transcribed from a PD Commons sheet, `TestKorobeinikiMatchesTheSheet`). Melody sources may also be MIDI (`internal/smf`, Mutopia: `.mid` + `.ly` companion, licence and `\partial` pickup corroborated from the `.ly`).
  - **Template JSON is compact, one object per line** — indented JSON cost 0.77 MiB raw WASM and broke `TestWasmAssetSizeBudget`. Every row's instrument is PINNED: `instrument_docs` holds `{"ref":"sha256:…"}` = the content hash of its base id's SHIPPED document (`pinInstrument`), so a template plays the same on every machine — EXCEPT where the user has Saved their own version of one of its instruments, which trumps the pin like any circuit's document (user decision 2026-10-14; only its document — the template's settings stay — `TestUserSaveTrumpsTemplateInstruments`). Import (`validateProjectDocs`) and `songrender.ParseTemplate` resolve refs with `audio.PackDoc` before anything resets; an unknown ref is a hard error; export never writes a ref. A shipped document that changes or disappears while a template must keep its sound goes to `internal/assets/instrument_pins/<hex>.json` (packed into the pack's docs, never shipped — `TestBuildPacksPinnedDocuments`). Guard: `TestEveryTemplateRefResolves`.
  - **`songrender` and the UI audio harness clock is `60/(BPM·subdiv)` per path step** (subdiv = units per BEAT) and they apply each node's groove; they read "units per bar" before and rendered at quarter tempo.
- **No audio file is ever committed or shipped** (`.wav`/`.mp3`/`.aif(f)`/`.flac`/`.ogg`/`.m4a`/…, any letter case). Reference recordings are local dev tooling: they live ONLY in `$BEATMO_WAV_REFS` (default `~/.beatmo/wav-refs`), the catalogue (`internal/wavref/catalogue.json`) records provenance (source URL/path, trim, measurements, PCM hash) and carries **no licence** — there is nothing to license because nothing ships; `wavref.Resolve` has no in-tree fallback. Test audio that must exist everywhere is GENERATED (`internal/wavgen`), and **no test ever reads reference audio** — not via `wavref` (unless the test points `$BEATMO_WAV_REFS` at `t.TempDir()`), not via a relative path, not from `assets/wav`: a test that does passes or skips by accident of one machine's disk (11 `-tags test` tests silently ran only on a developer machine; a browser stress test played whatever local WAV it found instead of the synths). The corpus CALIBRATIONS (`thresholds.json`, `organic_range.json`, `F0ReliableConfidence`, the shipped audition candidates) are re-derived by `make refs-check` (`refs check`, also an explicit CI step), never by `go test`. Guard: `TestNoTestReadsReferenceAudio` (`internal/deploy/tests_read_no_audio_test.go`). Four guards against committed audio: `.gitignore` (case-insensitive patterns + the local `/assets/` sample library), the pre-commit hook `scripts/precommit/no-audio-files.sh` (`make install-hooks`; catches `git add -f`), `TestNoAudioFilesInRepository` + friends (`internal/deploy/audio_files_guard_test.go`: tracked or staged audio fails `make test`), and the deploy script's staged-audio refusal. The score licence gates below are a different thing — templates DO ship, so their sources keep their licence rules.
- **Templates are DERIVED from scores, never hand-edited**: every built-in template in `internal/assets/templates/` is generated by `cmd/gen-showcase` from a committed public-domain score in `internal/scoreref/`. The spec table (`cmd/gen-showcase/template_specs.go`) names a score, a measure range, and a part→instrument mapping; the notes, durations, per-hit volumes (from `<dynamics>`) and groove drift (from grace notes, fermatas, tenuto and rit./accel.) are all computed from the score. Adding a note by hand fails `TestTemplateIsDerivedFromScore`, which re-reads the score and rejects any pitch not present in the trimmed range. A score is only admissible if it passes three gates enforced in `internal/scoreref` — the WORK is public domain (published ≤1929, composer died ≤1955), the ENCODING is `CC0-1.0 | PD | unrestricted | CC-BY-4.0`, and the provenance is verifiable (named corpus + upstream URL + SHA256 over the committed bytes). NonCommercial and ShareAlike encodings are excluded by policy, which is why the KernScores/Sapp editions cannot be used. `docs/score-sources.md` records the full source survey — read it before adding a corpus. `internal/scoreref` embeds ~5 MB and must never be imported by a shipped binary (`TestNoProductionImports`).
- **Reference coverage is measured against the ROSTER, not the catalogue**: `roster.All` (every shipped id) is the authority; `catalogue.json` mentions only 57 of them, and `refs doctor` collapses `snare-lofi` onto `snare` via `baseInstrumentID`, so 18 registered ids (`snare-crack`, `kick-lofi`, `fm-bell-soft`, …) were invisible to every reference tool. They are NOT duplicate rows — `snare-crack` has its own shipped document, so it is a distinct sound. `refs families` enumerates the roster and classifies each uncovered id: `via <base>` when `audio.ShippedSoundKey(id) == ShippedSoundKey(base)` (same flattened block, render settings and FX — same sound, the base's clip is valid ground truth), `NO REF` when its sound differs (a different sound — judging it against the base's clip measures the wrong thing), and nothing at all when the id or its base already carries a `synthetic`/`needs-source` status (a recorded decision, not an oversight). `refs adopt` validates against the roster for the same reason: adoption is the only way an uncatalogued id ever gets a reference.
- **Every instrument is ONE document — there are no defaults, overlays or recipes** (spec `docs/superpowers/specs/2026-10-04-instrument-config-documents-design.md`). An instrument is an `instrument.Doc` (`internal/instrument`: id, origin, name, category, notes, `render{seconds,amplitude,gate_tail,melodic}`, `stages`, `source`, `layers`, `fx`) holding EVERY value its voice depends on. Modules are present only when used; a module type is a declaration in `audio.BuiltinModuleRegistry()` (`module_registry.go`). The sound path is pure: `instrument.Flatten(doc, registry)` → the param block → `render_modular_p`; there is no identity fill, recipe layer or JS param map anywhere on it. Shipped documents live readable in `internal/assets/instruments/<id>.json` (+ `index.json` = the roster order) and ship as ONE embedded pack (`make asset-pack`). Three stores: the pack (shipped), the user library (`instruments.json` / localStorage `beatmo.instruments`, via `audio.SetInstrumentLibrarySink` + `LoadUserInstrumentLibrary` in both bootstraps), and the project store (each row's own COPY, `Origin` = the library entry it came from). **Save As is the only save** (Synth and Sampler tabs; spec `docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md` §2.7): prefilled with the instrument's current name, its confirm reads "Save over <name>" (the library instrument the copy comes from: a new VERSION of it, document + audio bundle, nothing overwritten), "Save as new" (a new content-addressed user instrument, `NewUserInstrumentID`; repoints the copy's origin and NAMES the rows, one undo step), or refuses another instrument's name in any locale; a save that cannot be stored notifies (`KeySaveFailed`) and changes nothing. There is no Save, Reset or Restore shipped. The user library is VERSIONED (`internal/objects`: content-addressed objects + per-identity history and current pointer, `audio.SaveInstrumentVersion` / `LibraryVersions` / `LibraryBundle`, one blob in the instrument store's `library` section, newest `LibraryVersionLimit` (20) kept; the shipped document is the implicit base). **The user's newest saved version trumps the document ANY circuit carries for the same instrument** (startup demo, imported file, template) every time a circuit is OPENED — its DOCUMENT only; the circuit keeps its own EQ / FX / Sampler settings (`audio.OpenCircuitInstrumentDocs`; identity = the document's `Origin`, else its id's instance base — `InstrumentLibraryIdentity`). A row's EQ, insert effects and Sampler edit are its INSTRUMENT's audio bundle (`audio.AudioBundle` over the stores; `EQSettingsOf`, no fields on `DrumRow`); choosing an instrument never drops them (`pickInstrument`: carry over / take the instrument's / ask). Undo/redo re-imports are NOT opens: `Import` checks `g.restoringUndo()` (its own `undoRestoring` flag — never the undo manager's `restoring`, which `buildDemo` raises for the STARTUP open too) and restores the snapshot verbatim. Guards: `synth_save_trumps_circuit_test.go` (real input), `save_as_only_test.go`, `pick_settings_test.go`, `circuit_open_versions_test.go`, `TestSynthSaveTrumpsTheStartupCircuitOnNextLaunch` (`!test`, real `buildDemo`), `synth_save_export_import.browser.test.js` §4b (reloaded localStorage profile); scenes `synth_saved_tone_reopened`, `synth_save_as_prompt` / `_new` / `_taken`, `pick_settings_question`, `kit_swap_settings_question`. The Synth tab still reads `[]ParamDef`, derived per instrument by `audio.InstrumentParamDefs(id)` and edited through `instrument.SetField` (`SetInstrumentParam` is the adapter); its one visibility rule is `FlatFieldDeclared`. The browser gets each instrument's flattened block through `window.setInstrumentBlock(id, block, seconds, amp, melodic)` — a page without Go has NO instruments. Adding an instrument: write its document (copy a neighbour), add its id to `index.json` and its category + kit role to `builtinProperties`, then `make asset-pack`, `go generate ./internal/audio/` (the `roster` constants), `make measure-loudness` (writes `render.amplitude`) and `make gen-fundamentals`. Guards: `TestShippedPackValidates`, `TestShippedLibraryMatchesTheSources`, `TestAssetPackIsCurrent`, `TestDeclaredModuleValuesAreLive` (no inert declared value), `TestOnlyTheModularParamBlockCrossesTheABI`, `TestSynthGoldenByteIdentity`, `internal/instrument` (`Flatten`/`SetField`/`Validate`/`Migrate`).
- **The param block is ONE generated table and the C engine has NO defaults** (Phase C, `docs/superpowers/plans/2026-10-03-config-only-synth-engine-phase-c.md`). `modularFields` (`internal/audio/modular_fields.go`: block, name, slots, init, doc) is the single source of the ABI: `cmd/gen-synth-abi -c -o src/c/modular_params.gen.h` emits the C struct (a dense array of doubles, `_Static_assert`ed) + the per-family `MODULAR_<BLOCK>_SLOTS` capacities, and `-o src/js/synth_param_abi.gen.js` the JS ABI (`make gen-synth-abi`; never redirect into the header — the shell truncates it before the cgo build that needs it). Go builds the block ONLY through `instrument.Flatten` (a field no present module writes is 0; `Init` is the value a NEW module starts with — never an absent-field fill). The single C entry point is `render_modular_p`; **NULL params render silence**, and there is no `kp_get`/`mp_get`/NaN or "0 means default" path — **0 is a real value**, so a document spells every value it relies on. A family voice beyond its slot capacity renders silence. The POST ops (`modular_finish`, `modular.c`) always run (neutral values = no-op; `out_clamp` 0 = no limit) and are a visible Synth-tab stage, **FINISH** (`audio.FinishGroup`: Resample, Length, Fade Speed, Low Boost, Crisp, Darken, Crunch, Limit; guards `TestFinishKnobExtremesStayFiniteAndBounded`, `TestFinishKnobsRespondProportionally`). **Each op is specified by its RESPONSE to known signals**, measured through the stage alone (`modular_finish_apply` → `applyModularFinish`, `finish_stage_response_test.go`) — a voice plus the peak normaliser hides what an op does per frequency, which is how Crisp shipped as a −6 dB broadband cut and Low Boost as a −6 dB cut of everything above 300 Hz. Low Boost = low shelf below 200 Hz, Crisp = high shelf above 3 kHz (0..+12 dB, linear in dB, the other end untouched); Resample = Kaiser-windowed sinc whose cutoff scales by 1/speed when shifting up (linear interpolation folded partials past Nyquist back at full level); a non-finite input sample is zeroed BEFORE the recursive filters (one NaN used to silence the rest of the voice). The linked engine must match the layout: C exports `modular_param_count()`, Go panics at init on a mismatch and `audio.js` refuses a stale `drums.single.js` (`TestLinkedEngineMatchesTheSchema`). A failed browser render throws and keeps the previous buffer — never a substitute voice. An unknown param name (or a family voice past its slot capacity) is rejected at every write boundary — `SetInstrumentParam(s)`, project import, bundle import (validated before anything registers); an invalid user-library document is skipped with a WARN. Guards: `TestGeneratedCHeaderIsCurrent`, `TestModularFieldsReproduceTheShippedLayout`, `TestTheEngineHasOneRenderEntryPoint`, `TestNoFallbackHelpersRemainInC`, `TestZeroIsARealValue`, `TestFinishStageShowsEveryPostKnob`, `TestImportRejectsUnknownDocumentValue`. Render parity across a refactor: `TestRenderParityDump`/`Compare` (`render_parity_tool_test.go`), which also renders every visible knob and document-set param at its extremes (`engine:<param>@min|max|zero`).
- **A gen-slot field every family reads is ONE registry row with `audio.AnyFamilyGroup`**: the generator waveform (`gen1_wave`, "Generator": Sine/Saw/Square/Triangle) is read by every C voice family, so `modularStageKnobs["wave"]` carries the sentinel group and the Synth tab shows it on whichever family the recipe plays (`synthFamilyParamHidden`), routed to VOICE and claimed by the Source card. The config-first conversion had left it hidden, so a config-first snare / FM bass lost the control the legacy `snare_wave` / `fm_wave` offered. Guard: `TestConfigFirstFamilyInstrumentsOfferTheGeneratorKnob`; scenes `crop_synth_snare_generator_voice` / `_source`.
- **Node note names are UNIVERSAL and come from a generated table, not from the melodic set**: every instrument except genuinely unpitched noise (hi-hats, claps, shaker, whisper, and the two cymbals — 12 of 133) names the note a node sounds. The authority is `instrumentFundamentals` in `internal/audio/instrument_fundamentals.gen.go`, produced by `cmd/gen-fundamentals`: it renders each voice through the real C DSP and measures its fundamental with `fingerprint.DetectF0Detailed`, preferring a declared base-frequency param wherever the two agree. Three rules keep it honest, and all three exist because breaking them shipped a wrong note name during development:
  - **A melodic recipe is never measured** (`SourceMelodic`). The curated melodic set is built so node pitch 0 sounds A3, and that convention IS its declaration. Measurement only perturbs it — harpsichord measures 440 Hz against its true 220 (YIN's octave artifact on struck strings) and kalimba lands 2.37 semitones out. So the table can only ADD note names, never change an existing one; `TestFundamentalsTableOnlyAddsNoteNames` pins that.
  - **Naming a note needs all THREE gates — use `fingerprint.UsableAsPitch(w, est)`, never `Reliable()` alone.** Each catches a failure the others cannot, proven independently by `TestUsableAsPitch_GatesAreIndependent`: (a) `Reliable()` — is it periodic; a hi-hat fails here. (b) `F0AtSearchFloor()` — YIN searches only `F0SearchMinHz`(50)–`F0SearchMaxHz`(2000), so a 44 Hz 808 kick pins at exactly 50.00 Hz *at confidence 1.000*; three shipped kicks sit in that blind spot. (c) `AnalyzeF0Support()` — does the spectrum actually contain the claimed frequency; a **ride cymbal passes (a) and (b) at confidence 0.998 and is still not a pitch**, because its lowest partial sits at 6.0× the f0 it reports (crash 9.7×, hi-hat 14.6×, clap 23.9×) — there is no energy whatsoever where it claims to sound.
  - **The support gate is lowest-partial-ratio, NOT `SeriesDeviation`.** Inharmonicity cannot do this job: struck metal is genuinely inharmonic, so glockenspiel (1.384) and glass-wash (1.561) crowd right up against ride (1.933) — a 1.24× band. Lowest-partial-ratio separates the same populations by 11.6 semitones (supported up to 3.08× at voice-opera, rejected from 6.00× at ride), asserted by `TestF0SupportBandIsCalibrated`.
  - **`AnalyzePartialRatios` has no absolute amplitude floor** — it filters peaks only *relative to each other*, so in a band holding nothing but noise-floor ripple it promotes that ripple to "partials". Asking support for 7000 Hz on a clean 220 Hz sine returned SUPPORTED before `f0SupportMinPartialRelAmp` was added. Any new caller measuring partials must impose its own energy floor.
  - **Measure support over the same window the f0 came from.** `wave.MagnitudeSpectrum` reads from the START of what it is handed, while `DetectF0Detailed` uses `sustainWindow` — so a naive implementation judges the broadband ATTACK of a struck instrument instead of the tone that follows. Guard: `TestAnalyzeF0Support_UsesTheSustainWindow`.
  - **The table stores a BASE at default params; the live delta is added at read time** (`InstrumentPitchInfo`). Reading the absolute value straight out would freeze the note against synth edits — `TestPitchOffsetTracksLiveParamEdits` catches that.

  Staleness is a wrong note name, so `TestInstrumentFundamentalsUpToDate` (in `cmd/gen-fundamentals`, run by `make test-real`) re-measures and fails when a retuned instrument drifts from the committed table. It **skips loudly** under `-tags test` rather than being build-tagged out, because the render path there is a stub and a tagged-out guard reports a silent pass.

  `pitched` widened when this landed, so a call site that used it to mean "is melodic" is now wrong: `cmd/gen-showcase`'s `instrumentPitchShift` gates on `audio.IsMelodicInstrument` (the document's `render.melodic`) instead, or every percussion part in every generated template would transpose down two octaves.
- **A node's Duration never changes a melodic voice's pitch**: melodic (pitch-aware, `render.melodic`) instruments are rendered AT pitch, so their playback rate is 1 (native `playedVoice`, `engine_play.go`) or the residual semitone ratio (browser dispatch, `audio.js`); a Duration below 1 GATES the note (`gateVoice`, 10 ms release; `src.stop` + gain ramp in `startRenderVoice`), and ≥ 1 plays the natural render. Both platforms used to play at `rate/dur` — right for drums, which are pitched AND sized by resampling — so every templated half note-length sounded an octave high (Ave Maria: dur 0.5 → +1200 cents, 0.25 → +2400). The generator writes Duration only for melodic rows (`noteDuration`); the browser's melodic set (`MELODIC_INSTRUMENTS` in `audio.js`) is filled by Go from each document's `render.melodic` through `setInstrumentBlock` — the old hand list held 31 of 79. Guards: `TestNodeDurationNeverChangesPitch`, `TestAveMariaVoiceSoundsAtTheNotatedPitch` (`cmd/gen-showcase`, `!test`), `melodic_duration_pitch.browser.test.js`.
- **Note names must never route through `hzToNote`**: it clamps to the piano range (MIDI 21–108), which is right for the spectrum cursor and wrong for a node. A kick spans MIDI 9–57 and the deep kick 4–52, so clamping renders a kick's whole bottom octave as "A0" while the note-matrix picker — which never clamped — offers A-1, G#-1, G-1 on the same cells: the picker promising a note the chip then denies. Both surfaces go through `midiNoteName(nodePitchMIDI(...))` in `node_note.go`. Guards: `TestPickerAndChipAgreeAcrossTheRoster` (walks every selectable cell of every instrument) and `TestDeepInstrumentsKeepDistinctNoteNames`.
- **WASM absolute GO path**: Pass absolute path when running browser tests:
  ```bash
  GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<test>.browser.test.js
  ```
  Relative `.tools/...` can fail because cwd becomes `src/go`.
- **The native audio reference is rendered FRESH, never committed**: `xplat_audio_compare` / `xplat_output_capture` get their desktop reference from `loadDesktopAudioReference()` (`browser_test_helpers.js`), which runs `cmd/export_audio` against the current tree. The committed `desktop_audio.json` snapshot was two months stale: the browser replayed old push blocks through today's C and 19 kick/snare cases rendered SILENCE — a stale file posing as a cross-platform bug. `cmd/export_audio`'s param cases are keyed by instrument id and each emits its COMPLETE flattened block (`audio.FlatFieldMap`), so the browser half rebuilds every block from the reference alone.
- **`window.audioReady` warms the ENGINE, never an instrument**: it resolves once the C module is loaded and every render-pool worker (plus the main-thread module) has done a couple of throwaway bare-voice renders — compile/tier-up only, nothing cached. Which voices to warm, with which params, is Go's knowledge — the circuit warm-up gate (`warmup_controller.go`) renders them after Go has pushed every instrument's block. The hardcoded 13-instrument list audio.js used to render ran BEFORE Go pushed any instrument (wrong voice for every config-first instrument, then evicted); dropping it outright left the workers cold and the first notes late (three-stage-latency 19 → 68 overdue), which is why the engine warm stayed. Play presses during warm-up are deliberately ignored: browser tests press Play via `startPlaybackWhenReady(page)` (`audio_probe_bench_helpers.js`) or, in-page, `pressPlayWhenReady()` (`scenarios/playback_ready.js`); a test that plays an arbitrary id calls `ensureSynthSample(id)` first (`window.playSound` ENQUEUES, and a cold voice defers).
- **Never await `window.audioReady` over CDP — use `waitForAudioReady(page)`** (`browser_test_helpers.js`): `page.evaluate(() => window.audioReady)` or an evaluate body that awaits it gets Chromium's `"Promise was collected"` when the engine-warm worker renders resolve it mid-await (raw CDP reproduced it 4/5 while an in-page async waiter on the same promise resumed every time — the product is unaffected), and Playwright rewrites that into **"Execution context was destroyed, most likely because of a navigation"** although nothing navigated. The helper watches settlement through an in-page `.then` flag polled by `waitForFunction`. Guard: `TestBrowserTestsNeverAwaitAudioReadyOverTheProtocol`.
- **`page.waitForFunction(fn, null, { timeout })` — options are the THIRD argument**: Playwright's signature is `(fn, arg, options)`, so `waitForFunction(fn, { timeout: 90000 })` silently ran on the 30 s default with rAF polling (74 call sites did; busy headless Firefox starved the rAF poll). Guard: `TestWaitForFunctionOptionsAreNotPassedAsTheArgument` (`js_waitforfunction_discipline_test.go`, trailing-comma form included).
- **The output-capture tap runs on the AUDIO thread**: `enableOutputCapture`/`startOutputCapture`/`getOutputCapture` are backed by a tiny AudioWorklet (inline Blob module, preloaded at AudioContext creation) on a PARALLEL branch off the limiter; the ScriptProcessorNode is only the fallback (`getOutputCaptureStats().audioThread`). A ScriptProcessor runs on the MAIN thread and, with the full app running, Chrome dropped most of its blocks — `xplat_output_capture` recorded fragments of voices that were playing correctly, which read as "inverted kick/snare". Worklet blocks are posted and queue instead of dropping; `await window.flushOutputCapture()` before stop/get when the exact window matters. The same test also used to serve a bare `audio.js` page with no Go, where every config-first instrument renders as the unseeded modular voice — it now loads the real app.
- **Cross-engine voice comparisons pin the sample rate**: headless WebKit runs its AudioContext at 44.1 kHz and Chromium at 48 kHz, and voices render at the device rate — pass `__testCaptureSynthRender(id, { sampleRate: 48000 })` (bypasses the playback cache) or two different rates get compared.
- **WebAudio deferred AudioBuffer**: Synth buffers pre-rendered at 48 kHz without AudioContext, lazily wrapped in `AudioBuffer` on first play via `ensureAudioBuffer()`. Avoids premature context creation on mobile (suspended state).
- **Browser test gotchas**: the exports are `startPlay()`/`stopPlay()` (not `start()`/`stop()`; optional chaining hides a typo); after `importJSON()` call `forceDraw()` and wait 300–500 ms before clicking; read canvas pixels with `canvasPixelAt()` (WebGL has no preserved buffer); stop playback between scenarios; unlock mobile audio with trusted CDP touches (`cdpTap()`).
- **Go E2E test patterns** (`testutil_test.go`): start with `assertDefaultParityState(t)`; after a circuit edit call `g.updateBeatInfos()` + `g.refreshDrumRow()`; stop with `stopPlaybackForTest(g)`; read triggers through the thread-safe `lastTriggered*ForTest` helpers; tick with `advanceFrames(g, N)`.
- **Production deploy ≠ serve-lan**: `make serve-lan` and every `*.browser.test.js` server (`real_input_test_helpers.js:246-302`, `visual_test_helpers.js:49-67`) read directly from `src/js/`, so every committed file is reachable and JS MIMEs are hardcoded to `application/javascript` / `application/wasm`. The real deploy (`scripts/deploy-wasm-gcs.sh`) only ships an explicit manifest into `build/wasm_site/` and previously relied on `gcloud storage rsync` to *infer* `Content-Type` — which falls back to `application/octet-stream` for `.wasm` on hosts whose Python `mimetypes` db pre-dates 3.12. Combined effect: a new `import './X.gen.js'` or `new URL('./Y_worklet.js', import.meta.url)` ships green on serve-lan + every browser test and 404s on beatmo.io; `main.wasm` can fail `WebAssembly.instantiateStreaming` on mobile Safari with a non-deterministic MIME. Two guards now keep the deploy honest:
  - `src/go/internal/deploy/manifest_drift_test.go` — parses `audio.js` + `index.html` for runtime refs (static `import`, dynamic `import()`, `new URL('./X', import.meta.url)`, `<script src>`, `fetch("X.wasm")`) and asserts every ref appears in the `ENTRY_FILES`/`ASSET_FILES`/`WASM_FILES` arrays in `deploy-wasm-gcs.sh`. Add a new runtime dep → update the array in the same change.
  - `scripts/verify_deploy_headers.sh` + `src/go/internal/deploy/headers_verify_test.go` — post-deploy curl-HEAD probe. Unit-tested against a fake bucket; live probe runs when `BEATMO_VERIFY_DEPLOY_URL=https://beatmo.io` is set.
- **Audio-panel pixel coverage**: `scene_crop_test.go` only checks SubjectRect *bounds* — a panel that draws chrome but renders nothing as data passes that test. Pixel content of Wave/Spectrum/Meters/Scope lives in `audio_panel_render_pixels_test.go` (Go fast path, color-counts data-trace rects via `drawRect` interceptor + `testAnalyzerStateOverride`/`testScopeStateOverride` hooks in `wasm_analyzer_bridge_stub.go`) and `audio_panel_render.browser.test.js` (real WASM↔WebAudio bridge, desktop + mobile viewports, color-histogram dominance check). When adding a new audio-data tab, extend both. Crop scenes that need the mobile audio panel exposed must declare `MobileSetup` (see `mobileAudioPanelPlaySetup`); Setup-only scenes silently fall back to invisible panels under `runSceneMobile`.

### Three axes: ScreenClass, Density, Runtime

The UI is parameterised on three **orthogonal** axes. Pick the right one when adding a divergent value — don't conflate them:

1. **`LayoutProfile.Class`** (`ScreenMobile` / `ScreenDesktop`, `layout_profile.go`) — *layout arrangement* only. Answers "where do things go?": panel orientation, hidden controls, one-column-stack vs side-by-side, bottom-sheet vs floating popup. Selected by viewport width via `detectSmallScreen()`. Use `Profile().IsMobile()` to branch on this axis — but **only for layout decisions, not sizing**.

2. **`Density`** (`DensityCompact` / `DensityComfortable` / `DensitySpacious`, `density_profile.go`) — *control sizing* only. Answers "how big are interactive controls?": button heights, knob diameters, touch targets, EQ handle radii, mini-meter widths. Read via `Profile().Density()` and `Profile().DensityValues().XxxField`. Defaults from screen-class (mobile→Spacious, desktop→Comfortable) but can be overridden independently — a tablet keeps desktop layout while picking touch-min-compliant sizing; a desktop power user picks Compact for data-dense screens. Tests override via `SetDensityForTest(d) (restore func())` mirroring `SetRuntimeProfileForTest`.

3. **`RuntimeProfile`** (`browserRuntimeProfile()` / `desktopRuntimeProfile()`, `runtime_profile.go`) — *browser-vs-desktop runtime knobs*. GC tick rate, predictor window cap, audio scheduler lookahead, etc. Selected by `//go:build js` vs `//go:build !js`. Do **not** add new `runtime.GOOS == "js"` checks in `internal/ui/` — extend `RuntimeProfile` instead.

**Where each field lives** (DESIGN.md YAML front matter):
- `profileOverrides.desktop` / `profileOverrides.mobile` — the layout-tied fields that genuinely depend on screen class (`minCellWidth`, `controlLeftInset`, `popupCornerRadius`, `defaultTimelineBeats`, and the menu width classes `menuWActionMin/Max`, `menuWList`, `menuWEditor`, `menuWWideMax` — read through `menu_sizing.go`, guarded by the `TestMenusUseWidthClass` ratchet).
- `densities.compact` / `densities.comfortable` / `densities.spacious` — the 34 density-tied fields (every control size, button height, touch target). Plus per-tab tokens: `synthKnobIdeal`, `chainMiniMeterW`, `levelsReadoutWFull`, `waveCursorStroke`, etc.

**Adding a new sizing knob**: declare it in `densities:` (kebab-case, all three densities). Run `make gen-design-tokens`. Read from `Profile().DensityValues().XxxField` at the call site. Don't add to `profileOverrides:` unless the value genuinely diverges because the panel is narrower / portrait — not because fingers are bigger than mice.

### Runtime Profile (browser/desktop divergence)

`RuntimeProfile` (`internal/ui/runtime_profile.go`) is the single source of truth for every value that differs between browser/WASM and native desktop. Sibling to `LayoutProfile` (which owns mobile↔desktop screen-class divergence). Do **not** add new `runtime.GOOS == "js"` or `runtime.GOARCH == "wasm"` checks in `internal/ui/` — extend `RuntimeProfile` and read from `RuntimeProf()` instead.

**Two builders, one chooser:**
- `browserRuntimeProfile()` and `desktopRuntimeProfile()` (no build tags) hold the values.
- `runtime_profile_js.go` (`//go:build js`) selects the browser builder; `runtime_profile_notjs.go` (`//go:build !js`) selects the desktop builder. The build-tag pair has no test-vs-non-test split, so it compiles cleanly under every `GOOS=js -tags=test` combination.

**Test override:**
```go
restore := SetRuntimeProfileForTest(browserRuntimeProfile())
defer restore()
```
or directly mutate `RuntimeProf().Field` for individual flips.

**Bench-time override:**
- Browser: set `window.__beatmoProfileOverride = { drawMinIntervalMS: 0, ... }` before WASM init. The Playwright harness in `webaudio_bench_startup.browser.test.js` reads `BEATMO_PROFILE_OVERRIDE` (JSON string) and injects it via `page.addInitScript`.
- Desktop: set `BEATMO_*` env vars (e.g. `BEATMO_AUDIO_LOOKAHEAD_SEC`, `BEATMO_DISABLE_NODE_GLOW`).

**Each surviving divergent field must carry a one-line bench citation** in `runtime_profile.go` justifying why the browser and desktop values differ. Knobs without a bench number should be unified across both profiles. See `bench-results/runtime_profile_sweep.md` for the methodology.

---

## Import/Export Format

**Download filenames are stamped, never constant** — `stampedFileName(base, ext)`
(`internal/ui/download_name.go`) produces `<base>-2006-01-02_150405<ext>` for
every generated download, and `window.stampedDownloadName` mirrors it for the JS
fallbacks. `a.download` is only a *suggestion*: when the name already exists in
the Downloads folder, the browser's own collision resolver renames the file and
each engine spells the uniquifier differently — Chrome `beatmo-export (1).json`,
Firefox `beatmo-export(1).json`, Safari `beatmo-export 2.json`. The page cannot
pin that format, so the only way to land an identical name everywhere is to
never collide. (On desktop the same constant name silently *overwrote* the
previous export when zenity was unavailable.) Guards:
`TestExportDownloadNameNeverCollides`, `TestInstrumentExportNameNeverCollides`.

**Schema versioning (all JSON formats)**: every Beatmo JSON document carries a
`"schema_version"` string from the single constant `schema.Version` in
`internal/schema` — pinned at **`0.0.1` until first release**. Embed
`schema.Envelope` anonymously in a document struct (encoding/json flattens it to
a top-level key). Readers apply ONE strict rule: the current version →
accept; anything else, an ABSENT version included → **reject** (there is no
pre-versioning migration — old projects are not read). Recovery differs by
format class — documents the user opens explicitly (project, instrument bundle,
`-ui-state`) surface a hard error, while stores loaded at boot (prefs,
instruments) log a WARN and start empty. The project check runs BEFORE any
reset, so a rejected file leaves the current circuit untouched, and it doubles
as the foreign-JSON sniffer (import is replace-not-merge, so an unrecognized doc
must be rejected, never imported as an empty project). Import also rejects an
instrument row that predates instrument documents (`recipe` / `synth_params`),
a document whose id does not match its `instrument_docs` key, and an unknown
module type or value — all before anything resets. See
`docs/superpowers/specs/2026-07-27-schema-versioning-design.md`.

### Export → import audio identity (byte-for-byte)

A project file is a COMPLETE description of everything that can be heard: load a
circuit, edit it, render N seconds of the MASTER bus, export, wipe every global
audio store, import, render again — the two renders are byte-identical. Guards
(all `//go:build !test`, so `make test-real` is their only runner — the `-tags
test` synth and EQ are stubs and prove nothing about the shipped sound):

| Test | Covers |
|---|---|
| `TestExportImportMasterMixIsByteIdentical` | one circuit exercising every subsystem at once + a named-field config diff |
| `TestExportImportMasterMixIsByteIdenticalAcrossReExport` | repeated save/load is a fixed point, and the re-export is byte-identical |
| `TestEveryBuiltinTemplateRoundTripsByteIdentical` | every shipped template (83), loaded → edited → exported → re-imported |
| `TestEveryBuiltinInstrumentMasterMixRoundTrips` | every roster id (sparse knob edits in → the exported document out must sound the same) |
| `TestEveryDSPStageRendersIdenticallyOnReplay` | per-stage determinism matrix |
| `TestExportImportMasterMixIsByteIdentical/{same-session,clean-machine}` | the headline circuit now also presses Synth-tab **Save** + **Save As** and makes post-Save knob + Sampler edits; imported into the same session AND a clean machine (Saves undone, user recipes unregistered). Same-session RE-SAVES the post-Save knob edits first: a user's Save trumps the file's document on open, so an unsaved edit comes back as the Saved tone there by design; the re-export fixed-point test runs on a clean machine for the same reason |
| `TestSynthTabSaveDoesNotChangeTheMasterMix` / `TestHeadlineEditsAreAudible` | Save/Save-As move zero samples; every post-Save edit moves the mix (no vacuous identity) |

Harness: `export_import_master_audio_harness_test.go` — `liveArrangement` reads
the LIVE engine predictor (not the file, so a note-schedule regression can't hide
behind a file that round-trips its own bytes) and `audio.RenderMasterMixOffline`
renders it through the real offline mixer WITHOUT re-applying any config, so the
render is a readout of live engine state.

- **`audio.ResetDSPState()` at every discontinuity.** Every stage between voice
  and speaker carries state — EQ/crossover biquads (2 samples of history),
  insert effects (delay lines, **LFO phase**) and the master compressor
  envelope. Correct during playback, wrong across a discontinuity: before
  this existed, the same circuit rendered differently depending on what ran
  before it (tremolo moved 100% of samples, the composite peaked at 27% of full
  scale). Called from `Import` and from every offline render pass. It MUTATES in
  place, so it is only safe at a quiescent point — never mid-stream.
- **There is no pan, no send bus and no per-song room (deleted 2026-10-27).**
  Pan was stored but never heard on either platform; the delay ("echo") and
  reverb ("room") send buses and the file's `send_effects` block had no
  control, and the two platforms tapped and voiced them differently. A row that
  wants a room or an echo puts a Reverb / Delay in its FX panel, where it is
  visible. The built-in templates were regenerated without them and the startup
  demo stripped; a file that still carries `pan` / `reverb_send` / `delay_send` /
  `send_effects` imports with those keys ignored. Don't add a hidden mix setting
  back: every stored setting needs a control (or goes).
- **`projectFile.InstrumentDocs` carries every row's instrument DOCUMENT**
  (`instrument_docs`, keyed by instrument id) — the whole sound, never a delta
  against something the reader might not have. A Save-As instrument therefore
  travels with the file and sounds the same on a clean machine; import installs
  the documents into the project store and never writes the user library
  (`TestImportNeverOverwritesTheLibrary`) — except that an instrument the user
  has SAVED plays the saved document instead (the file's is kept as Reset's
  target; see "Every instrument is ONE document"). Guards: `TestExportCarriesEditedDocument`,
  `TestImportReplacesDocuments`, `TestImportRejectsLegacySynthParams`.
- **User instrument ids are content-addressed** (`audio.NewUserInstrumentID`):
  the hash covers the name and the document, never `time.Now()`, so the same
  project is always the same file (`TestNewUserInstrumentIDIsContentAddressed`).
- **Per-instrument EQ is replace-not-merge at the AUDIO layer.** A flat EQ
  exports no `eq` block, and import used to apply EQ only when one was present —
  so an instrument flat in the new project kept the previous project's curve
  while the rebuilt row read flat in the UI. Import now calls `clearRowEQ`
  (NOT `applyRowEQ` with zeros: a unity biquad chain differs from an absent one
  in the last bits, and `clearRowEQ` re-wires the analyzers that live in the same
  processor list).
- **The browser renders exactly the document's block.** Every document change
  (edit, Save, Reset, import, undo) pushes its flattened block through
  `setInstrumentBlock`; there is no browser-side defaults layer or param map to
  drift from Go (that drift once made an import that rebound a row play a
  hybrid of two recipes). `ReplaceInstrumentDocs` pushes only documents that
  changed, so an undo/redo re-import re-renders nothing it does not have to
  (`TestReplaceInstrumentDocsTouchesOnlyWhatChanged`, `undo_reimport_perf_test.go`).
  Guard: `synth_save_export_import.browser.test.js` (full-PCM voice hashes across
  same page / reloaded profile / clean profile).
- **Sample edits fold into the voice-cache key BEFORE the first lookup** on the
  pitched path too (`tryRecipeVoicePitchedOrigin`). `ApplySampleEditLive`
  never invalidates the cache, so an unfolded first lookup served the cached
  UNEDITED voice and every melodic Sampler edit was inaudible — and the
  identity tests passed because both sides ignored it. Guards:
  `TestPitchedVoiceHonoursASampleEditAfterItWasCachedUnedited`,
  `TestHeadlineEditsAreAudible` (an identity test is only as good as the proof
  that its edits are audible).

**Project JSON** (`beatmo.json`) — one `projectFile` struct shared by export and
import (`internal/ui/project_file.go`); they were previously two field-identical
structs that silently dropped fields when they drifted:
```json
{
  "schema_version": "0.0.1",
  "subdiv": 8,
  "bpm": 120,
  "instruments": [{"name": "Kick", "id": "kick", "kind": "builtin", "volume": 1.0, "origin": 0, "color": "#C87850FF"}],
  "nodes": [{"id": 0, "i": 0, "j": 0, "type": "regular", "inputs": [], "outputs": [], "volume": 1.0, "pitch": 0, "duration": 1.0, "logic_kind": "", "logic_n": 0, "logic_p": 0}],
  "eq": {"gains_db": [0,0,0,0,0,0,0,0,0,0], "bands_hz": [[20,100],[100,200]]}
}
```

Every synth row's sound is its entry in the top-level `instrument_docs` map (the full `instrument.Doc`, keyed by instrument id; only embedded templates use the reference form `{"ref":"sha256:…"}` — `projectDoc` in `project_file.go`). Instruments carry an optional `sample_edit` (`omitempty`, see `export.go` `exportInstrument`) — the non-destructive Sampler edit (trim/pitch/gain/reverse/normalize/fade as a flat `map[string]float64`, bools 0/1; `fade` is the Fade toggle, a fixed `audio.SampleFadeMs` = 5 ms in and out) applied to the fresh synth render at trigger time. `sample_edit` is a post-render transform, NOT baked PCM: the synth stays the source of truth. Import is replace-not-merge — an instrument without `sample_edit` clears any in-session descriptor.

### Portable single-instrument format (`*.beatmo-inst.json`)

Separate from the project file, a **config-only, shareable instrument bundle** exports/imports 1+ instruments (Synth/Sampler `⋯` menu → Export/Import). Format `beatmo.instrument` (the magic string identifying WHICH format — orthogonal to `schema_version`, which identifies which revision). Forward/backward compatible via additive `omitempty` + unknown-field tolerance. Types live in `internal/audio/instrument_bundle.go`; the pipeline is `BuildInstrumentBundle` (`internal/ui/instrument_export.go`) → `ApplyInstrumentBundle` (`instrument_import.go`), IO in `instrument_io.go`.

- **Normalized effects** — a file-level `effects[]` library (deduped `EffectDef`s); instruments carry only an ordered `effect_chain` of ids (repeats ok). `NormalizeEffects`/`DenormalizeEffects` (`instrument_effects_norm.go`) convert to/from a row's `[]EffectSlot`.
- **WAV by reference, never PCM** — a `wav_ref` pins `{name, sha256, version}` + `sources[]` (`file` / `localstorage` / `url`, tried in order, hash-verified). `Sha256Hex`/`HashSamplePCM` (`content_hash.go`, no build tags — wasm-safe), version registry `WavRegistry` (`wav_registry.go`), resolution `ResolveWavRef` (`wav_resolve.go`) → Matched/Mismatch/Missing.
- **Synth** is the instrument's whole document (`BundleInstrument.Synth *instrument.Doc`), migrated + validated before anything registers.
- **Import safety** — never clobbers a builtin (re-namespaces to a fresh `user.*` id); a colliding user instrument resolves via Replace / Keep-both.
- **Decoupled store** — the user instrument library (documents) + wav registry live in their OWN store (`instruments.json` / `beatmo.instruments`), NOT the prefs blob. `userprefs.NewInstrumentStore` (`instrument_store*.go`); both bootstraps point `audio.SetInstrumentLibrarySink` + `LoadUserInstrumentLibrary` at it (`cmd/beatmo.go`, `js_bootstrap_wasm.go`). A recipe-era file's `user_recipes` / `recipe_overrides` sections load with ONE warning and are dropped on the next write (no migration).

---

## Environment Variables

### Runtime Configuration

| Variable | Purpose |
|----------|---------|
| `BEATMO_ASSETS=<path>` | Override default assets root directory |
| `BEATMO_CONFIG=<path>` | Load custom JSON circuit as initial demo |
| `BEATMO_DEMO_CONFIG=<path>` | Load custom JSON circuit (takes precedence over `BEATMO_CONFIG`) |
| `BEATMO_WAV_REFS=<path>` | THE reference-audio root (default `~/.beatmo/wav-refs`). The ONLY place reference audio lives — nothing is in the repo. Holds CONVERTED, catalogue-hashed audio plus `sources/` (raw drop-ins for `refs adopt`, invisible to `Resolve`), `candidates/`, `compare/`, `probes/` |
| `BEATMO_ROW_SNAPSHOTS=1` | Enable row snapshot mode |
| `BEATMO_AUDIO_SAMPLE_RATE=<int>` | Override the desktop engine rate (default: 48000 = `audio.EngineSampleRate`, the browser's rate; diagnostics only — the cross-platform contract assumes 48 kHz) |
| `SEQ_TICK_MS=<int>` | Override sequencer tick duration (default: 4ms on WASM) |
| `PERF_FAST_PATH=1` | Enable fast perf path (auto-enabled on WASM) |
| `PERF_BROWSER_UPDATE_MAX_MS=<int>` | Set max Update() duration for browser perf optimization |
| `BPM_TIMING_TEST=1` | Enable BPM timing test mode |

### Profiling

| Variable | Purpose |
|----------|---------|
| `PPROF=1` | Start pprof server on `localhost:6060` |
| `PERF_LOG=1` | Emit perf logs every ~2s |
| `PYROSCOPE_URL=<url>` | Pyroscope profiling backend URL |
| `PYROSCOPE_APP=<name>` | Pyroscope app identifier (default: `beatmo`) |

### Audio Debugging

| Variable | Purpose |
|----------|---------|
| `TEST_TONE=1` | Output pure 440Hz sine wave, bypassing all synth (tests Oto/driver) |
| `TEST_VOICE=1` | Replace C synth with simple 220Hz sine voices (tests voice/mixer path) |
| `TEST_RAW_VOICE=1` | Output first voice's raw samples, bypassing all processing |
| `TEST_RAW_VOICE_FULL=1` | With `TEST_RAW_VOICE`, use full amplitude (1.0×) instead of 0.2× |
| `TEST_RAW_VOICE_NOSCALE=1` | With `TEST_RAW_VOICE`, skip all multiplication (direct to int16) |
| `SINGLE_VOICE=1` | Limit mixer to one voice at a time (tests multi-voice accumulation) |
| `AUDIO_CLIP_DEBUG=1` | Log clipping events (samples exceeding ±1.0 before hard clamp) |
| `DEBUG_MIXER=1` | Log mixer workBuf min/max values and voice stats periodically |
| `BYPASS_CHANNEL_PROC=1` | Skip all channel processing (volume, EQ) — isolates mixer vs channel |
| `BYPASS_HEADROOM=1` | Skip per-voice headroom attenuation (0.25×) |
| `BYPASS_EQ=1` | Skip all EQ/processor processing |
| `BYPASS_NORMALIZE=1` | Skip peak normalization during export |
| `DEBUG_CHANNEL=1` | Log channel processing values |

### UI Rendering & Geometry

| Variable | Purpose |
|----------|---------|
| `RENDER_SAFE=1` | Force drawRect-based rendering instead of sprite cache (for pixel testing) |
| `SCREEN_EDGES=1` | Draw screen-space edge arrows at viewport edges |
| `NO_GRID_DRAW=1` | Skip grid pane drawing entirely |
| `NO_GRID_TILE_CACHE=1` | Disable grid tile caching (force redraw every frame) |
| `NO_PIXEL_SNAP=1` | Disable pixel snapping for geometry diagnostics |
| `NO_EDGE_CACHE=1` | Disable edge cache (force redraw every frame) |
| `NO_SPRITE_NODES=1` | Disable node sprite rendering |
| `DEBUG_GEOM=1` | Verbose geometry logs |
| `DEBUG_DRAW_NODES=1` | Enable detailed node draw logs |
| `BEATMO_DEBUG_INST=1` | Debug logging for instrument menu operations |

### Timeline & Parity

| Variable | Purpose |
|----------|---------|
| `TIMELINE_TRACE=1` | Trace timeline masking logic |
| `TIMELINE_TRACE_ROW=<int>` | Specify which row to trace (default: 0) |
| `DEBUG_HISTORY_SEED=1` | Debug timeline history seeding logic |
| `PARITY_FATAL=0\|false` | Disable parity panics |
| `PARITY_WATCH=log\|panic` | Parity mode (desktop) |
| `PARITY_WASM_FATAL=1\|true\|panic` | Enable parity panics on WASM |
| `PARITY_DUMP_STDERR=1` | Dump parity error messages to stderr |
| `PARITY_SCAN_STRIDE=<int>` | Parity scan stride (cells per scan pass) |

### Test Infrastructure (Go)

| Variable | Purpose |
|----------|---------|
| `BEATMO_TEST_LOG=1` | Enable verbose logging during tests (default silent) |
| `BEATMO_TEST_LOG_LEVEL=<level>` | Test log level: `TRACE`, `DEBUG`, `INFO`, `ERROR` |
| `FONTCACHE_SUBPROCESS=1` | Internal: font cache test subprocess marker |

### Test Infrastructure (Browser / JS)

| Variable | Purpose |
|----------|---------|
| `GO=<path>` | Path to Go binary for WASM builds |
| `BROWSER_JOBS=<int>` | Number of parallel browser test jobs (default: 4) |
| `BROWSER_TEST_PORT_BASE=<int>` | Base port for browser test servers (default: random 8500-9499) |
| `WASM_PREBUILT=1` | Skip WASM rebuild if binary exists |
| `TEST_LOG=1` | Enable page console logging in browser tests |
| `COVERAGE=1` | Enable Go coverage flushing during browser tests |
| `JS_COVERAGE=1` | Enable JS source coverage collection via Playwright |
| `FILTER=<pattern>` | Test filter pattern for `make test-browser-filter` |

### Browser Performance Thresholds

| Variable | Purpose |
|----------|---------|
| `AUDIO_PROBE=0` | Disable the playback probe in probe-enabled benches |
| `AUDIO_PROBE_STRICT=1` | Enforce lateness limits (probe lag/IOI/late notes + the honest scheduler `overdue`/`minLead`/lag gates); default reports them |
| `AUDIO_PROBE_DUMP=<dir>` | Write each bench's raw probe log as JSON (re-analyse offline with `audio_probe_analysis.js`) |
| `ISOLATION_SECS` / `ISOLATION_BPM` / `ISOLATION_SCENARIOS` / `WASM_FILE` | Isolation bench: seconds per scenario, tempo, scenario filter, custom wasm served as `main.wasm` |
| `PERF_E2E_FPS_MIN=<float>` | Min FPS for e2e perf test |
| `PERF_E2E_DRAW_MAX_MS=<float>` | Max avg draw time for e2e perf test |
| `PERF_E2E_UPDATE_MAX_MS=<float>` | Max avg update time for e2e perf test |
| `PERF_E2E_AUDIO_CALL_MAX_MS=<float>` | Max avg audio call duration for e2e perf test |
| `PERF_E2E_AUDIO_QLAT_MAX_MS=<float>` | Max audio queue latency for e2e perf test |
| `PERF_E2E_OVERDUE_MAX=<int>` | Max overdue audio events |
| `PERF_E2E_SMALL_LEAD_MAX=<int>` | Max audio events with <4ms lead time |
| `PERF_BROWSER_FPS_MIN=<float>` | Min FPS for browser perf test |
| `PERF_BROWSER_UPDATE_MAX_MS=<float>` | Max avg update time for perf test |
| `PERF_BROWSER_UPDATE_JITTER_MS=<float>` | Update timing jitter tolerance |
| `PAN_STRESS_FPS_MIN=<float>` | Min FPS for pan stress test |
| `PAN_STRESS_DRAW_MAX_MS=<float>` | Max avg draw time for pan stress test |
| `STRESS_COMPLEX_FPS_MIN=<float>` | Min FPS for complex stress test |
| `STRESS_COMPLEX_DRAW_MAX_MS=<float>` | Max avg draw time for complex stress test |
| `STRESS_COMPLEX_INTERVAL_MS=<int>` | Sample interval for stress test |
| `STRESS_COMPLEX_SAMPLES=<int>` | Number of stress test samples |
| `DRUM_EDIT_TIMEOUT_MS=<int>` | Timeout for drum edit changes (default: 350ms) |
| `DRUM_EDIT_CYCLE_MS=<int>` | Cycle wait for drum edits (default: 220ms) |

### Benchmarking

| Variable | Purpose |
|----------|---------|
| `BPM=<int>` | BPM for `make bench` |
| `SECS=<int>` | Benchmark duration in seconds |
| `BPM_LEVELS=<space-separated>` | BPM levels for `bench-desktop.sh` |
| `DURATION=<int>` | Duration per BPM level (default: 15s) |
| `RESULTS_DIR=<path>` | Results output directory (default: `bench-results`) |
| `BENCH_CIRCUIT=<name>` | Circuit for perf_e2e benchmark (`startup` or synthetic) |
| `BENCH_BPM=<int>` | BPM for perf_e2e benchmark (default: 200) |
| `BENCH_DURATION=<int>` | Duration for perf_e2e in ms |

### LLM Recording & Agent

| Variable | Purpose |
|----------|---------|
| `LLM_RECORD=1` | Enable browser recording for LLM tests |
| `LLM_RECORD_NAME=<string>` | Recording session name |
| `LLM_RECORD_INTERVAL=<int>` | Frame capture interval in ms (default: 500) |
| `ANTHROPIC_API_KEY=<key>` | Claude API key for agent/evaluate |
| `MODEL=<model-id>` | Claude model for agent (default: `claude-haiku-4-5-20251001`) |

### Deployment (GCP)

| Variable | Purpose |
|----------|---------|
| `GCP_PROJECT=<id>` | GCP project ID |
| `GCS_BUCKET=<name>` | GCS bucket name |
| `GCS_LOCATION=<region>` | GCS bucket region (default: `us-central1`) |
| `DOMAIN=<domain>` | Domain for static site (default: `beatmo.io`) |
| `DRY_RUN=1` | Preview deployment without changes |

### External Services

| Variable | Purpose |
|----------|---------|
| `BROWSERSTACK_USERNAME` | BrowserStack username for visual/device testing |
| `BROWSERSTACK_ACCESS_KEY` | BrowserStack access key |

---

## Design Tokens

`DESIGN.md` is the design-system document; its YAML front matter is normative. The Go runtime mirrors those values in `src/go/internal/ui/theme.go`, `theme_tokens.go`, and `touch_sizes.go`.

Two test guards keep the two sides aligned. Both run in the fast `-tags test` path and gate every PR that touches `internal/ui/`:

| Guard | What it checks |
|---|---|
| `design_md_drift_test.go` (`TestDesignMDDrift`) | Parses DESIGN.md YAML and asserts every named color (26), spacing (13), rounded (4), and alpha (5) token equals its Go constant. RGB-only comparison for tokens whose Go form carries a non-255 alpha. |
| `token_discipline_test.go` (`TestTokenDiscipline`) | Per-file budget for inline `color.RGBA{}` / `color.NRGBA{}` literals in `internal/ui/`. Files above budget fail; **files below budget also fail** — the budget must tighten when literals are removed (ratchet). New literals in non-budgeted files fail. Infrastructure files (`theme.go`, `theme_tokens.go`, `drawing.go`, `icons.go`) are exempt. |
| `design_md_lint_test.go` (`TestDesignMDLintSnapshot`) | Runs `npx @google/design.md@0.1.1 lint DESIGN.md` and pins the warning set to `testdata/design_md_lint.expected.json`. New or removed warnings fail. Skips when `npx` is unavailable. |

### Token-change checklist

- **Adding a color / spacing / rounded / alpha token:** update DESIGN.md YAML, the corresponding Go constant in `theme.go` / `touch_sizes.go`, and the want-map in `design_md_drift_test.go`. All three.
- **Replacing an inline literal with a token:** decrement the per-file count in `token_discipline_test.go`'s `allowedLiteralBudget`. Run the test — it fails if you forget.
- **Adding a new file with literals:** must be added to the budget map *with review* — prefer using existing `Token*()` accessors from `theme_tokens.go` instead.
- **Alpha buckets:** use `WithAlpha(token, AlphaFaint|AlphaSubtle|AlphaMedium|AlphaStrong|AlphaOverlay)` from `theme_tokens.go`. Don't introduce new opacity values; pick the closest bucket and bump the bucket only if you really need a new one.

### Semantic invariants (memorize)

- **Three reds, three roles.** `error` = stop/error **text only**; `mute` = mute button **fill only**; `destructive` = destructive button **fill only**. Never reuse a red variant for a different role; do not introduce a fourth red.
- **`primary` and `on-surface-accent` share a hex** (`#00C8FF`) but have distinct roles (interactive surface vs. text). Use the role-correct accessor.
- **Surface hierarchy nests strictly:** `background` → `surface-1` → `surface-2` → `surface-3`. Never place a lower-level surface inside a higher-level container.
- **Corner radius is chosen by ROLE, never by size.** `RadiusControl` (4) for **every** interactive control — buttons, chips, tabs, keycaps, inputs, synth controls — regardless of how big it is; `RadiusContainer` (8) for popups/menus/cards/panels; `RadiusSheet` (20) for the mobile bottom sheet; `RadiusNone` for anything that tiles. The raw `RadiusXXS…RadiusXL` steps exist only as the values those roles resolve to — reaching for one at a call site is how controls drifted into pill shapes. There is no `RadiusFull`.
- **The anti-pill invariant:** `radius <= min(w, h) / 4`. `drawRoundedRect` clamps to it, so a pill cannot reach the screen; `TestNoPillShapedControls` intercepts the func var to observe the *pre-clamp* request and fails a call site that over-asks anyway. A shared chrome helper serving many rect sizes wraps its role token in `fitRadius(role, r)`. A genuine circle or capsule (slider rail/thumb, drag handle, count badge) calls `drawCapsule` — the only sanctioned way past the ceiling, and a sentinel rather than a pixel value so it is always deliberate.
- **Cluster alignment:** controls in one horizontal run share `Min.Y`, `Max.Y` and height. Express hierarchy within a cluster with **tone**, never by shrinking a box — a shrunk sibling leaves container bands above and below it that read as a stray line. Guard: `TestTransportClusterSharesBounds`. Inset helpers must clamp each axis independently (`clampInsetAxis`); a shared budget lets a 1 px width difference change the vertical inset.

### Forbidden glyphs

`▶`, `▼`, `▲`, `✕`, `≡`, `⏸`, `■`, `↑`, `↓`, `→`, `←` in `internal/ui/` source. Use `IconID` enums + `DrawIcon(dst, IconID, r, col)`. Single-character text labels (M/S/FX/O/X) are permitted; raw chrome glyphs are not. See DESIGN.md "Permitted text-glyph exceptions" for the complete table.

### Validation commands

```bash
# Drift + discipline + lint snapshot (fast)
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/

# Direct lint (requires npx + network)
npx -y @google/design.md@0.1.1 lint DESIGN.md
```

---

## Maintenance Tips

### Tooling
- Use `.tools/go/bin/go` for all Go tasks
- Use `.tools/go/bin/gofmt` if system `gofmt` is unavailable
- Keep `src/js/main.wasm` out of commits — use `make wasm` locally

### After Modifying Predictor/DrumView/Caching
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline ./internal/ui

# Browser bridge smoke (canonical export catalogue)
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
# Cross-platform predictor/audio parity
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
make test-parity
```

### After Modifying Audio Mixer/EQ/Channels
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/... -v -run "Biquad|Channel|ProcessBlock"
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... -v -run "Mixer"
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_stages.browser.test.js
```

### For Perf Regressions
- Compare `webaudio_perf.browser.test.js` vs `webaudio_perf_e2e.browser.test.js` outputs
- Check `perfStats()` after `resetPerfStats()`
- Inspect `dumpRowState` / `dumpTimelineSegments` via browser console

### Concurrency Debugging
- Run with `-log DEBUG` or `-log TRACE` for lock acquisition logs
- Use `PPROF=1` then: `go tool pprof http://localhost:6060/debug/pprof/goroutine`
- Check for `seqMu` deadlocks: ensure no callbacks from DrumView acquire it
