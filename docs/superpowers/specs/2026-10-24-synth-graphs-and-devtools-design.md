# Synth-tab graphs that show the real sound + developer-only controls

Date: 2026-10-24 · Status: proposed (design only, nothing built)

## 1. Intent

The Synth tab has two problems the user named:

1. **Too much for regular users.** The low-level synth controls (the pipeline
   of engine stages, the raw knobs) and the Chain tab are tools for developing
   and diagnosing instruments and circuits. They stay available to the
   developer and disappear from the build that ships on beatmo.io.
2. **The graphs confuse instead of explain.** "All graphs pretty much confused
   me." The play-through line is inaccurate and is removed. Every graph must
   now (a) be **accurate**, meaning computed from the exact audio the
   instrument plays, and (b) **explain** what the control under the user's
   finger does to that audio, for every high-level dial and every low-level
   knob. When a note fires, the graph shines and then settles back. It never
   disappears.

What success looks like: a user turns *Drop* on Kick 808 and sees the
kick's pitch fall further, with the start and end pitch written in words. A
user turns *Bright* and sees the tone-colour curve tilt up and a "brightness
centre" readout climb. A developer opens any raw knob and sees the same kind
of honest picture, and can trust that **a graph that does not move means the
sound did not change**.

## 2. What is wrong today (evidence)

Captured with the screenshot harness (scenes `eq_tab_synth`,
`crop_synth_macro_*`, `crop_synth_focus_*`, `crop_synth_graph_playthrough_mid`,
`crop_synth_dials_*`, `crop_synth_modal_graph`, `crop_synth_sub_bass_*`, mobile
pass), plus the code and one engine measurement.

**The graphs are not the sound.**
- Every Synth-tab picture (`synth_concept_viz.go`, `synth_focus_graph.go`,
  `synth_full_note.go`, `synth_stage_thumbs.go`, the root card thumbnails, the
  "Your sound" pane) is drawn from `audio.RenderInstrumentPreview` /
  `RenderInstrumentPreviewWave` (`internal/audio/preview_render.go`). That is a
  pure-Go approximation of the voice, not the engine, and it reads no
  drum-family (`gen1_*`) value.
- Measured on Kick 808 (real build), sweeping each knob from min to max:

  | Knob | Real voice change (RMS) | Graph change |
  |---|---|---|
  | `gen1_kick_pe_amt` | 1.640 | 0.000 |
  | `gen1_kick_h2` | 1.625 | 0.000 |
  | `gen1_kick_env1` | 0.442 | 0.000 |
  | `gen1_kick_attack` | 0.151 | 0.000 |

  So turning Snap, Weight, Drop or Speed on a kick moves nothing on screen.
- About 20 groups (`bass kick snare tom cymbal family formant resonator unison
  filtenv core generic voice`) route to `conceptFocusLevelWave`. It draws the
  same three cycles, scaled taller by the knob's position within its range.
  The picture claims every one of those knobs is a volume knob.

**The time and level axes are invented.**
- The envelope picture uses a fixed 0.25 s sustain hold
  (`adsrSamplesCurved`). It ignores the note's real length
  (`render.beats × 60/BPM` on desktop, `render.seconds` in the browser) and the
  engine rule that the release starts at most halfway through the voice.
- "Your sound" sizes its window from the envelope (`synthFullNoteWindowMs` =
  1.2×(a+d+r)), so lengthening a note rescales the axis and the picture barely
  changes.
- Every wave is gain-normalised (`mirrorTraceGain`), so level never shows.
- There are no units. The filter graph says only "dull/bright". Attack, Decay,
  Sustain and Release draw the identical ADSR with nothing marked. The FM graph
  has an unexplained dark "depth bar" box. The modal stems have no axis. The
  full-note pane renders as stripes, because only the top and bottom pixel of
  each column is stroked.

**The play-through line is wrong three ways.** It sweeps the invented ADSR
timeline. On pitch graphs it crosses a 24 ms window and then parks at the
right edge for the rest of the note (`playFrac`). It starts on the UI's
grid-time highlight, not when the note is heard.

**High-level controls.**
- The low-level pipeline strip (VOICE · OSC · LFO · ENVELOPE · FILTER ·
  RESONATOR · POST · FINISH · + Add) is the first row of the tab.
- Tapping a category card drills straight into raw knobs: Brightness opens
  Cutoff / Resonance / Type / Filtenv Amt / Filtenv Decay / Crisp. The macro
  dials exist only on the card faces, at about 16 px, as a bare 0–100 number.
- At 1280×720 only one row of cards is visible: 4 of a kick's 7, with Length,
  Source and Grit behind a scrollbar.
- Every card's one-line explanation is truncated ("The basic character of …").
- The dial *engine* is good and stays: readings are a pure function of the
  document, `Turn` is a proportional shift, and the measurability contract
  holds (`internal/synthdial`, `synth_dials.go`,
  `docs/superpowers/specs/2026-10-13-absolute-synth-dials-design.md`).

