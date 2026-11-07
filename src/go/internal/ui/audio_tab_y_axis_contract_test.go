//go:build test

package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Two Y-axis families that are not interchangeable. Amplitude (Wave, Chain) normalises to the signal and tracks
// it. dB (Spectrum, EQ backdrop) carries an absolute scale and STAYS PUT: a band rises because it got louder,
// never because the axis slid under it.

// --- dB family: the axis must be stable -------------------------------------

// A decaying peak-hold slid the ceiling between drum hits, so unchanged bands appeared to jump.
func TestSpectrumWindowDoesNotMoveWhenTheSignalDrops(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})

	loud := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-18))

	// Many refreshes of a much quieter signal — the gap between transients.
	var quiet spectrumDBWindow
	for i := 0; i < 200; i++ {
		quiet = z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-60))
	}
	if quiet != loud {
		t.Errorf("window slid from %v to %v while the signal was merely quieter — the bars jump", loud, quiet)
	}

	// Silence must not move it either.
	silent := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-100))
	if silent != loud {
		t.Errorf("window moved to %v during silence, want it pinned at %v", silent, loud)
	}
}

// Stable is not frozen: a new peak must be representable, or the bars clip.
func TestSpectrumWindowRisesForALouderSignal(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})

	quiet := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-40))
	loud := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-6))

	if loud.Ceil <= quiet.Ceil {
		t.Errorf("ceiling did not rise for a louder peak: %.1f -> %.1f", quiet.Ceil, loud.Ceil)
	}
	if n := audioDBNorm(-6, loud.Floor, loud.Ceil); n >= 1.0 {
		t.Errorf("the new peak normalises to %.3f — it clips against its own ceiling", n)
	}
}

// Once settled, a level maps to the same height every frame, so readings a second apart compare.
func TestSpectrumWindowIsAbsoluteAcrossFrames(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	_ = z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-18))

	var heights []float64
	for i := 0; i < 50; i++ {
		// A constant -30 dB band while the overall peak wobbles as real material does.
		peak := -24.0
		if i%2 == 0 {
			peak = -50.0
		}
		win := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(peak))
		heights = append(heights, audioDBNorm(-30, win.Floor, win.Ceil))
	}
	for i, h := range heights {
		if h != heights[0] {
			t.Fatalf("frame %d: a constant -30 dB band drew at %.3f, but frame 0 drew it at %.3f", i, h, heights[0])
		}
	}
}

// A quieter frame does not move it.
func TestEQBackdropSharesTheSpectrumAxisBehaviour(t *testing.T) {
	flatSpectrum := func(linear float64) []float64 {
		out := make([]float64, 64)
		for i := range out {
			out[i] = linear
		}
		return out
	}
	level := 0.12 // about -18 dBFS
	z := NewEQPanelZone(EQCallbacks{
		AnalyzerSnapshot: func(channel.ID) audio.AnalyzerSnapshot {
			return audio.AnalyzerSnapshot{Spectrum: flatSpectrum(level)}
		},
	})

	loud := z.resolveSpectrumWindowForEQ()

	level = 0.001 // about -60 dBFS
	var quiet spectrumDBWindow
	for i := 0; i < 200; i++ {
		quiet = z.resolveSpectrumWindowForEQ()
	}
	if quiet != loud {
		t.Errorf("EQ backdrop window slid from %v to %v on a quieter frame", loud, quiet)
	}
}

// --- amplitude family: tracking is the point --------------------------------

// With no absolute reference, an unscaled quiet signal is unreadable.
func TestWaveAndChainStillTrackTheSignal(t *testing.T) {
	// Wave: a quiet peak buys a large gain, a hot one buys none.
	if g := autoGainForPeak(0.05); g <= 4 {
		t.Errorf("Wave gain for a -26 dBFS peak = %.1f, want it scaled well up", g)
	}
	if g := autoGainForPeak(0.95); g != 1.0 {
		t.Errorf("Wave gain for a hot peak = %.1f, want 1.0 (never zoom out)", g)
	}
	// Chain: same model, on by default.
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.AutoGain() {
		t.Error("Chain auto-gain is off by default; a quiet tap renders as a flat line")
	}
}

// A single instrument's scale comes from its pre-rendered one-shot; the previous channel's is never reused.
func TestSpectrumAxisRebaselinesOnChannelChange(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	// Anchor master on a live reading first.
	z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-6))

	z.SetActiveChannel(channel.Of(roster.Kick))
	got := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-40))

	want, ok := prerenderedChannelWindow(channel.Of(roster.Kick))
	if !ok {
		t.Fatal("no pre-rendered window for the instrument")
	}
	if got != want {
		t.Errorf("after switching channel the axis is %v, want the new channel's own %v", got, want)
	}
	if z.pinnedWindowFor != channel.Of(roster.Kick) {
		t.Errorf("pinned window still attributed to %q", z.pinnedWindowFor)
	}
}

// Programmatic switches (mute auto-focus, rename, import, bridge) must re-derive the axis like the dropdown,
// not keep measuring the previous channel.
func TestEveryChannelSwitchRebaselinesTheAxis(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone
	if z == nil || len(g.drum.Rows) == 0 {
		t.Fatal("no EQ panel zone / rows")
	}
	inst := g.drum.Rows[0].Instrument

	// Anchor master on a loud live reading.
	z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-6))

	// Programmatic switch — the path a mute's auto-focus takes.
	g.drum.selectAudioChannel(channel.Of(inst))
	got := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-45))

	want, ok := prerenderedChannelWindow(channel.Of(inst))
	if !ok {
		t.Fatalf("no pre-rendered window for %q", inst)
	}
	if got != want {
		t.Errorf("after a programmatic switch the axis is %v, want %q's own %v", got, inst, want)
	}
}

// Every assignment goes through setActiveChannelID, the one writer that re-baselines.
func TestZoneChannelHasOneWriter(t *testing.T) {
	writers := grepCount(t, "z.activeChannel = ", "eq_panel_zone.go") +
		grepCount(t, ".activeChannel = ", "drumview_audio_eq.go")
	if writers != 1 {
		t.Errorf("found %d direct writes to the zone's activeChannel, want exactly 1 (inside setActiveChannelID)", writers)
	}
}

// Source-level: a second writer compiles and passes every behavioural test while leaving one path stale.
func grepCount(t *testing.T, needle, file string) int {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return strings.Count(string(b), needle)
}

// Stored in dB and normalised at draw: a stored normalised height is re-read against a window it was never
// measured on once the axis moves.
func TestPeakHoldSurvivesAnAxisMove(t *testing.T) {
	var st SpectrumPeakState

	// Observe a -30 dB band under one window...
	loud := spectrumDBWindow{Floor: -66, Ceil: -24}
	st.observe(0, -30, loud, time.Now())
	before := st.holdNorm(0, loud)

	// The same -30 dB mark reads HIGHER once the ceiling comes down.
	quiet := spectrumDBWindow{Floor: -78, Ceil: -36}
	after := st.holdNorm(0, quiet)

	if after <= before {
		t.Errorf("a -30 dB hold read %.3f under a -66..-24 window and %.3f under -78..-36; "+
			"it must rise when the ceiling drops", before, after)
	}
	if want := audioDBNorm(-30, quiet.Floor, quiet.Ceil); after != want {
		t.Errorf("hold normalises to %.3f, want %.3f — it is not being re-read in the current window", after, want)
	}
}
