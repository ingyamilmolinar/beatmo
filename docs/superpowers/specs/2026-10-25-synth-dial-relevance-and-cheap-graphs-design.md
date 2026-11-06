# Synth dials the DSP vouches for, and graphs that stay cheap

Date: 2026-10-25 · Status: implemented 2026-10-25 (§7 records what changed from this plan and what was measured).
Follows `2026-10-24-synth-graphs-and-devtools-design.md` and its review
(memory `project_synth_graph_dial_review_2026_10_25`).

## 1. What the user asked for

1. Readouts in plain words by default, with a literal/objective mode for the
   numbers ("hide the details").
2. Graphs that never go over budget and never cost frames or audio; basic,
   cheap, and reused while the sound signature does not change. Verify it.
3. No known-weak lists ("a code smell"). A dial that is irrelevant for an
   instrument is HIDDEN. The audio DSP is the source of truth: the audio
   change decides relevance, and it is easy to re-validate and update.
4. A knob's value maps to the SAME underlying param values (or the same
   transformation) on every instrument: min/max are defined per knob, never
   per instrument. Backwards compatible.
5. Config-only, real-time audio changes; no baked parameters.
6. Alternate design approaches (C engine changes allowed) — §6.

## 2. Measured starting point (review 2026-10-25)

- 16 dial ends render SILENCE: Drive 0 on 10 kicks + 2 sub-basses
  (`softsat(mixed * kSat)` — kick_sat is a gain there), Shimmer 0 on the
  hi-hats/ride (`gen_cym_tone_m` is the plate's whole level), Breath 0 on
  voice-whisper. No test sees it (silence counts as "audible").
- 77 production dial × instrument sweeps are inaudible by the repo's own
  oracle; `synth_dial_known_weak.json` + `synth_graph_known_weak.json` hide
  them from the tests. The two lists disagree: 25 audible changes the graph
  does not show, 27 inaudible changes the graph draws.
- Spans are already global except `gen1_env_fast_rate` (clipped by a narrower
  range on 2 bass instruments) and the voice-length caps
  (`synthDialRangeCaps`: Tail and Fade-in stop at half the voice), which make
  "Tail 100" a different release on every instrument.
- Analysis: 24 ms per 1 s note, 56 ms per 2 s (native); the glow re-tessellates
  the NOW polyline every frame.

## 3. Design

### 3.1 One knob, one meaning (global spans)

- A term's span (`from`/`to` in `synth_dials.json`) is the ONLY definition of
  what the knob does to that param. No instrument range may clip it
  (`TestDialSpansAreTheSameOnEveryInstrument`), and the per-voice range caps
  are deleted.
- Turning is unchanged: `Turn(start, target)` is the same proportional shift on
  every instrument (ends exact and global; single-term dials exact at every
  value). This is "the same transformation" without making every shipped
  instrument jump on first touch.
- The Tail cap existed because of an engine defect (a release longer than half
  the voice kept its full ramp time and hard-cut). Fixed in C: when the release
  is capped to half the voice, its ramp is fitted to that half. Byte-identical
  for every document whose release is not capped (none of the shipped ones).
- Silent ends are fixed in the knob's definition, globally: Drive's
  `gen1_kick_sat` / `gen1_sat_k` terms start above 0, Shimmer stops driving the
  plate level (`gen1_cym_tone_m` leaves the dial), Breath's floor is above 0.
  `TestNoDialEndSilencesItsInstrument` (real build) guards every roster
  instrument and every dial, shown or not.

### 3.2 Relevance from the DSP (replaces both known-weak lists)

A dial is RELEVANT for an instrument document iff turning it from 0 to 100
changes the rendered note audibly (the shared audibility oracle, moved out of
test code into `soundview.Audible`). An irrelevant dial is hidden, in every
build. There is no exception list anywhere.

- **Which document.** The instrument as it was when the Synth tab first showed
  it in the open circuit (the ANCHOR). Edits made in the tab never re-hide or
  re-show dials mid-session (no card jumps); opening a circuit, an import or an
  undo across a circuit re-anchors.
- **Where the verdict comes from**, cheapest first, keyed by the anchor's
  SOUND SIGNATURE (`audio.SoundSignature`: flattened block + render meta):
  1. `internal/assets/synth_dial_relevance.gen.json`, generated for every
     shipped document by `make gen-dial-relevance`;
     `TestDialRelevanceTableIsCurrent` (real build, `make test-real`)
     re-measures and fails with the command to run when a tuning pass made it
     stale. This is the easy validate/update loop.
  2. A runtime cache of verdicts computed in the background (same function) for
     anything else — the startup demo, user-saved and imported instruments.
  3. Until a verdict lands, the dial shows (the config gates still apply).
- **Guards without lists:** every shown dial is audible (by construction, and
  re-proven by the staleness test); every shown dial's graph visibly moves
  (`TestEveryControlMovesItsGraph`, no list): a view that does not show an
  audible change is a view to fix, never an entry to add.

### 3.3 Graphs: words by default, numbers on request, fewer and cheaper views

- **Two readout modes.** WORDS (default): "A short, dark hit", "Drops from high
  to low, quickly", "Brighter than before". NUMBERS: the literal measurements
  ("Sounds for 248 ms (was 63 ms)"). One small toggle on the graph, persisted
  in prefs (`AudioPanelStateDoc.SynthGraphNumbers`). "(was …)" carries only the
  key number.
- **Views** (both builds): the first moment becomes a zoomed loudness curve
  (dB, 1 ms) instead of a raw waveform; colour over time becomes a
  brightness-over-time line (the heat map is gone); the wave close-up falls back
  to tone colour on an unpitched sound; the overview is a smooth loudness band
  without the colour strip; tone colour is drawn on ONE shared level scale (it
  normalised every curve to its own peak). Pitch readout: a glide is judged on
  medians, a wobble needs a real oscillation.
- **Labels** never overlap each other or the axis words.

### 3.4 Budget (and its proof)

- Reuse: the NOW slot is identified by the sound signature, not the revision —
  an edit that lands back on a sound already rendered costs nothing.
- Analysis only up to the note's audible end; STFT hop size/2; a running-sum
  noise floor; the overview from the envelope (no PCM scan on the UI thread).
- The glow re-blits a cached NOW layer with a ColorScale (no per-frame path
  tessellation).
- Budget gates: `BenchmarkSynthGraphAnalysis` + a native budget test (≤ 15 ms
  per 2 s note for the production views); the browser isolation bench gets a
  `synth-graph-storm` scenario (dial turns while playing): no late / missing
  notes, jsQueue ≤ one frame, frame time compared against `idle`.

## 4. Out of scope here

Re-recording the landing videos; the browser-vs-desktop note-length divergence
(`render.seconds` vs `render.beats`, §6.4 proposes the fix).

## 5. Implementation order

1. Oracle → `soundview.Audible` (move, with its tests).
2. Global spans + C release fit + silent-end floors + their guards.
3. Relevance: signature, anchor, table + generator + staleness test, runtime
   cache, hide; delete both known-weak lists and their ratchet code.
4. Views + word/number readouts + label layout.
5. Performance + budget tests + browser bench; screenshots before/after.

## 6. Alternate approaches (proposal; each keeps the config-only rule)

1. **Normalised module contracts in C.** Every module param gets one documented
   meaning with a neutral value that never mutes: drive = `(1+k)` pre-gain with
   output level compensation in every variant, a module's LEVEL separate from
   its colour (the cymbal plate's `tone_m` vs its beat), filters that report
   where they act. Same knob value → same transformation everywhere, by
   construction rather than by curating spans. Backwards compatibility through a
   schema-version migration that rewrites stored values (`k' = k − 1` for gain
   variants) and pins the old documents for templates (`instrument_pins`).
2. **Engine-reported sensitivity instead of 2·N renders.** The C render returns,
   alongside the PCM, a small per-stage activity report (each stage's input and
   output RMS / band energy, the filter's energy near its cutoff, a layer's
   contribution). Relevance and "which view shows this" fall out of ONE render
   of the anchor instead of two renders per dial; deterministic, cheap enough to
   run live on every document.
3. **Analysis as a render by-product.** The same pass emits the graph data
   (envelope, band energies per frame, centroid, f0 track) in the render worker,
   so the browser's main thread only draws; graphs, relevance and the refs
   tooling read one analysis.
4. **One voice-length definition.** Beats on both platforms (or seconds on
   both), with an automatic end fade, so the same document is the same note
   everywhere; the Length dial could then include the note length itself.
5. **Engine routing as data, not UI mirrors.** `synthDialConditions`,
   `synthDialRangeCaps` and the family routing tables restate C dispatch by hand.
   Each module declaration states which params it reads under which values;
   the generator emits that table for Go, JS and the dials.
6. **Loudness instead of peak normalisation.** Peak-normalising every voice
   erases level-type changes (many "inert" dials are a gain the normaliser
   removes) and lets one transient set a whole note's level. A per-document
   loudness target keeps tone changes heard as tone and makes gain-like terms
   either explicit LEVEL or gone.
7. **Streaming voices.** Knob changes today re-render whole voices (the lane
   swaps buffers). A block-rate voice that reads the param block per audio block
   (with smoothing) makes every config change real-time without a re-render; the
   graph render then becomes the same engine run offline.
8. **Content-addressed renders everywhere.** Graph, preview, relevance and the
   playback voice cache share one cache keyed by sound signature + tempo + pitch
   + Sampler edit, so a sound is rendered once per session wherever it is shown
   or played.

Recommended order: 4 and 5 first (small, remove whole classes of divergence),
then 2+3 together (they replace the expensive parts of this spec), then 1 with
its migration, then 6/7 as the larger engine projects.

## 7. As built (2026-10-25)

What changed from §3 while building it, each forced by a measurement:

- **Relevance = audible AND visible.** French horn's Sweep passed the
  audibility oracle (1.9 dB whole-note mean, all of it in quiet high bands:
  one band moved 0.5 dB, the brightness centre 440 → 437 Hz) yet no view could
  show it. A dial is shown only when its sweep is audible and some view moves
  ≥ 3 px. 79 dial × instrument pairs are hidden (shipped roster).
- **The graph chooses its view per instrument** (`chooseDialView`): a dial's
  named view is kept when it moves; otherwise the view that moves most, on
  the two end renders it has anyway. 56 shown dials needed it (Room on toms,
  Expression, brass Sweep, Looseness / Drift…). The brightness-over-time axis
  fits the sounds on screen (≥ 1.5 octaves) instead of 9 octaves.
- **The startup demo's own sounds are pre-measured** into the table as a cache
  (`source: "demo"`, never checked for staleness): the first Synth-tab visit
  renders nothing extra; a hand-edited demo sound is measured at runtime.
- **Content-keyed renders** (`cacheKey`): NOW, the ends and BEFORE share one
  render per sound; an undo or a dial turned back costs nothing.
- **Wave shape words** use lobe fullness (area / peak × width per half-cycle),
  placed between the dial's ends when the graph has them: crest factor,
  near-peak share and a Goertzel harmonic share were all fooled by a kick's
  decay and glide (the clean kick read "squared off").
- **A relevance-step race** (pool worker + a synchronous drain both finishing
  the same sound) panicked while holding the pipeline lock and froze the app
  in scene setup; steps are serialised (`relStep`), guarded by
  `TestRelevanceStepsFromTwoGoroutinesFinishEachSoundOnce` under `-race`.
- **The C release fix moved exactly seven shipped voices** (their release
  exceeds half their note at 120 BPM): goldens re-pinned with the reason;
  fundamentals unchanged in note (modular-pad +0.0001 Hz, tubular-bells only
  its detector confidence).
- **Spans**: the bass family's raw `env_fast_rate` range widened to the Length
  dial's 0.02–40 /s (log); shipped documents use 0–130 across 66 instruments.

Measured:

| | |
|---|---|
| analysis per view, 1 s note (native, fastest of 7) | 0.3 – 8.8 ms (budget 15) |
| analysis, all views, 1 s / 2 s note | 23.6 → 11.0 ms / 55.6 → 25.9 ms (STFT hop ×2, audible-end trim; machine load differed) |
| Synth tab allocations per frame | 921 budgeted → 78 (budget now 102) |
| glow | one bake per picture + theme, then one blit per frame |
| browser, 20 s storm (32 dial gestures, 8 instrument switches, playing) | every note heard; 0 late > 20 ms; jsQueue p99 0.8 ms; IOI p95 10.6 ms (idle 11.7); FPS 126 (tab open, idle storm: 124) |
| release WASM | 28.36 MiB raw (cap 28.50), main.wasm gzip 6.315 MiB (cap 6.40) |

Still open (proposals in §6): the browser/desktop note-length divergence; a
short note's Length dial can only shorten it (the note length is not a dial
term); Weight on kicks adds upper harmonics, so its graph says "brighter";
peak normalisation erases level-type dial changes.
