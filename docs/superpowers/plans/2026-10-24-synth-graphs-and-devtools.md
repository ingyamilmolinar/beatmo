# Synth Graphs + Developer Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hide the Chain tab and the low-level synth controls in the shipped build, and replace the Synth tab's model-drawn graphs with explanatory graphs computed from the audio the instrument actually plays.

**Architecture:** One build-time switch (`release` tag → `devToolsBuild`, a `var` so the code stays in the binary) behind one predicate `devToolsOn()`. The graphs come from a new pure analysis package `internal/soundview`, fed by `audio.RenderDocForGraph` (the voice-cache render on desktop, the playback render worker in the browser). The UI's graph pipeline renders slots (NOW / LO / HI / BEFORE) on the existing `synth.preview` pool. One drawer per view kind paints axes, ghosts, difference shading and a measured readout.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten v2, cgo C DSP (desktop), Go-WASM + audio.js WebAudio workers (browser).

**Spec:** `docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md`. Read §9 (amendments) first: the user's 2026-10-24 constraints override §4.8 and §4.1's dead-code elimination.

## Global Constraints

- **Do not remove existing code or functionality** (user, 2026-10-24). Old graph renderers, the mirror and the thumbnails stay in the tree; developer builds can still reach the legacy graphs through the view picker. Fixing bugs is allowed.
- **Chain tab code stays in the production binary; it is only hidden from view.** `devToolsBuild` is a `var`, never a `const`.
- **Never `git add` / commit / stash** (user preference, memory). Leave the tree for review.
- Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/...`. Real build: `CGO_ENABLED=1 ../../.tools/go/bin/go test ./internal/<pkg>/`.
- Visual verification is mandatory for every UI task (CLAUDE.md): build `/tmp`-scratch screenshot binary, run with `XDG_CONFIG_HOME=<scratch>`, open the PNG.
- Graphs never draw a model of the voice. The only sources are `audio.RenderDocForGraph` and `soundview`.
- The graph render must never delay playback renders (browser: wait while `synthWorkerPending.size > 0`).
- Colours come from tokens. Data traces use the Visualizers family (`colConceptStroke`/`colConceptFill`, `genColorVizScopeBg`). No new `color.RGBA` literals (token discipline ratchet).
- Every new user-visible string goes through `internal/i18n` (en + es).

## Review Focus

1. **A saved or imported Chain view in production** (prefs, ui-state, the context menu, the number keys, the JS export): a reasonable person expects to land on the current/default tab, never on a blank or hidden panel. Covered by Task 2's refusal tests.
2. **An instrument with a Sampler edit:** the graph must show the edited sound (trimmed/reversed), the same as what plays. Covered by Task 7's identity test.
3. **A dial turned while the render is still pending** (browser, slow worker): the old picture stays, no blank frame, no flicker to empty. Covered by Task 8's stale-while-revalidate test.
4. **Switching instrument mid-focus:** focus must not carry a dial id onto an instrument that lacks it. Covered by Task 4's focus-reset test.
5. **Muted or soloed-out rows firing:** no glow (nothing is heard). Covered by Task 9.

---

## Slice 1 — gate + production surface

### Task 1: The developer-tools switch

**Files:**
- Create: `src/go/internal/ui/devtools.go`, `devtools_build_dev.go` (`//go:build !release`), `devtools_build_release.go` (`//go:build release`), `devtools_test.go`
- Modify: `Makefile` (`WASM_RELEASE_TAGS = noscenes release`), `src/go/cmd/beatmo.go` (flag), the wasm bootstrap (`?devtools=0`), `src/go/internal/deploy/scene_catalog_shipping_test.go` (or a sibling) for the tag guard

**Interfaces — produces:**
- `var devToolsBuild bool` (true in dev, false under `release`)
- `func devToolsOn() bool` — `devToolsBuild && !devToolsForcedOff.Load()`
- `func SetDevToolsForcedOff(off bool)` (exported: cmd + wasm bootstrap)
- test helper `forceProductionUIForTest(t testing.TB)` in `devtools_test.go` (restores on cleanup)

- [ ] Test `TestDevToolsPredicate`: dev build → `devToolsOn()` true; after `SetDevToolsForcedOff(true)` false; restore.
- [ ] Deploy test `TestShippedWasmIsAReleaseBuild`: `shippedWasmTags` contains `release`.
- [ ] Implement; `-devtools` bool flag (default true) calls `ui.SetDevToolsForcedOff(!v)` before `ui.New`; wasm bootstrap reads `URLSearchParams(location.search).get("devtools") === "0"`.
- [ ] Run both tests green.

