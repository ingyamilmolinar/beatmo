// Package musicxml parses MusicXML score-partwise documents into a structured Score.
// It uses a streaming xml.Decoder so that element order within a measure is preserved,
// which is required for correct chord/backup/forward handling.
package musicxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
)

// Note is a single musical event within a Part.
type Note struct {
	Semitone    int  // pitch offset from A3 (A3 = MIDI 57 → Semitone 0)
	OnsetDiv    int  // onset in divisions from part start
	DurationDiv int  // duration in divisions
	Voice       int  // voice number (defaults to 1)
	Rest        bool // true when the note element contains a <rest/>
	Chord       bool // true when the note element contains a <chord/>
}

// Part corresponds to a <part> element.
type Part struct {
	ID    string
	Notes []Note
}

// Score is the top-level result of parsing a score-partwise MusicXML document.
type Score struct {
	Divisions    int
	KeyFifths    int
	Mode         string
	TempoBPM     float64
	TimeBeats    int
	TimeBeatType int
	Parts        []Part
}

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

// Parse parses a MusicXML score-partwise document and returns a Score.
func Parse(data []byte) (Score, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))

	var sc Score
	sc.TempoBPM = 120.0 // default
	sc.Mode = "major"   // default

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
		case "part":
			part, err := parsePart(dec, se, &sc)
			if err != nil {
				return Score{}, fmt.Errorf("musicxml: part %q: %w", part.ID, err)
			}
			sc.Parts = append(sc.Parts, part)
		}
	}

	return sc, nil
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

	// pos tracks the current division cursor within this part (accumulates across measures).
	pos := 0
	// lastNonChordOnset tracks the onset of the most recent non-chord note (for chord notes).
	lastNonChordOnset := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			return part, fmt.Errorf("reading part: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "measure":
				if err := parseMeasure(dec, t, sc, &part, &pos, &lastNonChordOnset); err != nil {
					return part, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "part" {
				return part, nil
			}
		}
	}
}

