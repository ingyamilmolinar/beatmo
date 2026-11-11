package smf

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// mid assembles a format-1 file from track bodies (already delta-encoded).
func mid(division int, tracks ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("MThd")
	binary.Write(&b, binary.BigEndian, uint32(6))
	binary.Write(&b, binary.BigEndian, uint16(1))
	binary.Write(&b, binary.BigEndian, uint16(len(tracks)))
	binary.Write(&b, binary.BigEndian, uint16(division))
	for _, t := range tracks {
		b.WriteString("MTrk")
		binary.Write(&b, binary.BigEndian, uint32(len(t)))
		b.Write(t)
	}
	return b.Bytes()
}

// ev is a tiny event builder: delta ticks then raw bytes.
type ev struct{ buf bytes.Buffer }

func (e *ev) delta(d int) *ev {
	var tmp [4]byte
	n := 0
	tmp[3] = byte(d & 0x7F)
	n = 1
	for d >>= 7; d > 0; d >>= 7 {
		tmp[3-n] = byte(d&0x7F) | 0x80
		n++
	}
	e.buf.Write(tmp[4-n:])
	return e
}
func (e *ev) raw(b ...byte) *ev { e.buf.Write(b); return e }
func (e *ev) meta(d int, typ byte, payload ...byte) *ev {
	e.delta(d).raw(0xFF, typ, byte(len(payload)))
	return e.raw(payload...)
}
func (e *ev) end() []byte { e.meta(0, 0x2F); return e.buf.Bytes() }

// control is the LilyPond-style conductor track: title, 2/4, quarter = 100.
func control() []byte {
	e := &ev{}
	e.meta(0, 0x03, []byte("Test Rag")...)
	e.meta(0, 0x58, 2, 2, 24, 8)      // 2/4
	e.meta(0, 0x51, 0x09, 0x27, 0xC0) // 600000 us = 100 bpm
	return e.end()
}

func TestParse_PitchedTracksWithPickup(t *testing.T) {
	const q = 384
	rh := &ev{}
	rh.meta(0, 0x03, []byte("up:")...)
	// pickup eighth A4, then the downbeat: C5 + E5 chord (quarter), with
	// RUNNING STATUS and a note-on velocity 0 standing in for the note-off.
	rh.delta(0).raw(0x90, 69, 90).delta(q/2).raw(69, 0)
	rh.delta(0).raw(72, 90).delta(0).raw(76, 90).delta(q).raw(0x80, 72, 0).delta(0).raw(0x80, 76, 0)
	data := mid(q, control(), rh.end())

	sc, err := Parse(data, Options{PickupTicks: q / 2})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Divisions != q || sc.TimeBeats != 2 || sc.TimeBeatType != 4 {
		t.Fatalf("divisions/meter = %d %d/%d", sc.Divisions, sc.TimeBeats, sc.TimeBeatType)
	}
	if sc.TempoBPM < 99.99 || sc.TempoBPM > 100.01 {
		t.Errorf("tempo %.3f want 100", sc.TempoBPM)
	}
	if sc.WorkTitle != "Test Rag" {
		t.Errorf("title %q", sc.WorkTitle)
	}
	if len(sc.Parts) != 1 || sc.Parts[0].Name != "up" {
		t.Fatalf("parts %+v", sc.Parts)
	}
	notes := sc.Parts[0].Notes
	if len(notes) != 3 {
		t.Fatalf("notes %+v", notes)
	}
	type want struct{ semi, onset, dur, measure int }
	for i, w := range []want{{12, 0, q / 2, 0}, {19, q / 2, q, 1}, {15, q / 2, q, 1}} {
		n := notes[i]
		if n.Semitone != w.semi || n.OnsetDiv != w.onset || n.DurationDiv != w.dur || n.Measure != w.measure {
			t.Errorf("note %d = semi %d onset %d dur %d measure %d, want %+v", i, n.Semitone, n.OnsetDiv, n.DurationDiv, n.Measure, w)
		}
		if n.Velocity != 0 {
			t.Errorf("note %d: constant velocities must read as NO dynamics, got %v", i, n.Velocity)
		}
	}
	if !notes[2].Chord {
		t.Error("second note at the downbeat should be flagged as a chord tone")
	}
	ms := sc.Parts[0].Measures
	if len(ms) < 2 || ms[0].Number != 0 || ms[0].LengthDiv != q/2 || ms[1].Number != 1 || ms[1].OnsetDiv != q/2 || ms[1].LengthDiv != 2*q {
		t.Errorf("measures %+v: want pickup bar 0 of an eighth, then 2/4 bars from the pickup's end", ms)
	}
}

