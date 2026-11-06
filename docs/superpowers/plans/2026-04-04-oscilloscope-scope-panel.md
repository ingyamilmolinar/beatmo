# Oscilloscope Scope Panel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a collapsible A/B oscilloscope panel below the drum pane that lets users tap any two points in the synthesis/effects pipeline and compare waveforms in real time.

**Architecture:** A new `ScopePanelZone` (implementing `Zone` interface) is registered in the DrumView tree alongside EQ/Transport/etc. A `ScopeService` goroutine in a new `scope` package captures triggered waveforms from tap points wired into the 3-phase mixer. The service publishes `ScopeState` atomically for lock-free UI reads. The scope renderer draws overlaid A/B traces with a pipeline strip header.

**Tech Stack:** Go 1.23, Ebiten v2, lock-free atomics (same pattern as analyzer package), existing `drawRect`/`DrawTextColorAtScale` UI primitives.

**Spec:** `docs/superpowers/specs/2026-04-04-oscilloscope-scope-panel-design.md`

---

## File Structure

### New files

| File | Purpose |
|------|---------|
| `src/go/internal/scope/state.go` | `ScopeState`, `ScopeTapData`, `ScopeStage` types |
| `src/go/internal/scope/service.go` | `Service` goroutine: ring buffers, triggered capture, atomic publish |
| `src/go/internal/scope/service_test.go` | Service tests: capture, freeze, stage switching |
| `src/go/internal/ui/scope_panel_zone.go` | `ScopePanelZone` struct, Zone interface, header bar, pipeline strip |
| `src/go/internal/ui/scope_panel_zone_test.go` | Zone tests: layout, hit areas, tap selection |
| `src/go/internal/ui/render_scope.go` | `drawScopeTraces()` — overlaid A/B waveform renderer |
| `src/go/internal/ui/render_scope_test.go` | Renderer tests: empty state, single trace, overlay |

### Modified files

| File | Changes |
|------|---------|
| `src/go/internal/audio/engine.go` | Init scope service alongside analyzer |
| `src/go/internal/audio/engine_stop.go` | Push samples to scope tap points at each mixer phase |
| `src/go/internal/audio/engine_mixer.go` | Capture raw voice buffer for "Synth" tap |
| `src/go/internal/ui/drumview.go` | Add `scopePanelZone` field, accessor |
| `src/go/internal/ui/drumview_ctor.go` | Create and register ScopePanelZone |
| `src/go/internal/ui/game_draw.go` | Draw scope panel below drum pane |
| `src/go/internal/ui/game_struct.go` | Add scope panel visibility state |

---

## Task 1: Scope State Types

**Files:**
- Create: `src/go/internal/scope/state.go`

- [ ] **Step 1: Create the scope package with state types**

```go
// Package scope provides real-time oscilloscope data types and services
// for A/B pipeline stage comparison.
package scope

// Stage identifies a point in the audio pipeline that can be tapped.
type Stage int

const (
	StageSynth    Stage = iota // Raw C synth output
	StageAntiPop               // After fade-in/out envelope
	StageInsertFX              // After insert effects chain
	StageEQ                    // After per-instrument EQ
	StageSends                 // After send effects mixed in
	StageMaster                // After master EQ + compressor
	stageCount
)

// StageLabel returns a short display label for the stage.
func StageLabel(s Stage) string {
	switch s {
	case StageSynth:
		return "Synth"
	case StageAntiPop:
		return "AntiPop"
	case StageInsertFX:
		return "InsertFX"
	case StageEQ:
		return "EQ"
	case StageSends:
		return "Sends"
	case StageMaster:
		return "Master"
	default:
		return "?"
	}
}

// AllStages returns all pipeline stages in signal-flow order.
func AllStages() []Stage {
	return []Stage{StageSynth, StageAntiPop, StageInsertFX, StageEQ, StageSends, StageMaster}
}

// State is the atomically-published snapshot the UI reads.
type State struct {
	TapA      TapData
	TapB      TapData
	Timestamp int64 // monotonic nanoseconds
}

// TapData holds the captured waveform and metrics for one tap point.
type TapData struct {
	Stage   Stage
	InstID  string
	Samples []float64
	PeakDB  float64
	RMSDB   float64
	Active  bool
}

// Config configures the scope service.
type Config struct {
	MaxWindowMs int // max capture window in ms (default 500)
	SampleRate  int // audio sample rate
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd src/go && ../../.tools/go/bin/go build ./internal/scope/...`
Expected: exit 0, no output

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/scope/state.go
git commit -m "feat(scope): add state types for oscilloscope A/B pipeline comparison"
```

---

## Task 2: Scope Service

**Files:**
- Create: `src/go/internal/scope/service.go`
- Create: `src/go/internal/scope/service_test.go`

- [ ] **Step 1: Write failing test for basic capture**

```go
//go:build test

package scope

import (
	"testing"
	"time"
)

