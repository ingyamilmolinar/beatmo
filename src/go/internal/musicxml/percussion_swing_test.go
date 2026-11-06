package musicxml

import "testing"

// drumDoc is a two-instrument drum part in the shape MuseScore and Finale
// export: kit pieces declared in the part-list with 1-based <midi-unpitched>,
// notes naming them through <instrument id>.
func drumDoc(measures string) string {
	return `<?xml version="1.0"?><score-partwise>` +
		`<part-list><score-part id="P1"><part-name>Drumset</part-name>` +
		`<score-instrument id="P1-I36"><instrument-name>Bass Drum 1</instrument-name></score-instrument>` +
		`<score-instrument id="P1-I39"><instrument-name>Acoustic Snare</instrument-name></score-instrument>` +
		`<midi-instrument id="P1-I36"><midi-channel>10</midi-channel><midi-unpitched>37</midi-unpitched></midi-instrument>` +
		`<midi-instrument id="P1-I39"><midi-channel>10</midi-channel><midi-unpitched>39</midi-unpitched></midi-instrument>` +
		`</score-part></part-list>` +
		`<part id="P1">` + measures + `</part></score-partwise>`
}

func drumNote(inst, step, oct, dur, extra string) string {
	return `<note><unpitched><display-step>` + step + `</display-step><display-octave>` + oct +
		`</display-octave></unpitched><duration>` + dur + `</duration><instrument id="` + inst +
		`"/><voice>1</voice>` + extra + `</note>`
}

func TestParse_UnpitchedNotesCarryKitPieceAndGMKey(t *testing.T) {
	doc := drumDoc(`<measure number="1"><attributes><divisions>4</divisions>` +
		`<time><beats>4</beats><beat-type>4</beat-type></time></attributes>` +
		drumNote("P1-I36", "F", "4", "4", "") +
		drumNote("P1-I39", "C", "5", "4", "") +
		drumNote("P1-I36", "F", "4", "4", "") +
		drumNote("P1-I39", "C", "5", "4", "") +
		`</measure>`)
	p := parseOne(t, doc)
	if len(p.Instruments) != 2 {
		t.Fatalf("instruments: got %v", p.Instruments)
	}
	if got := p.Instruments["P1-I36"]; got.Name != "Bass Drum 1" || got.PercKey != 36 {
		t.Errorf("kick instrument = %+v, want name Bass Drum 1, GM key 36 (midi-unpitched 37 is 1-based)", got)
	}
	if len(p.Notes) != 4 {
		t.Fatalf("notes: got %d want 4", len(p.Notes))
	}
	want := []int{36, 38, 36, 38}
	for i, n := range p.Notes {
		if !n.Unpitched {
			t.Errorf("note %d: Unpitched=false", i)
		}
		if n.PercKey != want[i] {
			t.Errorf("note %d: PercKey=%d want %d", i, n.PercKey, want[i])
		}
		if n.OnsetDiv != i*4 {
			t.Errorf("note %d: onset %d want %d", i, n.OnsetDiv, i*4)
		}
	}
}

// A drum part that declares a single instrument may leave <instrument> off its
// notes; they belong to that instrument.
func TestParse_SoleInstrumentIsTheDefaultKitPiece(t *testing.T) {
	doc := `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"><part-name>Triangle</part-name>` +
		`<score-instrument id="P1-X"><instrument-name>Triangle</instrument-name></score-instrument>` +
		`<midi-instrument id="P1-X"><midi-unpitched>82</midi-unpitched></midi-instrument>` +
		`</score-part></part-list><part id="P1"><measure number="1"><attributes><divisions>1</divisions></attributes>` +
		`<note><unpitched><display-step>E</display-step><display-octave>4</display-octave></unpitched><duration>1</duration></note>` +
		`</measure></part></score-partwise>`
	p := parseOne(t, doc)
	if len(p.Notes) != 1 || p.Notes[0].Instrument != "P1-X" || p.Notes[0].PercKey != 81 {
		t.Fatalf("got %+v, want the sole instrument P1-X with GM key 81", p.Notes)
	}
}

