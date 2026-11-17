//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// TestEQWaveButtonRendersWithoutTilde verifies that the EQ/Wave toggle button
// displays the current tab name without a tilde prefix.
func TestEQWaveButtonRendersWithoutTilde(t *testing.T) {
	assertDefaultParityState(t)

	z := NewEQPanelZone(EQCallbacks{})

	// Default state: EQ mode, button shows "EQ" (current tab).
	if z.ActiveTab() == TabWave {
		t.Fatal("expected default EQ mode")
	}

	got := z.toggleButtonLabel()
	if strings.Contains(got, "~") {
		t.Fatalf("toggle button label in EQ mode should not contain tilde, got %q", got)
	}
	if got != "EQ" {
		t.Fatalf("expected display text 'EQ' in EQ mode, got %q", got)
	}

	// Switch to waveform mode — button shows "Wave" (current tab).
	z.tabState.SetActiveTab(TabWave)
	got = z.toggleButtonLabel()
	if strings.Contains(got, "~") {
		t.Fatalf("toggle button label in waveform mode should not contain tilde, got %q", got)
	}
	if got != "Wave" {
		t.Fatalf("expected display text 'Wave' in waveform mode, got %q", got)
	}

	// Toggle back to EQ mode — shows "EQ".
	z.tabState.SetActiveTab(TabEQ)
	got = z.toggleButtonLabel()
	if got != "EQ" {
		t.Fatalf("expected display text 'EQ' after toggling back, got %q", got)
	}
}

// TestEQWaveButtonAlwaysHasVisibleBody verifies that the EQ/Wave toggle button
// has a visible silhouette in BOTH active and inactive states.
//
// Formerly TestEQWaveButtonAlwaysHasBorder. The flat-tonal restyle removed
// borders at rest (DESIGN.md "Elevation & Depth": a control's silhouette is a
// tone step, never a line), so asserting a stroke would now pin the wrong
// thing. The real intent — "this control is never invisible" — is preserved by
// asserting a FILLED body instead.
func TestEQWaveButtonAlwaysHasVisibleBody(t *testing.T) {
	assertDefaultParityState(t)

	z := NewEQPanelZone(EQCallbacks{})
	// Give the zone a layout rect so the toggle button gets positioned.
	z.Layout(image.Rect(0, 0, 400, 200))

	toggleRect := z.stickyBar.TabBtn(0).Rect()
	if toggleRect.Empty() {
		t.Fatal("toggle button rect is empty after layout")
	}

	screen := ebiten.NewImage(400, 200)

	// Flat-tonal: the pill paints one FILLED rounded rect as its body. Accept
	// either the uncached primitive or the cached rounded-button composite.
	hasFilledBody := func(rec *drawCallRecorder) bool {
		for _, c := range rec.inRegion(toggleRect) {
			if c.Kind == drawCallRoundedRect && c.Filled {
				return true
			}
			if c.Kind == drawCallRoundedButton {
				return true
			}
		}
		return false
	}

	// Active state (waveformMode=true, shows "EQ").
	z.tabState.SetActiveTab(TabWave)
	var rec drawCallRecorder
	rec.record(t, func() { z.Draw(screen) })
	if !hasFilledBody(&rec) {
		t.Fatal("active toggle button (EQ) has no filled body — control is invisible")
	}

	// Inactive state (waveformMode=false, shows "Wave").
	z.tabState.SetActiveTab(TabEQ)
	rec.record(t, func() { z.Draw(screen) })
	if !hasFilledBody(&rec) {
		t.Fatal("inactive toggle button (Wave) has no filled body — control is invisible")
	}
}

// TestEQPillTabsDrawAboveSpectrumBands verifies that the EQ panel pill tab
// buttons (channel, HPF, LPF, toggle) are drawn after the spectrum band
// overlays so they are not occluded.
func TestEQPillTabsDrawAboveSpectrumBands(t *testing.T) {
	assertDefaultParityState(t)

	// Provide mock spectrum data so drawSpectrumBars actually draws band fills.
	mockSpectrum := make([]float64, 256)
	for i := range mockSpectrum {
		mockSpectrum[i] = 0.5
	}
	z := NewEQPanelZone(EQCallbacks{
		AnalyzerSnapshot: func(_ channel.ID) audio.AnalyzerSnapshot {
			return audio.AnalyzerSnapshot{Spectrum: mockSpectrum}
		},
	})
	z.Layout(image.Rect(0, 0, 400, 200))

	// Collect all button rects.
	btnRects := map[string]image.Rectangle{
		"channel": z.stickyBar.ChannelBtn().Rect(),
		"toggle":  z.stickyBar.TabBtn(0).Rect(),
		"hpf":     z.hpfBtn.Rect(),
		"lpf":     z.lpfBtn.Rect(),
	}

	screen := ebiten.NewImage(400, 200)
	var rec drawCallRecorder
	rec.record(t, func() { z.Draw(screen) })

	// For each button region, find the highest-sequence drawCallRoundedRect
	// (the pill tab border) and the highest-sequence spectrum band fill.
	// Band fills are tall drawRect calls that span significantly more height
	// than the button itself (drawRoundedRect internally emits small drawRect
	// calls for corners/body which must not be mistaken for band fills).
	panelRect := image.Rect(0, 0, 400, 200)
	for name, br := range btnRects {
		if br.Empty() {
			continue
		}
		minPillSeq := -1
		maxBandFillSeq := -1
		for _, c := range rec.inRegion(br) {
			// Pill tabs draw their shell + cap face as cached drawRoundedButton
			// composites (keycap redesign); the legacy raw stroked rounded-rect
			// path still counts for any pill not yet migrated.
			if c.Kind == drawCallRoundedRect || c.Kind == drawCallRoundedButton {
				if minPillSeq < 0 || c.Seq < minPillSeq {
					minPillSeq = c.Seq
				}
			}
			// Spectrum band fills span the full panel height.
			isBandFill := c.Kind == drawCallRect && c.Filled &&
				c.Rect.Min.Y == panelRect.Min.Y && c.Rect.Max.Y == panelRect.Max.Y
			if isBandFill && c.Seq > maxBandFillSeq {
				maxBandFillSeq = c.Seq
			}
		}
		if minPillSeq < 0 {
			t.Errorf("%s button: no rounded-rect draw found in region %v", name, br)
			continue
		}
		if maxBandFillSeq >= 0 && minPillSeq < maxBandFillSeq {
			t.Errorf("%s button: pill tab (seq=%d) drawn before spectrum band fill (seq=%d) — button will be occluded",
				name, minPillSeq, maxBandFillSeq)
		}
	}
}
