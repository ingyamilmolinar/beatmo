//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Every data tab in the audio panel must use the height it was given. Wave
// already did (adaptive gain); Chain owned the same machinery but defaulted
// it OFF, and the two dB tabs had no adaptive scaling at all. These tests pin
// "the signal fills the plot" as a property of each tab, measured off real
// draw calls rather than off the control that used to enable it.

// TestChainAutoGainIsOnByDefault — Chain's auto-gain existed but its zero
// value left it off, so a quiet tap rendered as a thin line across the middle
// of the panel until the user found a pill labelled "AG". Framing the signal
// is not an opt-in.
func TestChainAutoGainIsOnByDefault(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.AutoGain() {
		t.Error("a fresh Chain panel has auto-gain off; the trace renders tiny until the user finds a pill")
	}
	if !z.AutoFit() {
		t.Error("a fresh Chain panel has auto-fit off; the trace occupies a sliver of the time axis")
	}
}

// chainStateAtPeak builds a one-tap scope.State carrying a decaying sine
// whose loudest sample is exactly `peak`.
func chainStateAtPeak(peak float64) *scope.State {
	samples := make([]float64, 512)
	for i := range samples {
		samples[i] = peak * math.Exp(-float64(i)/160) * math.Sin(2*math.Pi*float64(i)/40)
	}
	// Guarantee the stated peak survives the envelope.
	samples[10] = peak
	st := &scope.State{}
	st.TapA.Active = true
	st.TapA.Samples = samples
	return st
}

// chainRenderedFill renders the zone's traces and returns the VERTICAL EXTENT
// the trace covers as a fraction of the plot height. A line trace paints one
// 1-px rect per column, so the tallest single rect says nothing; the union of
// the columns is what the eye reads as "how much of the viewport the signal
// occupies".
func chainRenderedFill(t *testing.T, z *ChainPanelZone, state *scope.State) float64 {
	t.Helper()
	rect := image.Rect(0, 0, 600, 200)
	waveRect, _ := chainTraceRects(rect)
	if waveRect.Dy() <= 0 {
		t.Fatal("empty wave rect")
	}
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	want := color.RGBAModel.Convert(colScopeA).(color.RGBA)

	top, bot := math.MaxInt32, -1
	for _, r := range collectFilledRects(t, func() {
		z.renderTracesDirect(dst, rect, state)
	}) {
		// The legend swatches below the plot carry the SAME colour; counting
		// them would inflate the extent to ~60% no matter what the trace does.
		if r.Color != want || !r.Rect.In(waveRect) {
			continue
		}
		if r.Rect.Min.Y < top {
			top = r.Rect.Min.Y
		}
		if r.Rect.Max.Y > bot {
			bot = r.Rect.Max.Y
		}
	}
	if bot < 0 {
		return 0
	}
	return float64(bot-top) / float64(waveRect.Dy())
}

// TestChainQuietTraceFillsThePlot — a -20 dBFS transient (a perfectly normal
// level for one drum voice) must render at a readable height instead of at a
// tenth of the plot because 1.0 is hardcoded as the full-scale reference.
// The auto-off leg is what proves the measurement discriminates: it is the
// behaviour that shipped, and it must score badly.
func TestChainQuietTraceFillsThePlot(t *testing.T) {
	const quiet = 0.1 // -20 dBFS
	z := NewChainPanelZone(ChainCallbacks{})

	z.SetAutoGain(false)
	unscaled := chainRenderedFill(t, z, chainStateAtPeak(quiet))
	if unscaled > 0.3 {
		t.Fatalf("unscaled -20 dBFS trace already fills %.0f%% — the metric cannot tell the two apart", unscaled*100)
	}

	z.SetAutoGain(true)
	if got := chainRenderedFill(t, z, chainStateAtPeak(quiet)); got < 0.7 {
		t.Errorf("a -20 dBFS trace fills %.0f%% of the plot (unscaled: %.0f%%), want >= 70%%",
			got*100, unscaled*100)
	}
}

