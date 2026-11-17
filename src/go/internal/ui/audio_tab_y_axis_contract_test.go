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

// The audio panel's four data tabs split into two Y-axis families, and they
// are NOT interchangeable. Getting this wrong is what made Spectrum worse:
// it was given the amplitude family's continuously-tracking behaviour, and a
// dB axis that moves under the bars reads as the bars jumping.
//
//	AMPLITUDE (Wave, Chain) — the plot is normalised to the signal. There is
//	no absolute reference on screen to contradict, the edge label states the
//	gain in force, and tracking is the point: a quiet tap must still be
//	legible. Continuous peak-hold with release is correct here.
//
//	dB (Spectrum, EQ backdrop) — the plot carries an ABSOLUTE scale with
//	labelled reference lines. The axis is chosen from the loudest thing seen
//	and then STAYS PUT. A band that grows must move up because it got louder,
//	never because the axis slid down underneath it.
//
// These tests pin that split so a future change has to break them on purpose.

// --- dB family: the axis must be stable -------------------------------------

// TestSpectrumWindowDoesNotMoveWhenTheSignalDrops is the regression this file
// exists for. A decaying peak-hold slid the ceiling down between drum hits
// and snapped it back up on the next one, so every band appeared to jump
// while its actual level was unchanged.
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

// TestSpectrumWindowRisesForALouderSignal — stable is not the same as frozen.
// A peak above everything seen so far has to be representable, or the bars
// clip against a ceiling that is too low.
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

// TestSpectrumWindowIsAbsoluteAcrossFrames — the same band level must map to
// the same bar height on every frame once the scale has settled. This is what
// "absolute" buys: two readings taken a second apart are comparable.
func TestSpectrumWindowIsAbsoluteAcrossFrames(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	_ = z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-18))

	var heights []float64
	for i := 0; i < 50; i++ {
		// A band at a CONSTANT -30 dB, observed while the overall peak
		// wobbles between -24 and -50 the way real material does.
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

// TestEQBackdropSharesTheSpectrumAxisBehaviour — the EQ backdrop reads the
// same dB window, so it inherits the same contract: a quieter frame must not
// move it.
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

// TestWaveAndChainStillTrackTheSignal — the amplitude tabs must NOT be
// "fixed" by the same reasoning. They carry no absolute reference, so a quiet
// signal that is not scaled up is simply unreadable.
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

// TestSpectrumAxisRebaselinesOnChannelChange — picking a channel re-derives
// the axis for THAT channel. For a single instrument the scale now comes
// from its pre-rendered one-shot rather than from whatever the live signal
// happened to be doing (see prerenderedChannelWindow); the property that
// survives is that the previous channel's scale is not reused.
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

// TestEveryChannelSwitchRebaselinesTheAxis — the dropdown path went through
// SetActiveChannel and re-derived the axis. Every PROGRAMMATIC switch went
// through DrumView.setEQActiveChannel, which wrote eqPanelZone.activeChannel
// directly — so the auto-focus that fires when a mute leaves one audible row,
// and every rename / import / bridge call, changed the channel while the
// panel kept measuring the previous one.
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

// TestZoneChannelHasOneWriter — the bug above existed because
// eqPanelZone.activeChannel had two writers and only one of them
// re-baselined. Any new assignment must go through setActiveChannelID.
func TestZoneChannelHasOneWriter(t *testing.T) {
	writers := grepCount(t, "z.activeChannel = ", "eq_panel_zone.go") +
		grepCount(t, ".activeChannel = ", "drumview_audio_eq.go")
	if writers != 1 {
		t.Errorf("found %d direct writes to the zone's activeChannel, want exactly 1 (inside setActiveChannelID)", writers)
	}
}

// grepCount counts occurrences of needle in a source file of this package.
// Source-level because the invariant IS about the source: a second writer
// compiles and passes every behavioural test while leaving one path stale.
func grepCount(t *testing.T, needle, file string) int {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return strings.Count(string(b), needle)
}

// TestPeakHoldSurvivesAnAxisMove — the peak-hold and the MAX watermark are
// stored so they can be drawn later, and "later" can be after the axis has
// moved. Storing them as NORMALISED heights bakes in the window they were
// recorded under: re-anchor the axis and every stored mark is re-read against
// a scale it was never measured on, which is why watermark ticks floated over
// bands holding nothing. They have to be stored in dB — an absolute unit —
// and normalised at draw time.
func TestPeakHoldSurvivesAnAxisMove(t *testing.T) {
	var st SpectrumPeakState

	// Observe a -30 dB band under one window...
	loud := spectrumDBWindow{Floor: -66, Ceil: -24}
	st.observe(0, -30, loud, time.Now())
	before := st.holdNorm(0, loud)

	// ...then re-anchor the axis on a quieter mix. The SAME -30 dB mark must
	// now read HIGHER on the plot, because the plot's ceiling came down.
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
