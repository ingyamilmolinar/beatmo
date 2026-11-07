//go:build test

package ui

import "testing"

// Layout, recalcButtons, the tab, Invalidate and the zone layout, in that order.
func setupEQTabForTest(g *Game, w, h int) {
	g.Layout(w, h)
	g.drum.recalcButtons()
	g.drum.eqPanelZone.tabState.SetActiveTab(TabEQ)
	g.drum.eqPanelZone.Invalidate()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

// On desktop and mobile, editing the curve's source of truth.
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

// The wheel and a curve drag write the same field.
func TestEQWheelPopup_SharesSourceOfTruthWithCurve(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(1)
	b := dv.eqWheelPopup.binding

	// The per-notch OnChange lands on bandGainsDB.
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

// A centre click confirms: commit once, close.
func TestEQWheelPopup_CenterClickConfirms(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(2)
	w := dv.eqWheelPopup
	commits := 0
	orig := w.binding.OnCommit
	w.binding.OnCommit = func() {
		commits++
		if orig != nil {
			orig()
		}
	}
	w.StepValue(+1)
	h := &mobileWheelHitHandler{popup: w}
	cx, cy := center(w.centerH)
	h.OnPress(cx, cy)
	h.OnRelease(cx, cy)
	if w.IsOpen() {
		t.Fatal("a centre click must close the wheel")
	}
	if commits != 1 {
		t.Fatalf("a centre click after an edit commits exactly once, got %d", commits)
	}
}

// Non-empty and inside the DrumView on the desktop profile.
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

// In memory, touching no disk or browser storage.
type stubKnobStepSink struct{ steps map[string]float64 }

func (s *stubKnobStepSink) LoadKnobSteps() map[string]float64 { return s.steps }
func (s *stubKnobStepSink) SaveKnobStep(name string, step float64) error {
	if s.steps == nil {
		s.steps = map[string]float64{}
	}
	s.steps[name] = step
	return nil
}

// Restored lazily on first open: the constructor may run before the global sink is installed.
func TestEQWheelPopup_RestoresPersistedStep(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	// Ladder {0.02,0.05,0.1,0.2,0.5,1,2}; the default is 0.05, so 1.0 restores unambiguously.
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

	// One-shot: a second open does not re-read the sink.
	sink.steps["eq_band_gain"] = 0.02
	dv.openEQKnobWheelPopup(2)
	if got := dv.eqWheelStepBadge.Step(); got != persisted {
		t.Fatalf("eqWheelStepBadge.Step() = %v after second open, want restore to stay one-shot at %v", got, persisted)
	}
}