**Adjacent findings (not fixed here, surfaced by honest graphs).**
- Kick 808's Saturation (`gen1_kick_sat` 0→4) changed the real voice by 0.000:
  probably an inert knob for that variant.
- The browser's instrument push (`setInstrumentBlock`) carries neither the
  document's `fx` nor `render.gate_tail`, both of which the native path applies
  (`renderDocVoice` in `doc_voice_native.go`). 19 shipped instruments have
  document FX and 10 have a gate tail, so these probably sound different in the
  browser. Together with the known `render.seconds` vs `beats` sizing
  divergence, the new graphs will show the difference instead of hiding it.
  Verify and fix separately.

## 3. Goals, non-goals, anti-goals

**Goals**
- G1. Ship a production UI with no Chain tab and no low-level synth controls.
  Developer builds keep everything.
- G2. Every Synth-tab graph is computed from audio rendered by the same code
  path that plays the note on that platform.
- G3. Every control, all 40 dials and every visible raw param, has an
  explanatory view. Four things carry the explanation: the right picture for
  what the control changes, its range shown as ghosts, a "before" ghost while
  editing, and one measured readout in plain words.
- G4. No play-through line. On trigger the graph shines for as long as the
  note actually sounds, then settles to its resting look. It is never hidden.
- G5. A guard test fails when a control's graph does not move while the
  control changes the sound. Honest graphs are a diagnostic tool for the
  developer.

**Non-goals**
- Fixing inert knobs or the cross-platform voice divergences the graphs reveal.
  They get recorded in a ratchet file, not fixed.
- Changing the dial model, dial calibration, or any audio path.
- Redesigning the Chain tab (it is only hidden), the Sampler tab, or other
  audio-panel tabs.
- A runtime "unlock developer tools" switch on beatmo.io. The production build
  simply does not have them (see §4.1; easy to add later).

**Anti-goals (a failure even if it "works")**
- A graph drawn from any model, approximation, or schematic of the voice
  instead of rendered audio. That includes "analytic" filter or ADSR curves.
  Approximations are what caused this.
- A graph that moves when the sound does not, or that rescales its axes while
  the user turns a control, which hides the change.
- Graph work that reaches the audio: no late notes, no delayed playback renders
  (see the browser audio-isolation rules in CLAUDE.md).
- Hiding developer surfaces visually while leaving them reachable (JS exports,
  ui-state restore, saved prefs, the context menu's view mode).

## 4. The design

### 4.1 Developer tools: one switch, decided at build time

- **Build tag `release`.** Two files in `internal/ui`:
  `devtools_build_on.go` (`//go:build !release`, `const devToolsBuild = true`)
  and `devtools_build_off.go` (`//go:build release`, `false`). Plus
  `var devToolsForcedOff bool` and the one predicate
  `func devToolsOn() bool { return devToolsBuild && !devToolsForcedOff }`.
  Every developer surface consults `devToolsOn()` and nothing else. In a
  release build the constant folds, so the dev-only code paths can be dropped
  by the linker.
- **Builds.** `make wasm` (shipped) adds `release` to `WASM_RELEASE_TAGS`
  (now `noscenes release`). `make wasm-dev`, `make run`, `serve-lan`, the
  screenshot binary and every test build are developer builds. Any future
  packaged desktop build passes `-tags release`.
- **Seeing the production UI in a developer build.** The flag
  `-devtools=false` (desktop: screenshots, the tour, manual runs) and the URL
  parameter `?devtools=0` (dev wasm only; browser tests of the production UI)
  set `devToolsForcedOff`. Tests flip it with a `t.Cleanup` restore.
- **What `devToolsOn()` gates:**
  1. **Chain tab.** Its pill in `AllPanelTabs()`, the phone bottom-nav "Chn"
     segment (`viewModeChain`), `SetActiveEQTab("chain")`, the JS export that
     selects it (`js_exports_eq_widgets.go`), `SetChainVisible`, the context
     menu's view-mode switch (`drumview_context_menu.go`), restoring a saved
     or imported tab/view mode (falls back to Synth on desktop, Pads on
     mobile), and the Chain zone's construction.
  2. **Synth low-level.** The pipeline strip and its "+ Add" menu; the card
     chevron "›" that drills into a card's raw knobs; every drilled target
     (`setSynthDrillTarget` refuses any target other than the root in
     production: pipeline pill, card chevron, `selectSynthSection` /
     `SelectSynthSectionByLabel`, ui-state restore); the "More" expander;
     stage enable pills; and the view picker (§4.6).
- **The landing tour records the production UI.** The tour forces
  `devToolsForcedOff`. The `tab-chain` / `nav-chain` steps are deleted, and the
  caption "spectrum, levels and the whole signal chain" becomes "spectrum and
  levels, live". The landing media must be re-recorded (`make showcase-video`,
  a manual step on a real display) once the new Synth tab lands. Until then
  `TestLandingMediaMatchesTheTourAndThePage` is red, so the tour edit and the
  re-recording go in the same change, after §4.2–4.6 are done.

