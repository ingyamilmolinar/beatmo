package main

// Jobim — "The Girl from Ipanema" (Getz/Gilberto, 1963). F major, 130 BPM.
// Melody verbatim from the Ulf Bro ABC lead sheet (trillian.mit.edu, A + the
// full 16-bar bridge + A'); comp voicings + bass walk-ups from BitMidi 102483
// (solo-piano arrangement); João Gilberto comp rhythm + surdo/clave/cabasa
// patterns from Wikipedia bossa-nova + drum-pedagogy sources. Dossier:
// jobim-ipanema_dossier.md.
//
// 32 bars = the FULL head: A (8, Astrud-voice substitute) → B bridge (16 —
// the Gbmaj7/B7/F#m7/D7/Gm7/Eb7 centerpiece with its quarter-note-triplet
// runs) → A' (8, handed to the Getz tenor for the out-head).
//
// GROOVE: straight 16ths — bossa syncopation is pattern, not swing. The comp
// anticipates barlines on the &-of-4 (written in); the voice lays back a hair.
func ipanemaShowcase() Showcase {
	melodyA := []Hit{
		{Step: 0, Pitch: 10, Dur: 1.0, Vol: 0.71}, {Step: 4, Pitch: 10, Dur: 0.5, Vol: 0.65},
		{Step: 6, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 8, Pitch: 7, Dur: 1.0, Vol: 0.66},
		{Step: 12, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 14, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 16, Pitch: 10, Dur: 1.0, Vol: 0.69}, {Step: 20, Pitch: 10, Dur: 0.5, Vol: 0.65},
		{Step: 22, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 24, Pitch: 7, Dur: 0.5, Vol: 0.63},
		{Step: 26, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 28, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 30, Pitch: 10, Dur: 1.5, Vol: 0.68},
		{Step: 36, Pitch: 10, Dur: 0.5, Vol: 0.65}, {Step: 38, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 40, Pitch: 7, Dur: 0.5, Vol: 0.63}, {Step: 42, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 44, Pitch: 5, Dur: 0.5, Vol: 0.6}, {Step: 46, Pitch: 10, Dur: 1.5, Vol: 0.68},
		{Step: 52, Pitch: 10, Dur: 0.5, Vol: 0.65}, {Step: 54, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 56, Pitch: 7, Dur: 0.5, Vol: 0.63}, {Step: 58, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 60, Pitch: 5, Dur: 0.5, Vol: 0.6}, {Step: 62, Pitch: 8, Dur: 1.5, Vol: 0.66},
		{Step: 68, Pitch: 8, Dur: 0.5, Vol: 0.63}, {Step: 70, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 72, Pitch: 5, Dur: 0.5, Vol: 0.61}, {Step: 74, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 76, Pitch: 3, Dur: 0.5, Vol: 0.58}, {Step: 78, Pitch: 7, Dur: 1.5, Vol: 0.65},
		{Step: 84, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 86, Pitch: 3, Dur: 0.5, Vol: 0.58},
		{Step: 88, Pitch: 3, Dur: 0.5, Vol: 0.6}, {Step: 90, Pitch: 3, Dur: 0.5, Vol: 0.58},
		{Step: 92, Pitch: 1, Dur: 0.5, Vol: 0.57}, {Step: 94, Pitch: 3, Dur: 4.5, Vol: 0.61},
	}
	melodyB := []Hit{
		{Step: 0, Pitch: 8, Dur: 4.67, Vol: 0.69}, {Step: 19, Pitch: 9, Dur: 0.67, Vol: 0.65},
		{Step: 21, Pitch: 8, Dur: 0.67, Vol: 0.61}, {Step: 24, Pitch: 6, Dur: 0.67, Vol: 0.63},
		{Step: 27, Pitch: 8, Dur: 0.67, Vol: 0.61}, {Step: 29, Pitch: 6, Dur: 0.67, Vol: 0.6},
		{Step: 32, Pitch: 4, Dur: 1.5, Vol: 0.66}, {Step: 38, Pitch: 6, Dur: 5.5, Vol: 0.65},
		{Step: 62, Pitch: 11, Dur: 5.17, Vol: 0.69}, {Step: 83, Pitch: 12, Dur: 0.67, Vol: 0.66},
		{Step: 85, Pitch: 11, Dur: 0.67, Vol: 0.63}, {Step: 88, Pitch: 9, Dur: 0.67, Vol: 0.65},
		{Step: 91, Pitch: 11, Dur: 0.67, Vol: 0.63}, {Step: 93, Pitch: 9, Dur: 0.67, Vol: 0.61},
		{Step: 96, Pitch: 7, Dur: 1.5, Vol: 0.65}, {Step: 102, Pitch: 9, Dur: 5.5, Vol: 0.66},
		{Step: 126, Pitch: 12, Dur: 5.17, Vol: 0.69}, {Step: 147, Pitch: 14, Dur: 0.67, Vol: 0.68},
		{Step: 149, Pitch: 12, Dur: 0.67, Vol: 0.65}, {Step: 152, Pitch: 10, Dur: 0.67, Vol: 0.63},
		{Step: 155, Pitch: 12, Dur: 0.67, Vol: 0.65}, {Step: 157, Pitch: 10, Dur: 0.67, Vol: 0.61},
		{Step: 160, Pitch: 8, Dur: 1.5, Vol: 0.65}, {Step: 166, Pitch: 10, Dur: 4.5, Vol: 0.66},
		{Step: 187, Pitch: 12, Dur: 0.67, Vol: 0.66}, {Step: 189, Pitch: 14, Dur: 0.67, Vol: 0.68},
		{Step: 192, Pitch: 15, Dur: 0.67, Vol: 0.72}, {Step: 195, Pitch: 3, Dur: 0.67, Vol: 0.61},
		{Step: 197, Pitch: 5, Dur: 0.67, Vol: 0.63}, {Step: 200, Pitch: 7, Dur: 0.67, Vol: 0.65},
		{Step: 203, Pitch: 8, Dur: 0.67, Vol: 0.66}, {Step: 205, Pitch: 10, Dur: 0.67, Vol: 0.68},
		{Step: 208, Pitch: 11, Dur: 1.5, Vol: 0.69}, {Step: 214, Pitch: 12, Dur: 1.5, Vol: 0.68},
		{Step: 224, Pitch: 14, Dur: 0.67, Vol: 0.69}, {Step: 227, Pitch: 1, Dur: 0.67, Vol: 0.58},
		{Step: 229, Pitch: 3, Dur: 0.67, Vol: 0.61}, {Step: 232, Pitch: 5, Dur: 0.67, Vol: 0.63},
		{Step: 235, Pitch: 7, Dur: 0.67, Vol: 0.65}, {Step: 237, Pitch: 8, Dur: 0.67, Vol: 0.66},
		{Step: 240, Pitch: 9, Dur: 1.5, Vol: 0.68}, {Step: 246, Pitch: 10, Dur: 1.5, Vol: 0.66},
	}
	melodyA2 := []Hit{
		{Step: 0, Pitch: 10, Dur: 1.5, Vol: 0.71}, {Step: 6, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 8, Pitch: 7, Dur: 0.5, Vol: 0.63}, {Step: 10, Pitch: 7, Dur: 0.5, Vol: 0.61},
		{Step: 12, Pitch: 5, Dur: 0.5, Vol: 0.6}, {Step: 14, Pitch: 10, Dur: 1.5, Vol: 0.68},
		{Step: 20, Pitch: 10, Dur: 0.5, Vol: 0.65}, {Step: 22, Pitch: 7, Dur: 1.0, Vol: 0.61},
		{Step: 26, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 28, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 30, Pitch: 10, Dur: 1.5, Vol: 0.68}, {Step: 36, Pitch: 10, Dur: 0.5, Vol: 0.65},
		{Step: 38, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 40, Pitch: 7, Dur: 0.5, Vol: 0.63},
		{Step: 42, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 44, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 46, Pitch: 10, Dur: 1.5, Vol: 0.68}, {Step: 52, Pitch: 10, Dur: 0.5, Vol: 0.65},
		{Step: 54, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 56, Pitch: 7, Dur: 0.5, Vol: 0.63},
		{Step: 58, Pitch: 7, Dur: 0.5, Vol: 0.61}, {Step: 60, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 62, Pitch: 12, Dur: 1.5, Vol: 0.7}, {Step: 68, Pitch: 12, Dur: 0.5, Vol: 0.66},
		{Step: 70, Pitch: 8, Dur: 0.5, Vol: 0.63}, {Step: 72, Pitch: 8, Dur: 0.5, Vol: 0.65},
		{Step: 74, Pitch: 8, Dur: 0.5, Vol: 0.63}, {Step: 76, Pitch: 5, Dur: 0.5, Vol: 0.6},
		{Step: 78, Pitch: 15, Dur: 1.5, Vol: 0.72}, {Step: 84, Pitch: 15, Dur: 0.5, Vol: 0.69},
		{Step: 86, Pitch: 7, Dur: 0.5, Vol: 0.63}, {Step: 88, Pitch: 7, Dur: 0.67, Vol: 0.63},
		{Step: 91, Pitch: 7, Dur: 0.67, Vol: 0.61}, {Step: 93, Pitch: 5, Dur: 0.67, Vol: 0.6},
		{Step: 96, Pitch: 7, Dur: 5.0, Vol: 0.65},
	}
	// João's comp: 2-bar pattern (chords 0,4,10 + &-of-4 anticipation | 6,10 +
	// anticipation). a = this pair's chord, next = the following pair's chord
	// (2-tone anticipations so rolls never collide across barlines).
	compPair := func(base int, a, next []float64) []Hit {
		return concat(
			chordAt(base, 0.3, 0.62, a...),
			chordAt(base+4, 0.2, 0.55, a...),
			chordAt(base+10, 0.2, 0.55, a...),
			chordAt(base+14, 0.5, 0.65, a[len(a)-2], a[len(a)-1]),
			chordAt(base+22, 0.2, 0.55, a...),
			chordAt(base+26, 0.2, 0.55, a...),
			chordAt(base+30, 0.5, 0.65, next[len(next)-2], next[len(next)-1]),
		)
	}
	fmaj := []float64{3, 7, 10}
	g7 := []float64{0, 2, 7}
	gm7 := []float64{1, 5, 8}
	gb7 := []float64{1, 4, 8}
	gbmaj := []float64{1, 4, 8}
	b7 := []float64{0, 4, 6}
	fsm7 := []float64{0, 4, 11}
	d7 := []float64{3, 7, 9}
	gm9 := []float64{5, 8, 12}
	eb9 := []float64{4, 8, 10}
	am7 := []float64{0, 3, 7}
	c7 := []float64{-1, 3, 7}
	comp := concat(
		// A: F F | G7 G7 | Gm7 Gb7 | F F
		compPair(0, fmaj, g7), compPair(32, g7, gm7),
		chordAt(64, 0.3, 0.62, gm7...), chordAt(68, 0.2, 0.55, gm7...), chordAt(74, 0.2, 0.55, gm7...),
		chordAt(78, 0.5, 0.65, 4, 8),
		chordAt(86, 0.2, 0.55, gb7...), chordAt(90, 0.2, 0.55, gb7...), chordAt(94, 0.5, 0.65, 7, 10),
		compPair(96, fmaj, gbmaj),
		// B: Gb Gb | B7 B7 | F#m F#m | D7 D7 | Gm Gm | Eb7 Eb7 | Am D7 | Gm C7
		compPair(128, gbmaj, b7), compPair(160, b7, fsm7), compPair(192, fsm7, d7),
		compPair(224, d7, gm9), compPair(256, gm9, eb9), compPair(288, eb9, am7),
		chordAt(320, 0.3, 0.62, am7...), chordAt(324, 0.2, 0.55, am7...), chordAt(330, 0.2, 0.55, am7...),
		chordAt(334, 0.5, 0.65, 7, 9),
		chordAt(342, 0.2, 0.55, d7...), chordAt(346, 0.2, 0.55, d7...), chordAt(350, 0.5, 0.65, 5, 8),
		chordAt(352, 0.3, 0.62, gm9...), chordAt(356, 0.2, 0.55, gm9...), chordAt(362, 0.2, 0.55, gm9...),
		chordAt(366, 0.5, 0.65, 3, 7),
		chordAt(374, 0.2, 0.55, c7...), chordAt(378, 0.2, 0.55, c7...), chordAt(382, 0.5, 0.65, 7, 10),
		// A': F F | G7 G7 | Gm7 Gb7 | F F
		compPair(384, fmaj, g7), compPair(416, g7, gm7),
		chordAt(448, 0.3, 0.62, gm7...), chordAt(452, 0.2, 0.55, gm7...), chordAt(458, 0.2, 0.55, gm7...),
		chordAt(462, 0.5, 0.65, 4, 8),
		chordAt(470, 0.2, 0.55, gb7...), chordAt(474, 0.2, 0.55, gb7...), chordAt(478, 0.5, 0.65, 7, 10),
		compPair(480, fmaj, fmaj),
	)
	// Bass (+12 policy): root long / fifth pushes; the sourced F-bar walk-up
	// closes each F pair; bridge roots go long and sustained.
	fPair := []Hit{
		{Step: 0, Pitch: -16, Dur: 2.0, Vol: 0.7}, {Step: 8, Pitch: -12, Dur: 0.35, Vol: 0.6},
		{Step: 10, Pitch: -11, Dur: 0.3, Vol: 0.53}, {Step: 12, Pitch: -10, Dur: 0.4, Vol: 0.54},
		{Step: 14, Pitch: -9, Dur: 0.3, Vol: 0.52}, {Step: 16, Pitch: -16, Dur: 2.0, Vol: 0.66},
		{Step: 24, Pitch: -12, Dur: 0.3, Vol: 0.59}, {Step: 27, Pitch: -11, Dur: 0.3, Vol: 0.55},
		{Step: 30, Pitch: -7, Dur: 0.25, Vol: 0.57},
	}
	rootFifthBar := func(base int, r float64) []Hit {
		return []Hit{
			{Step: base, Pitch: r, Dur: 1.5, Vol: 0.68}, {Step: base + 6, Pitch: r + 7, Dur: 0.5, Vol: 0.55},
			{Step: base + 8, Pitch: r, Dur: 1.5, Vol: 0.64}, {Step: base + 14, Pitch: r + 7, Dur: 0.5, Vol: 0.55},
		}
	}
	bassAll := concat(
		fPair,
		rootFifthBar(32, -14), rootFifthBar(48, -14),
		rootFifthBar(64, -14), rootFifthBar(80, -15),
		at(96, fPair),
		// Bridge: long roots.
		rootFifthBar(128, -15), rootFifthBar(144, -15),
		rootFifthBar(160, -10), rootFifthBar(176, -10),
		rootFifthBar(192, -15), rootFifthBar(208, -15),
		rootFifthBar(224, -7), rootFifthBar(240, -7),
		rootFifthBar(256, -2), rootFifthBar(272, -2),
		rootFifthBar(288, -6), rootFifthBar(304, -6),
		rootFifthBar(320, -12), rootFifthBar(336, -7),
		rootFifthBar(352, -14), rootFifthBar(368, -9),
		// A'.
		at(384, fPair),
		rootFifthBar(416, -14), rootFifthBar(432, -14),
		rootFifthBar(448, -14), rootFifthBar(464, -15),
		at(480, fPair),
	)
	// Jobim: sparse single stabs, bridge odd bars only ("minimalist").
	pianoStabs := concat(
		chordAt(130, 0.3, 0.4, 13, 16), chordAt(162, 0.3, 0.4, 12, 16),
		chordAt(194, 0.3, 0.4, 12, 16), chordAt(226, 0.3, 0.4, 15, 19),
		chordAt(258, 0.3, 0.4, 17, 20), chordAt(290, 0.3, 0.4, 16, 20),
		chordAt(322, 0.3, 0.4, 15, 19), chordAt(354, 0.3, 0.4, 17, 20),
	)
	return Showcase{
		Stem: "jobim-ipanema", BPM: 130, Subdiv: 16, Bars: 32,
		Insts: []InstSpec{
			{ID: "voice-soprano", Name: "Voz", Volume: 0.66, ReverbSend: 0.18},
			{ID: "sax", Name: "Sax", Volume: 0.7, ReverbSend: 0.2},
			{ID: "guitar-nylon", Name: "Violão", Volume: 0.6, Pan: -0.15, ReverbSend: 0.12},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.78},
			{ID: "piano-grand", Name: "Piano", Volume: 0.42, Pan: 0.25, ReverbSend: 0.16},
			{ID: "sidestick", Name: "Rim", Volume: 0.6},
			{ID: "kick-acoustic", Name: "Surdo", Volume: 0.7},
			{ID: "hihat", Name: "Hihat", Volume: 0.42},
			{ID: "shaker", Name: "Cabasa", Volume: 0.4, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga", Name: "Pandeiro", Volume: 0.3, Pan: 0.3},
		},
		Rows: []RowSpec{
			// Astrud substitute sings A + the bridge; lays back a hair.
			{Inst: "voice-soprano", Hits: laidBack(concat(melodyA, at(128, melodyB)), 0.25)},
			// Getz takes the out-head (A').
			{Inst: "sax", Hits: laidBack(at(384, melodyA2), 0.25)},
			{Inst: "guitar-nylon", Hits: comp},
			{Inst: "bass-guitar", Hits: bassAll},
			{Inst: "piano-grand", Hits: pianoStabs},
			// Bossa clave rim (3-2), one extra rim cueing the bridge (labeled).
			{Inst: "sidestick", Hits: concat(
				ostinato2(32, []Hit{
					{Step: 0, Vol: 0.67}, {Step: 6, Vol: 0.63}, {Step: 12, Vol: 0.65},
					{Step: 20, Vol: 0.66}, {Step: 26, Vol: 0.63},
				}),
				[]Hit{{Step: 126, Vol: 0.6}},
			)},
			{Inst: "kick-acoustic", Hits: ostinato(32, []Hit{{Step: 0, Vol: 0.57}, {Step: 6, Vol: 0.47}, {Step: 8, Vol: 0.57}, {Step: 14, Vol: 0.47}})},
			{Inst: "hihat", Hits: ostinato(32, []Hit{
				{Step: 0, Vol: 0.51}, {Step: 2, Vol: 0.43}, {Step: 4, Vol: 0.43}, {Step: 6, Vol: 0.43},
				{Step: 8, Vol: 0.51}, {Step: 10, Vol: 0.43}, {Step: 12, Vol: 0.43}, {Step: 14, Vol: 0.43},
			})},
			{Inst: "shaker", Hits: ostinato(32, []Hit{
				{Step: 0, Vol: 0.55}, {Step: 1, Vol: 0.35}, {Step: 2, Vol: 0.43}, {Step: 3, Vol: 0.38},
				{Step: 4, Vol: 0.55}, {Step: 5, Vol: 0.35}, {Step: 6, Vol: 0.43}, {Step: 7, Vol: 0.38},
				{Step: 8, Vol: 0.55}, {Step: 9, Vol: 0.35}, {Step: 10, Vol: 0.43}, {Step: 11, Vol: 0.38},
				{Step: 12, Vol: 0.55}, {Step: 13, Vol: 0.35}, {Step: 14, Vol: 0.43}, {Step: 15, Vol: 0.38},
			})},
			{Inst: "conga", Hits: ostinato(32, []Hit{{Step: 2, Dur: 0.2, Vol: 0.35}, {Step: 7, Dur: 0.2, Vol: 0.3}, {Step: 10, Dur: 0.2, Vol: 0.35}, {Step: 15, Dur: 0.2, Vol: 0.3}})},
		},
	}
}