func TestServicePublishesState(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 20, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	svc.SetTapB(StageEQ)
	svc.SetInstrument("kick")
	go svc.Run()
	defer svc.Stop()

	// Push some samples to the synth tap.
	samples := make([]float64, 882) // 20ms at 44.1kHz
	for i := range samples {
		samples[i] = float64(i) / float64(len(samples))
	}
	svc.PushSamples(StageSynth, "kick", samples)
	svc.PushSamples(StageEQ, "kick", samples)

	// Wait for tick.
	time.Sleep(50 * time.Millisecond)

	state := svc.State()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if !state.TapA.Active {
		t.Error("expected TapA active")
	}
	if state.TapA.Stage != StageSynth {
		t.Errorf("expected TapA stage Synth, got %v", state.TapA.Stage)
	}
	if len(state.TapA.Samples) == 0 {
		t.Error("expected TapA samples")
	}
}

func TestServiceFreeze(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 20, SampleRate: 44100})
	svc.SetTapA(StageSynth)
	svc.SetInstrument("kick")
	go svc.Run()
	defer svc.Stop()

	samples1 := make([]float64, 882)
	for i := range samples1 {
		samples1[i] = 0.5
	}
	svc.PushSamples(StageSynth, "kick", samples1)
	time.Sleep(50 * time.Millisecond)

	svc.Freeze()

	// Push different samples.
	samples2 := make([]float64, 882)
	for i := range samples2 {
		samples2[i] = 1.0
	}
	svc.PushSamples(StageSynth, "kick", samples2)
	time.Sleep(50 * time.Millisecond)

	state := svc.State()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	// Should still have the old samples (frozen).
	if state.TapA.Active && len(state.TapA.Samples) > 0 {
		if state.TapA.Samples[0] == 1.0 {
			t.Error("expected frozen state to retain old samples")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/scope/... -count=1 -v`
Expected: FAIL — `NewService` undefined

- [ ] **Step 3: Implement the scope service**

```go
package scope

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Service captures triggered waveforms from pipeline tap points and
// publishes ScopeState atomically for lock-free UI reads.
type Service struct {
	cfg Config

	mu     sync.Mutex
	tapA   atomic.Int32 // Stage for tap A (-1 = none)
	tapB   atomic.Int32 // Stage for tap B (-1 = none)
	instID atomic.Pointer[string]
	frozen atomic.Bool

	// Per-stage ring buffers (written by audio thread, read by service).
	rings [stageCount]ringBuf

	state  atomic.Pointer[State]
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ringBuf is a simple append-only buffer that the service drains each tick.
type ringBuf struct {
	mu  sync.Mutex
	buf []float64
	id  string // instrument ID of last push
}

func (r *ringBuf) push(id string, samples []float64) {
	r.mu.Lock()
	r.buf = append(r.buf, samples...)
	r.id = id
	r.mu.Unlock()
}

func (r *ringBuf) drain(maxSamples int) ([]float64, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		return nil, r.id
	}
	out := r.buf
	if len(out) > maxSamples {
		out = out[len(out)-maxSamples:]
	}
	r.buf = r.buf[:0]
	return out, r.id
}

// NewService creates a scope service with the given config.
func NewService(cfg Config) *Service {
	if cfg.MaxWindowMs <= 0 {
		cfg.MaxWindowMs = 500
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 44100
	}
	s := &Service{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	s.tapA.Store(-1)
	s.tapB.Store(-1)
	return s
}

// SetTapA sets the pipeline stage for tap A.
func (s *Service) SetTapA(stage Stage) { s.tapA.Store(int32(stage)) }

// SetTapB sets the pipeline stage for tap B.
func (s *Service) SetTapB(stage Stage) { s.tapB.Store(int32(stage)) }

// TapA returns the current tap A stage, or -1 if none.
func (s *Service) TapA() Stage { return Stage(s.tapA.Load()) }

// TapB returns the current tap B stage, or -1 if none.
func (s *Service) TapB() Stage { return Stage(s.tapB.Load()) }

// ClearTapA unsets tap A.
func (s *Service) ClearTapA() { s.tapA.Store(-1) }

// ClearTapB unsets tap B.
func (s *Service) ClearTapB() { s.tapB.Store(-1) }

// SetInstrument sets the instrument ID to scope.
func (s *Service) SetInstrument(id string) { s.instID.Store(&id) }

// Instrument returns the current instrument ID.
func (s *Service) Instrument() string {
	p := s.instID.Load()
	if p == nil {
		return ""
	}
	return *p
}

// Freeze prevents new captures from replacing the current state.
func (s *Service) Freeze() { s.frozen.Store(true) }

// Unfreeze allows new captures.
func (s *Service) Unfreeze() { s.frozen.Store(false) }

// IsFrozen returns the freeze state.
func (s *Service) IsFrozen() bool { return s.frozen.Load() }

// PushSamples pushes audio samples for a given pipeline stage and instrument.
// Called from the audio thread — must be fast (no allocations in hot path).
func (s *Service) PushSamples(stage Stage, instID string, samples []float64) {
	if stage < 0 || int(stage) >= len(s.rings) {
		return
	}
	s.rings[stage].push(instID, samples)
}

// State returns the latest published state (lock-free atomic load).
func (s *Service) State() *State { return s.state.Load() }

// Run blocks until Stop is called.
func (s *Service) Run() {
	s.wg.Add(1)
	defer s.wg.Done()

	interval := time.Duration(float64(time.Second) * float64(s.cfg.MaxWindowMs) / 1000.0)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// Stop signals the service to stop.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Service) tick() {
	if s.frozen.Load() {
		// Drain buffers but don't update state.
		maxSamples := s.cfg.MaxWindowMs * s.cfg.SampleRate / 1000
		for i := range s.rings {
			s.rings[i].drain(maxSamples)
		}
		return
	}

	instID := s.Instrument()
	maxSamples := s.cfg.MaxWindowMs * s.cfg.SampleRate / 1000

	st := &State{
		Timestamp: time.Now().UnixNano(),
	}

	tapAStage := Stage(s.tapA.Load())
	tapBStage := Stage(s.tapB.Load())

	if tapAStage >= 0 && int(tapAStage) < len(s.rings) {
		samples, id := s.rings[tapAStage].drain(maxSamples)
		if len(samples) > 0 && (instID == "" || id == instID) {
			st.TapA = buildTapData(tapAStage, id, samples)
		}
	}

	if tapBStage >= 0 && int(tapBStage) < len(s.rings) {
		samples, id := s.rings[tapBStage].drain(maxSamples)
		if len(samples) > 0 && (instID == "" || id == instID) {
			st.TapB = buildTapData(tapBStage, id, samples)
		}
	}

	// Drain unused rings.
	for i := range s.rings {
		stage := Stage(i)
		if stage != tapAStage && stage != tapBStage {
			s.rings[i].drain(maxSamples)
		}
	}

	// Only publish if we have data.
	if st.TapA.Active || st.TapB.Active {
		s.state.Store(st)
	}
}

func buildTapData(stage Stage, instID string, samples []float64) TapData {
	peak := 0.0
	sumSq := 0.0
	for _, v := range samples {
		abs := math.Abs(v)
		if abs > peak {
			peak = abs
		}
		sumSq += v * v
	}
	rms := math.Sqrt(sumSq / float64(len(samples)))

	peakDB := -math.MaxFloat64
	rmsDB := -math.MaxFloat64
	if peak > 0 {
		peakDB = 20 * math.Log10(peak)
	}
	if rms > 0 {
		rmsDB = 20 * math.Log10(rms)
	}

	return TapData{
		Stage:   stage,
		InstID:  instID,
		Samples: samples,
		PeakDB:  peakDB,
		RMSDB:   rmsDB,
		Active:  true,
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/scope/... -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/scope/
git commit -m "feat(scope): add scope service with ring buffers and atomic state publishing"
```

---

## Task 3: Wire Scope Taps into Mixer

**Files:**
- Modify: `src/go/internal/audio/engine.go`
- Modify: `src/go/internal/audio/engine_stop.go`
- Modify: `src/go/internal/audio/engine_mixer.go`

- [ ] **Step 1: Add scope service global and init**

In `engine.go`, add alongside the existing `analyzerSvc`:

```go
var scopeSvc *scope.Service

// ScopeService returns the scope service, or nil if not initialized.
func ScopeService() *scope.Service { return scopeSvc }
```

In the `initAudioEngine` or wherever `analyzerSvc` is initialized (around line 109), add:

```go
scopeSvc = scope.NewService(scope.Config{
    MaxWindowMs: 500,
    SampleRate:  sampleRate,
})
go scopeSvc.Run()
```

Add import for `"github.com/ingyamilmolinar/beatmo/internal/scope"`.

- [ ] **Step 2: Add tap points in mixer phases**

In `engine_stop.go`, add taps at each mixer phase. After the existing `analyzerSvc.PushInstBuf` calls:

**Phase 1 (after line ~451, per-instrument post-headroom):**
```go
if scopeSvc != nil {
    for _, slot := range m.activeSlots {
        id := m.instSlotIDs[slot]
        scopeSvc.PushSamples(scope.StageAntiPop, id, m.instBufs[slot][:blockLen])
    }
}
```

**Phase 2 (after channel processing, ~line 468):**
Need to tap after insert FX and after EQ separately. This requires capturing intermediate buffers in `channels.go:ProcessBlockLocal`. For now, tap the post-channel output (which is post-EQ) and the pre-EQ signal:

Add after Phase 2 loop:
```go
if scopeSvc != nil {
    // Post-EQ is what's accumulated in masterBuf at this point.
    // We can't easily separate insert FX from EQ here without refactoring
    // ProcessBlockLocal. For now, push the per-instrument post-channel signal.
    for _, slot := range m.activeSlots {
        id := m.instSlotIDs[slot]
        scopeSvc.PushSamples(scope.StageEQ, id, m.instBufs[slot][:blockLen])
    }
}
```

**Phase 2.5 (after sends, ~line 473):**
```go
if scopeSvc != nil {
    scopeSvc.PushSamples(scope.StageSends, "master", m.workBuf[:blockLen])
}
```

**Phase 3 (after master, ~line 488):**
```go
if scopeSvc != nil {
    scopeSvc.PushSamples(scope.StageMaster, "master", m.workBuf[:blockLen])
}
```

- [ ] **Step 3: Add raw synth tap in voice scheduling**

In `engine_mixer.go`, in `Schedule()` around line 77-80 (where `analyzerSvc.NotifyTrigger` is called), add:

```go
if scopeSvc != nil {
    if cv, ok := v.(*cVoice); ok && cv.buf != nil {
        // Convert float32 voice buffer to float64 for scope.
        f64 := make([]float64, len(cv.buf))
        for i, s := range cv.buf {
            f64[i] = float64(s)
        }
        scopeSvc.PushSamples(scope.StageSynth, id, f64)
    }
}
```

- [ ] **Step 4: Add stub for WASM/test builds**

Create `src/go/internal/audio/scope_stub.go`:
```go
//go:build test || js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/scope"

func ScopeService() *scope.Service { return nil }
```

And `src/go/internal/audio/scope_desktop.go`:
```go
//go:build !test && !js

package audio

// scopeSvc is defined in engine.go for desktop builds.
```

Actually, this may be simpler to handle the same way as `analyzerSvc` — check if it already uses build tags or nil guards. Use the same pattern: nil checks everywhere, and only initialize on desktop.

- [ ] **Step 5: Verify it compiles and tests pass**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/audio/... && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... -count=1 -timeout 120s`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/audio/ src/go/internal/scope/
git commit -m "feat(scope): wire scope tap points into 3-phase mixer"
```

---

## Task 4: Scope Panel Zone — Shell

**Files:**
- Create: `src/go/internal/ui/scope_panel_zone.go`
- Create: `src/go/internal/ui/scope_panel_zone_test.go`

- [ ] **Step 1: Write failing test for zone creation and layout**

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestScopePanelZoneLayout(t *testing.T) {
	assertDefaultParityState(t)

	z := NewScopePanelZone(ScopeCallbacks{})
	if z.ID() != "scope-panel" {
		t.Errorf("expected ID 'scope-panel', got %q", z.ID())
	}

	r := image.Rect(0, 0, 800, 160)
	z.Layout(r)

	// Should have pipeline stage buttons.
	for i, btn := range z.stageButtons {
		if btn == nil {
			t.Errorf("stage button %d is nil", i)
			continue
		}
		if btn.Rect().Empty() {
			t.Errorf("stage button %d has empty rect", i)
		}
	}

	// Should have hit areas.
	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Error("expected non-empty hit areas")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestScopePanelZone -count=1 -v`
Expected: FAIL — `NewScopePanelZone` undefined

- [ ] **Step 3: Implement ScopePanelZone**

Create the zone with callbacks, pipeline strip buttons, instrument selector, freeze/close buttons, and layout logic. Follow the same pattern as `EQPanelZone`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// ScopeCallbacks contains callbacks for the ScopePanelZone.
type ScopeCallbacks struct {
	ScopeState     func() *scope.State
	ActiveRows     func() []*DrumRow
	OnTapAChange   func(stage scope.Stage)
	OnTapBChange   func(stage scope.Stage)
	OnClearTapA    func()
	OnClearTapB    func()
	OnInstrChange  func(id string)
	OnFreezeToggle func() bool
	OnClose        func()
}

const (
	scopeHeaderH  = 26 // 4px + 18px button + 4px
	scopePanelH   = 160
)

// ScopePanelZone implements the Zone interface for the oscilloscope panel.
type ScopePanelZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  ScopeCallbacks
	portal     *OverlayPortal

	// Pipeline strip buttons (one per stage).
	stageButtons [6]*Button // StageSynth..StageMaster
	instBtn      *Button    // Instrument selector
	freezeBtn    *Button    // Freeze toggle
	closeBtn     *Button    // Close panel

	// Tap selection state.
	tapA scope.Stage // -1 = none
	tapB scope.Stage // -1 = none

	// Zoom state.
	windowMs float64 // current display window in ms (1-500)

	// Hit areas cache.
	hitAreas []HitArea
}

func NewScopePanelZone(cb ScopeCallbacks) *ScopePanelZone {
	z := &ScopePanelZone{
		needLayout: true,
		callbacks:  cb,
		tapA:       -1,
		tapB:       -1,
		windowMs:   20,
	}
	z.initButtons()
	return z
}

func (z *ScopePanelZone) initButtons() {
	stages := scope.AllStages()
	for i, stage := range stages {
		st := stage
		z.stageButtons[i] = NewButton(scope.StageLabel(st), InstButtonStyle, func() {
			z.handleStageClick(st)
		})
	}

	z.instBtn = NewButton("Kick", InstButtonStyle, func() {
		// TODO: open instrument dropdown
	})

	z.freezeBtn = NewButton("||", InstButtonStyle, func() {
		if z.callbacks.OnFreezeToggle != nil {
			z.callbacks.OnFreezeToggle()
		}
	})

	z.closeBtn = NewButton("\u2715", InstButtonStyle, func() {
		if z.callbacks.OnClose != nil {
			z.callbacks.OnClose()
		}
	})
}

func (z *ScopePanelZone) handleStageClick(stage scope.Stage) {
	switch {
	case z.tapA == stage:
		z.tapA = -1
		if z.callbacks.OnClearTapA != nil {
			z.callbacks.OnClearTapA()
		}
	case z.tapB == stage:
		z.tapB = -1
		if z.callbacks.OnClearTapB != nil {
			z.callbacks.OnClearTapB()
		}
	case z.tapA < 0:
		z.tapA = stage
		if z.callbacks.OnTapAChange != nil {
			z.callbacks.OnTapAChange(stage)
		}
	case z.tapB < 0:
		z.tapB = stage
		if z.callbacks.OnTapBChange != nil {
			z.callbacks.OnTapBChange(stage)
		}
	default:
		// Both assigned — replace B.
		z.tapB = stage
		if z.callbacks.OnTapBChange != nil {
			z.callbacks.OnTapBChange(stage)
		}
	}
}

// --- Zone interface ---

func (z *ScopePanelZone) ID() string { return "scope-panel" }

func (z *ScopePanelZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	z.layoutButtons()
	z.rebuildHitAreas()
}

func (z *ScopePanelZone) Update() {}

func (z *ScopePanelZone) HitAreas() []HitArea { return z.hitAreas }

func (z *ScopePanelZone) Draw(screen *ebiten.Image) {
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}
	drawRect(screen, z.rect, colEQBg, true)

	// Draw scope traces in content area.
	cr := z.contentRect()
	if cb := z.callbacks.ScopeState; cb != nil {
		if state := cb(); state != nil {
			drawScopeTraces(screen, cr, state, z.windowMs)
		}
	}

	// Draw header buttons.
	z.drawHeader(screen)

	// Border.
	drawRect(screen, z.rect, colButtonBorder, false)
}

func (z *ScopePanelZone) NeedsLayout() bool { return z.needLayout }
func (z *ScopePanelZone) Invalidate()       { z.needLayout = true }

func (z *ScopePanelZone) HandleKey(key ebiten.Key) InputResult { return InputIgnored }
func (z *ScopePanelZone) HandleChars(chars []rune) InputResult  { return InputIgnored }

// SetPortal sets the portal reference.
func (z *ScopePanelZone) SetPortal(p *OverlayPortal) { z.portal = p }

// contentRect returns the drawable area below the header.
func (z *ScopePanelZone) contentRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.rect.Min.Y+scopeHeaderH, z.rect.Max.X, z.rect.Max.Y)
}

func (z *ScopePanelZone) layoutButtons() {
	r := z.rect
	btnH := 18

	// Instrument button (left).
	instW := TextWidth("Kick") + 16
	if instW < 50 {
		instW = 50
	}
	z.instBtn.SetRect(image.Rect(r.Min.X+6, r.Min.Y+4, r.Min.X+6+instW, r.Min.Y+4+btnH))

	// Pipeline strip buttons (center).
	stripX := z.instBtn.Rect().Max.X + 8
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		w := TextWidth(btn.Text) + 12
		if w < 28 {
			w = 28
		}
		btn.SetRect(image.Rect(stripX, r.Min.Y+4, stripX+w, r.Min.Y+4+btnH))
		stripX += w + 3 // 3px gap including arrow space
		_ = i
	}

	// Right-side controls.
	closeW := 24
	freezeW := 24
	zoomW := 48
	rightEdge := r.Max.X - 6

	z.closeBtn.SetRect(image.Rect(rightEdge-closeW, r.Min.Y+4, rightEdge, r.Min.Y+4+btnH))
	rightEdge -= closeW + 4

	z.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, r.Min.Y+4, rightEdge, r.Min.Y+4+btnH))
	rightEdge -= freezeW + 4
	_ = zoomW // zoom display drawn as text, not a button
}

