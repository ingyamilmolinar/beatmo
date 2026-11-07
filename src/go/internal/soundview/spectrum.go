package soundview

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Spectrum is the note's tone colour: average power over its audible part on
// a log-frequency axis (20 Hz..20 kHz, 1/6-octave smoothing), in dB relative
// to its own maximum (0 at the peak), floored at -60.
type Spectrum struct {
	Hz []float64
	DB []float64
	// Curves share one scale by shifting each by its RefDB: a quieter note sits lower, never re-normalised.
	RefDB float64
}

// Cols short-time spectra on Rows log bands, dB re the loudest cell, floored at -60; CentroidHz per column (0 when
// silent).
type Spectrogram struct {
	Cols, Rows     int
	T0, HopSec     float64
	FreqLo, FreqHi float64
	DB             []float64 // column-major: DB[c*Rows + r]
	CentroidHz     []float64
}

// At returns cell (c, r) in dB.
func (s Spectrogram) At(c, r int) float64 { return s.DB[c*s.Rows+r] }

// RowHz is band r's centre frequency.
func (s Spectrogram) RowHz(r int) float64 {
	if s.Rows <= 1 {
		return s.FreqLo
	}
	return s.FreqLo * math.Pow(s.FreqHi/s.FreqLo, float64(r)/float64(s.Rows-1))
}

// NoiseSplit is tone vs noise over time: per STFT hop, the energy in spectral
// peaks (Tonal) and the rest (Noise), linear power; Fraction is the noise
// share over the audible part (0..1).
type NoiseSplit struct {
	T0, HopSec float64
	Tonal      []float64
	Noise      []float64
	Fraction   float64
}

const (
	specFloorDB    = -60.0
	specPoints     = 240
	specLoHz       = 20.0
	specHiHz       = 20000.0
	sgRows         = 64
	sgLoHz         = 30.0
	sgHiHz         = 16000.0
	noisePeakDB    = 10.0 // a tonal peak stands 10 dB over its neighbourhood
	noiseFloorBins = 16   // the neighbourhood: ±16 bins
	noiseLobeBins  = 2    // a Hann main lobe: ±2 bins
)

// Spectrum returns the average tone colour.
func (a *Analysis) Spectrum() Spectrum {
	a.stftOnce.Do(a.computeSTFT)
	return a.spec
}

// CentroidHz is the brightness centre: the power-weighted mean frequency of
// the average spectrum. 0 for silence.
func (a *Analysis) CentroidHz() float64 {
	a.stftOnce.Do(a.computeSTFT)
	return a.centroid
}

// Spectrogram returns tone colour over time.
func (a *Analysis) Spectrogram() Spectrogram {
	a.stftOnce.Do(a.computeSTFT)
	return a.sg
}

// Noise returns tone vs noise over time.
func (a *Analysis) Noise() NoiseSplit {
	a.stftOnce.Do(a.computeSTFT)
	return a.noise
}

// NEAREST, not up: rounding up gave 4096 at 48 kHz, and the platforms read different noise shares for one sound.
func stftSize(sr int) int {
	target := float64(sr) * 0.0427
	n := 1
	for float64(n) < target {
		n <<= 1
	}
	if n > 1 && float64(n)-target > target-float64(n/2) {
		n /= 2
	}
	return n
}

