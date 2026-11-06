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

**File**: `engine_stop.go` `renderVoiceIntoInstBuf()`

#### Phase 2: Per-Instrument Channel Processing

For each active instrument slot, two block calls with a scope tap after each:
1. **Phase 2a** — `channel.ProcessInsertsBlockLocal(instBuf, postFXBuf)`: volume scaling (atomic float64) + enabled insert effects in chain order → tap `postFXBuf` (StageInsertFX)
2. **Phase 2b** — `channel.ProcessEQBlockLocal(postFXBuf, postEQBuf)`: the channel's EQ → tap `postEQBuf` (StageEQ)
3. Accumulate `postEQBuf` into the shared `masterBuf[]float64`

The two scratch buffers exist so each tap captures that stage's real output (not the unmodified instBuf).

⚠️ **No crossfade on this path.** `Channel.replaceProcessors` arms a ~5 ms old→new crossfade only when `samplesThruOld > 0`, which is set solely by `ProcessSampleLocal` — not called outside tests. So an insert-chain swap during playback switches instantly.

**File**: `engine_stop.go` Phase 2 block, `channels.go` `ProcessInsertsBlockLocal()` / `ProcessEQBlockLocal()`

#### Phase 3: Master Channel Processing

1. Main channel `ProcessBlockLocal(masterBuf, workBuf)` applies: volume + EQ (there is no master compressor — deleted 2026-10-27, it was dead on both platforms)
2. Soft clip (`mix_dsp.c` `soft_clip`, the browser's master node runs the same C): identity up to `SoftClipThreshold` (0.9, `chain_spec.go`), then `T + (1−T)·tanh((|x|−T)/(1−T))` — continuous and smooth (the old `T·tanh(x/T)` jumped from 0.9 to 0.685 at the knee). Output capture and the recording master tap read the signal here
3. Convert float64 → int16 with `math.Round()` (adds subtle dither-like behavior)
4. Return PCM bytes to oto player

**File**: `engine_stop.go` Phase 3 block + the final conversion loop

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

`Play(id)`, `PlayVol(id, vol)`, `PlayParams(id, vol, pitch, dur)` and `PlayParamsAt(...)` are independent entry points (not a call chain):
1. Build the voice via `newRecipeAwareVoice*` (`synth_recipe_dispatch.go`), which takes the recipe path when params are edited and otherwise falls back to `legacyNewVoice` (`Instrument.NewVoice`)
2. Check voice cache: hit → the cached buffer (SHARED, not cloned); miss → render + `Put`
3. Wrap voice in transformations (see below)
4. Call `mixer.Schedule(id, wrappedVoice, delaySamples, row, abs)`

**Files**: `engine_play.go` (entry points), `synth_recipe_dispatch.go` (voice construction)

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

1. `Schedule(id, voice, delaySamples, row, abs)` wraps in `antiPopVoice`, looks up instrument `Channel`, resolves stable slot index (`BEATMO_TEST_VOICE` swaps in a test sine here)
2. Creates a `voiceState` (start, id, slot, voice, channel, plus row/abs/emitted for highlight + scope attribution), appends to `pendingAdd` queue (separate lock)
3. At block start, `Read()` drains `pendingAdd` with a quick lock swap into the active voice list
4. Voices with `start > currentPos` wait silently; once `currentPos >= start`, they begin producing samples

**File**: `engine_mixer.go` `Schedule()`

---

### Voice Cache & Round-Robin

**Problem**: Repeated C synth calls are expensive. Identical instrument hits sound robotic ("machine gun effect").

**Solution**: `voiceCache` (desktop only — `//go:build !test && !js`; the browser caches in `audio.js`'s `renderCache`) stores pre-rendered buffers keyed by `{instrumentID, sampleRate, paramsHash, pitch}` — no tempo: a voice is `render.seconds` long at every tempo.

| Property | Value |
|----------|-------|
| Capacity | 48 MB byte budget, LRU eviction (`defaultVoiceCacheBudgetBytes`; RuntimeProfile sets a smaller browser value) + a secondary cap of 1024 entries |
| Variants per key | up to 3 (`RoundRobinVariants`) — capacity only; an entry normally holds 1 (see below) |
| Selection | Atomic counter: `idx = counter.Add(1) % len(variants)` (lock-free) |
| Variation source | **None today** — the C renderers seed `ma_noise` with 0, so renders are byte-identical (`TestLegacyRendersAreDeterministicAcrossCalls`) |

**Cache hit path**: Return the selected variant's buffer — SHARED, not cloned (callers must not mutate it). A hit never renders: the synchronous round-robin top-up was removed on 2026-07-13 because triggers #2/#3 paid a 13-20 ms C render on the audio dispatch goroutine for byte-identical buffers (`variants_no_topup_test.go`).

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
| Channel volume | Atomic float64 (via `math.Float64bits`) | Lock-free reads during audio processing |
| Processor chains | Channel `mu` RWMutex | Snapshot slice outside sample loop (no crossfade on the block path — see Phase 2) |
| Insert effect chains | `effectChainManager.mu` RWMutex | UI thread writes, audio thread reads snapshots |
| Output capture buffers | Capture mutex | Copy on read/write |

**Key constraint**: `seqMu` (sequencer mutex in `game_update.go`) is held during `drum.Update()`. Callbacks from DrumView must NOT acquire `seqMu` — Go mutexes are not reentrant.

---

### Buffer Sizes & Latency Constants

| Constant | Value | At 44.1kHz | File |
|----------|-------|------------|------|
| Block processing size | 64 samples | ~1.45ms | `engine_stop.go` |
| Oto buffer (desktop) | 20ms | 882 samples | `context_desktop.go` (the player also calls `SetBufferSize`, `engine_mixer.go`) |
| Oto buffer (WASM) | 10ms | 441 samples | `context_wasm.go` (unused for mixing — WebAudio mixes) |
| Anti-pop fade-in | 5ms | 220 samples | `voice_antipop.go` |
| Anti-pop fade-out | 20ms | 882 samples | `voice_antipop.go` |
| Channel crossfade | ~5ms | 220 samples | `channels.go` `channelXfadeSamples` (only armed via `ProcessSampleLocal` — dead on the block path) |
| Per-voice headroom | 0.18x | -14.9dB | `engine_stop.go` `mixHeadroom` |
| Voice cache budget | 48 MB (+ 1024-entry cap) | — | `voice_cache.go` |
| Round-robin variants | 3 | — | `voice_cache.go` |
| Wavetable size | 4096 + 1 guard | — | `wavetable.h` `WT_DEFAULT_LENGTH` |

### Sample Rate

Default: 44100 Hz (overridable via the `BEATMO_AUDIO_SAMPLE_RATE` env var — renamed from `AUDIO_SAMPLE_RATE`, which is no longer read). The desktop output is MONO (`ChannelCount: 1` in `context_desktop.go`), so channel pan is stored but not applied (`Channel.PanGains` has no callers). All C synthesis functions take `int sampleRate` as a parameter. Phase increments, delay times, envelope rates, and filter coefficients are all computed relative to the sample rate.

### Precision

- Oscillator phase accumulators: `double` (prevents drift over long durations)
- C synth I/O buffers: `float` (32-bit)
- Go mixer internal buffers: `float64`
- Final output: `int16` (via `math.Round()`)
- Parameters, filter coefficients: `float` (C side), `float64` (Go side)
