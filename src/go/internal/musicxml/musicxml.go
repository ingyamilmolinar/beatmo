// Package musicxml parses score-partwise documents with a streaming decoder (element order matters for chord / backup /
// forward), carrying the expression a circuit needs to sound like the score: dynamics, articulations, agogic words,
// merged ties, <transpose> (sounding pitch). A .mxl container is unwrapped by its ZIP magic.
package musicxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Articulations are the notated per-note marks that affect how a note is
// attacked or held. They drive velocity and groove, never pitch.
type Articulations struct {
	Accent       bool
	StrongAccent bool // marcato
	Staccato     bool
	Tenuto       bool
	Fermata      bool

	// Breaks AFTER this note: they delay the next onset, not the note carrying them.
	BreathMark bool
	Caesura    bool

	// +1 rolls upward (lowest first, what a bare <arpeggiate/> means), -1 downward, 0 not rolled.
	Arpeggiate int
}

// Note is a single musical event within a Part.
type Note struct {
	Semitone    int // SOUNDING pitch offset from A3 (A3 = MIDI 57 → Semitone 0)
	OnsetDiv    int // onset in divisions from part start
	DurationDiv int // duration in divisions
	Voice       int // voice number (defaults to 1)
	Staff       int // staff number (defaults to 1)
	Measure     int // 1-based measure number as printed; 0 when unparseable
	// 1-based, incremented wherever the printed number RESTARTS: a multi-movement work is one file, so a measure number
	// alone does not identify a bar, and a range without a Movement takes bars from every movement.
	Movement int
	Rest     bool // true when the note element contains a <rest/>
	Chord    bool // true when the note element contains a <chord/>
	Grace    bool // true when the note element contains a <grace/>

	// Velocity is the dynamic in force at this note, 0..1, already including
	// any accent. 0 means "no dynamic was ever marked in this part".
	Velocity float64

	Articulations Articulations

	// Its Semitone is the STAFF POSITION, not a pitch: identity comes from Instrument / PercKey.
	Unpitched bool
	// Instrument is the <instrument id> naming which kit piece this note is,
	// resolvable through Part.Instruments. Empty when the note names none.
	Instrument string
	// The General MIDI key (35 kick, 38 snare …) from its instrument's <midi-unpitched>, which MusicXML numbers key + 1.
	// 0 when the score does not say.
	PercKey int

	// tieStart/tieStop are consumed by mergeTies and cleared before return.
	tieStart bool
	tieStop  bool
}

// DirectionKind classifies the agogic directions that shift notes off the grid.
type DirectionKind int

const (
	DirNone DirectionKind = iota
	DirRitardando
	DirAccelerando
	DirATempo
	// DirSwing starts swung playback (SwingRatio/SwingUnit); DirStraight ends
	// it. Both come from <sound><swing> or from "Swing"/"Straight" words.
	DirSwing
	DirStraight
)

// Direction is a notated timing instruction, positioned like a note.
type Direction struct {
	Kind     DirectionKind
	Measure  int
	OnsetDiv int

	// SwingRatio is first:second of a swung pair (2 = the classic triplet
	// swing, 3 = dotted). SwingUnit is the swung note value in quarter notes
	// (0.5 = eighths, 0.25 = sixteenths). Set only on DirSwing.
	SwingRatio float64
	SwingUnit  float64
}

// PartInstrument is one <score-instrument> of a part: a kit piece in a drum
// part, or the sole instrument of an ordinary one.
type PartInstrument struct {
	ID      string
	Name    string
	PercKey int // GM percussion key; 0 when <midi-unpitched> is absent
}

// Pickup and repeat-ending bars are SHORTER than their meter, so LengthDiv is measured, never derived.
type Measure struct {
	Movement  int
	Number    int
	OnsetDiv  int
	LengthDiv int
	Beats     int
	BeatType  int
}

// Part corresponds to a <part> element.
type Part struct {
	ID   string
	Name string // from <part-list><score-part><part-name>

	Notes      []Note
	Directions []Direction

	// Meters change mid-score, so anything bar-relative (agogic spans, swing pairs) reads this, not the opening meter.
	Measures []Measure

	// Instruments maps an <instrument id> to the kit piece it names.
	Instruments map[string]PartInstrument

	Divisions int // divisions-per-quarter in force for this part
	Transpose int // semitones added to written pitch to reach sounding pitch
}

// MeasureAt returns the measure containing onsetDiv: the last measure that
// starts at or before it. ok is false before the first measure.
func (p Part) MeasureAt(onsetDiv int) (Measure, bool) {
	i := sort.Search(len(p.Measures), func(i int) bool { return p.Measures[i].OnsetDiv > onsetDiv })
	if i == 0 {
		return Measure{}, false
	}
	return p.Measures[i-1], true
}

