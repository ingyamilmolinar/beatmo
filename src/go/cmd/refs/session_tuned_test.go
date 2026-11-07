//go:build !js

package main

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// Starts at once and fades over its last 20 ms, like every clip the session writes.
func testTone(t *testing.T, sec float64) wave.Wave {
	t.Helper()
	pcm := wavgen.Sine(440, 0.5, sec, 44100)
	s := make([]float64, len(pcm))
	for i, v := range pcm {
		s[i] = float64(v)
	}
	fadeOutTail(s, int(matchFadeSec*44100))
	return wave.Wave{Samples: s, SampleRate: 44100}
}

func padTo(w wave.Wave, sec float64) wave.Wave {
	n := int(sec * float64(w.SampleRate))
	w.Samples = append(append([]float64(nil), w.Samples...), make([]float64, max(0, n-len(w.Samples)))...)
	return w
}

// TestClipFaultsHearBrokenPlayback: each way a clip can sound broken rather
// than like an instrument is reported, by name, and a clean clip passes.
func TestClipFaultsHearBrokenPlayback(t *testing.T) {
	if f := clipFaults("R", testTone(t, 1)); len(f) != 0 {
		t.Fatalf("a clean, faded tone reported %v", f)
	}
	cut := testTone(t, 1)
	cut.Samples = cut.Samples[:len(cut.Samples)-1000] // mid-note, like a catalogue trim
	late := testTone(t, 1)
	late.Samples = append(make([]float64, 44100/5), late.Samples...)
	gap := testTone(t, 1)
	for i := 20000; i < 21000; i++ {
		gap.Samples[i] = 0
	}
	hot := testTone(t, 1)
	for i := range hot.Samples {
		hot.Samples[i] *= 2.5
		hot.Samples[i] = min(1, max(-1, hot.Samples[i]))
	}
	cases := []struct {
		name string
		w    wave.Wave
		want string
	}{
		{"silent", wave.Wave{Samples: make([]float64, 100), SampleRate: 44100}, "is silent"},
		{"cut", cut, "ends on a step"},
		{"late", late, "starts 200 ms late"},
		{"dropout", gap, "drops out at 0.45 s"},
		{"clipped", hot, "clips"},
	}
	for _, c := range cases {
		f := clipFaults("X", c.w)
		if len(f) != 1 || !strings.Contains(f[0], c.want) {
			t.Errorf("%s: want one fault %q, got %v", c.name, c.want, f)
		}
	}
	fadeOutTail(cut.Samples, int(matchFadeSec*44100))
	if f := clipFaults("R", cut); len(f) != 0 {
		t.Errorf("fadeOutTail must remove the click: %v", f)
	}
}

// A 1 s voice padded with silence against a 2 s held recording.
func TestHoldFaultHearsAVoiceThatStopsEarly(t *testing.T) {
	ref := testTone(t, 2)
	if f := holdFault("C", ref, padTo(testTone(t, 1), 2)); !strings.Contains(f, "C is cut at 0.80 s while the recording still holds") {
		t.Fatalf("a 1 s voice against a 2 s note: %q", f)
	}
	if f := holdFault("C", ref, testTone(t, 2)); f != "" {
		t.Fatalf("a held voice: %q", f)
	}
	// A recording that has died away by then asks nothing of the voice.
	pluck := testTone(t, 2)
	for i := range pluck.Samples {
		pluck.Samples[i] *= 1 / (1 + float64(i)/2000)
	}
	if f := holdFault("C", pluck, padTo(testTone(t, 1), 2)); f != "" {
		t.Fatalf("against a decayed pluck: %q", f)
	}
	// A voice that decays faster than a held recording -- a pluck tuned
	// shorter, -40 dB by 1.5 s -- is timbre, not a cut.
	fast := testTone(t, 2)
	for i := range fast.Samples {
		fast.Samples[i] *= math.Pow(10, -40.0/20*float64(i)/(1.5*44100))
	}
	if f := holdFault("T", ref, fast); f != "" {
		t.Fatalf("a faster decay: %q", f)
	}
}

// A blocking player returns the moment a clip ends, so R, C and T ran together.
func TestGappedPlayerPausesAfterEveryClip(t *testing.T) {
	var log []string
	p := gappedPlayer{p: funcPlayer(func(path string) error { log = append(log, "play "+path); return nil }),
		gap: clipGap, sleep: func(d time.Duration) { log = append(log, "gap "+d.String()) }}
	rp := &replayer{p: p, out: &strings.Builder{}, clips: sessionClips{refPath: "R.wav", curPath: "C.wav", candPaths: map[string]string{"T": "T.wav"}}}
	rp.play([]string{"R", "C", "T"})
	want := []string{"play R.wav", "gap 600ms", "play C.wav", "gap 600ms", "play T.wav", "gap 600ms"}
	if !slices.Equal(log, want) {
		t.Fatalf("played %v, want %v", log, want)
	}
	t.Setenv("BEATMO_PLAYER", "true")
	if g, ok := defaultPlayer(1).(gappedPlayer); !ok || g.gap != clipGap {
		t.Fatalf("a blocking player must be gapped, got %#v", defaultPlayer(1))
	}
}

