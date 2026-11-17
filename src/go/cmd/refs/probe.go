//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// probe.go — `refs probe`: synthesise ISOLATED acoustic properties so the
// listener can name them.
//
// WHY
// ---
// Every axis in the battery is calibrated against instrument comparisons, where
// a dozen properties vary at once. That makes the listener's words ambiguous:
// when they wrote "too airy", six axes fired, and it took a separate
// specificity analysis to work out which one they meant. And it makes the
// thresholds guesses -- "0.05 is an audible shift" is an assertion nobody has
// tested.
//
// These probes vary ONE property at a time over a range, holding everything
// else fixed, at a constant pitch and level. Played in order, each set answers
// two questions no instrument comparison can:
//
//   - which WORD does this listener use for this property, in which direction?
//   - at what STEP does the difference become audible to them?
//
// The second is what turns a threshold from a guess into a measurement. The
// printed table gives the measured value of the target axis for every step, so
// "steps 1 and 2 sound the same, 3 is clearly different" reads directly off as
// a calibrated drift threshold.

const (
	probeSR      = 44100
	probeDurSec  = 2.0
	probeF0      = 220.0
	probeTargetR = 0.14 // common RMS every probe is normalised to
)

// probeStep is one rendered stimulus.
type probeStep struct {
	Set   string
	Index int
	Label string
	W     wave.Wave
	// Axis is the descriptor this set is designed to move, and Measured its
	// value on this step -- so the listener's answer lands directly on a number.
	Axis     string
	Measured float64
}

// probeSet is one property, varied over steps.
type probeSet struct {
	Name  string
	Axis  string
	Ask   string // what to listen for, in plain language
	Steps []probeStep
}

// --- Synthesis helpers -------------------------------------------------------

// harmonicTone builds a tone whose partial k has amplitude k^(slope/6) --
// i.e. `slope` dB per octave of harmonic number. Everything else is held fixed.
func harmonicTone(slopeDBPerOct float64, nHarm int) []float64 {
	n := int(probeSR * probeDurSec)
	out := make([]float64, n)
	for k := 1; k <= nHarm; k++ {
		f := probeF0 * float64(k)
		if f > 0.45*probeSR {
			break
		}
		amp := math.Pow(float64(k), slopeDBPerOct/6.0206)
		for i := range out {
			t := float64(i) / probeSR
			out[i] += amp * math.Sin(2*math.Pi*f*t)
		}
	}
	return out
}

// inharmonicTone places partials at explicit ratios instead of integers.
func inharmonicTone(ratios []float64) []float64 {
	n := int(probeSR * probeDurSec)
	out := make([]float64, n)
	for k, r := range ratios {
		f := probeF0 * r
		if f > 0.45*probeSR {
			continue
		}
		amp := 1.0 / float64(k+1)
		for i := range out {
			t := float64(i) / probeSR
			out[i] += amp * math.Sin(2*math.Pi*f*t)
		}
	}
	return out
}

// addNoise mixes band-limited-ish noise whose level follows an exponential from
// `at0` to `atEnd` over the note -- the shape noise.persistence measures.
func addNoise(sig []float64, at0, atEnd, seed float64) []float64 {
	out := append([]float64(nil), sig...)
	rng := rand.New(rand.NewSource(int64(seed)))
	// One-pole high-pass so the noise sits in the "air" band rather than
	// muddying the fundamental -- breath and bow hiss are high, not low.
	prev, prevOut := 0.0, 0.0
	for i := range out {
		t := float64(i) / float64(len(out)) // 0..1 over the note
		lvl := at0 * math.Pow(math.Max(atEnd, 1e-6)/math.Max(at0, 1e-6), t)
		w := rng.Float64()*2 - 1
		hp := 0.85 * (prevOut + w - prev)
		prev, prevOut = w, hp
		out[i] += lvl * hp
	}
	return out
}

// applyADSR shapes a signal with a simple attack/decay envelope.
func applyADSR(sig []float64, attackSec, decaySec, sustain float64) []float64 {
	out := append([]float64(nil), sig...)
	n := len(out)
	rel := 0.15
	for i := range out {
		t := float64(i) / probeSR
		dur := float64(n) / probeSR
		var e float64
		switch {
		case t < attackSec:
			e = t / math.Max(attackSec, 1e-6)
		case decaySec > 0 && t < attackSec+decaySec:
			x := (t - attackSec) / decaySec
			e = 1 + (sustain-1)*x
		case t > dur-rel:
			e = sustain * (dur - t) / rel
		default:
			e = sustain
		}
		out[i] *= e
	}
	return out
}

