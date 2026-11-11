## Synth Pipeline Reference

> **Before tuning any instrument, read `AGENTS.synth.instruments.md` §"Tuning by
> measurement".** Three of the ~55 analysis axes have earned trust against the
> listener corpus; two are ANTI-correlated and tuning to them makes instruments
> worse (`ring T60` is the trap — a previous rebuild optimised against it and
> recorded success while the ear disagreed). `cd src/go && go run ./cmd/refs
> validate` prints the current standing; `refs validate -terms` maps the
> listener's own words ("airy", "warmer", "wooden") onto axes; `make
> probe-sounds` plays isolated single-property stimuli for calibrating a
> threshold by ear. To ask whether a voice still reads as THAT instrument, use
> the essence distance — `make wav-match-map` names every fidelity gap with the
> losing term and the direction the voice is off (same §, "Is it still THAT
> instrument?"); `synth-match`'s distance is not a measure of that.

This is the hub document for Beatmo's audio synthesis and processing pipeline. It provides the architectural overview; satellite files contain detailed reference for specific subsystems.

**Audience**: AI agents working on audio code. Optimized for precise signal flow, exact file references, and complete parameter catalogs.

---

### End-to-End Signal Flow (Desktop)

```
Trigger: Play(id) / PlayVol(id, vol) / PlayParams(id, vol, pitch, dur)
  │
  ├─ Voice Cache lookup
  │   hit  → the cached float32 buffer (SHARED, not cloned; no render on a hit)
  │   miss → C synth render (modular engine: modular.c / modular_stages.c) → cache result
  │
  ├─ Voice wrapping (in order):
  │   cVoice (raw buffer) → antiPopVoice (5ms fade-in, 20ms fade-out)
  │   Optional: scaledVoice (volume), resampleVoice (pitch/duration)
  │
  ├─ mixer.Schedule(id, wrappedVoice, delaySamples)
  │   → pending queue (separate lock, drained at block start)
  │
  ╔══════════════════════════════════════════════════════════╗
  ║  Audio Thread — Read() called by oto player              ║
  ║                                                          ║
  ║  PHASE 1: Voice Rendering                                ║
  ║    For each active voice:                                ║
  ║      SampleBlock() → apply 0.18× headroom (−14.9dB)     ║
  ║      → accumulate into instBufs[instrumentSlot]          ║
  ║                                                          ║
  ║  PHASE 2: Per-Instrument Channel Processing              ║
  ║    2a ProcessInsertsBlockLocal: volume → inserts         ║
  ║       → postFXBuf (tap StageInsertFX)                    ║
  ║    2b ProcessEQBlockLocal: EQ → postEQBuf (tap StageEQ)  ║
  ║       → accumulate into masterBuf                        ║
  ║    (every channel with an effect or EQ runs every block, ║
  ║     voice or not: tails ring out, LFOs advance)          ║
  ║                                                          ║
  ║  PHASE 3: Master Channel Processing                      ║
  ║    masterBuf → volume → master EQ → workBuf              ║
  ║    → C soft clip (identity to 0.9, smooth to ±1) → int16 ║
  ╚══════════════════════════════════════════════════════════╝
  │
  └─ oto player → system audio output