### Task 2: Hide the Chain tab in production

**Files:** `panel_tab.go`, `eq_panel_zone.go` (`SetActiveTab`), `drumview_ctor.go` (`bottomNavModes`), `drumview_context_menu.go` (`setViewMode`), `js_exports_eq_widgets.go` (nav slugs from `bottomNavModes()` + `viewModeSlug`), test `devtools_chain_hidden_test.go`

**Interfaces — produces:**
- `func panelTabAvailable(t PanelTab) bool` (false for `TabScope` when `!devToolsOn()`)
- `func viewModeAvailable(m viewMode) bool` (false for `viewModeChain` likewise)
- `AllPanelTabs()` and `bottomNavModes()` return only available entries. The production nav list is a second package-level slice, so no per-frame allocation.

- [ ] Test `TestProductionHidesTheChainTab` (production forced, Game built AFTER forcing): no sticky-bar pill labelled Chain. Keys 1..7 never select `TabScope`. `SetActiveEQTab("chain")`, `SetChainVisible(true)`, `dv.eqPanelZone.tabState.SetActiveTab(TabScope)`, `setViewMode(viewModeChain)` leave the active tab unchanged. The mobile nav labels exclude "Chn".
- [ ] Twin `TestDevBuildShowsTheChainTab` (non-vacuous).
- [ ] Implement: refuse at `PanelTabState.SetActiveTab` (the chokepoint every path reaches), `EQPanelZone.SetActiveTab` (before analyzers) and `setViewMode`.
- [ ] Run the ui fast suite filters `-run 'Chain|ViewMode|Tab|Sticky|BottomNav'` green.

### Task 3: Hide the synth low-level surfaces in production; compact cards

**Files:** `synth_macro_render.go` (`layoutMacroRoot`, `drawMacroCard`, `macroCardHitAdapter`), `synth_dial_render.go` (`macroCardTopH` compact variant), `synth_macro_nav.go` (`setSynthDrillTarget`), `synth_panel_zone.go` (`SelectSynthSectionByLabel`), test `devtools_synth_hidden_test.go`

**Interfaces — produces:**
- `func synthCompactCards() bool` (= `!devToolsOn()`)
- `func productionMacroCards(cards []macroGroup) []macroGroup` (keeps cards with `len(Dials) > 0`)

- [ ] Test `TestProductionSynthHasNoLowLevelSurfaces` (real `Game.Update`, production): the pipeline strip has no pills and no "+ Add". A card tap does NOT drill (`synthAtMacroRoot` stays true). `SelectSynthSectionByLabel` returns false. No card is dial-less.
- [ ] Test `TestEveryInstrumentKeepsProductionDials`: every roster id has ≥ 4 live dials on ≥ 2 cards.
- [ ] Test `TestProductionCardsFitTwoRows` (1280×720, desktop profile): the laid-out card grid needs ≤ 2 rows for every roster id; a reasoned exception map starts empty.
- [ ] Implement: strip height 0 in production, chevron + gloss + signature graph omitted on compact cards, drill refused, dial-less cards filtered.
- [ ] Screenshot `synth_prod_root` (new scene; Setup forces production) desktop + mobile; inspect.

### Task 4: Focus model

**Files:** create `synth_focus.go`; modify `synth_dial_input.go` (press → focus), `synth_macro_render.go` (production card tap → focus the first dial; focused-card border), tests `synth_focus_model_test.go`

**Interfaces — produces:**
- `type synthFocus struct { inst instrument.ID; card, dial string; kIdx int }` (dial focus: card+dial; raw param focus: kIdx ≥ 0)
- `func (dv *DrumView) synthFocusFor(inst instrument.ID) (synthFocus, bool)`
- `func (dv *DrumView) setSynthFocusDial(inst instrument.ID, card, dial string)` / `clearSynthFocus()`

- [ ] Test: pressing a dial focuses it (both modes). A production card tap focuses its first dial. A dial id the new instrument lacks clears focus on instrument switch (Review Focus 4).
- [ ] Implement; the focused card draws the accent border (`drawFlatSelectionBar` already supports `Selected`).

### Task 5: Remove the play-through line; first glow

**Files:** `synth_signal_graph.go` (`drawSynthFocusGraph` stops calling `drawSynthPlayhead`; the functions stay), `synth_focus_graph.go`, `synth_signal_graph_test.go` (playhead assertions → glow assertions)

- [ ] Test `TestTriggerGlowsInsteadOfSweeping`: after `onInstrumentTriggered`, no playhead pixel column is drawn, and the glow alpha is > 0 then decays to 0.
- [ ] Implement (glow now applies to every graph, time-axis or not). Task 9 replaces the fixed decay with the envelope.

