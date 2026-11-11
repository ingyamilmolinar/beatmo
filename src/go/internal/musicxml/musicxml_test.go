package musicxml

import (
	"testing"
)

const tinyXML = `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>
<part id="P1"><measure number="1">
<attributes><divisions>2</divisions><key><fifths>1</fifths><mode>minor</mode></key><time><beats>3</beats><beat-type>4</beat-type></time></attributes>
<sound tempo="120"/>
<note><pitch><step>E</step><octave>3</octave></pitch><duration>2</duration><voice>1</voice></note>
<note><rest/><duration>2</duration><voice>1</voice></note>
<note><pitch><step>B</step><octave>3</octave></pitch><duration>2</duration><voice>1</voice></note>
</measure></part></score-partwise>`

func TestParse_BasicScore(t *testing.T) {
	sc, err := Parse([]byte(tinyXML))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if sc.Divisions != 2 {
		t.Errorf("Divisions: got %d, want 2", sc.Divisions)
	}
	if sc.KeyFifths != 1 {
		t.Errorf("KeyFifths: got %d, want 1", sc.KeyFifths)
	}
	if sc.Mode != "minor" {
		t.Errorf("Mode: got %q, want %q", sc.Mode, "minor")
	}
	if sc.TempoBPM != 120 {
		t.Errorf("TempoBPM: got %v, want 120", sc.TempoBPM)
	}
	if sc.TimeBeats != 3 {
		t.Errorf("TimeBeats: got %d, want 3", sc.TimeBeats)
	}
	if sc.TimeBeatType != 4 {
		t.Errorf("TimeBeatType: got %d, want 4", sc.TimeBeatType)
	}
	if len(sc.Parts) != 1 {
		t.Fatalf("Parts: got %d, want 1", len(sc.Parts))
	}
	notes := sc.Parts[0].Notes
	if len(notes) != 3 {
		t.Fatalf("Notes: got %d, want 3", len(notes))
	}
	// n[0]: E3 -> midi = (3+1)*12 + 4 + 0 = 52; Semitone = 52 - 57 = -5
	n0 := notes[0]
	if n0.Semitone != -5 {
		t.Errorf("n[0].Semitone: got %d, want -5", n0.Semitone)
	}
	if n0.OnsetDiv != 0 {
		t.Errorf("n[0].OnsetDiv: got %d, want 0", n0.OnsetDiv)
	}
	if n0.Rest {
		t.Errorf("n[0].Rest: got true, want false")
	}
	// n[1]: rest, onset 2
	n1 := notes[1]
	if !n1.Rest {
		t.Errorf("n[1].Rest: got false, want true")
	}
	if n1.OnsetDiv != 2 {
		t.Errorf("n[1].OnsetDiv: got %d, want 2", n1.OnsetDiv)
	}
	if n1.Semitone != 0 {
		t.Errorf("n[1].Semitone: got %d, want 0 (rest)", n1.Semitone)
	}
	// n[2]: B3 -> midi = (3+1)*12 + 11 + 0 = 59; Semitone = 59 - 57 = 2
	n2 := notes[2]
	if n2.Semitone != 2 {
		t.Errorf("n[2].Semitone: got %d, want 2", n2.Semitone)
	}
	if n2.OnsetDiv != 4 {
		t.Errorf("n[2].OnsetDiv: got %d, want 4", n2.OnsetDiv)
	}
}

const chordXML = `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>
<part id="P1"><measure number="1">
<attributes><divisions>1</divisions></attributes>
<note><pitch><step>E</step><octave>3</octave></pitch><duration>1</duration><voice>1</voice></note>
<note><chord/><pitch><step>G</step><octave>3</octave></pitch><duration>1</duration><voice>1</voice></note>
<note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration><voice>1</voice></note>
</measure></part></score-partwise>`