// addVibrato applies sinusoidal pitch modulation via a modulated delay, with
// optional rate jitter -- the same technique modular.c uses for the waveguides,
// so what the listener hears here is what the engine produces.
func addVibrato(sig []float64, depthSemis, rateHz, jitter float64) []float64 {
	if depthSemis <= 0 || rateHz <= 0 {
		return append([]float64(nil), sig...)
	}
	const dmax = 4096
	peak := depthSemis * 0.0091938185 / rateHz * probeSR
	if peak > dmax/2-4 {
		peak = dmax/2 - 4
	}
	centre := peak + 2
	buf := make([]float64, dmax)
	out := make([]float64, len(sig))
	wi := 0
	for i := range sig {
		buf[wi] = sig[i]
		t := float64(i) / probeSR
		phase := 2 * math.Pi * rateHz * (t + jitter*(1-math.Cos(2*math.Pi*0.37*t))/(2*math.Pi*0.37))
		d := centre + peak*math.Cos(phase)
		rp := float64(wi) - d
		for rp < 0 {
			rp += dmax
		}
		i0 := int(rp)
		fr := rp - float64(i0)
		i1 := (i0 + 1) % dmax
		out[i] = (1-fr)*buf[i0%dmax] + fr*buf[i1]
		wi = (wi + 1) % dmax
	}
	return out
}

