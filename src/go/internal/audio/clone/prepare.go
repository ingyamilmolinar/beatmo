package clone

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// PitchInfo is the nominal pitch the mode ratios are expressed against.
type PitchInfo struct {
	Pitched     bool    `json:"pitched"`
	F0Hz        float64 `json:"f0_hz"`
	Source      string  `json:"source"` // "flag" | "detected" | "none"
	Confidence  float64 `json:"confidence"`
	SemisFromA3 int     `json:"semis_from_a3"`
	HasSemis    bool    `json:"has_semis"`
}

// PrepareOptions carries what the caller knows about the reference.
type PrepareOptions struct {
	F0Hz        float64 // explicit nominal pitch (-f0); wins over detection
	SemisFromA3 int     // the catalogue's measured note, recorded for rendering
	HasSemis    bool
	Label       string
}

// Prepared is a reference ready for analysis.
type Prepared struct {
	Label       string
	Samples     []float64 // mono, the whole file, its own rate
	Rate        int
	Onset       int     // sample index
	FloorRMS    float64 // linear
	Peak        float64
	RMS400DB    float64 // RMS of the first 400 ms after the onset, dBFS
	WindowStart int     // decay-analysis window [WindowStart, WindowEnd), sample indices
	WindowEnd   int
	Pitch       PitchInfo
}

const (
	minDecayWindowSec = 0.150
	clipLevel         = 0.999
	clipMinSamples    = 3
)

// PrepareFile decodes a WAV (any channel count, folded to mono) and prepares it.
func PrepareFile(path string, opt PrepareOptions) (Prepared, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Prepared{}, err
	}
	pcm, rate, err := wavgen.DecodeWAV(b)
	if err != nil {
		return Prepared{}, fmt.Errorf("clone: decode %s: %w", path, err)
	}
	x := make([]float64, len(pcm))
	for i, v := range pcm {
		x[i] = float64(v)
	}
	if opt.Label == "" {
		opt.Label = path
	}
	return Prepare(x, rate, opt)
}

func Prepare(samples []float64, rate int, opt PrepareOptions) (Prepared, error) {
	p := Prepared{Label: opt.Label, Samples: samples, Rate: rate}
	clipped := 0
	for _, v := range samples {
		a := math.Abs(v)
		if a >= clipLevel {
			clipped++
		}
		if a > p.Peak {
			p.Peak = a
		}
	}
	if clipped >= clipMinSamples {
		return p, refuse(RefuseClipped, "%d samples at or above %.3f full scale", clipped, clipLevel)
	}
	fluxFrame := fluxPeakFrame(samples, rate)
	p.FloorRMS = noiseFloor(samples, rate, fluxFrame*onsetHop-onsetFrame)
	if p.Peak == 0 || p.Peak <= 10*p.FloorRMS {
		return p, refuse(RefuseNoOnset, "nothing rises 20 dB above the noise floor")
	}
	// Floor + 20 dB and at least −40 dB re the peak (a lower bar lets pre-roll noise fire it early, inflating every fast
	// mode's back-extrapolation), never above −20 dB.
	thr := math.Min(math.Max(10*p.FloorRMS, 0.01*p.Peak), 0.1*p.Peak)
	onset, ok := refineOnset(samples, fluxFrame, thr)
	if !ok {
		return p, refuse(RefuseNoOnset, "no sample above floor+20 dB near the spectral-flux peak")
	}
	p.Onset = onset
	p.RMS400DB = dB(rms(samples[onset:min(len(samples), onset+int(0.4*float64(rate)))]))

	// 5 ms after the 2 ms-RMS envelope first comes within 3 dB of its 50 ms maximum, not after the maximum: beating can
	// keep creeping up long after the fast modes have decayed.
	f2 := int(0.002 * float64(rate))
	var env []float64
	for k := 0; k < 25; k++ {
		a, b := onset+k*f2, onset+(k+1)*f2
		if b > len(samples) {
			break
		}
		env = append(env, rms(samples[a:b]))
	}
	best := 0.0
	for _, v := range env {
		best = math.Max(best, v)
	}
	settle := 0
	for k, v := range env {
		if v >= best/math.Sqrt2 {
			settle = k
			break
		}
	}
	p.WindowStart = onset + (settle+1)*f2 + int(0.005*float64(rate))
	// Or the clip end when it still rings: some clips clamp mid-ring.
	f10 := int(0.010 * float64(rate))
	p.WindowEnd = p.WindowStart
	for a := p.WindowStart; a+f10 <= len(samples); a += f10 {
		if rms(samples[a:a+f10]) > 2*p.FloorRMS {
			p.WindowEnd = a + f10
		}
	}
	if len(samples)-p.WindowEnd < f10 && p.WindowEnd > p.WindowStart {
		p.WindowEnd = len(samples)
	}
	if float64(p.WindowEnd-p.WindowStart)/float64(rate) < minDecayWindowSec {
		return p, refuse(RefuseShortDecay, "decay window %.0f ms < %.0f ms",
			1000*float64(p.WindowEnd-p.WindowStart)/float64(rate), 1000*minDecayWindowSec)
	}
	p.Pitch = decidePitch(samples[onset:p.WindowEnd], rate, opt)
	return p, nil
}