// SwingAt returns the swing in force at onsetDiv — the latest swing or
// straight instruction at or before it. ok is false when the onset is in
// straight time.
func (p Part) SwingAt(onsetDiv int) (Direction, bool) {
	var cur Direction
	have := false
	for _, d := range p.Directions {
		if d.OnsetDiv > onsetDiv {
			continue
		}
		switch d.Kind {
		case DirSwing:
			if !have || d.OnsetDiv >= cur.OnsetDiv {
				cur, have = d, true
			}
		case DirStraight:
			if !have || d.OnsetDiv >= cur.OnsetDiv {
				cur, have = d, true
			}
		}
	}
	if !have || cur.Kind != DirSwing {
		return Direction{}, false
	}
	return cur, true
}

// Normalised to quarters per minute (half = 100 is 200), so every mark compares directly.
type TempoMark struct {
	Measure int
	BPM     float64
}

// The FIRST marking also seeds TempoBPM (the opening tempo); later ones extend the timeline. A duplicate for the same
// measure (marks repeat across parts) keeps the first.
func (s *Score) recordTempo(measure int, quarterBPM float64) {
	if quarterBPM <= 0 {
		return
	}
	if len(s.Tempos) == 0 {
		s.TempoBPM = quarterBPM
	}
	for _, t := range s.Tempos {
		if t.Measure == measure {
			return // already have this measure (another part, or <sound> after <metronome>)
		}
	}
	s.Tempos = append(s.Tempos, TempoMark{Measure: measure, BPM: quarterBPM})
	sort.SliceStable(s.Tempos, func(i, j int) bool { return s.Tempos[i].Measure < s.Tempos[j].Measure })
}

// TempoAt returns the tempo in force at the given measure: the latest marking
// at or before it, or the opening tempo when the measure precedes them all.
func (s *Score) TempoAt(measure int) float64 {
	bpm := s.TempoBPM
	for _, t := range s.Tempos {
		if t.Measure > measure {
			break
		}
		bpm = t.BPM
	}
	return bpm
}

// A circuit runs at one BPM: the tempo covering most of the range, NOT the one at from, since a pickup at another tempo
// would set it from the bar the excerpt least resembles. Ties go to the earlier tempo.
func (s *Score) DominantTempo(from, to int) float64 {
	if to < from {
		to = from
	}
	var order []float64
	bars := map[float64]int{}
	cur := s.TempoAt(from)
	for m := from; m <= to; m++ {
		for _, t := range s.Tempos {
			if t.Measure == m {
				cur = t.BPM
			}
		}
		if _, seen := bars[cur]; !seen {
			order = append(order, cur)
		}
		bars[cur]++
	}
	best := cur
	most := -1
	for _, bpm := range order {
		if bars[bpm] > most {
			best, most = bpm, bars[bpm]
		}
	}
	return best
}

// Score is the top-level result of parsing a score-partwise MusicXML document.
type Score struct {
	Divisions int
	KeyFifths int
	Mode      string
	// The OPENING tempo, not the last parsed: a multi-tempo work has a timeline (Tempos), and one number is valid only for
	// a single-tempo piece.
	TempoBPM float64
	// Tempos is every tempo marking in the score, in measure order, converted
	// to quarter notes per minute. Use TempoAt/DominantTempo rather than
	// reading it directly.
	Tempos       []TempoMark
	TimeBeats    int
	TimeBeatType int
	Parts        []Part

	// Rights is the <rights> element of <identification>, verbatim. The score
	// catalogue's licence claim is checked against it.
	Rights string
	// WorkTitle / MovementTitle / Composer are provenance, not musical data.
	WorkTitle     string
	MovementTitle string
	Composer      string
	// Lyricist is the <creator type="lyricist"> (or "poet") of a song: an
	// author whose term the score catalogue's public-domain gate must clear.
	Lyricist string
}

// The conventional MIDI curve (pp ≈ 0.28, mf ≈ 0.62, ff ≈ 0.90) in 0..1, so it multiplies an instrument volume
// directly.
var dynamicVelocity = map[string]float64{
	"pppp": 0.10, "ppp": 0.16, "pp": 0.28, "p": 0.40,
	"mp": 0.52, "mf": 0.62, "f": 0.76, "ff": 0.90,
	"fff": 0.97, "ffff": 1.00,
	"sf": 0.90, "sfz": 0.92, "sffz": 0.95, "fz": 0.88,
	"rf": 0.82, "rfz": 0.84, "fp": 0.80,
}

// Modest: an accent is stress within the dynamic, not a new one.
const (
	accentBoost  = 1.15
	marcatoBoost = 1.25
)

// stepSemitone maps a MusicXML step letter to its semitone value within an octave
// relative to C (C=0, D=2, …).
var stepSemitone = map[string]int{
	"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11,
}

// pitchToSemitone converts a MusicXML pitch (step, alter, octave) to the Semitone
// offset from A3 (MIDI 57).
func pitchToSemitone(step string, alter, octave int) int {
	midi := (octave+1)*12 + stepSemitone[step] + alter
	return midi - 57
}

// IsCompressed reports whether data is a compressed .mxl (ZIP) container
// rather than a plain MusicXML document.
func IsCompressed(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}

