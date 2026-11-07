package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// What makes an instrument THAT instrument, insensitive to pitch, length, level and room: ERB bands resolve harmonics as
// the ear does, spectra are LOUDNESS across frequency and frames are weighted by ENERGY across time, so a long reverb
// tail cannot out-vote the hit. The caller plays both clips at one note.

// EssenceOpts says what the caller already knows about a clip.
type EssenceOpts struct {
	// 0 derives it from the clip (a reference); a candidate passes the reference's.
	WindowSec float64
}

// Essence is one clip's identity description over its analysis window.
type Essence struct {
	SampleRate int
	WindowSec  float64
	HopSec     float64
	// Silent: nothing above the numerical floor inside the window.
	Silent bool

	// Loud is the per-frame loudness, normalised so its peak is 1.
	Loud []float64
	// How much each moment counts in every time average.
	Weight []float64
	// Unit sum, so the colour is independent of loudness; nil for a silent frame.
	Shape [][]float64
	// Color is the clip's energy-weighted mean colour (unit sum).
	Color []float64
	// Median bin power over the octave mean, ÷ ln 2: white noise ~1, partials over silence ~0 (the median sits between
	// partials). Flatness was tried and is dominated by the partial peaks. FlatW is each octave's loudness share.
	Flat, FlatW []float64
	// dB re the loudest frame, clipped at essDecayFloor, after the onset of the loud part; NaN past the window.
	Decay []float64
	// AttackSec is the 10 → 90 % amplitude rise time.
	AttackSec float64
}

type EssenceScore struct {
	Color     float64 `json:"color"`     // mean colour, L1 of unit-sum loudness spectra (0..2)
	Evolution float64 `json:"evolution"` // how the colour moves over the note (attack → body), L1 (0..~2)
	Texture   float64 `json:"texture"`   // per-octave noise share |Δ|, weighted by the smaller octave share (0..1)
	Decay     float64 `json:"decay"`     // decay profile after the peak, mean |Δ dB| / 30 (0..1)
	Attack    float64 `json:"attack"`    // |Δ log10 rise time|, rise times floored at essAttackJND (0..2)
}

// Colour, its evolution, the noise texture and how the note dies away (how it is PLAYED, not for how long) are the
// instrument; the attack depends partly on the player and counts less. The terms share a currency, so they sum as
// measured: per-term median normalisation amplified the narrow terms and graded worse.
var EssenceWeights = EssenceScore{
	Color: 1, Evolution: 1, Texture: 1, Decay: 1, Attack: 0.1,
}

const (
	essFrameSec   = 0.0464 // ~2048 samples at 44.1 kHz: the STFT frame
	essWindowDB   = -30.0  // the reference region ends at its last frame within this of its peak
	essLoudExp    = 0.3    // Stevens' power law on band POWER
	essMinHz      = 30.0
	essMaxHz      = 12000.0 // corpus MP3 previews are codec-limited above ~14 kHz
	essDecayFloor = -30.0   // decay levels are clipped here (= essWindowDB)
	essAttackWinS = 0.020   // attack envelope: running max over >= one period of 50 Hz...
	essAttackHopS = 0.001   // ...sampled every ms
	essMinAttack  = 0.001
	// The ear hears every rise shorter than this as instantaneous.
	essAttackJND   = 0.010
	essSilentPeak  = 1e-9
	essSilentFrame = 1e-9 // a frame quieter than this × the loudest is silent
)