func (z *ScopePanelZone) drawHeader(screen *ebiten.Image) {
	// Instrument button.
	z.drawPillBtn(screen, z.instBtn, true)

	// Pipeline strip with arrows.
	stages := scope.AllStages()
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		stage := stages[i]
		active := z.tapA == stage || z.tapB == stage
		z.drawPillBtn(screen, btn, active)

		// Draw A/B badge on selected stages.
		if z.tapA == stage {
			z.drawBadge(screen, btn, "A", colScopeA)
		}
		if z.tapB == stage {
			z.drawBadge(screen, btn, "B", colScopeB)
		}

		// Arrow between stages.
		if i < len(z.stageButtons)-1 {
			nextBtn := z.stageButtons[i+1]
			if nextBtn != nil {
				arrowX := (btn.Rect().Max.X + nextBtn.Rect().Min.X) / 2
				arrowY := btn.Rect().Min.Y + btn.Rect().Dy()/2
				captionScale := FontSizeCaption / FontSizeBody
				DrawTextColorAtScale(screen, "\u2192", arrowX-3, arrowY-4, colTextSecondary, captionScale)
			}
		}
	}

	// Freeze and close.
	frozen := false
	if state := z.getScopeState(); state != nil {
		// Check frozen from service state if available.
	}
	z.drawPillBtn(screen, z.freezeBtn, frozen)
	z.drawPillBtn(screen, z.closeBtn, false)

	// Zoom display.
	captionScale := FontSizeCaption / FontSizeBody
	zoomText := formatWindowMs(z.windowMs)
	zoomX := z.freezeBtn.Rect().Min.X - TextWidth(zoomText) - 8
	zoomY := z.rect.Min.Y + 4 + (18-int(float64(TextHeight())*captionScale))/2
	DrawTextColorAtScale(screen, zoomText, zoomX, zoomY, colTextSecondary, captionScale)
}

