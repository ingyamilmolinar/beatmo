package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Frame is one timeline slice's descriptors. Section is a labeled span.
type Frame struct {
	TimeSec            float64
	RMS                float64
	Bands              []float64
	OnsetDensity       float64
	RegisterCentroidHz float64
	Chroma             [12]float64
	Key, Mode          int
}
type Section struct {
	StartSec, EndSec float64
	Label            string
}
type SongTimeline struct {
	Frames   []Frame
	Sections []Section
	FrameHz  float64
}

// SongTimelineOf builds per-hop frames and segments them into sections via a
// self-similarity novelty curve over per-frame feature vectors.
func SongTimelineOf(w wave.Wave, cfg AnalysisConfig) SongTimeline {
	frames, fhz, binHz := stftHops(w, cfg)
	if len(frames) == 0 {
		return SongTimeline{FrameHz: fhz}
	}
	sr := w.SampleRate
	hop := int(cfg.HopSec * float64(sr))

	env, _ := OnsetEnvelope(w, cfg) // same hop → aligned with frames

	out := SongTimeline{FrameHz: fhz}
	feats := make([][12]float64, len(frames)) // chroma feature per frame for novelty
	for i, mag := range frames {
		var f Frame
		f.TimeSec = float64(i) / fhz
		// RMS from the time-domain segment.
		start := i * hop
		end := start + cfg.FrameFFTSize
		if end > len(w.Samples) {
			end = len(w.Samples)
		}
		ss := 0.0
		for s := start; s < end; s++ {
			ss += w.Samples[s] * w.Samples[s]
		}
		if end > start {
			f.RMS = math.Sqrt(ss / float64(end-start))
		}
		f.Bands = BandEnergies(mag, binHz, cfg.Bands)
		// Spectral centroid (register).
		num, den := 0.0, 0.0
		for k, m := range mag {
			num += float64(k) * binHz * m
			den += m
		}
		if den > 0 {
			f.RegisterCentroidHz = num / den
		}
		f.Chroma = Chroma(mag, binHz)
		f.Key, f.Mode, _ = DetectKey(f.Chroma, cfg)
		if i < len(env) {
			f.OnsetDensity = env[i]
		}
		feats[i] = f.Chroma
		out.Frames = append(out.Frames, f)
	}

	out.Sections = segmentSections(feats, fhz, cfg)
	// Set EndSec of last section to track duration.
	if n := len(out.Sections); n > 0 {
		out.Sections[n-1].EndSec = w.Duration()
	}
	return out
}

// segmentSections computes a novelty curve from frame-to-frame feature distance
// (smoothed over NoveltyKernel) and emits a section at each above-threshold peak.
func segmentSections(feats [][12]float64, frameHz float64, cfg AnalysisConfig) []Section {
	n := len(feats)
	if n == 0 || frameHz <= 0 {
		return nil
	}
	novelty := make([]float64, n)
	k := cfg.NoveltyKernel
	if k < 1 {
		k = 1
	}
	for i := k; i < n-k; i++ {
		// Mean feature before vs after window i.
		var before, after [12]float64
		for j := i - k; j < i; j++ {
			for c := 0; c < 12; c++ {
				before[c] += feats[j][c]
			}
		}
		for j := i; j < i+k; j++ {
			for c := 0; c < 12; c++ {
				after[c] += feats[j][c]
			}
		}
		d := 0.0
		for c := 0; c < 12; c++ {
			diff := (before[c] - after[c]) / float64(k)
			d += diff * diff
		}
		novelty[i] = math.Sqrt(d)
	}
	// Normalize novelty 0..1.
	maxN := 0.0
	for _, v := range novelty {
		if v > maxN {
			maxN = v
		}
	}
	if maxN > 0 {
		for i := range novelty {
			novelty[i] /= maxN
		}
	}
	// Boundaries = local maxima above threshold.
	var bounds []int
	bounds = append(bounds, 0)
	for i := k + 1; i < n-k-1; i++ {
		if novelty[i] > cfg.NoveltyThresh && novelty[i] >= novelty[i-1] && novelty[i] >= novelty[i+1] {
			// Suppress near-duplicate boundaries within one kernel width.
			if i-bounds[len(bounds)-1] >= k {
				bounds = append(bounds, i)
			}
		}
	}
	var secs []Section
	for bi, b := range bounds {
		startSec := float64(b) / frameHz
		endSec := float64(n) / frameHz
		if bi+1 < len(bounds) {
			endSec = float64(bounds[bi+1]) / frameHz
		}
		secs = append(secs, Section{StartSec: startSec, EndSec: endSec, Label: sectionLabel(bi)})
	}
	return secs
}

// sectionLabel assigns generic ordinal labels (no song-specific knowledge).
func sectionLabel(i int) string {
	labels := []string{"intro", "A", "B", "C", "D", "E", "F", "G"}
	if i < len(labels) {
		return labels[i]
	}
	return "section"
}
