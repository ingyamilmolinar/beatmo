package ui

// Said the way a non-musician would say it, and a change as a change; no digit, unit or note name ever
// reaches this readout.

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// joinWords joins phrases with the readout separator.
func joinWords(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += p
	}
	return out
}

// "until the note ends" when it still sounds where the voice stops.
func lengthWords(a *soundview.Analysis, voiceSec float64) string {
	s := a.AudibleSec()
	var w string
	switch {
	case s < 0.12:
		w = i18n.T(i18n.KeyGraphWordsLenHit)
	case s < 0.35:
		w = i18n.T(i18n.KeyGraphWordsLenShort)
	case s < 0.8:
		w = i18n.T(i18n.KeyGraphWordsLenMedium)
	case s < 1.6:
		w = i18n.T(i18n.KeyGraphWordsLenLong)
	default:
		w = i18n.T(i18n.KeyGraphWordsLenVeryLong)
	}
	if voiceSec > 0 && s >= 0.95*voiceSec {
		w = i18n.Tf(i18n.KeyGraphWordsToTheEnd, w)
	}
	return w
}

// attackWords classes how a note starts, and names a click.
func attackWords(a *soundview.Analysis) string {
	s := a.AttackSec()
	var w string
	switch {
	case s < 0.005:
		w = i18n.T(i18n.KeyGraphWordsAtkInstant)
	case s < 0.03:
		w = i18n.T(i18n.KeyGraphWordsAtkQuick)
	case s < 0.12:
		w = i18n.T(i18n.KeyGraphWordsAtkGentle)
	default:
		w = i18n.T(i18n.KeyGraphWordsAtkSlow)
	}
	if a.ClickDB() >= clickWordsDB {
		w = i18n.Tf(i18n.KeyGraphWordsWithClick, w)
	}
	return w
}

// clickWordsDB is how far a start must stand over the body to read as a click.
const clickWordsDB = 6.0

// brightnessWords classes a brightness centre.
func brightnessWords(hz float64) string {
	switch {
	case hz <= 0:
		return ""
	case hz < 300:
		return i18n.T(i18n.KeyGraphWordsDeep)
	case hz < 700:
		return i18n.T(i18n.KeyGraphWordsWarm)
	case hz < 1800:
		return i18n.T(i18n.KeyGraphWordsBalanced)
	case hz < 4500:
		return i18n.T(i18n.KeyGraphWordsBright)
	}
	return i18n.T(i18n.KeyGraphWordsVeryBright)
}

// noiseWords classes the noise share 0..1.
func noiseWords(f float64) string {
	switch {
	case f < 0.10:
		return i18n.T(i18n.KeyGraphWordsClean)
	case f < 0.35:
		return i18n.T(i18n.KeyGraphWordsSomeHiss)
	case f < 0.65:
		return i18n.T(i18n.KeyGraphWordsHalfNoise)
	}
	return i18n.T(i18n.KeyGraphWordsMostlyNoise)
}

// harmonicsWords classes how many harmonics stand above −40 dB.
func harmonicsWords(n int) string {
	switch {
	case n <= 2:
		return i18n.T(i18n.KeyGraphWordsPure)
	case n <= 6:
		return i18n.T(i18n.KeyGraphWordsSomeOvertones)
	}
	return i18n.T(i18n.KeyGraphWordsRich)
}

// trendWords classes how the brightness moves over the note.
func trendWords(start, end float64) string {
	switch {
	case start <= 0 || end <= 0:
		return ""
	case end < 0.8*start:
		return i18n.T(i18n.KeyGraphWordsGetsDarker)
	case end > 1.25*start:
		return i18n.T(i18n.KeyGraphWordsGetsBrighter)
	}
	return i18n.T(i18n.KeyGraphWordsKeepsColour)
}

// shapeWords classes a wave's grit (waveGrit: 0 for a sine, 1 for a square).
func shapeWords(grit float64) string {
	switch {
	case grit < gritSlight:
		return i18n.T(i18n.KeyGraphWordsSmooth)
	case grit < gritSquared:
		return i18n.T(i18n.KeyGraphWordsSlightlySquared)
	}
	return i18n.T(i18n.KeyGraphWordsSquared)
}

// A sine driven three times into a clip, an unmistakable square, measures ~0.6.
const (
	gritSlight  = 0.2
	gritSquared = 0.5
)

// pitchWords classes the pitch motion.
func pitchWords(m pitchMotion) string {
	switch m.kind {
	case pitchGlide:
		if m.endCents < m.startCents {
			switch {
			case m.settleSec < 0.06:
				return i18n.T(i18n.KeyGraphWordsPitchDropFast)
			case m.settleSec < 0.25:
				return i18n.T(i18n.KeyGraphWordsPitchDrop)
			}
			return i18n.T(i18n.KeyGraphWordsPitchDropSlow)
		}
		return i18n.T(i18n.KeyGraphWordsPitchRise)
	case pitchWobble:
		if m.depthCents < 25 {
			return i18n.T(i18n.KeyGraphWordsPitchWobbleGentle)
		}
		return i18n.T(i18n.KeyGraphWordsPitchWobbleLot)
	case pitchHold:
		return i18n.T(i18n.KeyGraphWordsPitchSteady)
	}
	return i18n.T(i18n.KeyGraphNoPitch)
}

// "" when the move is below both rel (relative) and abs (absolute).
func changeWords(before, now, rel, abs float64, up, down i18n.Key) string {
	d := now - before
	if math.Abs(d) < abs || math.Abs(d) < rel*math.Abs(before) {
		return ""
	}
	if d > 0 {
		return i18n.T(up)
	}
	return i18n.T(down)
}
