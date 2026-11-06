// Package smf reads Standard MIDI Files (format 0 and 1) into the same score
// model the MusicXML parser produces, so the template generator treats a MIDI
// edition exactly like a MusicXML one.
//
// It exists for the Mutopia Project, whose Public-Domain editions are LilyPond
// sources that ship as LilyPond-generated MIDI. Two properties of that MIDI
// shape this package:
//
//   - LilyPond starts tick 0 at the START OF THE PICKUP bar (\partial), so the
//     first downbeat sits pickup-length ticks in. MIDI cannot say this; the
//     caller passes Options.PickupTicks, which the score catalogue corroborates
//     against the committed .ly source.
//   - Mutopia builds its MIDI with \unfoldRepeats, so measure numbers here are
//     PERFORMED bars, not printed ones.
//
// Channel 10 (index 9) is General MIDI percussion: its notes become unpitched
// notes carrying their GM key, exactly as a MusicXML drum part's would.
package smf

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

// Options carries what a MIDI file cannot express.
type Options struct {
	// PickupTicks is the length of the anacrusis: the first downbeat is at this
	// tick, and the ticks before it form measure 0.
	PickupTicks int
}

// percussionChannel is GM channel 10, zero-based.
const percussionChannel = 9

// a3MIDI is the MIDI key the score model calls Semitone 0.
const a3MIDI = 57

type rawNote struct {
	ch, key, vel int
	on, off      int
}

type track struct {
	name, instrument string
	notes            []rawNote
}

type tempoEvt struct {
	tick int
	bpm  float64
}

type meterEvt struct {
	tick, beats, beatType int
}

// Parse decodes data into a Score. Divisions is the file's ticks per quarter.
func Parse(data []byte, opt Options) (musicxml.Score, error) {
	var sc musicxml.Score
	if len(data) < 14 || string(data[:4]) != "MThd" {
		return sc, fmt.Errorf("smf: not a Standard MIDI File")
	}
	hlen := int(binary.BigEndian.Uint32(data[4:8]))
	if hlen < 6 || 8+hlen > len(data) {
		return sc, fmt.Errorf("smf: bad header length %d", hlen)
	}
	format := int(binary.BigEndian.Uint16(data[8:10]))
	ntrks := int(binary.BigEndian.Uint16(data[10:12]))
	division := int(binary.BigEndian.Uint16(data[12:14]))
	if format > 1 {
		return sc, fmt.Errorf("smf: format %d is not supported (0 or 1 only)", format)
	}
	if division&0x8000 != 0 || division == 0 {
		return sc, fmt.Errorf("smf: SMPTE time division is not supported")
	}
	if opt.PickupTicks < 0 {
		return sc, fmt.Errorf("smf: negative pickup")
	}

	var tracks []track
	var tempos []tempoEvt
	var meters []meterEvt
	copyright := ""
	pos := 8 + hlen
	for t := 0; t < ntrks; t++ {
		if pos+8 > len(data) || string(data[pos:pos+4]) != "MTrk" {
			return sc, fmt.Errorf("smf: track %d: missing MTrk chunk", t)
		}
		n := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		end := pos + 8 + n
		if end > len(data) {
			return sc, fmt.Errorf("smf: track %d: chunk overruns file", t)
		}
		tr, tt, mm, cr, err := parseTrack(data[pos+8 : end])
		if err != nil {
			return sc, fmt.Errorf("smf: track %d: %w", t, err)
		}
		if copyright == "" {
			copyright = cr
		}
		tracks = append(tracks, tr)
		tempos = append(tempos, tt...)
		meters = append(meters, mm...)
		pos = end
	}

	sc.Divisions = division
	sc.Mode = "major"
	sc.TempoBPM = 120
	sc.Rights = copyright
	if len(tracks) > 0 {
		sc.WorkTitle = tracks[0].name
	}

	sort.SliceStable(meters, func(i, j int) bool { return meters[i].tick < meters[j].tick })
	sort.SliceStable(tempos, func(i, j int) bool { return tempos[i].tick < tempos[j].tick })
	if len(meters) > 0 {
		sc.TimeBeats, sc.TimeBeatType = meters[0].beats, meters[0].beatType
	} else {
		sc.TimeBeats, sc.TimeBeatType = 4, 4
	}

	lastTick := 0
	for _, tr := range tracks {
		for _, n := range tr.notes {
			if n.off > lastTick {
				lastTick = n.off
			}
		}
	}
	measures := buildMeasures(meters, division, opt.PickupTicks, lastTick)

	measureAt := func(tick int) musicxml.Measure {
		i := sort.Search(len(measures), func(i int) bool { return measures[i].OnsetDiv > tick })
		if i == 0 {
			return measures[0]
		}
		return measures[i-1]
	}

	// Tempo marks are recorded against the bar they fall in, like MusicXML's.
	for _, t := range tempos {
		recordTempo(&sc, measureAt(t.tick).Number, t.bpm)
	}

	for ti, tr := range tracks {
		// One part per (track, channel): a format-0 file puts every channel in
		// one track, and percussion must not share a part with pitched notes.
		byCh := map[int][]rawNote{}
		var chans []int
		for _, n := range tr.notes {
			if _, ok := byCh[n.ch]; !ok {
				chans = append(chans, n.ch)
			}
			byCh[n.ch] = append(byCh[n.ch], n)
		}
		sort.Ints(chans)
		for _, ch := range chans {
			notes := byCh[ch]
			sort.SliceStable(notes, func(i, j int) bool {
				if notes[i].on != notes[j].on {
					return notes[i].on < notes[j].on
				}
				return notes[i].key > notes[j].key
			})
			part := musicxml.Part{
				ID:        fmt.Sprintf("T%dC%d", ti, ch+1),
				Name:      partName(tr, ti, ch),
				Divisions: division,
				Measures:  measures,
			}
			dynamic := hasDynamics(notes)
			lastOnset := -1
			for _, n := range notes {
				m := measureAt(n.on)
				note := musicxml.Note{
					OnsetDiv:    n.on,
					DurationDiv: n.off - n.on,
					Voice:       1,
					Staff:       1,
					Measure:     m.Number,
					Movement:    1,
					Chord:       n.on == lastOnset,
				}
				if dynamic {
					note.Velocity = float64(n.vel) / 127
				}
				if ch == percussionChannel {
					note.Unpitched = true
					note.PercKey = n.key
					note.Instrument = gmInstrumentID(n.key)
					note.Semitone = n.key - a3MIDI
					if part.Instruments == nil {
						part.Instruments = map[string]musicxml.PartInstrument{}
					}
					part.Instruments[note.Instrument] = musicxml.PartInstrument{
						ID: note.Instrument, Name: GMPercussionName(n.key), PercKey: n.key,
					}
				} else {
					note.Semitone = n.key - a3MIDI
				}
				part.Notes = append(part.Notes, note)
				lastOnset = n.on
			}
			sc.Parts = append(sc.Parts, part)
		}
	}
	return sc, nil
}