// normalise brings every probe to one RMS so loudness never confounds the
// comparison -- the same discipline the A/B renderer applies.
func normalise(sig []float64) []float64 {
	var sumsq float64
	for _, v := range sig {
		sumsq += v * v
	}
	if sumsq <= 0 {
		return sig
	}
	g := probeTargetR / math.Sqrt(sumsq/float64(len(sig)))
	// Guard the peak so normalisation cannot clip.
	var peak float64
	for _, v := range sig {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	if peak*g > 0.99 {
		g = 0.99 / peak
	}
	out := make([]float64, len(sig))
	for i, v := range sig {
		out[i] = v * g
	}
	return out
}

func mkWave(sig []float64) wave.Wave {
	return wave.Wave{Samples: normalise(sig), SampleRate: probeSR}
}

// --- The probe sets ----------------------------------------------------------

// buildProbeSets defines every set. Each varies ONE property; the pitch,
// duration and level are identical across all of them.
func buildProbeSets() []probeSet {
	base := func() []float64 { return harmonicTone(-9, 24) }

	return []probeSet{
		{
			Name: "warmth", Axis: "brightness ratio",
			Ask: "Cool/bright vs warm/dark. Same pitch, same loudness; only the harmonic roll-off changes.",
			Steps: []probeStep{
				{Label: "steepest roll-off (darkest)", W: mkWave(applyADSR(harmonicTone(-15, 24), 0.02, 0.05, 0.9))},
				{Label: "steep", W: mkWave(applyADSR(harmonicTone(-12, 24), 0.02, 0.05, 0.9))},
				{Label: "medium", W: mkWave(applyADSR(harmonicTone(-9, 24), 0.02, 0.05, 0.9))},
				{Label: "shallow", W: mkWave(applyADSR(harmonicTone(-6, 24), 0.02, 0.05, 0.9))},
				{Label: "shallowest (brightest)", W: mkWave(applyADSR(harmonicTone(-3, 24), 0.02, 0.05, 0.9))},
			},
		},
		{
			Name: "noise-persistence", Axis: "noise persistence",
			Ask: "String vs wind, and dry vs airy. Identical tone and identical TOTAL noise; only WHEN the noise sits changes -- onset only, fading, or constant.",
			Steps: []probeStep{
				{Label: "noise at onset only (bow/pluck bite)", W: mkWave(applyADSR(addNoise(base(), 0.60, 0.002, 1), 0.02, 0.05, 0.9))},
				{Label: "noise fading over the note", W: mkWave(applyADSR(addNoise(base(), 0.35, 0.03, 2), 0.02, 0.05, 0.9))},
				{Label: "noise slightly fading", W: mkWave(applyADSR(addNoise(base(), 0.18, 0.09, 3), 0.02, 0.05, 0.9))},
				{Label: "noise constant (breath)", W: mkWave(applyADSR(addNoise(base(), 0.12, 0.12, 4), 0.02, 0.05, 0.9))},
				{Label: "noise RISING through the note", W: mkWave(applyADSR(addNoise(base(), 0.04, 0.30, 5), 0.02, 0.05, 0.9))},
			},
		},
		{
			Name: "airiness", Axis: "air-band flatness",
			Ask: "How much high hiss sits on top of the tone. The tone underneath is identical; only the amount of air changes.",
			Steps: []probeStep{
				{Label: "no air", W: mkWave(applyADSR(base(), 0.02, 0.05, 0.9))},
				{Label: "a little air", W: mkWave(applyADSR(addNoise(base(), 0.05, 0.05, 6), 0.02, 0.05, 0.9))},
				{Label: "moderate air", W: mkWave(applyADSR(addNoise(base(), 0.15, 0.15, 7), 0.02, 0.05, 0.9))},
				{Label: "very airy", W: mkWave(applyADSR(addNoise(base(), 0.40, 0.40, 8), 0.02, 0.05, 0.9))},
			},
		},
		{
			Name: "partial-structure", Axis: "partial series deviation",
			Ask: "String/harmonic vs wooden bar vs metallic bell. Same pitch and envelope; only WHERE the overtones sit changes.",
			Steps: []probeStep{
				{Label: "pure harmonic series 1,2,3,4 (string/organ)", W: mkWave(applyADSR(inharmonicTone([]float64{1, 2, 3, 4, 5, 6, 7, 8}), 0.005, 0.6, 0.25))},
				{Label: "slightly stretched (piano-like)", W: mkWave(applyADSR(inharmonicTone([]float64{1, 2.01, 3.05, 4.11, 5.2, 6.32, 7.5, 8.7}), 0.005, 0.6, 0.25))},
				{Label: "tuned bar 1,4,9 (marimba/xylophone)", W: mkWave(applyADSR(inharmonicTone([]float64{1, 3.9, 9.2, 16.0}), 0.005, 0.6, 0.25))},
				{Label: "bell/tube modes (metallic)", W: mkWave(applyADSR(inharmonicTone([]float64{0.62, 1.0, 1.47, 1.71, 2.01, 2.61, 3.26}), 0.005, 1.4, 0.5))},
			},
		},
		{
			Name: "attack", Axis: "attack",
			Ask: "Percussive vs blown/bowed. Same tone throughout; only how fast it starts changes.",
			Steps: []probeStep{
				{Label: "1 ms (struck)", W: mkWave(applyADSR(base(), 0.001, 0.05, 0.9))},
				{Label: "20 ms (plucked)", W: mkWave(applyADSR(base(), 0.02, 0.05, 0.9))},
				{Label: "80 ms (bowed)", W: mkWave(applyADSR(base(), 0.08, 0.05, 0.9))},
				{Label: "300 ms (slow swell)", W: mkWave(applyADSR(base(), 0.30, 0.05, 0.9))},
			},
		},
		{
			Name: "energy-shape", Axis: "energy sections",
			Ask: "Where the note's energy sits in time: front-loaded and fading, steady, or swelling.",
			Steps: []probeStep{
				{Label: "front-loaded, fast fade (pluck)", W: mkWave(applyADSR(base(), 0.005, 0.4, 0.05))},
				{Label: "front-loaded, slow fade", W: mkWave(applyADSR(base(), 0.005, 1.2, 0.2))},
				{Label: "steady (held)", W: mkWave(applyADSR(base(), 0.02, 0.05, 0.9))},
				{Label: "swelling (louder at the end)", W: mkWave(applyADSR(base(), 1.4, 0.05, 1.0))},
			},
		},
		{
			Name: "vibrato", Axis: "vibrato extent",
			Ask: "Vibrato: how much, and whether it sounds like a player or a machine. Steps 4 and 5 have the SAME depth -- one is metronomic, one wanders.",
			Steps: []probeStep{
				{Label: "none", W: mkWave(applyADSR(base(), 0.02, 0.05, 0.9))},
				{Label: "2.5 cents, perfectly regular (what the engine shipped)", W: mkWave(applyADSR(addVibrato(base(), 0.025, 4.0, 0), 0.02, 0.05, 0.9))},
				{Label: "10 cents, perfectly regular", W: mkWave(applyADSR(addVibrato(base(), 0.10, 4.0, 0), 0.02, 0.05, 0.9))},
				{Label: "30 cents, perfectly regular (machine)", W: mkWave(applyADSR(addVibrato(base(), 0.30, 4.0, 0), 0.02, 0.05, 0.9))},
				{Label: "30 cents, wandering rate (player)", W: mkWave(applyADSR(addVibrato(base(), 0.30, 4.0, 0.35), 0.02, 0.05, 0.9))},
			},
		},
		{
			Name: "tristimulus", Axis: "tristimulus",
			Ask: "Which band carries the tone: the fundamental alone, the low overtones, or the high ones. Same pitch and loudness.",
			Steps: []probeStep{
				{Label: "fundamental only (flute-like)", W: mkWave(applyADSR(inharmonicTone([]float64{1}), 0.02, 0.05, 0.9))},
				{Label: "fundamental dominant", W: mkWave(applyADSR(harmonicTone(-18, 16), 0.02, 0.05, 0.9))},
				{Label: "partials 2-4 dominant (reedy)", W: mkWave(applyADSR(inharmonicTone([]float64{0.2, 2, 3, 4, 0.3, 0.2}), 0.02, 0.05, 0.9))},
				{Label: "high partials dominant (bright/thin)", W: mkWave(applyADSR(harmonicTone(-1.5, 24), 0.02, 0.05, 0.9))},
			},
		},
	}
}

// measureStep fills in the target axis's value for one step, so the printed
// table maps the listener's answer straight onto a number.
func measureStep(set *probeSet, i int) {
	st := &set.Steps[i]
	st.Set, st.Index, st.Axis = set.Name, i+1, set.Axis
	b := fingerprint.NewBundle(st.W, fmt.Sprintf("%s-%d", set.Name, i+1))
	for _, d := range fingerprint.DefaultRegistry().All() {
		if d.Name != set.Axis {
			continue
		}
		if !d.Applies(b) {
			return
		}
		if m := d.Extract(b); m.OK {
			st.Measured = m.Primary()
		}
		return
	}
}

func probeDir() (string, error) {
	base, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for probe audio", wavref.EnvVar)
	}
	d := filepath.Join(base, "probes")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func runProbe(w io.Writer, only string, sh bool) error {
	dir, err := probeDir()
	if err != nil {
		return err
	}
	sets := buildProbeSets()

	var wrote int
	for si := range sets {
		s := &sets[si]
		if only != "" && s.Name != only {
			continue
		}
		for i := range s.Steps {
			measureStep(s, i)
			p := filepath.Join(dir, fmt.Sprintf("%s_%d.wav", s.Name, i+1))
			if err := writeCompareWAV(p, s.Steps[i].W.Samples, probeSR); err != nil {
				return err
			}
			s.Steps[i].W.Label = absOrSelf(p)
			wrote++
		}
	}
	if wrote == 0 {
		return fmt.Errorf("refs probe: no set named %q (see the table from a bare `refs probe`)", only)
	}

	if sh {
		return printProbeSh(w, sets, only)
	}

	fmt.Fprintf(w, "Wrote %d probe clips to %s\n\n", wrote, dir)
	fmt.Fprintln(w, "Each SET varies exactly one property; pitch (220 Hz), duration (2 s) and RMS are")
	fmt.Fprintln(w, "identical across every clip, so anything you hear is the property and nothing else.")
	fmt.Fprintln(w, "Listen through a set in order and say (a) what word you'd use at each end, and")
	fmt.Fprintln(w, "(b) the first step where you hear a clear difference -- that step IS the threshold.")
	for si := range sets {
		s := &sets[si]
		if only != "" && s.Name != only {
			continue
		}
		fmt.Fprintf(w, "\n=== %s === (axis: %s)\n%s\n", s.Name, s.Axis, s.Ask)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "  #\tMEASURED %s\tDESCRIPTION\tFILE\n", s.Axis)
		for i, st := range s.Steps {
			fmt.Fprintf(tw, "  %d\t%.4g\t%s\t%s\n", i+1, st.Measured, st.Label, filepath.Base(st.W.Label))
		}
		tw.Flush()
	}
	return nil
}

