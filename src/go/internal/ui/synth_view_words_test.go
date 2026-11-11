package ui

import (
	"image"
	"math"
	"math/rand"
	"strings"
	"testing"
	"unicode"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// The graph's two readouts (spec 2026-10-25 §3.3): WORDS by default — a plain
// summary a non-musician reads at a glance — and NUMBERS on request (the
// "Details" toggle) — the literal measurements.

// renderFrom wraps a synthetic note as an analysed render.
func renderFrom(name string, pcm []float32, voice float64) *graphRender {
	r := &graphRender{ident: graphIdent{hash: name}, sr: testViewSR, voiceSec: voice, an: soundview.New(pcm, testViewSR, voice)}
	r.an.Warm(soundview.NeedAll, nil)
	return r
}

func genNote(sec float64, f func(t float64) float64) []float32 {
	out := make([]float32, int(sec*testViewSR))
	for i := range out {
		out[i] = float32(f(float64(i) / testViewSR))
	}
	return out
}

// No digit, no unit, no note name: words mode is for people who do not read
// Hz, dB or cents.
func TestWordReadoutsHaveNoNumbersOrUnits(t *testing.T) {
	now := synthRender("now", 220, 0.2, 0.2, 2)
	before := synthRender("before", 330, 0.6, 0.0, 1)
	for _, v := range allPlotViews {
		for _, in := range []synthViewInput{{now: now}, {now: now, before: before}} {
			vp := buildSynthViewPlot(v, 420, 170, in)
			if strings.TrimSpace(vp.summary) == "" {
				t.Errorf("%v: no words readout", v)
				continue
			}
			for _, r := range vp.summary {
				if unicode.IsDigit(r) {
					t.Errorf("%v: the words readout %q has a number in it", v, vp.summary)
					break
				}
			}
			for _, unit := range []string{"Hz", "dB", "ms", "cents", "%", "×"} {
				if strings.Contains(vp.summary, unit) {
					t.Errorf("%v: the words readout %q says %q", v, vp.summary, unit)
				}
			}
			if strings.TrimSpace(vp.readout) == "" {
				t.Errorf("%v: no numbers readout", v)
			}
		}
	}
}

// "(was …)" names only the number that moved, never the whole sentence again
// (the pitch readout used to repeat itself and get cut off).
func TestWasCarriesOnlyTheKeyNumber(t *testing.T) {
	now := synthRender("now", 55, 0.4, 0, 3)
	before := synthRender("before", 110, 0.1, 0.3, 1.5)
	prefix := strings.Split(i18n.T(i18n.KeyGraphWas), "%")[0]
	for _, v := range allPlotViews {
		vp := buildSynthViewPlot(v, 420, 170, synthViewInput{now: now, before: before})
		i := strings.Index(vp.readout, prefix)
		if i < 0 {
			continue
		}
		was := strings.TrimSuffix(vp.readout[i+len(prefix):], ")")
		if n := len(strings.Fields(was)); n > 3 {
			t.Errorf("%v: %q repeats a sentence (%d words after %q)", v, vp.readout, n, prefix)
		}
	}
}

// A changed sound says how it changed, in words.
func TestWordsSayHowTheSoundChanged(t *testing.T) {
	short := synthRender("short", 220, 0.05, 0, 1)
	long := synthRender("long", 220, 0.5, 0, 1)
	vp := buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{now: long, before: short})
	if !strings.Contains(vp.summary, i18n.T(i18n.KeyGraphWordsLonger)) {
		t.Fatalf("a longer note reads %q, want it to say %q", vp.summary, i18n.T(i18n.KeyGraphWordsLonger))
	}
	vp = buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{now: short, before: long})
	if !strings.Contains(vp.summary, i18n.T(i18n.KeyGraphWordsShorter)) {
		t.Fatalf("a shorter note reads %q, want it to say %q", vp.summary, i18n.T(i18n.KeyGraphWordsShorter))
	}
}

// A slow drift of the pitch (no oscillation) is not a wobble; real vibrato is.
func TestASlowDriftIsNotAWobble(t *testing.T) {
	ph := 0.0
	drift := renderFrom("drift", genNote(1, func(t float64) float64 {
		ph += 220 * math.Pow(2, -0.4*t/12) / testViewSR // 40 cents down over the note
		return 0.6 * math.Sin(2*math.Pi*ph)
	}), 1)
	if got := pitchReadout(drift.an); strings.HasPrefix(got, strings.Split(i18n.T(i18n.KeyGraphPitchWobble), "%")[0]) {
		t.Fatalf("a slow 40-cent drift reads %q: a drift is not a wobble", got)
	}
	ph = 0
	vib := renderFrom("vib", genNote(1, func(t float64) float64 {
		ph += 220 * math.Pow(2, 0.3/12*math.Sin(2*math.Pi*5*t)) / testViewSR
		return 0.6 * math.Sin(2*math.Pi*ph)
	}), 1)
	if got := pitchReadout(vib.an); !strings.HasPrefix(got, strings.Split(i18n.T(i18n.KeyGraphPitchWobble), "%")[0]) {
		t.Fatalf("a 5 Hz ±30-cent vibrato reads %q, want a wobble", got)
	}
}