func TestParse_ChordSharesOnset(t *testing.T) {
	// E3, then <chord/>G3, then A3: onsets 0, 0, 1
	// n[1].Chord true; n[2].Semitone==0 (A3 = MIDI 57, offset 0)
	sc, err := Parse([]byte(chordXML))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	notes := sc.Parts[0].Notes
	if len(notes) != 3 {
		t.Fatalf("Notes: got %d, want 3", len(notes))
	}
	// n[0]: E3, onset 0
	if notes[0].OnsetDiv != 0 {
		t.Errorf("n[0].OnsetDiv: got %d, want 0", notes[0].OnsetDiv)
	}
	// n[1]: G3 with chord, onset 0
	if !notes[1].Chord {
		t.Errorf("n[1].Chord: got false, want true")
	}
	if notes[1].OnsetDiv != 0 {
		t.Errorf("n[1].OnsetDiv: got %d, want 0", notes[1].OnsetDiv)
	}
	// n[2]: A3 onset 1; A3 midi=57 -> Semitone=0
	if notes[2].OnsetDiv != 1 {
		t.Errorf("n[2].OnsetDiv: got %d, want 1", notes[2].OnsetDiv)
	}
	if notes[2].Semitone != 0 {
		t.Errorf("n[2].Semitone: got %d, want 0 (A3)", notes[2].Semitone)
	}
}

const alterXML = `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>
<part id="P1"><measure number="1">
<attributes><divisions>1</divisions></attributes>
<note><pitch><step>F</step><alter>1</alter><octave>3</octave></pitch><duration>1</duration><voice>1</voice></note>
<note><pitch><step>A</step><octave>4</octave></pitch><duration>1</duration><voice>1</voice></note>
</measure></part></score-partwise>`

func TestParse_AlterAndOctave(t *testing.T) {
	// F#3: midi = (3+1)*12 + 5 + 1 = 54; Semitone = 54 - 57 = -3
	// A4:  midi = (4+1)*12 + 9 + 0 = 69; Semitone = 69 - 57 = 12
	sc, err := Parse([]byte(alterXML))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	notes := sc.Parts[0].Notes
	if len(notes) != 2 {
		t.Fatalf("Notes: got %d, want 2", len(notes))
	}
	if notes[0].Semitone != -3 {
		t.Errorf("F#3 Semitone: got %d, want -3", notes[0].Semitone)
	}
	if notes[1].Semitone != 12 {
		t.Errorf("A4 Semitone: got %d, want 12", notes[1].Semitone)
	}
}

const backupXML = `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>
<part id="P1"><measure number="1">
<attributes><divisions>1</divisions></attributes>
<note><pitch><step>E</step><octave>3</octave></pitch><duration>1</duration><voice>1</voice></note>
<backup><duration>1</duration></backup>
<note><pitch><step>B</step><octave>3</octave></pitch><duration>1</duration><voice>2</voice></note>
</measure></part></score-partwise>`

func TestParse_BackupMultiVoice(t *testing.T) {
	// voice1: E3 dur1 (onset0); <backup><duration>1</duration></backup>; voice2: B3 dur1 (onset0)
	sc, err := Parse([]byte(backupXML))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	notes := sc.Parts[0].Notes
	if len(notes) != 2 {
		t.Fatalf("Notes: got %d, want 2", len(notes))
	}
	// voice1 E3 at onset 0
	if notes[0].OnsetDiv != 0 {
		t.Errorf("voice1 E3 OnsetDiv: got %d, want 0", notes[0].OnsetDiv)
	}
	if notes[0].Voice != 1 {
		t.Errorf("voice1 E3 Voice: got %d, want 1", notes[0].Voice)
	}
	// voice2 B3 also at onset 0 after backup
	if notes[1].OnsetDiv != 0 {
		t.Errorf("voice2 B3 OnsetDiv: got %d, want 0", notes[1].OnsetDiv)
	}
	if notes[1].Voice != 2 {
		t.Errorf("voice2 B3 Voice: got %d, want 2", notes[1].Voice)
	}
}
