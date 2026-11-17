package soundview

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The AUDIBILITY ORACLE: does a change to an instrument sound different?
// It is the single judge behind the Synth tab's dial relevance (a dial whose
// full sweep is inaudible on an instrument is hidden there), the relevance
// generator and the dial contract tests. Phase-insensitive on purpose: a
// sample diff calls a −90 dB change "different".

// Thresholds of the oracle (just-noticeable differences).
const (
	// AudibleMinDB: whole-note mean |dB| of the third-octave band-energy
	// envelope (23 ms frames) over every cell within 60 dB of the louder
	// render's peak. ~1 dB is the JND for a level / spectral-balance change.
	AudibleMinDB = 1.0
	// AudibleLocalDB over AudibleLocalFrames (~70 ms): a change confined to
	// part of the note (a slower fade-in, a longer tail, a sharper onset) is
	// plainly audible yet barely moves the whole-note mean.
	AudibleLocalDB     = 2.0
	AudibleLocalFrames = 3
	// AudibleBandDB: ONE third-octave band within AudibleBandFloorDB of the
	// loudest moving this much over the note — a resonance, a formant, a
	// singer's ring averages out of the whole-spectrum mean.
	AudibleBandDB      = 3.0
	AudibleBandFloorDB = 30.0
	// AudibleMinCents: the strongest partial shifting this much on average
	// (a pitch bend moves partials across a band without changing its energy).
	AudibleMinCents = 5.0
	// SilentPeak is the sample peak under which a render counts as silence
	// (−80 dBFS): every voice is peak-normalised, so anything a listener could
	// hear comes back near full scale; only an all-but-zero render stays here.
	SilentPeak = 1e-4
)

// Change is how differently two renders of a note sound.
type Change struct {
	GlobalDB float64 // whole-note mean |dB| band-envelope difference
	LocalDB  float64 // the largest mean over AudibleLocalFrames frames
	BandDB   float64 // the largest note-long change of one band
	Cents    float64 // mean shift of the strongest partial (0 when the bands decided)
}

// Audible reports whether the change is above any of the oracle's thresholds.
func (c Change) Audible() bool {
	return c.GlobalDB >= AudibleMinDB || c.LocalDB >= AudibleLocalDB ||
		c.BandDB >= AudibleBandDB || c.Cents >= AudibleMinCents
}

// Sample is a PCM sample type (renders arrive as float32 from the graph path
// and as float64 from the playback path).
type Sample interface{ ~float32 | ~float64 }

// CompareNotes measures how differently a and b (mono, sample rate sr) sound.
// The shorter render is padded with silence, so a longer or shorter note
// counts as the change it is. The partial shift is only measured when the
// band envelope alone does not already find the change audible.
func CompareNotes[S Sample](a, b []S, sr int) Change { return CompareNotesYield(a, b, sr, nil) }

// CompareNotesYield is CompareNotes handing the thread back (yield) every few
// transform frames: the browser measures dial relevance on the page's one
// thread, between frames.
func CompareNotesYield[S Sample](a, b []S, sr int, yield func()) Change {
	fa, fb := toF64(a), toF64(b)
	A, B := bandEnvelopeDB(fa, sr, yield), bandEnvelopeDB(fb, sr, yield)
	var c Change
	c.GlobalDB, c.LocalDB = bandChangeDB(A, B)
	c.BandDB = bandMaxChangeDB(A, B)
	if !c.Audible() {
		c.Cents = partialShiftCents(fa, fb, sr, yield)
	}
	return c
}

// yieldEveryFrames is how many transform frames run between yields.
const yieldEveryFrames = 8

// Silent reports whether a render is (all but) silence.
func Silent[S Sample](pcm []S) bool {
	for _, s := range pcm {
		if math.Abs(float64(s)) >= SilentPeak {
			return false
		}
	}
	return true
}

func toF64[S Sample](x []S) []float64 {
	out := make([]float64, len(x))
	for i, v := range x {
		out[i] = float64(v)
	}
	return out
}

// bandEnvelopeDB returns per-frame third-octave band power in dB (25 Hz–16 kHz).
func bandEnvelopeDB(x []float64, sr int, yield func()) [][]float64 {
	const n, hop = 2048, 1024
	var edges []float64
	for f := 25.0; f < 16000; f *= math.Pow(2, 1.0/3) {
		edges = append(edges, f)
	}
	var out [][]float64
	seg := make([]float64, n)
	for s := 0; s < len(x); s += hop {
		if yield != nil && (s/hop)%yieldEveryFrames == yieldEveryFrames-1 {
			yield()
		}
		for i := range seg {
			seg[i] = 0
		}
		copy(seg, x[s:min(s+n, len(x))])
		mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: sr}, n, wave.WindowHann)
		row := make([]float64, len(edges)-1)
		for b := range row {
			p := 1e-20
			for k := int(edges[b] / binHz); k <= int(edges[b+1]/binHz) && k < len(mag); k++ {
				p += mag[k] * mag[k]
			}
			row[b] = 10 * math.Log10(p)
		}
		out = append(out, row)
	}
	return out
}