func (z *ScopePanelZone) drawPillBtn(dst *ebiten.Image, btn *Button, active bool) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	if active {
		drawRect(dst, r, colSurface2, true)
		drawRect(dst, r, colAccent, false)
	} else {
		drawRect(dst, r, colButtonBorder, false)
	}
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(btn.Text)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	col := colTextSecondary
	if active {
		col = colTextAccent
	}
	DrawTextColorAtScale(dst, btn.Text, tx, ty, col, captionScale)
}

func (z *ScopePanelZone) drawBadge(dst *ebiten.Image, btn *Button, label string, col color.Color) {
	// Small colored circle at top-right of button.
	r := btn.Rect()
	bx := r.Max.X - 4
	by := r.Min.Y - 2
	badgeR := image.Rect(bx-8, by, bx, by+8)
	drawRect(dst, badgeR, col, true)
	captionScale := FontSizeCaption / FontSizeBody * 0.7
	DrawTextColorAtScale(dst, label, bx-7, by, colEQBg, captionScale)
}

func (z *ScopePanelZone) getScopeState() *scope.State {
	if z.callbacks.ScopeState != nil {
		return z.callbacks.ScopeState()
	}
	return nil
}

func formatWindowMs(ms float64) string {
	if ms >= 1 {
		return fmt.Sprintf("%.0fms", ms)
	}
	return fmt.Sprintf("%.1fms", ms)
}

