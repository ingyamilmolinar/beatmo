# Wave Toolkit: Composable Waveform Observability & Transformation

**Date**: 2026-04-03
**Status**: Approved
**Scope**: Internal dev tools first, user-facing later

---

## Problem

Beatmo's synth pipeline has 15+ env-var debugging flags but no visual tooling for diagnosing audio issues. Common questions — "why is this instrument too quiet?", "where is the crackling coming from?", "what does the envelope look like?" — require manual env-var toggling and log reading. There is no way to inspect a rendered waveform, compare pre/post-effect signals, or monitor all instruments simultaneously.

## Solution

A composable wave toolkit built on four primitives — **Wave**, **Source**, **Transform**, **Observer** — that chain together for both static inspection and live monitoring. The same pipeline model applies to synth rendering and effect application, enabling offline waveform analysis and real-time monitoring through the same API.

## Design Principles

1. **Zero audio-thread impact**: The audio thread only memcopies buffers. All analysis runs on a dedicated goroutine.
2. **Composable pipeline**: Sources, transforms, and observers chain together. The same transform wrapping a C insert effect works for both live audio and offline inspection.
3. **Cross-platform from day one**: Go/Ebiten rendering. No browser APIs. Works on desktop and WASM.
4. **Minimize clicking**: Meter bridge shows all instruments at once. No per-instrument selection required for monitoring.
5. **Collapsed by default**: The panel maximizes info density at 190px, expands to ~380px on demand.

---

## Architecture

### Layer 1: Core Data (`src/go/internal/wave`)

Pure data layer. No dependencies on the audio engine, UI, or platform. Fully testable in isolation.

#### Wave

The fundamental unit — a buffer of samples with metadata.

```go
type Wave struct {
    Samples    []float64
    SampleRate int
    Label      string
}
```

Helper methods on Wave: `Duration()`, `PeakSample()`, `Slice(startMs, endMs)`, `Clone()`.

#### Source

Produces a Wave. Implementations live in Layer 2.

```go
type Source interface {
    Render() Wave
}
```

#### Transform

Takes a Wave, returns a new Wave. Chainable. Must not mutate the input.

```go
type Transform interface {
    Apply(w Wave) Wave
}
```

#### Observer

Reads a Wave non-destructively, returns metrics. Multiple observers can fan out from the same Wave.

```go
type Observer interface {
    Observe(w Wave) Observation
}
```

#### Observation

Tagged result from an observer. Each observer populates the fields relevant to its kind.

```go
type ObsKind int

const (
    ObsFFT ObsKind = iota
    ObsPeakRMS
    ObsEnvelope
    ObsZeroCrossing
)

type Observation struct {
    Kind         ObsKind
    Spectrum     []float64  // FFT magnitudes in dB (ObsFFT)
    FreqBins     []float64  // corresponding frequency labels (ObsFFT)
    PeakDB       float64    // (ObsPeakRMS)
    RMSDB        float64    // (ObsPeakRMS)
    ClipCount    int        // samples exceeding +/-1.0 (ObsPeakRMS)
    Envelope     []float64  // amplitude envelope (ObsEnvelope)
    ZeroCrossRate float64   // crossings per second (ObsZeroCrossing)
}
```

#### Chain Compositor

Composes sources, transforms, and observers into a pipeline.

```go
// Chain applies transforms in order to a source, returns the resulting Wave.
func Chain(src Source, transforms ...Transform) Wave

// Observe runs multiple observers on a Wave, returns all observations.
func Observe(w Wave, observers ...Observer) []Observation

// Convenience: render + transform + observe in one call.
func Pipeline(src Source, transforms []Transform, observers []Observer) (Wave, []Observation)
```

### Layer 2: Implementations

#### Sources

**RenderSource** — calls a C synth render function, returns the full buffer as a Wave.

```go
func RenderSource(instID string, sampleRate int, params audio.SynthParams) Source
```

Internally calls the same `renderSnareP()` / `renderKickP()` / `renderFM*()` functions used by the voice cache, but returns the raw buffer instead of scheduling it. No audio engine dependency �� just CGo calls.

**PipelineTap** — reads from a ring buffer fed by the mixer during live playback. Used by the analyzer service.

```go
type TapPoint int

const (
    TapPostRender   TapPoint = iota  // Phase 1: per-instrument, post-headroom
    TapPostChannel                    // Phase 2: per-instrument, post-inserts+EQ
    TapPostSends                      // Phase 2.5: master, post-send effects
    TapPostMaster                     // Phase 3: master, post-compressor
)

func PipelineTap(point TapPoint, id string, ring *RingBuffer) Source
```

