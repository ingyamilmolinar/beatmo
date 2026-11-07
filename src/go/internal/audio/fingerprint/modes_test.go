package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// 1215 Hz is 2.76×, a free bar's second mode; equal starting amplitudes.
func twoDecayingModes(sr int, dur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		a1 := math.Pow(10, -20*t/20)
		a2 := math.Pow(10, -60*t/20)
		s[i] = 0.4*a1*math.Sin(2*math.Pi*440*t) + 0.4*a2*math.Sin(2*math.Pi*1215*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestAnalyzeModes_FindsTwoModesWithRatioAndDecay(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 2.0), "bar")
	ms := b.Modes()
	strong := ms.Strong(20)
	if len(strong) != 2 {
		t.Fatalf("want 2 strong modes, got %d: %+v", len(strong), strong)
	}
	if math.Abs(strong[0].Hz-440) > 3 || math.Abs(strong[1].Hz-1215) > 6 {
		t.Fatalf("mode frequencies off: %.1f %.1f", strong[0].Hz, strong[1].Hz)
	}
	if r := ms.Ratios(2); math.Abs(r[1]-2.76) > 0.03 {
		t.Fatalf("ratio want 2.76, got %.3f", r[1])
	}
	if !strong[0].DecayOK || math.Abs(strong[0].DecayDBPerSec-(-20)) > 4 {
		t.Fatalf("mode 1 decay want -20 dB/s, got %.1f ok=%v", strong[0].DecayDBPerSec, strong[0].DecayOK)
	}
	if !strong[1].DecayOK || math.Abs(strong[1].DecayDBPerSec-(-60)) > 8 {
		t.Fatalf("mode 2 decay want -60 dB/s, got %.1f", strong[1].DecayDBPerSec)
	}
	if ms.AnchorHz() < 437 || ms.AnchorHz() > 443 {
		t.Fatalf("anchor want 440, got %.1f", ms.AnchorHz())
	}
}

// A 4 kHz partial drifting ±0.3 % at 1 Hz (phase-integrated FM), as a bell's upper partials do: the link tolerance
// must follow it as ONE mode rather than fragment it.
func fundamentalPlusDriftingPartial(sr int, dur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	phaseHigh := 0.0
	for i := range s {
		t := float64(i) / float64(sr)
		fHigh := 4000 * (1 + 0.003*math.Sin(2*math.Pi*1*t))
		s[i] = 0.4*math.Sin(2*math.Pi*440*t) + 0.4*math.Sin(phaseHigh)
		phaseHigh += 2 * math.Pi * fHigh / float64(sr)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestAnalyzeModes_DriftingPartialTracksAsOneMode(t *testing.T) {
	b := NewBundle(fundamentalPlusDriftingPartial(44100, 2.0), "drift")
	ms := b.Modes()
	strong := ms.Strong(20)
	if len(strong) != 2 {
		t.Fatalf("want 2 strong modes (fundamental + one unbroken drifting partial), got %d: %+v", len(strong), strong)
	}
	hi := strong[1]
	if math.Abs(hi.Hz-4000) > 20 {
		t.Fatalf("drifting partial mean Hz off: want ~4000, got %.1f", hi.Hz)
	}
	tracked := 0
	for _, v := range hi.Track {
		if !math.IsNaN(v) {
			tracked++
		}
	}
	if tracked < len(hi.Track)*9/10 {
		t.Fatalf("drifting partial fragmented: only %d/%d frames tracked in its single Mode entry", tracked, len(hi.Track))
	}
}

func TestAnalyzeModes_NoiseHasNoStableModes(t *testing.T) {
	b := NewBundle(wave.Noise(7, 0.3, 1.5, 44100), "noise")
	if RequiresModes(b) && len(b.Modes().Strong(20)) > 6 {
		t.Fatalf("white noise should not yield many stable strong modes, got %d", len(b.Modes().Strong(20)))
	}
}

func TestAnalyzeModes_SilenceHasNoModes(t *testing.T) {
	b := NewBundle(wave.Silence(1.0, 44100), "silent")
	if RequiresModes(b) {
		t.Fatal("silence must not satisfy RequiresModes")
	}
	if b.Modes().Anchor != -1 {
		t.Fatalf("anchor want -1, got %d", b.Modes().Anchor)
	}
}

func TestModes_MemoizedOnce(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 1.0), "bar")
	p1 := b.Modes()
	p2 := b.Modes()
	if p1 != p2 {
		t.Fatal("Modes() must return the memoized pointer")
	}
}

// A beating partial carries an isolated NaN mid-decay; frames before it fall at -20 dB/s and after at -100 dB/s, so a
// fit stopping at the gap reads about -20 and one using the whole track far steeper.
func TestBuildMode_DecayFitSkipsGapFrames(t *testing.T) {
	const (
		hop       = 0.01
		numFrames = 12
		gapFrame  = 3
	)
	// dB per frame: shallow before the gap, steep after it.
	db := make([]float64, numFrames)
	for f := 0; f < numFrames; f++ {
		if f <= gapFrame {
			db[f] = -10 - 0.2*float64(f) // -20 dB/s at hop=0.01
		} else {
			db[f] = -10.6 - 1.0*float64(f-gapFrame) // -100 dB/s
		}
	}

	tr := &modeTrack{}
	for f := 0; f < numFrames; f++ {
		if f == gapFrame {
			continue // the bridged frame: no measurement was ever recorded
		}
		tr.frames = append(tr.frames, f)
		tr.freqs = append(tr.freqs, 440)
		tr.amps = append(tr.amps, dbToAmpForTest(db[f]))
	}

	m := buildMode(tr, numFrames, hop)
	if !m.DecayOK {
		t.Fatal("decay fit did not run at all")
	}
	if !math.IsNaN(m.Track[gapFrame]) {
		t.Errorf("the bridged frame must stay NaN (nothing is interpolated), got %v", m.Track[gapFrame])
	}
	// The old break-at-NaN behaviour fits only frames 0..2 and lands near -20.
	if m.DecayDBPerSec > -50 {
		t.Errorf("decay fit = %.1f dB/s — it stopped at the gap instead of skipping it "+
			"(frames after the gap fall at -100 dB/s)", m.DecayDBPerSec)
	}
	// With all 11 measured frames the slope is steeper than -60 dB/s.
	if m.DecayDBPerSec < -110 {
		t.Errorf("decay fit = %.1f dB/s — implausibly steep; the fit should span the whole track", m.DecayDBPerSec)
	}
}

// Below the -40 dB floor the track reads noise, not decay.
func TestBuildMode_DecayFitStillStopsAtTheFloor(t *testing.T) {
	const hop = 0.01
	tr := &modeTrack{}
	// Clean decay, then a plunge whose frames would flatten the slope if fitted.
	levels := []float64{-10, -20, -30, -39, -95, -95, -95, -95}
	for f, l := range levels {
		tr.frames = append(tr.frames, f)
		tr.freqs = append(tr.freqs, 440)
		tr.amps = append(tr.amps, dbToAmpForTest(l))
	}
	m := buildMode(tr, len(levels), hop)
	if !m.DecayOK {
		t.Fatal("decay fit did not run")
	}
	// Frames 0..3 alone slope at about -970 dB/s.
	if m.DecayDBPerSec < -1100 {
		t.Errorf("decay fit = %.1f dB/s — the below-floor frames were fitted", m.DecayDBPerSec)
	}
}

// All levels sit above the floor.
func dbToAmpForTest(db float64) float64 { return math.Pow(10, db/20) }
