## Synth Pipeline: Insert Effects

Reference for all audio effects in the pipeline. For effect debugging, see `AGENTS.synth.debug.md`.

---

### Effect Chain Architecture

Each instrument has an insert effect chain managed by `effectChainManager` (`effect_chain.go`). The chain feeds into the channel's processor list.

**Chain order**: `[enabled insert effects in order] → [EQ processor]`

**Rebuild flow**: Any add/remove/move/toggle operation calls `rebuildChannelProcessors()` (`effect_chain.go`):
1. Collect enabled insert effect processors from the chain
2. Build a fresh EQ processor from THIS channel's own EQ record (`lookupChannelEQ` → `NewEQProcessor`) — per-channel, so an insert-chain change never wipes another channel's EQ
3. Call `channel.replaceProcessors(inserts, eqProcs)` (inserts and EQ are passed separately), which atomically swaps the processor slices and recomputes the block-processor cache
4. Then call `platformInsertEffectsChanged(id, slots)` (a no-op on desktop; on WASM it pushes the slot JSON to JS)

⚠️ **Crossfade is effectively dead on the live path.** `replaceProcessors` only arms its ~5 ms old→new crossfade when `samplesThruOld > 0`, and that flag is set ONLY by `Channel.ProcessSampleLocal` — which nothing outside tests calls. The mixer uses the block paths (`ProcessInsertsBlockLocal` / `ProcessEQBlockLocal`), so a chain swap during playback is not crossfaded.

**Thread safety**: `effectChainManager.mu` RWMutex protects the chains map. UI thread writes; audio thread reads snapshots (safe because `replaceProcessors` creates a new slice).

**API** (Go):
```go
audio.AddInsertEffect(id, effectType, params)    // Append effect to chain
audio.RemoveInsertEffect(id, slotIndex)           // Remove by index
audio.MoveInsertEffect(id, fromIndex, toIndex)    // Reorder
audio.SetInsertEffectParam(id, slot, param, val)  // Desktop: real-time SetParam, no rebuild. WASM: re-pushes the slots, JS rebuilds the subgraph
audio.ToggleInsertEffect(id, slotIndex, enabled)  // Enable/disable
audio.GetInsertEffects(id) → []EffectSlot         // Read current chain
audio.SetInsertEffects(id, slots)                 // Bulk import (for loading saved state)
```

**Files**: `effect_chain.go`, `insert_effects.go`, `channels.go`

---

### Build Tag Split

