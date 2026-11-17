# Wave Tab Adaptive Scale Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Wave tab's pause (freeze) button, replace it with an `AUTO` auto-gain pill, make the waveform auto-fit the signal's smoothed peak (with manual scroll-wheel zoom), and show the live Y-scale via dynamic edge labels + a gain badge.

**Architecture:** A pure gain helper (`autoGainForPeak`) + a smoothing accumulator on `EQPanelZone` resolve an effective Y-gain each data refresh. That gain is threaded into the existing `drawWaveTrace` `yGain` param and into relabeled axis text + a corner badge. The Wave tab's control component (`waveControls`) drops its freeze pill for an `AUTO` toggle; a `waveZoomHandler` mirrors the Chain tab's wheel-zoom. Mirrors the proven Chain/Scope auto-gain model (`chain_trace_cache.go:98-111`, `chain_panel_zone.go:1136-1166`).

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), package `internal/ui`.

**IMPORTANT — repo rules (override the skill's defaults):**
- **Do NOT `git add`/commit/stash** anything. Leave all work in the working tree. Each "Checkpoint" step below replaces the skill's normal "Commit" step — run the verification command, confirm green, and STOP there.
- **Never delete/`rm` or `git restore` files.** Revert your own edits manually with Edit.
- Test command (fast path, explicit package path — `./...` hits a phantom duplicate `ui` package on this branch):
  ```
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<RunFilter>'
  ```
- The full `internal/ui` suite has **pre-existing, partly non-deterministic failures** from other uncommitted WIP. Verify YOUR work with `-run` filters scoped to the tests you add. Confirm a failure is pre-existing by checking it fails on a stashless baseline only via reasoning — do not stash.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `src/go/internal/ui/render_waveform_scale.go` | Pure scale helpers: `autoGainForPeak`, `advanceWavePeak`, `waveEdgeAmplitude`, `waveGainBadgeText`, `waveWindowPeak`, constants. | Create |
| `src/go/internal/ui/render_waveform_scale_test.go` | Unit tests for the pure helpers. | Create |
| `src/go/internal/ui/render_waveform.go` | `drawAnalyzerWaveform` gains `gain`/`autoOn` params; dynamic edge labels + gain badge; pass gain into `drawWaveTrace`. | Modify |
| `src/go/internal/ui/audio_tab_controls.go` | Extract `layoutRightPill` helper; rewrite `waveControls` to own an `AUTO` pill (no freeze). | Modify |
| `src/go/internal/ui/eq_panel_zone.go` | Wave-scale state fields, `resolveWaveGain`, ctor wiring, `SyncAuto` draw call, `waveZoomHandler` + hit area. | Modify |
| `src/go/internal/ui/wave_trace_cache.go` | Compute gain via `resolveWaveGain`, thread into `drawAnalyzerWaveform`, fold quantized gain into cache key. | Modify |
| `src/go/internal/ui/wave_tab_scale_test.go` | Zone-level tests: AUTO default, wheel disables AUTO via real tree dispatch, SyncAuto visual, smoothing across refreshes. | Create |
| `src/go/internal/ui/audio_tab_controls_test.go` | Update Wave-freeze assertions → AUTO assertions. | Modify (if exists) |
| `src/go/internal/ui/audio_panel_render_pixels_test.go` | Add a quiet-signal-fills-panel pixel assertion. | Modify |

---

## Task 1: Pure scale helpers + constants

**Files:**
- Create: `src/go/internal/ui/render_waveform_scale.go`
- Test: `src/go/internal/ui/render_waveform_scale_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/render_waveform_scale_test.go`:

```go
package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func TestAutoGainForPeak(t *testing.T) {
	cases := []struct {
		name string
		peak float64
		want float64
	}{
		{"silence", 0.0, 1.0},
		{"near-silence", 0.0005, 1.0},
		{"loud-full-scale", 1.0, 1.0},     // 0.9/1.0 = 0.9 -> clamp up to 1.0
		{"half", 0.45, 2.0},               // 0.9/0.45 = 2.0
		{"quiet", 0.09, 10.0},             // 0.9/0.09 = 10.0
		{"tiny-clamped", 0.001, 16.0},     // 0.9/0.001 = 900 -> clamp to 16
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := autoGainForPeak(c.peak)
			if math.Abs(got-c.want) > 1e-6 {
				t.Fatalf("autoGainForPeak(%v) = %v, want %v", c.peak, got, c.want)
			}
		})
	}
}

func TestAdvanceWavePeak(t *testing.T) {
	// Attack: a louder peak snaps up instantly.
	if got := advanceWavePeak(0.1, 0.8); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("attack: got %v, want 0.8", got)
	}
	// Release: a quieter peak decays toward it, not instantly.
	got := advanceWavePeak(0.8, 0.0)
	if got >= 0.8 || got <= 0.0 {
		t.Fatalf("release: got %v, want between 0 and 0.8 (exclusive)", got)
	}
	if math.Abs(got-0.8*waveAutoGainRelease) > 1e-9 {
		t.Fatalf("release: got %v, want %v", got, 0.8*waveAutoGainRelease)
	}
}

func TestWaveEdgeAmplitude(t *testing.T) {
	if got := waveEdgeAmplitude(4.0); math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("waveEdgeAmplitude(4) = %v, want 0.25", got)
	}
	if got := waveEdgeAmplitude(1.0); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("waveEdgeAmplitude(1) = %v, want 1.0", got)
	}
}

func TestWaveGainBadgeText(t *testing.T) {
	if got := waveGainBadgeText(4.2, true); got != "AUTO x4.2" {
		t.Fatalf("auto badge = %q, want %q", got, "AUTO x4.2")
	}
	if got := waveGainBadgeText(2.0, false); got != "x2.0" {
		t.Fatalf("manual badge = %q, want %q", got, "x2.0")
	}
}

func TestWaveWindowPeak(t *testing.T) {
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.0, -0.3, 0.5, -0.2}}
	if got := waveWindowPeak(ch, nil); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("waveWindowPeak = %v, want 0.5", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestAutoGainForPeak|TestAdvanceWavePeak|TestWaveEdgeAmplitude|TestWaveGainBadgeText|TestWaveWindowPeak'`
Expected: FAIL — `undefined: autoGainForPeak` (and the other helpers).

- [ ] **Step 3: Write minimal implementation**

Create `src/go/internal/ui/render_waveform_scale.go`:

```go
package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Wave-tab adaptive Y-scale constants. Mirrors the Chain/Scope auto-gain model
// (chain_trace_cache.go:98-111): target gain fills ~90% of the panel at the
// signal's peak, clamped so a quiet signal scales up but never past 16x and a
// hot signal never shrinks below 1x.
const (
	waveAutoGainTargetFill = 0.9   // peak maps to 90% of half-height
	waveAutoGainMax        = 16.0  // clamp ceiling (matches chain)
	waveAutoGainMin        = 1.0   // never zoom out under auto (matches chain)
	waveAutoGainSilence    = 0.001 // below this peak, treat as silence -> 1x
	// waveAutoGainRelease is the per-data-refresh decay factor for the smoothed
	// peak-hold. The analyzer republishes ~30 Hz, so 0.90 gives a ~150 ms scale
	// decay (instant attack on a louder hit). Tunable; keep the rationale.
	waveAutoGainRelease = 0.90
)

// autoGainForPeak maps a (smoothed) peak amplitude to an effective Y-gain.
// Pure; unit-tested. Silence -> 1.0; otherwise clamp(0.9/peak, 1.0, 16.0).
func autoGainForPeak(peak float64) float64 {
	if peak < waveAutoGainSilence {
		return waveAutoGainMin
	}
	g := waveAutoGainTargetFill / peak
	if g < waveAutoGainMin {
		g = waveAutoGainMin
	}
	if g > waveAutoGainMax {
		g = waveAutoGainMax
	}
	return g
}

// advanceWavePeak applies smoothed peak-hold: instant attack (a louder peak
// wins immediately), slow release (a quieter peak decays the held value toward
// it by waveAutoGainRelease). Pure; unit-tested.
func advanceWavePeak(prev, peak float64) float64 {
	if peak >= prev {
		return peak
	}
	return prev * waveAutoGainRelease
}

// waveEdgeAmplitude returns the true sample amplitude represented by the top
// (and, negated, the bottom) edge of the wave panel at the given gain. At gain
// g the panel edge corresponds to amplitude 1/g.
func waveEdgeAmplitude(gain float64) float64 {
	if gain <= 0 {
		return 1.0
	}
	return 1.0 / gain
}

// waveGainBadgeText formats the corner Y-scale badge. "AUTO x4.2" when auto is
// engaged, "x2.0" when manually zoomed. The "x" is a plain letter (not the
// forbidden multiplication glyph).
func waveGainBadgeText(gain float64, autoOn bool) string {
	if autoOn {
		return fmt.Sprintf("AUTO x%.1f", gain)
	}
	return fmt.Sprintf("x%.1f", gain)
}

// waveWindowPeak returns the max |sample| over the waveform the Wave tab is
// currently showing — frozen capture wins, else the rolling channel waveform,
// scanning stereo L/R when present. Used to drive the adaptive gain.
func waveWindowPeak(ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer) float64 {
	var wave []float64
	switch {
	case capture != nil && capture.Frozen:
		wave = capture.Wave.Samples
	case ch != nil && ch.Active:
		wave = ch.Waveform
	}
	peak := maxAbs(wave)
	if ch != nil && ch.HasStereo() {
		if l := maxAbs(ch.WaveformL); l > peak {
			peak = l
		}
		if r := maxAbs(ch.WaveformR); r > peak {
			peak = r
		}
	}
	return peak
}

func maxAbs(xs []float64) float64 {
	m := 0.0
	for _, v := range xs {
		if v < 0 {
			v = -v
		}
		if v > m {
			m = v
		}
	}
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestAutoGainForPeak|TestAdvanceWavePeak|TestWaveEdgeAmplitude|TestWaveGainBadgeText|TestWaveWindowPeak'`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Checkpoint (no commit)**

Run the Step 4 command once more, confirm PASS. Leave changes in the working tree. Do **not** `git add`/commit.

---

## Task 2: Thread gain into the renderer + dynamic labels + badge

**Files:**
- Modify: `src/go/internal/ui/render_waveform.go:31-141` (`drawAnalyzerWaveform`)
- Test: `src/go/internal/ui/render_waveform_scale_test.go` (add a render-side test)

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/render_waveform_scale_test.go`:

```go
func TestDrawAnalyzerWaveformGainAcceptsParams(t *testing.T) {
	// Compile-time + smoke contract: the renderer accepts a gain + autoOn and
	// does not panic on a quiet mono signal. Pixel behavior is covered by
	// audio_panel_render_pixels_test.go.
	img := newTrackedImage("waveTest", 200, 80)
	defer releaseImage(img)
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: make([]float64, 256)}
	for i := range ch.Waveform {
		ch.Waveform[i] = 0.1 // quiet
	}
	drawAnalyzerWaveform(img, img.Bounds(), ch, nil, nil, 8.0, true)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDrawAnalyzerWaveformGainAcceptsParams'`
Expected: FAIL — `too many arguments in call to drawAnalyzerWaveform`.

- [ ] **Step 3: Change the renderer signature + labels + badge**

In `src/go/internal/ui/render_waveform.go`, change the signature (line ~31):

```go
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, beatGrid []float64, gain float64, autoOn bool) {
```

Add a guard right after the function opens (after the existing `if ch == nil` block is fine, but before computing labels):

```go
	if gain <= 0 {
		gain = 1.0
	}
```

Replace the hardcoded edge labels (lines ~65-67):

```go
	// Edge labels reflect the TRUE amplitude the panel edges represent at the
	// current Y-gain (adaptive auto-scale). At gain g the top/bottom edges are
	// +/- 1/g. Center is always 0.
	edge := waveEdgeAmplitude(gain)
	DrawTextColorAtScale(dst, fmt.Sprintf("+%.2g", edge), rect.Min.X+SpaceXS, waveRect.Min.Y+2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+SpaceXS, waveRect.Min.Y+waveRect.Dy()/2-lh/2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, fmt.Sprintf("-%.2g", edge), rect.Min.X+SpaceXS, waveRect.Max.Y-lh-2, colTextSecondary, captionScale)
```

Change the two `drawWaveTrace` calls to pass `gain` instead of `1.0` (lines ~115-116 stereo, ~124 mono):

```go
		drawWaveTrace(dst, ch.WaveformL, topRect, topMid, width, colWaveTrace, gain, colWaveTraceFill)
		drawWaveTrace(dst, ch.WaveformR, botRect, botMid, width, colWaveTrace, gain, colWaveTraceFill)
```
```go
		drawWaveTrace(dst, wave, waveRect, midY, width, colWaveTrace, gain, colWaveTraceFill)
```

Replace the frozen indicator block (lines ~136-140) so the gain badge shares the slot, with FROZEN winning when frozen:

```go
	// Top-right status slot: FROZEN takes priority (global freeze reachable
	// from other tabs); otherwise show the live Y-scale badge so the user
	// always knows the current vertical scale.
	if frozen {
		frozenW := int(float64(TextWidth("FROZEN")) * captionScale)
		DrawTextColorAtScale(dst, "FROZEN", waveRect.Max.X-frozenW-4, waveRect.Min.Y+2, colAccentBright, captionScale)
	} else {
		badge := waveGainBadgeText(gain, autoOn)
		bw := int(float64(TextWidth(badge)) * captionScale)
		DrawTextColorAtScale(dst, badge, waveRect.Max.X-bw-4, waveRect.Min.Y+2, colTextSecondary, captionScale)
	}
```

(`fmt` is already imported in this file.)

- [ ] **Step 4: Fix the two existing callers so the package compiles**

`drawAnalyzerWaveform` is called in `wave_trace_cache.go` (2 sites: lines ~45 and ~63). Temporarily pass `1.0, true` at BOTH so the package compiles now; Task 4 replaces them with the resolved gain:

```go
		drawAnalyzerWaveform(screen, cr, ch, cap, beatGrid, 1.0, true)
```
```go
		drawAnalyzerWaveform(z.waveCache, image.Rect(0, 0, w, h), ch, cap, beatGrid, 1.0, true)
```

Grep for any other caller and update it too:
Run: `cd src/go/internal/ui && grep -rn 'drawAnalyzerWaveform(' --include='*.go' | grep -v '_test.go'`
Expected: only the definition + the two `wave_trace_cache.go` sites. Update any test callers found by `grep -rn 'drawAnalyzerWaveform(' --include='*_test.go'` to pass `1.0, true`.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDrawAnalyzerWaveformGainAcceptsParams|TestAutoGainForPeak'`
Expected: PASS.

- [ ] **Step 6: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — confirm it builds clean (memory: trust `go vet`, not gopls stale-undefined). Leave in working tree.

---

## Task 3: Zone wave-scale state + `resolveWaveGain` + cache integration

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go` (struct fields ~249-256 area; add `resolveWaveGain`)
- Modify: `src/go/internal/ui/wave_trace_cache.go`
- Test: `src/go/internal/ui/wave_tab_scale_test.go` (smoothing test)

- [ ] **Step 1: Add zone fields**

In `eq_panel_zone.go`, next to the existing `waveCache`/`waveKey*` fields (~249-256), add:

```go
	// Wave-tab adaptive Y-scale. waveAutoGain (default true) auto-fits the
	// trace to the signal's smoothed peak; waveYGain is the manual scroll-wheel
	// zoom used when auto is off. waveSmoothedPeak is the peak-hold accumulator,
	// advanced once per analyzer-state refresh (deduped via waveSmoothState).
	waveAutoGain    bool
	waveYGain       float64
	waveSmoothedPeak float64
	waveSmoothState *analyzer.State
	waveKeyGain     float64 // quantized effective gain folded into the cache key
```

- [ ] **Step 2: Initialize defaults in the ctor**

Find the EQPanelZone constructor (the function that sets `z.waveControls = newWaveControls(...)` near line 344). Immediately before/after that line, add:

```go
	z.waveAutoGain = true
	z.waveYGain = 1.0
```

- [ ] **Step 3: Write the failing smoothing test**

Create `src/go/internal/ui/wave_tab_scale_test.go`:

```go
package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func TestResolveWaveGainSmoothsAcrossRefreshes(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: true, waveYGain: 1.0}

	loud := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.9, -0.9}}}
	g1, auto1 := z.resolveWaveGain(loud, &loud.Master, nil)
	if !auto1 {
		t.Fatalf("expected auto on")
	}
	if math.Abs(g1-1.0) > 1e-6 { // peak 0.9 -> 0.9/0.9 = 1.0
		t.Fatalf("loud gain = %v, want 1.0", g1)
	}

	// New (quieter) state pointer: attack-instant peak drop is a RELEASE, so the
	// smoothed peak decays gradually -> gain rises gradually, not straight to 9x.
	quiet := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.1, -0.1}}}
	g2, _ := z.resolveWaveGain(quiet, &quiet.Master, nil)
	if g2 <= g1 {
		t.Fatalf("quiet gain %v should exceed loud gain %v", g2, g1)
	}
	if g2 >= 9.0 {
		t.Fatalf("quiet gain %v should be smoothed below the 9x steady-state on first refresh", g2)
	}

	// Same state pointer again: no second smoothing advance (idempotent).
	g3, _ := z.resolveWaveGain(quiet, &quiet.Master, nil)
	if math.Abs(g3-g2) > 1e-9 {
		t.Fatalf("same-pointer refresh changed gain: %v -> %v", g2, g3)
	}
}

