## Synth Pipeline: Browser / WASM Audio

Reference for the browser audio pipeline. For the desktop pipeline, see `AGENTS.synth.pipeline.md`.

---

### Architecture Overview

Desktop and WASM share C synth rendering, voice cache, synth_params, and instrument registry. They diverge at mixing:

| Component | Desktop | WASM |
|-----------|---------|------|
| Voice rendering | C synth via CGo | C synth via Emscripten WASM |
| Mixing | Go 3-phase mixer (`engine_stop.go`) | WebAudio node graph (`audio.js`) |
| Insert effects | C via CGo (`insert_fx_c.go`) | C via AudioWorklet WASM, or JS node graph fallback |
| EQ | Go biquad (`biquad.go`, `eq.go`) | WebAudio `BiquadFilterNode` |
| Send effects | C delay/reverb (`effects.c`) | WebAudio `DelayNode` + `ConvolverNode` |
| Panning | Go equal-power (`channels.go`) | `StereoPannerNode` |
| Compressor | Go `Compressor` (`compressor.go`) | `DynamicsCompressorNode` |
| Output | oto player → system audio | `AudioContext.destination` |

**Key**: On WASM, Go calls `playSoundsBatch()` / `playSoundsBatchFlat()` via `syscall/js`. JavaScript handles all mixing, effects, and output. Go-side mixer code is not used.

**Files**: `src/go/internal/audio/engine_wasm.go`, `insert_effects_wasm.go`, `send_effects_wasm.go`, `channels_wasm.go`, `src/js/audio.js` (2861 lines)

---

### WebAudio Node Graph

Per-instrument signal path:

```
AudioBufferSourceNode (pre-rendered C synth sample)
  ↓
Anti-Pop Fade GainNode (5ms linear ramp 0→1)
  ↓
Volume Bus GainNode (quantized, LRU cached — see below)
  ↓
Channel Ingress GainNode
  ↓
[Insert Effects Chain] — AudioWorklet (C/WASM) or JS node subgraph
  ↓
Pre-EQ AnalyserNode (optional tap for spectrum capture)
  ↓
[EQ Chain] — 10-band ISO peaking BiquadFilterNodes (serial)
  OR [Multiband Processor] — used if any band is muted
  ↓
Post-EQ AnalyserNode (optional tap)
  ↓
Channel GainNode (setChannelVolume)
  ↓
StereoPannerNode (lazy-created, -1 to +1)
  ↓                    ↓                      ↓
Main Channel      Delay Send GainNode    Reverb Send GainNode
GainNode            ↓                      ↓
  ↓              Delay Bus              Reverb Bus
  ↓              (feedback loop)        (ConvolverNode)
  ↓                    ↓                      ↓
  ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
  ↓
DynamicsCompressorNode (threshold -6dB, ratio 4:1, attack 1ms, release 50ms, knee 3dB)
  ↓
WaveShaperNode (hard limiter, tanh curve [-1,1])
  ↓
AudioContext.destination
```

#### Volume Bus Caching

Each instrument has an LRU cache of GainNodes, quantized to 16 volume levels (`VOL_Q = 16`). Key: `${id}|${quantizedVolume}`. Max 16 buses per instrument, evicted after 2s unused. Avoids creating a new GainNode per hit.

#### EQ Chain

10-band ISO standard frequencies: 31, 62, 125, 250, 500, 1k, 2k, 4k, 8k, 16k Hz. Each band is a `BiquadFilterNode` type `peaking` with:
- `frequency = sqrt(loHz * hiHz)` (geometric center)
- `Q = 1 / (2 * sinh(ln2 / 2 * bwOctaves))` where `bwOctaves = log2(hiHz / loHz)`

If any band is muted: uses `createMultibandProcessor()` (serial chain with doubled filters at -120dB for muted bands). If all bands muted: single GainNode at 0.

---

### Insert Effects: Two Runtime Modes

#### AudioWorklet Mode (Preferred)

Runs C effects compiled to WASM in a dedicated AudioWorklet thread. Provides bit-accurate parity with desktop.

**File**: `src/js/insert_fx_worklet.js` (361 lines)

**Message protocol**:
```
Main → Worklet:
  { type: 'init', moduleFactory: String }       // Pass Emscripten WASM module code
  { type: 'configure', slots: [{type, enabled, params}] }
  { type: 'setParam', slotIndex, param, value }

Worklet → Main:
  { type: 'ready' }
```

**Processing** (128-sample render quantum):
1. Copy input from AudioWorklet input buffer → WASM heap (`_malloc`)
2. Ping-pong through effect chain: src → `_ifx_*_process()` → dst, swap pointers
3. Copy output from WASM heap → AudioWorklet output buffer
4. All effects process mono; output copied to all channels

**Memory**: Each effect allocates WASM heap memory during `configure()` — struct memory (32-256 bytes) plus auxiliary buffers for delay lines, reverb, etc. Freed on effect removal/disable.

#### JS Node Graph Fallback

Used when AudioWorklet is unavailable. Creates native WebAudio node subgraphs:

