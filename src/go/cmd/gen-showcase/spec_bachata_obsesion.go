package main

// Aventura — "Obsesión" (2002). C# minor, 134 BPM. All note data from BitMidi
// 8503 (karaoke arrangement with lyric markers pinning every section); ensemble
// patterns cross-checked against iASO Records bachata pedagogy (requinto/
// segunda/bass/bongó/güira derecho) + soundadventurer bongó guide. Chords
// verified vs lacuerda chart (the old template's C#m-G#m guess was wrong: the
// verse vamp is A↔C#m; chorus E|B|F#m|E ×2 halves ending V=G#).
// Dossier: bachata-obsesion_dossier.md.
//
// 32 bars: requinto intro loop ×2 (8) → verse 1 (8: vamp + derecho percussion
// + verse melody) → full 16-bar chorus (hook + choir pads + bell layer).
func obsesionShowcase() Showcase {
	// The signature intro arpeggio (4-bar loop: C#m | C#m/G# | G#m | G#m).
	introRiff := []Hit{
		{Step: 0, Pitch: -8, Dur: 3.4, Vol: 0.64}, {Step: 2, Pitch: 4, Dur: 1.6, Vol: 0.63},
		{Step: 4, Pitch: 7, Dur: 0.65, Vol: 0.64}, {Step: 6, Pitch: 11, Dur: 1.9, Vol: 0.62},
		{Step: 8, Pitch: 7, Dur: 0.63, Vol: 0.69}, {Step: 10, Pitch: 4, Dur: 0.61, Vol: 0.54},
		{Step: 12, Pitch: 7, Dur: 0.4, Vol: 0.68}, {Step: 14, Pitch: -8, Dur: 4.45, Vol: 0.57},
		{Step: 16, Pitch: -1, Dur: 3.9, Vol: 0.73}, {Step: 18, Pitch: 4, Dur: 1.6, Vol: 0.62},
		{Step: 20, Pitch: 7, Dur: 0.68, Vol: 0.69}, {Step: 22, Pitch: 11, Dur: 2.5, Vol: 0.69},
		{Step: 24, Pitch: 7, Dur: 0.58, Vol: 0.7}, {Step: 26, Pitch: 4, Dur: 0.67, Vol: 0.5},
		{Step: 28, Pitch: 7, Dur: 0.96, Vol: 0.79}, {Step: 30, Pitch: 4, Dur: 0.58, Vol: 0.47},
		{Step: 32, Pitch: -13, Dur: 3.6, Vol: 0.56}, {Step: 34, Pitch: 2, Dur: 1.5, Vol: 0.57},
		{Step: 36, Pitch: 6, Dur: 0.6, Vol: 0.54}, {Step: 38, Pitch: 11, Dur: 0.9, Vol: 0.57},
		{Step: 40, Pitch: 6, Dur: 0.54, Vol: 0.58}, {Step: 42, Pitch: 2, Dur: 0.73, Vol: 0.54},
		{Step: 44, Pitch: 6, Dur: 1.3, Vol: 0.52}, {Step: 46, Pitch: 2, Dur: 0.79, Vol: 0.55},
		{Step: 48, Pitch: -13, Dur: 4.0, Vol: 0.75}, {Step: 50, Pitch: 2, Dur: 1.7, Vol: 0.61},
		{Step: 52, Pitch: 6, Dur: 0.62, Vol: 0.54}, {Step: 54, Pitch: 11, Dur: 2.5, Vol: 0.57},
		{Step: 56, Pitch: 6, Dur: 0.47, Vol: 0.43}, {Step: 58, Pitch: 6, Dur: 0.65, Vol: 0.64},
		{Step: 60, Pitch: 2, Dur: 1.0, Vol: 0.54},
	}
	// Requinto verse comping + the signature descending answer fill.
	verseReq := concat(
		[]Hit{
			{Step: 0, Pitch: 0, Dur: 0.56, Vol: 0.68}, {Step: 2, Pitch: 4, Dur: 0.56, Vol: 0.58},
			{Step: 4, Pitch: 7, Dur: 0.41, Vol: 0.69},
		},
		chordAt(6, 0.9, 0.69, 4, 7, 12),
		[]Hit{
			{Step: 10, Pitch: 4, Dur: 0.46, Vol: 0.55}, {Step: 12, Pitch: 7, Dur: 0.55, Vol: 0.64},
			{Step: 14, Pitch: 4, Dur: 0.47, Vol: 0.49}, {Step: 16, Pitch: -1, Dur: 0.83, Vol: 0.5},
			{Step: 18, Pitch: 4, Dur: 0.47, Vol: 0.64}, {Step: 20, Pitch: 9, Dur: 0.2, Vol: 0.78},
			{Step: 22, Pitch: -1, Dur: 0.45, Vol: 0.72}, {Step: 24, Pitch: 9, Dur: 0.17, Vol: 0.83},
			{Step: 25, Pitch: 7, Dur: 0.17, Vol: 0.73}, {Step: 26, Pitch: 7, Dur: 0.17, Vol: 0.73},
			{Step: 27, Pitch: 9, Dur: 0.17, Vol: 0.8}, {Step: 28, Pitch: 9, Dur: 0.45, Vol: 0.8},
			{Step: 30, Pitch: 11, Dur: 0.24, Vol: 0.8}, {Step: 31, Pitch: 9, Dur: 0.2, Vol: 0.83},
			{Step: 32, Pitch: 0, Dur: 0.96, Vol: 0.69}, {Step: 34, Pitch: 4, Dur: 0.25, Vol: 0.65},
			{Step: 36, Pitch: 7, Dur: 0.19, Vol: 0.72},
		},
		chordAt(38, 0.3, 0.76, 7, 12),
		chordAt(40, 0.2, 0.87, 7, 14),
		[]Hit{
			{Step: 42, Pitch: 14, Dur: 0.41, Vol: 0.76}, {Step: 46, Pitch: 7, Dur: 0.32, Vol: 0.69},
			{Step: 48, Pitch: -1, Dur: 1.2, Vol: 0.38}, {Step: 50, Pitch: 11, Dur: 1.3, Vol: 0.54},
			{Step: 52, Pitch: 7, Dur: 0.84, Vol: 0.76}, {Step: 54, Pitch: 16, Dur: 0.4, Vol: 0.68},
			{Step: 56, Pitch: 14, Dur: 2.0, Vol: 0.72}, {Step: 58, Pitch: 11, Dur: 1.4, Vol: 0.53},
			{Step: 60, Pitch: 7, Dur: 0.87, Vol: 0.66}, {Step: 62, Pitch: 4, Dur: 0.43, Vol: 0.64},
		},
	)
	// Tremolo-guitar chord tops (intro): measured 8ths with a velocity swell.
	tremBar := func(base int, p float64) []Hit {
		vols := []float64{0.3, 0.36, 0.42, 0.48, 0.54, 0.5, 0.42, 0.34}
		out := make([]Hit, 8)
		for i := 0; i < 8; i++ {
			out[i] = Hit{Step: base + i*2, Pitch: p, Dur: 0.24, Vol: vols[i]}
		}
		return out
	}
	// Segunda 2-bar figure (upbeat chain of beats 3-4, landing each downbeat).
	segunda := []Hit{
		{Step: 8, Pitch: 14, Dur: 0.15, Vol: 0.85}, {Step: 9, Pitch: 12, Dur: 0.14, Vol: 0.62},
		{Step: 10, Pitch: 7, Dur: 0.14, Vol: 0.67}, {Step: 11, Pitch: 12, Dur: 0.07, Vol: 0.71},
		{Step: 12, Pitch: 12, Dur: 0.09, Vol: 0.77}, {Step: 14, Pitch: 7, Dur: 0.21, Vol: 0.86},
		{Step: 16, Pitch: 4, Dur: 0.15, Vol: 0.83}, {Step: 24, Pitch: 14, Dur: 0.19, Vol: 0.88},
		{Step: 25, Pitch: 11, Dur: 0.15, Vol: 0.71}, {Step: 26, Pitch: 7, Dur: 0.14, Vol: 0.84},
		{Step: 27, Pitch: 11, Dur: 0.14, Vol: 0.72}, {Step: 28, Pitch: 11, Dur: 0.18, Vol: 0.75},
		{Step: 30, Pitch: 7, Dur: 0.2, Vol: 0.89},
	}
	// Bass (+12): bolero derecho 1, &-of-2, 3(fifth), 4 over A | C#m.
	vamp2 := []Hit{
		{Step: 0, Pitch: -12, Dur: 0.83, Vol: 0.91}, {Step: 6, Pitch: -12, Dur: 0.44, Vol: 0.89},
		{Step: 8, Pitch: -5, Dur: 0.72, Vol: 0.92}, {Step: 12, Pitch: -12, Dur: 0.44, Vol: 0.91},
		{Step: 16, Pitch: -8, Dur: 0.9, Vol: 0.87}, {Step: 22, Pitch: -8, Dur: 0.45, Vol: 0.87},
		{Step: 24, Pitch: -13, Dur: 0.62, Vol: 0.88}, {Step: 28, Pitch: -8, Dur: 0.56, Vol: 0.87},
	}
	choBassBar := func(base int, r float64) []Hit {
		return []Hit{
			{Step: base, Pitch: r, Dur: 0.83, Vol: 0.9}, {Step: base + 6, Pitch: r, Dur: 0.44, Vol: 0.86},
			{Step: base + 8, Pitch: r + 7, Dur: 0.72, Vol: 0.82}, {Step: base + 12, Pitch: r, Dur: 0.44, Vol: 0.86},
		}
	}
	choRoots := []float64{-5, -5, -10, -10, -15, -15, -5, -5, -8, -8, -10, -10, -15, -15, -5, -13}
	var choBass []Hit
	for i, r := range choRoots {
		choBass = append(choBass, choBassBar(256+i*16, r)...)
	}
	// Verse melody (Romeo substitute) — first couplet, 4-bar phrase.
	verseMel := []Hit{
		{Step: 0, Pitch: 12, Dur: 0.39, Vol: 0.75}, {Step: 2, Pitch: 16, Dur: 0.39, Vol: 0.75},
		{Step: 4, Pitch: 19, Dur: 0.39, Vol: 0.74}, {Step: 6, Pitch: 16, Dur: 0.39, Vol: 0.74},
		{Step: 8, Pitch: 21, Dur: 0.71, Vol: 0.75}, {Step: 11, Pitch: 19, Dur: 0.71, Vol: 0.75},
		{Step: 14, Pitch: 16, Dur: 0.46, Vol: 0.74}, {Step: 16, Pitch: 16, Dur: 0.46, Vol: 0.74},
		{Step: 18, Pitch: 19, Dur: 0.46, Vol: 0.75}, {Step: 20, Pitch: 19, Dur: 0.46, Vol: 0.74},
		{Step: 22, Pitch: 16, Dur: 0.46, Vol: 0.75}, {Step: 24, Pitch: 21, Dur: 0.46, Vol: 0.75},
		{Step: 26, Pitch: 19, Dur: 0.39, Vol: 0.74}, {Step: 30, Pitch: 16, Dur: 0.46, Vol: 0.75},
		{Step: 32, Pitch: 16, Dur: 0.46, Vol: 0.74}, {Step: 34, Pitch: 19, Dur: 0.46, Vol: 0.74},
		{Step: 36, Pitch: 19, Dur: 0.46, Vol: 0.75}, {Step: 38, Pitch: 16, Dur: 0.46, Vol: 0.75},
		{Step: 40, Pitch: 21, Dur: 0.71, Vol: 0.74}, {Step: 43, Pitch: 19, Dur: 0.58, Vol: 0.74},
		{Step: 46, Pitch: 19, Dur: 0.46, Vol: 0.75}, {Step: 48, Pitch: 18, Dur: 0.71, Vol: 0.75},
		{Step: 51, Pitch: 18, Dur: 0.21, Vol: 0.74}, {Step: 52, Pitch: 18, Dur: 0.63, Vol: 0.74},
		{Step: 55, Pitch: 16, Dur: 0.29, Vol: 0.75}, {Step: 56, Pitch: 16, Dur: 0.67, Vol: 0.74},
		{Step: 60, Pitch: 11, Dur: 0.4, Vol: 0.75}, {Step: 62, Pitch: 11, Dur: 0.4, Vol: 0.75},
	}
	// Chorus hook (16 bars, verbatim).
	chorusMel := []Hit{
		{Step: 0, Pitch: 19, Dur: 4.7, Vol: 0.79}, {Step: 22, Pitch: 14, Dur: 0.45, Vol: 0.79},
		{Step: 24, Pitch: 19, Dur: 0.91, Vol: 0.78}, {Step: 28, Pitch: 23, Dur: 0.8, Vol: 0.79},
		{Step: 32, Pitch: 23, Dur: 1.5, Vol: 0.78}, {Step: 38, Pitch: 24, Dur: 0.21, Vol: 0.78},
		{Step: 39, Pitch: 23, Dur: 0.21, Vol: 0.79}, {Step: 40, Pitch: 21, Dur: 3.0, Vol: 0.79},
		{Step: 56, Pitch: 21, Dur: 0.63, Vol: 0.78}, {Step: 59, Pitch: 23, Dur: 0.63, Vol: 0.78},
		{Step: 61, Pitch: 24, Dur: 0.63, Vol: 0.79}, {Step: 64, Pitch: 24, Dur: 0.62, Vol: 0.79},
		{Step: 67, Pitch: 23, Dur: 0.79, Vol: 0.78}, {Step: 70, Pitch: 24, Dur: 0.22, Vol: 0.78},
		{Step: 71, Pitch: 23, Dur: 0.22, Vol: 0.79}, {Step: 72, Pitch: 21, Dur: 2.3, Vol: 0.78},
		{Step: 86, Pitch: 24, Dur: 0.42, Vol: 0.79}, {Step: 88, Pitch: 24, Dur: 0.67, Vol: 0.79},
		{Step: 91, Pitch: 23, Dur: 0.67, Vol: 0.78}, {Step: 94, Pitch: 21, Dur: 0.42, Vol: 0.79},
		{Step: 96, Pitch: 23, Dur: 0.96, Vol: 0.78}, {Step: 102, Pitch: 19, Dur: 2.1, Vol: 0.78},
		{Step: 118, Pitch: 14, Dur: 0.46, Vol: 0.78}, {Step: 120, Pitch: 19, Dur: 0.96, Vol: 0.79},
		{Step: 124, Pitch: 23, Dur: 0.96, Vol: 0.79}, {Step: 128, Pitch: 21, Dur: 1.5, Vol: 0.78},
		{Step: 134, Pitch: 23, Dur: 0.23, Vol: 0.78}, {Step: 135, Pitch: 21, Dur: 0.22, Vol: 0.79},
		{Step: 136, Pitch: 19, Dur: 0.3, Vol: 0.78}, {Step: 137, Pitch: 21, Dur: 0.3, Vol: 0.79},
		{Step: 139, Pitch: 19, Dur: 1.3, Vol: 0.79}, {Step: 149, Pitch: 23, Dur: 0.6, Vol: 0.78},
		{Step: 152, Pitch: 23, Dur: 0.6, Vol: 0.79}, {Step: 155, Pitch: 21, Dur: 0.6, Vol: 0.78},
		{Step: 157, Pitch: 21, Dur: 0.6, Vol: 0.78}, {Step: 160, Pitch: 23, Dur: 1.5, Vol: 0.79},
		{Step: 166, Pitch: 24, Dur: 0.22, Vol: 0.79}, {Step: 167, Pitch: 23, Dur: 0.22, Vol: 0.78},
		{Step: 168, Pitch: 21, Dur: 2.5, Vol: 0.78}, {Step: 182, Pitch: 21, Dur: 0.46, Vol: 0.79},
		{Step: 184, Pitch: 21, Dur: 0.63, Vol: 0.79}, {Step: 187, Pitch: 23, Dur: 0.63, Vol: 0.78},
		{Step: 189, Pitch: 24, Dur: 0.62, Vol: 0.78}, {Step: 192, Pitch: 24, Dur: 0.98, Vol: 0.79},
		{Step: 196, Pitch: 23, Dur: 0.21, Vol: 0.78}, {Step: 197, Pitch: 24, Dur: 0.21, Vol: 0.79},
		{Step: 198, Pitch: 23, Dur: 0.21, Vol: 0.78}, {Step: 199, Pitch: 21, Dur: 2.1, Vol: 0.78},
		{Step: 210, Pitch: 16, Dur: 0.43, Vol: 0.79}, {Step: 212, Pitch: 24, Dur: 0.43, Vol: 0.78},
		{Step: 214, Pitch: 24, Dur: 0.43, Vol: 0.78}, {Step: 216, Pitch: 24, Dur: 0.43, Vol: 0.78},
		{Step: 218, Pitch: 23, Dur: 0.43, Vol: 0.78}, {Step: 220, Pitch: 23, Dur: 0.43, Vol: 0.78},
		{Step: 222, Pitch: 21, Dur: 0.46, Vol: 0.78}, {Step: 224, Pitch: 23, Dur: 0.72, Vol: 0.79},
		{Step: 227, Pitch: 21, Dur: 0.21, Vol: 0.78}, {Step: 228, Pitch: 23, Dur: 0.21, Vol: 0.79},
		{Step: 229, Pitch: 21, Dur: 0.21, Vol: 0.78}, {Step: 230, Pitch: 19, Dur: 1.75, Vol: 0.79},
		{Step: 252, Pitch: 11, Dur: 0.39, Vol: 0.78}, {Step: 254, Pitch: 11, Dur: 0.39, Vol: 0.78},
	}
	// Judy Santos duet answer (labeled idiomatic): sparse 3rd-below doubles of
	// the hook's long holds.
	duet := []Hit{
		{Step: 0, Pitch: 16, Dur: 4.7, Vol: 0.4}, {Step: 28, Pitch: 19, Dur: 0.8, Vol: 0.4},
		{Step: 32, Pitch: 19, Dur: 1.5, Vol: 0.4}, {Step: 40, Pitch: 18, Dur: 3.0, Vol: 0.4},
		{Step: 96, Pitch: 19, Dur: 0.96, Vol: 0.4}, {Step: 102, Pitch: 16, Dur: 2.1, Vol: 0.4},
		{Step: 160, Pitch: 19, Dur: 1.5, Vol: 0.4}, {Step: 168, Pitch: 18, Dur: 2.5, Vol: 0.4},
		{Step: 224, Pitch: 19, Dur: 0.72, Vol: 0.4}, {Step: 230, Pitch: 16, Dur: 1.75, Vol: 0.4},
	}
	// Choir pads (chorus, one roll per bar).
	choPadChords := [][]float64{
		{7, 11, 14}, {7, 11, 14}, {2, 6, 9}, {2, 6, 9}, {9, 12, 16}, {9, 12, 16}, {7, 11, 14}, {7, 11, 14},
		{4, 7, 11}, {4, 7, 11}, {2, 6, 9}, {2, 6, 9}, {9, 12, 16}, {9, 12, 16}, {7, 11, 14}, {11, 15, 18},
	}
	var choPads []Hit
	for i, c := range choPadChords {
		choPads = append(choPads, chordAt(256+i*16, 3.8, 0.42, c...)...)
	}
	// Derecho percussion (per bar) — groove from the verse on.
	kickBar := []Hit{{Step: 0, Vol: 0.81}, {Step: 6, Vol: 0.72}, {Step: 8, Vol: 0.8}}
	hatBar := []Hit{{Step: 0, Vol: 0.49}, {Step: 4, Vol: 0.49}, {Step: 8, Vol: 0.49}, {Step: 12, Vol: 0.49}}
	guiraBar := []Hit{
		{Step: 0, Vol: 0.68}, {Step: 2, Vol: 0.45}, {Step: 4, Vol: 0.62}, {Step: 6, Vol: 0.45},
		{Step: 8, Vol: 0.65}, {Step: 10, Vol: 0.45}, {Step: 12, Vol: 0.62}, {Step: 14, Vol: 0.45},
	}
	bongoHiBar := []Hit{{Step: 0, Dur: 0.2, Vol: 0.83}, {Step: 8, Dur: 0.2, Vol: 0.83}}
	beat4Bar := []Hit{{Step: 12, Dur: 0.3, Vol: 0.81}, {Step: 14, Dur: 0.3, Vol: 0.81}}
	chatterBar := []Hit{{Step: 2, Dur: 0.2, Vol: 0.55}, {Step: 6, Dur: 0.2, Vol: 0.68}, {Step: 10, Dur: 0.2, Vol: 0.78}}
	// The sourced bongó roll into every 4th bar.
	roll := func(base int) []Hit {
		return []Hit{
			{Step: base + 9, Dur: 0.15, Vol: 0.61}, {Step: base + 10, Dur: 0.15, Vol: 0.67},
			{Step: base + 11, Dur: 0.15, Vol: 0.69}, {Step: base + 13, Dur: 0.15, Vol: 0.9},
			{Step: base + 15, Dur: 0.15, Vol: 0.91},
		}
	}
	return Showcase{
		Stem: "bachata-obsesion", BPM: 134, Subdiv: 16, Bars: 32,
		Insts: []InstSpec{
			{ID: "guitar-nylon-bright", Name: "Requinto", Volume: 0.72, Pan: -0.2, ReverbSend: 0.16},
			{ID: "guitar-steel", Name: "Tremolo", Volume: 0.45, Pan: 0.3, ReverbSend: 0.18},
			{ID: "guitar-electric-neck", Name: "Segunda", Volume: 0.5, Pan: 0.25},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "voice-soprano", Name: "Romeo", Volume: 0.66, ReverbSend: 0.18},
			{ID: "ensemble-lead", Name: "Judy", Volume: 0.45, Pan: 0.15, ReverbSend: 0.2},
			{ID: "viola-pad", Name: "Choir", Volume: 0.42, Pan: -0.15, ReverbSend: 0.24},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.85},
			{ID: "hihat", Name: "Hihat", Volume: 0.45},
			{ID: "shaker", Name: "Güira", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga", Name: "Bongó Hi", Volume: 0.62, Pan: 0.3, SynthParams: map[string]float64{"decay": 0.5}},
			{ID: "conga-open", Name: "Beat 4", Volume: 0.78, Pan: 0.3},
			{ID: "conga-tumba", Name: "Chatter", Volume: 0.5, Pan: -0.3},
			{ID: "sidestick", Name: "Bell", Volume: 0.6, Pan: 0.2},
		},
		Rows: []RowSpec{
			// Requinto: intro loop ×2, then verse comp + answer fill ×2.
			{Inst: "guitar-nylon-bright", Hits: concat(
				introRiff, at(64, introRiff),
				at(128, verseReq), at(192, verseReq),
			)},
			// Tremolo tops (intro only): C#5 G#5 D#5 G#5 per 4-bar loop.
			{Inst: "guitar-steel", Hits: concat(
				tremBar(0, 16), tremBar(16, 23), tremBar(32, 18), tremBar(48, 23),
				tremBar(64, 16), tremBar(80, 23), tremBar(96, 18), tremBar(112, 23),
			)},
			// Segunda: verse + chorus (the muted upbeat chain).
			{Inst: "guitar-electric-neck", Hits: concat(
				at(128, ostinato2(8, segunda)),
				at(256, ostinato2(16, segunda)),
			)},
			// Bass: verse vamp (with the octave C#3 run closing the verse) +
			// the chorus derecho over E|B|F#m|E / C#m|B|F#m|E-G#.
			{Inst: "bass-guitar", Hits: concat(
				at(128, vamp2), at(160, vamp2), at(192, vamp2),
				at(224, []Hit{
					{Step: 0, Pitch: -12, Dur: 0.83, Vol: 0.91}, {Step: 6, Pitch: -12, Dur: 0.44, Vol: 0.89},
					{Step: 8, Pitch: -5, Dur: 0.72, Vol: 0.92}, {Step: 12, Pitch: -12, Dur: 0.44, Vol: 0.91},
					{Step: 16, Pitch: -8, Dur: 0.9, Vol: 0.87}, {Step: 22, Pitch: -8, Dur: 0.45, Vol: 0.87},
					{Step: 24, Pitch: 4, Dur: 0.2, Vol: 0.72}, {Step: 25, Pitch: 4, Dur: 0.2, Vol: 0.74},
					{Step: 26, Pitch: 4, Dur: 0.2, Vol: 0.77}, {Step: 28, Pitch: 4, Dur: 0.2, Vol: 0.8},
					{Step: 29, Pitch: 4, Dur: 0.2, Vol: 0.82}, {Step: 30, Pitch: 4, Dur: 0.2, Vol: 0.86},
				}),
				choBass,
			)},
			{Inst: "voice-soprano", Hits: concat(at(128, verseMel), at(192, verseMel), at(256, chorusMel))},
			{Inst: "ensemble-lead", Hits: at(256, duet)},
			// Choir pads: chorus; plus the intro string pad (C#m / G#m holds).
			{Inst: "viola-pad", Hits: concat(
				chordAt(0, 7.8, 0.4, 4, 7, 11), chordAt(32, 7.8, 0.4, -1, 2, 6),
				chordAt(64, 7.8, 0.4, 4, 7, 11), chordAt(96, 7.8, 0.4, -1, 2, 6),
				choPads,
			)},
			{Inst: "kick-acoustic", Hits: at(128, ostinato(24, kickBar))},
			{Inst: "hihat", Hits: at(128, ostinato(24, hatBar))},
			{Inst: "shaker", Hits: at(128, ostinato(24, guiraBar))},
			// Bongó martillo + the roll into every 4th bar.
			{Inst: "conga", Hits: concat(
				at(128, ostinato(24, bongoHiBar)),
				roll(176), roll(240), roll(304), roll(368), roll(432), roll(496),
			)},
			{Inst: "conga-open", Hits: at(128, ostinato(24, beat4Bar))},
			{Inst: "conga-tumba", Hits: at(128, ostinato(24, chatterBar))},
			// Bell layer joins for the chorus (majao-leaning lift).
			{Inst: "sidestick", Hits: at(256, ostinato(16, []Hit{{Step: 0, Vol: 0.83}, {Step: 8, Vol: 0.83}}))},
		},
	}
}
