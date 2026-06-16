package ui

import (
	"image"
	"testing"
)

func TestAudioPillSizingHelpers(t *testing.T) {
	restoreC := SetDensityForTest(DensityComfortable)
	if h := audioPillHeight(); h < 24 {
		t.Fatalf("comfortable audioPillHeight=%d, want >=24", h)
	}
	restoreC()
	restoreS := SetDensityForTest(DensitySpacious)
	if h := audioPillHeight(); h < 30 {
		t.Fatalf("spacious audioPillHeight=%d, want >=30", h)
	}
	restoreS()

	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := audioPillHeight()
	pad := Profile().DensityValues().AudioPillPadX

	icon := NewButton("", InstButtonStyle, nil)
	icon.Icon = string(IconPause)
	if w := audioPillWidth(icon); w != h {
		t.Fatalf("icon pill width=%d, want square %d", w, h)
	}

	text := NewButton("AUTO", InstButtonStyle, nil)
	wantW := TextWidth("AUTO") + 2*pad
	if wantW < h {
		wantW = h
	}
	if w := audioPillWidth(text); w != wantW {
		t.Fatalf("text pill width=%d, want %d", w, wantW)
	}

	tiny := NewButton("R", InstButtonStyle, nil)
	if w := audioPillWidth(tiny); w < h {
		t.Fatalf("1-char pill width=%d, want >= square %d", w, h)
	}
}

func TestAnalyzerPillsUniformHeightAndTouch(t *testing.T) {
	forceSmallScreenForTest = true // mobile: touch-min hit areas
	t.Cleanup(func() { forceSmallScreenForTest = false })
	restore := SetDensityForTest(DensitySpacious) // mobile sizing
	defer restore()
	h := audioPillHeight()
	header := image.Rect(0, 0, 400, controlHeaderHeight())

	wave := newWaveControls(131, func() bool { return false }, func() bool { return false })
	wave.Layout(header)
	for _, b := range []*Button{wave.autoBtn, wave.freezeBtn} {
		if r := b.Rect(); !r.Empty() && r.Dy() != h {
			t.Fatalf("wave pill height=%d, want %d", r.Dy(), h)
		}
	}
	for _, ha := range wave.HitAreas() {
		if !ha.Touch || ha.ClipRect.Dx() < 44 || ha.ClipRect.Dy() < 44 {
			t.Fatalf("wave hit %q not touch-min: touch=%v clip=%v", ha.Tag, ha.Touch, ha.ClipRect)
		}
	}
}

func TestChainPillsUnified(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := audioPillHeight()
	cz := NewChainPanelZone(ChainCallbacks{})
	cz.Layout(image.Rect(0, 300, 600, 600))
	for _, b := range []*Button{cz.OverlayBtn(), cz.SplitBtn(), cz.DiffBtn(), cz.AGBtn(), cz.FitBtn(), cz.FreezeBtn()} {
		if b == nil {
			continue
		}
		if r := b.Rect(); !r.Empty() && r.Dy() != h {
			t.Fatalf("chain pill height=%d, want unified %d", r.Dy(), h)
		}
	}
}

func TestChainHasNoCloseButton(t *testing.T) {
	cz := NewChainPanelZone(ChainCallbacks{})
	cz.Layout(image.Rect(0, 300, 600, 460))
	for _, ha := range cz.HitAreas() {
		if ha.Tag == "scope-close-btn" {
			t.Fatal("Chain still publishes scope-close-btn; the X must be removed")
		}
	}
}