// parseMeasure reads a <measure> element, dispatching child elements in order.
func parseMeasure(dec *xml.Decoder, start xml.StartElement, sc *Score, part *Part, pos *int, lastNonChordOnset *int) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("reading measure: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "attributes":
				if err := parseAttributes(dec, t, sc); err != nil {
					return err
				}
			case "note":
				n, err := parseNote(dec, t, pos, lastNonChordOnset)
				if err != nil {
					return err
				}
				part.Notes = append(part.Notes, n)
			case "backup":
				d, err := parseDuration(dec, t)
				if err != nil {
					return err
				}
				*pos -= d
			case "forward":
				d, err := parseDuration(dec, t)
				if err != nil {
					return err
				}
				*pos += d
			case "sound":
				// tempo attribute
				for _, a := range t.Attr {
					if a.Name.Local == "tempo" {
						v, err := strconv.ParseFloat(a.Value, 64)
						if err == nil {
							sc.TempoBPM = v
						}
					}
				}
				if err := dec.Skip(); err != nil {
					return err
				}
			case "direction":
				if err := parseDirection(dec, t, sc); err != nil {
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

// parseAttributes reads an <attributes> element and updates sc.
func parseAttributes(dec *xml.Decoder, start xml.StartElement, sc *Score) error {
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
				v, err := strconv.Atoi(text)
				if err != nil {
					return fmt.Errorf("divisions: %w", err)
				}
				if sc.Divisions == 0 {
					sc.Divisions = v
				}
			case "key":
				if err := parseKey(dec, t, sc); err != nil {
					return err
				}
			case "time":
				if err := parseTime(dec, t, sc); err != nil {
					return err
				}
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
				v, err := strconv.Atoi(text)
				if err != nil {
					return fmt.Errorf("fifths: %w", err)
				}
				sc.KeyFifths = v
			case "mode":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				sc.Mode = text
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

// parseTime reads a <time> element and updates sc.
func parseTime(dec *xml.Decoder, start xml.StartElement, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "beats":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				v, err := strconv.Atoi(text)
				if err != nil {
					return fmt.Errorf("beats: %w", err)
				}
				sc.TimeBeats = v
			case "beat-type":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				v, err := strconv.Atoi(text)
				if err != nil {
					return fmt.Errorf("beat-type: %w", err)
				}
				sc.TimeBeatType = v
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "time" {
				return nil
			}
		}
	}
}

// parseNote reads a <note> element and returns a Note.
// pos and lastNonChordOnset are updated as per chord/non-chord rules.
func parseNote(dec *xml.Decoder, start xml.StartElement, pos *int, lastNonChordOnset *int) (Note, error) {
	var n Note
	n.Voice = 1 // default

	// We need to peek inside the element to detect <chord/>, <rest/>, <pitch>, etc.
	// Collect all child start elements within this note.
	var step string
	var alter, octave int
	hasChord := false
	hasRest := false
	hasPitch := false

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
				// <chord/> is self-closing; consume its end
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
			case "duration":
				text, err := readText(dec, t)
				if err != nil {
					return Note{}, err
				}
				v, err := strconv.Atoi(text)
				if err != nil {
					return Note{}, fmt.Errorf("duration: %w", err)
				}
				n.DurationDiv = v
			case "voice":
				text, err := readText(dec, t)
				if err != nil {
					return Note{}, err
				}
				v, err := strconv.Atoi(text)
				if err != nil {
					return Note{}, fmt.Errorf("voice: %w", err)
				}
				n.Voice = v
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

	if hasChord {
		// Chord note shares the onset of the previous non-chord note; do not advance pos.
		n.OnsetDiv = *lastNonChordOnset
	} else {
		// Non-chord note: onset is current pos, then advance by duration.
		n.OnsetDiv = *pos
		*lastNonChordOnset = *pos
		*pos += n.DurationDiv
	}

	if hasRest {
		n.Semitone = 0
	} else if hasPitch {
		n.Semitone = pitchToSemitone(step, alter, octave)
	}

	return n, nil
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
				step = text
			case "alter":
				text, terr := readText(dec, t)
				if terr != nil {
					return "", 0, 0, terr
				}
				// alter may be a float in MusicXML (e.g. 1 or -1)
				v, terr := strconv.ParseFloat(text, 64)
				if terr != nil {
					return "", 0, 0, fmt.Errorf("alter: %w", terr)
				}
				alter = int(v)
			case "octave":
				text, terr := readText(dec, t)
				if terr != nil {
					return "", 0, 0, terr
				}
				v, terr := strconv.Atoi(text)
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
				v, err := strconv.Atoi(text)
				if err != nil {
					return 0, fmt.Errorf("backup/forward duration: %w", err)
				}
				dur = v
			} else {
				if err := dec.Skip(); err != nil {
					return 0, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return dur, nil
			}
		}
	}
}

// parseDirection reads a <direction> element looking for <metronome><per-minute>.
func parseDirection(dec *xml.Decoder, start xml.StartElement, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "metronome":
				if err := parseMetronome(dec, t, sc); err != nil {
					return err
				}
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "direction" {
				return nil
			}
		}
	}
}

// parseMetronome reads a <metronome> element and updates sc.TempoBPM if per-minute is found.
func parseMetronome(dec *xml.Decoder, start xml.StartElement, sc *Score) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "per-minute":
				text, err := readText(dec, t)
				if err != nil {
					return err
				}
				v, err := strconv.ParseFloat(text, 64)
				if err != nil {
					return fmt.Errorf("per-minute: %w", err)
				}
				sc.TempoBPM = v
			default:
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "metronome" {
				return nil
			}
		}
	}
}

// readText reads the character data within an already-opened element, then
// consumes the closing end element. It does NOT skip the element via dec.Skip().
func readText(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var buf []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf = append(buf, t...)
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return string(buf), nil
			}
		case xml.StartElement:
			// Unexpected nested element; skip it.
			if err := dec.Skip(); err != nil {
				return "", err
			}
		}
	}
}