### 4.2 The production Synth tab

The header row is unchanged (`SYNTH · name`, Preview, Save, Save As, Reset,
⋮). Below it the layout and interaction change.

**Desktop layout**
- The left side holds category cards in a flow layout. Each card is sized by
  its dial count: width = padding + n × dial cell, and cards wrap. A card shows
  only its **title and its dials**. The one-line explanation moves into the
  graph caption when the card is focused, which ends the truncation.
- **Cards with no live dial are hidden in production.** 183 of the roster's
  957 root cards have no live dial on their instrument, for example Sound on
  82 melodic instruments and Source on 51 drums including Kick 808. Their
  content is discrete character choices (oscillator type, generator, variant)
  and tuning, which are developer territory; a regular user picks another
  instrument for another basic sound. Developer builds still show them, with
  their chevron. Measured 2026-10-24: after hiding, every roster instrument
  keeps at least 4 live dials on at least 2 cards (minimum 4, maximum 18).
  Guard `TestEveryInstrumentKeepsProductionDials` pins that floor as a ratchet.
- Layout goal: every roster instrument's production cards fit in two rows at
  1280×720. `TestProductionCardsFitTwoRows` checks it, with a reasoned
  exception list. Anything that does not fit scrolls, with the second row
  partly visible so the scroll is discoverable.
- The right side holds the **graph pane**, at least 40% of the Synth content
  width (min 320 px; new density tokens replace
  `SynthPreviewTargetW`/`SynthPreviewMinW`).
- Dials are larger, from a new density token `synthDialDiameter`
  (compact 28 / comfortable 36 / spacious 44 px), with the name under the ring
  and the 0–100 reading inside it. The focused dial has the focus ring; the
  focused card has an accent border.

**Phone layout.** The graph band is at the top (about 38% of the Synth content
height, min 120 px), with cards below in a two-column vertical scroll.

**Focus (one focused control at a time, in both modes)**
- Pressing, dragging or opening the wheel on a dial focuses that dial. Focus
  is sticky.
- Tapping a card body (not on a dial) focuses the card's first dial.
- Tapping empty space in the card area, or Esc with no popup open, clears
  focus, and the graph shows the overview.
- Dial gestures are unchanged: drag, the wheel popup and armed keys.
  Release/Accept is still one commit (`synth_dial_input.go`).

**Developer builds** draw exactly this plus the low-level surfaces: the
pipeline strip and "+ Add" above the cards, the card chevron "›" into the raw
knobs (with the dials repeated at the top of the drilled view), stage drills,
"More", and the Chain tab. Pressing a raw ParamCard focuses that param the
same way a dial is focused, and gets the same kind of graph (§4.4). The tap
behaviour is identical in both modes, so tests do not fork: card body = focus,
chevron = drill (developer builds only).

**Removed from both modes:** the root card thumbnails (`macroGraphKind` and
the "signature graphs"), the pipeline strip's stage thumbnails (text pills in
developer builds), the play-through line, and the "Up close"/mirror wave.

### 4.3 Where the audio comes from (the accuracy foundation)

One asynchronous entry point renders an instrument document the way the
platform plays it, including the instrument's Sampler edit:

`audio.RenderDocForGraph(id instrument.ID, d instrument.Doc, bpm int, pitch float64, done func(pcm []float32, sr int, err error))`

- **The Sampler edit is part of what plays, so it is part of the graph.**
  Playback applies the per-instrument edit (trim, pitch, gain, reverse,
  normalize, fade) after rendering on both platforms: native through the voice
  cache's `SampleEditFor(id)`, the browser through `instrumentSampleEdits` in
  `ensureRenderedSample`. The graph render applies `id`'s current edit to
  every slot (NOW, ENDs, BEFORE, ENUM, STAGE_OFF), through the same
  post-render step playback uses (factor it; never re-implement it). The cache
  key includes the edit descriptor. When an edit is present, the caption shows
  a "with Sampler edit" chip, so a trimmed or reversed picture is explained.
  Per-row insert effects, EQ, sends and pan stay out: the graph shows the
  instrument, not the row's mix.
- **Desktop:** the render the voice cache performs for a document (C render →
  document FX → gate tail → peak-normalise × amplitude → Sampler edit),
  factored out of `doc_voice_native.go` so that `RenderAsPlayed` and the graph
  share it. `RenderAsShipped` alone is not enough, because it skips the edit.
  It runs on the existing `synth.preview` pool (1 worker), so no new standing
  pool and no registry edits.