// Decompress unwraps a .mxl container and returns the root score document.
// The rootfile is read from META-INF/container.xml; if that is missing or names
// a path not present, the first non-META-INF .xml/.musicxml entry is used.
func Decompress(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("musicxml: open mxl container: %w", err)
	}

	byName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		byName[f.Name] = f
	}

	if c, ok := byName["META-INF/container.xml"]; ok {
		raw, err := readZipEntry(c)
		if err != nil {
			return nil, err
		}
		if p := rootfilePath(raw); p != "" {
			if f, ok := byName[p]; ok {
				return readZipEntry(f)
			}
		}
	}

	for _, f := range zr.File {
		n := f.Name
		if strings.HasPrefix(n, "META-INF/") {
			continue
		}
		if strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".musicxml") {
			return readZipEntry(f)
		}
	}
	return nil, fmt.Errorf("musicxml: mxl container has no score document")
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("musicxml: open %s: %w", f.Name, err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return nil, fmt.Errorf("musicxml: read %s: %w", f.Name, err)
	}
	return buf.Bytes(), nil
}

// rootfilePath extracts the first <rootfile full-path="..."> from a
// META-INF/container.xml document.
func rootfilePath(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "rootfile" {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "full-path" {
				return a.Value
			}
		}
	}
}

// Parse parses a MusicXML document — plain or compressed .mxl — and returns a Score.
func Parse(data []byte) (Score, error) {
	if IsCompressed(data) {
		raw, err := Decompress(data)
		if err != nil {
			return Score{}, err
		}
		data = raw
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = passthroughCharset

	var sc Score
	sc.TempoBPM = 120.0 // default
	sc.Mode = "major"   // default

	partNames := map[string]string{}
	partInstruments := map[string]map[string]PartInstrument{}

	// We collect parts in order. The outer loop walks top-level elements.
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "identification":
			if err := parseIdentification(dec, &sc); err != nil {
				return Score{}, err
			}
		case "work":
			if err := parseWork(dec, &sc); err != nil {
				return Score{}, err
			}
		case "movement-title":
			text, err := readText(dec, se)
			if err != nil {
				return Score{}, err
			}
			sc.MovementTitle = strings.TrimSpace(text)
		case "part-list":
			if err := parsePartList(dec, partNames, partInstruments); err != nil {
				return Score{}, err
			}
		case "part":
			part, err := parsePart(dec, se, &sc)
			if err != nil {
				return Score{}, fmt.Errorf("musicxml: part %q: %w", part.ID, err)
			}
			part.Name = partNames[part.ID]
			part.Instruments = partInstruments[part.ID]
			resolvePercussion(&part)
			part.Notes = mergeTies(part.Notes)
			sc.Parts = append(sc.Parts, part)
		}
	}

	return sc, nil
}

// passthroughCharset lets the decoder accept documents declaring encodings the
// stdlib does not know (MuseScore and Finale both emit UTF-8 either way).
func passthroughCharset(_ string, input io.Reader) (io.Reader, error) { return input, nil }

// parseWork reads <work> for the work title.
func parseWork(dec *xml.Decoder, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "work-title" {
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				sc.WorkTitle = strings.TrimSpace(text)
				continue
			}
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name.Local == "work" {
				return nil
			}
		}
	}
}

// parseIdentification reads <identification> for rights and composer.
func parseIdentification(dec *xml.Decoder, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rights":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				if sc.Rights == "" {
					sc.Rights = strings.TrimSpace(text)
				}
			case "creator":
				kind := attr(t, "type")
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				switch {
				case kind == "composer" && sc.Composer == "":
					sc.Composer = strings.TrimSpace(text)
				case (kind == "lyricist" || kind == "poet") && sc.Lyricist == "":
					sc.Lyricist = strings.TrimSpace(text)
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "identification" {
				return nil
			}
		}
	}
}

// parsePartList records the display name of every <score-part>, and the kit
// pieces it declares: each <score-instrument> (named by <instrument-name>) and
// the GM percussion key its <midi-instrument> assigns through <midi-unpitched>.
func parsePartList(dec *xml.Decoder, names map[string]string, insts map[string]map[string]PartInstrument) error {
	curID := ""
	put := func(id string, f func(*PartInstrument)) {
		if curID == "" || id == "" {
			return
		}
		m := insts[curID]
		if m == nil {
			m = map[string]PartInstrument{}
			insts[curID] = m
		}
		pi := m[id]
		pi.ID = id
		f(&pi)
		m[id] = pi
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "score-part":
				curID = attr(t, "id")
			case "part-name":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				if curID != "" && names[curID] == "" {
					// A two-line label ("Singstimme\nVoice") collapses to one line, usable as an identifier.
					names[curID] = strings.Join(strings.Fields(text), " ")
				}
			case "score-instrument":
				id := attr(t, "id")
				name, err := readChildText(dec, t, "instrument-name")
				if err != nil {
					return err
				}
				put(id, func(pi *PartInstrument) { pi.Name = strings.Join(strings.Fields(name), " ") })
			case "midi-instrument":
				id := attr(t, "id")
				text, err := readChildText(dec, t, "midi-unpitched")
				if err != nil {
					return err
				}
				// MusicXML numbers midi-unpitched 1-128: the GM key is one less.
				if v, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && v > 0 {
					put(id, func(pi *PartInstrument) { pi.PercKey = v - 1 })
				}
			case "score-instrument-group", "part-group":
				if err := dec.Skip(); err != nil {
					return err
				}
			default:
				// score-part itself must be ENTERED so its instruments are seen.
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "part-list" {
				return nil
			}
		}
	}
}

