## Synth Pipeline: Browser / WASM Audio

Reference for the browser audio pipeline. For the desktop pipeline, see `AGENTS.synth.pipeline.md`.

---

### Architecture Overview

Desktop and WASM share the C synth (the modular engine, `render_modular_p`) and the Go instrument registry + param state. They diverge at voice caching and mixing: the Go voice cache (`voice_cache.go`) is desktop-only (`!test && !js`; a no-op stub in the browser build) — in the browser `audio.js` renders and caches each instrument in its own `renderCache`.

| Component | Desktop | WASM |
|-----------|---------|------|
| Voice rendering | C synth via CGo | C synth via Emscripten WASM |
| Played voice (pitch, gate, fade-in) | Go voice wrappers (`engine_play.go` …) | `played_voice.js` — the same arithmetic, pinned per sample |
| Mixing (sum, volume, start sample, stops) | Go 3-phase mixer (`engine_stop.go`) | WebAudio BufferSource + one GainNode per note (`audio.js`) |
| Insert effects | C (`fx_host.c`) via CGo | the same C in the `beatmo-dsp` worklet |
| EQ | C program runner (`mix_dsp.c`) over Go's `BuildEQProgram` via CGo | the same C + the same program in the worklet |
| Master | volume → EQ → C soft clip | volume → EQ → the same C soft clip in the worklet |
| Sample rate | 48 kHz (`EngineSampleRate`) | `AudioContext({sampleRate: 48000})` |
| Output | oto player → system audio | `AudioContext.destination` |

**Key**: On WASM, Go calls `playSoundsBatch()` / `playSoundsBatchFlat()` via `syscall/js`. JavaScript handles all mixing, effects, and output. Go-side mixer code is not used.

**Files**: `src/go/internal/audio/engine_wasm.go`, `insert_effects_wasm.go`, `channels_wasm.go`, `src/js/audio.js` (~5500 lines)

---

### After the voice: desktop's C on the audio thread (2026-10-27)

Every stage between a voice and the speaker is the SAME computation as on
desktop (spec `docs/superpowers/specs/2026-10-27-one-audio-pipeline-design.md`):

```
note:    played_voice.js buffer (gate / resample / fade-in baked, desktop's arithmetic)
           → BufferSource (rate 1, start on a frame) → GainNode (headroom × volume)
channel: ingress → [synthAnalyser] → beatmo-dsp 'inserts' → [preEQAnalyser]
           → beatmo-dsp 'eq' → [analyser] → gain → main
master:  main gain (master volume) → [analysers] → beatmo-dsp 'eq' → [analyser]
           → beatmo-dsp 'softclip' → masterOut → destination
```

- `dsp_worklet.js` loads `drums.single.js` once per AudioWorkletGlobalScope (one
  WASM instance per AudioContext, shared by every node — the per-node instance
  that sank an earlier attempt is not needed). Readiness is a message (a 'ready'
  probe node); top-level await does NOT delay `addModule` in any engine.
