package main

import (
	_ "embed"
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

//go:embed scores/asturias-opening.musicxml
var asturiasMusicXML []byte

// defaultScoreInstVolume is the playback gain assigned to every instrument the
// MusicXML converter emits. MusicXML carries no mixing levels, so we pick a sane
// audible default in line with the hand-authored templates (0.4–0.85).
const defaultScoreInstVolume = 0.8

func asturiasShowcase() Showcase {
	sc, err := musicxml.Parse(asturiasMusicXML)
	if err != nil {
		panic("gen-showcase: parse asturias-opening.musicxml: " + err.Error())
	}
	return scoreToShowcase(sc, "asturias", "guitar-nylon", "Nylon Guitar", 16)
}

// scoreToShowcase converts a parsed MusicXML Score into a Beatmo Showcase.
//
// Each (partIndex, voice) pair becomes one RowSpec (and a corresponding InstSpec).
// Parts are processed in order; within a part, voices are emitted in ascending
// voice-number order.
//
// step mapping: stepsPerQuarter = subdiv / 4
//
//	step = round(onsetDiv / divisions * stepsPerQuarter)
//
// Rest notes are skipped. Step collisions within a row advance to the next free step.
// Guard: if divisions <= 0, returns an empty Showcase with Bars=1.
func scoreToShowcase(sc musicxml.Score, stem, instID, instName string, subdiv int) Showcase {
	partInsts := make([]InstSpec, len(sc.Parts))
	for i := range partInsts {
		partInsts[i] = InstSpec{ID: instID, Name: instName, Volume: defaultScoreInstVolume}
	}
	return scoreToShowcaseMulti(sc, stem, subdiv, partInsts)
}

// scoreToShowcaseMulti is the multi-instrument generalization of
// scoreToShowcase: part k plays partInsts[k] (every voice of that part gets its
// own row carrying the part's InstSpec — pan/sends/synth params included).
// len(partInsts) must equal len(sc.Parts); a mismatch is an authoring error and
// panics at generation time. An InstSpec with Volume 0 gets
// defaultScoreInstVolume (a 0 volume silences the imported row).
func scoreToShowcaseMulti(sc musicxml.Score, stem string, subdiv int, partInsts []InstSpec) Showcase {
	if len(partInsts) != len(sc.Parts) {
		panic(fmt.Sprintf("scoreToShowcaseMulti(%s): %d partInsts for %d score parts", stem, len(partInsts), len(sc.Parts)))
	}
	if sc.Divisions <= 0 {
		return Showcase{Stem: stem, Subdiv: subdiv, Bars: 1}
	}

	stepsPerQuarter := subdiv / 4

	// rowKey identifies a unique (partIndex, voice) combination.
	type rowKey struct {
		partIdx int
		voice   int
	}

	// Collect notes per rowKey, preserving insertion order within each group.
	type noteEntry struct {
		key  rowKey
		note musicxml.Note
	}

	// Track order of first-seen keys.
	keyOrder := []rowKey{}
	keySet := map[rowKey]bool{}
	notesByKey := map[rowKey][]musicxml.Note{}

	for pi, part := range sc.Parts {
		// Collect distinct voices for this part in order of first appearance,
		// then sort them ascending so that voice order is stable.
		partVoices := []int{}
		partVoiceSet := map[int]bool{}
		for _, n := range part.Notes {
			if !partVoiceSet[n.Voice] {
				partVoiceSet[n.Voice] = true
				partVoices = append(partVoices, n.Voice)
			}
		}
		sort.Ints(partVoices)

		// Register keys in ascending voice order for this part.
		for _, v := range partVoices {
			k := rowKey{partIdx: pi, voice: v}
			if !keySet[k] {
				keySet[k] = true
				keyOrder = append(keyOrder, k)
			}
		}

		// Bucket notes by key.
		for _, n := range part.Notes {
			k := rowKey{partIdx: pi, voice: n.Voice}
			notesByKey[k] = append(notesByKey[k], n)
		}
	}

	// Build rows and insts in key order.
	rows := make([]RowSpec, 0, len(keyOrder))
	insts := make([]InstSpec, 0, len(keyOrder))

	maxStep := 0

	for rowIdx, k := range keyOrder {
		notes := notesByKey[k]
		used := map[int]bool{}
		var hits []Hit

		for _, n := range notes {
			if n.Rest {
				continue
			}
			step := int(math.Round(float64(n.OnsetDiv) / float64(sc.Divisions) * float64(stepsPerQuarter)))
			origStep := step
			for used[step] {
				step++
			}
			if step != origStep {
				log.Printf("score: collision in row %d at step %d -> %d", rowIdx, origStep, step)
			}
			used[step] = true

			durBeats := float64(n.DurationDiv) / float64(sc.Divisions)
			hits = append(hits, Hit{
				Step:  step,
				Pitch: float64(n.Semitone),
				Dur:   durBeats,
				// Vol left at 0 = instrument default
			})

			if step > maxStep {
				maxStep = step
			}
		}

		ins := partInsts[k.partIdx]
		if ins.Volume <= 0 {
			// Volume must be > 0: the exporter writes instrument volume with
			// json:"volume" (no omitempty) and import.go copies it straight to
			// row.Volume without a 0→default fallback, so a 0 here silences the
			// row on load.
			ins.Volume = defaultScoreInstVolume
		}
		rows = append(rows, RowSpec{Inst: ins.ID, Hits: hits})
		insts = append(insts, ins)
	}

	bars := (maxStep / subdiv) + 1
	if bars < 1 {
		bars = 1
	}

	return Showcase{
		Stem:   stem,
		BPM:    int(math.Round(sc.TempoBPM)),
		Subdiv: subdiv,
		Bars:   bars,
		Insts:  insts,
		Rows:   rows,
	}
}