// attr returns the value of the named attribute, or "".
func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// readChildText consumes the element started by start and returns the text of
// its first child named child ("" when there is none).
func readChildText(dec *xml.Decoder, start xml.StartElement, child string) (string, error) {
	out := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == child && out == "" {
				text, err := readText(dec, t)
				if err != nil {
					return "", err
				}
				out = text
				continue
			}
			if err := dec.Skip(); err != nil {
				return "", err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return out, nil
			}
		}
	}
}

// A drum part with exactly ONE instrument may omit <instrument> on its notes.
func resolvePercussion(part *Part) {
	sole := ""
	if len(part.Instruments) == 1 {
		for id := range part.Instruments {
			sole = id
		}
	}
	for i := range part.Notes {
		n := &part.Notes[i]
		if !n.Unpitched {
			continue
		}
		if n.Instrument == "" {
			n.Instrument = sole
		}
		if pi, ok := part.Instruments[n.Instrument]; ok {
			n.PercKey = pi.PercKey
		}
	}
}

// partState is the mutable cursor a single <part> is parsed with.
type partState struct {
	pos               int     // division cursor within the part
	lastNonChordOnset int     // onset of the most recent non-chord note
	measure           int     // current printed measure number
	measureOrdinal    int     // 1-based fallback when the printed number is not an int
	movement          int     // 1-based; bumped when the printed number restarts
	prevMeasure       int     // previous printed number, to spot the restart
	velocity          float64 // dynamic currently in force
	transpose         int     // written → sounding semitones
	timeBeats         int     // meter in force (0 until a <time> is seen)
	timeBeatType      int
	maxPos            int // furthest division reached in the current measure
}

// parsePart reads a <part> element from dec, advancing past the closing </part>.
// It updates sc in-place with global attributes found in the first <attributes>.
func parsePart(dec *xml.Decoder, start xml.StartElement, sc *Score) (Part, error) {
	var part Part
	for _, a := range start.Attr {
		if a.Name.Local == "id" {
			part.ID = a.Value
		}
	}

	st := &partState{}

	for {
		tok, err := dec.Token()
		if err != nil {
			return part, fmt.Errorf("reading part: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "measure":
				st.measureOrdinal++
				num, numeric := measureNumber(t, st.measureOrdinal)
				// A NON-numeric bar ("X1") is a fragment of the bar before it (a mid-bar repeat, a system break); its ordinal would
				// collide with a real bar's number.
				continuation := !numeric && st.measureOrdinal > 1
				if continuation {
					num = st.measure
				}
				st.measure = num
				// A new movement is a RESTART (the number returns to 0 or 1), not any decrease: out-of-band fragments continue the
				// previous bar and must not read as one.
				if st.movement == 0 {
					st.movement = 1
				} else if st.measure <= 1 && st.prevMeasure > 1 {
					st.movement++
				}
				st.prevMeasure = st.measure
				start := st.pos
				st.maxPos = st.pos
				if err := parseMeasure(dec, t, sc, &part, st); err != nil {
					return part, err
				}
				if st.pos > st.maxPos {
					st.maxPos = st.pos
				}
				if last := len(part.Measures) - 1; continuation && last >= 0 &&
					part.Measures[last].Number == st.measure && part.Measures[last].Movement == st.movement {
					part.Measures[last].LengthDiv += st.maxPos - start
				} else {
					part.Measures = append(part.Measures, Measure{
						Movement: st.movement, Number: st.measure,
						OnsetDiv: start, LengthDiv: st.maxPos - start,
						Beats: st.timeBeats, BeatType: st.timeBeatType,
					})
				}
			}
		case xml.EndElement:
			if t.Name.Local == "part" {
				part.Transpose = st.transpose
				if part.Divisions == 0 {
					part.Divisions = sc.Divisions
				}
				return part, nil
			}
		}
	}
}

// numeric is false when absent or not an integer (pickups are often "0", fragments "X1"): the ordinal is the fallback.
func measureNumber(start xml.StartElement, ordinal int) (int, bool) {
	for _, a := range start.Attr {
		if a.Name.Local != "number" {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(a.Value)); err == nil {
			return v, true
		}
	}
	return ordinal, false
}