| Effect | Nodes | Notes |
|--------|-------|-------|
| distortion | WaveShaperNode | tanh curve, drive-controlled, 2x oversample |
| delay | DelayNode + feedback GainNode + BiquadFilter LP | 250ms default, 0.4 feedback |
| reverb | ConvolverNode + synthetic impulse response | Exponential decay IR |
| chorus | DelayNode + OscillatorNode (LFO) | Sine LFO modulates delay time |
| bitcrusher | (passthrough) | No native equivalent; worklet-only |
| filter | BiquadFilterNode | 3-mode: lowpass/highpass/bandpass |
| waveshaper | WaveShaperNode | 4 curve types: tanh, clip, fold, sine |
| ringmod | OscillatorNode → GainNode → input.gain | Sine or square carrier |
| tremolo | OscillatorNode → GainNode | Sine LFO modulates amplitude |
| flanger | DelayNode + OscillatorNode + feedback GainNode | Short modulated delay with feedback |
| phaser | (passthrough) | Complex; no good native equivalent |
| autowah | (passthrough) | Complex; no good native equivalent |
| gate | (passthrough) | Complex; no good native equivalent |
| transient | (passthrough) | Complex; no good native equivalent |
| pitchshift | (passthrough) | Complex; no good native equivalent |
| compressor | DynamicsCompressorNode | threshold, ratio, attack, release |
| limiter | DynamicsCompressorNode | Configured as limiter |
| tape | WaveShaperNode + BiquadFilter LP | Saturation + warmth (2k-20k Hz) |

All fallback effects use parallel gain nodes for wet/dry mixing.

**Parameter updates**:
- Worklet: `node.port.postMessage({ type: 'setParam', slotIndex, param, value })`
- JS fallback: Direct property updates (e.g., `biquadFilter.frequency.value = newValue`)

---

### Deferred AudioBuffer Pattern

Two-phase rendering avoids premature AudioContext creation on mobile (which would suspend):

**Phase A — CPU rendering (no AudioContext needed)**:
1. Get sample rate (default 48kHz if no context yet)
2. Allocate WASM heap (`frames * 4` bytes)
3. Call C render: `_render_<type>(ptr, sampleRate, frames)`
4. Copy Float32Array from WASM heap
5. Cache as `{ buffer: null, data, sr, frames }`

**Phase B — Lazy AudioBuffer creation (first playback)**:
1. Create AudioContext (triggered by user gesture)
2. `ctx.createBuffer()` with cached data
3. Copy Float32Array to buffer channel
4. Store AudioBuffer in cache for reuse
5. WebAudio handles resampling if `buffer.sampleRate !== ctx.sampleRate`

**File**: `audio.js` `ensureRenderedSample()` lines 1937-2017

---

### Audio Scheduling

Queue-based event processing to avoid blocking the main thread:

| Property | Value |
|----------|-------|
| Queue max size | 8192 (`audioQueue`) |
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

**File**: `audio.js` lines 101-483

---

### Send Effects (Browser)

#### Delay Bus
```
Input GainNode (1.0)
  → DelayNode (300ms fixed)
  → BiquadFilter LP (3kHz, Q 0.707)
  → Feedback GainNode (0.3) ──→ back to delay input
  → Output → Main Channel
```

#### Reverb Bus
```
Input GainNode (1.0)
  → ConvolverNode (synthetic 1.5s IR: exponential decay with random noise)
  → Wet GainNode (0.3)
  → Output → Main Channel
```

Per-instrument send control via `setDelaySend(id, amount)` / `setReverbSend(id, amount)` (0-1 range). Called from Go via WASM bridge.

**File**: `audio.js` lines 884-1003

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

**File**: `audio.js` `unlockAudio()` lines 527-687

---

### Go ↔ JS Bridge

**Playback**:
- `playSoundsBatch(events[])` — batch: `{id, vol, pitch, dur, when?}`
- `playSoundsBatchFlat(ids, vols, pitches, durs, whens, hasWhen)` — flat arrays for perf
- `stopSound(id)` — fade-out + stop active sources

**Effects**:
- `updateInsertEffects(id, slotsJSON)` — push JSON effect chain config from Go
- `setDelaySend(id, amount)` / `setReverbSend(id, amount)` — send levels
- `setInsertEffectParam(id, slot, param, value)` — real-time param tweak

**Timing**:
- `audioNow()` → `ctx.currentTime`
- `resumeAudio()` → `ctx.resume()`

**Volume/Pan**:
- `setChannelVolume(id, vol)` / `setChannelPan(id, pan)`

**Files**: `engine_wasm.go`, `insert_effects_wasm.go`, `send_effects_wasm.go`, `channels_wasm.go`

---

### Anti-Pop Fade (Browser)

Per-source: 5ms `linearRampToValueAtTime` from 0→1 on a dedicated fade GainNode at source start.

`stopSound()`: 20ms linear ramp 1→0 on the fade GainNode, then `source.stop()` after ~50ms to ensure fade completes. Safely falls back if no AudioContext exists.

**File**: `audio.js` `processAudioEvent()` (fade-in), `stopSound()` (fade-out)