// recordTempo mirrors musicxml's rule: the first mark is the opening tempo, a
// later mark in the same bar does not stack.
func recordTempo(sc *musicxml.Score, measure int, bpm float64) {
	if bpm <= 0 {
		return
	}
	if len(sc.Tempos) == 0 {
		sc.TempoBPM = bpm
	}
	for _, t := range sc.Tempos {
		if t.Measure == measure {
			return
		}
	}
	sc.Tempos = append(sc.Tempos, musicxml.TempoMark{Measure: measure, BPM: bpm})
	sort.SliceStable(sc.Tempos, func(i, j int) bool { return sc.Tempos[i].Measure < sc.Tempos[j].Measure })
}

// buildMeasures lays bars from tick 0 through lastTick: an optional pickup bar
// numbered 0, then full bars numbered from 1, each sized by the meter in force
// at its start.
func buildMeasures(meters []meterEvt, division, pickup, lastTick int) []musicxml.Measure {
	meterAt := func(tick int) (int, int) {
		b, bt := 4, 4
		for _, m := range meters {
			if m.tick > tick {
				break
			}
			b, bt = m.beats, m.beatType
		}
		return b, bt
	}
	var out []musicxml.Measure
	tick := 0
	if pickup > 0 {
		b, bt := meterAt(0)
		out = append(out, musicxml.Measure{Movement: 1, Number: 0, OnsetDiv: 0, LengthDiv: pickup, Beats: b, BeatType: bt})
		tick = pickup
	}
	num := 1
	for {
		b, bt := meterAt(tick)
		length := division * 4 * b / bt
		if length <= 0 {
			length = division * 4
		}
		out = append(out, musicxml.Measure{Movement: 1, Number: num, OnsetDiv: tick, LengthDiv: length, Beats: b, BeatType: bt})
		tick += length
		num++
		if tick >= lastTick {
			break
		}
	}
	return out
}

// hasDynamics reports whether a part's velocities vary. LilyPond writes one
// constant velocity when the score carries no dynamics; reading that constant
// as a dynamic would invent a uniform "forte" the score never marked.
func hasDynamics(notes []rawNote) bool {
	for _, n := range notes[1:] {
		if n.vel != notes[0].vel {
			return true
		}
	}
	return false
}