**CaptureSource** — returns a previously captured (frozen or last-triggered) waveform.

```go
func CaptureSource(buf *CaptureBuffer) Source
```

#### Transforms

All transforms produce a new Wave — no mutation.

**EffectTransform** — wraps any of the 18 C insert effects. Allocates a temporary C effect instance, processes the wave, frees the instance. Same C code used in the live audio path.

```go
func Effect(effectType string, params map[string]float64) Transform
```

**Normalize** — peak-normalize to a target dB level.

```go
func Normalize(targetDB float64) Transform
```

**Trim** — extract a time range from the wave.

```go
func Trim(startMs, endMs float64) Transform
```

**Resample** — linear interpolation to a target sample rate.

```go
func Resample(targetRate int) Transform
```

**Window** — apply a window function (for FFT preparation).

```go
type WindowType int
const (
    WindowHann WindowType = iota
    WindowHamming
    WindowBlackman
)

func Window(wt WindowType) Transform
```

#### Observers

**FFTObserver** — C Cooley-Tukey radix-2 FFT. Applies Hann window internally. Returns magnitude spectrum in dB.

```go
func FFT(size int) Observer  // size must be power of 2 (512, 1024, 2048)
```

Size defaults to 1024 (512 output bins). Computed via CGo call to `src/c/fft.c`. The observer windows the last `size` samples of the wave if the wave is longer than the FFT size.

**PeakRMSObserver** — computes peak level, RMS level, and clip count.

```go
func PeakRMS() Observer
```

**EnvelopeObserver** — amplitude envelope follower with configurable attack/release.

```go
func Envelope(attackMs, releaseMs float64) Observer
```

**ZeroCrossingObserver** — zero-crossing rate, useful for distinguishing noise-led from tone-led signals.

```go
func ZeroCrossing() Observer
```

### Layer 3: Analyzer Service (`src/go/internal/analyzer`)

Orchestrates live monitoring. Owns a goroutine and ring buffers. Publishes state atomically.

#### Ring Buffer Ingest

The audio thread pushes data via three methods. All are memcopies into pre-allocated SPSC (single-producer, single-consumer) ring buffers. Cost: ~512 bytes per instrument per block (64 samples x 8 bytes). Negligible.

```go
type Service struct { ... }

// Called from audio thread (engine_stop.go) — memcopy only
func (s *Service) PushInstBuf(slot int, buf []float64)
func (s *Service) PushMasterBuf(buf []float64)

// Called from mixer Schedule() — copies full rendered buffer for capture
func (s *Service) NotifyTrigger(instID string, rawBuf []float32)
```

#### Analyzer Goroutine

Wakes when any ring buffer accumulates enough data (~2048 samples, configurable). Per wake cycle:

1. Drain ring buffers into working copies
2. For each active instrument: run `PeakRMS()` observer
3. For master + selected detail channel: run `FFT()`, `PeakRMS()`, `Envelope()` observers
4. If a trigger notification arrived: update capture buffer
5. Atomic-swap publish new `State`

Wake rate: ~21/sec at 44.1kHz with 2048-sample analysis windows. FFT only runs on master + one detail channel to keep compute bounded.

#### Published State

```go
type State struct {
    Instruments []InstrumentMetrics  // all active instruments (for meter bridge)
    Master      ChannelMetrics       // master output
    Detail      *ChannelMetrics      // selected instrument's full metrics (FFT + envelope)
    Capture     *CaptureBuffer       // last-triggered or frozen waveform
    Timestamp   int64                // monotonic nanoseconds
}

type ChannelMetrics struct {
    ID        string
    Name      string
    PeakDB    float64
    RMSDB     float64
    ClipCount int
    Waveform  []float64   // rolling window for display
    FFTBins   []float64   // magnitude spectrum in dB (nil if not computed)
    FreqBins  []float64   // frequency labels for FFT bins
    Envelope  []float64   // amplitude envelope (nil if not computed)
    Active    bool        // had signal this window
}

type InstrumentMetrics struct {
    ID        string
    Name      string
    PeakDB    float64
    RMSDB     float64
    ClipCount int
    Active    bool
}

type CaptureBuffer struct {
    InstID     string
    Wave       wave.Wave   // full rendered buffer
    Frozen     bool        // user pinned this capture
    TriggerIdx int64       // monotonic trigger counter
}
```

UI reads state via:

```go
func (s *Service) State() *State  // atomic load, zero-copy read
```

#### Capture Control

```go
func (s *Service) Freeze()                                          // pin current capture
func (s *Service) Unfreeze()                                        // resume live capture
func (s *Service) RenderCapture(instID string, params SynthParams)  // on-demand render
func (s *Service) SetDetailChannel(instID string)                   // select for FFT/envelope
```