| Build context | Implementation | File |
|--------------|----------------|------|
| Desktop (CGo) | C via `fx_host.c` → `insert_fx.c` (one generic `cEffect`) | `insert_fx_c.go` (build tag `!test && !js`) |
| Browser | **The same C**, from `drums.single.js` in the `beatmo-dsp` AudioWorklet (one module instance per AudioContext; one node per channel's chain) | `dsp_worklet.js` + `dsp_host.js`; wiring in `audio.js` |
| Browser, Go side | Pass-through bookkeeping (the C runs in the worklet) | `insert_effects_js.go` (`js && !test`) |
| `-tags test` | Pure Go stand-ins (no cgo) | `fx_distortion.go`, `fx_delay.go`, etc. (build tag `test`) |

**One computation on both platforms (2026-10-27).** `fx_host.c` is the only place that knows how to build an effect — its init-argument order (`ifx_param_names`), its delay-buffer size, its state size. Go supplies the COMPLETE parameter set (`mergeDefaults`; the browser never fills a default), pushed as `InsertEffectInstance{uid, type, enabled, params}`. Each slot's `uid` names its processor instance: add / remove / move / toggle keep every surviving instance and its state on desktop, and the worklet keeps the C instance with the same uid, so no edit resets a neighbour on one platform only. A param edit is `ifx_set` on the live instance (state kept) on both. Every effect is block-size invariant (`TestEveryEffectIsBlockSizeInvariant`: blocks of 1/64/97/128 give the same samples), so desktop's 64-frame blocks and the worklet's 128-frame quanta agree. A channel with an effect is processed every block on both platforms, voice or not — delay echoes and reverb tails ring out, LFOs advance. `xplat_audio_stages.browser.test.js` compares all 18 effects at default/min/mid/max per sample against desktop in Chromium, Firefox and WebKit.

The factory `NewEffectProcessor()` in `insert_effects.go` and chain management in `effect_chain.go` are build-tag agnostic. `newDistortion()`, etc., resolve per build at compile time.

---

### BlockProcessor Interface

```go
type BlockProcessor interface {
    Processor
    ProcessBlockBuf(in []float32, out []float32, samples int)
}
```

All CGo insert effect wrappers (`insert_fx_c.go`) implement `BlockProcessor`; the pure-Go `fx_*.go` fallbacks (tests / WASM build) are per-sample only. When **all** processors in a channel's chain implement it, `ProcessBlockLocal()` uses the optimized path: apply volume once into a float32 buffer, then ping-pong through each effect's `ProcessBlockBuf()` — one CGo call per effect per block instead of per-sample. Buffers are pooled via `blockBufPool` to avoid per-block allocations.

**File**: `channels.go`

---

### All 18 Insert Effects

Every C effect has `_init()`, `_process(in, out, N)`, `_reset()`, `_set_param(name, value)`. The CGo wrappers implement both `Processor` (per-sample) and `BlockProcessor` (block); the pure-Go `fx_*.go` fallbacks implement `Processor` only. Param ranges below are the registry's (`effect_registry_entries.go`), which the C clamps in `insert_fx.c` match.

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

**Bitcrusher** (`EffectBitcrusher`, `ifx_bitcrusher_t`)
- Signal: sample-and-hold decimation (a phase accumulator advances by `rate` per sample; each wrap latches a new sample) quantized to `2^bits` levels, then dry/wet blend
- Parameters: `bits` (2-16), `rate` (0.01-1, fraction of the sample rate), `mix` (0-1)
- Memory: stateless (hold counter + held value)

**Compressor** (`EffectCompressor`, `ifx_compressor_t`)
- Signal: envelope follower → dB domain gain calc → makeup gain
- Dual attack/release coefficients, gain reduction above threshold
- Parameters: `threshold` (-60 to 0 dB), `ratio` (1-20), `attack` (0.1-100 ms), `release` (10-1000 ms), `makeup` (0-24 dB), `mix` (0-1)
- Memory: stateless (envelope state only)

**Limiter** (`EffectLimiter`, `ifx_limiter_t`)
- Signal: instant attack, exponential release, brick-wall limiting at ceiling
- Parameters: `threshold` (-20 to 0 dB), `release` (1-500 ms), `ceiling` (-6 to 0 dB)
- Memory: stateless (envelope state only)

**Gate** (`EffectGate`, `ifx_gate_t`)
- Signal: envelope follower → gain switch (1.0 above threshold, `range` below)
- Parameters: `threshold` (-60 to 0 dB), `attack` (0.1-50 ms), `release` (1-500 ms), `range` (-90 to 0 dB)
- Memory: stateless (envelope state only)

**Transient Shaper** (`EffectTransient`, `ifx_transient_t`)
- Signal: fast envelope and slow envelope followers → transient amount = fast - slow → gain adjustment
- Fast envelope detects transients; slow envelope tracks sustain level
- Parameters: `attack` (0-200, as percentage where 100=unity), `sustain` (0-200, percentage), `speed` (1-50 ms)
- Memory: stateless (envelope states only)

#### Modulation

**Chorus** (`EffectChorus`, `ifx_chorus_t`)
- Signal: sine LFO modulates delay tap position, linear interpolation, dry/wet blend
- Parameters: `rate` (0.1-10 Hz), `depth` (0-20 ms), `mix` (0-1)
- Memory: **caller-owned circular buffer** (Go allocates via `C.malloc`)

**Flanger** (`EffectFlanger`, `ifx_flanger_t`)
- Signal: short delay (0.5-10ms) with feedback loop, LFO modulates delay time
- Negative feedback inverts the comb effect (different spectral character)
- Parameters: `rate` (0.1-10 Hz), `depth` (0.5-10 ms), `feedback` (-0.95 to 0.95), `mix` (0-1)
- Memory: **caller-owned circular buffer**

**Phaser** (`EffectPhaser`, `ifx_phaser_t`)
- Signal: chain of allpass filters modulated by LFO, feedback from last stage
- More stages = more notches in frequency response
- Parameters: `stages` (2-12), `rate` (0.1-10 Hz), `depth` (0-1), `feedback` (0-0.95), `mix` (0-1)
- Memory: stateless (allpass state arrays: x1[12], y1[12])

**Tremolo** (`EffectTremolo`, `ifx_tremolo_t`)
- Signal: LFO (0-1 range) multiplies signal amplitude
- Parameters: `rate` (0.5-20 Hz), `depth` (0-1), `shape` (0=sine, 1=triangle, 2=square), `mix` (0-1)
- Memory: stateless (LFO phase only)

**Ring Modulator** (`EffectRingMod`, `ifx_ringmod_t`)
- Signal: carrier oscillator (sine or square) multiplies input signal
- Produces sum and difference frequencies (metallic/robotic character)
- Parameters: `frequency` (20-5000 Hz), `shape` (0=sine, 1=square), `mix` (0-1)
- Memory: stateless (oscillator phase only)

**Auto-Wah** (`EffectAutoWah`, `ifx_autowah_t`)
- Signal: envelope follower + LFO → state-variable bandpass filter → dry/wet blend
- State-variable filter: `hp = input - lp - (1/q)*bp; bp += f*hp; lp += f*bp`, with `q = 0.5 + 4.5·depth` and the damping `1/q` clamped ≤ 1.9 (feeding Q in directly — the original code — over-damped past the stable region and exploded on hot input)
- Parameters: `sensitivity` (0-1), `rate` (0.5-20 Hz), `depth` (0-1), `mix` (0-1)
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
- Signal: normalized tanh `tanh(drive·x)/tanh(drive)` → warmth LP filter → delay buffer with wow (0.5 Hz) + flutter (6 Hz) LFOs
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
- Passed to C `_init()` (`float *buf, int buf_len` for the delay-line effects; reverb takes only `float *mem`, sized by `ifx_reverb_mem_size`)
- ⚠️ **Never freed**: there is no `Close`/finalizer on the wrappers, so dropping a processor leaks its buffer (the only `C.free` calls are for the C-string param names)

Reverb has a special memory calculator: `ifx_reverb_mem_size(sr)` returns total floats needed for all comb + allpass buffers at the given sample rate.

Stateless effects (distortion, waveshaper, bitcrusher, filter, ringmod, tremolo, gate, limiter, phaser, autowah, compressor, transient) have no buffer allocation — internal state is small (envelope followers, LFO phase, filter coefficients).

**File**: `insert_fx_c.go` (CGo wrappers with allocation)

---

### No send buses

There are no delay/reverb send buses, no per-row send levels and no pan (all
deleted 2026-10-27: pan was never heard, the sends had no control and the two
platforms voiced them differently). A row that wants a room or an echo carries a
`reverb` / `delay` insert effect in its FX panel chain, visible to the user.

---

### Master Compressor

Separate from insert effects. Installed on the master channel via `SetupMasterCompressor()` during audio init.

**Go implementation** (`compressor.go`):
- Peak detection with separate attack/release envelope follower
- Soft knee (3dB) for transparent compression
- Defaults: threshold -3 dB, ratio 2.5:1, attack 5 ms, release 80 ms, knee 3 dB — the single source is `chain_spec.go` (`Compressor*` consts), which the browser reads too via `CHAIN_SPEC`
- Implements `Processor` and `BlockProcessor`

**Browser**: Native `DynamicsCompressorNode` feeding a `WaveShaperNode` soft-clip limiter (`ensureLimiter`): linear to ±`SoftClipThreshold` (0.9), then `0.9·tanh(x/0.9)`, clamped to ±1. Desktop applies the same soft-clip in the final conversion loop (`engine_stop.go`).

**Files**: `compressor.go`, `compressor_test.go`, `chain_spec.go`, `audio.js` `ensureCompressor()` / `ensureLimiter()`

---

### WASM (browser) Details

**Flow**: Go → `platformInsertEffectsChanged(id, chain)` → `audio.js` `updateInsertEffects(id, chainJSON)` → the channel's `beatmo-dsp` node (built with its first effect, then reconciled in place by message) → `dsp_host.js` `InsertChain.update` (C instances by uid). A param edit: `platformInsertEffectParamChanged(id, uid, name, value)` → `retuneInsertEffect` → `ifx_set`. Where AudioWorklet does not exist (an insecure origin) the same `dsp_host.js` stage runs on the main-thread module in a ScriptProcessor.

**Files**: `src/js/dsp_worklet.js`, `src/js/dsp_host.js`, `src/js/audio.js` (`updateInsertEffects`, `retuneInsertEffect`, `rewireChannel`)

---

### Creating a New Insert Effect

1. **C**: Add `ifx_myeffect_t` struct + `_init()/_process()/_reset()/_set_param()` to `insert_fx.h` / `insert_fx.c` — per-sample state only (block-size invariant)
2. **Host**: Add one `FX_ADAPT` + init adapter + `FX_ROW` (type, param names in init order, state type, buffer size) to `fx_host.c` — both platforms build it from that row
3. **Desktop**: one-line `newMyEffect` in `insert_fx_c.go` returning `newCEffect(EffectMyEffect, sr, p)`; pass-through stub in `insert_effects_js.go`
4. **Tests build**: pure Go stand-in in `fx_myeffect.go` (build tag `test`)
5. **Registry**: `EffectType` const, `EffectTypes` entry and parameter definitions (`effect_registry_entries.go`) — `TestEffectParamNamesMatchTheRegistry` checks the C names match
6. **Browser**: nothing — the worklet runs the C. `xplat_audio_stages` picks the new type up from `audio.EffectTypes` and compares it at four settings
