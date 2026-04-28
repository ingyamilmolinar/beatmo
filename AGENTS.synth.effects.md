## Synth Pipeline: Insert & Send Effects

Reference for all audio effects in the pipeline. For effect debugging, see `AGENTS.synth.debug.md`.

---

### Effect Chain Architecture

Each instrument has an insert effect chain managed by `effectChainManager` (`effect_chain.go`). The chain feeds into the channel's processor list.

**Chain order**: `[enabled insert effects in order] → [EQ processor]`

**Rebuild flow**: Any add/remove/move/toggle operation calls `rebuildChannelProcessors()`:
1. Collect enabled insert effect processors from the chain
2. Lookup the last-set EQ for this channel (preserved across rebuilds)
3. Append EQ processor to the end
4. Call `channel.replaceProcessors(newSlice)` which:
   - Sets old chain for crossfade if audio has flowed through it (~5ms / 220 samples)
   - Atomically swaps the processor slice
   - Recomputes the block processor cache
   - On WASM: calls `platformInsertEffectsChanged()` to push JSON to JS

**Thread safety**: `effectChainManager.mu` RWMutex protects the chains map. UI thread writes; audio thread reads snapshots (safe because `replaceProcessors` creates a new slice).

**API** (Go):
```go
audio.AddInsertEffect(id, effectType, params)    // Append effect to chain
audio.RemoveInsertEffect(id, slotIndex)           // Remove by index
audio.MoveInsertEffect(id, fromIndex, toIndex)    // Reorder
audio.SetInsertEffectParam(id, slot, param, val)  // Real-time param update (no rebuild)
audio.ToggleInsertEffect(id, slotIndex, enabled)  // Enable/disable
audio.GetInsertEffects(id) → []EffectSlot         // Read current chain
audio.SetInsertEffects(id, slots)                 // Bulk import (for loading saved state)
```

**Files**: `effect_chain.go`, `insert_effects.go`, `channels.go`

---

### Build Tag Split

| Build context | Implementation | File |
|--------------|----------------|------|
| Desktop (CGo) | C via `insert_fx.c` | `insert_fx_c.go` (build tag `!test && !js`) |
| Tests | Pure Go fallback | `fx_distortion.go`, `fx_delay.go`, etc. (build tag `test \|\| js`) |
| WASM (AudioWorklet) | C via Emscripten in worklet thread | `insert_fx_worklet.js` |
| WASM (JS fallback) | WebAudio node graph | `audio.js` |

The factory `NewEffectProcessor()` in `insert_effects.go` and chain management in `effect_chain.go` are build-tag agnostic. `newDistortion()`, etc., resolve to Go or C at compile time.

---

### BlockProcessor Interface

```go
type BlockProcessor interface {
    Processor
    ProcessBlockBuf(in []float32, out []float32, samples int)
}
```

All C insert effect wrappers implement `BlockProcessor`. When **all** processors in a channel's chain implement it, `ProcessBlockLocal()` uses the optimized path: apply volume once into a float32 buffer, then ping-pong through each effect's `ProcessBlockBuf()` — one CGo call per effect per block instead of per-sample. Buffers are pooled via `blockBufPool` to avoid per-block allocations.

**File**: `channels.go`

---

### All 18 Insert Effects

Every C effect has `_init()`, `_process(in, out, N)`, `_reset()`, `_set_param(name, value)`. All Go wrappers implement both `Processor` (per-sample) and `BlockProcessor` (block).

#### Dynamics

**Distortion** (`EffectDistortion`, `ifx_distortion_t`)
- Signal: `wet = LP(tanh(drive * x))`, then dry/wet blend
- One-pole LP at `tone` Hz for post-distortion warmth
- Parameters: `drive` (1-20), `tone` (200-8000 Hz), `mix` (0-1)
- Memory: stateless (LP filter state only)

**Waveshaper** (`EffectWaveshaper`, `ifx_waveshaper_t`)
- Signal: apply drive, then selected curve function, dry/wet blend
- Curves: 0=tanh, 1=hard clip, 2=wavefolding, 3=sine waveshaping
- Parameters: `curve` (0-3), `drive` (1-20), `mix` (0-1)
- Memory: stateless

**Compressor** (`EffectCompressor`, `ifx_compressor_t`)
- Signal: envelope follower → dB domain gain calc → makeup gain
- Dual attack/release coefficients, gain reduction above threshold
- Parameters: `threshold` (-60 to 0 dB), `ratio` (1-20), `attack` (0.1-100 ms), `release` (10-1000 ms), `makeup` (0-40 dB), `mix` (0-1)
- Memory: stateless (envelope state only)

**Limiter** (`EffectLimiter`, `ifx_limiter_t`)
- Signal: instant attack, exponential release, brick-wall limiting at ceiling
- Parameters: `threshold` (-60 to 0 dB), `release` (10-1000 ms), `ceiling` (-20 to 0 dB)
- Memory: stateless (envelope state only)