// w starts at its onset; refs match trims every clip alike.
func AnalyzeEssence(w wave.Wave, o EssenceOpts) Essence {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	frame := essNearestPow2(essFrameSec * float64(sr))
	hop := frame / 4
	e := Essence{SampleRate: sr, HopSec: float64(hop) / float64(sr)}

	win := int(math.Round(o.WindowSec * float64(sr)))
	if o.WindowSec <= 0 {
		win = essReferenceWindow(w.Samples, frame, hop)
	}
	if win < hop {
		win = hop
	}
	e.WindowSec = float64(win) / float64(sr)
	s := make([]float64, win)
	copy(s, w.Samples)

	var peak float64
	for _, v := range s {
		peak = math.Max(peak, math.Abs(v))
	}
	nFrames := (win + hop - 1) / hop
	if peak < essSilentPeak {
		e.Silent = true
		e.Loud = make([]float64, nFrames)
		e.Weight = make([]float64, nFrames)
		e.Shape = make([][]float64, nFrames)
		return e
	}

	bands := essERBBands(frame, sr)
	power := make([][]float64, nFrames) // [frame][band]
	var pmax float64
	for i := 0; i < nFrames; i++ {
		mag := essFrameMag(s, i*hop, frame, sr)
		row := make([]float64, len(bands))
		for b, bd := range bands {
			for k := bd.lo; k < bd.hi; k++ {
				row[b] += mag[k] * mag[k]
			}
			pmax = math.Max(pmax, row[b])
		}
		power[i] = row
	}

	e.Loud = make([]float64, nFrames)
	e.Weight = make([]float64, nFrames)
	e.Shape = make([][]float64, nFrames)
	e.Color = make([]float64, len(bands))
	var lmax, emax float64
	for i, row := range power {
		n := make([]float64, len(row))
		var l, en float64
		for b, p := range row {
			n[b] = math.Pow(p/pmax, essLoudExp)
			l += n[b]
			en += p
		}
		e.Loud[i], e.Weight[i] = l, en
		lmax, emax = math.Max(lmax, l), math.Max(emax, en)
		e.Shape[i] = n // normalised below, once lmax is known
	}
	var wtot float64
	for i := range e.Shape {
		l := e.Loud[i]
		e.Loud[i] = l / lmax
		e.Weight[i] /= emax
		if l <= essSilentFrame*lmax {
			e.Shape[i] = nil
			continue
		}
		for b := range e.Shape[i] {
			e.Shape[i][b] /= l
			e.Color[b] += e.Weight[i] * e.Shape[i][b]
		}
		wtot += e.Weight[i]
	}
	for b := range e.Color {
		e.Color[b] /= wtot
	}

	complete := 1 // frames that end inside the window
	if win > frame {
		complete += (win - frame) / hop
	}
	e.Decay = essDecayProfile(e.Weight[:min(complete, nFrames)], e.HopSec)
	e.Flat, e.FlatW = essTexture(s, sr, frame)
	e.AttackSec = essAttackTime(s, sr)
	return e
}

// ref defines the region and the weighting: "does cand sound like THIS recording".
func EssenceTerms(ref, cand Essence) EssenceScore {
	var s EssenceScore
	s.Color = essL1(ref.Color, cand.Color)

	// Each frame's DEVIATION from the clip's own mean colour, so it measures movement and nothing Color counted.
	var num, den float64
	for i := 0; i < len(ref.Shape) && i < len(cand.Shape); i++ {
		r, c := ref.Shape[i], cand.Shape[i]
		if r == nil || c == nil {
			continue
		}
		var d float64
		for b := range r {
			d += math.Abs((r[b] - ref.Color[b]) - (c[b] - cand.Color[b]))
		}
		num += ref.Weight[i] * d
		den += ref.Weight[i]
	}
	if den > 0 {
		s.Evolution = num / den
	}

	// Only where BOTH clips have sound: whether an octave has energy at all is Color's question, and weighting by one clip
	// let a faint noisy residue outweigh the best harmonic match.
	for b := range ref.FlatW {
		if b < len(cand.Flat) {
			w := math.Min(ref.FlatW[b], cand.FlatW[b])
			s.Texture += w * math.Abs(ref.Flat[b]-cand.Flat[b])
		}
	}

	// A candidate whose peak came later has fewer in-window delays and holds its last level.
	var dn int
	for k, r := range ref.Decay {
		if math.IsNaN(r) {
			continue
		}
		c := essDecayFloor
		for j := min(k, len(cand.Decay)-1); j >= 0; j-- {
			if !math.IsNaN(cand.Decay[j]) {
				c = cand.Decay[j]
				break
			}
		}
		s.Decay += math.Abs(r-c) / -essDecayFloor
		dn++
	}
	if dn > 0 {
		s.Decay /= float64(dn)
	}

	s.Attack = math.Abs(math.Log10(math.Max(ref.AttackSec, essAttackJND)) - math.Log10(math.Max(cand.AttackSec, essAttackJND)))
	return s
}

