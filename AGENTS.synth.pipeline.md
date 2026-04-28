## Synth Pipeline: Mixer, Voices & Threading

Reference for the desktop audio pipeline from voice generation to PCM output. For the WASM/browser pipeline, see `AGENTS.synth.wasm.md`.

---

### 3-Phase Mixer Architecture

**Why 3 phases exist**: The original mixer processed individual voices through shared biquad EQ chains. Biquad filters maintain internal state (x1, x2, y1, y2) that expects a continuous signal stream. Interleaving samples from different instruments through the same biquad corrupted filter state, producing audible distortion. The fix: sum all voices per-instrument first, then route the coherent sum through EQ.

**File**: `src/go/internal/audio/engine_stop.go` (lines 3-36 for rationale, `Read()` for full implementation)

#### Phase 1: Voice Rendering

For each active voice:
1. Call `BlockVoice.SampleBlock()` (or fallback `Sample()`) to fill a temporary float64 buffer
2. Apply per-voice headroom: `sample *= mixHeadroom` where `mixHeadroom = 0.18` (-14.9dB)
3. Accumulate into `instBufs[slot]` indexed by the voice's instrument slot number
4. Track which slots are active in `activeSlots []int`

**Result**: Per-instrument summed buffers. No EQ or effects applied yet.

**File**: `engine_stop.go` `renderVoiceIntoInstBuf()` (lines 590-647)

#### Phase 2: Per-Instrument Channel Processing

For each active instrument slot:
1. Zero `postEQBuf` scratch buffer
2. `channel.ProcessBlockLocal(instBuf, postEQBuf)` applies:
   - Volume scaling (atomic float64)
   - Enabled insert effects in chain order
   - EQ processor
   - Crossfade logic (~5ms / 220 samples) when processor chain was recently swapped
3. Tap `postEQBuf` for scope/export (StageEQ) — this captures the real post-EQ per-instrument signal
4. Accumulate `postEQBuf` into shared `masterBuf[]float64`

**Note**: `ProcessBlockLocal` reads from `input` and accumulates into `output` without modifying `input`. The `postEQBuf` scratch buffer exists specifically so the StageEQ tap captures post-EQ data (not the unmodified pre-EQ instBuf).

**File**: `engine_stop.go` Phase 2 block, `channels.go` `ProcessBlockLocal()` lines 231-289

#### Phase 2.5: Send Effects

1. Zero accumulation buffers: `delaySendBuf`, `reverbSendBuf`
2. For each active instrument slot, load atomic send levels
3. Accumulate: `sendBuf[j] += instBuf[j] * sendAmount`
4. Convert float64 → float32, process through C delay/reverb effects in-place
5. Mix wet signal back into `masterBuf`

**File**: `send_effects.go` `processSlotSends()` lines 141-197

#### Phase 3: Master Channel Processing

1. Main channel applies: volume + EQ processors + compressor
2. Accumulates into `workBuf[]float64`
3. Hard clamp to [-1, 1] (safety net, rarely triggered in normal operation)
4. Convert float64 → int16 with `math.Round()` (adds subtle dither-like behavior)
5. Return PCM bytes to oto player

**File**: `engine_stop.go` lines 468-476

#### Block Processing Optimization

When all processors in a channel's chain implement `BlockProcessor`:
- Allocate ping-pong float32 buffers from `blockBufPool`
- Chain: volume → block processors → accumulate to output
- One CGo call per effect per block instead of per-sample
- Buffers are pooled to avoid per-block allocations

**File**: `channels.go` `ProcessBlockLocal()`, `blockBufPool`

---

### Voice Lifecycle

#### Voice Creation

`Play(id)` → `PlayVol(id, vol)` → `PlayParams(id, vol, pitch, dur)`:
1. Look up instrument by ID in registry
2. Call `inst.NewVoice(bpm, sampleRate)` (or `NewVoiceWithParams` for parameterized)
3. Check voice cache: hit → clone buffer; miss → call C render function
4. Wrap voice in transformations (see below)
5. Call `mixer.Schedule(id, wrappedVoice, delaySamples)`

**Files**: `engine.go` `Play()`/`PlayVol()`/`PlayParams()`, `engine_play.go`

#### Voice Types & Wrappers

| Type | Source | Purpose | Interface |
|------|--------|---------|-----------|
| `cVoice` | `drums_c.go` | Raw float32 buffer from C synth | `Voice` + `BlockVoice` |
| `antiPopVoice` | `voice_antipop.go` | Click-free fade-in (5ms linear) / fade-out (20ms quadratic) | `Voice` + `BlockVoice` |
| `scaledVoice` | `engine_state.go` | Gain multiplier (from `PlayVol`) | `Voice` + `BlockVoice` |
| `resampleVoice` | `engine_resample.go` | Pitch/duration via linear interpolation. `step = 2^(pitch/12) / dur` | `Voice` + `BlockVoice` |