- **Browser:** a new JS export
  `renderDocPCM(id, block, seconds, amp, melodic) → Promise<{bytes, length, sr}>`
  that renders the flattened block through the **same function**
  `ensureRenderedSample` uses for a registered instrument, `id`'s sample edit
  included (factor it out; do not copy it), in the existing render-worker pool.
  The result crosses with `CopyBytesToGo`, like `captureInstrumentPCM`.
  - **Priority:** graph renders never delay playback. While any playback,
    prewarm or lane render is queued, graph renders wait. At most one graph
    render is in flight.
  - Add the export to `wasm_bridge_smoke.browser.test.js`.
- **`-tags test`:** a deterministic test-only toy voice (`//go:build test`)
  that honours a handful of params (amp envelope, a one-pole cutoff, pitch
  envelope, noise mix, drive), so UI plumbing tests can see graphs change. It
  is never linked into a developer or release binary.
- **What is rendered.** The note at the instrument's reference pitch (pitch 0;
  melodic instruments sound A3 there) at the current transport BPM. The
  caption names the note. The graph shows the natural, un-gated voice.
- **Render slots** per focused control, each coalesced latest-wins:
  - NOW: the live document.
  - END_LO / END_HI: the control at its two ends (§4.5).
  - BEFORE: free, because it is the NOW render from when the gesture started.
  - Developer builds only: ENUM_k (one per option of an enum knob, at most 8)
    and STAGE_OFF (the document with the focused param's stage switched off:
    "without this stage").
  - During a drag NOW is requested per detent, but only the latest pending
    request runs. The END slots are computed once per focus and recomputed only
    when another param changes.
- **Cache.** An LRU keyed by (flattened block hash + render fields, BPM,
  pitch, sample rate), holding the PCM and the computed view data. Capacity
  16 renders, about 6 MB at worst.
- **Stale and failure.** The previous NOW keeps drawing until the new one
  lands. A small "updating" chip appears only after 300 ms. If a render fails
  the graph keeps the last real picture. If there has never been one, it shows
  an empty plot with "no picture yet". It **never** falls back to an
  approximation.

### 4.4 The views (what each graph shows)

There is one new pure package, `internal/soundview`: no Ebiten, no cgo,
wasm-safe, and it imports only `internal/wave`. It turns PCM + sample rate +
voice length into the data for each view. It must **not** import
`internal/audio/fingerprint`, which is analysis tooling, large, and kept out
of shipped binaries; agreement with it is pinned by tests instead (§5).

Every view shares these rules:
- **True scale.** Level is never gain-normalised per render. NOW and every
  ghost share one axis.
- **Fixed axes per focus.** The time span is the longest audible length among
  the renders shown, rounded up to a nice step. It is fixed while the control
  stays focused and never rescales during a gesture.
- **Labelled axes.** Time in ms/s with beat ticks at the current BPM;
  frequency in Hz (100 · 1k · 10k) with "low/high" words at the ends; level in
  dB (0 · −20 · −40) with "quiet/loud". Developer builds add a hover readout
  of exact values.
- **One readout line**, measured on NOW, with "(was X)" once a BEFORE exists.

| View | Title shown | What it plots (from the rendered PCM) | Readout |
|---|---|---|---|
| `overview` | Your sound | The whole note as a clip: a peak band plus an RMS band per column (solid fill, no stroke dots), true scale, time axis = the voice length on this platform, and a "note ends" marker. Under it, a thin colour-over-time strip (the `colourTime` data). | "Sounds for 0.42 s · brightness centre 1.2 kHz · 35% noise" |
| `loudness` | How loud, over time | Short-term RMS in dBFS (10 ms window, 5 ms hop; 20 ms window when f0 < 100 Hz), floor −72 dB, with the "note ends" marker. | "Sounds for 0.42 s" (time to −40 dB below peak) |
| `attack` | The first moment | The waveform itself over [0, T], where T = clamp(3 × measured attack time, 20 ms, 250 ms), extended to cover every onset (burst hits) + 20 ms. The envelope is overlaid. | "Full volume after 12 ms" |
| `pitch` | How high, over time | f0 per 10 ms hop, 25–4000 Hz, drawn only where voiced, on a semitone axis with note-name ticks. | "Starts 150 Hz (D3), settles 44 Hz (F1) after 80 ms" or "wobbles ±20 cents, 5× a second" |
| `colour` | Tone colour | The average power spectrum over the audible part (onset → −40 dB), 1/6-octave smoothed, log-Hz 20 Hz–20 kHz, dB relative to max, floor −60. A marker sits at the brightness centre. | "Brightness centre 1.2 kHz" |
| `harmonics` | Harmonics (pitched sounds only; otherwise `colour`) | The same spectrum on a **linear** axis from 0 to 12·f0 over the sustain window, so harmonic peaks fall on evenly spaced 1×, 2×, 3× … ticks. Detune shows as split or wide peaks, richness as more and louder peaks, drive as rising odd harmonics. | "9 harmonics above −40 dB; strongest: 3rd" |
| `colourTime` | Tone colour, over time | A spectrogram: STFT 2048/512 at 48 kHz, 64 log-Hz rows from 30 Hz to 16 kHz, dB → brightness through the Visualizers ramp, with the brightness-centre line overlaid. | "Brightness centre falls 3 kHz → 800 Hz" |
| `noise` | Tone vs noise | Per frame, tonal energy (spectral peaks above the smoothed floor) vs noise energy (the rest), as a stacked area over time. | "35% noise, 65% tone" |
| `wave` | Wave close-up | Three period-aligned cycles at t* (25% into the audible length for sustained sounds; loudest frame + 10 ms for percussive ones), or a 20 ms window when unpitched. True scale, ghosts at the same t*. | none |

