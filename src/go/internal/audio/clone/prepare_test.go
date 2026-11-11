package clone

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

func TestPrepareFindsOnsetFloorAndWindow(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2.5, 0.001, 1)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d := math.Abs(float64(p.Onset - 2400)); d > 48 { // ≤ 1 ms
		t.Fatalf("onset %d, want 2400 ± 48", p.Onset)
	}
	if fl := dB(p.FloorRMS); math.Abs(fl-(-60)) > 3 {
		t.Fatalf("floor %.1f dB, want ≈ −60", fl)
	}
	if p.WindowStart <= p.Onset || p.WindowStart-p.Onset > 48*60 {
		t.Fatalf("window start %d not in (onset, onset+60 ms]", p.WindowStart)
	}
	if p.WindowEnd-p.WindowStart < 48000 {
		t.Fatalf("window %d..%d shorter than 1 s for a 2 Np/s fundamental", p.WindowStart, p.WindowEnd)
	}
	if p.Rate != 48000 || len(p.Samples) != len(x) {
		t.Fatalf("rate/len changed: %d %d", p.Rate, len(p.Samples))
	}
}

func TestPrepareOnsetAtSampleZero(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0, 2, 0.001, 2)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Onset > 48 {
		t.Fatalf("onset %d, want ≤ 48 (1 ms) for a signal starting at sample 0", p.Onset)
	}
}

func TestPrepareStillRingingAtClipEnd(t *testing.T) {
	m := []Mode{{Freq: 300, Alpha: 0.5, Amp: 0.5}} // −6.5 dB after 1.5 s
	x := modalSignal(m, 48000, 0.05, 1.5, 0.0005, 3)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatalf("refused a reference still ringing at its end: %v", err)
	}
	if p.WindowEnd != len(x) {
		t.Fatalf("window end %d, want the clip end %d", p.WindowEnd, len(x))
	}
}

func TestPrepareRefusals(t *testing.T) {
	silent := make([]float64, 48000)
	if _, err := Prepare(silent, 48000, PrepareOptions{}); !IsRefusal(err, RefuseNoOnset) {
		t.Fatalf("silence: err = %v, want no_onset", err)
	}
	short := modalSignal([]Mode{{Freq: 800, Alpha: 60, Amp: 0.5}}, 48000, 0.05, 1, 0.001, 4)
	if _, err := Prepare(short, 48000, PrepareOptions{}); !IsRefusal(err, RefuseShortDecay) {
		t.Fatalf("60 Np/s decay: err = %v, want short_decay", err)
	}
	clipped := modalSignal(barModes(), 48000, 0.05, 1, 0.001, 5)
	for i := 2400; i < 2410; i++ {
		clipped[i] = 1
	}
	if _, err := Prepare(clipped, 48000, PrepareOptions{}); !IsRefusal(err, RefuseClipped) {
		t.Fatalf("clipped: err = %v, want clipped", err)
	}
}

func TestPreparePitch(t *testing.T) {
	harm := []Mode{{Freq: 220, Alpha: 1.5, Amp: 0.4}, {Freq: 440, Alpha: 2, Amp: 0.2}, {Freq: 660, Alpha: 3, Amp: 0.1}}
	x := modalSignal(harm, 48000, 0.05, 2, 0.0005, 6)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Pitch.Pitched || p.Pitch.Source != "detected" || math.Abs(p.Pitch.F0Hz-220) > 2 {
		t.Fatalf("pitch %+v, want detected 220 Hz", p.Pitch)
	}
	p, err = Prepare(x, 48000, PrepareOptions{F0Hz: 110})
	if err != nil {
		t.Fatal(err)
	}
	if p.Pitch.F0Hz != 110 || p.Pitch.Source != "flag" {
		t.Fatalf("flag pitch %+v, want 110 from flag", p.Pitch)
	}
}

func TestPrepareFileKeepsRate(t *testing.T) {
	x := modalSignal(barModes(), 44100, 0.05, 1.5, 0.001, 7)
	f32 := make([]float32, len(x))
	for i, v := range x {
		f32[i] = float32(v)
	}
	b, err := wavgen.EncodeWAV(f32, 44100)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bar.wav")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareFile(path, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Rate != 44100 {
		t.Fatalf("rate %d, want the file's own 44100", p.Rate)
	}
}

func TestSynthesizeMatchesClosedForm(t *testing.T) {
	m := Mode{Freq: 1000, Alpha: 3, Amp: 0.5, Phase: 0.4}
	got := Synthesize([]Mode{m}, 48000, 48000)
	for _, i := range []int{0, 1, 4800, 47999} {
		tt := float64(i) / 48000
		want := m.Amp * math.Exp(-m.Alpha*tt) * math.Cos(2*math.Pi*m.Freq*tt+m.Phase)
		if math.Abs(got[i]-want) > 1e-9 {
			t.Fatalf("sample %d = %v, want %v", i, got[i], want)
		}
	}
}
