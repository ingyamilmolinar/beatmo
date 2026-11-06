package clone

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// eightModes is an inharmonic struck set (higher = faster, quieter).
func eightModes() []Mode {
	ms := []Mode{}
	for i, r := range []float64{1, 2.76, 5.40, 8.93, 13.34, 18.64, 24.81, 31.87} {
		f := 261.63 * r
		ms = append(ms, Mode{Freq: f, Alpha: 1.5 * math.Pow(f/1000, 0.8), Amp: 0.5 / float64(i+1), Phase: 0.4*float64(i) - 1.3})
	}
	return ms
}

func analysisOf(modes []Mode, pitch PitchInfo) Analysis {
	return Analysis{
		Format: AnalysisFormat, Rate: 48000, FloorDB: -80, Pitch: pitch,
		Modes:    modes,
		Strike:   Strike{TcSec: 0.0008, HardnessDBPerOct: -2.5},
		DecayLaw: FitDecayLaw(modes),
		Residual: Residual{Bands: []NoiseBand{
			{CenterHz: 2000, Q: thirdOctaveQ, LevelDB: -30, AttackSec: 0.002, FastRate: 80, FastMix: 0.7, TailRate: 9, Active: true},
			{CenterHz: 6300, Q: thirdOctaveQ, LevelDB: -36, AttackSec: 0.001, FastRate: 200, FastMix: 0.9, TailRate: 30, Active: true},
			{CenterHz: 9000, Q: thirdOctaveQ, LevelDB: -200},
		}},
	}
}

// assertSameModes matches each want mode to the doc mode in its row.
func assertSameModes(t *testing.T, got, want []Mode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d modes, want %d", len(got), len(want))
	}
	rel := func(a, b float64) float64 { return math.Abs(a-b) / math.Max(math.Abs(b), 1e-12) }
	for _, w := range want {
		var g *Mode
		for i := range got {
			if rel(got[i].Freq, w.Freq) < 1e-6 {
				g = &got[i]
			}
		}
		if g == nil {
			t.Fatalf("no mode at %.3f Hz in %v", w.Freq, got)
		}
		if rel(g.Freq, w.Freq) > 1e-9 || rel(g.Alpha, w.Alpha) > 1e-9 || rel(g.Amp, w.Amp) > 1e-9 ||
			math.Abs(math.Remainder(g.Phase-w.Phase, 2*math.Pi)) > 1e-12 {
			t.Errorf("mode %.2f Hz: got f=%.6f α=%.6f A=%.6g φ=%.4f, want f=%.6f α=%.6f A=%.6g φ=%.4f",
				w.Freq, g.Freq, g.Alpha, g.Amp, g.Phase, w.Freq, w.Alpha, w.Amp, w.Phase)
		}
	}
}

func TestInvertReproducesTheMeasuredModes(t *testing.T) {
	cases := []struct {
		name  string
		pitch PitchInfo
	}{
		{"pitched", PitchInfo{Pitched: true, F0Hz: 261.63, SemisFromA3: 3, HasSemis: true}},
		{"unpitched at its catalogue note", PitchInfo{SemisFromA3: -5, HasSemis: true}},
		{"unpitched, no note", PitchInfo{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := analysisOf(eightModes(), c.pitch)
			d, rep, err := Invert(a, InvertOptions{ID: "x-clone"})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Melodic != c.pitch.Pitched || d.Render.Melodic != c.pitch.Pitched {
				t.Fatalf("melodic %v/%v, want %v", rep.Melodic, d.Render.Melodic, c.pitch.Pitched)
			}
			assertSameModes(t, DocModes(d, rep.FitPitch, 48000), a.Modes)
			nb := DocNoise(d, rep.FitPitch)
			if len(nb) != 2 || math.Abs(nb[0].CenterHz-2000) > 1e-9 || math.Abs(nb[0].TailRate-9) > 1e-9 ||
				math.Abs(nb[1].AttackSec-0.001) > 1e-12 || nb[0].LevelDB != -30 {
				t.Fatalf("noise rows %+v", nb)
			}
		})
	}
}

// TestInvertPercussionPreScalesForTheSessionPitch: an unpitched clone is
// stored in the pitch-0 domain — at pitch 0 every frequency and rate is the
// measurement ÷ 2^(s/12), so the playback resample at pitch s restores it.
func TestInvertPercussionPreScalesForTheSessionPitch(t *testing.T) {
	a := analysisOf(eightModes(), PitchInfo{SemisFromA3: -5, HasSemis: true})
	d, _, err := Invert(a, InvertOptions{ID: "tom-clone"})
	if err != nil {
		t.Fatal(err)
	}
	k := math.Pow(2, -5.0/12)
	want := make([]Mode, len(a.Modes))
	for i, m := range a.Modes {
		want[i] = m
		want[i].Freq /= k
		want[i].Alpha /= k
	}
	assertSameModes(t, DocModes(d, 0, 48000), want)
	if got := d.Stages["strike"].Values["strike_tc"]; math.Abs(got-0.0008*k) > 1e-15 {
		t.Fatalf("strike_tc %v, want %v", got, 0.0008*k)
	}
}