// Also the pitch-search objective: resampling a drum moves its decay as much as its colour, so both are judged.
func EssenceTotal(s EssenceScore) float64 {
	return essenceTotalsOf([]EssenceScore{s})[0]
}

// Lower is closer; a silent candidate is +Inf.
func EssenceTotals(ref Essence, cands []Essence) []float64 {
	scores := make([]EssenceScore, len(cands))
	for i, c := range cands {
		scores[i] = EssenceTerms(ref, c)
	}
	tot := essenceTotalsOf(scores)
	for i, c := range cands {
		if c.Silent {
			tot[i] = math.Inf(1)
		}
	}
	return tot
}

// silent[i] marks a candidate that rendered nothing.
func EssenceTotalsOf(scores []EssenceScore, silent []bool) []float64 {
	tot := essenceTotalsOf(scores)
	for i := range tot {
		if i < len(silent) && silent[i] {
			tot[i] = math.Inf(1)
		}
	}
	return tot
}

func essenceTotalsOf(scores []EssenceScore) []float64 {
	w := EssenceWeights
	tot := make([]float64, len(scores))
	for i, s := range scores {
		tot[i] = w.Color*s.Color + w.Evolution*s.Evolution + w.Texture*s.Texture +
			w.Decay*s.Decay + w.Attack*s.Attack
	}
	return tot
}

// Three readable directions (brightness, noise, decay half a second in) telling a person HOW a voice differs; the
// terms only say by how much.
type EssenceTraits struct {
	// BrightnessHz is the centre of mass of the mean colour, as a frequency.
	BrightnessHz float64 `json:"brightness_hz"`
	// 0 = pure partials, 1 = white noise.
	Noise float64 `json:"noise"`
	// 0 = held, essDecayFloor = gone.
	DecayDB float64 `json:"decay_db"`
}

// Traits summarises e (see EssenceTraits).
func (e Essence) Traits() EssenceTraits {
	var t EssenceTraits
	if e.Silent || len(e.Color) == 0 {
		t.DecayDB = essDecayFloor
		return t
	}
	frame := essNearestPow2(essFrameSec * float64(e.SampleRate))
	bands := essERBBands(frame, e.SampleRate)
	binHz := float64(e.SampleRate) / float64(frame)
	erb := func(f float64) float64 { return 21.4 * math.Log10(1+0.00437*f) }
	var c float64
	for b, v := range e.Color {
		c += v * erb(binHz*float64(bands[b].lo+bands[b].hi)/2)
	}
	t.BrightnessHz = (math.Pow(10, c/21.4) - 1) / 0.00437
	for b, w := range e.FlatW {
		t.Noise += w * e.Flat[b]
	}
	var n int
	for _, d := range e.Decay {
		if !math.IsNaN(d) {
			t.DecayDB += d
			n++
		}
	}
	if n > 0 {
		t.DecayDB /= float64(n)
	}
	return t
}

// --- analysis pieces -----------------------------------------------------------

type essBand struct{ lo, hi int } // FFT bin range [lo, hi)

// One band per ERB: the auditory filter resolution.
func essERBBands(frame, sr int) []essBand {
	binHz := float64(sr) / float64(frame)
	top := math.Min(essMaxHz, float64(sr)/2)
	erb := func(f float64) float64 { return 21.4 * math.Log10(1+0.00437*f) }
	inv := func(e float64) float64 { return (math.Pow(10, e/21.4) - 1) / 0.00437 }
	var out []essBand
	prev := int(math.Ceil(essMinHz / binHz))
	for e := erb(essMinHz) + 1; ; e++ {
		f := inv(e)
		last := f >= top
		if last {
			f = top
		}
		hi := int(math.Ceil(f / binHz))
		if hi > prev { // a band narrower than a bin merges into the next
			out = append(out, essBand{prev, hi})
			prev = hi
		}
		if last {
			return out
		}
	}
}

