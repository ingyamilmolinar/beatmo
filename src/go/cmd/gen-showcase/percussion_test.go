package main

import (
	"regexp"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

// Timpani are PITCHED notes in MusicXML, so only the part's name tells them apart; an unpitched part announces itself.
var percussionPartName = regexp.MustCompile(`(?i)timpan|kettle|pauke|drum|cymbal|becken|triang|tambour|tamburin|percussion|schlagwerk|snare|tam-tam|gong|castanet|woodblock|cajon|conga|bongo|clave`)

func isPercussionPart(p musicxml.Part) bool {
	if percussionPartName.MatchString(p.Name) {
		return true
	}
	for _, n := range p.Notes {
		if n.Unpitched {
			return true
		}
	}
	return false
}

// A percussion part that SOUNDS inside the selected measures must be mapped to rows: a drum machine's templates must
// not drop the score's drums.
func TestEveryPercussionPartInRangeIsMapped(t *testing.T) {
	for _, spec := range templateSpecs() {
		sc := mustScore(t, spec.ScoreID)
		mapped := map[int]bool{}
		for _, pm := range spec.Parts {
			mapped[pm.Index] = true
		}
		for i, p := range sc.Parts {
			if !isPercussionPart(p) || mapped[i] {
				continue
			}
			sounding := 0
			for _, n := range p.Notes {
				if !n.Rest && inRange(n, spec) {
					sounding++
				}
			}
			if sounding > 0 {
				t.Errorf("%s: percussion part %d %q plays %d notes in measures %d..%d but is not mapped",
					spec.Stem, i, p.Name, sounding, spec.From, spec.To)
			}
		}
	}
}

// Every template maps a percussion part of its own score or loops a groove, unless its score has none sounding there.
func TestGrooveTemplatesHaveDrumRows(t *testing.T) {
	for _, spec := range templateSpecs() {
		if spec.Groove == nil {
			continue
		}
		sh, _, err := buildFromScore(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec.Stem, err)
		}
		kit := map[string]bool{}
		for _, pm := range spec.Groove.Parts {
			for _, kp := range pm.Kit {
				kit[string(kp.Inst)] = true
			}
		}
		drums := 0
		for _, r := range sh.Rows {
			if kit[string(r.Inst)] {
				drums++
			}
		}
		if drums < 2 {
			t.Errorf("%s: %d drum rows from its groove; a groove template needs a beat", spec.Stem, drums)
		}
	}
}
