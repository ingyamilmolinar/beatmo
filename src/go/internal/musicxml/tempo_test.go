package musicxml

import "testing"

// tempo_test.go — a score's tempo is a TIMELINE, not a single number.
//
// Score.TempoBPM used to be one field overwritten by every tempo marking in the
// document, so a multi-tempo work reported whatever marking happened to appear
// LAST. gen-showcase then stamped that on an excerpt taken from somewhere else
// entirely: the Ode to Joy template (measures 92-107, which the score marks
// quarter=160) shipped at 264 — the tempo of the Prestissimo coda at measure
// 923 — and played 65% too fast.

const tempoDoc = `<?xml version="1.0"?>
<score-partwise version="4.0"><part-list><score-part id="P1"><part-name>P</part-name></score-part></part-list>
<part id="P1">
  <measure number="1">
    <attributes><divisions>1</divisions></attributes>
    <direction><direction-type><metronome><beat-unit>quarter</beat-unit><per-minute>60</per-minute></metronome></direction-type><sound tempo="60"/></direction>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><type>quarter</type></note>
  </measure>
  <measure number="2">
    <direction><direction-type><metronome><beat-unit>quarter</beat-unit><per-minute>120</per-minute></metronome></direction-type><sound tempo="120"/></direction>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><type>quarter</type></note>
  </measure>
  <measure number="3">
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><type>quarter</type></note>
  </measure>
  <measure number="4">
    <direction><direction-type><metronome><beat-unit>half</beat-unit><per-minute>100</per-minute></metronome></direction-type></direction>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><type>quarter</type></note>
  </measure>
  <measure number="5">
    <direction><direction-type><metronome><beat-unit>quarter</beat-unit><beat-unit-dot/><per-minute>80</per-minute></metronome></direction-type></direction>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><type>quarter</type></note>
  </measure>
</part></score-partwise>`

func TestTempoAtMeasureTracksTheTimeline(t *testing.T) {
	sc, err := Parse([]byte(tempoDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// TempoBPM is the OPENING tempo, not whichever marking came last.
	if sc.TempoBPM != 60 {
		t.Errorf("TempoBPM = %v, want 60 (the opening tempo)", sc.TempoBPM)
	}
	for _, c := range []struct {
		measure int
		want    float64
		why     string
	}{
		{1, 60, "explicit at m1"},
		{2, 120, "explicit at m2"},
		{3, 120, "m2's tempo is still in force"},
		{4, 200, "beat-unit half: half=100 is quarter=200"},
		{5, 120, "beat-unit quarter dotted: dotted-quarter=80 is quarter=120"},
	} {
		if got := sc.TempoAt(c.measure); got != c.want {
			t.Errorf("TempoAt(%d) = %v, want %v (%s)", c.measure, got, c.want, c.why)
		}
	}
}

// TestDominantTempoOverRange picks the single constant a fixed-BPM circuit
// should run at: the tempo covering the most measures of the excerpt, not the
// one that happens to be in force at its first bar. Ode to Joy is exactly this
// case — one bar of quarter=88 followed by fifteen of quarter=160.
func TestDominantTempoOverRange(t *testing.T) {
	sc, err := Parse([]byte(tempoDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := sc.DominantTempo(2, 3); got != 120 {
		t.Errorf("DominantTempo(2,3) = %v, want 120", got)
	}
	// m1=60 (1 bar) vs m2,m3=120 (2 bars) → 120 wins despite 60 starting it.
	if got := sc.DominantTempo(1, 3); got != 120 {
		t.Errorf("DominantTempo(1,3) = %v, want 120 (2 bars beat 1)", got)
	}
}

// movementDoc is two movements in one file, numbered 1..2 and then restarting
// at 1 — exactly how a published quartet is engraved.
const movementDoc = `<?xml version="1.0"?>
<score-partwise version="4.0"><part-list><score-part id="P1"><part-name>P</part-name></score-part></part-list>
<part id="P1">
  <measure number="1"><attributes><divisions>1</divisions></attributes>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration></note></measure>
  <measure number="2">
    <note><pitch><step>B</step><octave>3</octave></pitch><duration>1</duration></note></measure>
  <measure number="1">
    <note><pitch><step>C</step><octave>4</octave></pitch><duration>1</duration></note></measure>
  <measure number="2">
    <note><pitch><step>D</step><octave>4</octave></pitch><duration>1</duration></note></measure>
</part></score-partwise>`

// TestMovementSplitsOnRenumber — a multi-movement file restarts measure
// numbering per movement, so a measure number alone does not identify a bar.
// gen-showcase selected "measures 1-10" by printed number and so pulled those
// bars out of ALL FOUR movements of each quartet, building 336-668 bar loops
// for 8-10 bar excerpts.
func TestMovementSplitsOnRenumber(t *testing.T) {
	sc, err := Parse([]byte(movementDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sc.Parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(sc.Parts))
	}
	got := map[int][]int{} // movement → measures
	for _, n := range sc.Parts[0].Notes {
		got[n.Movement] = append(got[n.Movement], n.Measure)
	}
	if len(got) != 2 {
		t.Fatalf("movements = %d (%v), want 2 — the renumber at the third measure starts one", len(got), got)
	}
	for mv, want := range map[int][]int{1: {1, 2}, 2: {1, 2}} {
		if len(got[mv]) != len(want) {
			t.Errorf("movement %d measures = %v, want %v", mv, got[mv], want)
		}
	}
}

// TestSingleMovementIsAllMovementOne guards the common case: a score whose
// numbering never restarts must stay one movement, or every existing template
// would silently lose its music.
func TestSingleMovementIsAllMovementOne(t *testing.T) {
	sc, err := Parse([]byte(tempoDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, n := range sc.Parts[0].Notes {
		if n.Movement != 1 {
			t.Fatalf("single-movement score produced Movement=%d", n.Movement)
		}
	}
}

// numberingExceptionDoc numbers a repeat ending "X1" the way engravers do.
// It is NOT a movement boundary, and the parser must not treat it as one: the
// printed number falls back to the ordinal there, so it reads as a decrease
// against the bar before it.
const numberingExceptionDoc = `<?xml version="1.0"?>
<score-partwise version="4.0"><part-list><score-part id="P1"><part-name>P</part-name></score-part></part-list>
<part id="P1">
  <measure number="1"><attributes><divisions>1</divisions></attributes>
    <note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration></note></measure>
  <measure number="2"><note><pitch><step>B</step><octave>3</octave></pitch><duration>1</duration></note></measure>
  <measure number="X1"><note><pitch><step>C</step><octave>4</octave></pitch><duration>1</duration></note></measure>
  <measure number="3"><note><pitch><step>D</step><octave>4</octave></pitch><duration>1</duration></note></measure>
  <measure number="4"><note><pitch><step>E</step><octave>4</octave></pitch><duration>1</duration></note></measure>
</part></score-partwise>`

func TestNumberingExceptionIsNotAMovement(t *testing.T) {
	sc, err := Parse([]byte(numberingExceptionDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, n := range sc.Parts[0].Notes {
		if n.Movement != 1 {
			t.Fatalf("an X-numbered repeat ending started movement %d; the whole excerpt after it "+
				"would be dropped", n.Movement)
		}
	}
}
