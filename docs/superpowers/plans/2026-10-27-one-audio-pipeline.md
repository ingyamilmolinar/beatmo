# One Audio Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** every stage between a voice and the speaker computes the same samples on desktop and in the browser, proven per stage by cross-platform tests.

**Architecture:** the C engine gains one generic insert-effect API, an EQ DSP and the soft clip; desktop calls them through cgo, the browser through `drums.single.js` inside one AudioWorklet module (`beatmo-dsp`). The browser plays desktop's exact played-voice samples (JS mirrors of the Go resampler/gate/fade, pinned per sample) at 48 kHz. A desktop exporter renders stage cases fresh; a browser test renders the same cases through audio.js on an OfflineAudioContext and compares every tap.

**Tech Stack:** Go 1.26 (cgo + js/wasm), C (gcc + Emscripten), WebAudio/AudioWorklet, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-27-one-audio-pipeline-design.md`

## Global Constraints

- Sample rate 48 000 Hz on both platforms (`CHAIN_SPEC.sampleRate`).
- Headroom 0.18 per voice; fade-in 5 ms linear (`floor(sr·5/1000)` frames, factor i/N); stop fade 20 ms linear; gate release 10 ms (`min(floor(sr·10/1000), limit/2)`, ≥ 1).
- Soft clip: |x| ≤ 0.9 → x; else sign(x)·(0.9 + 0.1·tanh((|x|−0.9)/0.1)).
- No tempo input on any audio path. Never stage/commit (user rule). Never `git stash`.
- Every test Game closed; probes via `go -overlay`; the desktop reference is rendered fresh, never committed.

## Review Focus

1. Worklet module unavailable (insecure origin, old engine) → the same C on a ScriptProcessor; FX/EQ/clip still applied, never dry-silent. Test: Task 5 `dsp fallback` case forces `__beatmoDSPHost='scriptprocessor'`.
2. Fixed WASM heap exhausted by many delay/reverb slots → `ifx_create` returns 0 → that slot passes through and is reported, never NaN/silence. Test: Task 1 `TestEffectCreateFailurePassesThrough`.
3. A note quieter than 1/32 (row 0.25 × node 0.1) must sound on both. Test: Task 8 played case vol 0.025.
4. A WAV at 44.1 or 96 kHz plays at the right pitch/length on both. Test: Task 3 `TestForeignRateSampleIsResampled` + Task 8 sample case.
5. A param edit on a ringing effect keeps its state (no reset click) on both. Test: Task 1 `TestSetKeepsState` (C) + Task 8 live-edit case.

---

### Task 1: C stage API (effects, EQ, soft clip)

**Files:** Create `src/c/fx_host.c`, `src/c/fx_host.h`, `src/c/eq_dsp.c`, `src/c/eq_dsp.h`; Modify `Makefile` (C_SRC, emcc EXPORTED_FUNCTIONS); Test `src/go/internal/audio/c_stage_native_test.go` (`!test && !js`).

**Interfaces — Produces (C):**
- `const char *ifx_param_names(const char *type)` — comma list in init order, `""` unknown type.
- `void *ifx_create(const char *type, int sr, const float *values, int n)` — 0 on unknown type / malloc failure.
- `void ifx_run(void *h, const float *in, float *out, int n)`; `void ifx_set(void *h, const char *name, float v)`; `void ifx_clear(void *h)`; `void ifx_destroy(void *h)`; `int ifx_latency(void *h)`.
- `void *eq_create(const double *prog, int n)`; `int eq_update(void *h, const double *prog, int n)` (1 = same shape, coefficients swapped, state kept; 0 = refused); `void eq_run(void *h, const float *in, float *out, int n)`; `void eq_clear(void *h)`; `void eq_destroy(void *h)`.
- Program layout (doubles): `[nStages, (kind, ...)*]`; kind 0 = biquad `b0 b1 b2 a1 a2`; kind 1 = split `nBands, (muted, gain, nFilt, (b0..a2)*nFilt)*nBands`; kind 2 = silence.
- `void soft_clip_run(const float *in, float *out, int n)`.

- [ ] Tests: `TestEveryEffectIsBlockSizeInvariant` (18 types × defaults + an extreme set, 1 s of a fixed signal, blocks 1/64/128/97 → identical floats); `TestEffectCreateFailurePassesThrough` (unknown type → 0); `TestSetKeepsState` (delay: set "mix" mid-stream ≠ a fresh instance); `TestEQProgramMatchesGoEQ` (scratch overlay oracle once, then analytic: magnitude at each band centre within 0.05 dB of `EQResponse`); `TestSoftClipIsContinuousAndBounded`.
- [ ] Implement; `make` C lib + `make wasm` builds; tests pass.

### Task 2: Desktop channel + master on the C stages

**Files:** Modify `internal/audio/insert_fx_c.go` (one `cEffect` replaces 18 wrappers), `insert_effects.go` (constructor by type), `eq.go` (C EQ processor), `eq_program.go` (new, no build tags: `BuildEQProgram(sr int, bands []EQBand) []float64`), `effect_chain.go`, `compressor.go` (delete master compressor + `SetupMasterCompressor`), `engine_stop.go` (soft clip via shared Go mirror `softClip`, capture after clip), `channels.go` (delete per-instrument volume), `chain_spec.go`, `voice_fx.go` (voice FX through `ifx_create`).

**Interfaces — Produces:** `BuildEQProgram(sr int, bands []EQBand) []float64`; `EffectParamsFull(t EffectType, p map[string]float64) map[string]float64`; `SoftClip(x float64) float64` (Go mirror for the `-tags test`/wasm analysis paths; C on device).

- [ ] Tests: `TestHPFAndMutedBandKeepEveryFilter` (HPF+LPF+muted band program = HPF, split of 10, LPF); `TestEQEditKeepsFilterState`; `TestMasterHasNoCompressor`; `TestOutputCaptureIsAfterSoftClip`; existing export→import identity + `TestEveryDSPStageRendersIdenticallyOnReplay` stay green.
- [ ] Implement; `go test -tags test ./internal/audio/...` and `make test-real` audio subset pass.

### Task 3: Desktop voice path + 48 kHz + samples

**Files:** Modify `engine_stop.go` (mid-block start), `engine_resample.go` (positions i·step, no dropped sample), `sample_edit.go` (`resampleByStep` is the one resampler), `voice_antipop.go` (linear stop, `RequestStopAt(pos int)`), `lane_mixer.go` (exact-sample stops), `env` default sample rate 48000, `sample_desktop.go` / `sample_pcm_notjs.go` (resample at registration).

- [ ] Tests: `TestMixerVoiceIsThePlayedVoice` (one voice at start 1000+37 = RenderAsPlayed × vol × headroom with fade-in, every sample); `TestLaneStopLandsOnItsSample`; `TestForeignRateSampleIsResampled` (44.1 and 96 kHz WAV/PCM → engine-rate length, f0 within 1 cent); `TestEngineRunsAt48k`.
- [ ] Implement; regenerate what the rate change moves (`make gen-dial-relevance`, `gen-fundamentals`, goldens) only where a test names it.

### Task 4: Config from Go

**Files:** `chain_spec.go`, `cmd/gen-chain-spec`, `src/js/chain_spec.gen.js`; delete `src/js/audio_config.generated.js`, `MixHeadroom`, `exportJSConfig`.

- [ ] `CHAIN_SPEC = {sampleRate, voiceHeadroom, softClipThreshold, fadeInMs, stopFadeMs, gateFadeMs, maxMasterVolume}`; `TestChainSpecGenFileIsCurrent`; `TestAudioJSHasNoMixLiterals` (no sample-rate or fade-time literal outside CHAIN_SPEC in audio.js).

### Task 5: Browser DSP worklet + graph

**Files:** Create `src/js/dsp_host.js` (shared `ChannelDSP`: inserts | eq | softclip over a Module), `src/js/dsp_worklet.js` (`beatmo-dsp`, static `import Module from './drums.single.js'`); Modify `audio.js` (channel/master wiring, delete WebAudio FX/EQ/compressor/limiter/buses), `scripts/deploy-wasm-gcs.sh` (manifest); delete `insert_fx_worklet.js`, `insert_fx_kernels.js`.

**Interfaces — Consumes:** Task 1 C API, Task 2 payloads (`eq` program Float64Array, slots with full params). **Produces:** `updateInsertEffects(id, slotsJSON)`, `setInsertEffectParams(...)`, `setChannelEQ(id, program)` keep their names; `audioGraphStats()` keeps its counters.

- [ ] Browser tests: the stage harness (Task 8) + `webaudio_live_edit_budget` (graph churn stays 0) + `dsp fallback` case.

### Task 6: Browser voice

**Files:** Create `src/js/played_voice.js` (`playedBuffer(data, sr, {melodic, pitch, dur})`, `resampleByStep`, `gate`, `bakeFadeIn`); Modify `audio.js` (`processAudioEvent`, `startRenderVoice`, stops, `createCtx` 48 kHz, exact melodic pitch cache, WAV decode at native rate).

- [ ] Covered by Task 8 played/stop/sample cases (all three engines).

### Task 7: Go WASM bridge

**Files:** `eq_wasm.go` (push `BuildEQProgram`), `insert_effects_wasm.go` (full params), `channels_wasm.go` (master clamp), `sample_pcm_wasm.go` (resample before push), `sample_wasm.go` (native-rate decode → `RegisterSamplePCM`).

- [ ] `js_bridge_contract_discipline_test.go` covers new args; Task 8 circuit cases prove the payloads.

### Task 8: Cross-platform stage harness

**Files:** Modify `cmd/export_audio/main.go` (`--stages <sr>`, `--circuit <path> <seconds>`), `engine_stop.go` (stage capture taps), `audio.js` (`__offlineRenderForTest`); Create `src/js/xplat_audio_stages.browser.test.js`.

- [ ] Cases per spec § Tests; per tap: frames equal, worst |Δ| ≤ 2e-5, bit-exact count printed; Chromium + Firefox + WebKit (`XPLAT_ENGINES`).

### Task 9: Clean-up, docs, full runs

- [ ] Remove/rewrite tests of deleted code; update CLAUDE.md, `docs/master-mix-dsp.md`, AGENTS.synth.*; `go test -tags test ./...`, `make test-real`, `make js-audit`, relevant browser suites, wasm size budget.
