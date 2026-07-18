package ui

import (
	"strconv"
	"strings"
)

// positionValueSpec builds the beat-counter editor spec. The carried value is
// an ABSOLUTE SUBDIVISION STEP (integer, stored in the float64). Text grammar:
//
//	"B"      -> beat B, subdivision 0
//	"B.S"    -> beat B, subdivision index S (0-based, clamped to [0, div-1])
//
// Parsing operates on the raw string (not a float), so "17.10" is unambiguous
// (beat 17, subdiv 10) — a float editor could not distinguish .10 from .1.
func positionValueSpec(div, maxSteps int) ValueSpec {
	if div <= 0 {
		div = 1
	}
	return ValueSpec{
		MinW:   64,
		MaxLen: 8,
		Format: func(v float64) string {
			steps := int(v)
			if steps < 0 {
				steps = 0
			}
			beat := steps / div
			sub := steps % div
			if sub == 0 {
				return strconv.Itoa(beat)
			}
			return strconv.Itoa(beat) + "." + strconv.Itoa(sub)
		},
		Parse: func(text string) (float64, bool) {
			text = strings.TrimSpace(text)
			if text == "" {
				return 0, false
			}
			beatPart, subPart := text, ""
			if i := strings.IndexByte(text, '.'); i >= 0 {
				beatPart, subPart = text[:i], text[i+1:]
			}
			beat, err := strconv.Atoi(strings.TrimSpace(beatPart))
			if err != nil || beat < 0 {
				return 0, false
			}
			sub := 0
			if subPart != "" {
				s, err := strconv.Atoi(strings.TrimSpace(subPart))
				if err != nil || s < 0 {
					return 0, false
				}
				sub = s
			}
			if sub > div-1 {
				sub = div - 1 // clamp to the last subdivision in the beat
			}
			steps := beat*div + sub
			if steps < 0 {
				steps = 0
			}
			if maxSteps > 0 && steps > maxSteps {
				steps = maxSteps
			}
			return float64(steps), true
		},
	}
}
