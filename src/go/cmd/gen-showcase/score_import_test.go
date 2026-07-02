package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

func TestScoreToShowcase_PitchStepVoiceRows(t *testing.T) {
	sc := musicxml.Score{Divisions: 2, TempoBPM: 120, TimeBeats: 3, TimeBeatType: 4, Parts: []musicxml.Part{{ID: "P1", Notes: []musicxml.Note{
		{Semitone: -5, OnsetDiv: 0, DurationDiv: 2, Voice: 1},
		{Semitone: 2, OnsetDiv: 2, DurationDiv: 2, Voice: 1},
		{Semitone: 2, OnsetDiv: 0, DurationDiv: 6, Voice: 2},
	}}}}
	sh := scoreToShowcase(sc, "asturias", "guitar-nylon", "Nylon Guitar", 16)
	if sh.BPM != 120 || sh.Subdiv != 16 {
		t.Fatalf("bpm/subdiv %+v", sh)
	}
	if len(sh.Rows) != 2 || len(sh.Insts) != 2 {
		t.Fatalf("want 2 rows+insts: %d/%d", len(sh.Rows), len(sh.Insts))
	}
	if sh.Rows[0].Inst != "guitar-nylon" || sh.Insts[0].ID != "guitar-nylon" {
		t.Fatalf("inst wiring %+v", sh.Rows[0])
	}
	r1 := sh.Rows[0].Hits
	if len(r1) != 2 || r1[0].Pitch != -5 || r1[0].Step != 0 || r1[1].Step != 4 {
		t.Fatalf("voice1 hits %+v", r1)
	}
	if sh.Rows[1].Hits[0].Pitch != 2 || sh.Rows[1].Hits[0].Step != 0 {
		t.Fatalf("voice2 pedal %+v", sh.Rows[1])
	}
}

func TestScoreToShowcase_AudibleInstrumentVolume(t *testing.T) {
	// A converted instrument left at Volume==0 exports "volume":0, which import.go
	// copies straight to row.Volume (no 0→default), silencing the row. Converted
	// instruments must be audible (>0).
	sc := musicxml.Score{Divisions: 1, TempoBPM: 120, Parts: []musicxml.Part{{Notes: []musicxml.Note{
		{Semitone: 0, OnsetDiv: 0, DurationDiv: 1, Voice: 1},
	}}}}
	sh := scoreToShowcase(sc, "x", "guitar-nylon", "Nylon Guitar", 16)
	for i, in := range sh.Insts {
		if in.Volume <= 0 {
			t.Errorf("inst %d (%s) volume=%v; converted instruments must be audible (>0) or the imported row is silent", i, in.ID, in.Volume)
		}
	}
}

func TestScoreToShowcase_SkipsRests(t *testing.T) {
	sc := musicxml.Score{Divisions: 1, Parts: []musicxml.Part{{Notes: []musicxml.Note{
		{Rest: true, OnsetDiv: 0, DurationDiv: 1, Voice: 1}, {Semitone: 7, OnsetDiv: 1, DurationDiv: 1, Voice: 1}}}}}
	sh := scoreToShowcase(sc, "x", "guitar-nylon", "G", 16)
	if len(sh.Rows[0].Hits) != 1 || sh.Rows[0].Hits[0].Pitch != 7 {
		t.Fatalf("rest not skipped %+v", sh.Rows)
	}
}

func TestScoreToShowcase_GuardDivisionsZero(t *testing.T) {
	sc := musicxml.Score{Divisions: 0, TempoBPM: 100, Parts: []musicxml.Part{{Notes: []musicxml.Note{
		{Semitone: 0, OnsetDiv: 0, DurationDiv: 4, Voice: 1},
	}}}}
	sh := scoreToShowcase(sc, "empty", "guitar-nylon", "G", 16)
	if sh.Stem != "empty" || sh.Subdiv != 16 || sh.Bars != 1 {
		t.Fatalf("guard divisions<=0: %+v", sh)
	}
	if len(sh.Rows) != 0 && len(sh.Insts) != 0 {
		t.Fatalf("guard should produce empty rows, got %+v", sh)
	}
}

func TestScoreToShowcase_BarsComputed(t *testing.T) {
	// 2 notes at steps 0 and 15 (last step in bar 0): maxStep=15 → bars=(15/16)+1=1
	// 1 note at step 16 (bar 1): maxStep=16 → bars=(16/16)+1=2
	sc := musicxml.Score{Divisions: 4, TempoBPM: 120, Parts: []musicxml.Part{{Notes: []musicxml.Note{
		// step = int(round(onsetDiv / divisions * stepsPerQuarter))
		// stepsPerQuarter = 16/4 = 4
		// OnsetDiv=0 → step 0
		// OnsetDiv=16 → step 4*4=16
		{Semitone: 0, OnsetDiv: 0, DurationDiv: 4, Voice: 1},
		{Semitone: 1, OnsetDiv: 16, DurationDiv: 4, Voice: 1},
	}}}}
	sh := scoreToShowcase(sc, "bars-test", "guitar-nylon", "G", 16)
	// maxStep = 16, Bars = (16/16)+1 = 2
	if sh.Bars != 2 {
		t.Fatalf("expected 2 bars, got %d", sh.Bars)
	}
}

func TestScoreToShowcase_CollisionAdvances(t *testing.T) {
	// Two notes in the same voice mapping to the same step should advance the second.
	sc := musicxml.Score{Divisions: 1, TempoBPM: 120, Parts: []musicxml.Part{{Notes: []musicxml.Note{
		// Both onset 0, divisions=1, stepsPerQuarter=4 → both step 0.
		// Second should be advanced to step 1.
		{Semitone: 5, OnsetDiv: 0, DurationDiv: 1, Voice: 1},
		{Semitone: 7, OnsetDiv: 0, DurationDiv: 1, Voice: 1},
	}}}}
	sh := scoreToShowcase(sc, "coll", "guitar-nylon", "G", 16)
	hits := sh.Rows[0].Hits
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Step != 0 || hits[1].Step != 1 {
		t.Fatalf("collision not handled: steps=%d,%d", hits[0].Step, hits[1].Step)
	}
}

func TestScoreToShowcase_MultiPartVoiceOrder(t *testing.T) {
	// Two parts, each with two voices → rows in order: part0/voice1, part0/voice2, part1/voice1.
	sc := musicxml.Score{Divisions: 1, TempoBPM: 90, Parts: []musicxml.Part{
		{ID: "P1", Notes: []musicxml.Note{
			{Semitone: 1, OnsetDiv: 0, DurationDiv: 1, Voice: 2},
			{Semitone: 0, OnsetDiv: 0, DurationDiv: 1, Voice: 1},
		}},
		{ID: "P2", Notes: []musicxml.Note{
			{Semitone: 3, OnsetDiv: 0, DurationDiv: 1, Voice: 1},
		}},
	}}
	sh := scoreToShowcase(sc, "multi", "guitar-nylon", "G", 16)
	if len(sh.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(sh.Rows))
	}
	// Row0 = part0 voice1, Row1 = part0 voice2, Row2 = part1 voice1
	if sh.Rows[0].Hits[0].Pitch != 0 {
		t.Fatalf("row0 should be voice1 pitch=0, got %+v", sh.Rows[0])
	}
	if sh.Rows[1].Hits[0].Pitch != 1 {
		t.Fatalf("row1 should be voice2 pitch=1, got %+v", sh.Rows[1])
	}
	if sh.Rows[2].Hits[0].Pitch != 3 {
		t.Fatalf("row2 should be part1 voice1 pitch=3, got %+v", sh.Rows[2])
	}
}