## Slice 2 — real audio

### Task 6: `internal/soundview`

**Files:** create `src/go/internal/soundview/{analysis.go,envelope.go,pitch.go,spectrum.go,noise.go,wave.go,soundview_test.go}`; modify `src/go/internal/wave/fft.go` (export `FFT(buf []complex128)`).

**Interfaces — produces:**
```go
func New(pcm []float32, sr int, voiceSec float64) *Analysis
func (a *Analysis) Envelope() Envelope            // HopSec 0.005; RMSdB, PeakdB []float64 (floor -72)
func (a *Analysis) PeakDB() float64
func (a *Analysis) AudibleSec() float64           // last time RMS ≥ peak-40 dB
func (a *Analysis) AttackSec() float64            // onset → first frame within 1 dB of peak
func (a *Analysis) Onsets() []float64             // seconds
func (a *Analysis) Pitch() PitchTrack             // HopSec 0.01; Hz []float64 (0 = unvoiced)
func (a *Analysis) Spectrum() Spectrum            // log axis 20..20k: Hz[], DB[] (rel max, floor -60)
func (a *Analysis) CentroidHz() float64
func (a *Analysis) Harmonics() (Harmonics, bool)  // linear 0..12·f0: Hz[], DB[]; F0
func (a *Analysis) Spectrogram() Spectrogram      // Cols×Rows dB, HopSec, FreqLo 30, FreqHi 16000, 64 rows
func (a *Analysis) Noise() NoiseSplit             // HopSec; Tonal, Noise []float64 (linear); Fraction
func (a *Analysis) Wave(cycles int) WaveWindow    // Samples []float32, T0 sec, PeriodSec (0 = fixed 20 ms)
```
- [ ] Known-answer tests (`-tags test` fast path, synthetic signals): exponential decay audible time ±5 %; linear 20 ms ramp attack ±2 ms; 150→44 Hz exponential glide within 10 cents after 80 ms; 440 Hz sine spectrum peak bin and centroid ±3 %; harmonics of a saw (1/n levels within 1 dB); sine+white-noise mixes at known SNR → fraction ±0.1; chirp maximum row tracks frequency; the wave window starts at a rising zero crossing, period ±1 %. Silence and empty input return zero values, never a panic.
- [ ] Implement: YIN via FFT autocorrelation on a 4× decimated signal (range 25–3000 Hz); the STFT shares frames between Spectrum and Spectrogram; everything is lazy and memoised.

### Task 7: `audio.RenderDocForGraph`

**Files:** create `src/go/internal/audio/graph_render.go` (key), `graph_render_native.go` (`!test && !js`), `graph_render_wasm.go` (`js && wasm && !test`), `graph_render_teststub.go` (`test`), `graph_render_native_test.go`; modify `src/js/audio.js` (`renderDocPCM` + factored post-process), `src/js/wasm_bridge_smoke.browser.test.js`

**Interfaces — produces:**
```go
func RenderDocForGraph(id instrument.ID, d instrument.Doc, bpm int, pitch float64) (pcm []float32, sr int, err error) // blocking; call off the UI goroutine
func GraphRenderKey(id instrument.ID, d instrument.Doc, bpm int, pitch float64) uint64
func GraphVoiceSec(d instrument.Doc, bpm int) float64 // what this platform sizes the voice to
```
- [ ] Real-build test `TestGraphRenderIsThePlayedVoice`: for kick-808 and violin, with and without a Sampler edit (trim + reverse + gain), `RenderDocForGraph(id, liveDoc, bpm, 0)` is bit-identical to the voice-cache buffer playback uses (`docVoiceBuffer`).
- [ ] Test (fast): the key changes with a param, BPM or edit, and is stable otherwise.
- [ ] Implement. Native = `renderDocVoice` + `ApplySampleEditToBuffer`. Browser = `renderDocPCM(id, block, seconds, amp)` through `renderVoiceInWorker` plus the same post-process, after waiting for `synthWorkerPending.size === 0`. The test stub is a deterministic toy voice from the document's amp env, pitch and noise values.

### Task 8: UI graph pipeline

**Files:** create `src/go/internal/ui/synth_graph_pipeline.go`, `synth_graph_pipeline_test.go`

