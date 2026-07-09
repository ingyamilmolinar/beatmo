package main

// Eagles — "Hotel California" (1976). B minor, 75 BPM (exact MIDI tempo).
// Note data from BitMidi 58321 (13 labeled tracks) + 101107 (full song incl.
// the outro cascade, double-time notation converted). Personnel/production
// from Wikipedia (Felder 12-string + Les Paul, Walsh Telecaster, Meisner bass,
// Henley vocal/drums; "Mexican Reggae" working title). Dossier:
// eagles-hotel-california_dossier.md.
//
// 20-bar arc: bars 1–8 the fingerpicked intro cycle (Bm F#7 A E G D Em F#7)
// with the octave-pair guitar fills and whole-note bass; bars 9–16 verse 1
// (vocal substitute, R-5-O bass, the one-drop-flavored groove with back-half
// cabasa pairs); bars 17–20 the iconic dual-guitar harmony cascade.
//
// GROOVE: dead-straight 16ths — the lean is orchestration (cabasa on the back
// half of each beat, kick pushing the and-of-4). Vocal phrases enter an 8th
// behind each downbeat and are additionally laid back slightly.
func hotelCaliforniaShowcase() Showcase {
	// One bar of the 12-string picking shape: TOP-MID-LOW cycling 16ths on
	// steps 0–7, then the high cap note held.
	arpBar := func(base int, top, mid, low, cap float64) []Hit {
		return []Hit{
			{Step: base, Pitch: top, Dur: 0.24, Vol: 0.85}, {Step: base + 1, Pitch: mid, Dur: 0.24, Vol: 0.5},
			{Step: base + 2, Pitch: low, Dur: 0.24, Vol: 0.5}, {Step: base + 3, Pitch: top, Dur: 0.24, Vol: 0.85},
			{Step: base + 4, Pitch: mid, Dur: 0.24, Vol: 0.5}, {Step: base + 5, Pitch: low, Dur: 0.24, Vol: 0.5},
			{Step: base + 6, Pitch: top, Dur: 0.24, Vol: 0.85}, {Step: base + 7, Pitch: mid, Dur: 0.24, Vol: 0.5},
			{Step: base + 8, Pitch: cap, Dur: 1.5, Vol: 0.85},
		}
	}
	// The 8-chord cycle: Bm F#7 A E G D Em F#7 (TOP/MID/LOW/cap per bar).
	arpCycle := func(base int) []Hit {
		return concat(
			arpBar(base+0, 9, 5, 2, 14), arpBar(base+16, 9, 4, 1, 13),
			arpBar(base+32, 7, 4, 0, 12), arpBar(base+48, 7, 2, -1, 11),
			arpBar(base+64, 5, 2, -2, 10), arpBar(base+80, 5, 0, -3, 9),
			arpBar(base+96, 7, 2, -2, 10), arpBar(base+112, 9, 4, 1, 13),
		)
	}
	// Whole-bar triad rolls (the doubled 12-string rhythm bed), all 20 bars.
	padCycle := func(base int) []Hit {
		return concat(
			chordAt(base+0, 3.8, 0.5, 2, 5, 9), chordAt(base+16, 3.8, 0.5, 1, 4, 9),
			chordAt(base+32, 3.8, 0.5, 0, 4, 7), chordAt(base+48, 3.8, 0.5, -1, 2, 7),
			chordAt(base+64, 3.8, 0.5, -2, 2, 5), chordAt(base+80, 3.8, 0.5, -3, 0, 5),
			chordAt(base+96, 3.8, 0.5, -2, 2, 7), chordAt(base+112, 3.8, 0.5, 1, 4, 9),
		)
	}
	// Verse bass bar: R R 5 O · 5 O O 5(held) — Meisner's lope.
	// Encoded +12 (bass-guitar renders one octave down).
	bassBar := func(base int, r float64) []Hit {
		return []Hit{
			{Step: base, Pitch: r, Dur: 0.24, Vol: 0.85}, {Step: base + 1, Pitch: r, Dur: 0.24, Vol: 0.8},
			{Step: base + 2, Pitch: r + 7, Dur: 0.24, Vol: 0.82}, {Step: base + 3, Pitch: r + 12, Dur: 0.24, Vol: 0.85},
			{Step: base + 7, Pitch: r + 7, Dur: 0.24, Vol: 0.8}, {Step: base + 8, Pitch: r + 12, Dur: 0.13, Vol: 0.85},
			{Step: base + 9, Pitch: r + 12, Dur: 0.13, Vol: 0.8}, {Step: base + 10, Pitch: r + 7, Dur: 1.5, Vol: 0.82},
		}
	}
	// One bar of the harmony-cascade 16ths (hi/mid/lo inversion alternation);
	// emits the TOP voice when top=true, else the BOTTOM voice.
	cascadeBar := func(base int, hi, mid, lo [2]float64, top bool) []Hit {
		idx := []int{0, 1, 2, 0, 1, 2, 0, 1, 1, 2, 0, 1, 2, 0, 1, 0}
		var out []Hit
		for s, k := range idx {
			var p float64
			switch k {
			case 0:
				p = hi[map[bool]int{true: 1, false: 0}[top]]
			case 1:
				p = mid[map[bool]int{true: 1, false: 0}[top]]
			default:
				p = lo[map[bool]int{true: 1, false: 0}[top]]
			}
			v := 0.62
			if k == 0 {
				v = 0.78
			}
			out = append(out, Hit{Step: base + s, Pitch: p, Dur: 0.2, Vol: v})
		}
		return out
	}
	verseRoots := []float64{-10, -15, -12, -17, -14, -19, -17, -15}
	var verseBass []Hit
	for i, r := range verseRoots {
		verseBass = append(verseBass, bassBar(128+i*16, r)...)
	}
	return Showcase{
		Stem: "eagles-hotel-california", BPM: 75, Subdiv: 16, Bars: 20,
		Insts: []InstSpec{
			{ID: "guitar-steel", Name: "12-String", Volume: 0.7, Pan: -0.1, ReverbSend: 0.16},
			{ID: "guitar-steel-warm", Name: "Rhythm", Volume: 0.42, Pan: 0.2, ReverbSend: 0.18},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "sax", Name: "Vocal", Volume: 0.66, ReverbSend: 0.18},
			{ID: "guitar-electric-neck", Name: "Fill Hi", Volume: 0.5, Pan: 0.3, ReverbSend: 0.2},
			{ID: "guitar-electric-neck", Name: "Fill Lo", Volume: 0.42, Pan: -0.3, ReverbSend: 0.2},
			{ID: "guitar-electric", Name: "Lead 1", Volume: 0.62, Pan: 0.35, ReverbSend: 0.22},
			{ID: "guitar-electric", Name: "Lead 2", Volume: 0.56, Pan: -0.35, ReverbSend: 0.22},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.9},
			{ID: "sidestick", Name: "Rim", Volume: 0.72},
			{ID: "snare", Name: "Snare", Volume: 0.78},
			{ID: "hihat", Name: "Hihat", Volume: 0.5},
			{ID: "shaker", Name: "Cabasa", Volume: 0.38, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
			{ID: "tom", Name: "Toms", Volume: 0.65},
		},
		Rows: []RowSpec{
			// 12-string arpeggio: intro cycle + continuing under the verse.
			{Inst: "guitar-steel", Hits: concat(arpCycle(0), arpCycle(128))},
			// Rhythm bed all 20 bars (cascade bars: Bm F#7 A E).
			{Inst: "guitar-steel-warm", Hits: concat(
				padCycle(0), padCycle(128),
				chordAt(256, 3.8, 0.5, 2, 5, 9), chordAt(272, 3.8, 0.5, 1, 4, 9),
				chordAt(288, 3.8, 0.5, 0, 4, 7), chordAt(304, 3.8, 0.5, 2, 7, 11),
			)},
			// Bass: whole-note roots (intro), verse lope, cascade whole notes.
			{Inst: "bass-guitar", Hits: concat(
				[]Hit{
					{Step: 0, Pitch: -10, Dur: 3.8, Vol: 0.8}, {Step: 16, Pitch: -15, Dur: 3.8, Vol: 0.8},
					{Step: 32, Pitch: -12, Dur: 3.8, Vol: 0.8}, {Step: 48, Pitch: -17, Dur: 3.8, Vol: 0.8},
					{Step: 64, Pitch: -14, Dur: 3.8, Vol: 0.8}, {Step: 80, Pitch: -19, Dur: 3.8, Vol: 0.8},
					{Step: 96, Pitch: -17, Dur: 3.8, Vol: 0.8},
					// Diatonic approach into F#7 (labeled idiomatic in dossier).
					{Step: 112, Pitch: -15, Dur: 3.5, Vol: 0.8}, {Step: 127, Pitch: -17, Dur: 0.2, Vol: 0.6},
				},
				verseBass,
				[]Hit{
					{Step: 256, Pitch: -10, Dur: 3.8, Vol: 0.85}, {Step: 272, Pitch: -15, Dur: 3.8, Vol: 0.85},
					{Step: 288, Pitch: -12, Dur: 3.8, Vol: 0.85}, {Step: 304, Pitch: -17, Dur: 3.8, Vol: 0.85},
				},
			)},
			// Verse vocal (Henley substitute), phrases enter an 8th behind the
			// downbeat; laid back a touch more.
			{Inst: "sax", Hits: laidBack(at(128, []Hit{
		{Step: 2, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 3, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 4, Pitch: 9, Dur: 0.50, Vol: 0.75}, {Step: 6, Pitch: 7, Dur: 0.24, Vol: 0.75},
		{Step: 7, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 8, Pitch: 7, Dur: 0.50, Vol: 0.75}, {Step: 10, Pitch: 9, Dur: 1.42, Vol: 0.75}, {Step: 18, Pitch: 9, Dur: 0.49, Vol: 0.75},
		{Step: 20, Pitch: 9, Dur: 0.33, Vol: 0.75}, {Step: 21, Pitch: 7, Dur: 0.32, Vol: 0.75}, {Step: 23, Pitch: 7, Dur: 0.32, Vol: 0.75}, {Step: 24, Pitch: 7, Dur: 2.00, Vol: 0.75},
		{Step: 34, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 35, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 36, Pitch: 9, Dur: 0.50, Vol: 0.75}, {Step: 38, Pitch: 7, Dur: 0.24, Vol: 0.75},
		{Step: 39, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 40, Pitch: 7, Dur: 0.50, Vol: 0.75}, {Step: 42, Pitch: 9, Dur: 1.50, Vol: 0.75}, {Step: 50, Pitch: 9, Dur: 0.24, Vol: 0.75},
		{Step: 51, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 52, Pitch: 9, Dur: 0.32, Vol: 0.75}, {Step: 53, Pitch: 7, Dur: 0.32, Vol: 0.75}, {Step: 55, Pitch: 7, Dur: 0.32, Vol: 0.75},
		{Step: 56, Pitch: 7, Dur: 0.25, Vol: 0.75}, {Step: 57, Pitch: 5, Dur: 0.25, Vol: 0.75}, {Step: 58, Pitch: 2, Dur: 1.49, Vol: 0.75}, {Step: 66, Pitch: 9, Dur: 0.24, Vol: 0.75},
		{Step: 67, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 68, Pitch: 9, Dur: 0.50, Vol: 0.75}, {Step: 70, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 71, Pitch: 5, Dur: 0.24, Vol: 0.75},
		{Step: 72, Pitch: 5, Dur: 0.50, Vol: 0.75}, {Step: 74, Pitch: 9, Dur: 1.50, Vol: 0.75}, {Step: 81, Pitch: 0, Dur: 0.24, Vol: 0.75}, {Step: 82, Pitch: 9, Dur: 0.24, Vol: 0.75},
		{Step: 83, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 84, Pitch: 9, Dur: 0.32, Vol: 0.75}, {Step: 85, Pitch: 7, Dur: 0.32, Vol: 0.75}, {Step: 87, Pitch: 5, Dur: 0.32, Vol: 0.75},
		{Step: 88, Pitch: 5, Dur: 2.00, Vol: 0.75}, {Step: 97, Pitch: 2, Dur: 0.24, Vol: 0.75}, {Step: 98, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 99, Pitch: 7, Dur: 0.24, Vol: 0.75},
		{Step: 100, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 101, Pitch: 5, Dur: 0.24, Vol: 0.75}, {Step: 102, Pitch: 7, Dur: 0.24, Vol: 0.75}, {Step: 103, Pitch: 5, Dur: 0.24, Vol: 0.75},
		{Step: 104, Pitch: 7, Dur: 0.50, Vol: 0.75}, {Step: 106, Pitch: 5, Dur: 0.24, Vol: 0.75}, {Step: 107, Pitch: 9, Dur: 1.25, Vol: 0.75}, {Step: 113, Pitch: 2, Dur: 0.24, Vol: 0.75},
		{Step: 114, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 115, Pitch: 9, Dur: 0.24, Vol: 0.75}, {Step: 116, Pitch: 9, Dur: 0.33, Vol: 0.75}, {Step: 117, Pitch: 7, Dur: 0.32, Vol: 0.75},
		{Step: 119, Pitch: 7, Dur: 0.32, Vol: 0.75}, {Step: 120, Pitch: 9, Dur: 2.00, Vol: 0.75},			}), 0.12)},
			// Octave-pair fills (true dual-guitar doubling), intro cycle.
			{Inst: "guitar-electric-neck", Hits: []Hit{
		{Step: 0, Pitch: 14, Dur: 3.00, Vol: 0.75}, {Step: 12, Pitch: 17, Dur: 0.50, Vol: 0.75}, {Step: 14, Pitch: 19, Dur: 0.50, Vol: 0.75}, {Step: 16, Pitch: 13, Dur: 3.50, Vol: 0.75},
		{Step: 32, Pitch: 12, Dur: 2.00, Vol: 0.75}, {Step: 40, Pitch: 12, Dur: 1.00, Vol: 0.65}, {Step: 44, Pitch: 7, Dur: 1.00, Vol: 0.73}, {Step: 48, Pitch: 11, Dur: 4.00, Vol: 0.75},
		{Step: 64, Pitch: 10, Dur: 2.50, Vol: 0.73}, {Step: 74, Pitch: 10, Dur: 0.25, Vol: 0.56}, {Step: 75, Pitch: 10, Dur: 0.24, Vol: 0.53}, {Step: 76, Pitch: 12, Dur: 0.17, Vol: 0.75},
		{Step: 77, Pitch: 14, Dur: 0.33, Vol: 0.75}, {Step: 78, Pitch: 17, Dur: 0.50, Vol: 0.75}, {Step: 80, Pitch: 5, Dur: 2.00, Vol: 0.75}, {Step: 88, Pitch: 9, Dur: 1.00, Vol: 0.75},
		{Step: 92, Pitch: 5, Dur: 1.00, Vol: 0.75}, {Step: 96, Pitch: 7, Dur: 3.00, Vol: 0.75}, {Step: 108, Pitch: 10, Dur: 1.00, Vol: 0.65}, {Step: 112, Pitch: 9, Dur: 4.00, Vol: 0.75},			}},
			{Inst: "guitar-electric-neck", Hits: transposeHits([]Hit{
		{Step: 0, Pitch: 14, Dur: 3.00, Vol: 0.75}, {Step: 12, Pitch: 17, Dur: 0.50, Vol: 0.75}, {Step: 14, Pitch: 19, Dur: 0.50, Vol: 0.75}, {Step: 16, Pitch: 13, Dur: 3.50, Vol: 0.75},
		{Step: 32, Pitch: 12, Dur: 2.00, Vol: 0.75}, {Step: 40, Pitch: 12, Dur: 1.00, Vol: 0.65}, {Step: 44, Pitch: 7, Dur: 1.00, Vol: 0.73}, {Step: 48, Pitch: 11, Dur: 4.00, Vol: 0.75},
		{Step: 64, Pitch: 10, Dur: 2.50, Vol: 0.73}, {Step: 74, Pitch: 10, Dur: 0.25, Vol: 0.56}, {Step: 75, Pitch: 10, Dur: 0.24, Vol: 0.53}, {Step: 76, Pitch: 12, Dur: 0.17, Vol: 0.75},
		{Step: 77, Pitch: 14, Dur: 0.33, Vol: 0.75}, {Step: 78, Pitch: 17, Dur: 0.50, Vol: 0.75}, {Step: 80, Pitch: 5, Dur: 2.00, Vol: 0.75}, {Step: 88, Pitch: 9, Dur: 1.00, Vol: 0.75},
		{Step: 92, Pitch: 5, Dur: 1.00, Vol: 0.75}, {Step: 96, Pitch: 7, Dur: 3.00, Vol: 0.75}, {Step: 108, Pitch: 10, Dur: 1.00, Vol: 0.65}, {Step: 112, Pitch: 9, Dur: 4.00, Vol: 0.75},			}, -12)},
			// The cascade: top voice (Felder) / bottom voice (Walsh).
			{Inst: "guitar-electric", Hits: concat(
				cascadeBar(256, [2]float64{14, 21}, [2]float64{9, 17}, [2]float64{5, 14}, true),
				cascadeBar(272, [2]float64{13, 19}, [2]float64{9, 16}, [2]float64{4, 13}, true),
				cascadeBar(288, [2]float64{12, 19}, [2]float64{7, 16}, [2]float64{4, 12}, true),
				cascadeBar(304, [2]float64{11, 17}, [2]float64{7, 14}, [2]float64{2, 11}, true),
			)},
			{Inst: "guitar-electric", Hits: concat(
				cascadeBar(256, [2]float64{14, 21}, [2]float64{9, 17}, [2]float64{5, 14}, false),
				cascadeBar(272, [2]float64{13, 19}, [2]float64{9, 16}, [2]float64{4, 13}, false),
				cascadeBar(288, [2]float64{12, 19}, [2]float64{7, 16}, [2]float64{4, 12}, false),
				cascadeBar(304, [2]float64{11, 17}, [2]float64{7, 14}, [2]float64{2, 11}, false),
			)},
			// Kit: silent intro (pickup fill in bar 8), verse one-drop lope,
			// cascade opens to full backbeat.
			{Inst: "kick-acoustic", Hits: concat(
				[]Hit{{Step: 124, Vol: 0.7}, {Step: 126, Vol: 0.75}},
				at(128, ostinato(8, []Hit{{Step: 0, Vol: 0.85}, {Step: 8, Vol: 0.85}, {Step: 14, Vol: 0.65}})),
				at(256, ostinato(4, []Hit{{Step: 0, Vol: 0.9}, {Step: 8, Vol: 0.88}, {Step: 14, Vol: 0.7}})),
			)},
			{Inst: "sidestick", Hits: at(128, ostinato(8, []Hit{{Step: 4, Vol: 0.72}, {Step: 12, Vol: 0.72}}))},
			// Snare: fill into the cascade, then chorus backbeat.
			{Inst: "snare", Hits: concat(
				[]Hit{{Step: 250, Vol: 0.5}, {Step: 252, Vol: 0.6}, {Step: 253, Vol: 0.65}, {Step: 254, Vol: 0.7}, {Step: 255, Vol: 0.75}},
				at(256, ostinato(4, []Hit{{Step: 4, Vol: 0.78}, {Step: 12, Vol: 0.78}})),
			)},
			{Inst: "hihat", Hits: at(128, hatEighths(12, 0.5))},
			// Cabasa pairs on the BACK half of every beat — the reggae lean.
			{Inst: "shaker", Hits: at(128, ostinato(12, []Hit{
				{Step: 2, Vol: 0.38}, {Step: 3, Vol: 0.3}, {Step: 6, Vol: 0.38}, {Step: 7, Vol: 0.3},
				{Step: 10, Vol: 0.38}, {Step: 11, Vol: 0.3}, {Step: 14, Vol: 0.38}, {Step: 15, Vol: 0.3},
			}))},
			{Inst: "crash", Hits: []Hit{{Step: 128, Vol: 0.5}, {Step: 256, Vol: 0.55}}},
			// Tom pickup with the bar-8 kick fill.
			{Inst: "tom", Hits: []Hit{{Step: 124, Pitch: 0, Dur: 0.3, Vol: 0.85}, {Step: 126, Pitch: -3, Dur: 0.3, Vol: 0.9}}},
		},
	}
}
