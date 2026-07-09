//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestRowRackNotHiddenBehindAudioPanel_SynthTab is the regression for the
// screenshot bug: the add-row "+" button (and the bottom band of the row rack)
// render UNDERNEATH the bottom audio/EQ panel on the Synth tab.
//
// Root cause: the row rack's bottom edge is `rowsBottom() == Bounds.Max.Y -
// dv.eqH`, but the actually-painted audio panel rect (`dv.eqRect`) is floored
// independently. On the Synth tab, drumview_layout.go applies a hard
// `minSynthPanelH = 240` floor to `dv.eqRect` that is NOT mirrored into
// `dv.eqH` (drumview_geometry.go). When `PanelHeightAt(Bounds.Dy())` resolves
// below 240 — a short viewport where the 0.60 screen-fraction cap bites, on the
// Synth tab (exactly the screenshot) — `dv.eqRect.Min.Y < rowsBottom()`, so the
// row rack extends below the panel top and the add-row button is occluded.
//
// The invariant: NO part of the row rack (the main instrument drum-view panel)
// may sit below the top of the audio panel.
func TestRowRackNotHiddenBehindAudioPanel_SynthTab(t *testing.T) {
	assertDefaultParityState(t)

	// Clean DESKTOP profile (guard against a leaked mobile profile from a
	// prior test — mirrors driveRealEQDivider).
	forceSmallScreenForTest = false
	SetTouchScreenSize(0, 0)
	UpdateProfile()
	t.Cleanup(func() { SetTouchScreenSize(0, 0); UpdateProfile() })

	// Exercise the REAL production-floored panel (not the test-mode collapse).
	eqPanelHeightForTest = 190
	t.Cleanup(func() { eqPanelHeightForTest = 0 })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// A perfectly ordinary 1280x720 desktop window (the screenshot's short look
	// is just a crop). The drum-view pane is the bottom split (~360 px tall), so
	// PanelHeightAt (capped at 0.60*360 = 216 px) resolves BELOW the Synth tab's
	// 240 px floor — the exact divergence that stranded the add-row "+" behind
	// the panel. Rows and the "+" are genuinely visible here, so INVARIANT 2 is
	// a real (non-vacuous) occlusion check.
	const vpW, vpH = 1280, 720
	g.Layout(vpW, vpH)

	// Real input wiring so g.Update() dispatches through the whole stack.
	mx, my := new(int), new(bool)
	_ = my
	restore := SetInputForTest(
		func() (int, int) { return *mx, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return vpW, vpH },
	)
	t.Cleanup(restore)

	dv := g.drum

	// Bind row 0 to a recipe so the Synth tab has a real ParamDef set, then
	// switch to the Synth tab through the production path.
	if len(dv.Rows) > 0 && dv.Rows[0] != nil && dv.Rows[0].Instrument != "" {
		id := dv.Rows[0].Instrument
		audio.BindInstrumentToRecipe(id, "drum-snare")
		t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	}
	dv.eqPanelZone.SetActiveTab(TabSynth)

	// Settle the layout through the real per-frame loop.
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}

	// Precondition: we really are on the Synth tab and the panel is floored
	// (top edge above the widget-board boundary) — otherwise the production
	// scenario is not reproduced.
	if dv.eqPanelZone.ActiveTab() != TabSynth {
		t.Fatalf("precondition: expected Synth tab active, got %v", dv.eqPanelZone.ActiveTab())
	}

	eqTop := dv.eqRect.Min.Y
	rackBottom := dv.rowsBottom()

	// INVARIANT 1: the row rack must end at or above the audio panel top.
	if rackBottom > eqTop {
		t.Errorf("row rack extends BELOW the audio panel top: rowsBottom()=%d > eqRect.Min.Y=%d (overlap %d px). "+
			"The add-row + button (anchored at rowsBottom) is hidden behind the panel.",
			rackBottom, eqTop, rackBottom-eqTop)
	}

	// INVARIANT 2: the add-row "+" button must be present AND not occluded by
	// the panel. Assert non-empty first so this scenario really exercises a
	// visible button (a collapsed/empty button would satisfy the overlap check
	// vacuously).
	if dv.rowRackZone != nil {
		btn := dv.rowRackZone.AddRowButton().Rect()
		if btn.Empty() {
			t.Errorf("add-row + button is empty at %dx%d — expected a visible button in this viewport", vpW, vpH)
		} else if btn.Overlaps(dv.eqRect) {
			t.Errorf("add-row + button %v overlaps the audio panel %v — the button is drawn (z=120) beneath the panel (z=130) and is visually hidden",
				btn, dv.eqRect)
		}
	}

	// INVARIANT 3 (general): the row rack zone rect itself must clear the panel.
	if dv.rowRackZone != nil {
		rack := dv.rowRackZone.rect
		if !rack.Empty() && rack.Max.Y > eqTop {
			t.Errorf("row rack zone rect %v extends below audio panel top (eqRect.Min.Y=%d); the main instrument panel is partially hidden",
				rack, eqTop)
		}
	}

	// Sanity: keep the assertions meaningful — the panel must actually be
	// present (non-empty) below the rack.
	if dv.eqRect.Empty() {
		t.Fatalf("eqRect empty; scenario not exercised (rack=%v)", image.Rectangle{})
	}
}