// Tone colour draws every curve on ONE level scale: a copy of the note 12 dB
// quieter sits lower, it is not re-normalised to the same peak.
func TestToneColourSharesOneLevelScale(t *testing.T) {
	loud := synthRender("loud", 220, 0.3, 0, 1)
	quietPCM := append([]float32(nil), loud.an.PCM()...)
	for i := range quietPCM {
		quietPCM[i] *= float32(math.Pow(10, -12.0/20))
	}
	quiet := renderFrom("quiet", quietPCM, 0.5)
	vp := buildSynthViewPlot(viewColour, 420, 170, synthViewInput{now: loud, hi: quiet})
	var now, hi []fpt
	for _, l := range vp.lines {
		switch l.role {
		case roleNow:
			now = l.pts
		case roleHi:
			hi = l.pts
		}
	}
	if len(now) == 0 || len(hi) == 0 {
		t.Fatal("both curves must be drawn")
	}
	// At the loudest point of NOW, HI is 12 dB lower: 12/60 of the plot height.
	best := 0
	for i := range now {
		if now[i].y < now[best].y {
			best = i
		}
	}
	y, ok := interpY(hi, now[best].x)
	if !ok {
		t.Fatal("the quiet curve does not cover the loud curve's peak")
	}
	want := float32(12.0 / -plotFloorDB * float64(vp.plot.Dy()))
	if d := y - now[best].y; math.Abs(float64(d-want)) > 3 {
		t.Fatalf("the 12 dB quieter copy sits %.1f px under the note, want %.1f px", d, want)
	}
}

// Brightness over time is a line per sound (NOW, ends, BEFORE): readable, and
// comparable — never a heat map.
func TestBrightnessOverTimeIsALine(t *testing.T) {
	now := synthRender("now", 220, 0.3, 0.1, 1)
	lo := synthRender("lo", 220, 0.3, 0.6, 1)
	vp := buildSynthViewPlot(viewColourTime, 420, 170, synthViewInput{now: now, lo: lo})
	if nowLines(vp) == 0 {
		t.Fatal("no NOW brightness line")
	}
	for _, l := range vp.lines {
		if l.role == roleLo {
			return
		}
	}
	t.Fatal("the LO end has no brightness line")
}

// The first moment is its loudness at 1 ms (dB), so a fade-in rises and a
// click stands out — not a raw waveform.
func TestFirstMomentIsALoudnessCurve(t *testing.T) {
	fade := renderFrom("fade", genNote(0.5, func(t float64) float64 {
		return 0.6 * math.Min(1, t/0.1) * math.Sin(2*math.Pi*220*t)
	}), 0.5)
	vp := buildSynthViewPlot(viewAttack, 420, 170, synthViewInput{now: fade})
	if nowLines(vp) == 0 {
		t.Fatal("no NOW curve")
	}
	pts := vp.lines[0].pts
	early, late := pts[len(pts)/10].y, pts[len(pts)*6/10].y
	if early <= late {
		t.Fatalf("a 100 ms fade-in does not rise on screen (y %.0f early vs %.0f later; y grows downward)", early, late)
	}
	if !strings.Contains(vp.summary, i18n.T(i18n.KeyGraphWordsAtkGentle)) && !strings.Contains(vp.summary, i18n.T(i18n.KeyGraphWordsAtkSlow)) {
		t.Fatalf("a 100 ms fade-in reads %q, want it to say it fades in", vp.summary)
	}
}

// A wave close-up of noise shows nothing readable: an unpitched note falls
// back to its tone colour.
func TestWaveCloseUpOfNoiseShowsToneColour(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	noise := renderFrom("noise", genNote(0.4, func(t float64) float64 { return 0.5 * math.Exp(-t/0.1) * rng.NormFloat64() }), 0.4)
	if vp := buildSynthViewPlot(viewWave, 420, 170, synthViewInput{now: noise}); vp.view != viewColour {
		t.Fatalf("the wave close-up of noise is drawn as %v, want tone colour", vp.view)
	}
	if vp := buildSynthViewPlot(viewWave, 420, 170, synthViewInput{now: synthRender("saw", 220, 0.4, 0, 1)}); vp.view != viewWave {
		t.Fatalf("the wave close-up of a saw is drawn as %v, want the wave", vp.view)
	}
}

// Labels never sit on each other or on the axis words.
func TestGraphLabelsNeverOverlap(t *testing.T) {
	now := synthRender("now", 220, 0.3, 0.3, 1)
	lo := synthRender("lo", 220, 0.29, 0.31, 1)
	hi := synthRender("hi", 220, 0.31, 0.29, 1)
	before := synthRender("before", 220, 0.3, 0.32, 1)
	for _, v := range allPlotViews {
		vp := buildSynthViewPlot(v, 420, 170, synthViewInput{now: now, lo: lo, hi: hi, before: before, loLabel: "clean", hiLabel: "sizzly"})
		rects := vp.labelRects()
		for i := range rects {
			for j := 0; j < i; j++ {
				if rects[i].r.Overlaps(rects[j].r) {
					t.Errorf("%v: %q overlaps %q", v, rects[i].text, rects[j].text)
				}
			}
		}
	}
}