### Layer 4: UI Rendering (`src/go/internal/ui`)

#### Panel Structure

Replaces the current EQ/Wave toggle in `EQPanelZone`. The `waveformMode` bool becomes a `panelTab` enum.

```go
type PanelTab int
const (
    TabWave PanelTab = iota
    TabSpectrum
    TabMeters
    TabEQ
)
```

The panel has two height states:
- **Collapsed** (~190px, default): Shows the active tab's content. Tab bar at top with Expand button.
- **Expanded** (~380px): Active tab gets the top ~200px. Bottom ~180px shows secondary views (spectrum + meters for Wave tab, waveform + meters for Spectrum tab, etc.).

#### Tab Renderers

Each tab has a dedicated renderer that reads from `analyzer.State()`:

**WaveformRenderer** — draws waveform trace using `drawRect` per pixel column (same technique as current `drawWaveformTrace`, but enhanced):
- Pre-EQ dim trace + post-EQ bright trace (existing behavior)
- Inline peak/RMS/clip readout (top-right)
- Time axis labels (bottom)
- Amplitude scale (left edge)
- When capture is frozen: draws from `CaptureBuffer.Wave` instead of rolling window
- Capture controls: Freeze/Unfreeze button, Render button

**SpectrumRenderer** — draws FFT magnitude bars:
- Bars grouped by ISO frequency bands (31Hz to 16kHz)
- Logarithmic frequency axis
- dB scale on left
- Peak hold (slowly decaying peak marker per bin)

**MeterBridgeRenderer** — the full-mixer-at-a-glance view:
- One horizontal bar per active instrument, stacked vertically
- Each bar: instrument name (left), peak meter (green/yellow/red gradient), RMS overlay, clip indicator (right)
- Master meter at the bottom, visually separated
- Sorted by slot order (matches instrument list)
- No instrument selection needed — all visible simultaneously

**EQRenderer** — existing EQ slider UI, unchanged.

#### Expand/Collapse

- `expandBtn` in the tab bar toggles `panelExpanded bool`
- When expanded, the panel's `rect` grows by ~190px downward
- The grid layout above must accommodate this — `WidgetWave` row span increases
- Collapse restores to default 190px

### C FFT (`src/c/fft.c`, `src/c/fft.h`)

Cooley-Tukey radix-2 in-place FFT. ~60 lines of C.

```c
// In-place complex FFT. Input: interleaved [re0, im0, re1, im1, ...] of length 2*n.
void fft_complex(float *data, int n);

// Convenience: real input → magnitude spectrum in dB.
// Applies Hann window, computes FFT, returns n/2 magnitude bins.
// out must be at least n/2 floats. freqs must be at least n/2 floats (filled with Hz labels).
void fft_magnitude_db(const float *in, int n, int sample_rate, float *out, float *freqs);
```

CGo bridge in `src/go/internal/wave/fft_c.go`:

```go
func ComputeFFT(samples []float64, sampleRate int) (magnitudeDB []float64, freqBins []float64)
```

Build tags: CGo on desktop (`!test && !js`), pure Go fallback for tests and WASM (`test || js`). The Go fallback can be slower since it only runs in tests and on the analyzer goroutine (never on the audio thread).

### Integration Points

Minimal changes to existing audio code:

**`engine_stop.go`** — add 3 lines at end of Phase 1 and Phase 3:

```go
// End of Phase 1 (after voice rendering loop)
if analyzer != nil {
    analyzer.PushInstBufs(activeSlots, instBufs)
}

// End of Phase 3 (after master processing)
if analyzer != nil {
    analyzer.PushMasterBuf(workBuf[:blockSize])
}
```

**`engine_mixer.go`** — add 2 lines in `Schedule()`:

```go
// After antiPop wrapping, if voice has a raw buffer
if analyzer != nil && rawBuf != nil {
    analyzer.NotifyTrigger(id, rawBuf)
}
```

**`engine.go`** — init the analyzer service during audio init:

```go
var analyzer *analyzer.Service

func init() {
    analyzer = analyzer.NewService(analyzer.Config{
        FFTSize:      1024,
        WindowSize:   2048,
        MaxInstruments: 32,
    })
    go analyzer.Run()
}
```

**`eq_panel_zone.go`** — replace `waveformMode bool` with `panelTab PanelTab` and `panelExpanded bool`. Replace the single toggle button with a tab bar. Route to the appropriate renderer based on active tab + expanded state.

### WASM Considerations