// parseMeasure reads a <measure> element, dispatching child elements in order.
func parseMeasure(dec *xml.Decoder, start xml.StartElement, sc *Score, part *Part, st *partState) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("reading measure: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "attributes":
				if err := parseAttributes(dec, t, sc, part, st); err != nil {
					return err
				}
			case "note":
				n, err := parseNote(dec, t, st)
				if err != nil {
					return err
				}
				part.Notes = append(part.Notes, n)
				if st.pos > st.maxPos {
					st.maxPos = st.pos
				}
			case "backup":
				d, err := parseDuration(dec, t)
				if err != nil {
					return err
				}
				st.pos -= d
			case "forward":
				d, err := parseDuration(dec, t)
				if err != nil {
					return err
				}
				st.pos += d
				if st.pos > st.maxPos {
					st.maxPos = st.pos
				}
			case "sound":
				if err := parseSound(dec, t, sc, part, st); err != nil {
					return err
				}
			case "direction":
				if err := parseDirection(dec, "direction", sc, part, st); err != nil {
					return err
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "measure" {
				return nil
			}
		}
	}
}

// parseAttributes reads an <attributes> element and updates sc/part/state.
func parseAttributes(dec *xml.Decoder, start xml.StartElement, sc *Score, part *Part, st *partState) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "divisions":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return fmt.Errorf("divisions: %w", err)
				}
				part.Divisions = v
				if sc.Divisions == 0 {
					sc.Divisions = v
				}
			case "key":
				if err := parseKey(dec, t, sc); err != nil {
					return err
				}
			case "time":
				if err := parseTime(dec, t, sc, st); err != nil {
					return err
				}
			case "transpose":
				v, err := parseTranspose(dec)
				if err != nil {
					return err
				}
				st.transpose = v
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "attributes" {
				return nil
			}
		}
	}
}

// parseTranspose reads <transpose> and returns the written→sounding shift in
// semitones (chromatic + 12 × octave-change).
func parseTranspose(dec *xml.Decoder) (int, error) {
	chromatic, octaveChange := 0, 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "chromatic", "octave-change":
				text, err := readText(dec, t)
				if err != nil {
					return 0, err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return 0, fmt.Errorf("transpose/%s: %w", t.Name.Local, err)
				}
				if t.Name.Local == "chromatic" {
					chromatic = v
				} else {
					octaveChange = v
				}
			default:
				if err := dec.Skip(); err != nil {
					return 0, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "transpose" {
				return chromatic + 12*octaveChange, nil
			}
		}
	}
}

// parseKey reads a <key> element and updates sc.
func parseKey(dec *xml.Decoder, start xml.StartElement, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "fifths":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return fmt.Errorf("fifths: %w", err)
				}
				sc.KeyFifths = v
			case "mode":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				sc.Mode = strings.TrimSpace(text)
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "key" {
				return nil
			}
		}
	}
}

// The score keeps the OPENING meter; the part state tracks the meter IN FORCE, which each Measure records.
func parseTime(dec *xml.Decoder, start xml.StartElement, sc *Score, st *partState) error {
	beats, beatType := 0, 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "beats", "beat-type":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				// Additive meters ("3+2+2") sum to the bar length.
				v := 0
				for _, f := range strings.Split(strings.TrimSpace(text), "+") {
					x, err := strconv.Atoi(strings.TrimSpace(f))
					if err != nil {
						return fmt.Errorf("%s: %w", t.Name.Local, err)
					}
					v += x
				}
				if t.Name.Local == "beats" {
					if beats == 0 {
						beats = v
					}
				} else if beatType == 0 {
					beatType = v
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "time" {
				if beats > 0 && beatType > 0 {
					if sc.TimeBeats == 0 {
						sc.TimeBeats, sc.TimeBeatType = beats, beatType
					}
					st.timeBeats, st.timeBeatType = beats, beatType
				}
				return nil
			}
		}
	}
}

