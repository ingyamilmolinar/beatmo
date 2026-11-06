package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// twoDecayingModes: 440 Hz decaying at -20 dB/s and 1215 Hz (2.76x, a free
// bar's second mode) decaying at -60 dB/s. Both start at equal amplitude.
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

// fundamentalPlusDriftingPartial: a stable 440 Hz fundamental plus a 4 kHz
// partial with slow +/-0.3% pitch drift at a 1 Hz rate (sinusoidal FM,
// phase-integrated sample-by-sample so the modulation is clean rather than a
// discontinuous approximation). This is the regime that motivates modes.go
// in the first place — a bell/bar's upper partials drift slightly rather
// than holding an exact bin-locked frequency — and the frame-link tolerance
// must be loose enough to follow that drift as ONE mode without fragmenting
// it into several sub-modeMinFrames tracks that then vanish from Strong().
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

// TestBuildMode_DecayFitSkipsGapFrames is the regression for the interaction
// between modeMaxMissFrames (the tracker's one-frame gap tolerance) and the
// decay fit.
//
// A beating partial's composite peak dips below the peak-picker's gate at
// each amplitude null, so its Track carries an isolated NaN mid-decay. The
// fit used to `break` at that NaN, discarding every frame after it — exactly
// the frames the tracker had gone to the trouble of keeping. The fitted rate
// therefore came from the handful of frames before the first null.
//
// The construction below makes the two answers unmistakably different: the
// frames BEFORE the gap fall at -20 dB/s, the frames AFTER it at -100 dB/s.
// A fit that stops at the gap reads about -20; a fit that uses the whole
// track reads far steeper. Nothing is interpolated — the gap frame
// contributes no sample at all.
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
	// And it really used the post-gap frames: with all 11 measured frames the
	// OLS slope over this piecewise track is steeper than -60 dB/s.
	if m.DecayDBPerSec < -110 {
		t.Errorf("decay fit = %.1f dB/s — implausibly steep; the fit should span the whole track", m.DecayDBPerSec)
	}
}

// TestBuildMode_DecayFitStillStopsAtTheFloor: skipping gaps must not have
// removed the -40 dB floor, which is the fit's real ending — below it the
// level track is reading the noise floor, not a decay.
func TestBuildMode_DecayFitStillStopsAtTheFloor(t *testing.T) {
	const hop = 0.01
	tr := &modeTrack{}
	// Four frames of clean decay, then a plunge far below the floor followed
	// by frames that would flatten the slope if they were fitted.
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
	// Frames 0..3 alone slope at about -970 dB/s. Including the -95 frames
	// would drag it toward -1400 and beyond.
	if m.DecayDBPerSec < -1100 {
		t.Errorf("decay fit = %.1f dB/s — the below-floor frames were fitted", m.DecayDBPerSec)
	}
}

// dbToAmpForTest inverts ampToDBFloor for the levels these tests use (all
// comfortably above the floor).
func dbToAmpForTest(db float64) float64 { return math.Pow(10, db/20) }
