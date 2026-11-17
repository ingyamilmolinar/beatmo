# One audio pipeline on both platforms

**Date:** 2026-10-27 · **Status:** approved in chat ("Yes, makes sense. Let's make
everything consistent! Test thoroughly please and write some tests to compare
audio pipeline stages in both platforms") · **Audit it follows:** the 10-27
desktop↔browser audit (sample rate, volume rounding, 18 re-implemented effects,
EQ shape, dead master compressor, soft-clip jump, sample-rate bugs).

## Intent

The user's rule (10-27): *the same circuit sounds exactly the same at any tempo
in both platforms. This is paramount.* The voice already matches (fixed note
length work). Everything after the voice did not: the browser re-implemented
the row effects, the EQ and the master bus in WebAudio, ran at the device rate,
rounded note volumes, and nothing compared its mix with desktop's. This makes
every stage between the voice and the speaker the SAME computation on both
platforms, and adds tests that compare each stage, sample by sample.

## Goals

1. Every DSP stage after the voice runs one implementation on both platforms.
2. Both platforms run at 48 kHz.
3. A note is the same samples on both platforms: pitch, length gate, volume,
   fade-in, start sample, stop fade.
4. Per-stage, per-sample cross-platform tests (voice → inserts → EQ → master →
   whole circuit), with the desktop reference rendered fresh from the tree.
5. Every value both platforms use comes from Go (one generated table).

## Non-goals / what stays different by construction

- **Real-time timing**: when a live edit or a mute reaches the audio thread
  (desktop: the next 64-frame block; browser: the next message/quantum). The
  computation is the same; the moment it lands is wall-clock.
- **The stale-render policy after an edit** (browser plays the old timbre on
  time while the new one renders; desktop renders synchronously). A scheduling
  trade-off, not DSP.
- **Insecure origins** (`make serve-lan` over plain HTTP): AudioWorklet does not
  exist there, so the same C runs on a main-thread ScriptProcessor fallback,
  which adds its buffer latency. beatmo.io is HTTPS.
- Kits: untouched (user, 10-27).

## Anti-goals

- Two implementations "kept close" by a loose (correlation/RMS) test. That is
  what let every audited bug through.
- A test that compares the browser with itself.
- Any audio path that reads the BPM (the 10-27 rule stands).

## Decisions (the how)

### Sample rate
48 000 Hz everywhere: desktop engine default (`BEATMO_AUDIO_SAMPLE_RATE`
still overrides), `new AudioContext({sampleRate: 48000})` (falls back to the
device rate only where the option is refused), every browser render at
`CHAIN_SPEC.sampleRate`.

### Voice → mix ("played voice" stage)
The browser plays exactly the samples desktop computes:
- One linear resampler (Go `resampleByStep` semantics: position = i·step, hold
  the last sample) for drum pitch/length, sample-rate conversion and Sampler
  pitch. Desktop's streaming `resampleVoice` uses the same positions and stops
  dropping one sample per 64-frame block (its SampleBlock "probe" consumed one).
- Melodic voices render at the EXACT node pitch on both (the browser's
  half-semitone cache rounding and residual playbackRate go); a Duration < 1
  gates them with desktop's gate (`floor(len·dur)` frames, 10 ms linear release).
- The browser builds that played buffer in JS (pure functions mirroring the Go
  ones, pinned per sample by the stage test), bakes desktop's 5 ms linear
  fade-in into it, and plays it at playbackRate 1 through ONE gain node =
  headroom × note volume. The 1/16 volume buses go.
- Start sample: desktop starts a voice mid-block at its exact sample (it waited
  for the next 64-frame block); the browser starts on a frame boundary.
- Stops (mute node, lane stop): a 20 ms LINEAR fade on both, from the exact
  stop sample; the browser's fade starts from the voice's own level.

### Row effects (inserts)
One C API for all 18 effects (`ifx_create(type, sr, values)`, `ifx_run`,
`ifx_set`, `ifx_clear`, `ifx_destroy`, `ifx_param_names`): C owns construction
(arg order, buffer sizes); Go owns defaults (every slot carries its full
parameter map). Desktop calls it through cgo (one generic wrapper replaces 18),
the browser through `drums.single.js` loaded ONCE per AudioWorkletGlobalScope
(`beatmo-dsp` worklet, one node per channel chain). All JS/WebAudio effect
implementations are deleted. A param edit is a message (C `set_param`, state
kept — desktop `SetParam`); a chain edit rebuilds the C chain inside the same
node.

### EQ
One Go function turns a channel's bands into an EQ program (HPF → graphic
section → LPF, in series; the graphic section is a biquad chain, a
Linkwitz–Riley band split when a band is muted, or silence), with coefficients
from the shared `ComputeBiquadCoeffs`. One C DSP applies it on both platforms
(float I/O per filter, double state — desktop's existing arithmetic). Fixes:
WebAudio's shelf-ignores-Q / Q-in-dB / notch-bleed, and HPF/LPF + a muted band
(broken on both). An edit with the same program shape updates coefficients in
place and keeps filter state on both platforms.

### Master
Master volume → master EQ → soft clip → output. The master compressor is
deleted (dead on both platforms after the first master EQ write, which every
import makes). Soft clip becomes continuous and smooth: identity up to
T = 0.9, then `T + (1−T)·tanh((|x|−T)/(1−T))` (C, both platforms; the browser's
WaveShaper pinned inputs above 1). Desktop's output capture moves after the
soft clip (it is what the device receives). One master-volume clamp, in Go.
Per-instrument channel volume stays: nothing sets it (always 1 on both
platforms), so where each platform applies it cannot change a sample.

### Samples
Every PCM that enters an engine is converted to the engine rate by the one Go
resampler before it is registered, on both platforms. Desktop stops installing
silence for a WAV at another rate and stops ignoring the rate of embedded PCM;
the browser decodes WAVs at their own rate (not `decodeAudioData`'s resampler)
and hands the PCM to Go.

### Config
`chain_spec.go` → `chain_spec.gen.js` carries every value both platforms use
(sample rate, headroom, soft-clip knee, fade/gate/stop times, volume clamp).
Dead copies go (`audio_config.generated.js`, `MixHeadroom`, compressor fields).

## Tests

- **Cross-platform stages** (`xplat_audio_stages.browser.test.js` +
  `cmd/export_audio --stages`): desktop renders each case fresh through the
  real offline mixer with per-stage taps; the browser renders the same case
  through its real graph (audio.js on an OfflineAudioContext, the real worklet)
  and every tap is compared per sample (bit-exact count + worst |Δ|, tight
  bound). Cases: played voices (drums pitched/sized, melodic pitched/gated,
  volumes incl. < 1/32, odd start samples, overlaps, lane stops), every insert
  effect at several settings, EQ (boost/cut, shelves, HPF, LPF, muted band,
  muted + filters, all muted), master (EQ, volume, soft-clip region), chains.
- **Whole circuits**: the startup demo, the test fixture and templates
  imported through Go WASM in the browser, rendered offline, compared with
  desktop's `RenderMasterMixOffline`. This also proves Go pushes the payloads
  the stage cases use.
- **Native invariants** (`make test-real`): every C stage is block-size
  invariant; a mixer voice == `RenderAsPlayed` × volume × headroom from any
  start sample; lane stops land on the exact sample; soft clip continuous and
  bounded; foreign-rate samples resampled, never silent.
- Obsolete tests that compared the browser with itself or exercised the
  deleted JS effects are removed or rewritten against the new stages.

## Left to the builder

File layout, the offline-render test seam, case lists and tolerances (as tight
as the libm/compiler seam allows — the voice stage measured ≤ 5.6e-6).