func TestParse_VaryingVelocityIsADynamic(t *testing.T) {
	tr := &ev{}
	tr.delta(0).raw(0x90, 60, 127).delta(10).raw(0x80, 60, 0)
	tr.delta(0).raw(0x90, 62, 64).delta(10).raw(0x80, 62, 0)
	sc, err := Parse(mid(96, control(), tr.end()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	n := sc.Parts[0].Notes
	if n[0].Velocity != 1 || n[1].Velocity < 0.50 || n[1].Velocity > 0.51 {
		t.Errorf("velocities %v %v, want 1 and 64/127", n[0].Velocity, n[1].Velocity)
	}
}

func TestParse_Channel10IsUnpitchedPercussion(t *testing.T) {
	tr := &ev{}
	tr.meta(0, 0x03, []byte("drums")...)
	tr.delta(0).raw(0x99, 36, 100).delta(0).raw(42, 80).delta(48).raw(0x89, 36, 0).delta(0).raw(42, 0)
	tr.delta(0).raw(0x90, 57, 90).delta(48).raw(0x80, 57, 0) // a pitched note on channel 1 in the same track
	sc, err := Parse(mid(96, control(), tr.end()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Parts) != 2 {
		t.Fatalf("want the pitched and the percussion channel as separate parts, got %d", len(sc.Parts))
	}
	var perc = sc.Parts[1]
	if sc.Parts[0].Notes[0].Unpitched || sc.Parts[0].Notes[0].Semitone != 0 {
		t.Errorf("channel-1 note should be pitched A3: %+v", sc.Parts[0].Notes[0])
	}
	if len(perc.Notes) != 2 {
		t.Fatalf("percussion notes %+v", perc.Notes)
	}
	keys := map[int]bool{}
	for _, n := range perc.Notes {
		if !n.Unpitched || n.Instrument == "" {
			t.Errorf("percussion note not unpitched/identified: %+v", n)
		}
		keys[n.PercKey] = true
		if perc.Instruments[n.Instrument].PercKey != n.PercKey {
			t.Errorf("instrument table disagrees with note %+v", n)
		}
	}
	if !keys[36] || !keys[42] {
		t.Errorf("GM keys %v, want 36 and 42", keys)
	}
	if got := perc.Instruments["gm42"].Name; got != "Closed Hi-Hat" {
		t.Errorf("gm42 named %q", got)
	}
}

func TestParse_MeterChangeResizesBars(t *testing.T) {
	c := &ev{}
	c.meta(0, 0x58, 4, 2, 24, 8)    // 4/4
	c.meta(4*96, 0x58, 6, 3, 24, 8) // after one bar: 6/8
	tr := &ev{}
	tr.delta(0).raw(0x90, 60, 90).delta(4*96+3*96+10).raw(0x80, 60, 0) // runs into bar 3
	sc, err := Parse(mid(96, c.end(), tr.end()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	ms := sc.Parts[0].Measures
	if len(ms) != 3 || ms[0].LengthDiv != 4*96 || ms[1].LengthDiv != 3*96 || ms[1].Beats != 6 || ms[1].BeatType != 8 {
		t.Errorf("measures %+v: want a 4/4 bar then 6/8 bars", ms)
	}
}

func TestParse_RejectsWhatItCannotRead(t *testing.T) {
	bad := [][]byte{
		[]byte("not midi at all"),
		func() []byte { b := mid(96, control()); b[13] = 0; b[12] = 0xE7; return b }(), // SMPTE
		func() []byte { b := mid(96, control()); b[9] = 2; return b }(),                // format 2
		func() []byte { b := mid(96, control()); return b[:len(b)-3] }(),               // truncated
	}
	for i, b := range bad {
		if _, err := Parse(b, Options{}); err == nil {
			t.Errorf("case %d parsed without error", i)
		}
	}
	if _, err := Parse(mid(96, control()), Options{PickupTicks: -1}); err == nil {
		t.Error("negative pickup accepted")
	}
}