**Comparison marks** draw on top of NOW:
- END ghosts are faint, labelled with the dial's end words ("dull" / "bright")
  or "min"/"max" for raw params.
- The BEFORE ghost is dashed, labelled "before", and kept until focus moves.
- **Difference shading** tints the area between NOW and BEFORE wherever they
  differ by more than the view's visible threshold. This is the automatic
  "here is what changed" highlight, with no per-control rules.
- The spectrogram cannot overlay ghosts. It shows its ENDs (and BEFORE) as
  two small side thumbnails instead.

**Morph.** When NOW's data changes, the curve eases from old to new with the
existing `synth-graph-morph` token. The spectrogram crossfades.

**Budget.**
- Desktop: analysis runs on the pool, ≤ 15 ms per render for the views
  required.
- WASM (one thread): analysis is incremental, advanced from `Update` at
  ≤ 3 ms per frame. Only the focused view plus the overview readouts are
  computed. A browser bench pins it.
- Drawing: each view is a cached `TraceSprite` keyed by (view data hash, size,
  `ThemeGeneration()`). The NOW trace is its own sprite layer, so the glow
  (§4.7) re-tints only that layer.

### 4.5 Which view each control gets

**Dial ends** are `Turn(start, 0)` and `Turn(start, 100)`. Ends are exact, so
they do not depend on the start. **Raw param ends** are the ParamDef's
`Min`/`Max`. **Enum knobs** (developer builds) show small multiples, one per
option.

Dial views are DATA. They go in `internal/assets/synth_dials.json` as a new
`"view"` field per dial, plus `"lo"`/`"hi"` i18n keys for the end words
(validated by `synthdial.Load`; en + es strings in `internal/i18n`). The
mapping:

| Dial | Cards | View | End words (lo / hi) |
|---|---|---|---|
| richness | sound, source | harmonics | plain / rich |
| bite | sound | harmonics | smooth / biting |
| glassy | sound | harmonics | warm / glassy |
| bright | brightness, tone | colour | dull / bright |
| sizzle | tone | noise | clean / sizzly |
| shimmer | tone | colourTime | steady / shimmering |
| edge | brightness | colour | soft / ringing |
| sweep | brightness, tone | colourTime | still / sweeping |
| bloom | brightness | colourTime | instant / blooming |
| fadein | attack | attack | instant / slow |
| click | attack | attack | soft / clicky |
| snap | punch | attack | soft / snappy |
| length | length | loudness | short / long |
| room | length | loudness | dry / roomy |
| tail | length | loudness | cut / lingering |
| hold | length | loudness | fades / holds |
| spread | width | harmonics | narrow / wide |
| blend | width | harmonics | single / blended |
| drift | humanfeel | pitch | steady / drifting |
| vibrato | humanfeel | pitch | still / wavy |
| looseness | humanfeel | pitch | tight / loose |
| depth | movement | by LFO target¹ | none / deep |
| speed-lfo | movement | by LFO target¹ | slow / fast |
| delay | movement | by LFO target¹ | at once / later |
| body | body, boom | colour | thin / full |
| expression | body | colourTime | even / expressive |
| weight | boom, warmth | colour | light / heavy |
| ring | boom | colourTime | dead / ringing |
| air | boom | noise | dry / airy |
| drive | warmth, grit | wave | clean / driven |
| drop | pitchdrop | pitch | flat / big drop |
| speed-drop | pitchdrop | pitch | slow / fast |
| character | voice | colour | plain / vocal |
| breath | voice | noise | clear / breathy |
| shift | voice | colour | low / high |
| shine | voice | colour | dull / singing |
| talk | voice | colourTime | still / talking |
| spread-flam | flam, punch | attack | tight / spread |
| strength | flam, punch | attack | faint / strong |
| sharp | flam | attack | soft / sharp |

¹ `lfo_target` Amp → `loudness`, Pitch → `pitch`, Cutoff → `colourTime`.
The data spells this as `"view": "lfo-target"`, resolved against the live
document.

