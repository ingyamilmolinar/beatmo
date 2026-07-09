package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_segments.go — SEGMENT-based kick comparison. A whole-wave metric tells you
// THAT two kicks differ; a per-segment metric tells you WHERE in time — and time
// maps to a knob: ATTACK ↔ the beater click, BODY ↔ the modes/harmonics, TAIL ↔
// decay + reverb. This re-bins the same kind of per-window spectra KickAnalyze
// already computes into three fixed-time segments and exposes a
// per-segment × per-metric divergence grid (KickSegmentDistance), whose sorted
// "top deltas" read out as actionable targets ("ATTACK centroid +1.2 oct →
// darken the click", "TAIL flatness low → reverb not diffuse", "BODY MFCC Δ high
// → mode balance"). It complements (does not replace) the whole-signal traces in
// KickAnalyze, which remain the right shape for the pointwise trace distances.

// Segment names (stable keys for the distance grid).
const (
	SegAttack = "attack"
	SegBody   = "body"
	SegTail   = "tail"
)

// Fixed perceptual segment boundaries (ms), applied IDENTICALLY to both the
// synth and the reference. Fixed (not per-signal envelope-derived) boundaries are
// essential for a comparison tool: envelope-derived bounds shift when the signals
// differ (a brighter attack raises the peak → moves every boundary → even the
// "tail" then covers different samples, fabricating divergence). A kick's coarse
// structure is roughly time-stationary: click in the first ~35 ms, tonal body to
// ~150 ms, decay/reverb tail after.
const (
	segAttackEndMs = 35.0
	segBodyEndMs   = 150.0
)

// KickSegment holds the compact metric set for one time-region of a kick.
type KickSegment struct {
	Name             string
	StartSec, EndSec float64
	EnergyFrac       float64   // fraction of the whole signal's energy in this segment
	Centroid         float64   // spectral centroid (Hz) — brightness
	Flatness         float64   // spectral flatness (0=tonal … 1=noise)
	LowMidRatio      float64   // energy(40-100 Hz) / energy(100-500 Hz) — fundamental dominance
	Bands            []float64 // KickBandCount energy fractions, segment-averaged
	MFCC             [13]float64
}

// KickSegments is the per-segment analysis of a kick (attack, body, tail).
type KickSegments struct {
	SampleRate int
	Segments   []KickSegment
}

// Segment returns the segment with the given name, or nil.
func (ks KickSegments) Segment(name string) *KickSegment {
	for i := range ks.Segments {
		if ks.Segments[i].Name == name {
			return &ks.Segments[i]
		}
	}
	return nil
}

// KickSegmentAnalyze splits a peak-normalized kick into ATTACK/BODY/TAIL by fixed
// perceptual time boundaries (aligned across signals) and computes each segment's metrics.
func KickSegmentAnalyze(w wave.Wave) KickSegments {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out := KickSegments{SampleRate: sr}
	if w.PeakSample() < 1e-9 {
		return out
	}
	w = PeakNormalize(w)
	sr = w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out.SampleRate = sr
	x := w.Samples

	lo, mid, hi := kickSegmentBounds(x, sr)
	total := 0.0
	for _, v := range x {
		total += v * v
	}
	if total <= 0 {
		total = 1
	}
	spans := []struct {
		name string
		a, b int
	}{
		{SegAttack, lo, mid},
		{SegBody, mid, hi},
		{SegTail, hi, len(x)},
	}
	for _, s := range spans {
		out.Segments = append(out.Segments, segmentMetrics(x, sr, s.name, s.a, s.b, total))
	}
	return out
}

// kickSegmentBounds returns the fixed sample indices [0, attackEnd, bodyEnd],
// clamped to the signal length (a short kick may have an empty tail/body).
func kickSegmentBounds(x []float64, sr int) (attackStart, attackEnd, bodyEnd int) {
	attackEnd = secToSamples(segAttackEndMs/1000, sr)
	bodyEnd = secToSamples(segBodyEndMs/1000, sr)
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > len(x) {
			return len(x)
		}
		return v
	}
	attackEnd = clamp(attackEnd)
	bodyEnd = clamp(bodyEnd)
	if bodyEnd < attackEnd {
		bodyEnd = attackEnd
	}
	return 0, attackEnd, bodyEnd
}