func (z *ScopePanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]
	const zIdx = 140 // scope panel z-index

	// Stage buttons.
	for i, btn := range z.stageButtons {
		if btn == nil {
			continue
		}
		if r := btn.Rect(); !r.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    r,
				ZIndex:  zIdx + 1,
				Handler: &buttonHitAdapter{btn: btn},
				Tag:     fmt.Sprintf("scope-stage-%d", i),
			})
		}
	}

	// Instrument, freeze, close buttons.
	for _, pair := range []struct {
		btn *Button
		tag string
	}{
		{z.instBtn, "scope-inst"},
		{z.freezeBtn, "scope-freeze"},
		{z.closeBtn, "scope-close"},
	} {
		if pair.btn == nil {
			continue
		}
		if r := pair.btn.Rect(); !r.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    r,
				ZIndex:  zIdx + 1,
				Handler: &buttonHitAdapter{btn: pair.btn},
				Tag:     pair.tag,
			})
		}
	}

	// Scroll wheel on content area for zoom.
	cr := z.contentRect()
	if !cr.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    cr,
			ZIndex:  zIdx,
			Handler: &scopeZoomHandler{zone: z},
			Tag:     "scope-zoom",
		})
	}
}
```

Note: This references `colScopeA`, `colScopeB`, `color`, `fmt`, `drawScopeTraces`, and `scopeZoomHandler` which will be created in subsequent tasks. For compilation to pass, add placeholder color vars and a stub renderer. The full renderer is Task 5.

Add to the file:

```go
import "image/color"

