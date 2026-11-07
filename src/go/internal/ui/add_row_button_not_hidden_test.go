//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// No part of the row rack sits below the top of the audio panel. The Synth tab floors the painted panel at
// 240 px without moving the rack's bottom edge with it, so a short viewport hid the add-row "+" behind it.
func TestRowRackNotHiddenBehindAudioPanel_SynthTab(t *testing.T) {
	assertDefaultParityState(t)

	// Against a mobile profile leaked by an earlier test.
	forceSmallScreenForTest = false
	SetTouchScreenSize(0, 0)
	UpdateProfile()
	t.Cleanup(func() { SetTouchScreenSize(0, 0); UpdateProfile() })

	// Exercise the REAL production-floored panel (not the test-mode collapse).
	eqPanelHeightForTest = 190
	t.Cleanup(func() { eqPanelHeightForTest = 0 })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// At 1280x720 the drum pane is ~360 px, so the 0.60 cap (216 px) falls below the 240 px floor: the divergence,
	// with the rows and "+" really visible.
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

	// Row 0 bound, so the tab has real ParamDefs; then the production tab path.
	if len(dv.Rows) > 0 && dv.Rows[0] != nil && dv.Rows[0].Instrument != "" {
		id := dv.Rows[0].Instrument
		_ = audio.InstallInstrumentDocForTest(id, roster.Snare)
		t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	}
	dv.eqPanelZone.SetActiveTab(TabSynth)

	// Settle the layout through the real per-frame loop.
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}

	// Otherwise the production scenario is not reproduced.
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

	// Non-empty first, or the overlap check passes vacuously.
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

	if dv.eqRect.Empty() {
		t.Fatalf("eqRect empty; scenario not exercised (rack=%v)", image.Rectangle{})
	}
}