// bandChangeDB returns the whole-note mean |dB| band-envelope difference and
// the largest mean over any AudibleLocalFrames consecutive frames.
func bandChangeDB(a, b [][]float64) (global, local float64) {
	pad := func(m [][]float64, n, w int) [][]float64 {
		for len(m) < n {
			row := make([]float64, w)
			for i := range row {
				row[i] = -400
			}
			m = append(m, row)
		}
		return m
	}
	if len(a) == 0 || len(b) == 0 {
		return 0, 0
	}
	w := len(a[0])
	a, b = pad(a, len(b), w), pad(b, len(a), w)
	peak := math.Inf(-1)
	for i := range a {
		for j := range a[i] {
			peak = math.Max(peak, math.Max(a[i][j], b[i][j]))
		}
	}
	floor := peak - 60
	sum, cnt := 0.0, 0
	frame := make([]float64, len(a)) // per-frame mean |dB| (−1 = silent in both)
	for i := range a {
		fs, fc := 0.0, 0
		for j := range a[i] {
			x, y := math.Max(a[i][j], floor), math.Max(b[i][j], floor)
			if x == floor && y == floor {
				continue
			}
			fs += math.Abs(x - y)
			fc++
		}
		sum += fs
		cnt += fc
		frame[i] = -1
		if fc > 0 {
			frame[i] = fs / float64(fc)
		}
	}
	if cnt == 0 {
		return 0, 0
	}
	for i := 0; i+AudibleLocalFrames <= len(frame); i++ {
		w, ok := 0.0, true
		for k := i; k < i+AudibleLocalFrames; k++ {
			if frame[k] < 0 {
				ok = false
				break
			}
			w += frame[k]
		}
		if ok {
			local = math.Max(local, w/AudibleLocalFrames)
		}
	}
	return sum / float64(cnt), local
}

// partialShiftCents is the mean |cents| between a and c's strongest partial,
// frame by frame (186 ms frames, 46 ms hop): in each frame of c take its
// strongest partial (parabolic-interpolated), find a's strongest bin within
// ±6% of it, and average the shift over frames within 40 dB of c's loudest.
func partialShiftCents(a, c []float64, sr int, yield func()) float64 {
	const n, hop = 8192, 2048
	peak := func(mag []float64, lo, hi int) (bin, db float64) {
		best := lo
		for k := lo; k <= hi && k < len(mag)-1; k++ {
			if mag[k] > mag[best] {
				best = k
			}
		}
		db = 20 * math.Log10(mag[best]+1e-12)
		if best <= 0 || best >= len(mag)-1 {
			return float64(best), db
		}
		l, m, r := math.Log(mag[best-1]+1e-12), math.Log(mag[best]+1e-12), math.Log(mag[best+1]+1e-12)
		return float64(best) + 0.5*(l-r)/(l-2*m+r), db
	}
	frames := func(x []float64) [][]float64 {
		var out [][]float64
		for s := 0; s+n <= len(x); s += hop {
			if yield != nil && (s/hop)%yieldEveryFrames == yieldEveryFrames-1 {
				yield()
			}
			m, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: x[s : s+n], SampleRate: sr}, n, wave.WindowHann)
			out = append(out, m)
		}
		return out
	}
	A, C := frames(a), frames(c)
	m := min(len(A), len(C))
	minBin := int(40 / (float64(sr) / n))
	bins, dbs := make([]float64, m), make([]float64, m)
	top := math.Inf(-1)
	for i := 0; i < m; i++ {
		bins[i], dbs[i] = peak(C[i], minBin, len(C[i])-2)
		top = math.Max(top, dbs[i])
	}
	sum, cnt := 0.0, 0
	for i := 0; i < m; i++ {
		k := bins[i]
		if dbs[i] < top-40 || k <= 0 {
			continue
		}
		fa, _ := peak(A[i], max(int(k*0.94), 1), int(k*1.06)+1)
		if fa <= 0 {
			continue
		}
		sum += math.Abs(1200 * math.Log2(fa/k))
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

// bandMaxChangeDB is the largest note-long level change of any third-octave
// band within AudibleBandFloorDB of the loudest band (in either render).
func bandMaxChangeDB(envA, envB [][]float64) float64 {
	mean := func(m [][]float64) []float64 {
		if len(m) == 0 {
			return nil
		}
		out := make([]float64, len(m[0]))
		for _, row := range m {
			for j, v := range row {
				out[j] += math.Pow(10, v/10)
			}
		}
		for j := range out {
			out[j] = 10 * math.Log10(out[j]/float64(len(m)))
		}
		return out
	}
	A, B := mean(envA), mean(envB)
	if len(A) == 0 || len(B) == 0 {
		return 0
	}
	top := math.Inf(-1)
	for j := range A {
		top = math.Max(top, math.Max(A[j], B[j]))
	}
	best := 0.0
	for j := range A {
		if math.Max(A[j], B[j]) < top-AudibleBandFloorDB {
			continue
		}
		best = math.Max(best, math.Abs(A[j]-B[j]))
	}
	return best
}
