//go:build test

package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// One instrument's channel showed an empty plot most of the time, from three independent causes, one test each.

// --- 1. the analyser's silence sentinel ------------------------------------

// A silent frame reports -Inf (-MaxFloat64 once averaged), which must not reach the axis anchor or peak holds.
func TestSpectrumIgnoresTheSilenceSentinel(t *testing.T) {
	silent := &analyzer.ChannelMetrics{Active: true}
	for i := 0; i < 64; i++ {
		silent.FFTBins = append(silent.FFTBins, math.Inf(-1))
		silent.FreqBins = append(silent.FreqBins, 20+float64(i)*300)
	}
	if got := spectrumPeakDB(silent); got != spectrumMinDB {
		t.Errorf("spectrumPeakDB on a silent channel = %v, want the reference floor %v", got, spectrumMinDB)
	}

	// The real band wins and nothing drops below the floor.
	mixed := &analyzer.ChannelMetrics{Active: true}
	for i := 0; i < 64; i++ {
		db := math.Inf(-1)
		if i == 3 {
			db = -22
		}
		mixed.FFTBins = append(mixed.FFTBins, db)
		mixed.FreqBins = append(mixed.FreqBins, 20+float64(i)*300)
	}
	if got := spectrumPeakDB(mixed); got != -22 {
		t.Errorf("spectrumPeakDB with one live band = %v, want -22", got)
	}
}

// --- 2. bars that blink between hits ---------------------------------------

// A per-instrument channel is silent between hits; a held bar falls smoothly in dB/s rather than off a cliff,
// or the tab blinks at the tempo.
func TestSpectrumBarsDecaySmoothlyBetweenHits(t *testing.T) {
	var st SpectrumPeakState
	win := spectrumDBWindow{Floor: -66, Ceil: -24}
	now := time.Now()

	// One hit.
	st.observe(0, -30, win, now)
	if got := st.holdNorm(0, win); got < 0.8 {
		t.Fatalf("the hit itself reads %.2f, want it near the top", got)
	}

	// Then silence, one frame at a time for a full second.
	prev := st.holdNorm(0, win)
	for f := 1; f <= 60; f++ {
		now = now.Add(16 * time.Millisecond)
		st.observe(0, spectrumMinDB, win, now)
		cur := st.holdNorm(0, win)
		if cur > prev+0.001 {
			t.Fatalf("frame %d: the hold ROSE during silence (%.3f -> %.3f)", f, prev, cur)
		}
		if drop := prev - cur; drop > 0.2 {
			t.Fatalf("frame %d: the hold fell %.2f in a single frame — that is a cliff, not a decay", f, drop)
		}
		prev = cur
	}
	// Still readable after a second, before a 120 bpm kick's next hit.
	if prev <= 0 {
		t.Errorf("the bar reached zero within a second of silence; the plot is empty between hits")
	}
}

// --- 3. the axis for a single instrument -----------------------------------

// One instrument's channel is mostly silent, so its scale comes from the pre-rendered one-shot, available the
// instant it is picked.
func TestPerInstrumentAxisComesFromThePrerenderedBuffer(t *testing.T) {
	const inst = roster.Kick
	want, ok := prerenderedChannelWindow(channel.Of(inst))
	if !ok {
		t.Fatal("no pre-rendered window for a builtin instrument")
	}

	z := NewEQPanelZone(EQCallbacks{})
	z.SetActiveChannel(channel.Of(inst))

	// On the first silent frame after the switch.
	got := z.resolveSpectrumWindow(&analyzer.State{}, &analyzer.ChannelMetrics{})
	if got != want {
		t.Errorf("first frame on %q uses %v, want the buffer-derived %v", inst, got, want)
	}

	// ...and it must STICK: silence, then a quiet frame, must not move it.
	for i := 0; i < 50; i++ {
		got = z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-55))
	}
	if got != want {
		t.Errorf("axis drifted to %v over 50 frames, want it pinned at %v", got, want)
	}
}

// A mix has no single buffer.
func TestMasterChannelKeepsTheLiveAnchor(t *testing.T) {
	if _, ok := prerenderedChannelWindow(channel.Master); ok {
		t.Error("master resolved to a pre-rendered window; it is a mix, not one instrument")
	}
}