var (
	colScopeA = color.RGBA{68, 136, 255, 255} // blue
	colScopeB = color.RGBA{255, 136, 68, 255} // orange
)

// scopeZoomHandler handles scroll wheel zoom on the scope display.
type scopeZoomHandler struct {
	zone *ScopePanelZone
}

func (h *scopeZoomHandler) OnPress(x, y int) InputResult { return InputIgnored }
func (h *scopeZoomHandler) OnDrag(x, y int)              {}
func (h *scopeZoomHandler) OnRelease(x, y int)           {}
func (h *scopeZoomHandler) OnWheel(x, y, steps int) InputResult {
	// Zoom: scroll up = zoom in (shorter window), scroll down = zoom out.
	factor := 1.0 + float64(steps)*0.15
	h.zone.windowMs *= factor
	if h.zone.windowMs < 1 {
		h.zone.windowMs = 1
	}
	if h.zone.windowMs > 500 {
		h.zone.windowMs = 500
	}
	return InputConsumed
}
```

- [ ] **Step 4: Run test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestScopePanelZone -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/scope_panel_zone.go src/go/internal/ui/scope_panel_zone_test.go
git commit -m "feat(scope): add ScopePanelZone shell with pipeline strip and tap selection"
```

---

## Task 5: Scope Trace Renderer

