//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestLegendText_CoversEveryTab — every PanelTab must have non-empty
// legend text. Drift here = missing kid-readable explanation.
func TestLegendText_CoversEveryTab(t *testing.T) {
	for _, tab := range AllPanelTabs() {
		if LegendText(tab) == "" {
			t.Errorf("tab=%v: LegendText empty (every tab must have a 3-sentence explanation)", tab)
		}
	}
}

// TestDrawAudioPanelLegend_ProducesSheet — invoking the popover
// renderer with a populated chip rect must produce filled rects
// (background + accent border at minimum).
func TestDrawAudioPanelLegend_ProducesSheet(t *testing.T) {
	dst := ebiten.NewImage(800, 200)
	chipR := image.Rect(700, 4, 720, 22)
	var rects []drawnRect
	rects = collectFilledRects(t, func() {
		drawAudioPanelLegend(dst, chipR, 800, TabSpectrum)
	})
	if len(rects) == 0 {
		t.Errorf("expected legend popover rects, got 0")
	}
}

// TestStickyBar_LegendAndExpanderAlwaysVisible — the ? chip and the
// chevron-down pill must claim non-empty rects on every tab (not
// just one).
func TestStickyBar_LegendAndExpanderAlwaysVisible(t *testing.T) {
	for _, tab := range AllPanelTabs() {
		bar := NewAudioStickyBar(0, func() {}, func(PanelTab) {})
		bar.SetActiveTab(tab)
		bar.Layout(image.Rect(0, 0, 800, stickyBarH))
		if lg := bar.LegendBtn(); lg == nil || lg.Rect().Empty() {
			t.Errorf("tab=%v: legend pill must claim a rect", tab)
		}
		if ex := bar.ExpanderBtn(); ex == nil || ex.Rect().Empty() {
			t.Errorf("tab=%v: expander pill must claim a rect", tab)
		}
	}
}

// TestStickyBar_ExpanderTogglesState — clicking the expander pill via
// the bound OnClick toggles PanelTabState.Expanded.
func TestStickyBar_ExpanderTogglesPanelState(t *testing.T) {
	cb := EQCallbacks{}
	z := NewEQPanelZone(cb)
	if z.tabState == nil {
		t.Fatalf("tabState should be non-nil")
	}
	if z.tabState.Expanded() {
		t.Fatalf("expander should default to collapsed")
	}
	ex := z.stickyBar.ExpanderBtn()
	if ex == nil || ex.OnClick == nil {
		t.Fatalf("expander button or OnClick missing")
	}
	ex.OnClick()
	if !z.tabState.Expanded() {
		t.Errorf("after click expander: Expanded()=false want true")
	}
	ex.OnClick()
	if z.tabState.Expanded() {
		t.Errorf("after second click expander: Expanded()=true want false")
	}
}

// TestWrapTextToWidth_KeepsWordsWhole — the kid-readable popover wrap
// must never split a word.
func TestWrapTextToWidth_KeepsWordsWhole(t *testing.T) {
	out := wrapTextToWidth("the quick brown fox jumped", 60)
	if len(out) < 2 {
		t.Errorf("expected ≥2 lines from narrow wrap, got %d (lines=%v)", len(out), out)
	}
	// No line should contain a partial word: each word from the original
	// must appear intact in one of the wrapped lines.
	words := []string{"the", "quick", "brown", "fox", "jumped"}
	for _, w := range words {
		found := false
		for _, ln := range out {
			if ln == w || hasWordExact(ln, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("word %q missing in wrapped output %v", w, out)
		}
	}
}

func hasWordExact(line, word string) bool {
	// Naive substring check ringed by word boundaries.
	if len(line) < len(word) {
		return false
	}
	for i := 0; i+len(word) <= len(line); i++ {
		if line[i:i+len(word)] != word {
			continue
		}
		leftOK := i == 0 || line[i-1] == ' '
		rightOK := i+len(word) == len(line) || line[i+len(word)] == ' '
		if leftOK && rightOK {
			return true
		}
	}
	return false
}