func TestResolveWaveGainManualIgnoresAuto(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: false, waveYGain: 3.5}
	st := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.05}}}
	g, auto := z.resolveWaveGain(st, &st.Master, nil)
	if auto {
		t.Fatalf("expected auto off")
	}
	if math.Abs(g-3.5) > 1e-9 {
		t.Fatalf("manual gain = %v, want 3.5", g)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestResolveWaveGain'`
Expected: FAIL — `z.resolveWaveGain undefined`.

- [ ] **Step 5: Implement `resolveWaveGain`**

Add to `eq_panel_zone.go` (near `resolveChannel`, ~line 2241):

```go
// resolveWaveGain returns the effective Y-gain for the Wave tab and whether
// auto-gain is engaged. Auto-gain advances a smoothed peak-hold ONCE per
// analyzer-state refresh (state-pointer change), so the scale eases instead of
// jittering per frame; manual mode returns the scroll-wheel gain verbatim.
func (z *EQPanelZone) resolveWaveGain(state *analyzer.State, ch *analyzer.ChannelMetrics, cap *analyzer.CaptureBuffer) (float64, bool) {
	if state != z.waveSmoothState {
		z.waveSmoothState = state
		z.waveSmoothedPeak = advanceWavePeak(z.waveSmoothedPeak, waveWindowPeak(ch, cap))
	}
	if z.waveAutoGain {
		return autoGainForPeak(z.waveSmoothedPeak), true
	}
	if z.waveYGain <= 0 {
		return 1.0, false
	}
	return z.waveYGain, false
}
```

- [ ] **Step 6: Thread gain through the cache**

In `wave_trace_cache.go`, replace the body of `drawWaveformCached`. Compute the gain up front, pass it to BOTH `drawAnalyzerWaveform` calls, and fold a quantized gain into the rebuild condition:

```go
func (z *EQPanelZone) drawWaveformCached(screen *ebiten.Image, cr image.Rectangle, state *analyzer.State, ch *analyzer.ChannelMetrics, cap *analyzer.CaptureBuffer, beatGrid []float64) {
	w, h := cr.Dx(), cr.Dy()
	if w < 1 || h < 1 {
		return
	}
	gain, autoOn := z.resolveWaveGain(state, ch, cap)
	if !waveTraceCacheEnabled {
		drawAnalyzerWaveform(screen, cr, ch, cap, beatGrid, gain, autoOn)
		return
	}

	frozen := cap != nil && cap.Frozen
	// Quantize gain to 0.01 so tiny smoothing deltas don't rebuild every frame.
	gq := math.Round(gain*100) / 100
	dimsChanged := z.waveCache == nil ||
		z.waveCache.Bounds().Dx() != w || z.waveCache.Bounds().Dy() != h
	if !z.waveKeyOK || dimsChanged || z.waveKeyW != w || z.waveKeyH != h ||
		z.waveKeyState != state || z.waveKeyCh != ch || z.waveKeyCap != cap ||
		z.waveKeyFrozen != frozen || z.waveKeyGain != gq {
		if dimsChanged {
			releaseImage(z.waveCache)
			z.waveCache = newTrackedImage("waveTrace", w, h)
		} else {
			z.waveCache.Clear()
		}
		drawAnalyzerWaveform(z.waveCache, image.Rect(0, 0, w, h), ch, cap, beatGrid, gain, autoOn)
		z.waveKeyW, z.waveKeyH = w, h
		z.waveKeyState, z.waveKeyCh, z.waveKeyCap = state, ch, cap
		z.waveKeyFrozen = frozen
		z.waveKeyGain = gq
		z.waveKeyOK = true
	}

	bumpDrawCall()
	chainBlitOp.GeoM.Reset()
	chainBlitOp.GeoM.Translate(float64(cr.Min.X), float64(cr.Min.Y))
	screen.DrawImage(z.waveCache, &chainBlitOp)
}
```

Add `"math"` to `wave_trace_cache.go`'s import block.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestResolveWaveGain'`
Expected: PASS (both tests).

- [ ] **Step 8: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean build. Leave in working tree.

---

## Task 4: Replace the Wave freeze pill with an `AUTO` pill

**Files:**
- Modify: `src/go/internal/ui/audio_tab_controls.go` (extract `layoutRightPill`; rewrite `waveControls` ~132-156)
- Modify: `src/go/internal/ui/eq_panel_zone.go` (`newWaveControls` call ~344; `SyncAuto` draw call ~637-643)
- Test: `src/go/internal/ui/audio_tab_controls_test.go` (update) + `wave_tab_scale_test.go` (add)

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/wave_tab_scale_test.go`:

```go
import "image" // ensure present at top with the other imports

func TestWaveControlsHasAutoNotFreeze(t *testing.T) {
	autoToggled := false
	c := newWaveControls(5, func() bool { autoToggled = !autoToggled; return autoToggled })
	c.Layout(image.Rect(0, 0, 300, 26))

	var tags []string
	for _, h := range c.HitAreas() {
		tags = append(tags, h.Tag)
	}
	for _, tag := range tags {
		if tag == "wave-freeze-btn" {
			t.Fatalf("Wave tab must not register a freeze hit area; tags=%v", tags)
		}
	}
	foundAuto := false
	for _, tag := range tags {
		if tag == "wave-auto-btn" {
			foundAuto = true
		}
	}
	if !foundAuto {
		t.Fatalf("expected a wave-auto-btn hit area; tags=%v", tags)
	}

	// Clicking the AUTO pill fires the toggle callback.
	for _, h := range c.HitAreas() {
		if h.Tag == "wave-auto-btn" {
			h.Handler.OnPress(h.Rect.Min.X+1, h.Rect.Min.Y+1)
		}
	}
	if !autoToggled {
		t.Fatalf("AUTO pill OnPress did not fire the toggle callback")
	}

	// SyncFreeze is a no-op (interface compliance) and must not panic.
	c.SyncFreeze(true)
	// SyncAuto updates the visual without panicking.
	c.SyncAuto(false)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControlsHasAutoNotFreeze'`
Expected: FAIL — `newWaveControls` still wires a freeze pill / `SyncAuto` undefined.

- [ ] **Step 3: Extract `layoutRightPill` helper**

In `audio_tab_controls.go`, add this free helper (place it just above `freezeControls`):

```go
// layoutRightPill places btn flush against header's right edge and returns the
// clamped button height, the pill top Y, and the running right-edge cursor for
// any further pills laid out leftward. Shared by the freeze-pill base and the
// Wave tab's AUTO pill so the header geometry stays identical across tabs.
func layoutRightPill(header image.Rectangle, btn *Button, w int) (btnH, y, rightEdge int) {
	d := Profile().DensityValues()
	btnH = header.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y = header.Min.Y + 4
	rightEdge = header.Max.X - SpaceSM
	btn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
	rightEdge -= w + d.AudioPillGap
	return btnH, y, rightEdge
}
```

Refactor `freezeControls.layoutFreezePill` to delegate (keeps the other tabs identical):

```go
func (c *freezeControls) layoutFreezePill(header image.Rectangle) (btnH, y, rightEdge int) {
	return layoutRightPill(header, c.freezeBtn, Profile().DensityValues().AudioPillIconW)
}
```

- [ ] **Step 4: Rewrite `waveControls`**

Replace the whole `waveControls` block (lines ~132-156) with:

```go
// waveControls owns the Wave tab's control header: a single AUTO pill that
// toggles adaptive Y-auto-gain. The Wave tab has no freeze pill (removed) —
// global freeze is still reachable from the other tabs and still honored by the
// Wave renderer. waveControls does not embed freezeControls; it implements the
// tabControls interface directly (SyncFreeze is a no-op here).
type waveControls struct {
	autoBtn  *Button
	autoOn   bool
	z        int
	hitAreas []HitArea
}

// newWaveControls builds the Wave header. onAuto toggles auto-gain and returns
// the new state (mirrors newFreezePill's onFreeze contract).
func newWaveControls(z int, onAuto func() bool) *waveControls {
	c := &waveControls{z: z, autoOn: true}
	c.autoBtn = NewButton("AUTO", InstButtonStyle, nil)
	c.autoBtn.TextColor = colTextSecondary
	c.autoBtn.OnClick = func() {
		if onAuto == nil {
			return
		}
		c.autoOn = onAuto()
	}
	return c
}

func (c *waveControls) HeaderH() int          { return controlHeaderHeight() }
func (c *waveControls) HitAreas() []HitArea    { return c.hitAreas }
func (c *waveControls) SyncFreeze(bool)        {} // Wave tab has no freeze pill.

// SyncAuto records the zone's auto-gain state so the pill's active-highlight
// follows wheel-zoom (which disables auto) as well as the pill click.
func (c *waveControls) SyncAuto(on bool) { c.autoOn = on }

func (c *waveControls) Layout(header image.Rectangle) {
	layoutRightPill(header, c.autoBtn, Profile().DensityValues().AudioPillWideW)
	c.hitAreas = c.hitAreas[:0]
	if r := c.autoBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, HitArea{
			Rect: r, ZIndex: c.z,
			Handler: &buttonHitAdapter{btn: c.autoBtn}, Tag: "wave-auto-btn",
		})
	}
}

func (c *waveControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.autoBtn, c.autoOn)
}
```

- [ ] **Step 5: Rewire the ctor + draw sync in `eq_panel_zone.go`**

Change the `newWaveControls` call (~line 344) to toggle the zone's auto state:

```go
	z.waveControls = newWaveControls(ctrlZ, func() bool {
		z.waveAutoGain = !z.waveAutoGain
		return z.waveAutoGain
	})
```

In the per-tab control draw block (~637-643), after `c.SyncFreeze(frozen)`, sync the AUTO pill when the Wave controls are active:

```go
		c.SyncFreeze(frozen)
		if wc, ok := c.(*waveControls); ok {
			wc.SyncAuto(z.waveAutoGain)
		}
		c.Draw(screen)
```

- [ ] **Step 6: Update any existing freeze-based Wave test**

Run: `cd src/go/internal/ui && grep -rn 'wave-freeze-btn\|waveControls\|newWaveControls\|TabWave' --include='*_test.go'`
For each hit: if a test asserts a Wave freeze pill exists, or constructs `newWaveControls(z, onFreeze)` expecting freeze behavior, update it to the AUTO contract (tag `wave-auto-btn`, no freeze). If a mobile-freeze matrix test includes `TabWave`, drop `TabWave` from that matrix (Wave no longer has a freeze pill). Show the concrete edit for each occurrence found; do not leave a stale freeze assertion.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControlsHasAutoNotFreeze|TestResolveWaveGain'`
Expected: PASS.

- [ ] **Step 8: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean. Leave in working tree.

---

## Task 5: Scroll-wheel manual zoom (disables AUTO) via the real tree

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go` (add `waveZoomHandler`; register a Wave-body hit area in the hit-area builder ~1311)
- Test: `src/go/internal/ui/wave_tab_scale_test.go` (functional wheel test)

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/wave_tab_scale_test.go`:

```go
func TestWaveWheelZoomDisablesAuto(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: true, waveYGain: 1.0}
	h := &waveZoomHandler{zone: z}

	// Scroll up: gain increases, auto turns off.
	if res := h.OnWheel(10, 10, 2); res != InputConsumed {
		t.Fatalf("OnWheel result = %v, want InputConsumed", res)
	}
	if z.waveAutoGain {
		t.Fatalf("wheel zoom must disable auto-gain")
	}
	if z.waveYGain <= 1.0 {
		t.Fatalf("scroll-up should increase yGain, got %v", z.waveYGain)
	}

	// Clamp ceiling.
	for i := 0; i < 100; i++ {
		h.OnWheel(10, 10, 5)
	}
	if z.waveYGain > 16.0 {
		t.Fatalf("yGain exceeded clamp: %v", z.waveYGain)
	}

	// Clamp floor (zoom out).
	for i := 0; i < 200; i++ {
		h.OnWheel(10, 10, -5)
	}
	if z.waveYGain < 0.25 {
		t.Fatalf("yGain below clamp: %v", z.waveYGain)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveWheelZoomDisablesAuto'`
Expected: FAIL — `waveZoomHandler undefined`.

- [ ] **Step 3: Implement `waveZoomHandler`**

Add to `eq_panel_zone.go` near the other hit adapters (after `buttonHitAdapter`, ~line 1388):

```go
// waveZoomHandler implements HitHandler for scroll-wheel Y-zoom on the Wave
// tab's content area. The Wave tab has only one axis, so plain scroll zooms Y
// (no Shift needed). Any scroll disables auto-gain so the manual scale sticks;
// re-enable via the AUTO pill. Mirrors chainZoomHandler's factor + clamp.
type waveZoomHandler struct {
	zone *EQPanelZone
}

func (h *waveZoomHandler) OnPress(x, y int) InputResult { return InputIgnored }
func (h *waveZoomHandler) OnDrag(x, y int)              {}
func (h *waveZoomHandler) OnRelease(x, y int)           {}
func (h *waveZoomHandler) OnWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}
	z := h.zone
	z.waveAutoGain = false
	if z.waveYGain <= 0 {
		z.waveYGain = 1.0
	}
	z.waveYGain *= 1 + float64(steps)*0.15
	if z.waveYGain < 0.25 {
		z.waveYGain = 0.25
	}
	if z.waveYGain > 16.0 {
		z.waveYGain = 16.0
	}
	return InputConsumed
}
```

- [ ] **Step 4: Register the Wave-body wheel hit area**

In the hit-area builder, right after the mobile cursor-scrub block (~1316-1328), add a desktop+mobile wheel zone for the Wave tab content:

```go
	// Wave tab: scroll-wheel Y-zoom over the content rect. Registered below the
	// chrome (zIdx, chrome sits at zIdx+1) so the AUTO pill still wins a press,
	// but the wheel falls through to the zoom handler. The handler ignores
	// press/drag, so it never steals taps (mobile cursor-scrub still works).
	if z.tabState.ActiveTab() == TabWave {
		if cr := z.bodyRect(); !cr.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    cr,
				ZIndex:  zIdx,
				Handler: &waveZoomHandler{zone: z},
				Tag:     "wave-zoom",
			})
		}
	}
```

(Confirm the local variable holding the panel z is named `zIdx` in this function; if it differs, match the surrounding code.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveWheelZoomDisablesAuto|TestWaveControlsHasAutoNotFreeze|TestResolveWaveGain'`
Expected: PASS.

- [ ] **Step 6: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean. Leave in working tree.

---

## Task 6: Pixel proof + full verification

**Files:**
- Modify: `src/go/internal/ui/audio_panel_render_pixels_test.go`
- Verify: browser `audio_panel_render.browser.test.js`

- [ ] **Step 1: Add a quiet-fills-panel pixel test**

Read `audio_panel_render_pixels_test.go` to learn its existing Wave-tab helper (the `drawRect` interceptor + `testAnalyzerStateOverride`). Add a test that renders the same quiet signal at the resolved auto-gain vs gain 1.0 and asserts the auto-gain trace paints strictly more colored wave-trace rows (taller trace). Use the established interceptor pattern in that file — match its helpers exactly rather than inventing new ones. Concretely, the assertion is: count distinct Y rows touched by `colWaveTrace` rects inside the wave body; the auto path (call `drawAnalyzerWaveform(dst, r, quietCh, nil, nil, autoGainForPeak(peak), true)`) must exceed the flat path (`...,1.0, true`).

- [ ] **Step 2: Run the pixel test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Pixel|Waveform'`
Expected: PASS (your new test + existing wave pixel tests still green).

- [ ] **Step 3: Run the full Wave/audio-panel Go test set (scoped)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Wave|AutoGain|AudioPanel|TabControls|Freeze'`
Expected: PASS. If a failure appears, confirm it references YOUR new symbols before treating it as real — the suite has pre-existing unrelated failures (see header). Fix anything in your surface area.

- [ ] **Step 4: Token-discipline + design guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDDrift|TestTokenDiscipline'`
Expected: PASS. (No new inline `color.RGBA{}` literals were added — all colors use existing `col*` accessors. If `TestTokenDiscipline` complains, you introduced a literal; replace it with an accessor.)

- [ ] **Step 5: Browser render smoke**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/audio_panel_render.browser.test.js`
Expected: PASS (Wave still renders desktop + mobile; the badge addition doesn't break the histogram-dominance check). If WASM build is heavy, `WASM_PREBUILT=1` after a first `make wasm`. This step needs a browser toolchain; if unavailable in this environment, note it as "not run here" and flag for the user.

- [ ] **Step 6: Final checkpoint (no commit)**

Summarize what changed and the verification output. Leave everything in the working tree — the user owns all commits.

---

## Self-Review notes (author)

- **Spec coverage:** remove pause → Task 4; auto-scale on by default → Task 3 (default true) + Task 1 (gain math); smoothed peak-hold → Task 1 `advanceWavePeak` + Task 3 once-per-refresh advance; dynamic labels + badge → Task 2; disable + zoom → Task 4 (AUTO toggle) + Task 5 (wheel); readout clarity / clip overlay unchanged → Task 2 (clip overlay untouched). All spec sections map to a task.
- **Type consistency:** `drawAnalyzerWaveform(..., gain float64, autoOn bool)` used identically in Task 2 (def + temp callers) and Task 3 (real callers). `resolveWaveGain(state, ch, cap) (float64, bool)` consistent across Task 3 def and Task 3/Task tests. `newWaveControls(z int, onAuto func() bool)` consistent in Task 4 def + ctor wiring. Fields `waveAutoGain/waveYGain/waveSmoothedPeak/waveSmoothState/waveKeyGain` declared in Task 3, used in Tasks 3/4/5.
- **No placeholders:** every code step shows the code; Task 6 Step 1 references the existing interceptor pattern (must be read in-file) but states the exact assertion.
- **Open verification risk:** Task 5 Step 4 assumes the panel-z local is `zIdx` — the implementer must confirm against the surrounding builder code and match it.
```