**Raw params (developer builds)** route through one UI table,
`synthParamViews` (it replaces `synthFocusRenderers`), keyed by group, with
name overrides. Every visible param must resolve explicitly. There is no
fallback, and a discipline test enforces it, like
`TestSynthFocusRenderer_EveryGroupExplicit`. Starting assignment:

| Group | Default view | Overrides |
|---|---|---|
| osc | harmonics | `osc_type` → wave; `osc_octave`, `osc_detune` → pitch |
| fm | harmonics | `gen1_fm_dec*` → colourTime; `gen1_fm_pe_*`, `gen1_fm_base` → pitch; `fm_algorithm`, `gen1_fm_variant` → harmonics (small multiples) |
| filter | colour | STAGE_OFF ghost = the sound before the filter |
| filtenv | colourTime | |
| env | loudness | `amp_attack` → attack |
| lfo | by LFO target | |
| pitchenv, voice | pitch | |
| burst | attack | |
| unison | harmonics | drift, vib, humanize, jitter, scatter → pitch |
| formant | colour | `formant_morph_rate`, `formant_morph_to` → colourTime; `formant_breath` → noise |
| resonator | colour | `bow_dynamics` → colourTime |
| post | wave | `gain` → loudness; `pitch` → pitch |
| finish | colour | `post_pitch` → pitch; `post_decay*` → loudness; `post_drive`, `out_clamp` → wave |
| family (`gen1_wave`) | wave | |
| kick | colour | `attack`, `click` → attack; `env0`, `env1`, `fade`, `hold`, `mode_decay`, `reverb` → loudness; `noise`, `hiss` → noise; `pe_amt`, `pe_rate` → pitch; `sat` → wave; `mode_gain`, `mode_detune` → colourTime; `variant` → overview |
| snare | colour | `attack` → attack; `*_d` decays → loudness; `noise_m`, `wire_m` → noise; `variant` → overview |
| tom | colour | `stick` → attack; `ring`, `room` → loudness; `sweep` → pitch; `variant` → overview |
| cymbal | colour | `attack`, `spark*` → attack; `decay_tilt`, `env_*`, `noise_d`, `rattle_decay`, `rattle_hold` → loudness; `beat`, `rattle_rate`, `drive_delay` → colourTime; `noise_m`, `wash`, `rattle` → noise; `variant` → overview |
| bass | harmonics | `atk_*` → attack; `env_fast_rate`, `out_scale` → loudness; `pitch_env_*` → pitch; `sat_k` → wave |
| modal, noiseband | colour | `modal_decay_x`, `modal_law_*` → colourTime; `noiseband_level` → noise |
| strike | attack | |

The measurability test (§5, T4) is the arbiter. A control whose assigned view
does not move goes to a different view, or into the known-weak ratchet with a
reason.

### 4.6 Developer extras

- **View picker.** A chip in the graph caption shows the focused control
  through any view, for diagnosis.
- **STAGE_OFF ghost** on stage params: "without this stage".
- **ENUM small multiples.**
- **Hover readouts** with exact values: Hz, dB, ms, cents.

All of these are gated by `devToolsOn()`.

### 4.7 Trigger glow (replaces the play-through line)

- **Triggers.** The existing sync sink `onInstrumentTriggered` (the highlight
  drain plus Preview) now starts a glow, not a play-through. It fires only
  when the triggering row is audible (not muted, not soloed out), because a
  graph that shines while nothing sounds is a lie. It starts on the same
  highlight event that lights the row's cell, so the whole UI flashes together.
  There is no output-latency compensation: a flash within ±30 ms reads as
  simultaneous, and the line that needed precision is gone.
- **Look.** The NOW trace (stroke + fill) brightens toward the brighter
  Visualizers tone, plus a soft ring on the plot. Ghosts, axes and text do not
  change. The resting look is the minimum; the graph never dims or hides.
- **Duration follows the real note.** Intensity(t) is the NOW render's own
  smoothed amplitude envelope at the elapsed time, normalised to its peak.
  The flash holds full intensity for at least 100 ms and ends when the
  envelope falls below −40 dB or at 1.5 s. A hi-hat flickers, a pad glows
  long, and a retrigger restarts it. This applies to every view, including the
  overview and the phone band.
- **Motion token.** New DESIGN.md token `synth-graph-glow` (min hold 100 ms,
  floor −40 dB, cap 1.5 s), generated like `synth-graph-morph`.
- **Cost.** While glowing, two draw calls per frame: a re-blit of the NOW
  layer with a `ColorScale` boost, plus the ring. Nothing re-renders.

### 4.8 What gets deleted

- `internal/audio/preview_render.go` and its tests (the only users are
  Synth-tab files).