// parseNote reads a <note> element and returns a Note.
// st is updated as per chord/non-chord rules.
func parseNote(dec *xml.Decoder, start xml.StartElement, st *partState) (Note, error) {
	var n Note
	n.Voice = 1
	n.Staff = 1
	n.Measure = st.measure
	n.Movement = st.movement

	var step string
	var alter, octave int
	hasChord, hasRest, hasPitch, hasUnpitched := false, false, false, false

	for {
		tok, err := dec.Token()
		if err != nil {
			return Note{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "chord":
				hasChord = true
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			case "grace":
				n.Grace = true
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			case "rest":
				hasRest = true
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			case "pitch":
				hasPitch = true
				var err error
				step, alter, octave, err = parsePitch(dec)
				if err != nil {
					return Note{}, err
				}
			case "unpitched":
				// <unpitched> carries a staff POSITION (display-step/octave),
				// not a pitch.
				hasUnpitched = true
				var err error
				step, alter, octave, err = parseDisplayPosition(dec)
				if err != nil {
					return Note{}, err
				}
			case "instrument":
				n.Instrument = attr(t, "id")
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			case "duration":
				text, err := readText(dec, t)
				if err != nil {
					return Note{}, err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return Note{}, fmt.Errorf("duration: %w", err)
				}
				n.DurationDiv = v
			case "voice":
				text, err := readText(dec, t)
				if err != nil {
					return Note{}, err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return Note{}, fmt.Errorf("voice: %w", err)
				}
				n.Voice = v
			case "staff":
				text, err := readText(dec, t)
				if err != nil {
					return Note{}, err
				}
				if v, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
					n.Staff = v
				}
			case "tie":
				for _, a := range t.Attr {
					if a.Name.Local != "type" {
						continue
					}
					switch a.Value {
					case "start":
						n.tieStart = true
					case "stop":
						n.tieStop = true
					}
				}
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			case "notations":
				if err := parseNotations(dec, &n); err != nil {
					return Note{}, err
				}
			default:
				if err := dec.Skip(); err != nil {
					return Note{}, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "note" {
				goto done
			}
		}
	}

done:
	n.Chord = hasChord
	n.Rest = hasRest

	switch {
	case n.Grace:
		// Grace notes take no time: they hang off the next note's onset.
		n.OnsetDiv = st.pos
	case hasChord:
		n.OnsetDiv = st.lastNonChordOnset
	default:
		n.OnsetDiv = st.pos
		st.lastNonChordOnset = st.pos
		st.pos += n.DurationDiv
	}

	n.Unpitched = hasUnpitched && !hasRest
	switch {
	case hasRest:
		n.Semitone = 0
		n.Instrument = ""
	case hasPitch:
		n.Semitone = pitchToSemitone(step, alter, octave) + st.transpose
	case hasUnpitched:
		// Staff position, untransposed: it identifies a line, not a pitch.
		n.Semitone = pitchToSemitone(step, alter, octave)
	}

	n.Velocity = accentedVelocity(st.velocity, n.Articulations)
	// THIS note's loudness as a percentage of MIDI 90: how notation software writes a ghost note or a single accent without
	// changing the dynamic.
	if v, err := strconv.ParseFloat(attr(start, "dynamics"), 64); err == nil && v > 0 {
		n.Velocity = clamp01(v / 100 * 0.9)
	}
	return n, nil
}

// Clamped to 1. A part with no dynamic anywhere keeps 0, read as "the instrument default".
func accentedVelocity(base float64, a Articulations) float64 {
	if base <= 0 {
		return 0
	}
	v := base
	switch {
	case a.StrongAccent:
		v *= marcatoBoost
	case a.Accent:
		v *= accentBoost
	}
	if v > 1 {
		v = 1
	}
	return v
}

// parseNotations reads <notations>, recording articulations and tie stops that
// are expressed as <tied> rather than <tie>.
func parseNotations(dec *xml.Decoder, n *Note) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "articulations", "ornaments":
				if err := parseArticulations(dec, t.Name.Local, n); err != nil {
					return err
				}
			case "fermata":
				n.Articulations.Fermata = true
				if err := dec.Skip(); err != nil {
					return err
				}
			case "arpeggiate":
				n.Articulations.Arpeggiate = 1
				if attr(t, "direction") == "down" {
					n.Articulations.Arpeggiate = -1
				}
				if err := dec.Skip(); err != nil {
					return err
				}
			case "tied":
				for _, a := range t.Attr {
					if a.Name.Local != "type" {
						continue
					}
					switch a.Value {
					case "start":
						n.tieStart = true
					case "stop":
						n.tieStop = true
					}
				}
				if err := dec.Skip(); err != nil {
					return err
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "notations" {
				return nil
			}
		}
	}
}

// parseArticulations reads the children of <articulations> (or <ornaments>).
func parseArticulations(dec *xml.Decoder, closing string, n *Note) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "accent":
				n.Articulations.Accent = true
			case "strong-accent":
				n.Articulations.StrongAccent = true
			case "staccato", "staccatissimo":
				n.Articulations.Staccato = true
			case "tenuto":
				n.Articulations.Tenuto = true
			case "breath-mark":
				n.Articulations.BreathMark = true
			case "caesura":
				n.Articulations.Caesura = true
			}
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name.Local == closing {
				return nil
			}
		}
	}
}

// parsePitch reads a <pitch> element and returns step, alter, octave.
func parsePitch(dec *xml.Decoder) (step string, alter, octave int, err error) {
	for {
		tok, terr := dec.Token()
		if terr != nil {
			return "", 0, 0, terr
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "step":
				text, terr := readText(dec, t)
				if terr != nil {
					return "", 0, 0, terr
				}
				step = strings.TrimSpace(text)
			case "alter":
				text, terr := readText(dec, t)
				if terr != nil {
					return "", 0, 0, terr
				}
				v, terr := strconv.ParseFloat(strings.TrimSpace(text), 64)
				if terr != nil {
					return "", 0, 0, fmt.Errorf("alter: %w", terr)
				}
				alter = int(v)
			case "octave":
				text, terr := readText(dec, t)
				if terr != nil {
					return "", 0, 0, terr
				}
				v, terr := strconv.Atoi(strings.TrimSpace(text))
				if terr != nil {
					return "", 0, 0, fmt.Errorf("octave: %w", terr)
				}
				octave = v
			default:
				if terr := dec.Skip(); terr != nil {
					return "", 0, 0, terr
				}
			}
		case xml.EndElement:
			if t.Name.Local == "pitch" {
				return step, alter, octave, nil
			}
		}
	}
}