func TestInvertBudget(t *testing.T) {
	var ms []Mode
	for i := 0; i < 200; i++ {
		ms = append(ms, Mode{Freq: 100 + 37*float64(i), Alpha: 5, Amp: math.Pow(10, -float64(i)/100), Phase: 0})
	}
	// A doublet straddling the cut (ranks 127 / 128 by energy).
	ms[127].Freq, ms[128].Freq = 5000, 5010
	ms = append(ms, Mode{Freq: 300, Alpha: 5, Amp: 1e-4}) // −80 dB: below the floor
	sel, dropped := selectModes(ms, 128, -60)
	if len(sel) != 127 || dropped != len(ms)-127 {
		t.Fatalf("kept %d dropped %d, want 127 (the doublet is never split)", len(sel), dropped)
	}
	for i := 1; i < len(sel); i++ {
		if sel[i].Energy() > sel[i-1].Energy() {
			t.Fatal("not in energy order")
		}
	}
	for _, m := range sel {
		if m.Freq == 300 {
			t.Fatal("kept a mode below the −60 dB floor")
		}
	}
}

func TestInvertClampsRowsFarAboveTheContactCorner(t *testing.T) {
	// A 9 ms contact puts ms[2] (1.41 kHz) 75 dB down the strike's roll-off:
	// its stored amplitude would exceed the ±1000 bound. Only it is clamped.
	ms := eightModes()[:3]
	a := analysisOf(ms, PitchInfo{Pitched: true, F0Hz: 261.63})
	a.Strike.TcSec = 0.009
	_, rep, err := Invert(a, InvertOptions{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clamped != 1 || len(rep.Notes) == 0 {
		t.Fatalf("clamped %d notes %v, want 1 + a note", rep.Clamped, rep.Notes)
	}
}

func TestInvertValidatesAndKeepsIdentityInto(t *testing.T) {
	into := instrument.Doc{ID: roster.TubularBells, Name: "Tubular Bells", Category: "mallet", Notes: "n",
		Render: instrument.Render{Seconds: 3}}
	d, _, err := Invert(analysisOf(eightModes(), PitchInfo{Pitched: true, F0Hz: 261.63}), InvertOptions{ID: "ignored", Into: &into})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != roster.TubularBells || d.Name != "Tubular Bells" || d.Category != "mallet" || d.Render.Seconds != 3 {
		t.Fatalf("identity not kept: %+v", d.Render)
	}
	if err := instrument.Validate(d, audio.BuiltinModuleRegistry()); err != nil {
		t.Fatal(err)
	}
	if len(d.FX) != 0 {
		t.Fatal("a clone carries no FX")
	}
	if got := d.Stages["modal"].Values["modal_keep"]; got != 8 {
		t.Fatalf("keep %v", got)
	}
	if b, err := instrument.MarshalFile(d); err != nil || len(b) > 64<<10 {
		t.Fatalf("marshal %d bytes err %v", len(b), err)
	}
}

func TestRenderSecondsReachTheFloor(t *testing.T) {
	d, _, err := Invert(analysisOf(eightModes(), PitchInfo{Pitched: true, F0Hz: 261.63}), InvertOptions{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Strongest-lasting part: mode 0 at 0.5, α = 1.5·0.2616^0.8 ≈ 0.51 Np/s,
	// to −80 dB → ≈ 16.7 s, clamped to 10.
	if d.Render.Seconds != maxRenderSec {
		t.Fatalf("render %+v", d.Render)
	}
}

// TestInvertDropsRowsTheResampleWouldPushOutOfRange: a +22 st percussion
// reference stores its model at pitch 0 — a 31.5 Hz noise band would land at
// 8.8 Hz, below the column's bound, and is dropped instead of failing.
func TestInvertDropsRowsTheResampleWouldPushOutOfRange(t *testing.T) {
	a := analysisOf(eightModes(), PitchInfo{SemisFromA3: 22, HasSemis: true})
	a.Residual.Bands = append(a.Residual.Bands, NoiseBand{CenterHz: 31.5, Q: thirdOctaveQ, LevelDB: -50, TailRate: 5, Active: true})
	d, rep, err := Invert(a, InvertOptions{ID: "conga-clone"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.NoiseRows != 2 || len(DocNoise(d, 0)) != 2 {
		t.Fatalf("noise rows %d, want the two in-range bands", rep.NoiseRows)
	}
}