// Two kit pieces on the same staff line must not tie into each other: a tie
// is resolved per instrument.
func TestParse_UnpitchedTiesResolvePerInstrument(t *testing.T) {
	doc := drumDoc(`<measure number="1"><attributes><divisions>1</divisions></attributes>` +
		drumNote("P1-I36", "F", "4", "1", `<tie type="start"/>`) +
		drumNote("P1-I39", "F", "4", "1", `<tie type="stop"/>`) +
		`</measure>`)
	p := parseOne(t, doc)
	if len(p.Notes) != 2 {
		t.Fatalf("a tie crossed kit pieces: got %d notes, want 2", len(p.Notes))
	}
}

func TestParse_SoundSwingRecorded(t *testing.T) {
	p := parseOne(t, wrap(``,
		`<direction><direction-type><words>Swing</words></direction-type>`+
			`<sound><swing><first>3</first><second>1</second><swing-type>16th</swing-type></swing></sound></direction>`+
			note("A", "3", "4", "")+`</measure>`+
			`<measure number="2"><sound><swing><straight/></swing></sound>`+note("A", "3", "4", "")+`</measure>`))
	var kinds []DirectionKind
	for _, d := range p.Directions {
		kinds = append(kinds, d.Kind)
	}
	// The words AND the <sound> both state the swing; the <sound> is the
	// precise one (3:1 sixteenths) and comes after the words at the same onset.
	sw, ok := p.SwingAt(0)
	if !ok {
		t.Fatalf("no swing in force at onset 0; directions %v", kinds)
	}
	if sw.SwingRatio != 3 || sw.SwingUnit != 0.25 {
		t.Errorf("swing = ratio %v unit %v, want 3 and 0.25 (16th)", sw.SwingRatio, sw.SwingUnit)
	}
	if _, ok := p.SwingAt(4); ok {
		t.Errorf("swing still in force after <straight/>")
	}
}

func TestSwingWords(t *testing.T) {
	cases := []struct {
		text string
		kind DirectionKind
		unit float64
	}{
		{"Swing", DirSwing, 0.5},
		{"Swung", DirSwing, 0.5},
		{"Shuffle", DirSwing, 0.5},
		{"Swing 16ths", DirSwing, 0.25},
		{"16th swing", DirSwing, 0.25},
		{"Straight 8ths", DirStraight, 0},
		{"(straight)", DirStraight, 0},
		{"Even 8ths", DirStraight, 0},
	}
	for _, c := range cases {
		d, ok := swingWords(c.text)
		if !ok || d.Kind != c.kind || d.SwingUnit != c.unit {
			t.Errorf("swingWords(%q) = %+v ok=%v, want kind %v unit %v", c.text, d, ok, c.kind, c.unit)
		}
	}
	for _, text := range []string{"dolce", "rit.", "a tempo", ""} {
		if _, ok := swingWords(text); ok {
			t.Errorf("swingWords(%q) matched", text)
		}
	}
}

func TestParse_ArpeggiateBreathAndCaesura(t *testing.T) {
	p := parseOne(t, wrap(``,
		note("C", "4", "4", `<notations><arpeggiate/></notations>`)+
			note("E", "4", "4", `<chord/><notations><arpeggiate direction="down"/></notations>`)+
			note("G", "4", "4", `<notations><articulations><breath-mark/></articulations></notations>`)+
			note("A", "4", "4", `<notations><articulations><caesura/></articulations></notations>`)+
			`</measure>`))
	if len(p.Notes) != 4 {
		t.Fatalf("notes: %d", len(p.Notes))
	}
	if p.Notes[0].Articulations.Arpeggiate != 1 || p.Notes[1].Articulations.Arpeggiate != -1 {
		t.Errorf("arpeggiate: got %d/%d want 1/-1", p.Notes[0].Articulations.Arpeggiate, p.Notes[1].Articulations.Arpeggiate)
	}
	if !p.Notes[2].Articulations.BreathMark || !p.Notes[3].Articulations.Caesura {
		t.Errorf("breath/caesura not recorded: %+v %+v", p.Notes[2].Articulations, p.Notes[3].Articulations)
	}
}