// The one-shot is the raw voice at unity and runs high, so it is shown at once and the axis re-pins once on the
// measured level about a second later, then holds until the next discontinuity.
func TestChannelAxisSettlesOnTheLiveLevelThenSticks(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.SetActiveChannel(channel.Of(roster.Kick))

	start := time.Now()
	z.nowFn = func() time.Time { return start }

	// Immediately: the buffer estimate, so there is never an empty plot.
	est, _ := prerenderedChannelWindow(channel.Of(roster.Kick))
	if got := z.resolveSpectrumWindow(&analyzer.State{}, &analyzer.ChannelMetrics{}); got != est {
		t.Errorf("first frame = %v, want the buffer estimate %v", got, est)
	}

	// 60 frames of 25 ms pass the 1200 ms settle.
	for i := 1; i <= 60; i++ {
		z.nowFn = func() time.Time { return start.Add(time.Duration(i) * 25 * time.Millisecond) }
		z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-35))
	}

	settled := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-35))
	if n := audioDBNorm(-35, settled.Floor, settled.Ceil); n < 0.8 {
		t.Errorf("after settling, a -35 dB band fills %.0f%% of the plot, want >= 80%%", n*100)
	}

	// ...and then it STICKS: neither silence nor a quieter frame moves it.
	for i := 61; i <= 220; i++ {
		z.nowFn = func() time.Time { return start.Add(time.Duration(i) * 25 * time.Millisecond) }
		if got := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-70)); got != settled {
			t.Fatalf("frame %d: axis moved to %v after settling at %v", i, got, settled)
		}
	}
}

// A settle window of pure silence (a muted row, a hit a bar away) must not freeze the high estimate: a frozen
// axis never re-evaluates. It commits only once there is signal.
func TestChannelAxisWaitsForRealSignalBeforeFreezing(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.SetActiveChannel(channel.Of(roster.Kick))

	start := time.Now()
	at := func(ms int) { z.nowFn = func() time.Time { return start.Add(time.Duration(ms) * time.Millisecond) } }

	// The channel is silent for well past the settle window (a muted row).
	for ms := 0; ms <= 3000; ms += 25 {
		at(ms)
		z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(spectrumMinDB))
	}

	// Now the row is audible and the channel measures -35 dB.
	for ms := 3025; ms <= 6000; ms += 25 {
		at(ms)
		z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-35))
	}
	at(6025)
	got := z.resolveSpectrumWindow(&analyzer.State{}, spectrumChannelAtDB(-35))

	if n := audioDBNorm(-35, got.Floor, got.Ceil); n < 0.8 {
		t.Errorf("after silence-then-signal, a -35 dB band fills %.0f%% of the plot (axis %v); "+
			"the axis froze on the estimate while the channel had nothing to measure", n*100, got)
	}
}

// observe runs once per band per frame on one clock; stamping on the first call froze every later band at its
// loudest.
func TestPeakHoldDecaysEveryBandNotJustTheFirst(t *testing.T) {
	var st SpectrumPeakState
	win := spectrumDBWindow{Floor: -66, Ceil: -24}
	now := time.Now()

	// One loud frame across every band.
	for b := 0; b < 10; b++ {
		st.observe(b, -30, win, now)
	}
	// Then a second of silence, all bands each frame, as the renderer does.
	for f := 1; f <= 60; f++ {
		now = now.Add(16 * time.Millisecond)
		for b := 0; b < 10; b++ {
			st.observe(b, spectrumMinDB, win, now)
		}
	}
	for b := 0; b < 10; b++ {
		if got := st.heldDB(b); got > -45 {
			t.Errorf("band %d held at %.1f dB after a second of silence; it is not decaying", b, got)
		}
	}
}

// On WASM a channel has no analyser until enabled, and tab transitions were the only enabler, so picking a channel
// on the Spectrum tab drew nothing. Enabling is idempotent, so the channel-change chokepoint does it.
func TestChannelChangeEnablesThatChannelsAnalyzer(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.drum.Rows) == 0 {
		t.Fatal("no rows")
	}
	inst := g.drum.Rows[0].Instrument

	_ = g.SetActiveEQTab("spectrum")
	audio.ResetAnalyserEnableCounts()

	// No tab transition: the case the tab dispatcher never sees.
	g.drum.selectAudioChannel(channel.Of(inst))

	if n := audio.ChannelAnalyzerEnableCount(channel.Of(inst)); n == 0 {
		t.Errorf("no analyser was enabled for %q after selecting its channel; "+
			"in the browser that channel reports zeros and the Spectrum plot is empty", inst)
	}
}