// printProbeSh emits an eval-able plan so a Makefile can play a set in order,
// mirroring `refs compare -sh` / `refs audition -sh`.
func printProbeSh(w io.Writer, sets []probeSet, only string) error {
	var flat []probeStep
	for si := range sets {
		s := &sets[si]
		if only != "" && s.Name != only {
			continue
		}
		flat = append(flat, s.Steps...)
	}
	fmt.Fprintf(w, "BEATMO_PROBE_N=%s\n", shQuote(fmt.Sprintf("%d", len(flat))))
	fmt.Fprintf(w, "BEATMO_PROBE_DUR=%s\n", shQuote(fmt.Sprintf("%.2f", probeDurSec)))
	for i, st := range flat {
		fmt.Fprintf(w, "BEATMO_PROBE%d_NAME=%s\n", i+1, shQuote(fmt.Sprintf("%s %d/%d: %s", st.Set, st.Index, 0, st.Label)))
		fmt.Fprintf(w, "BEATMO_PROBE%d_PATH=%s\n", i+1, shQuote(st.W.Label))
	}
	return nil
}

func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	only := fs.String("set", "", "render only the named set (default: all)")
	sh := fs.Bool("sh", false, "print an eval-able BEATMO_PROBE* plan instead of the table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runProbe(os.Stdout, *only, *sh)
}