// Zero-padded past the end of s.
func essFrameMag(s []float64, start, frame, sr int) []float64 {
	end := min(start+frame, len(s))
	seg := wave.Wave{SampleRate: sr}
	if start < end {
		seg.Samples = s[start:end]
	}
	mag, _ := wave.MagnitudeSpectrum(seg, frame, wave.WindowHann)
	return mag
}

// Onset to the end of the last frame within essWindowDB of the loudest.
func essReferenceWindow(s []float64, frame, hop int) int {
	if len(s) == 0 {
		return 0
	}
	var pows []float64
	var pmax float64
	for start := 0; start < len(s); start += hop {
		var p float64
		for _, v := range s[start:min(start+frame, len(s))] {
			p += v * v
		}
		pows = append(pows, p)
		pmax = math.Max(pmax, p)
	}
	thr := pmax * math.Pow(10, essWindowDB/10)
	last := 0
	for i, p := range pows {
		if p >= thr {
			last = i
		}
	}
	return min(len(s), last*hop+frame)
}

// 2×frame STFTs, fine enough to separate a low note's partials.
func essTexture(s []float64, sr, frame int) (flat, share []float64) {
	tf := 2 * frame
	hop := tf / 2
	binHz := float64(sr) / float64(tf)
	var edges [][2]int
	for lo := 62.5; lo < math.Min(essMaxHz, float64(sr)/2); lo *= 2 {
		hi := math.Min(2*lo, math.Min(essMaxHz, float64(sr)/2))
		edges = append(edges, [2]int{int(math.Ceil(lo / binHz)), int(math.Ceil(hi / binHz))})
	}
	nb := len(edges)
	flat, share = make([]float64, nb), make([]float64, nb)
	type fr struct{ pow, flat []float64 }
	var frames []fr
	var pmax float64
	for start := 0; start < len(s); start += hop {
		mag := essFrameMag(s, start, tf, sr)
		var fmax float64
		for _, m := range mag {
			fmax = math.Max(fmax, m*m)
		}
		f := fr{make([]float64, nb), make([]float64, nb)}
		eps := 1e-12*fmax + 1e-300
		var bins []float64
		for b, ed := range edges {
			bins = bins[:0]
			var sum float64
			for k := ed[0]; k < ed[1] && k < len(mag); k++ {
				p := mag[k]*mag[k] + eps
				sum += p
				bins = append(bins, p)
			}
			if len(bins) > 0 {
				f.pow[b] = sum
				f.flat[b] = math.Min(1, essMedian(bins)/(sum/float64(len(bins)))/math.Ln2)
			}
			pmax = math.Max(pmax, sum)
		}
		frames = append(frames, f)
	}
	if pmax <= 0 {
		return flat, share
	}
	// Over time ENERGY-weighted (a reverb tail is noise-like and long, the note short and loud); across octaves loudness.
	wsum := make([]float64, nb)
	for _, f := range frames {
		for b := range f.pow {
			flat[b] += f.pow[b] * f.flat[b]
			wsum[b] += f.pow[b]
		}
	}
	var stot float64
	for b := range flat {
		if wsum[b] > 0 {
			flat[b] /= wsum[b]
		}
		share[b] = math.Pow(wsum[b]/pmax, essLoudExp)
		stot += share[b]
	}
	for b := range share {
		if stot > 0 {
			share[b] /= stot
		}
	}
	return flat, share
}

// The note's "peak" is the ONSET of its loud part, not the arg-max: on a held plateau the arg-max lands anywhere, and a
// late one read the decay off the cut end, leaking the note length into Decay.
const essPeakFrac = 0.794