**Files:**
- Create: `src/go/internal/ui/render_scope.go`

- [ ] **Step 1: Implement the overlaid A/B trace renderer**

```go
package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// drawScopeTraces renders overlaid A/B waveform traces with grid, labels,
// trigger marker, and legend.
func drawScopeTraces(dst *ebiten.Image, rect image.Rectangle, state *scope.State, windowMs float64) {
	if state == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Reserve margins: 32px left (amplitude labels), 0px bottom (time in info bar area).
	marginL := 32
	waveRect := image.Rect(rect.Min.X+marginL, rect.Min.Y+2, rect.Max.X-2, rect.Max.Y-14)
	if waveRect.Dx() <= 0 || waveRect.Dy() <= 0 {
		return
	}

	// Background.
	drawRect(dst, waveRect, color.RGBA{10, 10, 22, 255}, true)

	midY := waveRect.Min.Y + waveRect.Dy()/2
	halfH := float64(waveRect.Dy()) * 0.48

	// Grid lines.
	gridCol := color.NRGBA{255, 255, 255, 15}
	// Horizontal: 0 line (solid), +/-0.5 (dashed).
	drawRect(dst, image.Rect(waveRect.Min.X, midY, waveRect.Max.X, midY+1), color.NRGBA{255, 255, 255, 25}, true)
	for _, frac := range []float64{0.25, 0.75} {
		y := waveRect.Min.Y + int(frac*float64(waveRect.Dy()))
		for x := waveRect.Min.X; x < waveRect.Max.X; x += 6 {
			endX := x + 3
			if endX > waveRect.Max.X {
				endX = waveRect.Max.X
			}
			drawRect(dst, image.Rect(x, y, endX, y+1), gridCol, true)
		}
	}
	// Vertical time divisions (5 divisions).
	for i := 1; i < 5; i++ {
		x := waveRect.Min.X + i*waveRect.Dx()/5
		for y := waveRect.Min.Y; y < waveRect.Max.Y; y += 6 {
			endY := y + 3
			if endY > waveRect.Max.Y {
				endY = waveRect.Max.Y
			}
			drawRect(dst, image.Rect(x, y, x+1, endY), gridCol, true)
		}
	}

	// Trigger marker at 10% from left.
	trigX := waveRect.Min.X + waveRect.Dx()/10
	trigCol := color.NRGBA{255, 200, 50, 60}
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), trigCol, true)

	// Draw traces.
	width := waveRect.Dx()
	if state.TapA.Active && len(state.TapA.Samples) > 0 {
		drawWaveTrace(dst, state.TapA.Samples, waveRect, midY, width, colScopeA)
	}
	if state.TapB.Active && len(state.TapB.Samples) > 0 {
		drawWaveTrace(dst, state.TapB.Samples, waveRect, midY, width, colScopeB)
	}

	// Amplitude labels.
	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)
	DrawTextColorAtScale(dst, "+1", rect.Min.X+2, waveRect.Min.Y+2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+2, midY-lh/2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "-1", rect.Min.X+2, waveRect.Max.Y-lh-2, colTextSecondary, captionScale)

	// Time axis labels.
	timeLabels := 6
	for i := 0; i < timeLabels; i++ {
		t := windowMs * float64(i) / float64(timeLabels-1)
		label := fmt.Sprintf("%.0fms", t)
		if t < 1 {
			label = fmt.Sprintf("%.1fms", t)
		}
		x := waveRect.Min.X + i*(waveRect.Dx()-1)/(timeLabels-1)
		lw := int(float64(TextWidth(label)) * captionScale)
		DrawTextColorAtScale(dst, label, x-lw/2, waveRect.Max.Y+1, colTextSecondary, captionScale)
	}

	// Legend with peak/RMS.
	legendY := waveRect.Max.Y + 1
	legendX := waveRect.Max.X - 10
	if state.TapB.Active {
		bLabel := fmt.Sprintf("B:%s pk:%.2f rms:%.1fdB", scope.StageLabel(state.TapB.Stage), state.TapB.PeakDB, state.TapB.RMSDB)
		bw := int(float64(TextWidth(bLabel)) * captionScale)
		DrawTextColorAtScale(dst, bLabel, legendX-bw, legendY, colScopeB, captionScale)
		legendX -= bw + 12
	}
	if state.TapA.Active {
		aLabel := fmt.Sprintf("A:%s pk:%.2f rms:%.1fdB", scope.StageLabel(state.TapA.Stage), state.TapA.PeakDB, state.TapA.RMSDB)
		aw := int(float64(TextWidth(aLabel)) * captionScale)
		DrawTextColorAtScale(dst, aLabel, legendX-aw, legendY, colScopeA, captionScale)
	}

	// Border.
	drawRect(dst, waveRect, color.NRGBA{26, 26, 46, 255}, false)
}
```

