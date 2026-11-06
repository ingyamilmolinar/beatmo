package musicxml

import (
	"archive/zip"
	"bytes"
	"testing"
)

// wrap builds a minimal score-partwise document around the supplied measures of
// part P1, so each test states only the notation under test.
func wrap(partAttrs, measures string) string {
	return `<?xml version="1.0"?><score-partwise>` +
		`<work><work-title>Test Work</work-title></work>` +
		`<identification><rights>OpenScore (CC0)</rights>` +
		`<creator type="composer">Test Composer</creator></identification>` +
		`<part-list><score-part id="P1"><part-name>Bb Clarinet</part-name></score-part></part-list>` +
		`<part id="P1"><measure number="1">` +
		`<attributes><divisions>4</divisions>` + partAttrs + `</attributes>` +
		measures + `</part></score-partwise>`
}

func parseOne(t *testing.T, doc string) Part {
	t.Helper()
	sc, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sc.Parts) != 1 {
		t.Fatalf("parts: got %d want 1", len(sc.Parts))
	}
	return sc.Parts[0]
}

func note(pitch, oct, dur string, extra string) string {
	return `<note><pitch><step>` + pitch + `</step><octave>` + oct +
		`</octave></pitch><duration>` + dur + `</duration><voice>1</voice>` + extra + `</note>`
}

// ── .mxl container ──────────────────────────────────────────────────────────

