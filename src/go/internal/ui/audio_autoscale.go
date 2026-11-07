package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The dB-domain sibling of the waveform's amplitude scaling, so Spectrum and the EQ backdrop agree on
// what a bar height means.

const (
	// Too wide flattens a normal 20-25 dB spread into stubs; too narrow bottoms out all but the loudest band.
	audioDBWindowSpan = 42.0

	// Quantised edges, or the axis renumbers on every refresh and reads as flicker.
	audioDBWindowGrid = 6.0

	// Below this there is nothing to frame: zooming further would draw silence as a full spectrum.
	audioDBSilenceDB = -70.0

	// Keeps the ceiling above the peak, so the loudest bar never sits flush on the top edge and reads as clipping.
	audioDBHeadroomDB = 1.0
)

// Zooms IN on quiet material, but never above full scale nor below the analyser's floor.
func audioDBWindow(peakDB float64) (floorDB, ceilDB float64) {
	if peakDB < audioDBSilenceDB {
		return spectrumMinDB, spectrumMaxDB
	}
	ceil := math.Ceil((peakDB+audioDBHeadroomDB)/audioDBWindowGrid) * audioDBWindowGrid
	if ceil > spectrumMaxDB {
		ceil = spectrumMaxDB
	}
	// Bound to the data, not taste: every bin is already clamped at -80 dB, so a lower floor would map
	// every quiet band to one row of pixels. The two constants move together.
	if minCeil := spectrumMinDB + audioDBWindowSpan; ceil < minCeil {
		ceil = minCeil
	}
	return ceil - audioDBWindowSpan, ceil
}

// 0 = floor, 1 = ceiling, clamped.
func audioDBNorm(db, floorDB, ceilDB float64) float64 {
	span := ceilDB - floorDB
	if span <= 0 {
		return 0
	}
	n := (db - floorDB) / span
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

// Only ever RISES: a decaying hold moved the axis between transients, so an unchanged band appeared
// to jump. One early transient sets the scale until resetSpectrumMax re-baselines it.
func raiseDBMax(prev, peak float64) float64 {
	if peak > prev {
		return peak
	}
	return prev
}

// new = old*blend + target*(1-blend).
const eqBandLevelsBlend = 0.5

// Through the SAME dB window as Spectrum, so a band is the same height on both tabs. An empty spec
// decays the bars rather than freezing them; prev is reused in place.
func eqBandLevels(spec []float64, prev []float64, win spectrumDBWindow) []float64 {
	if len(prev) != len(eqBandDefs) {
		prev = make([]float64, len(eqBandDefs))
	}
	specLen := len(spec)
	if specLen == 0 {
		for i := range prev {
			prev[i] *= eqBandLevelsBlend
		}
		return prev
	}
	floorDB, ceilDB := win.resolve()
	const nyquist = 24000.0
	for i, band := range eqBandDefs {
		start := int(math.Floor((band.loHz / nyquist) * float64(specLen)))
		end := int(math.Ceil((band.hiHz / nyquist) * float64(specLen)))
		if end <= start {
			end = start + 1
		}
		if start < 0 {
			start = 0
		}
		if end > specLen {
			end = specLen
		}
		maxV := 0.0
		for j := start; j < end; j++ {
			if spec[j] > maxV {
				maxV = spec[j]
			}
		}
		if maxV > 1 {
			maxV = 1
		}
		target := audioDBNorm(linearToDB(maxV), floorDB, ceilDB)
		prev[i] = prev[i]*eqBandLevelsBlend + target*(1-eqBandLevelsBlend)
	}
	return prev
}

// Floored so log(0) never reaches the plot.
func linearToDB(m float64) float64 {
	if m <= 0 {
		return spectrumMinDB
	}
	db := 20 * math.Log10(m)
	if db < spectrumMinDB {
		return spectrumMinDB
	}
	return db
}

// From the instrument's pre-rendered one-shot: a single channel is silent between hits, so the live
// analyser cannot set its scale (most frames carry nothing). false for the master bus.
func prerenderedChannelWindow(ch channel.ID) (spectrumDBWindow, bool) {
	inst, ok := ch.Instrument()
	if !ok {
		return spectrumDBWindow{}, false
	}
	buf, _ := audio.RenderInstrumentOneShot(inst)
	if len(buf) == 0 {
		return spectrumDBWindow{}, false
	}
	peak := 0.0
	for _, v := range buf {
		a := math.Abs(float64(v))
		if a > peak {
			peak = a
		}
	}
	if peak <= 0 {
		return spectrumDBWindow{}, false
	}
	floor, ceil := audioDBWindow(linearToDB(peak))
	return spectrumDBWindow{Floor: floor, Ceil: ceil}, true
}

// Long enough for a transient at any tempo, short enough that the one re-scale reads as settling.
const spectrumChannelSettle = 1200 * time.Millisecond
