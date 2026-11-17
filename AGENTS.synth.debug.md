## Synth Pipeline: Debugging & Observability

Reference for diagnosing audio issues. For pipeline architecture, see `AGENTS.synth.pipeline.md`.

---

### Signal Isolation Env Vars

These bypass pipeline stages to narrow down where a problem originates. Apply via environment variable before running. The test/single-voice vars were renamed with a `BEATMO_` prefix (`internal/env/vars.go`, "Was TEST_TONE." etc.) and the old names are NOT read; the `BYPASS_*`, `AUDIO_CLIP_DEBUG` and `DEBUG_*` names are unchanged.

| Var | What It Does | Use When |
|-----|-------------|----------|
| `BEATMO_TEST_TONE=1` | Pure 440Hz sine wave, bypasses all synth and mixing | Verifying oto/driver layer produces any sound at all |
| `BEATMO_TEST_VOICE=1` | Simple 220Hz sine per drum hit, bypasses C synth | Testing voice scheduling and mixer path without C render complexity |
| `BEATMO_TEST_RAW_VOICE=1` | Output first voice's raw samples, bypass all channel processing | Testing buffer integrity — are C-rendered samples correct? |
| `BEATMO_TEST_RAW_VOICE_FULL=1` | With `BEATMO_TEST_RAW_VOICE`: full amplitude (1.0x instead of 0.2x) | Testing if attenuation is masking a signal issue |
| `BEATMO_TEST_RAW_VOICE_NOSCALE=1` | With `BEATMO_TEST_RAW_VOICE`: skip all multiplication (direct to int16) | Isolating whether any Go-side scaling is corrupting the signal |
| `BEATMO_SINGLE_VOICE=1` | Limit mixer to one voice at a time | Isolating multi-voice accumulation or clipping issues |
| `BYPASS_CHANNEL_PROC=1` | Skip all channel processing (volume, inserts, EQ) | Isolating mixer vs channel processing issues |
| `BYPASS_EQ=1` | Skip EQ/processor processing only | Isolating EQ-specific distortion or biquad corruption |
| `BYPASS_HEADROOM=1` | Skip per-voice 0.18x headroom attenuation (`VoiceHeadroom`, `chain_spec.go`) | Testing if headroom is too aggressive (instruments too quiet) |
| `BYPASS_NORMALIZE=1` | Skip peak normalization during export | Testing export-specific amplitude issues |

**Files**: declared in `src/go/internal/env/vars.go`; read in `src/go/internal/audio/engine_stop.go` (most), `engine_mixer.go` (`BEATMO_TEST_VOICE`, in `Schedule`), `channels.go` (`BYPASS_EQ`, `BYPASS_CHANNEL_PROC`), `export.go` (`BYPASS_NORMALIZE`)

---

### Logging Env Vars

| Var | What It Logs | Use When |
|-----|-------------|----------|
| `AUDIO_CLIP_DEBUG=1` | Clipping events: samples exceeding +/-1.0 before hard clamp | Diagnosing distortion — shows exactly which samples clip |
| `DEBUG_MIXER=1` | workBuf min/max values and active voice stats, periodically | Understanding signal levels at master output stage |
| `DEBUG_CHANNEL=1` | Per-channel processing values (volume, processor output) | ⚠️ Only read in `Channel.ProcessSample`, which the block mixer doesn't call — expect no output from the live mixer |
| `BEATMO_DEBUG_INST=1` | Instrument menu operations | Debugging instrument registration/lookup issues |

**File**: `src/go/internal/audio/engine_stop.go` (mixer/clip), `channels.go` (channel); `BEATMO_DEBUG_INST` is read in `internal/ui/drumview_instruments.go`

---

### Diagnostic Flowcharts

#### Problem: Instrument Too Quiet

```
1. BEATMO_TEST_TONE=1 → silent?
   YES → oto/driver issue, not synth. Check system audio, BEATMO_AUDIO_SAMPLE_RATE.
   NO  ↓
2. BEATMO_TEST_VOICE=1 → audible at expected level?
   NO  → mixer/scheduling issue. Check voice is being scheduled.
   YES ↓
3. BEATMO_TEST_RAW_VOICE=1 → C render output level?
   LOW → C synth generating quiet samples. Check the voice in modular_stages.c and the instrument's document (internal/assets/instruments/<id>.json).
   OK  ↓
4. BYPASS_HEADROOM=1 → louder now?
   YES → headroom (0.18x) is correct but too aggressive for this use case.
         Consider: adjusting voice amplitude in C render, or channel volume.
   NO  ↓
5. BYPASS_EQ=1 → louder now?
   YES → EQ is cutting signal. Check band gains, especially if any are negative dB.
   NO  ↓
6. BYPASS_CHANNEL_PROC=1 → louder now?
   YES → insert effect chain is attenuating. Check effect mix/gain params.
   NO  → DEBUG_MIXER=1, check workBuf min/max. Signal may be fine but output
         conversion (float64→int16) is the issue.
```

#### Problem: Crackling / Distortion

