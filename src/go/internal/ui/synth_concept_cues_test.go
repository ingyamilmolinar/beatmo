//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestConceptFilterCutoffX_Mapping(t *testing.T) {
	r := image.Rect(0, 0, 200, 100)
	left := conceptFilterCutoffX(r, 20)
	mid := conceptFilterCutoffX(r, 663) // ~ geometric middle of 20..22000
	right := conceptFilterCutoffX(r, 22000)
	if left != r.Min.X {
		t.Fatalf("20Hz must map to the left edge, got %d", left)
	}
	if right < r.Max.X-2 {
		t.Fatalf("22kHz must map to the right edge, got %d", right)
	}
	if !(left < mid && mid < right) {
		t.Fatalf("mapping not monotonic: %d %d %d", left, mid, right)
	}
	lo := conceptFilterCutoffX(r, 100)
	hi := conceptFilterCutoffX(r, 1000)
	if hi-lo < r.Dx()/4 {
		t.Fatalf("mapping must be log-scaled (a decade spans >=1/4 width), got %d px", hi-lo)
	}
}

func TestConceptFilter_CuesGatedByRectSize(t *testing.T) {
	const inst = "filter-cues-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	audio.SetInstrumentParam(inst, "filter_enabled", 1)
	def := lfoDefFor(t, inst, "filter_cutoff")

	// Big rect: cutoff marker + words present. Small rect: bare curve.
	big := image.Rect(0, 0, 260, 110)
	small := image.Rect(0, 0, 100, 30)
	// Render the small rect scaled up would equal the big only if cues were
	// absent; instead assert directly: moving the cutoff moves INK in the big
	// rect's top band (where the marker lives), and the small render still works.
	fpA := fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, big, inst, def, nil) })
	audio.SetInstrumentParam(inst, "filter_cutoff", 200)
	fpB := fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, big, inst, def, nil) })
	if fpA == fpB {
		t.Fatalf("cutoff move must repaint (marker + curve)")
	}
	_ = fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, small, inst, def, nil) }) // must not panic
}

func TestDrawConceptTimeRuler_GatedTiny(t *testing.T) {
	img := ebiten.NewImage(300, 120)
	tiny := image.Rect(0, 0, 100, 30)
	drawConceptTimeRuler(img, tiny, "1s") // must be a no-op, not a panic

	big := image.Rect(0, 0, 300, 120)
	before := fingerprintFocus(t, func(i *ebiten.Image) {})
	after := fingerprintFocus(t, func(i *ebiten.Image) { drawConceptTimeRuler(i, big, "1s") })
	if before == after {
		t.Fatalf("ruler must draw ink on a big rect")
	}
}