The analyzer service runs entirely in Go — no browser API dependencies. On WASM:
- The audio thread (WebAudio) doesn't run in Go, so `PushInstBuf`/`PushMasterBuf` aren't called from Go's mixer
- Instead, the WASM bridge in `engine_wasm.go` can call `NotifyTrigger` when `playSoundsBatch` fires, providing the pre-rendered buffer
- For live monitoring on WASM, the analyzer can pull from the voice cache's rendered buffers on trigger events rather than mixer taps
- FFT and all observers work identically (pure Go fallback for C FFT)
- The meter bridge shows per-instrument peak/RMS from the last-rendered buffers

This gives WASM waveform inspection and per-trigger metrics. Full pipeline-tap monitoring (Phase 1/2/3) is desktop-only since the Go mixer doesn't run on WASM. This is acceptable — desktop is the primary dev-tool platform, and WASM gets the user-facing inspection features.

---

## File Plan

### New Files

| File | Layer | Purpose |
|------|-------|---------|
| `src/go/internal/wave/wave.go` | 1 | Wave struct, Source/Transform/Observer interfaces, Chain compositor |
| `src/go/internal/wave/transforms.go` | 2 | Normalize, Trim, Resample, Window transforms |
| `src/go/internal/wave/observers.go` | 2 | PeakRMS, Envelope, ZeroCrossing observers (pure Go) |
| `src/go/internal/wave/fft_c.go` | 2 | CGo bridge to C FFT (build tag `!test && !js`) |
| `src/go/internal/wave/fft_go.go` | 2 | Pure Go FFT fallback (build tag `test \|\| js`) |
| `src/go/internal/wave/sources.go` | 2 | RenderSource, CaptureSource |
| `src/go/internal/wave/effect_transform.go` | 2 | EffectTransform wrapping C insert effects |
| `src/c/fft.c` | — | C Cooley-Tukey radix-2 FFT |
| `src/c/fft.h` | — | C FFT header |
| `src/go/internal/analyzer/service.go` | 3 | Analyzer goroutine, ring buffers, state publishing |
| `src/go/internal/analyzer/ring.go` | 3 | SPSC ring buffer implementation |
| `src/go/internal/analyzer/state.go` | 3 | State, ChannelMetrics, InstrumentMetrics, CaptureBuffer types |
| `src/go/internal/analyzer/tap.go` | 3 | PipelineTap source adapter |
| `src/go/internal/ui/panel_tab.go` | 4 | Tab bar, expand/collapse, tab routing |
| `src/go/internal/ui/render_waveform.go` | 4 | Enhanced waveform renderer |
| `src/go/internal/ui/render_spectrum.go` | 4 | FFT spectrum bar renderer |
| `src/go/internal/ui/render_meters.go` | 4 | Meter bridge renderer |

### Modified Files

| File | Change |
|------|--------|
| `src/go/internal/audio/engine_stop.go` | Add analyzer push calls (~6 lines) |
| `src/go/internal/audio/engine_mixer.go` | Add trigger notification (~4 lines) |
| `src/go/internal/audio/engine.go` | Init analyzer service (~5 lines) |
| `src/go/internal/ui/eq_panel_zone.go` | Replace waveformMode with tab system, add expand/collapse |
| `src/go/internal/ui/drumview_draw_waveform.go` | Deprecate in favor of `render_waveform.go` |
| `src/go/internal/ui/drumview_ctor.go` | Update DrawWaveform callback to route through new renderer |
| `Makefile` | Add `fft.c` to C build, add `wave` and `analyzer` packages to test targets |

### Test Files

| File | Tests |
|------|-------|
| `src/go/internal/wave/wave_test.go` | Wave helpers, Chain compositor, transform chaining |
| `src/go/internal/wave/observers_test.go` | PeakRMS accuracy, envelope follower, zero-crossing |
| `src/go/internal/wave/fft_test.go` | FFT correctness (known frequencies), magnitude accuracy |
| `src/go/internal/wave/transforms_test.go` | Normalize, Trim, Resample, Window correctness |
| `src/go/internal/analyzer/service_test.go` | Ring buffer SPSC, state publishing, capture/freeze |
| `src/go/internal/analyzer/ring_test.go` | Ring buffer edge cases, overflow, underflow |

---

## Non-Goals (Explicitly Out of Scope)

- **Pipeline stage comparison** (A/B between tap points) — future work, architecture supports it via TapPoint enum
- **Audio recording/export** from the analyzer — existing capture infrastructure handles this
- **User-facing effect preview UI** (drag-and-drop effect chains) — future, but EffectTransform makes it possible
- **Spectrogram (time-frequency)** — future enhancement to SpectrumRenderer
- **WASM mixer-tap monitoring** — WASM gets trigger-based capture, not live mixer taps
