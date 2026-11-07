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

// Every data tab uses the height it was given, measured off real draw calls.

// Framing the signal is not opt-in: off, a quiet tap was a thin line across the middle.
func TestChainAutoGainIsOnByDefault(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.AutoGain() {
		t.Error("a fresh Chain panel has auto-gain off; the trace renders tiny until the user finds a pill")
	}
	if !z.AutoFit() {
		t.Error("a fresh Chain panel has auto-fit off; the trace occupies a sliver of the time axis")
	}
}

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

// The union of the 1 px column rects is what the eye reads as the signal's extent.
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
		// The legend swatches share the colour and would inflate the extent to ~60%.
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

// A -20 dBFS transient, normal for one drum, renders at a readable height; the auto-off leg must score badly,
// proving the measurement discriminates.
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

// Through the drawSpectrumBarGradient seam, so it cannot drift from what is painted.
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

// The loudest band reaches near the ceiling; the fixed leg proves the measurement discriminates.
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

// The curve is the same measurement at finer resolution, so it must share the bars' window.
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

// The same dB window as Spectrum, so a band of a given loudness is the same height on both tabs; a sqrt of the
// linear magnitude drew -28 dBFS at 20% whatever the size.
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

// With no spectrum they fall away rather than freeze.
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

// The full-height marker read as stray grid and told the user nothing; guarded by shape, so a reinstated one in
// any colour fails.
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