// parseDisplayPosition reads an <unpitched> element's display-step and
// display-octave — a percussion note's staff position.
func parseDisplayPosition(dec *xml.Decoder) (step string, alter, octave int, err error) {
	for {
		tok, terr := dec.Token()
		if terr != nil {
			return "", 0, 0, terr
		}
		switch t := tok.(type) {
		case xml.StartElement:
			text, terr := readText(dec, t)
			if terr != nil {
				return "", 0, 0, terr
			}
			switch t.Name.Local {
			case "display-step":
				step = strings.TrimSpace(text)
			case "display-octave":
				if v, e := strconv.Atoi(strings.TrimSpace(text)); e == nil {
					octave = v
				}
			}
		case xml.EndElement:
			if t.Name.Local == "unpitched" {
				return step, 0, octave, nil
			}
		}
	}
}

// parseDuration reads a <backup> or <forward> element and returns the duration value.
func parseDuration(dec *xml.Decoder, start xml.StartElement) (int, error) {
	var dur int
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "duration" {
				text, err := readText(dec, t)
				if err != nil {
					return 0, err
				}
				v, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil {
					return 0, fmt.Errorf("backup/forward duration: %w", err)
				}
				dur = v
				continue
			}
			if err := dec.Skip(); err != nil {
				return 0, err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return dur, nil
			}
		}
	}
}

// closing names the element to consume, so one switch serves <direction> and its <direction-type> children (the
// payload lives there, except <sound>).
func parseDirection(dec *xml.Decoder, closing string, sc *Score, part *Part, st *partState) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "direction-type":
				if err := parseDirection(dec, "direction-type", sc, part, st); err != nil {
					return err
				}
			case "metronome":
				if err := parseMetronome(dec, t, sc, st.measure); err != nil {
					return err
				}
			case "dynamics":
				v, err := parseDynamics(dec)
				if err != nil {
					return err
				}
				if v > 0 {
					st.velocity = v
				}
			case "words":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				if k := directionKind(text); k != DirNone {
					part.Directions = append(part.Directions, Direction{
						Kind: k, Measure: st.measure, OnsetDiv: st.pos,
					})
				} else if d, ok := swingWords(text); ok {
					d.Measure, d.OnsetDiv = st.measure, st.pos
					part.Directions = append(part.Directions, d)
				}
			case "sound":
				if err := parseSound(dec, t, sc, part, st); err != nil {
					return err
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == closing {
				return nil
			}
		}
	}
}

// Tempo in quarters per minute, dynamics as a percentage of MIDI 90, and its <swing>.
func parseSound(dec *xml.Decoder, start xml.StartElement, sc *Score, part *Part, st *partState) error {
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "tempo":
			if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
				// <sound tempo> is defined as quarter notes per minute, so it
				// needs no beat-unit conversion.
				sc.recordTempo(st.measure, v)
			}
		case "dynamics":
			if v, err := strconv.ParseFloat(a.Value, 64); err == nil && v > 0 {
				st.velocity = clamp01(v / 100 * 0.9)
			}
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "swing" {
				d, err := parseSwing(dec)
				if err != nil {
					return err
				}
				d.Measure, d.OnsetDiv = st.measure, st.pos
				part.Directions = append(part.Directions, d)
				continue
			}
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

// Default swing when the notation says "swing" without a ratio: the triplet
// feel, 2:1, on eighth notes.
const (
	defaultSwingRatio = 2.0
	swingUnitEighth   = 0.5
	swingUnitSixteen  = 0.25
)