**Gate** (`EffectGate`, `ifx_gate_t`)
- Signal: envelope follower → gain switch (1.0 above threshold, `range` below)
- Parameters: `threshold` (-60 to 0 dB), `attack` (0.1-100 ms), `release` (10-1000 ms), `range` (-80 to 0 dB)
- Memory: stateless (envelope state only)

**Transient Shaper** (`EffectTransient`, `ifx_transient_t`)
- Signal: fast envelope and slow envelope followers → transient amount = fast - slow → gain adjustment
- Fast envelope detects transients; slow envelope tracks sustain level
- Parameters: `attack` (0-200, as percentage where 100=unity), `sustain` (0-200, percentage), `speed` (1-50 ms)
- Memory: stateless (envelope states only)

#### Modulation

**Chorus** (`EffectChorus`, `ifx_chorus_t`)
- Signal: sine LFO modulates delay tap position, linear interpolation, dry/wet blend
- Parameters: `rate` (0.1-5 Hz), `depth` (1-20 ms), `mix` (0-1)
- Memory: **caller-owned circular buffer** (Go allocates via `C.malloc`)

**Flanger** (`EffectFlanger`, `ifx_flanger_t`)
- Signal: short delay (0.5-10ms) with feedback loop, LFO modulates delay time
- Negative feedback inverts the comb effect (different spectral character)
- Parameters: `rate` (0.1-5 Hz), `depth` (0.5-10 ms), `feedback` (-0.95 to 0.95), `mix` (0-1)
- Memory: **caller-owned circular buffer**

**Phaser** (`EffectPhaser`, `ifx_phaser_t`)
- Signal: chain of allpass filters modulated by LFO, feedback from last stage
- More stages = more notches in frequency response
- Parameters: `stages` (2-12), `rate` (0.1-5 Hz), `depth` (0-1), `feedback` (0-0.95), `mix` (0-1)
- Memory: stateless (allpass state arrays: x1[12], y1[12])

**Tremolo** (`EffectTremolo`, `ifx_tremolo_t`)
- Signal: LFO (0-1 range) multiplies signal amplitude
- Parameters: `rate` (0.1-20 Hz), `depth` (0-1), `shape` (0=sine, 1=triangle, 2=square), `mix` (0-1)
- Memory: stateless (LFO phase only)

**Ring Modulator** (`EffectRingMod`, `ifx_ringmod_t`)
- Signal: carrier oscillator (sine or square) multiplies input signal
- Produces sum and difference frequencies (metallic/robotic character)
- Parameters: `frequency` (20-5000 Hz), `shape` (0=sine, 1=square), `mix` (0-1)
- Memory: stateless (oscillator phase only)

**Auto-Wah** (`EffectAutoWah`, `ifx_autowah_t`)
- Signal: envelope follower + LFO → state-variable bandpass filter → dry/wet blend
- State-variable filter: `hp = input - lp - q*bp; bp += f*hp; lp += f*bp`
- Parameters: `sensitivity` (0-1), `rate` (0.1-5 Hz), `depth` (0-1), `mix` (0-1)
- Memory: stateless (envelope + filter + LFO state)

#### Time-Based

**Delay** (`EffectDelay`, `ifx_delay_t`)
- Signal: circular buffer delay line with LP-filtered feedback for warmth
- Feedback LP: fixed one-pole at 3000 Hz (darkening)
- Parameters: `time` (10-1000 ms), `feedback` (0-0.95), `mix` (0-1)
- Memory: **caller-owned circular buffer** (size = max 1s at sample rate)

**Reverb** (`EffectReverb`, `ifx_reverb_t`)
- Signal: Schroeder algorithm — 4 parallel comb filters (LP-damped) + 2 series allpass diffusers
- Comb delays at 44.1kHz: 1116, 1188, 1277, 1356 samples (prime-spaced)
- Allpass delays at 44.1kHz: 556, 441 samples
- Comb feedback: `0.7 + 0.28 * room`
- Parameters: `room` (0-1), `damping` (0-1), `mix` (0-1)
- Memory: **caller-owned buffer** (size from `ifx_reverb_mem_size(sr)`)

**Tape Saturation** (`EffectTape`, `ifx_tape_t`)
- Signal: `tanh(drive * x)` → warmth LP filter → delay buffer with wow (0.5 Hz) + flutter (6 Hz) LFOs
- Wow: slow pitch modulation for tape-machine character
- Flutter: fast pitch modulation for subtle instability
- Parameters: `drive` (1-10), `warmth` (0-1), `wow` (0-1), `flutter` (0-1), `mix` (0-1)
- Memory: **caller-owned circular buffer** for wow/flutter delay

#### Pitch

**Pitch Shifter** (`EffectPitchShift`, `ifx_pitchshift_t`)
- Signal: dual delay-line read heads with triangular cross-fade window
- Heads drift at rate determined by pitch interval; cross-fade prevents discontinuities
- Parameters: `pitch` (-24 to +24 semitones), `mix` (0-1), `window` (20-100 ms)
- Memory: **caller-owned circular buffer**

#### Filter

