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

// Selecting a single instrument's channel showed an EMPTY plot most of the
// time. Measured on the shipped circuit with the Dnb Kick channel selected:
// 88 of 148 frames rendered no bar at all.
//
// Three separate causes, one test each below. They are separate because each
// one alone still leaves the tab unusable.

// --- 1. the analyser's silence sentinel ------------------------------------

// TestSpectrumIgnoresTheSilenceSentinel — on a silent frame the analyser does
// not report the reference floor, it reports -Inf (and -math.MaxFloat64 once
// averaged). Those values flow straight into the axis anchor and the peak
// holds, where they are not merely quiet but meaningless.
func TestSpectrumIgnoresTheSilenceSentinel(t *testing.T) {
	silent := &analyzer.ChannelMetrics{Active: true}
	for i := 0; i < 64; i++ {
		silent.FFTBins = append(silent.FFTBins, math.Inf(-1))
		silent.FreqBins = append(silent.FreqBins, 20+float64(i)*300)
	}
	if got := spectrumPeakDB(silent); got != spectrumMinDB {
		t.Errorf("spectrumPeakDB on a silent channel = %v, want the reference floor %v", got, spectrumMinDB)
	}

	// Mixed: one real band, the rest silent. The real band must win and the
	// sentinel must not drag anything below the floor.
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

// TestSpectrumBarsDecaySmoothlyBetweenHits — a per-instrument channel is
// SILENT between transients: a kick at 120 bpm sounds for a few frames and
// nothing for the next thirty. The peak-hold bridged that gap with a
// frame-counted cliff — it held the bar flat, then dropped it to zero in one
// frame — so the tab blinked at the tempo of the music and read as broken.
// A held bar must fall the way an analyser's does: smoothly, in dB per
// second, so there is always something on screen to read.
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
	// After a second of silence the bar must still be readable: the next hit
	// of a 120 bpm kick has not even arrived yet.
	if prev <= 0 {
		t.Errorf("the bar reached zero within a second of silence; the plot is empty between hits")
	}
}

// --- 3. the axis for a single instrument -----------------------------------

// TestPerInstrumentAxisComesFromThePrerenderedBuffer — the live analyser
// cannot set the scale for one instrument's channel: it is silent most of the
// time, so the anchor is whatever the last transient happened to be, and
// right after a switch there may have been none at all. The instrument's
// pre-rendered one-shot is the honest source — it is what the channel WILL
// sound like — and it is available the instant the channel is picked.
func TestPerInstrumentAxisComesFromThePrerenderedBuffer(t *testing.T) {
	const inst = roster.Kick
	want, ok := prerenderedChannelWindow(channel.Of(inst))
	if !ok {
		t.Fatal("no pre-rendered window for a builtin instrument")
	}

	z := NewEQPanelZone(EQCallbacks{})
	z.SetActiveChannel(channel.Of(inst))

	// The very first frame after the switch, with the channel still silent,
	// must already carry the buffer-derived scale.
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

// TestMasterChannelKeepsTheLiveAnchor — the master bus is a mix with no
// single pre-rendered buffer, so it stays on the live max-anchored axis.
func TestMasterChannelKeepsTheLiveAnchor(t *testing.T) {
	if _, ok := prerenderedChannelWindow(channel.Master); ok {
		t.Error("master resolved to a pre-rendered window; it is a mix, not one instrument")
	}
}

// TestChannelAxisSettlesOnTheLiveLevelThenSticks — the pre-rendered one-shot
// is the right thing to show IMMEDIATELY (it needs no hit to have happened)
// but it is the raw voice at unity, while the analyser measures the channel
// after row volume, per-voice headroom and EQ. Measured on the shipped
// circuit: pinning the buffer's peak left the tallest bar at 37% of the plot
// for the whole session.
//
// So the buffer is the estimate the user sees while the channel settles, and
// within about a second the axis is re-pinned on what the channel actually
// measures — once. After that it does not move until the next discontinuity.
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

	// Live frames at -35 dB across the settle window (spectrumChannelSettle
	// is 1200ms; 60 frames at 25ms carries us past it).
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

// TestChannelAxisWaitsForRealSignalBeforeFreezing — the settle re-pins the
// axis on what the channel MEASURES, but a settle window can contain nothing
// to measure: pick the channel of a muted row, or of a row whose next hit is
// a bar away, and the window elapses on silence. Freezing then locks in the
// pre-rendered ESTIMATE, which runs high (raw voice at unity), and every bar
// afterwards is a stub against a ceiling the channel never reaches — for the
// rest of the session, because a frozen axis never re-evaluates.
//
// The axis may only commit once it has something to commit to.
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

// TestPeakHoldDecaysEveryBandNotJustTheFirst — observe is called once per
// band per frame and shares one clock. Stamping "last observed" on the first
// call left every LATER band with dt=0, so bands 1-9 never fell: they froze
// at their loudest reading for the session. Visible as permanent stubs in
// bands a kick cannot reach.
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

// TestChannelChangeEnablesThatChannelsAnalyzer — THE browser bug. On WASM a
// per-instrument channel has no analyser until something calls
// audio.EnableChannelAnalyzer for it: channelAnalyzerSnapshot(id) reads
// chain.analyser in audio.js and returns zeros when that node does not
// exist. EnsureAnalyzersForTab does enable it, but it is wired to TAB
// transitions only — so picking a channel while staying on the Spectrum tab
// left that channel with no analyser at all, and the plot was empty in the
// browser no matter what the axis did. (Desktop hid it: the analyzer service
// wires the detail tap through SetDetailChannel, which has no WASM twin.)
//
// Enabling is idempotent and cheap on every platform, so the channel-change
// chokepoint is the right place for it.
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

	// A channel pick with NO tab transition — the case the tab-keyed
	// dispatcher never sees.
	g.drum.selectAudioChannel(channel.Of(inst))

	if n := audio.ChannelAnalyzerEnableCount(channel.Of(inst)); n == 0 {
		t.Errorf("no analyser was enabled for %q after selecting its channel; "+
			"in the browser that channel reports zeros and the Spectrum plot is empty", inst)
	}
}