// segmentMetrics computes the metric set over samples[a:b].
func segmentMetrics(x []float64, sr int, name string, a, b int, totalEnergy float64) KickSegment {
	seg := KickSegment{
		Name:     name,
		StartSec: float64(a) / float64(sr),
		EndSec:   float64(b) / float64(sr),
		Bands:    make([]float64, KickBandCount),
	}
	if b <= a || a < 0 || b > len(x) {
		return seg
	}
	s := x[a:b]

	// Energy fraction (per-segment crest is intentionally NOT computed — it is
	// confounded by segment length/decay: a long decaying tail reads as high
	// crest. Transient sharpness is a whole-signal property, in KickFingerprint).
	e := 0.0
	for _, v := range s {
		e += v * v
	}
	seg.EnergyFrac = e / totalEnergy

	// Per-window spectra averaged over the segment (reuse the band framing).
	win := secToSamples(kickBandWindowSec, sr)
	hop := secToSamples(kickBandHopSec, sr)
	if win < 8 {
		win = 8
	}
	bands := KickBands()
	var cSum, fSum, lmSum float64
	var n int
	for start := 0; start < len(s); start += hop {
		end := start + win
		if end > len(s) {
			end = len(s)
		}
		if end-start < 8 {
			break
		}
		sw := wave.Wave{Samples: s[start:end], SampleRate: sr}
		mag, binHz := wave.MagnitudeSpectrum(sw, kickBandFFT, wave.WindowHann)
		cSum += spectralCentroidHz(mag, binHz)
		fSum += spectralFlatnessOf(mag)
		low := bandEnergy(mag, binHz, 40, 100)
		midE := bandEnergy(mag, binHz, 100, 500)
		lmSum += low / (midE + 1e-12)
		frac := kickBandFractions(sw, bands, kickBandFFT)
		for bi := 0; bi < KickBandCount && bi < len(frac); bi++ {
			seg.Bands[bi] += frac[bi]
		}
		n++
	}
	if n > 0 {
		seg.Centroid = cSum / float64(n)
		seg.Flatness = fSum / float64(n)
		seg.LowMidRatio = lmSum / float64(n)
		for bi := range seg.Bands {
			seg.Bands[bi] /= float64(n)
		}
	}
	seg.MFCC = computeMFCC(wave.Wave{Samples: s, SampleRate: sr}, 13)
	return seg
}

// --- distance -----------------------------------------------------------------

// SegmentDelta is one (segment, metric) divergence between ref and synth.
// NormDelta is scaled so magnitudes are comparable across metrics (used to rank).
type SegmentDelta struct {
	Segment   string
	Metric    string
	Ref       float64
	Synth     float64
	NormDelta float64
}

// KickSegmentDistanceBreakdown is the per-segment × per-metric grid. Deltas is
// sorted by |NormDelta| descending — the actionable "top divergences" list.
type KickSegmentDistanceBreakdown struct {
	PerSegment map[string]float64 // segment name → summed distance for that segment
	Deltas     []SegmentDelta
	Total      float64
}

// KickSegmentDistance compares two segmented kicks and returns the divergence
// grid + a ranked delta list. Lower Total = more similar.
func KickSegmentDistance(ref, synth KickSegments) KickSegmentDistanceBreakdown {
	out := KickSegmentDistanceBreakdown{PerSegment: map[string]float64{}}
	for _, rs := range ref.Segments {
		ss := synth.Segment(rs.Name)
		if ss == nil {
			continue
		}
		add := func(metric string, r, s, norm float64) {
			d := SegmentDelta{Segment: rs.Name, Metric: metric, Ref: r, Synth: s, NormDelta: norm}
			out.Deltas = append(out.Deltas, d)
			out.PerSegment[rs.Name] += math.Abs(norm)
			out.Total += math.Abs(norm)
		}
		add("energyFrac", rs.EnergyFrac, ss.EnergyFrac, rs.EnergyFrac-ss.EnergyFrac)
		add("centroid", rs.Centroid, ss.Centroid, math.Log2((rs.Centroid+25)/(ss.Centroid+25)))
		add("flatness", rs.Flatness, ss.Flatness, (rs.Flatness-ss.Flatness)*4)
		add("lowMid", rs.LowMidRatio, ss.LowMidRatio, math.Log((rs.LowMidRatio+0.2)/(ss.LowMidRatio+0.2)))
		bandD := 0.0
		for bi := 0; bi < KickBandCount && bi < len(rs.Bands) && bi < len(ss.Bands); bi++ {
			bandD += math.Abs(rs.Bands[bi] - ss.Bands[bi])
		}
		add("bands", 0, 0, bandD)
		mfccD := 0.0
		for i := 1; i < 13; i++ {
			d := (rs.MFCC[i] - ss.MFCC[i]) / 20.0
			mfccD += d * d
		}
		add("mfcc", 0, 0, math.Sqrt(mfccD/12.0))
	}
	sort.SliceStable(out.Deltas, func(i, j int) bool {
		return math.Abs(out.Deltas[i].NormDelta) > math.Abs(out.Deltas[j].NormDelta)
	})
	return out
}