```
1. BEATMO_TEST_TONE=1 → clean sine?
   NO  → driver/oto buffer underrun. Check BEATMO_AUDIO_SAMPLE_RATE, buffer sizes.
   YES ↓
2. BEATMO_SINGLE_VOICE=1 → clean single hits?
   YES → multi-voice accumulation clipping. Too many voices × headroom.
         AUDIO_CLIP_DEBUG=1 to confirm. Consider: fewer simultaneous voices,
         lower per-voice amplitude, or adjust mixHeadroom.
   NO  ↓
3. BYPASS_CHANNEL_PROC=1 → clean now?
   YES → insert effect or EQ causing distortion.
         BYPASS_EQ=1 to isolate: if clean, biquad issue; if still dirty, insert effect.
   NO  ���
4. BEATMO_TEST_RAW_VOICE=1 → clean C render output?
   NO  → C synth producing distorted samples. Check saturation/drive in render function.
   YES → mixer accumulation issue. DEBUG_MIXER=1, check workBuf ranges.
```

#### Problem: Clicks / Pops

```
1. Clicks on note start?
   → Check antiPopVoice wrapping (voice_antipop.go). Every voice should be wrapped
     at Schedule() time. Verify fade-in is 5ms / 220 samples.

2. Clicks on note stop / mute?
   → Check RequestStop() is called (not immediate removal). Verify 20ms fade-out
     completes before voice is dropped.

3. Clicks when toggling effects?
   → Expected today on desktop: the channel crossfade (channelXfadeSamples = 220)
     is only armed via ProcessSampleLocal (samplesThruOld), which the block mixer
     never calls — so chain swaps are NOT crossfaded during playback.
   → BYPASS_CHANNEL_PROC=1 → no clicks? Confirms processor chain swap issue.

4. Clicks in browser only?
   → Check audio.js anti-pop fade GainNode. Verify linearRampToValueAtTime is
     being called on source start. Check stopSound() fade-out timing.
```

#### Problem: Wrong Instrument Sound / Silent Instrument

```
1. Verify the instrument's document (internal/assets/instruments/<id>.json, or its
   project copy via audio.InstrumentDoc) and, in the browser, that Go pushed its block
   (window.__instrumentBlockForTest(id)).
2. Check voice cache: a wrong paramsHash key could serve a cached buffer for different params.
3. Run with BEATMO_DEBUG_INST=1 to log instrument lookup.
4. Every instrument renders through render_modular_p, the engine's only entry point
   (NULL params render silence; there are no C defaults). In the browser an id renders
   ONLY once Go has pushed its block (setInstrumentBlock); before that it has no voice
   at all — see AGENTS.synth.wasm.md.
```

---

### Browser-Side Diagnostics

#### Built-in Functions (available in browser console or test code)

| Function | Returns | Use When |
|----------|---------|----------|
| `perfStats()` | Frame timing: fps, update/draw durations | Checking if audio scheduling is starved by frame drops |
| `resetPerfStats()` | — | Reset before a measurement window |
| `getAudioScheduleMetrics()` | Lead time, lag, overdue event counts | Diagnosing late/missed audio events |
| `exportAudioHistoryCSV()` | CSV string of per-second scheduling history | Detailed timing analysis over time |
| `dumpRowState` | Sequencer row state | Checking if hits are being scheduled at all |
| `dumpTimelineSegments` | Timeline segment data | Verifying timeline/predictor state |

#### AnalyserNodes

Three AnalyserNodes exist per channel (when created), all wired IN-LINE:
- **Synth** (`synthAnalyser`): before the insert effects (scope StageSynth / StageAntiPop)
- **Pre-EQ** (`preEQAnalyser`): after inserts, before EQ (scope StageInsertFX)
- **Post-EQ** (`analyser`): after EQ

These can be tapped for spectrum analysis (FFT) or time-domain waveform capture. Useful for visualizing where in the per-channel chain signal is being altered.

#### Output Capture

`startOutputCapture()` (no arguments) / `stopOutputCapture()` captures the final mixed output AFTER the limiter (limiter → mono ScriptProcessor capture node → destination). Returns Float32Array of captured samples. Used for testing and bit-accuracy verification against desktop.

**File**: `audio.js`

---

### Observability Tap Points

These are locations in the signal pipeline where signal can be measured or captured. Listed in signal flow order:

| Tap Point | Platform | What You See | How to Access |
|-----------|----------|-------------|---------------|
| Post-C-render | Both | Raw synthesized voice buffer (float32) | `BEATMO_TEST_RAW_VOICE=1` (desktop), or inspect `ensureRenderedSample()` cache (browser) |
| Post-voice-headroom | Desktop | Voice after 0.18x attenuation | `instBufs[slot]` in `engine_stop.go` Phase 1 |
| Per-instrument sum | Desktop | All voices for one instrument summed | `instBufs[slot]` after Phase 1 completes |
| Post-inserts | Desktop | After volume + inserts, before EQ | `postFXBuf` in Phase 2a |
| Post-channel-processing | Desktop | After volume + inserts + EQ | `postEQBuf` in Phase 2b (per instrument, before masterBuf accumulation) |
| Post-master | Desktop | After master EQ + compressor | `workBuf` after Phase 3 |
| Pre-clamp | Desktop | Signal before the 0.9 soft-clip + [-1,1] safety clamp | `AUDIO_CLIP_DEBUG=1` logs clipping here |
| Pre-EQ (browser) | Browser | Per-channel signal before EQ | `AnalyserNode` (pre-EQ) |
| Post-EQ (browser) | Browser | Per-channel signal after EQ | `AnalyserNode` (post-EQ) |
| Final mix (browser) | Browser | Mono output after the limiter | `startOutputCapture()` / `stopOutputCapture()` |
| All stages (export) | Desktop | All 6 stages for all instruments + master | `SCOPE_EXPORT=1` → JSONL file (see Scope Export section) |

---

### Scope Export Flight Recorder

A continuous JSONL exporter that captures all 6 pipeline stages for every instrument + master, independent of the scope UI.

**Package**: `src/go/internal/scopeexport/`

**Activation**: `-scope-export` CLI flag or `SCOPE_EXPORT=1` env var.

| Env Var | Default | Purpose |
|---------|---------|---------|
| `SCOPE_EXPORT=1` | off | Enable flight recorder |
| `SCOPE_EXPORT_PATH` | `scope_export.jsonl` | Output file path |
| `SCOPE_EXPORT_INTERVAL` | `2` | Seconds between snapshots |

**What each snapshot contains** (one JSON line, ~5-10KB):
- Per-instrument: peak/RMS dB, clip count, 64-point waveform (min/max pairs), top-16 FFT bins (Hz + dB), zero-crossing rate — at each per-instrument stage (synth, antipop, insertfx, eq)
- Master: same metrics at the master stage
- Metadata: BPM, sample rate, per-channel volume/pan/name/kind

**Pipeline stages captured** (push points in `engine_mixer.go` and `engine_stop.go`):

| Stage | ID | Location | Signal |
|-------|----|----------|--------|
| StageSynth | 0 | `engine_mixer.go` Schedule() | Raw C synth buffer (float32→float64) |
| StageAntiPop | 1 | `engine_stop.go` Phase 1 post | Per-instrument after headroom (before EQ) |
| StageInsertFX | 2 | `engine_stop.go` Phase 2a | Per-instrument after volume + inserts, before EQ (via `postFXBuf`) |
| StageEQ | 3 | `engine_stop.go` Phase 2b | Per-instrument after volume + inserts + EQ (via `postEQBuf`) |
| StageMaster | 4 | `engine_stop.go` Phase 3 post | Final master output |

**Analysis with jq**:

```bash
# Latest snapshot
tail -1 scope_export.jsonl | jq .

# Track kick peak_db at synth stage over time
jq -r '.channels[] | select(.id=="kick") | .stages.synth.peak_db' scope_export.jsonl

# Master RMS trend
jq -r '[.tick, .master.stages.master.rms_db] | @tsv' scope_export.jsonl

# Detect clipping at any stage
jq 'select([.channels[].stages[].clip_count] | add > 0)' scope_export.jsonl

# Compare pre-EQ vs post-EQ
jq -r '.channels[] | select(.id=="kick") | [.stages.antipop.peak_db, .stages.eq.peak_db] | @tsv' scope_export.jsonl

# Feed last 5 snapshots to LLM for analysis
tail -5 scope_export.jsonl | jq -s .
```

**Architecture**: The service owns its own ring buffers per (stage, instrument) pair. Samples are pushed from the audio thread via nil-gated calls (`if exportSvc != nil`). A ticker goroutine drains buffers, runs wave observers (PeakRMS, FFT, ZeroCrossing), downsamples waveforms, and appends JSONL. Zero overhead when disabled.

---

### Common Pitfalls

- **Biquad state corruption**: If you ever see distortion that only appears when multiple instruments play simultaneously, suspect biquad interleaving. The 3-phase mixer architecture prevents this, but regressions could reintroduce it. See `AGENTS.synth.pipeline.md` for the full explanation.

- **Voice cache serving wrong sound**: If `synthParams` changes but the cache key doesn't include the changed field, stale cached buffers are served. Verify `voiceCacheKey` includes all relevant fields.

- **Crossfade not triggering**: The `samplesThruOld` atomic flag in `channels.go` tracks whether audio actually flowed through the current processor chain. If no audio played since the last chain swap, crossfade is skipped (correctly). But if you're testing with continuous audio and still hear clicks on effect toggle, the crossfade window (220 samples / ~5ms) may be too short for your effect.

- **Browser scheduling lag**: If `getAudioScheduleMetrics()` shows many overdue events, the main thread is too busy. Check `perfStats()` for frame timing. Consider reducing `AUDIO_FLUSH_CHUNK` or increasing `AUDIO_FLUSH_BUDGET_MS`.

- **Mobile silence**: AudioContext may be suspended. Check `ctx.state` in console. Ensure user gesture triggered `unlockAudio()`. See `AGENTS.synth.wasm.md` for the mobile unlock flow.