var _ = image.Rect

// A dial whose named view does not move on an instrument is shown through the
// view that does: two ends that differ only in how long they ring hold the
// same pitch, so a dial naming the pitch view is shown as loudness.
func TestADialIsShownThroughAViewThatMoves(t *testing.T) {
	short := synthRender("short", 220, 0.05, 0, 1)
	long := synthRender("long", 220, 0.5, 0, 1)
	if px := viewMovePx(viewPitch, short, long, 420, 170); px >= viewMinMovePx {
		t.Fatalf("precondition: the pitch view moves %.1f px between two notes of the same pitch", px)
	}
	v, px := chooseDialView(viewPitch, short, long, 420, 170)
	if v != viewLoudness || px < viewMinMovePx {
		t.Fatalf("ends differing only in length are shown as %v (%.1f px), want loudness", v, px)
	}
	if v, _ := chooseDialView(viewLoudness, short, long, 420, 170); v != viewLoudness {
		t.Fatalf("a named view that moves must be kept, got %v", v)
	}
}

// The wave's shape words follow how long it sits near its peak: a sine about
// half the time (smooth), a clipped wave nearly always (squared off) — a
// rippled or lopsided drive included.
func TestWaveShapeWordsFollowHowSquareTheWaveIs(t *testing.T) {
	wave := func(f func(ph float64) float64) []float32 {
		out := make([]float32, 600)
		for i := range out {
			out[i] = float32(f(2 * math.Pi * float64(i) / 200))
		}
		return out
	}
	sine := wave(math.Sin)
	clipped := wave(func(ph float64) float64 { return math.Max(-0.8, math.Min(0.8, 3*math.Sin(ph))) + 0.05*math.Sin(9*ph) })
	if w := shapeWords(waveGrit(sine)); w != i18n.T(i18n.KeyGraphWordsSmooth) {
		t.Fatalf("a sine reads %q, want smooth", w)
	}
	// A driven kick's close-up: three periods of a fast-decaying, offset,
	// clipped wave, decaying as fast as a real kick does (Kick Wood: −40 dB
	// at 248 ms, a ~18 ms period: ~28 % per period). The decay must not hide
	// the squaring.
	k := 0
	drivenKick := wave(func(ph float64) float64 {
		k++
		return 0.1 + math.Exp(-float64(k)/600)*math.Max(-0.8, math.Min(0.8, 3*math.Sin(ph)))
	})
	if w := shapeWords(waveGrit(drivenKick)); w != i18n.T(i18n.KeyGraphWordsSquared) {
		t.Fatalf("a decaying, offset, clipped wave reads %q (grit %.3f), want squared off", w, waveGrit(drivenKick))
	}
	if w := shapeWords(waveGrit(clipped)); w != i18n.T(i18n.KeyGraphWordsSquared) {
		t.Fatalf("a hard-clipped, rippled wave reads %q (grit %.3f), want squared off", w, waveGrit(clipped))
	}
}

// With the dial's two ends on screen the shape words place the sound between
// them (a kick's clean end already carries harmonics of its own: an absolute
// scale would call every kick gritty).
func TestWaveWordsPlaceTheSoundBetweenTheDialsEnds(t *testing.T) {
	// A kick-like note: its pitch glides down and it decays fast, so no
	// close-up window holds whole, equal periods.
	note := func(name string, drive float64) *graphRender {
		ph := 0.0
		return renderFrom(name, genNote(0.5, func(tt float64) float64 {
			ph += (55 + 90*math.Exp(-tt/0.04)) / testViewSR
			x := math.Sin(2*math.Pi*ph) + 0.3*math.Sin(4*math.Pi*ph) // its own 2nd harmonic
			return 0.7 * math.Exp(-tt/0.12) * math.Tanh(drive*x) / math.Tanh(drive)
		}), 0.5)
	}
	clean, mid, driven := note("clean", 0.3), note("mid", 2.5), note("driven", 6)
	at := clean.an.WaveMoment()
	g := func(r *graphRender) float64 { return soundGrit(r, r.an.WaveAt(at, 3).Samples) }
	if a, b, c := g(clean), g(mid), g(driven); !(a < b && b < c) {
		t.Fatalf("grit does not rise with drive on a gliding, decaying note: %.3f, %.3f, %.3f", a, b, c)
	}
	if vp := buildSynthViewPlot(viewWave, 420, 170, synthViewInput{now: clean, lo: clean, hi: driven}); !strings.HasPrefix(vp.summary, i18n.T(i18n.KeyGraphWordsSmooth)) {
		t.Fatalf("at the clean end the wave reads %q, want smooth", vp.summary)
	}
	if vp := buildSynthViewPlot(viewWave, 420, 170, synthViewInput{now: driven, lo: clean, hi: driven}); !strings.HasPrefix(vp.summary, i18n.T(i18n.KeyGraphWordsSquared)) {
		t.Fatalf("at the driven end the wave reads %q, want squared off", vp.summary)
	}
}