// The meter in force is a timeline: a 4/4 bar, a 7/8 bar and a pickup that is
// shorter than its meter each record their own length.
func TestParse_MeasureTimelineTracksMeterChangesAndPickups(t *testing.T) {
	doc := `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"><part-name>X</part-name></score-part></part-list>` +
		`<part id="P1">` +
		`<measure number="0" implicit="yes"><attributes><divisions>2</divisions><time><beats>4</beats><beat-type>4</beat-type></time></attributes>` +
		note("A", "3", "2", "") + `</measure>` +
		`<measure number="1">` + note("A", "3", "8", "") + `</measure>` +
		`<measure number="2"><attributes><time><beats>3+2+2</beats><beat-type>8</beat-type></time></attributes>` +
		note("A", "3", "7", "") + `</measure>` +
		`</part></score-partwise>`
	sc, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	ms := sc.Parts[0].Measures
	if len(ms) != 3 {
		t.Fatalf("measures: %+v", ms)
	}
	want := []Measure{
		{Movement: 1, Number: 0, OnsetDiv: 0, LengthDiv: 2, Beats: 4, BeatType: 4},
		{Movement: 1, Number: 1, OnsetDiv: 2, LengthDiv: 8, Beats: 4, BeatType: 4},
		{Movement: 1, Number: 2, OnsetDiv: 10, LengthDiv: 7, Beats: 7, BeatType: 8},
	}
	for i := range want {
		if ms[i] != want[i] {
			t.Errorf("measure %d = %+v, want %+v", i, ms[i], want[i])
		}
	}
	if sc.TimeBeats != 4 || sc.TimeBeatType != 4 {
		t.Errorf("opening meter %d/%d, want 4/4", sc.TimeBeats, sc.TimeBeatType)
	}
	if m, ok := sc.Parts[0].MeasureAt(12); !ok || m.Number != 2 {
		t.Errorf("MeasureAt(12) = %+v %v, want measure 2", m, ok)
	}
}

// A per-note dynamics attribute is a one-note loudness (a ghost note), not a
// new prevailing dynamic.
func TestParse_NoteDynamicsAttributeIsPerNote(t *testing.T) {
	p := parseOne(t, wrap(``,
		`<direction><direction-type><dynamics><f/></dynamics></direction-type></direction>`+
			`<note dynamics="40"><pitch><step>A</step><octave>3</octave></pitch><duration>4</duration><voice>1</voice></note>`+
			note("A", "3", "4", "")+`</measure>`))
	if got := p.Notes[0].Velocity; got < 0.359 || got > 0.361 {
		t.Errorf("ghost note velocity %v, want 0.36 (40%% of MIDI 90)", got)
	}
	if got := p.Notes[1].Velocity; got != 0.76 {
		t.Errorf("following note velocity %v, want the prevailing forte 0.76", got)
	}
}

// A non-numeric bar ("X1") is a fragment of the bar before it: it keeps that
// bar's number and its length merges into one timeline entry, so a bar split
// by a mid-bar repeat is still one bar.
func TestParse_NonNumericBarContinuesThePreviousBar(t *testing.T) {
	doc := `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"><part-name>X</part-name></score-part></part-list>` +
		`<part id="P1">` +
		`<measure number="1"><attributes><divisions>1</divisions><time><beats>4</beats><beat-type>4</beat-type></time></attributes>` +
		note("A", "3", "4", "") + `</measure>` +
		`<measure number="2">` + note("A", "3", "3", "") + `</measure>` +
		`<measure number="X1" implicit="yes">` + note("B", "3", "1", "") + `</measure>` +
		`<measure number="3">` + note("A", "3", "4", "") + `</measure>` +
		`</part></score-partwise>`
	sc, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	p := sc.Parts[0]
	if len(p.Measures) != 3 || p.Measures[1].Number != 2 || p.Measures[1].LengthDiv != 4 || p.Measures[2].Number != 3 {
		t.Fatalf("measures %+v: want bars 1, 2 (3+1 divisions merged), 3", p.Measures)
	}
	if p.Notes[2].Measure != 2 || p.Notes[2].Movement != 1 {
		t.Errorf("fragment note: measure %d movement %d, want measure 2 of movement 1", p.Notes[2].Measure, p.Notes[2].Movement)
	}
}
