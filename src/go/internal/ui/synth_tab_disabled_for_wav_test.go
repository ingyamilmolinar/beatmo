//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synthTabIndexForTest returns the index of the Synth tab within AllPanelTabs.
func synthTabIndexForTest(t *testing.T) int {
	t.Helper()
	for i, tab := range AllPanelTabs() {
		if tab == TabSynth {
			return i
		}
	}
	t.Fatal("TabSynth not present in AllPanelTabs")
	return -1
}

// TestActiveInstrumentHasSynth verifies the per-row synth/WAV classifier that
// gates the Synth tab selector: a recipe-backed instrument has synth controls;
// a WAV sample (no bound recipe) does not.
func TestActiveInstrumentHasSynth(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 3)
	dv.eqPanelZone.SetActiveChannel("main") // resolve to first row instrument

	if !dv.activeInstrumentHasSynth() {
		t.Fatalf("demo first-row instrument %q should be a synth", dv.synthTabActiveInstrument())
	}

	// Turn row 0 into a WAV sample with no recipe binding.
	dv.Rows[0].Instrument = "wav-sample-x"
	audio.BindInstrumentToRecipe("wav-sample-x", "")
	t.Cleanup(func() { audio.ResetInstrumentParams("wav-sample-x") })

	if dv.activeInstrumentHasSynth() {
		t.Fatalf("WAV instrument must not be classified as a synth")
	}
}

// TestSynthTabPillDisabledForWavInstrument verifies the desktop Synth tab pill
// is greyed/disabled when the active instrument is a WAV sample, and enabled
// for a synth instrument.
func TestSynthTabPillDisabledForWavInstrument(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 3)
	idx := synthTabIndexForTest(t)

	// Select the concrete instrument (not Master, which independently disables
	// the pill) so this test isolates the WAV-vs-synth gating.
	dv.eqPanelZone.SetActiveChannel(dv.Rows[0].Instrument)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	if pill := dv.eqPanelZone.stickyBar.TabBtn(idx); pill == nil || pill.Disabled {
		t.Fatalf("synth pill should be enabled for a synth instrument")
	}

	dv.Rows[0].Instrument = "wav-sample-y"
	audio.BindInstrumentToRecipe("wav-sample-y", "")
	t.Cleanup(func() { audio.ResetInstrumentParams("wav-sample-y") })

	dv.eqPanelZone.SetActiveChannel("wav-sample-y")
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	if pill := dv.eqPanelZone.stickyBar.TabBtn(idx); pill == nil || !pill.Disabled {
		t.Fatalf("synth pill should be disabled for a WAV instrument")
	}
}

// TestSegmentedControlDisabledSegmentIgnoresHitTest verifies a disabled segment
// consumes the tap (so it does not fall through) but neither activates nor fires
// the click handler, while other segments still work.
func TestSegmentedControlDisabledSegmentIgnoresHitTest(t *testing.T) {
	var clicked []int
	sc := NewSegmentedControl([]string{"A", "B", "C"}, 0, func(i int) { clicked = append(clicked, i) })
	sc.SetRect(image.Rect(0, 0, 300, 40))
	sc.SetSegmentDisabled(1, true)

	// Tap the disabled middle segment.
	mid := sc.SegmentRect(1)
	consumed := sc.HitTest((mid.Min.X+mid.Max.X)/2, (mid.Min.Y+mid.Max.Y)/2)
	if !consumed {
		t.Fatalf("tap inside the control should be consumed even when the segment is disabled")
	}
	if len(clicked) != 0 {
		t.Fatalf("disabled segment must not fire onClick: %v", clicked)
	}
	if sc.Active() != 0 {
		t.Fatalf("disabled segment must not become active: got %d", sc.Active())
	}

	// An enabled segment still works.
	last := sc.SegmentRect(2)
	sc.HitTest((last.Min.X+last.Max.X)/2, (last.Min.Y+last.Max.Y)/2)
	if sc.Active() != 2 || len(clicked) != 1 || clicked[0] != 2 {
		t.Fatalf("enabled segment should activate + fire: active=%d clicked=%v", sc.Active(), clicked)
	}
}
