# Oscilloscope Scope Panel — Design Spec

## Context

The existing Wave/Spectrum/Meters tabs provide basic audio monitoring but are too primitive for sound design debugging and live performance monitoring. Users need to see how each stage of the synthesis and effects pipeline transforms the signal — like an A/B oscilloscope that can tap any two points in the chain.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Use case | Sound design debugging + live monitoring | Primary workflows |
| Tap depth | 2 taps (A/B comparison) | Focused, low CPU overhead |
| Placement | Dedicated collapsible bottom panel | Full width, coexists with EQ panel |
| Tap selection | Pipeline strip with clickable stage blocks | Visual and intuitive |
| Trace display | Overlaid A/B with color coding | Best for seeing signal transformation |
| Capture mode | Triggered (per note hit) | Stable, repeatable waveforms |
| Time scale | Continuous zoom via scroll wheel | 1ms to 500ms range |

## Signal Pipeline Stages (Tap Points)

```
Synth → AntiPop → InsertFX → EQ → Sends → Master
```

Each stage corresponds to a real point in the 3-phase mixer:

| Stage | Where in code | What it captures |
|-------|--------------|-----------------|
| Synth | Phase 1: raw voice buffer (cVoice.buf) | Raw C synth output |
| AntiPop | Phase 1: after antiPopVoice envelope | Fade-in/out applied |
| InsertFX | Phase 2: after insert effect chain | Distortion, delay, etc. |
| EQ | Phase 2: after channel EQ processors | Frequency shaping |
| Sends | Phase 2.5: after send effects mixed in | Delay/reverb returns |
| Master | Phase 3: after master EQ + compressor | Final output |

## UI Components

### 1. Scope Panel (bottom panel)

- Collapsible panel below the drum pane, ~160px default height
- Toggled via a toolbar button (or keyboard shortcut)
- Contains: header bar + scope display + info bar

### 2. Header Bar

Left to right:
- **Instrument selector**: Dropdown to pick which instrument to scope
- **Pipeline strip**: Horizontal chain of clickable stage blocks (Synth → AntiPop → InsertFX → EQ → Sends → Master). Click to toggle A or B tap. Selected stages show colored A/B badges.
- **Freeze button**: Pauses capture on last triggered waveform
- **Zoom display**: Shows current time window (e.g., "20ms"). Scroll wheel adjusts.
- **Close button**: Collapses the scope panel

### 3. Scope Display

- Dark background with grid lines (amplitude + time divisions)
- Y-axis: amplitude labels (+1.0 to -1.0) on left margin
- X-axis: time labels (0ms to window end) on bottom
- Trigger marker: vertical line showing trigger point
- Two overlaid traces: A (blue #4488ff) and B (orange #ff8844)
- Legend with peak and RMS readouts per trace

### 4. Tap Point Interaction

- First click on an unselected stage → assigns it as A
- Second click on a different unselected stage → assigns it as B
- Click on an already-selected stage → unassigns it
- A/B badges (small colored circles) appear on selected stages

## Data Architecture

### ScopeTap

A ring buffer per tap point that captures audio samples at that pipeline stage. Reuses the existing `analyzer.RingBuffer` (SPSC lock-free) pattern.

### ScopeService

A goroutine similar to `analyzer.Service` that:
1. Receives triggered captures from the audio thread (lock-free)
2. Stores the last N triggered waveforms per tap point
3. Publishes a `ScopeState` atomically for the UI to read

### ScopeState

```go
type ScopeState struct {
    TapA      ScopeTapData  // data from tap point A
    TapB      ScopeTapData  // data from tap point B
    Timestamp int64
}

type ScopeTapData struct {
    Stage    string     // "synth", "antipop", etc.
    InstID   string     // instrument being scoped
    Samples  []float64  // captured waveform
    PeakDB   float64
    RMSDB    float64
    Active   bool
}
```

### Trigger Mechanism

When a voice is scheduled for the scoped instrument, the audio thread captures a fixed window of samples at each active tap point. The window size is determined by the current zoom level (default 20ms = 882 samples at 44.1kHz).

## Implementation Phases

### Phase 0: Visual Validation Infrastructure
Add a `-scope-tab` CLI flag that opens the app with the scope panel visible and a demo circuit playing, so screenshots can verify the UI.

### Phase 1: Scope Data Infrastructure
- `ScopeTap` type with ring buffer
- `ScopeService` goroutine
- `ScopeState` atomic publishing
- Wire tap points into the 3-phase mixer

### Phase 2: Scope Panel UI Shell
- `ScopePanelZone` implementing Zone interface
- Header bar with instrument selector and pipeline strip
- Collapsible panel with toggle button in toolbar
- Empty scope display area (no traces yet)

### Phase 3: Scope Renderer
- `drawScopeTraces()` — overlaid A/B waveform rendering
- Grid lines, amplitude labels, time axis
- Trigger marker
- Legend with peak/RMS readouts

### Phase 4: Tap Selection Interaction
- Pipeline strip click handling (A/B assignment)
- A/B badge rendering on selected stages
- Wire tap selection to ScopeService

### Phase 5: Zoom and Freeze
- Scroll wheel zoom on scope display (1ms–500ms)
- Freeze button toggling capture
- Zoom display in header bar

### Phase 6: Integration and Polish
- Instrument selector dropdown
- Keyboard shortcut for panel toggle
- Visual verification via screenshots