Note: This reuses `drawWaveTrace` from `render_waveform.go` (already exists).

- [ ] **Step 2: Verify compilation**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/...`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/render_scope.go
git commit -m "feat(scope): add overlaid A/B trace renderer with grid and legend"
```

---

## Task 6: Wire Scope Panel into DrumView

**Files:**
- Modify: `src/go/internal/ui/drumview.go`
- Modify: `src/go/internal/ui/drumview_ctor.go`
- Modify: `src/go/internal/ui/game_struct.go`
- Modify: `src/go/internal/ui/game_draw.go`

- [ ] **Step 1: Add scope zone to DrumView**

In `drumview.go`, add field:
```go
scopePanelZone *ScopePanelZone
```

And accessor:
```go
func (dv *DrumView) ScopePanelVisible() bool { return dv.scopePanelZone != nil }
```

- [ ] **Step 2: Create and register scope zone in drumview_ctor.go**

After the EQ panel zone registration (around line 281), add:

```go
// Phase 2.5: Scope panel zone — oscilloscope A/B comparison.
dv.scopePanelZone = NewScopePanelZone(ScopeCallbacks{
    ScopeState: func() *scope.State {
        svc := audio.ScopeService()
        if svc == nil {
            return nil
        }
        return svc.State()
    },
    ActiveRows: func() []*DrumRow {
        return dv.Rows
    },
    OnTapAChange: func(stage scope.Stage) {
        if svc := audio.ScopeService(); svc != nil {
            svc.SetTapA(stage)
        }
    },
    OnTapBChange: func(stage scope.Stage) {
        if svc := audio.ScopeService(); svc != nil {
            svc.SetTapB(stage)
        }
    },
    OnClearTapA: func() {
        if svc := audio.ScopeService(); svc != nil {
            svc.ClearTapA()
        }
    },
    OnClearTapB: func() {
        if svc := audio.ScopeService(); svc != nil {
            svc.ClearTapB()
        }
    },
    OnInstrChange: func(id string) {
        if svc := audio.ScopeService(); svc != nil {
            svc.SetInstrument(id)
        }
    },
    OnFreezeToggle: func() bool {
        svc := audio.ScopeService()
        if svc == nil {
            return false
        }
        if svc.IsFrozen() {
            svc.Unfreeze()
            return false
        }
        svc.Freeze()
        return true
    },
    OnClose: func() {
        dv.scopeVisible = false
    },
})
dv.scopePanelZone.SetPortal(dv.tree.Portal())
dv.tree.RegisterZone(dv.scopePanelZone, 135) // between EQ (130) and scope
```

Add `scopeVisible bool` field to DrumView struct, and import `scope` package.

- [ ] **Step 3: Integrate scope panel into the layout**

The scope panel needs to be laid out below the drum pane. This requires modifying the layout in `game_draw.go` to allocate space for it when visible. Read the current layout code and integrate accordingly — the scope panel gets a fixed rect below the drum pane, reducing the drum pane height by `scopePanelH` when visible.

- [ ] **Step 4: Verify compilation and tests**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/... && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... -count=1 -timeout 120s`
Expected: All pass

- [ ] **Step 5: Take a screenshot to verify the scope panel appears**

Run:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_scope ./cmd
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_scope -log ERROR -screenshot /tmp/scope_test.png
```

Then read the screenshot to verify the layout.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/drumview.go src/go/internal/ui/drumview_ctor.go src/go/internal/ui/game_struct.go src/go/internal/ui/game_draw.go
git commit -m "feat(scope): wire ScopePanelZone into DrumView with collapsible layout"
```

---

## Task 7: Visual Validation Infrastructure

**Files:**
- Modify: `src/go/internal/ui/demo.go`
- Modify: `src/go/cmd/beatmo.go`

- [ ] **Step 1: Add -scope flag to open with scope panel visible**

In `beatmo.go`, add a flag:
```go
scopeOpen := flag.Bool("scope", false, "open with scope panel visible")
```

After game creation, set scope visible:
```go
if *scopeOpen {
    g.SetScopeVisible(true)
}
```

Add `SetScopeVisible` method to `Game`:
```go
func (g *Game) SetScopeVisible(v bool) {
    if g.drum != nil {
        g.drum.scopeVisible = v
    }
}
```

- [ ] **Step 2: Create a scope screenshot script**

Create a simple shell one-liner that can be run to capture the scope state:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_scope ./cmd && \
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_scope -log ERROR -scope -screenshot /tmp/scope_screenshot.png
```

After each visual change, run this and read the resulting PNG to verify.

- [ ] **Step 3: Commit**

```bash
git add src/go/cmd/beatmo.go src/go/internal/ui/
git commit -m "feat(scope): add -scope flag for visual validation screenshots"
```

---

## Verification

After all tasks:

```bash
# Unit tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... -count=1 -timeout 120s

# Build verification
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_scope ./cmd

# Visual verification
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_scope -log ERROR -scope -screenshot /tmp/scope_final.png
# Then read /tmp/scope_final.png to verify layout
```