func TestParse_CompressedMXLContainer(t *testing.T) {
	inner := wrap("", note("A", "3", "4", "")+`</measure>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	c, _ := zw.Create("META-INF/container.xml")
	c.Write([]byte(`<container><rootfiles><rootfile full-path="score.xml"/></rootfiles></container>`))
	s, _ := zw.Create("score.xml")
	s.Write([]byte(inner))
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}

	if !IsCompressed(buf.Bytes()) {
		t.Fatal("IsCompressed: got false for a zip container")
	}
	sc, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse(.mxl): %v", err)
	}
	if len(sc.Parts) != 1 || len(sc.Parts[0].Notes) != 1 {
		t.Fatalf("parts/notes: got %d/%v", len(sc.Parts), len(sc.Parts))
	}
	if sc.WorkTitle != "Test Work" {
		t.Errorf("WorkTitle: got %q", sc.WorkTitle)
	}
}

// A container whose META-INF names a missing rootfile still resolves via the
// first non-META-INF document, which is how some exporters lay files out.
func TestParse_CompressedMXLFallbackRootfile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	c, _ := zw.Create("META-INF/container.xml")
	c.Write([]byte(`<container><rootfiles><rootfile full-path="nope.xml"/></rootfiles></container>`))
	s, _ := zw.Create("actual.musicxml")
	s.Write([]byte(wrap("", note("A", "3", "4", "")+`</measure>`)))
	zw.Close()

	sc, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sc.Parts) != 1 {
		t.Fatalf("parts: got %d want 1", len(sc.Parts))
	}
}

// ── provenance ──────────────────────────────────────────────────────────────

func TestParse_RightsAndComposer(t *testing.T) {
	sc, err := Parse([]byte(wrap("", note("A", "3", "4", "")+`</measure>`)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sc.Rights != "OpenScore (CC0)" {
		t.Errorf("Rights: got %q want %q", sc.Rights, "OpenScore (CC0)")
	}
	if sc.Composer != "Test Composer" {
		t.Errorf("Composer: got %q", sc.Composer)
	}
	if sc.Parts[0].Name != "Bb Clarinet" {
		t.Errorf("part name: got %q", sc.Parts[0].Name)
	}
}

// ── measure numbers ─────────────────────────────────────────────────────────

func TestParse_MeasureNumbers(t *testing.T) {
	doc := `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>` +
		`<part id="P1">` +
		`<measure number="0"><attributes><divisions>4</divisions></attributes>` + note("A", "3", "4", "") + `</measure>` +
		`<measure number="1">` + note("B", "3", "4", "") + `</measure>` +
		`<measure number="X2">` + note("C", "4", "4", "") + `</measure>` +
		`</part></score-partwise>`
	p := parseOne(t, doc)
	got := []int{p.Notes[0].Measure, p.Notes[1].Measure, p.Notes[2].Measure}
	// "0" is a real printed number (a pickup); "X2" is not an integer: it is a
	// fragment of the bar before it (a mid-bar repeat or system break) and
	// keeps that bar's number. It used to take its ordinal (3), which
	// collided with a real bar 3 in Haydn Op.76 and Aloha Oe.
	want := []int{0, 1, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d measure: got %d want %d", i, got[i], want[i])
		}
	}
}

// ── transposition ───────────────────────────────────────────────────────────

func TestParse_TransposeYieldsSoundingPitch(t *testing.T) {
	// Bb clarinet: written C4 sounds Bb3. <chromatic>-2</chromatic>.
	p := parseOne(t, wrap(`<transpose><chromatic>-2</chromatic></transpose>`,
		note("C", "4", "4", "")+`</measure>`))
	// Written C4 = semitone 3 (A3=0); sounding Bb3 = 1.
	if got := p.Notes[0].Semitone; got != 1 {
		t.Errorf("sounding semitone: got %d want 1 (Bb3)", got)
	}
	if p.Transpose != -2 {
		t.Errorf("Part.Transpose: got %d want -2", p.Transpose)
	}
}

func TestParse_TransposeWithOctaveChange(t *testing.T) {
	// Tenor voice / guitar: sounds an octave below written.
	p := parseOne(t, wrap(`<transpose><chromatic>0</chromatic><octave-change>-1</octave-change></transpose>`,
		note("A", "3", "4", "")+`</measure>`))
	if got := p.Notes[0].Semitone; got != -12 {
		t.Errorf("sounding semitone: got %d want -12", got)
	}
}

// ── ties ────────────────────────────────────────────────────────────────────

func TestParse_TiedNotesMergeIntoOneEvent(t *testing.T) {
	doc := wrap("",
		note("A", "3", "4", `<tie type="start"/>`)+
			`</measure><measure number="2">`+
			note("A", "3", "4", `<tie type="stop"/>`)+
			note("B", "3", "4", "")+
			`</measure>`)
	p := parseOne(t, doc)
	if len(p.Notes) != 2 {
		t.Fatalf("notes after tie merge: got %d want 2 (%+v)", len(p.Notes), p.Notes)
	}
	if p.Notes[0].DurationDiv != 8 {
		t.Errorf("tied duration: got %d want 8", p.Notes[0].DurationDiv)
	}
	if p.Notes[1].Semitone != 2 { // B3
		t.Errorf("following note survived wrongly: %+v", p.Notes[1])
	}
}

// A chain of three tied notes collapses to one, and the onset of the untied
// note after it is unaffected — the cursor still advanced for every note.
func TestParse_TieChainOfThree(t *testing.T) {
	doc := wrap("",
		note("A", "3", "4", `<tie type="start"/>`)+
			note("A", "3", "4", `<tie type="stop"/><tie type="start"/>`)+
			note("A", "3", "4", `<tie type="stop"/>`)+
			note("C", "4", "4", "")+
			`</measure>`)
	p := parseOne(t, doc)
	if len(p.Notes) != 2 {
		t.Fatalf("notes: got %d want 2", len(p.Notes))
	}
	if p.Notes[0].DurationDiv != 12 {
		t.Errorf("chained duration: got %d want 12", p.Notes[0].DurationDiv)
	}
	if p.Notes[1].OnsetDiv != 12 {
		t.Errorf("onset after chain: got %d want 12", p.Notes[1].OnsetDiv)
	}
}

// <tied> inside <notations> is the other legal spelling and must behave the same.
func TestParse_TiedInNotations(t *testing.T) {
	doc := wrap("",
		note("A", "3", "4", `<notations><tied type="start"/></notations>`)+
			note("A", "3", "4", `<notations><tied type="stop"/></notations>`)+
			`</measure>`)
	p := parseOne(t, doc)
	if len(p.Notes) != 1 || p.Notes[0].DurationDiv != 8 {
		t.Fatalf("notations tie: got %d notes %+v", len(p.Notes), p.Notes)
	}
}

// ── dynamics ────────────────────────────────────────────────────────────────

func TestParse_DynamicsSetVelocity(t *testing.T) {
	doc := wrap("",
		`<direction><direction-type><dynamics><pp/></dynamics></direction-type></direction>`+
			note("A", "3", "4", "")+
			`<direction><direction-type><dynamics><ff/></dynamics></direction-type></direction>`+
			note("B", "3", "4", "")+
			`</measure>`)
	p := parseOne(t, doc)
	if got := p.Notes[0].Velocity; got != dynamicVelocity["pp"] {
		t.Errorf("first note velocity: got %v want %v", got, dynamicVelocity["pp"])
	}
	if got := p.Notes[1].Velocity; got != dynamicVelocity["ff"] {
		t.Errorf("second note velocity: got %v want %v", got, dynamicVelocity["ff"])
	}
}

// A part with no dynamic anywhere reports velocity 0, which consumers read as
// "use the instrument default" rather than "silent".
func TestParse_NoDynamicsLeavesVelocityZero(t *testing.T) {
	p := parseOne(t, wrap("", note("A", "3", "4", "")+`</measure>`))
	if p.Notes[0].Velocity != 0 {
		t.Errorf("velocity with no marking: got %v want 0", p.Notes[0].Velocity)
	}
}

func TestParse_AccentBoostsVelocity(t *testing.T) {
	doc := wrap("",
		`<direction><direction-type><dynamics><mf/></dynamics></direction-type></direction>`+
			note("A", "3", "4", `<notations><articulations><accent/></articulations></notations>`)+
			note("B", "3", "4", `<notations><articulations><strong-accent/></articulations></notations>`)+
			note("C", "4", "4", "")+
			`</measure>`)
	p := parseOne(t, doc)
	base := dynamicVelocity["mf"]
	if got, want := p.Notes[0].Velocity, base*accentBoost; got != want {
		t.Errorf("accent velocity: got %v want %v", got, want)
	}
	if got, want := p.Notes[1].Velocity, base*marcatoBoost; got != want {
		t.Errorf("marcato velocity: got %v want %v", got, want)
	}
	if got := p.Notes[2].Velocity; got != base {
		t.Errorf("plain note velocity: got %v want %v", got, base)
	}
	if !p.Notes[0].Articulations.Accent {
		t.Error("accent articulation not recorded")
	}
}

// Velocity never exceeds 1: ffff with a marcato would otherwise overflow.
func TestParse_VelocityClampedToOne(t *testing.T) {
	doc := wrap("",
		`<direction><direction-type><dynamics><ffff/></dynamics></direction-type></direction>`+
			note("A", "3", "4", `<notations><articulations><strong-accent/></articulations></notations>`)+
			`</measure>`)
	p := parseOne(t, doc)
	if got := p.Notes[0].Velocity; got != 1 {
		t.Errorf("clamped velocity: got %v want 1", got)
	}
}

func TestParse_SoundDynamicsAttribute(t *testing.T) {
	doc := wrap("",
		`<direction><sound dynamics="100"/></direction>`+note("A", "3", "4", "")+`</measure>`)
	p := parseOne(t, doc)
	if got := p.Notes[0].Velocity; got <= 0 || got > 1 {
		t.Errorf("sound dynamics velocity out of range: %v", got)
	}
}

// ── agogic directions ───────────────────────────────────────────────────────

func TestParse_AgogicDirectionsRecorded(t *testing.T) {
	doc := wrap("",
		`<direction><direction-type><words>rit.</words></direction-type></direction>`+
			note("A", "3", "4", "")+
			`<direction><direction-type><words>a tempo</words></direction-type></direction>`+
			note("B", "3", "4", "")+
			`<direction><direction-type><words>accel.</words></direction-type></direction>`+
			note("C", "4", "4", "")+
			`<direction><direction-type><words>dolce</words></direction-type></direction>`+
			`</measure>`)
	p := parseOne(t, doc)
	if len(p.Directions) != 3 {
		t.Fatalf("directions: got %d want 3 (%+v)", len(p.Directions), p.Directions)
	}
	want := []DirectionKind{DirRitardando, DirATempo, DirAccelerando}
	for i, w := range want {
		if p.Directions[i].Kind != w {
			t.Errorf("direction %d: got %v want %v", i, p.Directions[i].Kind, w)
		}
	}
	if p.Directions[1].OnsetDiv != 4 {
		t.Errorf("a tempo onset: got %d want 4", p.Directions[1].OnsetDiv)
	}
}

func TestDirectionKind_Spellings(t *testing.T) {
	cases := map[string]DirectionKind{
		"rit.": DirRitardando, "ritard.": DirRitardando, "ritardando": DirRitardando,
		"rall.": DirRitardando, "allargando": DirRitardando,
		"accel.": DirAccelerando, "accelerando": DirAccelerando, "stringendo": DirAccelerando,
		"a tempo": DirATempo, "Tempo I": DirATempo,
		"dolce": DirNone, "": DirNone, "cresc.": DirNone,
	}
	for in, want := range cases {
		if got := directionKind(in); got != want {
			t.Errorf("directionKind(%q): got %v want %v", in, got, want)
		}
	}
}

func TestParse_FermataRecorded(t *testing.T) {
	p := parseOne(t, wrap("",
		note("A", "3", "4", `<notations><fermata/></notations>`)+`</measure>`))
	if !p.Notes[0].Articulations.Fermata {
		t.Error("fermata not recorded")
	}
}

// ── grace notes ─────────────────────────────────────────────────────────────

// A grace note is flagged and does NOT advance the division cursor, so the note
// it decorates keeps its true onset.
func TestParse_GraceNoteDoesNotAdvanceCursor(t *testing.T) {
	doc := wrap("",
		note("A", "3", "4", "")+
			`<note><grace/><pitch><step>B</step><octave>3</octave></pitch><voice>1</voice></note>`+
			note("C", "4", "4", "")+
			`</measure>`)
	p := parseOne(t, doc)
	if len(p.Notes) != 3 {
		t.Fatalf("notes: got %d want 3", len(p.Notes))
	}
	if !p.Notes[1].Grace {
		t.Error("grace note not flagged")
	}
	if p.Notes[1].OnsetDiv != 4 {
		t.Errorf("grace onset: got %d want 4", p.Notes[1].OnsetDiv)
	}
	if p.Notes[2].OnsetDiv != 4 {
		t.Errorf("decorated note onset: got %d want 4 (grace must not advance the cursor)", p.Notes[2].OnsetDiv)
	}
}

// ── staff ───────────────────────────────────────────────────────────────────

func TestParse_StaffRecorded(t *testing.T) {
	p := parseOne(t, wrap("",
		note("A", "3", "4", `<staff>2</staff>`)+`</measure>`))
	if p.Notes[0].Staff != 2 {
		t.Errorf("staff: got %d want 2", p.Notes[0].Staff)
	}
}
