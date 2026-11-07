package musicxml

import "testing"

// A score's tempo is a TIMELINE: one field overwritten by every marking reports whichever appears last, not what the
// excerpt's measures are marked.

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

// One bar of quarter=88, then fifteen of 160: the tempo covering most of the excerpt, not the first bar's.
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

// Numbering restarts per movement, so a measure number alone pulls bars from every movement.
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

// A score whose numbering never restarts must stay one movement, or every template silently loses its music.
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

// An engraver's "X1" repeat ending, NOT a movement boundary, although its ordinal fallback reads as a decrease.
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
