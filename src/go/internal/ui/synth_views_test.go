package ui

import (
	"image"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// Every view draws NOW from a render, shows a ghost only when its render exists, keeps its axes while the control
// moves, shades only against a BEFORE, and says so when there is no picture yet.

const testViewSR = 48000

// A saw decaying with tau, noise mixed in, dropping from drop×hz to hz over 40 ms when drop > 1.
func synthRender(name string, hz, tau, noise, drop float64) *graphRender {
	rng := rand.New(rand.NewSource(int64(len(name))))
	n := testViewSR
	pcm := make([]float32, n)
	ph := 0.0
	for i := range pcm {
		t := float64(i) / testViewSR
		f := hz
		if drop > 1 {
			f = hz * (1 + (drop-1)*math.Exp(-t/0.02))
		}
		ph += f / testViewSR
		saw := 2*(ph-math.Floor(ph)) - 1
		pcm[i] = float32(0.6 * math.Exp(-t/tau) * ((1-noise)*saw + noise*(rng.Float64()*2-1)))
	}
	r := &graphRender{ident: graphIdent{hash: name}, sr: testViewSR, voiceSec: 0.5, an: soundview.New(pcm, testViewSR, 0.5)}
	r.an.Warm(soundview.NeedAll, nil)
	return r
}

var allPlotViews = []synthView{viewOverview, viewLoudness, viewAttack, viewPitch, viewColour, viewHarmonics, viewColourTime, viewNoise, viewWave}

func nowLines(vp *viewPlot) int {
	n := 0
	for _, l := range vp.lines {
		if l.role == roleNow && len(l.pts) >= 2 {
			n++
		}
	}
	return n
}

func TestEveryViewBuildsFromARender(t *testing.T) {
	now := synthRender("now", 220, 0.15, 0.2, 2)
	in := synthViewInput{now: now, lo: synthRender("lo", 220, 0.05, 0, 1), hi: synthRender("hi", 220, 0.3, 0.6, 3),
		loLabel: "lo", hiLabel: "hi", caption: "Test — a control", beatSec: 0.5}
	for _, v := range allPlotViews {
		vp := buildSynthViewPlot(v, 420, 170, in)
		if vp.empty != "" {
			t.Fatalf("%v: empty with a render: %q", v, vp.empty)
		}
		if nowLines(vp) == 0 {
			t.Errorf("%v: no NOW curve drawn", v)
		}
		if strings.TrimSpace(vp.readout) == "" {
			t.Errorf("%v: no measured readout", v)
		}
		if len(vp.xTicks) == 0 {
			t.Errorf("%v: an unlabelled x axis", v)
		}
		pr := vp.plot.Inset(-2)
		for _, l := range vp.lines {
			for _, p := range l.pts {
				if !image.Pt(int(p.x), int(p.y)).In(pr) {
					t.Fatalf("%v: a %v point %v escapes the plot %v", v, l.role, p, vp.plot)
				}
			}
		}
	}
}

func TestGhostsAppearOnlyWhenTheirRenderExists(t *testing.T) {
	now := synthRender("now", 220, 0.15, 0.2, 1)
	lo := synthRender("lo", 220, 0.05, 0, 1)
	for _, v := range []synthView{viewLoudness, viewPitch, viewColour, viewHarmonics, viewNoise, viewWave, viewAttack} {
		has := func(vp *viewPlot, role plotRole) bool {
			for _, l := range vp.lines {
				if l.role == role {
					return true
				}
			}
			return false
		}
		bare := buildSynthViewPlot(v, 420, 170, synthViewInput{now: now})
		if has(bare, roleLo) || has(bare, roleHi) || has(bare, roleBefore) {
			t.Errorf("%v: a ghost drawn with no ghost render", v)
		}
		with := buildSynthViewPlot(v, 420, 170, synthViewInput{now: now, lo: lo, loLabel: "short"})
		if !has(with, roleLo) {
			t.Errorf("%v: the LO render drew no ghost", v)
		}
		found := false
		for _, l := range with.labels {
			found = found || (l.role == roleLo && l.text == "short")
		}
		if !found {
			t.Errorf("%v: the LO ghost carries no end-word label", v)
		}
	}
}

// From the renders on screen (the ends bracket NOW), not from the control's value.
func TestAxesHoldStillWhileTheControlMoves(t *testing.T) {
	lo := synthRender("lo", 220, 0.05, 0, 1)
	hi := synthRender("hi", 220, 0.4, 0, 1)
	var ref []plotTick
	for i, tau := range []float64{0.08, 0.15, 0.25, 0.35} {
		now := synthRender("now", 220, tau, 0, 1)
		vp := buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{now: now, lo: lo, hi: hi})
		if i == 0 {
			ref = vp.xTicks
			continue
		}
		if len(vp.xTicks) != len(ref) {
			t.Fatalf("tau %.2f: the time axis changed (%d ticks, was %d)", tau, len(vp.xTicks), len(ref))
		}
		for k := range ref {
			if vp.xTicks[k] != ref[k] {
				t.Fatalf("tau %.2f: tick %d moved %v -> %v", tau, k, ref[k], vp.xTicks[k])
			}
		}
	}
}

func TestDifferenceShadingNeedsABefore(t *testing.T) {
	now := synthRender("now", 220, 0.3, 0, 1)
	before := synthRender("before", 220, 0.08, 0, 1)
	if vp := buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{now: now}); len(vp.diff) != 0 {
		t.Fatal("difference shading with no BEFORE")
	}
	vp := buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{now: now, before: before})
	if len(vp.diff) == 0 {
		t.Fatal("NOW and BEFORE differ, but nothing is shaded")
	}
	if !strings.Contains(vp.readout, strings.TrimSuffix(strings.Split(i18n.T(i18n.KeyGraphWas), "%")[0], " ")) {
		t.Fatalf("the readout %q does not say what it was", vp.readout)
	}
}

func TestNoPictureBeforeTheFirstRender(t *testing.T) {
	vp := buildSynthViewPlot(viewLoudness, 420, 170, synthViewInput{caption: "x"})
	if vp.empty != i18n.T(i18n.KeyGraphNoPicture) || len(vp.lines) != 0 {
		t.Fatalf("with no render the graph must say %q and draw nothing; got %q, %d lines", i18n.T(i18n.KeyGraphNoPicture), vp.empty, len(vp.lines))
	}
}

func TestPitchReadoutNamesTheMotion(t *testing.T) {
	glide := pitchReadout(synthRender("g", 55, 0.4, 0, 3).an)
	if !strings.HasPrefix(glide, strings.Split(i18n.T(i18n.KeyGraphPitchGlide), "%")[0]) {
		t.Fatalf("a pitch drop reads %q, want the glide sentence", glide)
	}
	hold := pitchReadout(synthRender("h", 220, 0.4, 0, 1).an)
	if !strings.Contains(hold, "A3") {
		t.Fatalf("a steady 220 Hz reads %q, want it to name A3", hold)
	}
}