**Interfaces — produces:**
```go
type graphSlot int // slotNow, slotLo, slotHi, slotBefore
type graphRender struct{ key uint64; sr int; voiceSec float64; an *soundview.Analysis }
func (dv *DrumView) synthGraphPipe() *synthGraphPipeline
func (p *synthGraphPipeline) want(slot graphSlot, id instrument.ID, d instrument.Doc, bpm int)
func (p *synthGraphPipeline) get(slot graphSlot) (*graphRender, bool) // last landed (stale ok)
func (p *synthGraphPipeline) pending(slot graphSlot) bool
func (p *synthGraphPipeline) renderNowSync(...) // scenes
```
- [ ] Tests (fake render func): a param change re-requests NOW; ten requests during a drag run at most two renders (latest wins); a pending render keeps returning the previous result (Review Focus 3); a failed render keeps the last; the LRU caps at 16.
- [ ] Implement on the `synth.preview` pool (reuse the mirror's pool name and fallback recipe). Analysis precomputes Envelope + the views the focus needs; on js it yields between stages.

### Task 9: Overview + envelope glow

**Files:** create `synth_view_overview.go`, `synth_graph_glow.go`; modify `synth_panel_zone.go` (root right pane), `synth_signal_graph.go` (`onInstrumentTriggered` audibility check)

- [ ] Test `TestGlowFollowsTheNoteEnvelope` (fake clock + synthetic NOW): intensity ≥ the resting value always; full for ≥ 100 ms; zero once the envelope is < −40 dB or after 1.5 s. A muted row → no glow; another instrument → no glow; Preview → glow.
- [ ] Overview: peak+RMS clip columns at true scale, a time axis with beat ticks, a note-end marker, a colour strip, and a readout ("Sounds for … · brightness … · … noise").
- [ ] Screenshot `crop_synth_overview` + glow scene; inspect.

## Slice 3 — explanatory views

### Task 10: View drawers

**Files:** create `synth_views.go` (kinds, titles, registry), `synth_view_draw.go` (axes, ghosts, shading, readout), `synth_views_test.go`

**Interfaces — produces:** `type synthView int` with `viewOverview, viewLoudness, viewAttack, viewPitch, viewColour, viewHarmonics, viewColourTime, viewNoise, viewWave, viewLegacy`; `func drawSynthView(dst *ebiten.Image, r image.Rectangle, v synthView, in synthViewInput)`; `type synthViewInput struct{ now, lo, hi, before *graphRender; loLabel, hiLabel string; caption string }`.

- [ ] Tests: each drawer paints NOW pixels for a synthetic render (pixel interceptor). Ghosts appear only when their slot is present. The axes stay fixed when NOW changes within a focus. Difference shading is present only when BEFORE differs.
- [ ] Scenes `crop_synth_view_<view>` for each; inspect desktop + mobile.

### Task 11: Dial views as data

**Files:** `internal/assets/synth_dials.json` (+ `view`, `lo`, `hi` per dial), `internal/synthdial` (fields + `Load` validation), `internal/i18n` catalogs (80 end words en + es), `synth_dials_discipline_test.go`

- [ ] Test: every dial names a known view (`lfo-target` allowed) and two existing i18n keys.
- [ ] Implement: a focused dial → its view with LO = `Turn(start, 0)`, HI = `Turn(start, 100)` documents.

### Task 12: Raw param views (developer builds) + view picker with Legacy

**Files:** create `synth_param_views.go` (`synthParamViews`), modify `synth_focus_graph.go` (the drilled graph uses the new views; `viewLegacy` calls the old `synthFocusRendererForControl`), test `synth_param_views_test.go`

- [ ] Test `TestEveryVisibleParamHasAnExplicitView` across the roster.
- [ ] The view-picker chip (dev only) cycles through views, Legacy included.

### Task 13: Honesty guards (real build)

**Files:** create `synth_graph_honesty_test.go` (`!test`), `testdata/synth_graph_known_weak.json`

- [ ] `TestEveryControlMovesItsGraph`: every roster instrument × every live dial; the view data at LO vs HI differs visibly, or the pair is listed with a reason (`SYNTH_GRAPH_WEAK_UPDATE=1` writes the list; shrink-only).
- [ ] `TestSoundviewAgreesWithFingerprint`: attack, audible, centroid and noise fraction against `fingerprint` within tolerances, on every roster instrument.

### Task 14: Docs + visual sweep

- [ ] CLAUDE.md: a short "Synth graphs come from rendered audio" section, the developer-tools gate, and the new tests.
- [ ] Sweep the new scenes desktop + `MOBILE=1`; fix until they look right.

## Slice 4 — landing (deferred to the user)

The tour edit plus `make showcase-video` need the user's real display (CLAUDE.md § Landing page). Leave the tour unchanged and report it.
