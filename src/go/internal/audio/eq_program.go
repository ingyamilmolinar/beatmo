package audio

import "math"

// multibandBandDef defines frequency boundaries for a single band of the
// graphic EQ's band split.
type multibandBandDef struct {
	loHz float64
	hiHz float64
}

// defaultBandDefs matches the 10-band ISO standard EQ definitions used in the UI.
var defaultBandDefs = []multibandBandDef{
	{loHz: 22, hiHz: 44},       // 31 Hz
	{loHz: 44, hiHz: 88},       // 62 Hz
	{loHz: 88, hiHz: 177},      // 125 Hz
	{loHz: 177, hiHz: 354},     // 250 Hz
	{loHz: 354, hiHz: 707},     // 500 Hz
	{loHz: 707, hiHz: 1414},    // 1 kHz
	{loHz: 1414, hiHz: 2828},   // 2 kHz
	{loHz: 2828, hiHz: 5657},   // 4 kHz
	{loHz: 5657, hiHz: 11314},  // 8 kHz
	{loHz: 11314, hiHz: 20000}, // 16 kHz
}

// EQ program stage kinds (src/c/mix_dsp.h).
const (
	eqStageBiquad  = 0
	eqStageSplit   = 1
	eqStageSilence = 2
)

// crossoverQ is the Butterworth Q of the band split's Linkwitz–Riley
// crossovers (two cascaded stages each side: -24 dB/octave).
const crossoverQ = 0.707

// BuildEQProgram turns a channel's EQ bands into the program both platforms'
// C EQ runs (mix_dsp.h): ONE description of the filter topology and its
// coefficients, computed by this Go code on desktop and in the browser alike.
//
// High- and low-pass bands are filters in series at their position (the UI
// puts the high-pass first and the low-pass last). The shelf/peaking bands
// form the graphic section: a biquad chain when no band is muted; when some
// are, a band split (each band isolated by Linkwitz–Riley crossovers, muted
// bands dropped, the rest scaled by their gain) so muting a band never
// touches its neighbours; when all are, silence. The high/low-pass filters
// stay in series around the split — they used to be counted as bands of it,
// which dropped them and slid every band onto the wrong frequencies.
func BuildEQProgram(sr int, bands []EQBand) []float64 {
	var graphic []EQBand
	anyMuted, allMuted := false, true
	for _, b := range bands {
		if isGraphicBand(b.Kind) {
			graphic = append(graphic, b)
			if b.Muted {
				anyMuted = true
			} else {
				allMuted = false
			}
		}
	}
	prog := []float64{0}
	stages := 0
	addBiquad := func(kind EQKind, freq, q, gainDB float64) {
		c := ComputeBiquadCoeffs(kind, sr, freq, q, gainDB)
		if c == (BiquadCoeffs{}) {
			return
		}
		prog = append(prog, eqStageBiquad, c.B0, c.B1, c.B2, c.A1, c.A2)
		stages++
	}
	graphicDone := false
	for _, b := range bands {
		if !isGraphicBand(b.Kind) {
			addBiquad(b.Kind, b.Freq, b.Q, b.GainDB)
			continue
		}
		if !anyMuted {
			addBiquad(b.Kind, b.Freq, b.Q, b.GainDB)
			continue
		}
		if graphicDone {
			continue
		}
		graphicDone = true
		if allMuted {
			prog = append(prog, eqStageSilence)
			stages++
			continue
		}
		prog = append(prog, appendSplit(nil, sr, graphic)...)
		stages++
	}
	prog[0] = float64(stages)
	return prog
}

func isGraphicBand(k EQKind) bool { return k == EQPeaking || k == EQLowShelf || k == EQHighShelf }

// appendSplit is the band-split stage for the graphic bands.
func appendSplit(prog []float64, sr int, bands []EQBand) []float64 {
	defs := defaultBandDefs
	if len(bands) != len(defs) {
		defs = make([]multibandBandDef, len(bands))
		bandWidth := (20000.0 - 20.0) / float64(len(bands))
		for i := range defs {
			defs[i].loHz = 20.0 + float64(i)*bandWidth
			defs[i].hiHz = 20.0 + float64(i+1)*bandWidth
		}
	}
	prog = append(prog, eqStageSplit, float64(len(bands)))
	for i, b := range bands {
		var filters []BiquadCoeffs
		add := func(kind EQKind, hz float64) {
			if c := ComputeBiquadCoeffs(kind, sr, hz, crossoverQ, 0); c != (BiquadCoeffs{}) {
				filters = append(filters, c)
			}
		}
		if i > 0 {
			add(EQHighpass, defs[i].loHz)
			add(EQHighpass, defs[i].loHz)
		}
		if i < len(bands)-1 {
			add(EQLowpass, defs[i].hiHz)
			add(EQLowpass, defs[i].hiHz)
		}
		muted := 0.0
		if b.Muted {
			muted = 1
		}
		prog = append(prog, muted, math.Pow(10, b.GainDB/20), float64(len(filters)))
		for _, c := range filters {
			prog = append(prog, c.B0, c.B1, c.B2, c.A1, c.A2)
		}
	}
	return prog
}