**Wrapping order**: Every voice is wrapped in `antiPopVoice` at `Schedule()` time. `scaledVoice` and `resampleVoice` are applied at the `Play` call site before scheduling.

#### Voice Completion

Voice returns `isDone=true` from `Sample()`/`SampleBlock()`. The mixer removes it from the active list during the next block. For `antiPopVoice`, `RequestStop()` triggers the 20ms fade-out; the voice self-reports `isDone` after the fade completes.

#### Voice Scheduling

1. `Schedule(id, voice, delaySamples)` wraps in `antiPopVoice`, looks up instrument `Channel`, resolves stable slot index
2. Creates `voiceState{start, id, slot, v, ch}`, appends to `pendingAdd` queue (separate lock)
3. At block start, `Read()` drains `pendingAdd` with a quick lock swap into the active voice list
4. Voices with `start > currentPos` wait silently; once `currentPos >= start`, they begin producing samples

**File**: `engine_mixer.go` `Schedule()` lines 70-86

---

### Voice Cache & Round-Robin

**Problem**: Repeated C synth calls are expensive. Identical instrument hits sound robotic ("machine gun effect").

**Solution**: `voiceCache` stores pre-rendered buffers keyed by `{instrumentID, bpm, sampleRate, synthParams}`.

| Property | Value |
|----------|-------|
| Max entries | 256 |
| Variants per key | 3 (`RoundRobinVariants`) |
| Selection | Atomic counter: `idx = counter.Add(1) % len(variants)` (lock-free) |
| Variation source | C synth uses `ma_noise` with different seeds per render |

**Cache hit path**: Return clone of selected variant. If entry has fewer than 3 variants, render one more in background.

**Cache miss path**: Render immediately, cache the result.

**Files**: `voice_cache.go`, `variants.go`

---

### Thread Safety Model

| Resource | Protection | Access Pattern |
|----------|------------|----------------|
| Mixer state (voices, position) | `m.mu` Mutex | Lock once per block in `Read()` |
| Pending voices queue | `pendingMu` Mutex | Quick drain at block start, separate from main lock |
| Global audio context | `sync.Once` | Single-shot initialization |
| Instrument registry | `instMu` RWMutex | Read on lookup, write on register |
| Channel volume/pan | Atomic float64 (via `math.Float64bits`) | Lock-free reads during audio processing |
| Send levels | Atomic float64 | Lock-free reads in Phase 2.5 |
| Processor chains | Channel `mu` RWMutex | Snapshot slice outside sample loop; crossfade guards chain swaps |
| Insert effect chains | `effectChainManager.mu` RWMutex | UI thread writes, audio thread reads snapshots |
| C send effect state | `sendFX.mu` Mutex | Held during Phase 2.5 processing only |
| Output capture buffers | Capture mutex | Copy on read/write |

**Key constraint**: `seqMu` (sequencer mutex in `game_update.go`) is held during `drum.Update()`. Callbacks from DrumView must NOT acquire `seqMu` — Go mutexes are not reentrant.

---

### Buffer Sizes & Latency Constants

| Constant | Value | At 44.1kHz | File |
|----------|-------|------------|------|
| Block processing size | 64 samples | ~1.45ms | `engine_stop.go` |
| Oto buffer (desktop) | 20ms | 882 samples | `engine.go` |
| Oto buffer (WASM) | 10ms | 441 samples | `engine_wasm.go` |
| Anti-pop fade-in | 5ms | 220 samples | `voice_antipop.go` |
| Anti-pop fade-out | 20ms | 882 samples | `voice_antipop.go` |
| Channel crossfade | ~5ms | 220 samples | `channels.go` `channelXfadeSamples` |
| Per-voice headroom | 0.18x | -14.9dB | `engine_stop.go` `mixHeadroom` |
| Send delay line | 300ms | 13230 samples | `send_effects.go` |
| Voice cache max | 256 entries | — | `voice_cache.go` |
| Round-robin variants | 3 | — | `voice_cache.go` |
| Wavetable size | 4096 + 1 guard | — | `wavetable.h` `WT_DEFAULT_LENGTH` |

### Sample Rate

Default: 44100 Hz (overridable via `AUDIO_SAMPLE_RATE` env var). All C synthesis functions take `int sampleRate` as a parameter. Phase increments, delay times, envelope rates, and filter coefficients are all computed relative to the sample rate.

### Precision

- Oscillator phase accumulators: `double` (prevents drift over long durations)
- C synth I/O buffers: `float` (32-bit)
- Go mixer internal buffers: `float64`
- Final output: `int16` (via `math.Round()`)
- Parameters, filter coefficients: `float` (C side), `float64` (Go side)