func essLoudOnset(energy []float64) int {
	var mx float64
	for _, v := range energy {
		mx = math.Max(mx, v)
	}
	for i, v := range energy {
		if v >= essPeakFrac*mx {
			return i
		}
	}
	return 0
}

// Log-spaced, so the early decay where the direct sound dominates is sampled densely; they stop at half a second,
// where struck separates from held. How many SECONDS a note rings is duration-like and let a sustained tuba out-rank
// the right instrument.
var essDecayDelays = []float64{0.03, 0.06, 0.12, 0.25, 0.5}

// Only frames that end inside the window: one running past it is part padding and would read as a decay.
func essDecayProfile(weight []float64, hopSec float64) []float64 {
	pk := essLoudOnset(weight)
	out := make([]float64, len(essDecayDelays))
	for k, d := range essDecayDelays {
		j := pk + int(math.Round(d/hopSec))
		if j >= len(weight) {
			out[k] = math.NaN()
			continue
		}
		db := essDecayFloor
		if weight[j] > 0 {
			db = math.Max(essDecayFloor, 10*math.Log10(weight[j]))
		}
		out[k] = db
	}
	return out
}

// A running max over at least one period has no ripple on a 50 Hz kick (a 5 ms RMS read it by phase) and jumps the
// moment the onset arrives.
func essAttackTime(s []float64, sr int) float64 {
	hop := max(1, int(essAttackHopS*float64(sr)))
	span := max(1, int(math.Round(essAttackWinS/essAttackHopS)))
	var blocks []float64 // max |x| per hop-long block
	for start := 0; start < len(s); start += hop {
		var m float64
		for _, v := range s[start:min(start+hop, len(s))] {
			m = math.Max(m, math.Abs(v))
		}
		blocks = append(blocks, m)
	}
	env := make([]float64, len(blocks))
	for i := range blocks {
		var m float64
		for j := max(0, i-span+1); j <= i; j++ {
			m = math.Max(m, blocks[j])
		}
		env[i] = m
	}
	if len(env) == 0 {
		return essMinAttack
	}
	pk := 0
	for i, v := range env {
		if v > env[pk] {
			pk = i
		}
	}
	t10, t90 := -1, -1
	for i := 0; i <= pk; i++ {
		if t10 < 0 && env[i] >= 0.1*env[pk] {
			t10 = i
		}
		if t90 < 0 && env[i] >= 0.9*env[pk] {
			t90 = i
			break
		}
	}
	if t10 < 0 || t90 < 0 {
		return essMinAttack
	}
	return math.Max(essMinAttack, float64(t90-t10)*float64(hop)/float64(sr))
}

func essL1(a, b []float64) float64 {
	var d float64
	for i := range a {
		var v float64
		if i < len(b) {
			v = b[i]
		}
		d += math.Abs(a[i] - v)
	}
	return d
}

// Reorders v; selects rather than sorts, exactly the sorted middle.
func essMedian(v []float64) float64 {
	n := len(v)
	if n == 0 {
		return 0
	}
	hi := essSelect(v, n/2)
	if n%2 == 1 {
		return hi
	}
	lo := v[0] // everything before n/2 is <= v[n/2]: the largest is the other middle
	for _, x := range v[1 : n/2] {
		lo = math.Max(lo, x)
	}
	return (lo + hi) / 2
}

// Hoare's FIND.
func essSelect(v []float64, k int) float64 {
	l, r := 0, len(v)-1
	for l < r {
		x := v[k]
		i, j := l, r
		for i <= j {
			for v[i] < x {
				i++
			}
			for x < v[j] {
				j--
			}
			if i <= j {
				v[i], v[j] = v[j], v[i]
				i++
				j--
			}
		}
		if j < k {
			l = i
		}
		if k < i {
			r = j
		}
	}
	return v[k]
}

func essNearestPow2(x float64) int {
	p := 1
	for float64(p*2) <= x*math.Sqrt2 {
		p *= 2
	}
	return p
}