// ONE transform (Hann, ~43 ms, hop a quarter) feeds the spectrum, spectrogram and tone/noise split, so frames are
// never kept.
func (a *Analysis) computeSTFT() {
	defer a.markDone(NeedSTFT)
	size := stftSize(a.sr)
	hop := size / 2
	bins := size/2 + 1
	binHz := float64(a.sr) / float64(size)
	// Column f is CENTRED at f*hop (its frame starts at f*hop - size/2), so
	// T0 = 0 and the columns tile the buffer.
	nFrames := (a.analysedLen() + hop - 1) / hop
	a.sg = Spectrogram{Cols: nFrames, Rows: sgRows, HopSec: float64(hop) / float64(a.sr),
		FreqLo: sgLoHz, FreqHi: sgHiHz, DB: make([]float64, nFrames*sgRows), CentroidHz: make([]float64, nFrames)}
	a.noise = NoiseSplit{HopSec: a.sg.HopSec, Tonal: make([]float64, nFrames), Noise: make([]float64, nFrames)}
	if nFrames == 0 {
		a.spec = logSpectrum(nil, binHz)
		return
	}
	win := wave.HannWindow(size)
	buf := make([]complex128, size)
	pw := make([]float64, bins)
	avg := make([]float64, bins)
	logp := make([]float64, bins)
	tonal := make([]bool, bins)
	edges := bandEdges(sgRows, sgLoHz, sgHiHz)
	on := a.Onsets()
	start := 0.0
	if len(on) > 0 {
		start = on[0]
	}
	end := a.AudibleSec()
	maxCell := 0.0
	cells := make([]float64, nFrames*sgRows)
	var noiseSum, totalSum float64
	avgN := 0
	for f := 0; f < nFrames; f++ {
		a.yieldEvery(f, 2)
		lo := f*hop - size/2
		for i := range buf {
			j := lo + i
			v := 0.0
			if j >= 0 && j < len(a.pcm) {
				v = float64(a.pcm[j]) * win[i]
			}
			buf[i] = complex(v, 0)
		}
		wave.FFT(buf)
		var e, fe float64
		for k := 0; k < bins; k++ {
			re, im := real(buf[k]), imag(buf[k])
			pw[k] = re*re + im*im
			e += pw[k]
			fe += pw[k] * float64(k) * binHz
		}
		if e > 0 {
			a.sg.CentroidHz[f] = fe / e
		}
		at := float64(f*hop) / float64(a.sr)
		if at >= start-0.01 && at <= end+0.01 {
			for k := range avg {
				avg[k] += pw[k]
			}
			avgN++
		}
		for r := 0; r < sgRows; r++ {
			c := bandPower(pw, binHz, edges[r], edges[r+1])
			cells[f*sgRows+r] = c
			maxCell = math.Max(maxCell, c)
		}
		// Tone vs noise: a local maximum standing noisePeakDB over the
		// geometric mean of its neighbourhood marks its main lobe tonal.
		for k := range logp {
			logp[k] = math.Log(pw[k] + 1e-30)
			tonal[k] = false
		}
		var ton float64
		for k := 1; k < bins-1; k++ {
			if pw[k] < pw[k-1] || pw[k] < pw[k+1] {
				continue
			}
			lo, hi := max(0, k-noiseFloorBins), min(bins-1, k+noiseFloorBins)
			s := 0.0
			for j := lo; j <= hi; j++ {
				s += logp[j]
			}
			floor := math.Exp(s / float64(hi-lo+1))
			if pw[k] > floor*math.Pow(10, noisePeakDB/10) {
				for j := max(0, k-noiseLobeBins); j <= min(bins-1, k+noiseLobeBins); j++ {
					tonal[j] = true
				}
			}
		}
		for k := range pw {
			if tonal[k] {
				ton += pw[k]
			}
		}
		a.noise.Tonal[f] = ton
		a.noise.Noise[f] = e - ton
		if at >= start-0.01 && at <= end+0.01 {
			noiseSum += e - ton
			totalSum += e
		}
	}
	for i, c := range cells {
		if maxCell > 0 && c > 0 {
			a.sg.DB[i] = math.Max(specFloorDB, 10*math.Log10(c/maxCell))
		} else {
			a.sg.DB[i] = specFloorDB
		}
	}
	if totalSum > 0 {
		a.noise.Fraction = noiseSum / totalSum
	}
	var e, fe float64
	for k, p := range avg {
		e += p
		fe += p * float64(k) * binHz
	}
	if e > 0 {
		a.centroid = fe / e
	}
	if avgN > 0 {
		for k := range avg {
			avg[k] /= float64(avgN)
		}
	}
	a.spec = logSpectrum(avg, binHz)
}

// bandEdges returns rows+1 geometric band edges around rows log-spaced centres.
func bandEdges(rows int, lo, hi float64) []float64 {
	ratio := math.Pow(hi/lo, 1/float64(rows-1))
	edges := make([]float64, rows+1)
	for r := 0; r <= rows; r++ {
		edges[r] = lo * math.Pow(ratio, float64(r)-0.5)
	}
	return edges
}

// bandPower is the mean power of the bins inside [lo, hi) Hz, or the power
// interpolated at the band's centre when no bin falls inside (low bands).
func bandPower(pw []float64, binHz, lo, hi float64) float64 {
	a, b := int(math.Ceil(lo/binHz)), int(math.Floor(hi/binHz))
	if b >= len(pw) {
		b = len(pw) - 1
	}
	if a <= b {
		s := 0.0
		for k := a; k <= b; k++ {
			s += pw[k]
		}
		return s / float64(b-a+1)
	}
	return interpPower(pw, math.Sqrt(lo*hi)/binHz)
}

func interpPower(pw []float64, x float64) float64 {
	if len(pw) == 0 {
		return 0
	}
	i := int(x)
	if i < 0 {
		return pw[0]
	}
	if i >= len(pw)-1 {
		return pw[len(pw)-1]
	}
	f := x - float64(i)
	return pw[i]*(1-f) + pw[i+1]*f
}

// logSpectrum resamples a linear power spectrum onto specPoints log-spaced
// frequencies with 1/6-octave smoothing, in dB re its maximum.
func logSpectrum(pw []float64, binHz float64) Spectrum {
	s := Spectrum{Hz: make([]float64, specPoints), DB: make([]float64, specPoints)}
	ratio := math.Pow(specHiHz/specLoHz, 1/float64(specPoints-1))
	half := math.Pow(2, 1.0/12)
	vals := make([]float64, specPoints)
	mx := 0.0
	for i := range s.Hz {
		f := specLoHz * math.Pow(ratio, float64(i))
		s.Hz[i] = f
		if len(pw) > 0 {
			vals[i] = bandPower(pw, binHz, f/half, f*half)
			mx = math.Max(mx, vals[i])
		}
	}
	for i, v := range vals {
		if mx > 0 && v > 0 {
			s.DB[i] = math.Max(specFloorDB, 10*math.Log10(v/mx))
		} else {
			s.DB[i] = specFloorDB
		}
	}
	s.RefDB = powDB(mx)
	return s
}

