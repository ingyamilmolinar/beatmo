package scopeexport

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// computeStageMetrics runs wave observers on samples and returns metrics.
func computeStageMetrics(samples []float64, sampleRate, waveformBins, fftTopN int, peakObs, fftObs, zcObs wave.Observer) *StageMetrics {
	w := wave.Wave{Samples: samples, SampleRate: sampleRate}
	obs := wave.Observe(w, peakObs, fftObs, zcObs)

	return &StageMetrics{
		PeakDB:        obs[0].PeakDB,
		RMSDB:         obs[0].RMSDB,
		ClipCount:     obs[0].ClipCount,
		Waveform64:    downsampleMinMax(samples, waveformBins),
		FFTTop:        topNBins(obs[1], fftTopN),
		ZeroCrossings: obs[2].ZeroCrossings,
		ZCRate:        obs[2].ZCRate,
	}
}

// downsampleMinMax divides samples into bins equal buckets, returning
// the [min, max] pair for each bucket. This is the standard waveform
// overview technique used in DAW waveform displays.
func downsampleMinMax(samples []float64, bins int) [][2]float64 {
	n := len(samples)
	if n == 0 || bins <= 0 {
		return nil
	}
	result := make([][2]float64, bins)
	bucketSize := float64(n) / float64(bins)
	for i := 0; i < bins; i++ {
		lo := int(float64(i) * bucketSize)
		hi := int(float64(i+1) * bucketSize)
		if hi > n {
			hi = n
		}
		if lo >= hi {
			continue
		}
		mn, mx := samples[lo], samples[lo]
		for j := lo + 1; j < hi; j++ {
			if samples[j] < mn {
				mn = samples[j]
			}
			if samples[j] > mx {
				mx = samples[j]
			}
		}
		result[i] = [2]float64{mn, mx}
	}
	return result
}

// topNBins extracts the top N FFT bins by magnitude (descending),
// skipping the DC bin at index 0.
func topNBins(obs wave.Observation, n int) []FFTBin {
	if len(obs.Bins) < 2 || len(obs.FreqBins) < 2 {
		return nil
	}
	type entry struct {
		hz float64
		db float64
	}
	// Skip DC bin (index 0).
	entries := make([]entry, 0, len(obs.Bins)-1)
	for i := 1; i < len(obs.Bins) && i < len(obs.FreqBins); i++ {
		entries = append(entries, entry{hz: obs.FreqBins[i], db: obs.Bins[i]})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].db > entries[j].db
	})
	if n > len(entries) {
		n = len(entries)
	}
	result := make([]FFTBin, n)
	for i := 0; i < n; i++ {
		result[i] = FFTBin{Hz: entries[i].hz, DB: entries[i].db}
	}
	return result
}
