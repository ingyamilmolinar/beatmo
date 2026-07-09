package main

// Miles Davis — "So What" (Kind of Blue, 1959). D dorian / Eb dorian bridge,
// 136 BPM (record ≈137 per songbpm; the source MIDI arrangement runs 108).
// Note data from BitMidi 70824; form (32-bar AABA: 16 D / 8 Eb / 8 D) from the
// saxteacheruk lead-sheet page; "So What chord" theory from Wikipedia; Cobb
// crash + Chambers lay-back prose from thebluemoment/pas.org. Dossier:
// miles-so-what_dossier.md.
//
// 24 bars: 16-bar head (bass call → "So What" answer chords; Eb bridge bars
// 9–12) + 8 bars of Miles' opening solo cell over Dm11 pads, entered by Jimmy
// Cobb's famous crash.
//
// GROOVE (measured): ride is a 5:1 "ding-di-DING" — beat, straight "&", and a
// pickup at 82.8% of the beat (encoded: step 3 of each beat + delay 0.31).
// The bass's grace echo sits at the triplet position (delay 0.33) and the whole
// bass lays back ~10ms (Chambers vs Cobb).
func soWhatShowcase() Showcase {
	// ---- Head: bass call (melody register; +12 octave policy) ----
	d3 := func(b int, withA2 bool) []Hit {
		out := []Hit{{Step: b, Pitch: 5, Dur: 0.26, Vol: 0.82}}
		if withA2 {
			out = append(out, Hit{Step: b + 1, Pitch: 0, Dur: 0.25, Vol: 0.75})
		}
		return out
	}
	varA := func(b, tr int) []Hit { // pickup run G-A-B / D-E-C
		t := float64(tr)
		return []Hit{
			{Step: b + 9, Pitch: -2 + t, Dur: 0.1, Vol: 0.78}, {Step: b + 10, Pitch: 0 + t, Dur: 0.25, Vol: 0.8},
			{Step: b + 11, Pitch: 2 + t, Dur: 0.37, Vol: 0.8}, {Step: b + 13, Pitch: 5 + t, Dur: 0.1, Vol: 0.8},
			{Step: b + 14, Pitch: 7 + t, Dur: 0.23, Vol: 0.8}, {Step: b + 15, Pitch: 3 + t, Dur: 0.1, Vol: 0.78},
		}
	}
	varB := func(b, tr int) []Hit { // adds the C passing tone
		t := float64(tr)
		return append(varA(b, tr)[:3], []Hit{
			{Step: b + 12, Pitch: 3 + t, Dur: 0.27, Vol: 0.66},
			{Step: b + 13, Pitch: 5 + t, Dur: 0.11, Vol: 0.8}, {Step: b + 14, Pitch: 7 + t, Dur: 0.3, Vol: 0.8},
			{Step: b + 15, Pitch: 3 + t, Dur: 0.1, Vol: 0.78},
		}...)
	}
	varC := func(b, tr int) []Hit { // phrase-end D# neighbor tail
		t := float64(tr)
		return []Hit{
			{Step: b + 9, Pitch: 6 + t, Dur: 0.63, Vol: 0.8}, {Step: b + 12, Pitch: 6 + t, Dur: 0.41, Vol: 0.8},
			{Step: b + 14, Pitch: 6 + t, Dur: 0.45, Vol: 0.8},
		}
	}
	call := concat(
		d3(0, false), varB(0, 0), // H1
		d3(16, true), varA(16, 0), // H2
		d3(32, false), varC(32, 0), // H3
		d3(48, true), varA(48, 0), // H4
		d3(64, false), varB(64, 0), // H5
		d3(80, true), varA(80, 0), // H6
		d3(96, false), varC(96, 0), // H7
		d3(112, true), varA(112, 1), // H8 → Eb
		[]Hit{{Step: 128, Pitch: 6, Dur: 0.26, Vol: 0.82}}, varB(128, 1), // H9
		[]Hit{{Step: 144, Pitch: 6, Dur: 0.26, Vol: 0.82}, {Step: 145, Pitch: 1, Dur: 0.25, Vol: 0.75}}, varA(144, 1), // H10
		[]Hit{{Step: 160, Pitch: 6, Dur: 0.26, Vol: 0.82}}, varC(160, 1), // H11
		[]Hit{{Step: 176, Pitch: 6, Dur: 0.26, Vol: 0.82}, {Step: 177, Pitch: 1, Dur: 0.25, Vol: 0.75}}, varA(176, 0), // H12 → D
		d3(192, false), varB(192, 0), // H13
		d3(208, true), varA(208, 0), // H14
		d3(224, false), varC(224, 0), // H15
		d3(240, true), // H16 (walkup lives on the 2-feel row)
	)
	// ---- 2-feel upright bass (enters H5; +12 policy: D2 → -7) ----
	feelBar := func(b int, root float64, tail []Hit) []Hit {
		out := []Hit{
			{Step: b, Pitch: root, Dur: 0.6, Vol: 0.8},
			{Step: b + 1, Pitch: root, Dur: 0.2, Vol: 0.6, Groove: "delay", GroovePct: 0.33},
			{Step: b + 6, Pitch: root, Dur: 0.3, Vol: 0.72}, {Step: b + 7, Pitch: root, Dur: 0.3, Vol: 0.68},
		}
		return append(out, at(b, tail)...)
	}
	t1 := []Hit{{Step: 11, Pitch: -10, Dur: 0.43, Vol: 0.75}, {Step: 13, Pitch: -9, Dur: 0.31, Vol: 0.75}, {Step: 15, Pitch: -8, Dur: 0.11, Vol: 0.72}}
	t2 := []Hit{{Step: 11, Pitch: -3, Dur: 0.2, Vol: 0.75}, {Step: 12, Pitch: -2, Dur: 0.2, Vol: 0.72}, {Step: 13, Pitch: -5, Dur: 0.2, Vol: 0.72}, {Step: 14, Pitch: -4, Dur: 0.2, Vol: 0.72}, {Step: 15, Pitch: -8, Dur: 0.15, Vol: 0.72}}
	t3 := []Hit{{Step: 11, Pitch: -12, Dur: 0.43, Vol: 0.75}, {Step: 13, Pitch: -9, Dur: 0.51, Vol: 0.75}, {Step: 15, Pitch: -8, Dur: 0.11, Vol: 0.72}}
	t4 := []Hit{{Step: 11, Pitch: -10, Dur: 0.2, Vol: 0.75}, {Step: 12, Pitch: -12, Dur: 0.2, Vol: 0.72}, {Step: 13, Pitch: -10, Dur: 0.2, Vol: 0.72}, {Step: 14, Pitch: -8, Dur: 0.2, Vol: 0.75}, {Step: 15, Pitch: -7, Dur: 0.15, Vol: 0.78}}
	twoFeel := concat(
		feelBar(64, -7, t1), feelBar(80, -7, t2), feelBar(96, -7, t3), feelBar(112, -7, t4),
		feelBar(128, -6, transposeHits(t1, 1)), feelBar(144, -6, transposeHits(t2, 1)),
		feelBar(160, -6, transposeHits(t3, 1)), feelBar(176, -6, transposeHits(t4, 1)),
		feelBar(192, -7, t1), feelBar(208, -7, t2), feelBar(224, -7, t3),
		// H16: pedal + the F1-G1-A1 walkup into the crash.
		[]Hit{
			{Step: 240, Pitch: -7, Dur: 0.68, Vol: 0.8}, {Step: 243, Pitch: -7, Dur: 0.3, Vol: 0.72},
			{Step: 244, Pitch: -16, Dur: 0.78, Vol: 0.8}, {Step: 248, Pitch: -14, Dur: 0.75, Vol: 0.82},
			{Step: 252, Pitch: -12, Dur: 0.97, Vol: 0.85},
		},
		// Solo bars: same 2-feel with cycling tails.
		feelBar(256, -7, t3), feelBar(272, -7, t2), feelBar(288, -7, t1), feelBar(304, -7, t4),
		feelBar(320, -7, t3), feelBar(336, -7, t2), feelBar(352, -7, t1),
		[]Hit{{Step: 368, Pitch: -7, Dur: 0.6, Vol: 0.8}, {Step: 374, Pitch: -7, Dur: 0.3, Vol: 0.72}, {Step: 379, Pitch: -12, Dur: 0.3, Vol: 0.75}, {Step: 381, Pitch: -9, Dur: 0.3, Vol: 0.75}, {Step: 383, Pitch: -8, Dur: 0.2, Vol: 0.72}},
	)
	// ---- "So What" answer chords ----
	// Horn dyad (all head bars): G3+F4 on beat 2 (long) + swung "&" (short).
	hornBar := func(b int, tr float64) []Hit {
		return concat(
			chordAt(b+4, 0.78, 0.72, -2+tr, 8+tr),
			chordAt(b+7, 0.25, 0.68, -2+tr, 8+tr),
		)
	}
	var horns []Hit
	for i := 0; i < 16; i++ {
		tr := 0.0
		if i >= 8 && i < 12 {
			tr = 1
		}
		horns = append(horns, hornBar(i*16, tr)...)
	}
	// Piano voicings join at H9 (top-3 of the MIDI's 5-note stacks).
	pianoBar := func(b int, tr float64) []Hit {
		return concat(
			chordAt(b+4, 0.78, 0.7, 5+tr, 7+tr, 10+tr),
			chordAt(b+7, 0.25, 0.66, 3+tr, 5+tr, 8+tr),
		)
	}
	var pianoAnswers []Hit
	for i := 8; i < 16; i++ {
		tr := 0.0
		if i < 12 {
			tr = 1
		}
		pianoAnswers = append(pianoAnswers, pianoBar(i*16, tr)...)
	}
	// ---- Solo section (steps 256–383) ----
	soloCellA := []Hit{
		{Step: 2, Pitch: 8, Dur: 0.28, Vol: 0.8}, {Step: 3, Pitch: 10, Dur: 0.09, Vol: 0.68},
		{Step: 4, Pitch: 6, Dur: 0.59, Vol: 0.75}, {Step: 7, Pitch: 3, Dur: 0.68, Vol: 0.84},
		{Step: 10, Pitch: 5, Dur: 0.14, Vol: 0.84}, {Step: 12, Pitch: 5, Dur: 0.68, Vol: 0.84},
		{Step: 21, Pitch: -2, Dur: 0.16, Vol: 0.84}, {Step: 22, Pitch: 0, Dur: 0.24, Vol: 0.7},
		{Step: 23, Pitch: 3, Dur: 0.64, Vol: 0.84}, {Step: 26, Pitch: 5, Dur: 0.15, Vol: 0.84},
		{Step: 28, Pitch: 5, Dur: 0.4, Vol: 0.84}, {Step: 31, Pitch: 9, Dur: 0.67, Vol: 0.66},
	}
	soloCellB := concat(soloCellA[:6], []Hit{
		{Step: 19, Pitch: 0, Dur: 0.2, Vol: 0.8}, {Step: 20, Pitch: 3, Dur: 0.2, Vol: 0.8},
		{Step: 22, Pitch: 6, Dur: 0.43, Vol: 0.8}, {Step: 24, Pitch: 5, Dur: 0.21, Vol: 0.8},
		{Step: 31, Pitch: 9, Dur: 0.67, Vol: 0.66},
	})
	// Piano comping 4-bar cycle behind the solo.
	compCycle := concat(
		chordAt(0, 4.0, 0.6, -4, 0, 3, 7),
		[]Hit{{Step: 22, Pitch: -19, Dur: 0.2, Vol: 0.5}},
		chordAt(23, 0.25, 0.62, 1, 2, 5),
		chordAt(28, 0.63, 0.66, 1, 2, 5),
		chordAt(31, 3.3, 0.62, -4, 0, 3),
		chordAt(48, 0.6, 0.62, -7, -4, 0),
		chordAt(51, 0.57, 0.64, -4, 0, 3),
		chordAt(54, 0.5, 0.62, 2, 3),
		chordAt(56, 0.17, 0.68, -2, 2, 5),
	)
	vibeCycle := concat(
		chordAt(0, 4.0, 0.45, 7, 12, 15, 19),
		chordAt(24, 0.6, 0.45, 10, 14, 17),
		chordAt(28, 0.6, 0.45, 10, 14, 17),
		chordAt(32, 5.5, 0.45, 7, 12, 15),
	)
	// ---- Drums ----
	// Ride: beat / straight "&" / late pickup at 82.8% (delay 0.31).
	rideBar := []Hit{
		{Step: 0, Vol: 0.94}, {Step: 2, Vol: 1.0}, {Step: 3, Vol: 0.68, Groove: "delay", GroovePct: 0.31},
		{Step: 4, Vol: 0.84}, {Step: 6, Vol: 1.0}, {Step: 7, Vol: 0.68, Groove: "delay", GroovePct: 0.31},
		{Step: 8, Vol: 0.82}, {Step: 10, Vol: 1.0}, {Step: 11, Vol: 0.65, Groove: "delay", GroovePct: 0.31},
		{Step: 12, Vol: 0.86}, {Step: 14, Vol: 1.0}, {Step: 15, Vol: 0.67, Groove: "delay", GroovePct: 0.31},
	}
	return Showcase{
		Stem: "miles-so-what", BPM: 136, Subdiv: 16, Bars: 24,
		Insts: []InstSpec{
			{ID: "bass-guitar", Name: "Bass Call", Volume: 0.88},
			{ID: "bass-guitar", Name: "Bass 2-Feel", Volume: 0.72},
			{ID: "piano-grand", Name: "Piano", Volume: 0.6, Pan: -0.2, ReverbSend: 0.14},
			{ID: "sax", Name: "Horns", Volume: 0.55, Pan: 0.2, ReverbSend: 0.16},
			{ID: "trumpet-mellow", Name: "Miles", Volume: 0.72, ReverbSend: 0.18},
			{ID: "piano-felt", Name: "Comp", Volume: 0.5, Pan: -0.15, ReverbSend: 0.16},
			{ID: "viola-pad", Name: "Vibes", Volume: 0.38, Pan: 0.25, ReverbSend: 0.24},
			{ID: "ride", Name: "Ride", Volume: 0.55, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
			{ID: "hihat", Name: "Hihat", Volume: 0.45},
			{ID: "hihat-pedal", Name: "Foot Hat", Volume: 0.4},
			{ID: "sidestick", Name: "Stick", Volume: 0.5},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.7},
			{ID: "snare-1", Name: "Snare", Volume: 0.6},
			{ID: "snare-ghost", Name: "Ghost", Volume: 0.4},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "crash", Name: "Crash", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Chambers lays back ~10ms behind Cobb (prose-sourced).
			{Inst: "bass-guitar", Hits: laidBack(transposeHits(call, 12), 0.1)},
			{Inst: "bass-guitar", Hits: laidBack(transposeHits(twoFeel, 12), 0.1)},
			{Inst: "piano-grand", Hits: pianoAnswers},
			{Inst: "sax", Hits: horns},
			// Miles: pickup into the crash, then the 2-bar cell ×4.
			{Inst: "trumpet-mellow", Hits: concat(
				[]Hit{{Step: 255, Pitch: 9, Dur: 0.67, Vol: 0.66}},
				at(256, soloCellA), at(288, soloCellB), at(320, soloCellA), at(352, soloCellB),
			)},
			{Inst: "piano-felt", Hits: concat(at(256, compCycle), at(320, compCycle))},
			{Inst: "viola-pad", Hits: concat(at(256, vibeCycle), at(320, vibeCycle))},
			{Inst: "ride", Hits: ostinato(24, rideBar)},
			{Inst: "hihat", Hits: ostinato(24, []Hit{{Step: 2, Vol: 0.8}, {Step: 6, Vol: 0.8}, {Step: 10, Vol: 0.78}, {Step: 14, Vol: 0.82}})},
			// Foot hat on 2&4 (idiomatic per dossier; the MIDI omits it).
			{Inst: "hihat-pedal", Hits: ostinato(24, []Hit{{Step: 4, Vol: 0.45}, {Step: 12, Vol: 0.45}})},
			{Inst: "sidestick", Hits: ostinato(24, []Hit{{Step: 6, Vol: 0.78}, {Step: 14, Vol: 0.8}})},
			// Kick/backbeat/ghosts join for the solo (plus the bar-16 fill-in).
			{Inst: "kick-acoustic", Hits: concat(
				[]Hit{{Step: 240, Vol: 0.7}, {Step: 244, Vol: 0.7}, {Step: 248, Vol: 0.7}, {Step: 252, Vol: 0.72}},
				at(256, ostinato(8, []Hit{{Step: 0, Vol: 0.84}, {Step: 7, Vol: 0.6}, {Step: 8, Vol: 0.84}, {Step: 15, Vol: 0.47}})),
			)},
			{Inst: "snare-1", Hits: concat(
				[]Hit{{Step: 244, Vol: 0.6}, {Step: 250, Vol: 0.62}, {Step: 252, Vol: 0.66}, {Step: 254, Vol: 0.7}},
				at(256, ostinato(8, []Hit{{Step: 4, Vol: 0.8}, {Step: 7, Vol: 0.55}, {Step: 10, Vol: 0.52}, {Step: 12, Vol: 0.82}})),
			)},
			{Inst: "snare-ghost", Hits: concat(
				[]Hit{{Step: 249, Vol: 0.14}},
				at(256, ostinato(8, []Hit{{Step: 9, Vol: 0.14}})),
			)},
			{Inst: "hihat-1", Hits: concat([]Hit{{Step: 250, Vol: 0.7}}, at(256, ostinato(8, []Hit{{Step: 10, Vol: 0.7}})))},
			// Ride-wash crashes from H9; THE crash at the solo entry.
			{Inst: "crash", Hits: []Hit{
				{Step: 128, Vol: 0.32}, {Step: 144, Vol: 0.3}, {Step: 160, Vol: 0.3}, {Step: 176, Vol: 0.3},
				{Step: 192, Vol: 0.3}, {Step: 208, Vol: 0.3}, {Step: 224, Vol: 0.3}, {Step: 240, Vol: 0.3},
				{Step: 256, Vol: 0.66},
				{Step: 288, Vol: 0.32}, {Step: 320, Vol: 0.32}, {Step: 352, Vol: 0.32},
			}},
		},
	}
}