// parseSwing reads MusicXML 4's <swing>: either <straight/>, or
// <first>/<second> (the ratio of a swung pair) with an optional <swing-type>
// naming the swung note value (eighth by default).
func parseSwing(dec *xml.Decoder) (Direction, error) {
	d := Direction{Kind: DirSwing, SwingRatio: defaultSwingRatio, SwingUnit: swingUnitEighth}
	first, second := 0.0, 0.0
	for {
		tok, err := dec.Token()
		if err != nil {
			return d, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "straight":
				d.Kind = DirStraight
				if err := dec.Skip(); err != nil {
					return d, err
				}
			case "first", "second", "swing-type":
				text, err := readText(dec, t)
				if err != nil {
					return d, err
				}
				text = strings.TrimSpace(text)
				switch t.Name.Local {
				case "first":
					first, _ = strconv.ParseFloat(text, 64)
				case "second":
					second, _ = strconv.ParseFloat(text, 64)
				case "swing-type":
					if text == "16th" {
						d.SwingUnit = swingUnitSixteen
					}
				}
			default:
				if err := dec.Skip(); err != nil {
					return d, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "swing" {
				if d.Kind == DirStraight {
					d.SwingRatio, d.SwingUnit = 0, 0
				} else if first > 0 && second > 0 {
					d.SwingRatio = first / second
				}
				return d, nil
			}
		}
	}
}

// swingWords recognises a swing instruction written as text: "Swing",
// "Swung", "Shuffle" (2:1 eighths), the same with "16th"/"16ths" (2:1
// sixteenths), and "Straight"/"Even" (back to straight time).
func swingWords(text string) (Direction, bool) {
	s := strings.ToLower(strings.Join(strings.Fields(text), " "))
	s = strings.Trim(s, ".()")
	sixteenths := strings.Contains(s, "16")
	switch {
	case strings.HasPrefix(s, "straight"), strings.HasPrefix(s, "even 8"), strings.HasPrefix(s, "even eighth"),
		strings.HasPrefix(s, "even 16"), s == "even":
		return Direction{Kind: DirStraight}, true
	case strings.HasPrefix(s, "swing"), strings.HasPrefix(s, "swung"), strings.HasPrefix(s, "shuffle"),
		strings.HasPrefix(s, "16th swing"), strings.HasPrefix(s, "16th shuffle"):
		unit := swingUnitEighth
		if sixteenths {
			unit = swingUnitSixteen
		}
		return Direction{Kind: DirSwing, SwingRatio: defaultSwingRatio, SwingUnit: unit}, true
	}
	return Direction{}, false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// directionKind classifies a <words> direction into an agogic instruction.
// Matching is prefix-based on the normalized text so "rit.", "ritard.",
// "ritardando" and "riten." all land together.
func directionKind(text string) DirectionKind {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.TrimRight(s, ".")
	switch {
	case s == "":
		return DirNone
	case strings.HasPrefix(s, "rit"), strings.HasPrefix(s, "rall"),
		strings.HasPrefix(s, "allarg"), strings.HasPrefix(s, "slentando"),
		strings.HasPrefix(s, "smorz"):
		return DirRitardando
	case strings.HasPrefix(s, "accel"), strings.HasPrefix(s, "string"),
		strings.HasPrefix(s, "affrett"):
		return DirAccelerando
	case strings.HasPrefix(s, "a tempo"), strings.HasPrefix(s, "tempo primo"),
		strings.HasPrefix(s, "tempo i"), strings.HasPrefix(s, "in tempo"):
		return DirATempo
	}
	return DirNone
}

// parseDynamics reads a <dynamics> element and returns its velocity, or 0 when
// the marking is not one we recognise.
func parseDynamics(dec *xml.Decoder) (float64, error) {
	var out float64
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if v, ok := dynamicVelocity[t.Name.Local]; ok {
				out = v
			}
			if err := dec.Skip(); err != nil {
				return 0, err
			}
		case xml.EndElement:
			if t.Name.Local == "dynamics" {
				return out, nil
			}
		}
	}
}

// beatUnitQuarters maps a <beat-unit> note type to its length in quarter notes.
var beatUnitQuarters = map[string]float64{
	"breve": 8, "whole": 4, "half": 2, "quarter": 1,
	"eighth": 0.5, "16th": 0.25, "32nd": 0.125, "64th": 0.0625,
}

// The number is per BEAT-UNIT: "half = 100" is quarter = 200, "dotted quarter = 80" quarter = 120. A score marked only
// with a <metronome> (no <sound tempo>) otherwise plays at the wrong speed.
func parseMetronome(dec *xml.Decoder, start xml.StartElement, sc *Score, measure int) error {
	quarters := 1.0 // beat unit, in quarter notes; default quarter
	dots := 0
	perMinute := 0.0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "beat-unit":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				if q, ok := beatUnitQuarters[strings.TrimSpace(text)]; ok {
					quarters = q
				}
				continue
			case "beat-unit-dot":
				dots++
				if err := dec.Skip(); err != nil {
					return err
				}
				continue
			case "per-minute":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				if v, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
					perMinute = v
				}
				continue
			}
			if err := dec.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name.Local == "metronome" {
				if perMinute > 0 {
					// Each dot adds half of the previous value: . = 1.5x, .. = 1.75x.
					mult, add := 1.0, 0.5
					for i := 0; i < dots; i++ {
						mult += add
						add /= 2
					}
					sc.recordTempo(measure, perMinute*quarters*mult)
				}
				return nil
			}
		}
	}
}

// A note held across a barline is ONE event, never a re-attack the score does not contain; matched by (voice, staff,
// semitone), MusicXML's own tie identity.
func mergeTies(notes []Note) []Note {
	type key struct {
		voice, staff, semi int
		inst               string // kit piece: two drums on one line are different notes
	}

	open := map[key]int{} // key → index in out of the note holding the tie
	out := make([]Note, 0, len(notes))

	for _, n := range notes {
		k := key{n.Voice, n.Staff, n.Semitone, n.Instrument}
		if n.tieStop && !n.Rest {
			if idx, ok := open[k]; ok {
				out[idx].DurationDiv += n.DurationDiv
				// A fermata anywhere in the group belongs to the held note.
				if n.Articulations.Fermata {
					out[idx].Articulations.Fermata = true
				}
				if !n.tieStart {
					delete(open, k)
				}
				continue
			}
		}
		startsTie := n.tieStart && !n.Rest
		n.tieStart, n.tieStop = false, false
		out = append(out, n)
		if startsTie {
			open[k] = len(out) - 1
		}
	}
	return out
}

// readText reads the character data of the element started by start and consumes
// its end element.
func readText(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.StartElement:
			if err := dec.Skip(); err != nil {
				return "", err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return sb.String(), nil
			}
		}
	}
}
