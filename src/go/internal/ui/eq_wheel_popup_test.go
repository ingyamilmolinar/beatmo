//go:build test

package ui

import "testing"

// setupEQTabForTest brings up a full Game+DrumView, sizes it, switches to the
// EQ tab, and forces a zone re-layout — the canonical sequence used by
// eq_panel_zone_input_isolation_test.go's TestEQPanelZoneIsOpaqueToZ (g.Layout
// -> recalcButtons -> tabState.SetActiveTab -> Invalidate -> zone.Layout).
func setupEQTabForTest(g *Game, w, h int) {
	g.Layout(w, h)
	g.drum.recalcButtons()
	g.drum.eqPanelZone.tabState.SetActiveTab(TabEQ)
	g.drum.eqPanelZone.Invalidate()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

// pressing an EQ dB readout cell opens the wheel popup on BOTH desktop and
// mobile, and the wheel edits the same source of truth as the curve.
func TestEQWheelPopup_OpensOnBothProfiles(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		mobile := mobile
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			if mobile {
				forceSmallScreenForTest = true
				t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
				UpdateProfile()
			}
			setupEQTabForTest(g, 1280, 720)
			dv := g.drum

			// Route a dB-cell press through the zone's OnPress adapter.
			a := &eqDBOpenAdapter{z: dv.eqPanelZone, band: 3}
			a.OnPress(0, 0)

			if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
				t.Fatalf("[%s] EQ wheel popup did not open on dB-cell press", name)
			}
		})
	}
}

// The wheel's OnChange writes bandGainsDB (single source of truth); a curve
// drag on the same band writes the SAME field — both surfaces agree.
func TestEQWheelPopup_SharesSourceOfTruthWithCurve(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(1)
	b := dv.eqWheelPopup.binding

	// Drive the wheel's live path: move the transient knob to a known dB and
	// fire the same OnChange the wheel fires per notch. This must land on
	// EQPanelZone.bandGainsDB — the SAME field the curve-drag path writes.
	b.Knob.Value = (6.0 - eqDBMin) / eqDBSpan // 6.0 dB in normalized knob space
	b.OnChange()
	if got := dv.eqPanelZone.bandGainsDB[1]; got != 6.0 {
		t.Fatalf("wheel OnChange wrote %v to bandGainsDB[1], want 6.0", got)
	}

	// The curve-drag path writes the exact same backing array via applyBandGainLive.
	dv.eqPanelZone.applyBandGainLive(1, -3.0)
	if got := dv.eqPanelZone.bandGainsDB[1]; got != -3.0 {
		t.Fatalf("curve/seam path wrote %v to bandGainsDB[1], want -3.0 (same source of truth)", got)
	}
}

// The wheel's center-box tap opens the numeric editor (numeric entry preserved).
func TestEQWheelPopup_CenterBoxOpensNumericEditor(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(2)
	dv.eqWheelPopup.binding.OpenEditor()
	if dv.eqPanelZone.paramEditor == nil || !dv.eqPanelZone.paramEditor.Active() {
		t.Fatal("center-box OpenEditor did not open the numeric editor")
	}
}

// Desktop sizing sanity: the EQ wheel popup's rect must be non-empty and fit
// entirely inside the DrumView bounds on the DESKTOP (comfortable-density)
// profile — no forceSmallScreenForTest here, mirroring the "desktop" branch
// of TestEQWheelPopup_OpensOnBothProfiles.
func TestEQWheelPopup_DesktopRectFitsBounds(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(0)
	if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
		t.Fatal("EQ wheel popup did not open on desktop")
	}

	r := dv.EQWheelPopupRect()
	if r.Empty() {
		t.Fatal("desktop EQ wheel popup rect is empty")
	}
	if !r.In(dv.Bounds) {
		t.Fatalf("desktop EQ wheel popup rect %v not inside DrumView bounds %v", r, dv.Bounds)
	}
}

// stubKnobStepSink is a minimal in-memory KnobStepSaveSink for tests, mirroring
// the shape of userprefs.fileStore/localStorageStore without touching disk or
// browser storage.
type stubKnobStepSink struct{ steps map[string]float64 }

func (s *stubKnobStepSink) LoadKnobSteps() map[string]float64 { return s.steps }
func (s *stubKnobStepSink) SaveKnobStep(name string, step float64) error {
	if s.steps == nil {
		s.steps = map[string]float64{}
	}
	s.steps[name] = step
	return nil
}

// The EQ wheel's step-resolution rung must survive across sessions like the
// synth/sampler wheels: a persisted "eq_band_gain" rung is restored into
// eqWheelStepBadge lazily on first popup open (the ctor may run before
// SetKnobStepSink installs the global sink, so restoring there would miss).
func TestEQWheelPopup_RestoresPersistedStep(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	// eq_band_gain's ladder for [-12,12] dB is {0.02,0.05,0.1,0.2,0.5,1,2};
	// defaultStepIndex picks 0.05 (closest to 400 notches). 1.0 dB is an exact
	// ladder rung far from the default, so restoring to it is unambiguous.
	defaultStep := dv.eqWheelStepBadge.Step()
	const persisted = 1.0
	if defaultStep == persisted {
		t.Fatalf("test fixture invalid: default step %v already equals persisted %v", defaultStep, persisted)
	}

	sink := &stubKnobStepSink{steps: map[string]float64{"eq_band_gain": persisted}}
	SetKnobStepSink(sink)
	t.Cleanup(func() { SetKnobStepSink(nil) })

	if dv.eqWheelStepRestored {
		t.Fatal("eqWheelStepRestored should start false on a fresh DrumView")
	}

	dv.openEQKnobWheelPopup(1)

	if !dv.eqWheelStepRestored {
		t.Fatal("eqWheelStepRestored should be true after the first popup open")
	}
	if got := dv.eqWheelStepBadge.Step(); got != persisted {
		t.Fatalf("eqWheelStepBadge.Step() = %v after restore, want persisted %v (default was %v)", got, persisted, defaultStep)
	}

	// A second open must NOT re-read the sink (the restore is one-shot): drop
	// the persisted value out from under it and confirm the badge doesn't revert.
	sink.steps["eq_band_gain"] = 0.02
	dv.openEQKnobWheelPopup(2)
	if got := dv.eqWheelStepBadge.Step(); got != persisted {
		t.Fatalf("eqWheelStepBadge.Step() = %v after second open, want restore to stay one-shot at %v", got, persisted)
	}
}