**Filter** (`EffectFilter`, `ifx_filter_t`)
- Signal: RBJ biquad filter (Direct Form I), real-time coefficient update
- Parameters: `mode` (0=lowpass, 1=highpass, 2=bandpass), `cutoff` (20-20000 Hz), `q` (0.1-10), `mix` (0-1)
- Memory: stateless (biquad state: x1, x2, y1, y2)

---

### Memory Management

Effects with circular buffers (delay, chorus, flanger, tape, pitchshift, reverb) use **caller-owned memory**:
- Go allocates via `C.malloc()`, stores `unsafe.Pointer` in the wrapper struct
- Passed to C `_init()` as `float *buf, int buf_len`
- Freed when effect is destroyed

Reverb has a special memory calculator: `ifx_reverb_mem_size(sr)` returns total floats needed for all comb + allpass buffers at the given sample rate.

Stateless effects (distortion, waveshaper, bitcrusher, filter, ringmod, tremolo, gate, limiter, phaser, autowah, compressor, transient) have no buffer allocation — internal state is small (envelope followers, LFO phase, filter coefficients).

**File**: `insert_fx_c.go` (CGo wrappers with allocation)

---

### Send Effects (Delay + Reverb)

Parallel wet/dry architecture. Each instrument channel has atomic "delay" and "reverb" send amounts (0-1).

**C implementations** (`effects.c` / `effects.h`):
- **Delay**: Circular buffer, LP-filtered feedback (300ms, 0.3 feedback, 3kHz LP damping)
- **Reverb**: Schroeder — 4 parallel combs (prime-spaced) + 2 series allpass diffusers

**Mixer integration**: Phase 2.5 in `processBlock()` between per-instrument EQ (Phase 2) and master EQ (Phase 3):
1. Zero accumulation buffers
2. For each active instrument: `sendBuf[j] += instBuf[j] * sendAmount`
3. Process C delay/reverb in-place on accumulation buffers
4. Mix wet signal back into `masterBuf`

**API**:
```go
audio.SetDelaySend("snare", 0.3)  // 30% to delay bus
audio.SetReverbSend("snare", 0.2) // 20% to reverb bus
```
```javascript
window.setDelaySend("snare", 0.3);
window.setReverbSend("snare", 0.2);
```

**Thread safety**: Send levels stored atomically per instrument (lock-free reads during Phase 2.5). C effect state protected by `sendFX.mu` mutex.

**Files**: `effects.c`, `effects.h`, `send_effects.go`, `effects_test.go`, `engine_stop.go` (Phase 2.5), `audio.js` (WebAudio sends)

---

### Master Compressor

Separate from insert effects. Installed on the master channel via `SetupMasterCompressor()` during audio init.

**Go implementation** (`compressor.go`):
- Peak detection with separate attack/release envelope follower
- Soft knee (3dB) for transparent compression
- Defaults: threshold -6dB, ratio 4:1, attack 1ms, release 50ms
- Implements `Processor` and `BlockProcessor`

**Browser**: Native `DynamicsCompressorNode` between main channel and hard limiter, matching the same parameters.

**Files**: `compressor.go`, `compressor_test.go`, `audio.js` `ensureCompressor()`

---

### WASM AudioWorklet Details

On WASM, insert effects run in an `AudioWorkletProcessor` that loads the Emscripten WASM module.

**Flow**: Go → `platformInsertEffectsChanged()` → `audio.js` `updateInsertEffects(id, slotsJSON)` → worklet `port.postMessage({type: 'configure', slots})`

**Worklet processing** (128-sample render quantum):
1. Copy input buffer → WASM heap (via `Module._malloc`)
2. For each enabled effect: call `Module._ifx_*_process(structPtr, inPtr, outPtr, 128)`
3. Ping-pong src/dst pointers between effects
4. Copy output from WASM heap → AudioWorklet output buffer

**Fallback**: If AudioWorklet init fails, `audio.js` creates JS WebAudio node subgraphs. See `AGENTS.synth.wasm.md` for the fallback node mapping table.

**File**: `src/js/insert_fx_worklet.js`

---

### Creating a New Insert Effect

1. **C**: Add `ifx_myeffect_t` struct + `_init()/_process()/_reset()/_set_param()` to `insert_fx.h` / `insert_fx.c`
2. **CGo bridge**: Add wrapper struct in `insert_fx_c.go` implementing `InsertEffect` + `BlockProcessor` (build tag `!test && !js`)
3. **Go fallback**: Add pure Go implementation in `fx_myeffect.go` (build tag `test || js`)
4. **Factory**: Add `EffectType` const and `newMyEffect()` in `insert_effects.go`. Add parameter definitions to `InsertEffectCatalog()`
5. **JS AudioWorklet**: Add message handling in `insert_fx_worklet.js` `configure()` switch
6. **JS fallback**: Add WebAudio node graph in `audio.js` `createInsertEffectSubgraph()`
7. **Makefile**: Append new `_ifx_*` functions to Emscripten `-sEXPORTED_FUNCTIONS`