- `dsp_host.js` is the stage code: effects through `fx_host.c` (`ifx_create` with
  Go's complete params, ordered by `ifx_param_names`), the EQ program
  (Go's `BuildEQProgram`) through `mix_dsp.c`, and the soft clip. The worklet,
  the main-thread ScriptProcessor fallback (no AudioWorklet: insecure origins)
  and the voice render (voice FX) all run it.
- The AudioContext runs at `CHAIN_SPEC.sampleRate` (48 kHz, desktop's rate);
  `getSampleRate()` (every voice render) is that rate too.
- Notes wait for the DSP host in the first moments of a context
  (`processAudioEvent`), so nothing plays unprocessed.
- Edits are messages: a param retunes one C instance (state kept), a chain edit
  reconciles instances by uid, an EQ edit of the same shape swaps coefficients
  and keeps filter state. The graph is rewired only when a channel gains its
  first fx / eq node (`window.audioGraphStats()`).
- **Proof**: `xplat_audio_stages.browser.test.js` — every stage and whole
  circuits against desktop, per sample, in Chromium, Firefox and WebKit
  (`window.__offlineRenderForTest` renders audio.js's real graph on an
  OfflineAudioContext); `chain_spec_parity.browser.test.js` — the live wiring.

---

### Deferred AudioBuffer Pattern

Two-phase rendering avoids premature AudioContext creation on mobile (which would suspend):

**Phase A — CPU rendering (no AudioContext needed)**:
1. Get sample rate (default 48kHz if no context yet)
2. Take the param block: a copy of the instrument's block Go pushed with `setInstrumentBlock(id, block, seconds, amp, melodic, gateTail, fx)` (its document, flattened, plus its render settings; `frames = floor(sr × seconds)` — a fixed time, never beats); a melodic render patches only the copy's `pitch` field. No block = no voice.
3. Render `render_modular_p(ptr, sampleRate, frames, params)` (every `RENDER` entry is `render_modular_p`; the Emscripten module exports only `_render_modular_p` + the effects) — normally OFF-THREAD in `synth_render_worker.js` (`renderVoiceInWorker`), with a synchronous main-thread fallback (`renderVoiceSync`); a failed render throws and the previous buffer keeps playing (no substitute voice)
4. `voice_post.js` (in the worker and the sync fallback alike): the document's voice FX through the same C `ifx_*` as desktop, gate tail, peak-normalise × amplitude (float32), end fade — desktop's `renderDocBlock` step for step; then the optional sample-edit
5. Cache as `{ buffer: null, data, sr, frames, gen }`

⚠️ **No block, no voice.** An id Go has not pushed (a bare audio.js page, or a
render requested before Go's init pushed every document) renders nothing — there is no
bare-voice fallback. A test on a bare page must install real blocks first (read them
from the app with `window.__instrumentBlockForTest(id)` and call `setInstrumentBlock`;
see `audio_lane_cancel.browser.test.js`).

**Phase B — Lazy AudioBuffer creation (first playback)**:
1. Create AudioContext (triggered by user gesture)
2. `ctx.createBuffer()` with cached data
3. Copy Float32Array to buffer channel
4. Store AudioBuffer in cache for reuse
5. WebAudio handles resampling if `buffer.sampleRate !== ctx.sampleRate`

**File**: `audio.js` `ensureRenderedSample()` (≈ line 3880), `synth_render_worker.js`

---

### Audio Scheduling

Queue-based event processing to avoid blocking the main thread:

| Property | Value |
|----------|-------|
| Queue max size | 8192 (`AUDIO_QUEUE_MAX`; the queue itself is `audioQueue`) |
| Flush chunk size | 64 events per frame (`AUDIO_FLUSH_CHUNK`) |
| Flush time budget | 1.2ms (`AUDIO_FLUSH_BUDGET_MS`) |
| Min look-ahead | 4ms (`AUDIO_MIN_LEAD_SEC`) |
| Small lead threshold | 3ms (`SMALL_LEAD_SEC`) — triggers warnings |
| Warmup period | 0.5s (`SCHEDULE_WARMUP_SEC`) — no metrics collection |

**Per-event scheduling**:
```javascript
now = ctx.currentTime
if (event.when < now + 0.004)
  when = now + 0.004  // clamp to minimum look-ahead
src.start(when)
```

Uses microtask scheduling to flush the queue without blocking animation frames.

**Metrics**: Lead time and lag time tracked per-event. Per-second buckets (up to 180s history). Exposed as `getAudioScheduleMetrics()` and `exportAudioHistoryCSV()`.

**File**: `audio.js` scheduling constants ≈ lines 250-490

---

### Mobile Audio Unlock

AudioContext is created lazily on user gesture to comply with browser autoplay policies:

1. `navigator.audioSession.type = 'playback'` (Safari 16.4+)
2. Play silent `<audio>` element (iOS < 16.4 fallback for speaker routing)
3. Create AudioContext inside gesture handler
4. Play silent buffer
5. `ctx.resume()`

Event listeners (touchstart, touchend, pointerdown, mousedown, keydown) are never removed — any user input can re-unlock if context re-suspends (tab switch, phone call).

**Visibility handling**: On `visibilitychange`, resume context and flush deferred audio events. On `pagehide`, stop all active sources and close context cleanly.

**File**: `audio.js` `unlockAudio()` (≈ line 1050)

---

### Go ↔ JS Bridge

**Playback**:
- `playSoundsBatch(events[])` — batch: `{id, vol, pitch, dur, when?, row, abs}`
- `playSoundsBatchFlat(ids, vols, pitches, durs, whens, hasWhen, rows, abss)` — flat arrays for perf (invoked from `batch_wasm.go`)
- `stopSound(id)` — fade-out + stop active sources

**Effects**:
- `updateInsertEffects(id, slotsJSON)` — push JSON effect chain config from Go
- (JS → Go) `setInsertEffectParam(id, slot, param, value)` is a Go-exported global (`ui/js_exports_insert_effects.go`) that calls `audio.SetInsertEffectParam`; on WASM that re-pushes the slots and JS rebuilds the subgraph — it is not an in-place param tweak

**Synth params / seeding**:
- `setInstrumentBlock(id, block, seconds, amp, melodic)` — Go pushes an instrument's flattened document on every change (boot, edit, Save, Reset, import, undo); invalidates that id's render cache
- `clearInstrumentBlock(id)` — the instrument has no document any more (e.g. a WAV instrument)
- `__instrumentBlockForTest(id)` — test seam: the pushed block + render settings

**Timing**:
- `audioNow()` → `ctx.currentTime`
- `resumeAudio()` → `ctx.resume()`

**Volume**:
- `setChannelVolume(id, vol)`

**Files**: `engine_wasm.go`, `batch_wasm.go`, `insert_effects_wasm.go`, `channels_wasm.go`, `synth_recipe_wasm.go`, `ui/js_exports_insert_effects.go`

---

### Anti-Pop Fade (Browser)

Per-source: 5ms `linearRampToValueAtTime` from 0 → `CHAIN_SPEC.voiceHeadroom` (0.18, NOT 1.0 — the per-voice headroom the desktop mixer applies; omitting it made N voices sum ~5x hot into the limiter) on a dedicated fade GainNode at source start.

`stopSound()`: 20ms linear ramp to 0 on the fade GainNode, with `src.stop()` scheduled at the end of the fade (`now + fadeOut + 0.001`); the tracking set is cleared ~50 ms later. Safely falls back if no AudioContext exists.

**File**: `audio.js` `startRenderVoice()` (fade-in), `stopSound()` (fade-out)