// Every sample lands at ±0.707 of the true peak: the textbook inter-sample over.
func fs4Sine(amp float64, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = amp * math.Sin(math.Pi/2*float64(i)+math.Pi/4)
	}
	return s
}

// The DAC and every resampler reconstruct between samples: a 0.99 sample peak can swing past full scale.
func TestTruePeakSeesBetweenTheSamples(t *testing.T) {
	s := fs4Sine(1, 4410)
	if got := truePeak(s[100:4300]); math.Abs(got-1) > 0.02 {
		t.Fatalf("true peak of a 0.707-sampled unit sine: %.3f, want 1.0", got)
	}
	tone := testTone(t, 0.5)
	var sp float64
	for _, v := range tone.Samples {
		sp = math.Max(sp, math.Abs(v))
	}
	if got := truePeak(tone.Samples); math.Abs(got-sp) > 0.01*sp {
		t.Fatalf("a well-sampled 440 Hz tone: true peak %.4f vs sample peak %.4f", got, sp)
	}
}

// References driven to a 0.99 SAMPLE peak came out of the DAC clipped: the "noisy, vibrating reference".
func TestClipFaultsHearAnInterSampleOver(t *testing.T) {
	s := fs4Sine(0.8, 44100) // samples at 0.57, true peak 0.80: inside the headroom
	fadeOutTail(s, 882)
	if f := clipFaults("R", wave.Wave{Samples: s, SampleRate: 44100}); len(f) != 0 {
		t.Fatalf("a 0.80 true peak is inside the -1 dBTP headroom, got %v", f)
	}
	s = fs4Sine(0.99*math.Sqrt2, 44100) // samples at 0.99, true peak 1.40
	fadeOutTail(s, 882)
	f := clipFaults("R", wave.Wave{Samples: s, SampleRate: 44100})
	if len(f) != 1 || !strings.Contains(f[0], "true peak") {
		t.Fatalf("a 0.99 sample peak swinging to 1.40 between samples must be reported, got %v", f)
	}
}

// The SYNTHS move to the recording, never the reverse: raising a quiet recording raised its hiss into the DAC's clip.
func TestTunedLevelsKeepTheReferenceAtItsOwnLevel(t *testing.T) {
	// A quiet recording (rms 0.02, true peak 0.11) against two loud,
	// peak-normalised voices.
	g := tunedLevelGains([]float64{0.02, 0.3, 0.25}, []float64{0.11, 0.95, 0.9})
	if g[0] != 1 {
		t.Fatalf("the recording must play at its own level, gain %v", g[0])
	}
	if math.Abs(g[1]*0.3-0.02) > 1e-12 || math.Abs(g[2]*0.25-0.02) > 1e-12 {
		t.Fatalf("the voices must land at the recording's loudness: gains %v", g)
	}
	// A voice so peaky that matching it would breach the headroom pulls the
	// whole set down together -- the recording is attenuated, never raised.
	g = tunedLevelGains([]float64{0.2, 0.05}, []float64{0.7, 0.9})
	if g[0] >= 1 || math.Abs(g[1]*0.05-g[0]*0.2) > 1e-12 {
		t.Fatalf("equal loudness kept by scaling everything down: %v", g)
	}
	if tp := g[1] * 0.9; tp > clipHeadroom+1e-12 {
		t.Fatalf("voice true peak %.3f past the headroom %.3f", tp, clipHeadroom)
	}
}

// The onset trim cut references mid-rise, a click from silence that headphones expose; a clip whose first sample IS
// its attack (a rimshot) is the voice.
func TestClipFaultsHearAStepAtTheStart(t *testing.T) {
	tone := testTone(t, 1)
	cut := wave.Wave{Samples: append([]float64(nil), tone.Samples[30:]...), SampleRate: 44100}
	for i := range cut.Samples[:400] { // a slow rise the trim cut into
		cut.Samples[i] *= 0.05
	}
	if f := clipFaults("R", cut); len(f) != 1 || !strings.Contains(f[0], "starts on a step") {
		t.Fatalf("a clip cut mid-rise: %v", f)
	}
	fadeInLead(cut.Samples, 44100)
	if f := clipFaults("R", cut); len(f) != 0 {
		t.Fatalf("fadeInLead must remove the step: %v", f)
	}
	hit := testTone(t, 0.3)
	hit.Samples = hit.Samples[25:] // a quarter period in: the first sample is the crest, the attack itself
	for i := range hit.Samples {
		hit.Samples[i] *= math.Exp(-float64(i) / 2000)
	}
	before := append([]float64(nil), hit.Samples...)
	fadeInLead(hit.Samples, 44100)
	if !slices.Equal(before, hit.Samples) {
		t.Fatal("fadeInLead must leave a voice whose attack is its first sample untouched")
	}
	if f := clipFaults("C", hit); len(f) != 0 {
		t.Fatalf("an attack on the first sample is the voice: %v", f)
	}
}