func partName(tr track, ti, ch int) string {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(tr.name), ":"))
	if name == "" {
		name = strings.TrimSpace(tr.instrument)
	}
	if name == "" {
		name = fmt.Sprintf("Track %d", ti)
	}
	if ch == percussionChannel {
		name += " (percussion)"
	}
	return name
}

func gmInstrumentID(key int) string { return fmt.Sprintf("gm%d", key) }

// parseTrack decodes one MTrk chunk body.
func parseTrack(b []byte) (tr track, tempos []tempoEvt, meters []meterEvt, copyright string, err error) {
	open := map[[2]int][]rawNote{} // (channel, key) → sounding notes, FIFO
	tick := 0
	status := byte(0)
	i := 0
	closeNote := func(ch, key int) {
		k := [2]int{ch, key}
		q := open[k]
		if len(q) == 0 {
			return
		}
		n := q[0]
		open[k] = q[1:]
		n.off = tick
		if n.off > n.on {
			tr.notes = append(tr.notes, n)
		}
	}
	for i < len(b) {
		d, n, e := vlq(b, i)
		if e != nil {
			return tr, nil, nil, "", e
		}
		i = n
		tick += d
		if i >= len(b) {
			return tr, nil, nil, "", fmt.Errorf("truncated event at tick %d", tick)
		}
		c := b[i]
		switch {
		case c == 0xFF:
			if i+2 > len(b) {
				return tr, nil, nil, "", fmt.Errorf("truncated meta event")
			}
			typ := b[i+1]
			l, n, e := vlq(b, i+2)
			if e != nil || n+l > len(b) {
				return tr, nil, nil, "", fmt.Errorf("bad meta event length")
			}
			payload := b[n : n+l]
			i = n + l
			switch typ {
			case 0x02:
				copyright = strings.TrimSpace(string(payload))
			case 0x03:
				if tr.name == "" {
					tr.name = string(payload)
				}
			case 0x04:
				if tr.instrument == "" {
					tr.instrument = string(payload)
				}
			case 0x51:
				if len(payload) == 3 {
					us := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
					if us > 0 {
						tempos = append(tempos, tempoEvt{tick: tick, bpm: 60e6 / float64(us)})
					}
				}
			case 0x58:
				if len(payload) >= 2 && payload[0] > 0 {
					meters = append(meters, meterEvt{tick: tick, beats: int(payload[0]), beatType: 1 << payload[1]})
				}
			case 0x2F:
				i = len(b)
			}
		case c == 0xF0 || c == 0xF7:
			l, n, e := vlq(b, i+1)
			if e != nil || n+l > len(b) {
				return tr, nil, nil, "", fmt.Errorf("bad sysex length")
			}
			i = n + l
		default:
			if c&0x80 != 0 {
				status = c
				i++
			} else if status == 0 {
				return tr, nil, nil, "", fmt.Errorf("running status with no prior status at tick %d", tick)
			}
			ch := int(status & 0x0F)
			argc := 2
			if hi := status & 0xF0; hi == 0xC0 || hi == 0xD0 {
				argc = 1
			}
			if i+argc > len(b) {
				return tr, nil, nil, "", fmt.Errorf("truncated channel event")
			}
			a1 := int(b[i])
			a2 := 0
			if argc == 2 {
				a2 = int(b[i+1])
			}
			i += argc
			switch status & 0xF0 {
			case 0x90:
				if a2 == 0 {
					closeNote(ch, a1) // note-on velocity 0 is a note-off
				} else {
					k := [2]int{ch, a1}
					open[k] = append(open[k], rawNote{ch: ch, key: a1, vel: a2, on: tick})
				}
			case 0x80:
				closeNote(ch, a1)
			}
		}
	}
	// A note never released ends at the track end.
	for k, q := range open {
		for range q {
			closeNote(k[0], k[1])
		}
	}
	sort.SliceStable(tr.notes, func(i, j int) bool { return tr.notes[i].on < tr.notes[j].on })
	return tr, tempos, meters, copyright, nil
}

// vlq decodes a MIDI variable-length quantity at b[i], returning the value and
// the index just past it.
func vlq(b []byte, i int) (int, int, error) {
	v := 0
	for n := 0; n < 4; n++ {
		if i >= len(b) {
			return 0, i, fmt.Errorf("truncated variable-length quantity")
		}
		c := b[i]
		i++
		v = v<<7 | int(c&0x7F)
		if c&0x80 == 0 {
			return v, i, nil
		}
	}
	return 0, i, fmt.Errorf("variable-length quantity longer than 4 bytes")
}
