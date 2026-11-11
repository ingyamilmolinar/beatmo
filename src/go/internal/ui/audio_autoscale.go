package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Adaptive Y-scaling shared by the audio panel's data tabs.
//
// Wave already framed its own amplitude (render_waveform_scale.go: peak-hold
// with instant attack and slow release, gain = 0.9/peak). Chain owned the same
// model but defaulted it OFF, and the two dB tabs — Spectrum and the EQ
// backdrop — had no adaptive scaling at all: Spectrum mapped a FIXED -80..0 dB
// window and the EQ bars used a raw sqrt of linear FFT magnitude. On normal
// material both left most of the plot permanently empty.
//
// The amplitude half of the model stays in render_waveform_scale.go. This file
// is its dB-domain sibling, so Spectrum and EQ agree on what a bar height
// means.

const (
	// audioDBWindowSpan is the dB distance from the plot's floor to its
	// ceiling. It sets how much dynamic range one plot height buys: too wide
	// and a normal 20-25 dB spread between bands reads as a flat row of
	// stubs, too narrow and everything but the loudest band bottoms out.
	// Tuned against the shipped demo circuit, whose bands span roughly
	// -28 dB (kick fundamental) to -48 dB (16k air).
	audioDBWindowSpan = 42.0

	// audioDBWindowGrid quantises the window's edges. Without it the ceiling
	// chases the peak continuously and the axis labels renumber on every
	// analyser refresh (~30 Hz), which reads as flicker rather than as scale.
	audioDBWindowGrid = 6.0

	// audioDBSilenceDB is the peak below which there is nothing worth
	// framing. Zooming further would scale the noise floor up to fill the
	// plot, rendering silence as a full-height spectrum. Mirrors Wave's
	// waveAutoGainSilence, which snaps back to 1x for the same reason.
	audioDBSilenceDB = -70.0

	// audioDBHeadroomDB keeps the ceiling strictly above the peak it frames.
	// Without it a peak landing exactly on a grid multiple maps to the very
	// top pixel of the plot, and a bar flush against the panel edge reads as
	// clipping rather than as "loudest" — the same reason the 0 dB reference
	// line is suppressed when it would land flush under the sticky bar.
	audioDBHeadroomDB = 1.0
)

// audioDBWindow maps a (smoothed) peak level to the dB window a plot should
// render: the ceiling rides just above the peak on the grid, the floor sits
// one span below. Both stay inside the analyser's fixed reference range, so
// the window can zoom IN on quiet material but never invents headroom above
// full scale nor plot height below the noise floor. Pure; unit-tested.
func audioDBWindow(peakDB float64) (floorDB, ceilDB float64) {
	if peakDB < audioDBSilenceDB {
		return spectrumMinDB, spectrumMaxDB
	}
	ceil := math.Ceil((peakDB+audioDBHeadroomDB)/audioDBWindowGrid) * audioDBWindowGrid
	if ceil > spectrumMaxDB {
		ceil = spectrumMaxDB
	}
	// Keep the floor inside the reference range rather than letting the
	// window slide off the bottom for very quiet material.
	//
	// This clamp is NOT an arbitrary taste limit — it is bound to the data.
	// Every bin reaching a renderer has already been clamped at
	// analyzerDBFloor (-80) by LinearToDBSafe, and the band levels are
	// clamped again at spectrumMinDB in spectrumBandDB, so a window whose
	// floor sat below -80 would spend plot height on a region no bin can
	// ever occupy and map every quiet band to one constant row of pixels.
	// The two constants move together or not at all.
	//
	// Removed once (2026-09-18) on the theory that WASM bins are "true dBFS
	// ~25 dB lower" than desktop's ampToDB(linMag/fftSize). They are not:
	// WebAudio's getFloatFrequencyData divides by fftSize exactly as
	// wave.fftObserver does. The real browser defect was on the FREQUENCY
	// axis — see analyzer.SpectrumFFTSize — and this clamp had nothing to
	// do with it. It could not have: under the clamp a -55 dB peak still
	// fills 60% of the plot, which is a short plot, not the empty one that
	// was being chased.
	if minCeil := spectrumMinDB + audioDBWindowSpan; ceil < minCeil {
		ceil = minCeil
	}
	return ceil - audioDBWindowSpan, ceil
}

// audioDBNorm places a dB value in [0,1] within the window, clamped at both
// ends. 0 is the plot's floor, 1 its ceiling. Pure; unit-tested.
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

// raiseDBMax is the dB-domain axis anchor: the loudest peak observed so far,
// which only ever RISES.
//
// It deliberately does NOT decay, unlike advanceWavePeak in the amplitude
// family. A decaying hold slid the Spectrum ceiling down between drum
// transients and snapped it back up on the next hit, so a band whose level
// never changed appeared to jump — the axis was moving underneath it. On a
// plot that carries labelled absolute reference lines that is simply wrong:
// the scale is chosen from the loudest thing seen and then stays put, so two
// readings taken a second apart are comparable.
//
// The trade-off is that one early transient sets the scale for the session;
// resetSpectrumMax re-baselines it (channel change, or the DTL reset that
// already clears the peak watermark). Pure; unit-tested.
func raiseDBMax(prev, peak float64) float64 {
	if peak > prev {
		return peak
	}
	return prev
}

// eqBandLevelsBlend is the per-frame smoothing factor for the EQ backdrop
// bars: new = old*blend + target*(1-blend). Unchanged from the pre-adaptive
// renderer, so only the mapping moved, not the ballistics.
const eqBandLevelsBlend = 0.5

// eqBandLevels maps a linear FFT magnitude spectrum to one smoothed 0..1 bar
// height per EQ band, through the SAME dB window the Spectrum tab uses — so a
// band of a given loudness is the same height on both tabs.
//
// It replaces a sqrt() of the raw linear magnitude, which was not a dB scale
// at all: a perfectly normal -28 dBFS band (linear 0.04) drew at sqrt(0.04) =
// 20% of the plot and no panel size could make it read.
//
// `prev` is the previous frame's levels (its length defines the band count and
// it is reused in place); an empty `spec` decays them toward zero rather than
// freezing the bars at their last height. Pure; unit-tested.
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

// linearToDB converts a linear magnitude in [0,1] to dBFS, floored at the
// analyser's reference minimum so log(0) never reaches the plot.
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

// prerenderedChannelWindow derives a channel's dB window from the
// instrument's PRE-RENDERED one-shot instead of from the live analyser.
//
// The live analyser cannot set the scale for a single instrument's channel:
// that channel is silent between transients, so the anchor is whatever the
// last hit happened to be, and in the moment the user picks the channel
// there may not have been one yet. Measured on the shipped circuit with the
// Dnb Kick selected, 88 of 148 frames carried no signal at all.
//
// The instrument's one-shot is what the channel WILL sound like, it is
// already computed, and it is available the instant the channel is picked —
// so the scale is right immediately and then never moves.
//
// Returns ok=false for the master bus, which is a mix of instruments with no
// single buffer behind it; that channel keeps the live max-anchored axis.
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

// spectrumChannelSettle is how long the pre-rendered estimate stands in
// before the axis is re-pinned on the channel's measured level. Long enough
// for at least one transient at any musical tempo (a 60 bpm beat is 1 s),
// short enough that the single re-scale reads as the tab finding its feet
// rather than as the axis moving about.
const spectrumChannelSettle = 1200 * time.Millisecond