// spectrumChannelAtDB builds a channel whose every ISO band sits at `db`.
func spectrumChannelAtDB(db float64) *analyzer.ChannelMetrics {
	const n = 256
	fft := make([]float64, n)
	freq := make([]float64, n)
	for i := range fft {
		fft[i] = db
		freq[i] = 20 + float64(i)*(20000-20)/float64(n-1)
	}
	return &analyzer.ChannelMetrics{Active: true, FFTBins: fft, FreqBins: freq}
}

// spectrumRenderedFill renders the bars under the given dB window and returns
// the tallest bar as a fraction of the plot height. Measured off the real bar
// rects via the drawSpectrumBarGradient seam, so it cannot drift from what
// the renderer actually paints.
func spectrumRenderedFill(t *testing.T, ch *analyzer.ChannelMetrics, win spectrumDBWindow) float64 {
	t.Helper()
	rect := image.Rect(0, 0, 600, 200)
	barRect, _, _ := spectrumPanelRows(rect)
	if barRect.Dy() <= 0 {
		t.Fatal("empty bar rect")
	}
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	tallest := 0
	orig := drawSpectrumBarGradient
	drawSpectrumBarGradient = func(d *ebiten.Image, bar image.Rectangle) {
		if bar.Dy() > tallest {
			tallest = bar.Dy()
		}
		orig(d, bar)
	}
	t.Cleanup(func() { drawSpectrumBarGradient = orig })

	drawAnalyzerSpectrumImpl(dst, rect, ch, nil, freqScaleLog, spectrumChromeOpts{dbWin: win})
	return float64(tallest) / float64(barRect.Dy())
}

// TestSpectrumBarsFillThePlot — the fixed -80..0 dB window left the top third
// of the plot permanently empty for normal material. The adaptive window must
// bring the loudest band up near the ceiling. The fixed leg is the shipped
// behaviour and proves the measurement discriminates.
func TestSpectrumBarsFillThePlot(t *testing.T) {
	ch := spectrumChannelAtDB(-28)

	fixed := spectrumRenderedFill(t, ch, spectrumDBWindow{spectrumMinDB, spectrumMaxDB})
	if fixed > 0.75 {
		t.Fatalf("the fixed window already fills %.0f%% — the metric cannot tell the two apart", fixed*100)
	}

	floor, ceil := audioDBWindow(-28)
	if got := spectrumRenderedFill(t, ch, spectrumDBWindow{floor, ceil}); got < 0.85 {
		t.Errorf("a -28 dB band fills %.0f%% of the plot (fixed window: %.0f%%), want >= 85%%",
			got*100, fixed*100)
	}
}

// TestSpectrumCurveSharesTheBarWindow — the hi-res FFT curve is drawn UNDER
// the bars as the same measurement at finer resolution. If it keeps the fixed
// window while the bars adapt, the underlay detaches from the bars it is
// supposed to explain.
func TestSpectrumCurveSharesTheBarWindow(t *testing.T) {
	ch := spectrumChannelAtDB(-28)
	rect := image.Rect(0, 0, 600, 200)
	barRect, _, _ := spectrumPanelRows(rect)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	curveTop := func(win spectrumDBWindow) int {
		top := barRect.Max.Y
		for _, r := range collectFilledRects(t, func() {
			drawSpectrumCurve(dst, barRect, ch, freqScaleLog, win)
		}) {
			if r.Rect.Min.Y < top && r.Rect.In(barRect) {
				top = r.Rect.Min.Y
			}
		}
		return top
	}

	floor, ceil := audioDBWindow(-28)
	adaptive := curveTop(spectrumDBWindow{floor, ceil})
	fixed := curveTop(spectrumDBWindow{spectrumMinDB, spectrumMaxDB})

	if adaptive >= fixed {
		t.Errorf("curve top under the adaptive window (y=%d) is no higher than under the fixed one (y=%d)",
			adaptive, fixed)
	}
	if frac := float64(barRect.Max.Y-adaptive) / float64(barRect.Dy()); frac < 0.85 {
		t.Errorf("curve reaches %.0f%% of the plot, want >= 85%% (it must track the bars)", frac*100)
	}
}

