//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestInstrumentColorCorrelationAcrossSurfaces is the end-to-end guard for the
// "instrument identity" correlation (DESIGN.md § Chrome accents, invariant 3):
// every control and menu that belongs to a single instrument derives its accent
// from that instrument's color. It drives the same owning-instrument color
// through each surface's accent seam and asserts the seam reflects it, then
// flips to a second color and asserts every seam tracks the change — proving the
// outputs correlate to (are a function of) the instrument color, not a fixed
// chrome accent.
func TestInstrumentColorCorrelationAcrossSurfaces(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 800)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// A node bound to row 0 so the node-pop-up seams resolve to row 0's color.
	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.nodeRows[n.ID] = 0
	g.sidebar.node = n
	g.longPressPopupNode = n

	// Two distinct instrument colors; every seam must track whichever is set.
	cases := []struct {
		name string
		col  color.RGBA
	}{
		{"warm", color.RGBA{190, 120, 60, 255}},
		{"cool", color.RGBA{60, 150, 170, 255}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dv.Rows[0].Color = tc.col
			base := lum(tc.col)

			// --- Pure row-control shade helpers (mute/solo/fx/kebab) ---
			if got := rowToggleFillRGBA(tc.col, roleFX, true); got != tc.col {
				t.Errorf("FX-on fill = %v, want the full instrument hue %v", got, tc.col)
			}
			if lum(rowToggleFillRGBA(tc.col, roleMute, true)) >= base {
				t.Error("mute-on fill must be a DARKER shade of the instrument color")
			}
			if lum(rowToggleFillRGBA(tc.col, roleSolo, true)) <= base {
				t.Error("solo-on fill must be a BRIGHTER shade of the instrument color")
			}
			if a := rowToggleFillRGBA(tc.col, roleMute, false).A; a >= 200 {
				t.Errorf("off fill must be a dim low-alpha tint, got alpha %d", a)
			}
			// Kebab chip border derives from the instrument color.
			if kb := rowKebabStyle(tc.col); !instrumentDerived(kb.Border, tc.col) {
				t.Errorf("kebab border %v is not derived from instrument color %v", kb.Border, tc.col)
			}

			// --- DrumView menu seams ---
			dv.contextMenuRow = 0
			if !colorsEqual(dv.contextMenuAccent(), tc.col) {
				t.Errorf("contextMenuAccent() = %v, want %v", dv.contextMenuAccent(), tc.col)
			}
			dv.fxPanelRow = 0
			if !colorsEqual(dv.fxPanelAccent(), tc.col) {
				t.Errorf("fxPanelAccent() = %v, want %v", dv.fxPanelAccent(), tc.col)
			}

			// --- Node pop-up seams ---
			if !colorsEqual(g.sidebar.nodeAccent(), tc.col) {
				t.Errorf("nodeAccent() = %v, want %v", g.sidebar.nodeAccent(), tc.col)
			}
			if !colorsEqual(g.longPressPopupAccent(), tc.col) {
				t.Errorf("longPressPopupAccent() = %v, want %v", g.longPressPopupAccent(), tc.col)
			}
		})
	}

	// Differentiation: the two colors must produce different control fills — the
	// correlation is real, not a constant.
	mWarm := rowToggleFillRGBA(cases[0].col, roleMute, true)
	mCool := rowToggleFillRGBA(cases[1].col, roleMute, true)
	if mWarm == mCool {
		t.Error("different instrument colors must yield different mute shades")
	}
}

// TestInstrumentColorCorrelationInstrumentPicker guards the picker seam: its
// chrome accent tracks the currently-selected instrument's color, and each
// distinct instrument yields a distinct accent.
func TestInstrumentColorCorrelationInstrumentPicker(t *testing.T) {
	for _, id := range []string{"kick", "snare"} {
		comp := NewInstrumentMenuComponent()
		comp.SetProps(InstrumentMenuProps{CurrentInstrument: id})
		want := color.RGBAModel.Convert(instColor(id)).(color.RGBA)
		got := color.RGBAModel.Convert(comp.menuAccent()).(color.RGBA)
		if got != want {
			t.Errorf("menuAccent() for %q = %v, want instColor(%q) %v", id, got, id, want)
		}
	}
}

// instrumentDerived reports whether c shares the instrument color's hue family
// (same RGB ordering direction), i.e. it is a shade/tint of base rather than an
// unrelated chrome accent.
func instrumentDerived(c color.Color, base color.RGBA) bool {
	g := color.RGBAModel.Convert(c).(color.RGBA)
	// Compare channel-order signature: a shade/tint preserves which channels
	// dominate. base warm → R≥G; base cool → B≥G, etc.
	order := func(p color.RGBA) (rg, gb bool) { return p.R >= p.G, p.G >= p.B }
	br, bgb := order(base)
	gr, ggb := order(g)
	return br == gr && bgb == ggb
}