```

### End-to-End Signal Flow (WASM / Browser)

```
Trigger: Play(id) — Go side schedules only (no Go voice cache in the browser build)
  │
  ├─ Go calls JS: playSoundsBatch() / playSoundsBatchFlat()
  ├─ audio.js renders via Emscripten render_modular_p (off-thread worker)
  │   and caches in its own renderCache
  │
  ╔══════════════════════════════════════════════════════════╗
  ║  Browser — desktop's computation on WebAudio + worklet   ║
  ║                                                          ║
  ║  played_voice.js buffer (gate / resample / fade-in)      ║
  ║    → AudioBufferSourceNode (rate 1, on a frame)          ║
  ║    → GainNode (headroom × note volume, exact)            ║
  ║    → Channel Ingress                                     ║
  ║    → beatmo-dsp 'inserts' (desktop's C effects)          ║
  ║    → beatmo-dsp 'eq' (desktop's C EQ program)            ║
  ║    → Channel Gain → Main (master volume)                 ║
  ║    → beatmo-dsp 'eq' → beatmo-dsp 'softclip'             ║
  ║    → masterOut → AudioContext.destination                ║
  ╚══════════════════════════════════════════════════════════╝
```

---

### Platform Divergence

| Component | Shared | Desktop | WASM/Browser |
|-----------|--------|---------|-------------|
| C synth rendering | Yes (CGo / Emscripten) | — | — |
| Voice cache + round-robin | — | `voice_cache.go` (`!js`) | `audio.js` `renderCache` (no round-robin) |
| modular_params (the only ABI param block) | Yes | — | — |
| Instrument registry | Yes | — | — |
| Mixer | — | Go 3-phase (`engine_stop.go`) | WebAudio BufferSource + per-note GainNode (`audio.js`, `played_voice.js`) |
| Insert effects | Yes — C `fx_host.c` | via CGo (`insert_fx_c.go`) | in the `beatmo-dsp` worklet (`dsp_worklet.js`) |
| EQ | Yes — Go `BuildEQProgram` + C `mix_dsp.c` | via CGo (`eq.go`) | in the `beatmo-dsp` worklet |
| Master soft clip | Yes — C `mix_dsp.c` | via CGo | in the `beatmo-dsp` worklet |
| Send effects / panning | — | none (deleted 2026-10-27; a room or an echo is a row's Reverb / Delay insert) | none |
| Output | — | oto player → system audio | `AudioContext.destination` |

---

### Key Architectural Decisions

**3-phase mixer** (`engine_stop.go`): Exists to prevent biquad filter state corruption. The old approach processed individual voices through shared biquad EQ chains. Biquad state (x1, x2, y1, y2) expects a continuous signal stream; interleaving voices from different instruments corrupted it, producing audible distortion. Fix: sum all voices per-instrument first (Phase 1), then route the coherent sum through EQ (Phase 2).

**Voice cache with round-robin** (`voice_cache.go`, desktop only): Avoids expensive C synth calls. Up to 3 variants per cache key (`RoundRobinVariants = 3`), selected by atomic counter — but the C renderers seed `ma_noise` with `0`, so renders are byte-identical, and the synchronous top-up that filled variants #2/#3 was removed on 2026-07-13 (it paid a 13-20 ms render on the audio dispatch goroutine for identical buffers; `variants_no_topup_test.go`). An entry therefore normally holds ONE buffer; per-variant seeding is reserved infrastructure (see the note at the top of `voice_cache.go`).

**Anti-pop wrapping** (`voice_antipop.go`): Every voice is wrapped at `Schedule()` time with 5ms linear fade-in and 20ms quadratic fade-out. Prevents clicks from abrupt start/stop. `mixer.Stop()` calls `RequestStop()` — voice self-removes after fade completes.

**Caller-owned C memory**: Go allocates delay/reverb/chorus buffers via `C.malloc()` and passes pointers to C init functions. Avoids GC/CGo interop issues.

**Deferred AudioBuffer** (WASM): Synth buffers pre-rendered at 48kHz without AudioContext, lazily wrapped in `AudioBuffer` on first play via `ensureAudioBuffer()`. Avoids premature context creation on mobile (suspended state).

**BlockProcessor optimization**: When all processors in a channel implement `BlockProcessor`, the channel uses ping-pong float32 buffers (pooled) with one CGo call per effect per block instead of per-sample.

---

### Instrument documents (2026-10-04 — replaced the recipe registry)

Every instrument is ONE **instrument document** (`instrument.Doc`,
`src/go/internal/instrument`) holding every value its synth voice depends on.
There is no recipe registry, defaults map, overlay, identity fill or JS param
map anywhere on the sound path (spec
`docs/superpowers/specs/2026-10-04-instrument-config-documents-design.md`).

```go
type Doc struct {
    ID, Origin, Name, Category, Notes string
    Render Render            // seconds, amplitude, gate_tail, melodic
    Stages map[string]Module // present only when used; {v, on, values}
    Source *Module           // the oscillator source (one source rule)
    Layers []Module          // gen-bank layers (slot capacity per family)
    FX     []Module          // voice FX chain
}
```

- **Module types** are declarations in `audio.BuiltinModuleRegistry()`
  (`module_registry.go`): each lists its values (with `Init` — the value a NEW
  module starts with, never an absent-field fill) and the param-block fields it
  writes. Adding a synth step is one declaration plus its DSP; no existing
  document has to change (a value added later reaches old documents through a
  `Migrate` step).
- **Pure functions** (`internal/instrument`): `Flatten(doc, reg)` → the param
  block (a zero block; each present ON module writes its fields),
  `FieldValues`/`OwnedFields` (the flat view), `SetField` (structural edits —
  source switch, layer source, enables — with typed errors), `Diff`,
  `BlockHash`, `ContentHash`, `Validate`, `Migrate`, `MarshalFile`/`ParseFile`.
- **Three stores** (`instrument_library.go`, `instrument_docs.go`): the
  embedded **pack** (shipped documents; readable sources in
  `internal/assets/instruments/<id>.json` + `index.json`, rebuilt by
  `make asset-pack`); the **user library** (Save / Save As, persisted through
  `SetInstrumentLibrarySink` → `userprefs.InstrumentStore`); the **project
  store** — each row's own copy with `Origin` = the library entry it came from.
  `LibraryDoc(id)` = user entry shadowing shipped; `EnsureInstrumentDoc(id)`
  installs a library copy on first use.
- **Edits**: `SetInstrumentParam(id, name, v)` / `SetInstrumentParams` /
  `GetInstrumentParams` / `ResetInstrumentParams` (`instrument_params.go`) are
  adapters over `SetField` on the project copy; `InstrumentParamDefs(id)` is the
  Synth tab's `[]ParamDef` derived from the document (Default = its value, range
  widened to contain it); `FlatFieldDeclared` is the tab's one visibility rule.
- **Native voice** (`doc_voice_native.go`): render(`Flatten`) → the document's
  FX → gate tail → normalise × `render.amplitude` → end fade; `render.seconds`
  long at every tempo; cached by `BlockHash` + pitch (melodic documents render
  AT pitch). **Browser**: every document change pushes its flattened block via
  `window.setInstrumentBlock(id, block, seconds, amp, melodic, gateTail, fx)`
  (`instrument_docs_wasm.go`); audio.js renders exactly that block and
  `voice_post.js` post-processes it like desktop (same C voice FX, gate tail,
  float32 normalise, end fade).
- Projects carry each row's document in `instrument_docs`; instrument bundles
  carry it in `BundleInstrument.Synth`. Both are migrated + validated before
  anything is installed.

**Param block ABI (one table, no engine defaults).** `modularFields`
(`internal/audio/modular_fields.go`) lists every field the engine reads —
`{Block, Name, Slots, Init, Doc}` — and everything else is generated or
derived from it:

- `cmd/gen-synth-abi -c -o src/c/modular_params.gen.h` → the C struct (a dense
  array of doubles, `_Static_assert`ed), `MODULAR_PARAM_COUNT` and one
  `MODULAR_<BLOCK>_SLOTS` capacity per family block; `-o src/js/synth_param_abi.gen.js`
  → the JS layout. `make gen-synth-abi` runs both (built under `-tags test`, no
  cgo, so a broken header can still be regenerated) and every WASM build runs it.
  Drift guard: `TestGeneratedCHeaderIsCurrent`; layout pin:
  `TestModularFieldsReproduceTheShippedLayout` (`testdata/modular_schema_layout.json`,
  re-freeze only for a deliberate reshape).
- Go builds a block ONLY through `instrument.Flatten` (a field no present module
  writes is 0); `Init` seeds a newly created module, nothing else.
- C reads each field directly. `render_modular_p` is the only entry point; NULL
  params render silence; there is no NaN/`kp_get`/"0 means default" path, so
  **0 is a real value** (`TestZeroIsARealValue`, `TestNoFallbackHelpersRemainInC`).
  What used to be a per-variant C literal is spelled in the document.
- Slot capacity per family block: bass / KS / kick / tom / snare / FM = 1,
  cymbal + cymbal-modal = 3, the generic gen bank = 12. A family voice in a slot
  beyond its capacity renders silence (`TestFamilyVoiceBeyondCapacityRendersSilence`);
  `validateSlotCapacity` rejects such params on the Go side.
- POST stage: `modular_finish` (`modular.c`); the ops always run (neutral values =
  no-op; `out_clamp` 0 = no limit) and are the
  Synth tab's **FINISH** stage
  (`audio.FinishGroup`: Resample, Length, Fade Speed, Low Boost, Crisp, Darken,
  Crunch, Limit).
- Writes are validated: `ValidateRecipeParams` (unknown name, non-finite value,
  slot capacity) gates project import and bundle import (every doc before any
  registers); `SetInstrumentParam(s)` refuses an unknown name or a family voice
  past its slot capacity; an invalid user-library document is skipped with a
  WARN.
- The LINKED engine must match the layout: C exports `modular_param_count()`;
  Go panics at init on a mismatch (a `build/libdrums.a` built from an older
  header) and `audio.js` refuses a stale `drums.single.js` (`checkEngineABI`).
  `C_HDRS` lists the generated header, so `make` rebuilds the objects.
- A render that FAILS in the browser throws loudly; the instrument's previous
  (param-stale) buffer keeps playing. There is no substitute voice.

**Adding a param**: one row in `modularFields`, `make gen-synth-abi`, read it in
C, give it a range in `modular_recipe.go`, then `make build/libdrums.a` and
re-freeze the layout pin.

**UI surface**: a dedicated "Synth" tab inside the EQ panel, alongside
EQ / Wave / Spectrum / Levels / Chain. The tab is implemented in
`synth_panel_zone.go`; layout/draw/hit-areas are delegated from
EQPanelZone via `EQCallbacks.OnSynthTabLayout / DrawSynthTab /
SynthTabHitAreas`. Sliders are one per ParamDef; drag → audio.SetInstrumentParam.

**Adding a new built-in instrument.** Write its document as
`internal/assets/instruments/<id>.json` (copy the nearest neighbour and change
values — a family variant such as a new snare / cymbal / FM flavor is the same
document with different `gen1_<family>_*` values), add the id to `index.json`,
then `make asset-pack`, `go generate ./internal/audio/` (the `roster` constants),
`make measure-loudness` (writes `render.amplitude`) and `make gen-fundamentals`.
No Go or C change. What is per-family is only the C voice (`modular_stages.c`),
which reads every value straight from the param block (see § Param block ABI).
Deleting an instrument: remove its document and its `index.json` line. At
runtime the user's own instruments come from Synth-tab **Save As** (a
content-addressed user-library id, `NewUserInstrumentID`).

**Cymbals are modal plates now (2026-09-28).** The ride and all four hi-hats run
`gen_cymbal_modal` (`modular_stages.c`): hundreds of inharmonic doublet modes with a
strike spectral envelope and frequency-dependent damping, plus a collision "rattle"
train for the hi-hats — no noise generator. The hi-hat (variant 0) and ride (4) ARE
the plate, always (Plate Modes clamps to ≥ 1; their old fixed-partial voices were
deleted 2026-10-02); the open hi-hat (1) and crash (5) run it once `gen1_cym_modes`
≥ 1 and their fixed-partial voices otherwise; the cowbell and shaker never do. The same plate, DRIVEN
(`cym_drive_delay`), is the snare WIRES of the organic snares (`organicSnareWires`). The
model, its 20 knobs, how
the seeds were fitted to reference recordings, and the full add-a-knob checklist are in
`AGENTS.synth.instruments.md` § Cymbals as modal plates.

**Plugin path (v2+, deferred)**: the v1 interface is the freezing
surface — `SynthRecipe` signature, `ParamDef` JSON shape, `RecipeParams`
semantics, `Render` ABI parity with C, `paramsHash` determinism, and the
`platformInstrumentParamsChanged` IPC seam. All six are pinned in
`synth_recipe_invariants_test.go`; changing any of them is a coordinated
v2 break, not a drive-by edit. A future `WASMSynthRecipe` would
implement the same interface against a wazero (desktop) /
AudioWorklet (browser) loader.

---

### WAV-clone stages: MODAL / STRIKE / NOISEBAND and tables in documents (2026-10-13)

A measured modal model (spec `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md`)
lives in three STAGES rendered by `src/c/modal_bank.c` right after the gen bank
(`render_modular_p`), adding into the mix pre-envelope/filter. Each is an exact
no-op while disabled (the default), so no shipped sound moves.

- **Tables in documents.** A stage type may declare a `TableDecl` (columns with
  physical bounds, a row capacity, a count field); a document's `Stage.Table`
  (`{"cols": [...], "rows": [[...]]}`) flattens to dense ABI arrays
  (`modal_freq[0..127]` …) plus the row count. `instrument.SetTable` is the
  only writer; `Validate` checks columns, capacity, finiteness and bounds;
  `Clone`/`Diff`/`ContentHash` cover tables, and a knob edit keeps the table
  (`withModule`). Table cells and counts are `modularFields` rows with
  `Rows`/`Derived` and get NO ParamDef — rows are never knobs
  (`IsTableField`). The JS ABI lists them as `MODULAR_PARAM_TABLES`
  (`[first index, rows]`), not in `MODULAR_PARAM_INDEX`. The data rule —
  ≤ 1,024 table values per document, every column physically bounded — is
  `TestTablesAreModelParametersOnly`: a document can never carry audio.
- **MODAL** (≤ 128 rows: `freq, decay, amp, phase, attack`): phasor
  resonators x = A e^{−αt} cos(2πft + φ) (the `clone.Mode` convention), f =
  row freq × voice frequency (`modal_freq_mode` 0) or Hz (1), then tune and
  stretch; α = row decay × `modal_decay_x` × (a·(f/1k)^p + b·(f/10k)^6) at the
  ACTUAL frequency, so a transposed note rings shorter by the fitted law with
  no stored reference pitch; `modal_keep` plays the first N rows (rows are in
  priority order); modes ≥ min(20 kHz, 0.45·sr) are skipped; each mode stops
  100 dB under the bank's analytic peak bound Σ|A|. 128 modes × 3 s ≈ 9 ms.
- **STRIKE** reweights the modal amplitudes by the contact spectrum's smooth
  envelope S(f; `strike_tc`) = 1/√(1 + (π (f·tc)³)²) (the raised cosine's
  corner and −18 dB/oct asymptote without its nulls — the nulls made the
  clone's inverse ill-posed) and a `strike_hardness` tilt (dB/oct re 1 kHz) at
  each mode's actual frequency — identity at tc 0 / hardness 0
  (`TestStrikeAtIdentityIsBitIdentical`); `strike_click` adds a seeded noise
  burst of length max(tc, 1 ms).
- **NOISEBAND** (≤ 32 rows: `center, q, level, attack, fast_rate, fast_mix,
  tail_rate`): one seeded white source through per-row RBJ band-passes
  (`mod_biquad`), each scaled so the row's RMS IS its `level` dB (noise
  bandwidth π·fc/(2Q)), with a raised-cosine attack then a two-rate decay.
  Each row stops 100 dB under its own level.
- **Synth tab**: MODAL (Tune, Damping, Brightness, Partials; More: Stretch and
  the decay law), STRIKE (Contact, Hardness, Click) and NOISE (Noise Level)
  stages after OSC, each with its pill; root categories **Ring** and
  **Mallet** draw the mode spectrum (`synth_modes_graph.go`: stems on a log
  axis, faded by ring time, the noise rows as faint bands behind), which is
  also the focus graph of every knob of the three groups. The stages are
  TABLE-GATED (`tableGatedStages`): a document with no rows shows none of
  them — `InstrumentParamDefs` hides their knobs and `FlatFieldDeclared`
  refuses them — because knobs over an empty table act on silence and rows
  never come from the Synth tab. `modal_freq_mode` stays hidden (how the rows
  are written, not a sound). Scenes `crop_synth_modal_root` /
  `crop_synth_modal_graph` install a GENERATED clone-shaped document
  (`synth_modal_demo.go`) on tubular bells.
- `audio.ModalModes` is the Go mirror of the bank (the stem graph and
  `clone.DocModes` both read it), pinned to the C render with every macro off
  identity (`TestModalModesMatchTheEngine`). The three types are listed in
  `dead_module_values.txt` (no shipped clone yet) and their liveness is
  `TestModalStagesAreLive`. Bank vs the closed-form mode model:
  `TestModalBankMatchesTheModeModel` (≤ 1e-5).

### Tuning a Melodic Instrument from a Reference WAV

The reliable way to make a synth instrument sound like a real one is **analysis-driven A/B matching**, not ear-guessing: render the synth's own note, FFT it next to the real recording, and close the measured gaps one metric at a time. This is how flute, french-horn, trumpet, and oboe were tuned. Per-instrument recipes + the synthesis-structure decision table live in [`AGENTS.synth.instruments.md`](AGENTS.synth.instruments.md) § Wind & brass synthesis; this is the **general process**.

**The loop:**
1. **Decode the reference** to mono 44.1 kHz: `ffmpeg -i ref.wav -ac 1 -ar 44100 -sample_fmt s16 /tmp/real.wav`. (Reference WAVs that are NOT public-domain stay LOCAL — never commit them; only PD/CC0 may be committed.)
2. **Measure the reference** (see metrics below). **Trust the audio, not the metadata** — the trumpet file's stated note was wrong on two counts; the FFT settled it.
3. **Render the synth's same note** with the prod CGo path: `cd src/go && go run ./cmd/synth-analyze synth -inst <id> -pitch <P> -dur 3 -out /tmp/synth.wav`, where **`-pitch P = MIDI − 57`** (A3 = 220 Hz = pitch 0; fractional P is fine).
   - 🚨 **PITCH-MATCH GATE — the A/B is MEANINGLESS unless the synth and reference play the SAME note.** Derive `-pitch` from the reference's **MEASURED fundamental** (`P = 12·log2(f0_ref / 220)`), NEVER from the filename. A file named `celtic_harp_g2` actually had its fundamental at **776 Hz (G5)**, not G2 (98 Hz) — rendering at G2 compared two unrelated tones (and produced a bogus 0.64 "noise" reading); at the real pitch the synth matched the ref to F0 773≈776, centroid 1878≈1816, noise 0.01=0.01. The F0 DETECTOR also picks a harmonic on low/bright notes — find the TRUE lowest fundamental via an FFT peak scan, then **assert `f0_synth ≈ f0_ref` within a few cents (not octaves) BEFORE reading any other metric.** If they differ by an octave, you're comparing different sounds. Always stage the comparison WAVs at the same pitch.
4. **FFT both** (numpy via `uv run --with numpy python3`), print a **side-by-side H1–H16 dB table + the scalar metrics**, and read off the Δ.
5. **Adjust the document** (`internal/assets/instruments/<id>.json`, then `make asset-pack`) toward the gaps, re-render, re-measure. Iterate until convergence.
6. **Verify + persist:** `gofmt` + `go build ./...`, render the instrument's template (`make song-render TEMPLATE=<t>`), run the audio goldens (`go test -count=1 ./internal/audio/`), then record what worked in `AGENTS.synth.instruments.md` + memory. Seed-only edits don't touch C; **any `src/c/*` edit needs `make build/libdrums.a` first** (`go test` links the static lib, it does NOT recompile C). The Makefile rule uses **per-object rules + `go clean -cache`** so a C edit reliably rebuilds AND relinks (Go caches the cgo link keyed on the LDFLAGS *string*, not the `.a`'s content — without the cache-clear, C edits silently ship stale). In fast SCRIPTED edit→build→measure loops the sub-second mtime can still race; force it with `rm build/<obj>.o build/libdrums.a` (real human edits are seconds apart and unaffected). Tell-tale of a stale build: a C change has byte-identical output — verify with a stderr `fprintf` in the render loop.

**The DSP metrics that matter (measure all; match in this priority order):**

| Metric | What it tells you | How to match |
|---|---|---|
| **f0 / pitch** (cents from target) | In tune? | First gate — must be within a few cents. Measure from the WAV; set `-pitch = MIDI−57`. |
| ⚠️ **f0 measurement** | The synth renders at A=440, references are often A=442 (≈8¢ sharp) — and a wrong f0 makes you read BETWEEN harmonics and see phantom "weak harmonics" | **FIT f0 per-signal** (argmax in a ±6% band) before reading the harmonic table; never reuse the reference's f0 to measure the synth. Vibrato also SMEARS harmonics in a long FFT window — measure harmonics in a SHORT (~0.05 s) window or with vibrato off. |
| **Harmonic envelope** H1–H16 (dB rel H1) | THE fingerprint — *where is the energy?* | Primary target. Decides the whole synthesis structure (below). Match peaks within ~1–3 dB mean \|Δ\|. |
| **Spectral centroid** (Hz) | Overall brightness | The perceptual "match" lever. Close via filter cutoff / formant level / the noise band. |
| **Band energy** (e.g. <500, 500–1k, 1–2k, 2–4k, 4k+ as dB-of-total) | Is the low end boomy / is it fundamental-heavy? A high note (e.g. D5=590 Hz) whose FUNDAMENTAL is the loudest component reads "deep/echoey/boomy" | A **band-pass** main filter (`filter_type 2`, RBJ; `1`=HP, `0`=LP) kills the boomy fundamental/sub-bass AND tames the harsh top in one biquad; high-pass any noise gens (`gen_filt_type 4`) so they add air not rumble. Check band-energy whenever "too much bass" / "muddy". |
| **Spectral flatness** (0–1) | Tonal (low) vs noisy/airy (high) | Reeds/bows/breath need broadband noise — see the "add noise" rule below. |
| **Attack time** (ms to −6 dB) | Onset speed | `amp_attack` (+ `filtenv` bloom for brass "brassiness"). |
| **Decay / ring time** (ms peak→−20 dB) | For PLUCKED/struck sounds — how long it rings | `gen_ks_sustain` (KS loop gain) + `amp_decay`. A too-short ring = "plinky/mandolin"; this is usually the #1 fix for guitars. |
| **Sustain micro-variation** (amp std/mean over the sustain; ~0.05–0.15 real, ~0.009 dead) | Is it ALIVE or a static drone? A frozen, unchanging sustain is THE "synth/sci-fi" tell | Real jitter+shimmer: a **physical-model waveguide** (`gen_source:3`, inherently non-repeating) is the only source here with built-in life; else add vibrato + an attack transient. NOT a resonant filter sweep (that reads as a sci-fi laser). |
| **Amplitude-envelope SHAPE over the whole note** (plot the RMS at 50 ms; compare the curve, not just its std/mean) | ⚠️ std/mean MAGNITUDE can MATCH while the CHARACTER is totally wrong — a real bowed note is a slow swell→sustain→release; an over-deep random modulator gives fast CHAOTIC swinging with the SAME std/mean but reads as warble/"weird/echoey/organ" (the quiet dips also darken→sound bassy) | Plot the envelope and eyeball it. A modulator (e.g. `bow_dynamics`) must be GENTLE (±~20 %, slow) — it adds TEXTURE on top of the stroke shape; it is NOT the main variation. The stroke shape comes from the amp ADSR + note duration. |
| **Sustain ripple** (dB) | Vibrato/tremolo depth | `lfo_depth` (+ `lfo_target`; use `0`/amplitude for unison instruments — a pitch LFO collapses the unison ensemble). |
| **Inter-harmonic noise floor** | Hiss / "lo-fi radio" | Reduce breath-gen level + lower cutoff; a pitch-relative HP kills sub-fundamental rumble. |
| **Autocorrelation** | Pitch stability | Sanity-check the synth isn't warbling / self-oscillating off-pitch. |

**Pick the synthesis structure by where the energy is + whether it sustains or decays** (then see the instruments-doc decision table for the exact recipe):
- **Strong sustained fundamental, smooth monotonic decline** → subtractive (saw + low-pass), or a **blown waveguide** for air-reed winds (flute).
- **Weak fundamental, or non-monotonic formants** (a bump/plateau a saw's monotonic roll-off can't make) → **additive** (place each partial at the measured dB), or a **formant-bank** (per-gen-filtered saws) when one region is a distinct high formant. Horn = additive (low formant plateau); trumpet = formant-bank (high brilliance formant); oboe = additive (weak fundamental + twin mid formants).
- **A decaying transient** (plucked/struck — guitar, harp) → **Karplus-Strong** (`gen_source:3`). Analyse it **time-varying** (EARLY ~0.1 s vs MID ~0.8 s windows show the upper harmonics decaying faster than the fundamental) and treat **ring/decay time** as a first-class target. Caveat: the KS excitation has no pluck-*position* control, so you can't null a specific harmonic — approximate the timbre with a resonant body LP + a short attack "chiff" noise.

**Two rules learned the hard way:**
- **After the harmonic peaks match, you are not done — add broadband noise to hit the real flatness/centroid.** A pure harmonic stack measures flatness ≈ 0.001 and reads as a "sci-fi synth"; real reeds/brass carry reed/breath noise (flatness ~0.05–0.08). A single band-passed noise gen restores both flatness *and* centroid. (This was the oboe fix.)
- **Keep an additive bank's sum under the FINISH Limit before placing partials by measurement.** `out_clamp` (1.5) is a hard clip, and a bank whose gains sum past it (the trombone 4.9, the tuba 4.1, the oboe 2.7) is clipped there: the clip writes intermodulation harmonics, holds the boosted partials down (the old "upper partials render ~10 dB under their gain") and makes an added partial cancel the harmonic the clip already wrote. Halve the gains (the voice is peak-normalised) to place partials; brighten through drive or the clip on purpose. Details: `AGENTS.synth.instruments.md` § Signal-chain limits.

**⚠️ A spectral match is NOT a perceptual match.** Hitting the harmonic envelope to 1–3 dB can STILL sound wrong — the trumpet matched to 3.9 dB and sounded like a "synth violin." Two things the steady spectrum doesn't capture, both decisive: **(1) one voice vs many** — `unison_voices > 1` turns any *solo* instrument into a section/synth-string pad (the #1 cause of "this sounds like synth strings"); keep solo brass/winds to ONE voice. **(2) dynamics** — a filter-envelope bloom (the brass "brassiness"), the attack transient, pitch/amplitude motion; a static chorused tone reads as synthetic no matter how good the spectrum. When a tuned patch still sounds wrong, suspect **unison + missing dynamics before re-tuning gains.**

**⚠️ Analyse the WHOLE signal over TIME, not a single steady window (the trumpet breakthrough).** A note's IDENTITY often lives in its time-evolution, which a mid-sustain FFT throws away. Measure trajectories, not just one snapshot: **the amplitude envelope** (sharp onset? slow crescendo/swell?), **per-harmonic dB trajectories** (do the upper harmonics BLOOM in over the note — "brightness tracks loudness" — or stay put?), **the centroid over time** (dark→bright bloom?), **attack noise fraction over time** (real winds/brass/bows are very noisy at onset, ~0.5–0.8, then clean to ~0.02), and **whether vibrato is even present** (the real trumpet is a STRAIGHT tone — a wrong vibrato is a huge synth tell). The trumpet only stopped sounding synthetic once these were matched: a 500 ms crescendo + a slow brightening bloom (added the `filtenv_attack` rising-filter param for exactly this) + a noisy decaying attack + no vibrato. Build the side-by-side trajectory table FIRST; it tells you which dynamic feature is missing.

**Convergence targets:** pitch within a few cents; harmonic mean \|Δ\| ≤ ~1–3 dB; centroid within ~10–15 %; flatness in the same ballpark. Add **an attack transient** for life, and **vibrato only when the reference has it** (measure the f0 track first: the Iowa cello, trombone, tuba and clarinet notes are straight, and the cello's 4 Hz LFO was heard as "vibrating too mechanically"); unison ONLY for genuine sections/ensembles, never a solo voice (the tuba shipped with two voices and was heard as "wavy"). Final judge is always the ear — verify structure (voice count, dynamics) before chasing the last dB.

---

### Digital-Waveguide Physical Models (the real-instrument tier)

For SUSTAINED acoustic instruments, additive/subtractive recipes top out at "good synth"; a **self-oscillating digital waveguide** is what finally sounds like a real instrument (the user reaction went from "electronic hum" to "amazing"). These live in `src/c/modular.c` as `osc_type` branches (each a delay-line resonator + a family-specific nonlinear exciter):

| `osc_type` | Model | Used by | State |
|---|---|---|---|
| 7 | **Bowed string** (STK Bowed: 2 velocity delay lines + stick-slip friction table) | violin, viola, cello, double-bass | ✅ shipped, "amazing" |
| 8 | **Brass** (lip BiQuad + squared nonlinearity + tube) | — | ⚠️ chaotic, deferred (trumpet stays additive) |
| 9 | **Reed** (STK Clarinet: bore delay + reed table + bell loss filter) | — (the oboe moved to an ADDITIVE seed; the waveguide read as a clarinet) | self-oscillates (finite-tested), not used by a seed |
| 10 | **Air-jet flute** (bore+jet delays + cubic jet) | — | ⚠️ oscillates only in a narrow short-bore regime (mistunes ~1.47×); flute uses KS blown-tube instead |
| 11 | **Saxofony reed-cone** (STK Saxofony: inverting reed loop + TWO delay lines split at a blow position → all harmonics) | — (the sax is a hybrid additive + formant-filtered saw seed) | stable, all-harmonic; not used by a seed |

**As of 2026-10-19 the four bowed strings are the ONLY shipped documents on a
waveguide source.** Every wind and brass instrument is additive (`gen.wavetable`
partials + a `gen.noise` layer), and `osc.brass` / `osc.reed` / `osc.flute`
declare **no values** in `module_decls.go` — their breath, reed and lip
constants are hard-coded in `render_brass` / `render_reed` / `render_flute`, so
a document can select them but cannot tune them (only `osc.bowed` and `osc.sax`
expose knobs). Each one needs exposed values before tuning or a matcher can
reach it. The essence map is consistent with what that costs: brass reads
**darker** than every recording of it and winds **noisier**, and a fixed partial
table cannot brighten with loudness or keep a formant fixed across the range.

**Body resonators** (`body_model`, a parallel bank of band-pass biquads the string/bore feeds through): 1=violin, 2=classical/nylon guitar (low-Q warm upper modes), 3=cello, **4=steel guitar (high-Q RINGING upper modes = the metallic zing)**. Nylon vs steel is the SAME box with different upper-mode Q — a high-Q upper resonance reads as bright steel, low-Q as warm nylon.

**Self-oscillation debugging — a waveguide that renders SILENT** is almost never a build/dispatch bug; it's the loop sitting at a stable fixed point below threshold:
1. **Loop gain must exceed 1** at the playing frequency. For the reed, small-signal gain = `bell_refl·(offset + 2·slope·Δp)`; at rest Δp ≈ −breath, so **RAISING BREATH crosses the threshold** (the clarinet was silent at breath 0.55/gain 0.98, oscillated at 0.8/gain 1.12).
2. **Add excitation noise** (`breath += breath·0.2·white`) — it breaks the static fixed point so the loop kicks into its limit cycle.
3. **Keep the DC block OUT of the loop** for reeds/flutes (the breath DC is the drive); brass DOES need an in-loop DC block (its squared nonlinearity rectifies).
4. **Brass goes CHAOTIC not silent = too much gain** (lip Q·gain saturates → noise); lower `lip_gain`.
5. **Slow onset / initial quiet-or-different gap** (a waveguide whose loop gain is barely >1 builds its limit cycle SLOWLY — ~2 s from silence — so the first chunk of the render is quiet and timbrally unsettled): two-part fix — **(a) SEED the delay lines with energy** (e.g. `buf[k] = 0.8·white()` instead of 0) so the amplitude is high enough for the reed/nonlinearity to CLAMP into its limit cycle immediately; **(b) PRE-WARM** — run the loop SILENTLY for ~0.5 s and only write `out[i-warm]` once it's established, so `out[0]` is the full steady tone. Keep the instrument's `amp_attack` short (~8 ms) so the amp ADSR doesn't re-fade it. (This is how the baritone sax onset gap was removed.)
6. Confirm reached-not-silent with a **sine probe** (`out[i]=0.4*sin(2πft)` inside the render fn) before blaming the DSP.

Verified by `internal/audio/synthmatch/physical_models_test.go` (`//go:build !test`, native CGo): every shipped physical instrument must render finite/bounded/stable/on-pitch, and every `osc_type` 7–11 must be finite/non-runaway (`TestWaveguideOscTypesAreFinite`) (the crash-safety guarantee — the completeness test renders `osc_type` at its max).

---

### Satellite Files

| File | When to Load | Contents |
|------|-------------|----------|
| [`AGENTS.synth.instruments.md`](AGENTS.synth.instruments.md) | Creating or modifying instrument sounds | All drum + FM recipes, synthesis principles, DSP primitives, parameter reference, key learnings, **analysis-driven tuning (`synth-match` vs isolated MIS samples) + confidence-ranked orchestral baselines**, **wind/brass/reed synthesis structures + the per-instrument flute/horn/trumpet/oboe recipes tuned to reference WAVs** (the general process lives in § Tuning a Melodic Instrument from a Reference WAV above) |
| [`AGENTS.synth.effects.md`](AGENTS.synth.effects.md) | Working on insert effects or the effect chain | All 18 insert effects (params, signal flow, memory), effect chain architecture, browser WebAudio graph |
| [`AGENTS.synth.pipeline.md`](AGENTS.synth.pipeline.md) | Working on the mixer, voice system, or channel processing | 3-phase mixer detail, voice lifecycle + wrappers, voice cache, thread safety model, buffer sizes |
| [`AGENTS.synth.wasm.md`](AGENTS.synth.wasm.md) | Working on browser audio, WebAudio, or WASM bridge | WebAudio node graph, AudioWorklet, deferred AudioBuffer, audio scheduling, mobile unlock, Go↔JS bridge |
| [`AGENTS.synth.debug.md`](AGENTS.synth.debug.md) | Diagnosing audio issues (volume, crackling, clicks, silence) | Diagnostic env vars, decision-tree flowcharts, browser diagnostics, observability tap points |

---

### Key Source Files

| Area | Files |
|------|-------|
| C synthesis | `src/c/modular.c`, `modular.h`, `modular_stages.c`, `modular_stages.h` (the gen-bank engine — hosts the migrated drum + FM DSP), `fmsynth.c`, `fmsynth.h` (`fm_render` core only), `drums.c`, `drums.h` (`load_audio` + shared primitives), `modular_params.gen.h` (GENERATED param struct + slot capacities — never hand-edit) |
| C DSP primitives | `src/c/wavetable.c/.h`, `adsr.c/.h` (no standalone `lfo.c` — LFO is inline in `modular.c` (vibrato/tremolo/filter-sweep) and `insert_fx.c` (autowah)) |
| C insert effects | `src/c/insert_fx.c`, `insert_fx.h` |
| Go audio engine | `src/go/internal/audio/engine.go`, `engine_stop.go`, `engine_mixer.go`, `engine_play.go` |
| Go voice system | `voice_cache.go`, `voice_antipop.go`, `drums_c.go`, `engine_resample.go`, `doc_voice_native.go` (every instrument's voice: render(`Flatten(doc)`) → FX → gate → amplitude) |
| Go channels | `channels.go`, `biquad.go`, `eq.go`, `eq_program.go`, `mix_dsp_c.go` |
| Go effects | `insert_fx_c.go`, `effect_chain.go`, `insert_effects.go` |
| Go params | `modular_fields.go` (THE ABI table), `synth_param_schema.go` (schema derived from it), `slot_capacity.go`, `param_validation.go` (`ValidateRecipeParams`), `module_registry.go` (module type declarations) |
| Go instruments | `engine_instruments.go` (no `node_types.go` in `internal/audio`; NodeType events live in `internal/hooks/events.go`) |
| Go instrument documents | `internal/instrument/` (Doc + pure functions), `instrument_docs.go` (project store), `instrument_library.go` (pack + user library), `instrument_params.go` / `instrument_param_defs.go` (Synth-tab adapters), `synth_recipe_dispatch.go` (voice dispatch), `modular_recipe.go` (`ModularSynthParamDefs` knob metadata) |
| Instrument values + tuning | `internal/assets/instruments/*.json` (the shipped documents), `internal/audio/synthmatch/` (Hooke-Jeeves `synth-match` optimizer) |
| Genre / showcase templates | `src/go/cmd/gen-showcase/{main.go,template_specs.go,score_import.go}` (the declarative spec table + the score→circuit pipeline), reference scores `internal/scoreref/`, drift guard `cmd/gen-showcase/drift_test.go`, fidelity guards `cmd/gen-showcase/fidelity_test.go`, output JSON `internal/assets/templates/*.json`. Regenerate with `make templates` (or `go run ./cmd/gen-showcase` from `src/go`). Every template is derived from a committed public-domain score — there is no hand-authored note. See `docs/score-sources.md`. |
| C FINISH stage | `modular_finish` in `src/c/modular.c` — the always-on post ops, a pure function of the `post_*` fields (`synth_post.h` now only declares the shared osc/softsat primitives) |
| UI Synth tab | `src/go/internal/ui/synth_panel_zone.go` |
| JS audio | `src/js/audio.js` |
| WASM bridges | `engine_wasm.go`, `insert_effects_wasm.go`, `channels_wasm.go`, `instrument_docs_wasm.go` (document → `setInstrumentBlock`), `synth_recipe_wasm.go` (Sampler-edit push) |
| JS exports — synth | `src/go/internal/ui/js_exports_synth_recipe.go`, `src/js/wasm_bridge_smoke.browser.test.js` (synth_recipe section) |
| Discipline tests | `synth_recipe_invariants_test.go`, `module_liveness_test.go` (`TestDeclaredModuleValuesAreLive`), `instrument_library_test.go`, `c_header_parity_test.go` |