// --- harmonics ---

// LINEAR axis 0..12·F0 so harmonics land on even ticks; dB re its maximum, and harmonics 1..12 (interpolated).
type Harmonics struct {
	F0    float64
	Hz    []float64
	DB    []float64
	Peaks []float64
}

const (
	harmCount       = 12
	harmPoints      = 480
	harmMinVoiced   = 0.3
	harmPeakFloorDB = -60.0
)

// Harmonics returns the harmonic ladder, ok=false when the note has no clear
// pitch (noise, a cymbal, silence).
func (a *Analysis) Harmonics() (Harmonics, bool) {
	a.harmOnce.Do(a.computeHarmonics)
	return a.harm, a.harmOK
}

func (a *Analysis) computeHarmonics() {
	defer a.markDone(NeedHarmonics)
	tr := a.Pitch()
	f0 := tr.MedianHz()
	if f0 <= 0 || tr.VoicedFraction() < harmMinVoiced*nonSilentFraction(a) {
		return
	}
	n := 4096
	for float64(n) < 8*float64(a.sr)/f0 && n < 32768 {
		n <<= 1
	}
	start := 0
	if on := a.Onsets(); len(on) > 0 {
		start = int(on[0] * float64(a.sr))
	}
	avail := min(n, len(a.pcm)-start)
	if avail < n/4 {
		return
	}
	buf := make([]complex128, n)
	for i := 0; i < avail; i++ {
		w := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(avail)))
		buf[i] = complex(float64(a.pcm[start+i])*w, 0)
	}
	wave.FFT(buf)
	bins := n/2 + 1
	binHz := float64(a.sr) / float64(n)
	pw := make([]float64, bins)
	for k := range pw {
		re, im := real(buf[k]), imag(buf[k])
		pw[k] = re*re + im*im
	}
	h := Harmonics{F0: f0, Hz: make([]float64, harmPoints), DB: make([]float64, harmPoints), Peaks: make([]float64, harmCount)}
	top := float64(harmCount) * f0
	mx := 0.0
	vals := make([]float64, harmPoints)
	for i := range vals {
		f := top * float64(i) / float64(harmPoints-1)
		h.Hz[i] = f
		vals[i] = interpPower(pw, f/binHz)
		mx = math.Max(mx, vals[i])
	}
	peakDB := make([]float64, harmCount)
	pmx := math.Inf(-1)
	for k := 1; k <= harmCount; k++ {
		c := float64(k) * f0 / binHz
		lo, hi := int(c-0.25*f0/binHz), int(c+0.25*f0/binHz)+1
		best := -1
		for j := max(1, lo); j <= min(bins-2, hi); j++ {
			if best < 0 || pw[j] > pw[best] {
				best = j
			}
		}
		if best < 0 {
			peakDB[k-1] = math.Inf(-1)
			continue
		}
		// Parabolic interpolation in dB removes the Hann scalloping loss.
		y0, y1, y2 := powDB(pw[best-1]), powDB(pw[best]), powDB(pw[best+1])
		pk := y1
		if den := y0 - 2*y1 + y2; den < 0 {
			d := 0.5 * (y0 - y2) / den
			pk = y1 - 0.25*(y0-y2)*d
		}
		peakDB[k-1] = pk
		pmx = math.Max(pmx, pk)
	}
	for i, v := range vals {
		if mx > 0 && v > 0 {
			h.DB[i] = math.Max(specFloorDB, 10*math.Log10(v/mx))
		} else {
			h.DB[i] = specFloorDB
		}
	}
	for k := range peakDB {
		h.Peaks[k] = math.Max(harmPeakFloorDB, peakDB[k]-pmx)
	}
	a.harm, a.harmOK = h, true
}

// nonSilentFraction is the share of pitch frames that are loud enough to be
// judged at all — a short hit is voiced over its sounding part only.
func nonSilentFraction(a *Analysis) float64 {
	tr := a.Pitch()
	if len(tr.Hz) == 0 {
		return 1
	}
	env := a.Envelope()
	mx := env.MaxRMS()
	n := 0
	for i := range tr.Hz {
		if env.RMSAt(tr.T0+float64(i)*tr.HopSec) >= mx-pitchQuietDrop {
			n++
		}
	}
	return float64(n) / float64(len(tr.Hz))
}

// A steady f0 over enough of the sounding part. A drum hit, cymbal or noise is not, so the views fall back to tone
// colour (a close-up would show only noise).
func (a *Analysis) Pitched() bool {
	tr := a.Pitch()
	return tr.MedianHz() > 0 && tr.VoicedFraction() >= harmMinVoiced*nonSilentFraction(a)
}