- `synth_concept_viz.go`, the renderer table in `synth_focus_graph.go`,
  `synth_full_note.go`, `synth_mirror.go` (keep its pool name for §4.3),
  `synth_stage_thumbs.go`, `synth_modes_graph.go`, and the plotting half of
  `synth_preview_layer.go`.
- The playhead half of `synth_signal_graph.go` (`playFrac`,
  `drawSynthPlayhead`, `synthGraphTimeWindowSec`) and the `synthCurveHook`
  seam.
- `macroGraphKind` and the card signature graphs.
- Every test and scene that pins those, replaced per §5. Examples:
  `synth_concept_*_test.go`, `synth_focus_sweep_test.go`,
  `crop_synth_graph_playthrough_mid`, `crop_synth_focus_*`.

## 5. How we know it works (guards)

All of these are new.

- **T1 `TestSynthGraphsDrawOnlyRenderedAudio`** (discipline, fast path). No
  Synth-tab graph file calls anything except `RenderDocForGraph` and
  `soundview`, and nothing references a voice model, `preview_render`, or a
  playhead.
- **T2 soundview known answers** (pure, fast path), checked against synthetic
  signals:
  - an exponential decay's −40 dB time;
  - a linear ramp's attack time;
  - an exponential glide 150→44 Hz tracked within 10 cents after two periods;
  - vibrato depth and rate;
  - sine sums giving peaks at the right bins on both spectrum axes;
  - sine+noise mixes giving the right noise fraction;
  - a chirp in the spectrogram;
  - wave period alignment.
- **T3 Agreement with `fingerprint`** (real build, test-only import). The
  soundview readouts (attack, audible length, brightness centre, noise
  fraction) agree with `env.attack`, `env.total_audible`, `bright.centroid`
  and `noise.fraction` on every roster instrument within stated tolerances. The
  numbers a user reads are the ones the dial tests already trust.
- **T4 `TestEveryControlMovesItsGraph`** (real build, `!test`), the inverse of
  today's failure. For every roster instrument × every live dial (and, in a
  second pass, every visible raw param), the control's view at END_LO vs END_HI
  differs visibly at the reference size: ≥ 3 px peak deviation for curves,
  ≥ 6 dB in ≥ 3% of cells for the spectrogram, and the readout moves.
  Misses go in `testdata/synth_graph_known_weak.json`, one reason per entry,
  shrink-only, `SYNTH_GRAPH_WEAK_UPDATE=1`. Expected first entries include the
  Kick 808 Saturation finding.
- **T5 `TestGraphRenderIsThePlayedVoice`.** Each check covers an instrument
  with no Sampler edit and one with an edit (trim + reverse + gain).
  - Desktop: `RenderDocForGraph(id, liveDoc)` is bit-identical to
    `RenderAsPlayed(id)`.
  - Browser: `synth_graph_render.browser.test.js` checks that the bridge
    render of the live block is bit-identical to the cached playback buffer,
    and that soundview digests on WASM match native through the
    `audiotap`-style quantised digest.
- **T6 Axes.** Turning Length lengthens the drawn note on an unchanged axis
  (no rescale mid-gesture), and a BPM change changes the drawn voice length on
  desktop.
- **T7 Glow** (fake clock). The glow follows the envelope, never drops below
  rest, and ends. A muted row, another instrument, or the tab not showing → no
  glow. Preview → glow.
- **T8 `TestProductionUIHasNoDevSurfaces`** (`devToolsForcedOff`, real input
  through `Game.Update`). No Chain pill, no "Chn", no pipeline strip, chevron,
  "+ Add" or "More" hit areas. Every entry point refuses: JS exports, ui-state
  restore, saved prefs, the context menu, keys. Its non-vacuous twin
  **`TestDevBuildHasEveryDevSurface`** checks the opposite. A deploy test
  (beside `TestShippedWasmLeavesOutTheSceneCatalog`) checks that the shipped
  tags include `release`.
- **T9 Tour.** `TestShowcaseTourReachesEveryStep` runs in production mode with
  no Chain step, and the landing media tests pass after the re-recording.
- **T10 Audio isolation.** `make bench-isolation` with the `tab-synth`
  scenario plus a dial drag shows no rise in `arrivedLate` or `jsQueue` over
  baseline (CLAUDE.md § Browser audio isolation).
- **Budgets.** Update `synth_drawcall_budget_test.go` and the Synth-tab alloc
  budgets with a one-line rationale each. Add a WASM analysis bench for the
  ≤ 3 ms/frame rule.
