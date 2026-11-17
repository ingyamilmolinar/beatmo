package ui

import (
	"image/color"
	"testing"
)

func TestPointBeatsAndPointIJ(t *testing.T) {
	// MaxDiv defaults to 32 for a freshly-constructed grid.
	g := NewGrid(64)
	if got := PointBeats(g, 1, 0.5); got != [2]int{32, 16} {
		t.Fatalf("PointBeats(1,0.5) on MaxDiv=32 grid: %v want [32 16]", got)
	}
	// Negative + rounding behavior of ToSub.
	if got := PointBeats(g, -0.5, 0.0); got != [2]int{-16, 0} {
		t.Fatalf("PointBeats(-0.5,0): %v want [-16 0]", got)
	}
	// PointIJ is a pass-through.
	if got := PointIJ(7, -3); got != [2]int{7, -3} {
		t.Fatalf("PointIJ(7,-3): %v", got)
	}
}

func TestCameraGeoMAndRounded(t *testing.T) {
	c := &Camera{Scale: 2.5, OffsetX: 10.4, OffsetY: -3.6}

	// Read translate via Apply(0,0); scale via Apply(1,1) - translate.
	m := c.GeoM()
	tx, ty := m.Apply(0, 0)
	if tx != 10.4 {
		t.Fatalf("GeoM tx=%v want 10.4 (no rounding)", tx)
	}
	if ty != -3.6 {
		t.Fatalf("GeoM ty=%v want -3.6 (no rounding)", ty)
	}
	sx, sy := m.Apply(1, 1)
	if sx-tx != 2.5 {
		t.Fatalf("GeoM scaleX=%v want 2.5", sx-tx)
	}
	if sy-ty != 2.5 {
		t.Fatalf("GeoM scaleY=%v want 2.5", sy-ty)
	}

	mr := c.GeoMRounded()
	rtx, rty := mr.Apply(0, 0)
	if rtx != 10 {
		t.Fatalf("GeoMRounded tx=%v want 10", rtx)
	}
	if rty != -4 {
		t.Fatalf("GeoMRounded ty=%v want -4", rty)
	}
	rsx, _ := mr.Apply(1, 0)
	if rsx-rtx != 2.5 {
		t.Fatalf("GeoMRounded scaleX=%v want 2.5 (scale must not be rounded)", rsx-rtx)
	}
}

func TestBlendColor(t *testing.T) {
	base := color.RGBA{R: 0, G: 0, B: 0, A: 100}
	accent := color.RGBA{R: 200, G: 100, B: 50, A: 200}

	if got := blendColor(base, accent, 0); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("t=0: got %+v want base RGB with A=255", got)
	}
	if got := blendColor(base, accent, 1); got != (color.RGBA{200, 100, 50, 255}) {
		t.Fatalf("t=1: got %+v want accent RGB with A=255", got)
	}
	got := blendColor(base, accent, 0.5)
	if got.R != 100 || got.G != 50 || got.B != 25 {
		t.Fatalf("t=0.5: got %+v want (100,50,25,255)", got)
	}
	if got.A != 255 {
		t.Fatalf("alpha must always be 255; got %d", got.A)
	}

	// Direction matters: from non-zero base to lower accent ramps down correctly.
	from := color.RGBA{R: 100, G: 0, B: 0, A: 0}
	to := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	if g := blendColor(from, to, 0.5); g.R != 50 {
		t.Fatalf("ramp-down R: got %d want 50", g.R)
	}
}

func TestMaxIAbsI(t *testing.T) {
	if maxI(1, 2) != 2 {
		t.Fatal("maxI(1,2)")
	}
	if maxI(2, 1) != 2 {
		t.Fatal("maxI(2,1)")
	}
	if maxI(-3, -7) != -3 {
		t.Fatal("maxI(-3,-7)")
	}
	if maxI(5, 5) != 5 {
		t.Fatal("maxI(5,5)")
	}

	if absI(0) != 0 {
		t.Fatal("absI(0)")
	}
	if absI(7) != 7 {
		t.Fatal("absI(7)")
	}
	if absI(-7) != 7 {
		t.Fatal("absI(-7)")
	}
}

func TestInteractionDeltaIsZero(t *testing.T) {
	if !(InteractionDelta{}).IsZero() {
		t.Fatal("zero delta should be IsZero=true")
	}
	cases := []InteractionDelta{
		{FillDelta: 1},
		{BorderDelta: -1},
		{HighlightDelta: 2},
		{FillDelta: 1, BorderDelta: 1, HighlightDelta: 1},
	}
	for _, d := range cases {
		if d.IsZero() {
			t.Fatalf("non-zero delta should not be IsZero: %+v", d)
		}
	}
}
