package main

import "testing"

// TestAsturias_DiatonicToEMinor asserts every note in the generated Asturias circuit
// is in the E natural-minor scale (pitch classes A,B,C,D,E,F#,G), matching the piece's key/mode.
func TestAsturias_DiatonicToEMinor(t *testing.T) {
	// pc = ((pitch%12)+12)%12 with A3=0: A=0,B=2,C=3,D=5,E=7,F#=9,G=10.
	eMinor := map[int]bool{0: true, 2: true, 3: true, 5: true, 7: true, 9: true, 10: true}
	sh := asturiasShowcase()
	for ri, r := range sh.Rows {
		for _, h := range r.Hits {
			pc := ((int(h.Pitch) % 12) + 12) % 12
			if !eMinor[pc] {
				t.Errorf("row %d: non-E-minor pitch class %d (pitch %v)", ri, pc, h.Pitch)
			}
		}
	}
}

// TestAsturias_AudibleVolume guards against the "loads but no sound" regression:
// every instrument must carry a nonzero volume, otherwise import sets row.Volume=0
// (gain 0) and the whole circuit is silent on load.
func TestAsturias_AudibleVolume(t *testing.T) {
	sh := asturiasShowcase()
	if len(sh.Insts) == 0 {
		t.Fatal("asturias produced no instruments")
	}
	for i, in := range sh.Insts {
		if in.Volume <= 0 {
			t.Errorf("asturias inst %d (%s) volume=%v — silent on load", i, in.ID, in.Volume)
		}
	}
}

// TestAsturias_HasBPedalVoice asserts one voice is the constant open-B (B3 = +2) pedal —
// the defining texture of the Asturias opening.
func TestAsturias_HasBPedalVoice(t *testing.T) {
	sh := asturiasShowcase()
	for _, r := range sh.Rows {
		if len(r.Hits) < 4 {
			continue
		}
		allB3 := true
		for _, h := range r.Hits {
			if h.Pitch != 2 { // B3 = +2 semitones from A3
				allB3 = false
				break
			}
		}
		if allB3 {
			return // found the B3 pedal voice
		}
	}
	t.Fatal("no constant B3 (+2) pedal voice found in Asturias circuit")
}

// TestAsturias_BassDescends asserts the bass voice carries the descending E-minor line
// (starts on E3 = -5, reaches a lower note) rather than a flat pedal.
func TestAsturias_BassDescends(t *testing.T) {
	sh := asturiasShowcase()
	var bass []float64
	for _, r := range sh.Rows {
		// the non-pedal voice (has more than one distinct pitch)
		distinct := map[float64]bool{}
		for _, h := range r.Hits {
			distinct[h.Pitch] = true
		}
		if len(distinct) > 1 {
			for _, h := range r.Hits {
				bass = append(bass, h.Pitch)
			}
			break
		}
	}
	if len(bass) < 3 {
		t.Fatalf("bass voice too short: %v", bass)
	}
	if bass[0] != -5 { // E3
		t.Errorf("bass should open on E3 (-5), got %v", bass[0])
	}
	min := bass[0]
	for _, p := range bass {
		if p < min {
			min = p
		}
	}
	if min >= bass[0] {
		t.Errorf("bass should descend below its opening note %v (min %v)", bass[0], min)
	}
}