func decidePitch(seg []float64, rate int, opt PrepareOptions) PitchInfo {
	pi := PitchInfo{SemisFromA3: opt.SemisFromA3, HasSemis: opt.HasSemis, Source: "none"}
	if opt.F0Hz > 0 {
		pi.Pitched, pi.F0Hz, pi.Source, pi.Confidence = true, opt.F0Hz, "flag", 1
		return pi
	}
	w := wave.Wave{Samples: seg, SampleRate: rate}
	est := fingerprint.DetectF0Detailed(w)
	pi.Confidence = est.Confidence
	if ok, _ := fingerprint.UsableAsPitch(w, est); ok {
		pi.Pitched, pi.F0Hz, pi.Source = true, est.Hz, "detected"
	}
	return pi
}

// The median 10 ms RMS of ≥ 20 ms of pre-roll, else the quietest NON-ZERO 2 ms frame. Biased low on purpose: too
// high refuses the clip and cuts its window (the quietest stretch of a ring is still the ring), too low only
// lengthens it. Digital zeros are never the noise.
func noiseFloor(x []float64, rate, preEnd int) float64 {
	if f := int(0.010 * float64(rate)); preEnd >= 2*f {
		var pre []float64
		for a := 0; a+f <= preEnd; a += f {
			if r := rms(x[a : a+f]); r > 0 {
				pre = append(pre, r)
			}
		}
		if len(pre) >= 2 {
			sort.Float64s(pre)
			return pre[len(pre)/2]
		}
	}
	f := max(1, int(0.002*float64(rate)))
	floor := 0.0
	for a := 0; a+f <= len(x); a += f {
		if r := rms(x[a : a+f]); r > 0 && (floor == 0 || r < floor) {
			floor = r
		}
	}
	if sf := spectralFloor(x, rate); sf > 0 && (floor == 0 || sf < floor) {
		floor = sf
	}
	return floor
}

// Noise is broadband and a ring a handful of tones, so the MEDIAN windowed bin power is noise even mid-ring: for
// white noise of variance σ², |X_k|² is exponential with mean σ²·Σw² and median ln 2 times that.
func spectralFloor(x []float64, rate int) float64 {
	const n = 4096
	if len(x) < n {
		return 0
	}
	mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: x[len(x)-n:], SampleRate: rate}, n, wave.WindowHann)
	pw := make([]float64, 0, len(mag))
	for _, m := range mag[1 : len(mag)-1] {
		pw = append(pw, m*m)
	}
	sort.Float64s(pw)
	med := pw[len(pw)/2]
	const sumW2 = 0.375 * n // Σ Hann² ≈ 3N/8
	return math.Sqrt(med / (math.Ln2 * sumW2))
}

const onsetFrame, onsetHop = 512, 128

// Independent of any level threshold.
func fluxPeakFrame(x []float64, rate int) int {
	var prev []float64
	bestFlux, bestFrame := -1.0, 0
	for i := 0; i*onsetHop+onsetFrame <= len(x); i++ {
		seg := x[i*onsetHop : i*onsetHop+onsetFrame]
		mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: rate}, onsetFrame, wave.WindowHann)
		var flux float64
		for k, m := range mag {
			d := m
			if prev != nil {
				d = m - prev[k]
			}
			if d > 0 {
				flux += d
			}
		}
		if flux > bestFlux {
			bestFlux, bestFrame = flux, i
		}
		prev = mag
	}
	return bestFrame
}

// Searched from four hops before the flux peak frame to its end.
func refineOnset(x []float64, frame int, thr float64) (int, bool) {
	lo := max(0, (frame-4)*onsetHop)
	hi := min(len(x), frame*onsetHop+onsetFrame)
	for i := lo; i < hi; i++ {
		if math.Abs(x[i]) > thr {
			return i, true
		}
	}
	return 0, false
}

func rms(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}