// The window's per-refresh behaviour (rises for a louder peak, holds
// otherwise, never advances twice on the same analyzer state) is pinned in
// audio_tab_y_axis_contract_test.go, which owns the whole Y-axis contract
// across all four tabs. It used to live here under a name that claimed
// smoothing the dB axis no longer does.

// TestEQBandLevelsUseTheSharedDBWindow — the EQ backdrop bars were the worst
// of the three: not a dB mapping at all, but sqrt() of the raw linear FFT
// magnitude. A -28 dBFS band (linear 0.04) therefore drew at sqrt(0.04) = 20%
// of the plot no matter how the tab was sized. They must now read off the
// same dB window Spectrum uses, so a band of a given loudness is the same
// height on both tabs.
func TestEQBandLevelsUseTheSharedDBWindow(t *testing.T) {
	const linear = 0.04 // -28 dBFS
	spec := make([]float64, 256)
	for i := range spec {
		spec[i] = linear
	}
	floor, ceil := audioDBWindow(-28)
	win := spectrumDBWindow{floor, ceil}

	// The smoother converges over successive frames; run it to steady state.
	levels := make([]float64, len(eqBandDefs))
	for i := 0; i < 60; i++ {
		levels = eqBandLevels(spec, levels, win)
	}

	peak := 0.0
	for _, v := range levels {
		if v > peak {
			peak = v
		}
	}
	if peak < 0.85 {
		t.Errorf("a -28 dBFS band settles at %.0f%% of the plot, want >= 85%% (sqrt-of-linear gave 20%%)", peak*100)
	}
}

// TestEQBandLevelsDecayToSilence — with no spectrum at all the bars must fall
// away rather than freeze at their last height.
func TestEQBandLevelsDecayToSilence(t *testing.T) {
	levels := make([]float64, len(eqBandDefs))
	for i := range levels {
		levels[i] = 1.0
	}
	for i := 0; i < 60; i++ {
		levels = eqBandLevels(nil, levels, spectrumDBWindow{})
	}
	for i, v := range levels {
		if v > 0.01 {
			t.Errorf("band %d held at %.3f with no data, want it to decay to ~0", i, v)
		}
	}
}

// TestChainPaintsNoTriggerIndicator — the Chain tab used to paint a
// full-height vertical bar with a head tick at 10% of the trace width. It was
// mistaken for stray grid content invading the panel, and it told the user
// nothing they could act on, so it is gone for good. Guarded by SHAPE rather
// than by colour: the token it used is deleted too, and a reinstated marker
// in any colour would still be a narrow full-height mark at that position.
func TestChainPaintsNoTriggerIndicator(t *testing.T) {
	rect := image.Rect(0, 0, 600, 200)
	waveRect, _ := chainTraceRects(rect)
	trigX := waveRect.Min.X + waveRect.Dx()/10

	for _, tc := range []struct {
		name  string
		state *scope.State
	}{
		{"empty plot", nil},
		{"live trace", chainStateAtPeak(0.8)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z := NewChainPanelZone(ChainCallbacks{})
			dst := ebiten.NewImage(rect.Dx(), rect.Dy())
			for _, r := range collectFilledRects(t, func() {
				z.renderTracesDirect(dst, rect, tc.state)
			}) {
				narrow := r.Rect.Dx() <= 8
				tall := r.Rect.Dy() >= (waveRect.Dy()*9)/10
				atTrigger := r.Rect.Min.X <= trigX+8 && r.Rect.Max.X >= trigX-8
				if narrow && tall && atTrigger {
					t.Errorf("a trigger-marker-shaped rect %v is being painted at the 10%% mark", r.Rect)
				}
			}
		})
	}
}