- **Visual verification (mandatory, CLAUDE.md).** New scenes, desktop and
  `MOBILE=1`, each opened and inspected:
  - `synth_prod_root` (production, `-devtools=false`, nothing focused);
  - `crop_synth_view_<view>` for all nine views on fitting instruments (Kick
    808 Drop → pitch, Violin Bright → colour, Snare Sizzle → noise, …);
  - `crop_synth_view_before_after` (BEFORE + difference shading);
  - `crop_synth_glow_peak` (deterministic clock);
  - `crop_synth_dev_raw_param` and `crop_synth_dev_view_picker`.

  Scenes render synchronously (like `renderSynthMirrorNow`) so captures are
  deterministic. Run them with `XDG_CONFIG_HOME` pointed at a scratch dir.

## 6. Suggested slicing (each slice shippable)

1. **Gate + layout.** `devToolsOn()`, the `release` tag, the flag and URL
   parameter, hiding Chain and the low-level surfaces everywhere (T8), the
   production card layout and focus model (hiding dial-less cards,
   `TestEveryInstrumentKeepsProductionDials`, `TestProductionCardsFitTwoRows`),
   and removing the playhead. The graphs stay as they are for now.
2. **Real audio.** `RenderDocForGraph` (desktop + browser bridge + test toy),
   the cache, `soundview` with T2, T3 and T5, the overview view, and the glow
   (T7).
3. **Explaining views.** The nine views, the ENDs, BEFORE and difference
   shading, readouts, the dial `view`/`lo`/`hi` data, `synthParamViews`, T1,
   T4, T6, the developer extras, and all deletions (§4.8).
4. **Landing.** The tour edit, `make showcase-video` re-recording, T9, then
   deploy.

## 7. Decisions made in this document (not by the user)

- Gate at build time (`release` tag) with no runtime unlock on beatmo.io.
  Developer builds can preview the production UI with
  `-devtools=false` / `?devtools=0`.
- In production, tapping a card focuses it rather than drilling. Dials are the
  only production controls, and cards without a live dial are hidden there.
  That costs regular users the discrete character choices (oscillator type,
  generator, variant); every instrument still keeps at least 4 dials.
- The graph includes the instrument's Sampler edit, because playback applies
  it, and excludes per-row FX, EQ, sends and pan.
- No model-drawn curves at all, including filter-response and ADSR shapes.
  Even "exact" analytic curves are a second implementation that drifts.
- The graph renders at pitch 0 and the current BPM, and shows the
  platform's own voice, so cross-platform divergences become visible rather
  than averaged away.
- A spectrogram strip sits under the overview, and harmonics is a
  linear-axis spectrum zoom rather than a separate measurement.
- The glow starts on the row-highlight event (no latency compensation), and
  its length is the measured envelope.
- The root card thumbnails and stage thumbnails go away rather than being
  rebuilt on real audio.
- The dial → view and param → view tables in §4.5 and the 80 end words are a
  first pass. T4 corrects any that do not move.

## 10. Amendment (user, 2026-10-25): "Please remove the old graphs."

§4.8 is reinstated for the graphs: everything it lists is deleted, together
with the "Legacy" view, the mirror's two JS exports and their browser test,
and the helpers only the old graphs used (`audio.ModularFilterResponse`,
`audio.GetChannelEQResponse`, `audio.ModularFamilyStages`, the
`synth-graph-morph` motion token). The `synth.preview` pool moves to
`synth_graph_pipeline.go` under the same name. The glow keeps its own small
clock (`synth_graph_glow.go`). T1 is now implemented as
`TestOldSynthGraphsAreGone` + `TestSynthGraphsDrawOnlyRenderedAudio`. The
rest of §9 stands: the Chain tab and the low-level controls are still hidden,
not removed.

## 9. Amendments (user, 2026-10-24, on approval)

These override the sections above where they conflict:
- **Do not remove existing code or functionality.** §4.8 is withdrawn.
  Everything listed there stays in the tree. The new views become the default
  graph, and developer builds can still show the legacy graph through the view
  picker (a "Legacy" entry). Fixing bugs is fine.
- **Hidden, not removed:** the Chain tab and the low-level controls stay
  compiled into the production binary and are only hidden from view there.
  `devToolsBuild` is therefore a `var` set by the build-tagged file, not a
  `const`, and no dead-code elimination is sought.
- A refused tab or view-mode switch (Chain in production) leaves the current
  tab active. That is simpler than the "falls back to Synth/Pads" of §4.1, and
  equally never blank.
- Developer builds keep today's card look and tap-to-drill. The compact
  cards and tap-to-focus are production-only. Dial focus works in both modes.

## 8. Left to the builder

- The exact algorithms inside the T2/T3 contracts (pitch tracker, tonal/noise
  split, smoothing).
- Pixel metrics, through density tokens.
- Colours within the DESIGN.md Visualizers family. New tokens for ghost,
  before, difference tint and glow follow the token-change checklist.
- Readout copy in en/es, in the existing plain-English tone.
- File and package names other than `soundview`, `devToolsOn`,
  `RenderDocForGraph`, `renderDocPCM`, `synthParamViews`, the `release` tag and
  the T-numbered test names.
