package main

// Toto — "Africa" (1982). A major (verse leans B/G#m; chorus F#m–D–A–E),
// 94 BPM (both fetched MIDIs). Note data from BitMidi 105027 (30 tracks,
// drums pre-split per voice); production research: Reverb synth-sounds piece
// (GS-1 kalimba riff ×6 layers, CS-80 pad), MusicRadar Paich interview,
// Porcaro's own account of the tape percussion loop. Dossier:
// toto-africa_dossier.md.
//
// 22-bar arc: intro riff (2-bar unit ×2) → verse 4-bar cycle ×2 (vocal enters)
// → 2-bar pre-chorus climb → 8-bar chorus (hook stack + woodblock/bongo join).
// The percussion loop (kick lope + tom backbeat + pedal-hat 16ths + conga
// ghost-"e"-of-3) runs unbroken start to finish — that constancy IS the song.
//
// GROOVE: straight 16ths (measured; zero swing at all four positions) — the
// lope lives in the kick's 1–"a"–2 cell and the velocity lattice.
func africaShowcase() Showcase {
	// Intro 2-bar unit. Bar A: GS-1 stabs (top voice; the A3 line lives on the
	// marimba row); bar B: the 16th answer-run (top notes of the dyads).
	riffTop := []Hit{
		{Step: 0, Pitch: 4, Dur: 0.63, Vol: 0.7}, {Step: 3, Pitch: 4, Dur: 0.17, Vol: 0.68},
		{Step: 5, Pitch: 4, Dur: 0.13, Vol: 0.72}, {Step: 7, Pitch: 4, Dur: 0.1, Vol: 0.66},
		{Step: 8, Pitch: 4, Dur: 0.23, Vol: 0.7}, {Step: 10, Pitch: 2, Dur: 0.15, Vol: 0.7},
		{Step: 12, Pitch: 7, Dur: 4.9, Vol: 0.72},
	}
	marimba := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.6, Vol: 0.6}, {Step: 3, Pitch: 0, Dur: 0.17, Vol: 0.6},
		{Step: 5, Pitch: 0, Dur: 0.13, Vol: 0.6}, {Step: 7, Pitch: 0, Dur: 0.1, Vol: 0.6},
		{Step: 8, Pitch: 0, Dur: 0.23, Vol: 0.6}, {Step: 10, Pitch: -1, Dur: 0.15, Vol: 0.6},
		{Step: 12, Pitch: 4, Dur: 4.9, Vol: 0.6},
	}
	answerRun := []Hit{
		{Step: 14, Pitch: 21, Dur: 0.24, Vol: 0.77}, {Step: 15, Pitch: 19, Dur: 0.24, Vol: 0.71},
		{Step: 16, Pitch: 16, Dur: 0.24, Vol: 0.65}, {Step: 17, Pitch: 19, Dur: 0.24, Vol: 0.68},
		{Step: 18, Pitch: 21, Dur: 0.24, Vol: 0.74}, {Step: 19, Pitch: 19, Dur: 0.24, Vol: 0.68},
		{Step: 20, Pitch: 16, Dur: 0.24, Vol: 0.68}, {Step: 21, Pitch: 19, Dur: 0.24, Vol: 0.71},
		{Step: 22, Pitch: 21, Dur: 0.24, Vol: 0.74}, {Step: 23, Pitch: 19, Dur: 0.24, Vol: 0.68},
		{Step: 24, Pitch: 16, Dur: 0.24, Vol: 0.68}, {Step: 25, Pitch: 14, Dur: 0.24, Vol: 0.71},
		{Step: 26, Pitch: 16, Dur: 0.24, Vol: 0.65}, {Step: 27, Pitch: 14, Dur: 0.24, Vol: 0.65},
		{Step: 28, Pitch: 16, Dur: 0.24, Vol: 0.71}, {Step: 29, Pitch: 21, Dur: 0.24, Vol: 0.82},
		{Step: 30, Pitch: 19, Dur: 0.24, Vol: 0.79},
	}
	// Verse keys cycle (CS-80 pad voicings) + the riff-stab turnaround.
	verseKeys := concat(
		chordAt(0, 3.8, 0.5, -3, 2, 6),
		chordAt(8, 2.0, 0.45, 1, 4),
		chordAt(16, 3.8, 0.5, -1, 2, 6),
		chordAt(32, 3.0, 0.5, -5, 0, 4),
		chordAt(40, 3.0, 0.45, -1, 2),
		chordAt(48, 1.8, 0.45, -3, 6),
		// Riff-stab turnaround (the source's step-66/68 spill into the next
		// cycle is dropped — it would collide with that cycle's downbeat roll).
		[]Hit{
			{Step: 56, Pitch: 4, Dur: 0.24, Vol: 0.6}, {Step: 59, Pitch: 4, Dur: 0.24, Vol: 0.6},
			{Step: 61, Pitch: 4, Dur: 0.24, Vol: 0.6}, {Step: 63, Pitch: 4, Dur: 0.24, Vol: 0.6},
		},
	)
	// Verse bass cell: beat 1, "a"-of-1, beat 2 per half-bar. Encoded +12
	// (bass-guitar renders one octave down).
	bassCell := func(base int, p float64) []Hit {
		return []Hit{
			{Step: base, Pitch: p, Dur: 0.4, Vol: 0.85}, {Step: base + 3, Pitch: p, Dur: 0.2, Vol: 0.8},
			{Step: base + 4, Pitch: p, Dur: 0.5, Vol: 0.85},
		}
	}
	verseBass := concat(
		bassCell(0, -10), bassCell(8, -6), bassCell(16, -13), bassCell(24, -15),
		bassCell(32, -5), bassCell(40, -15), bassCell(48, -13),
		[]Hit{{Step: 56, Pitch: -12, Dur: 0.3, Vol: 0.85}, {Step: 59, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 61, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 63, Pitch: -12, Dur: 0.2, Vol: 0.8}},
	)
	// Verse vocal (solo, intimate — sax substitute).
	verseVocal := []Hit{
		{Step: 1, Pitch: 9, Dur: 0.2, Vol: 0.53}, {Step: 2, Pitch: 9, Dur: 0.2, Vol: 0.53},
		{Step: 3, Pitch: 9, Dur: 0.2, Vol: 0.53}, {Step: 4, Pitch: 9, Dur: 0.73, Vol: 0.53},
		{Step: 8, Pitch: 9, Dur: 0.13, Vol: 0.53}, {Step: 9, Pitch: 9, Dur: 0.55, Vol: 0.53},
		{Step: 12, Pitch: 11, Dur: 0.39, Vol: 0.53}, {Step: 14, Pitch: 13, Dur: 0.15, Vol: 0.53},
		{Step: 15, Pitch: 14, Dur: 1.58, Vol: 0.53},
		{Step: 24, Pitch: 6, Dur: 0.13, Vol: 0.52}, {Step: 25, Pitch: 6, Dur: 0.65, Vol: 0.53},
		{Step: 28, Pitch: 7, Dur: 0.38, Vol: 0.47}, {Step: 30, Pitch: 9, Dur: 0.15, Vol: 0.43},
		{Step: 31, Pitch: 9, Dur: 0.4, Vol: 0.5}, {Step: 33, Pitch: 7, Dur: 0.54, Vol: 0.52},
		{Step: 36, Pitch: 7, Dur: 0.39, Vol: 0.47}, {Step: 38, Pitch: 6, Dur: 0.15, Vol: 0.43},
		{Step: 39, Pitch: 4, Dur: 0.94, Vol: 0.5}, {Step: 44, Pitch: 4, Dur: 0.52, Vol: 0.52},
		{Step: 46, Pitch: 2, Dur: 0.48, Vol: 0.52}, {Step: 48, Pitch: 6, Dur: 1.23, Vol: 0.47},
		{Step: 53, Pitch: 4, Dur: 0.21, Vol: 0.45}, {Step: 54, Pitch: 2, Dur: 0.46, Vol: 0.45},
		{Step: 56, Pitch: 4, Dur: 2.06, Vol: 0.53},
	}
	// Pre-chorus climbing harmony (top notes of the dyads) over G#→A→C# bass.
	preChorusHook := []Hit{
		{Step: 0, Pitch: 18, Dur: 0.24, Vol: 0.53}, {Step: 1, Pitch: 18, Dur: 0.7, Vol: 0.5},
		{Step: 4, Pitch: 19, Dur: 0.4, Vol: 0.5}, {Step: 6, Pitch: 21, Dur: 0.35, Vol: 0.45},
		{Step: 8, Pitch: 21, Dur: 0.4, Vol: 0.5}, {Step: 10, Pitch: 19, Dur: 0.15, Vol: 0.45},
		{Step: 11, Pitch: 19, Dur: 0.4, Vol: 0.52}, {Step: 12, Pitch: 14, Dur: 0.26, Vol: 0.37},
		{Step: 13, Pitch: 18, Dur: 0.17, Vol: 0.5}, {Step: 14, Pitch: 19, Dur: 5.0, Vol: 0.5},
	}
	// Chorus 2-bar chord cycle (keys voicings + counterline tail).
	chorusKeys := concat(
		chordAt(0, 5.0, 0.6, 0, 4, 9, 12),
		[]Hit{{Step: 8, Pitch: 5, Dur: 2.0, Vol: 0.6}},
		chordAt(16, 2.0, 0.55, 4, 7),
		chordAt(24, 1.5, 0.55, -1, 2, 11),
		[]Hit{{Step: 27, Pitch: 12, Dur: 0.68, Vol: 0.55}, {Step: 30, Pitch: 11, Dur: 0.49, Vol: 0.58}},
	)
	chorusBass := concat(
		bassCell(0, -15), bassCell(8, -7), bassCell(16, -12), bassCell(24, -5),
		[]Hit{{Step: 30, Pitch: -10, Dur: 0.2, Vol: 0.75}},
	)
	// Chorus hook: the stacked A5 block (4-bar phrase = 2 chord cycles).
	hookPhrase := []Hit{
		{Step: 2, Pitch: 24, Dur: 0.35, Vol: 0.68}, {Step: 4, Pitch: 24, Dur: 0.31, Vol: 0.65},
		{Step: 6, Pitch: 24, Dur: 0.17, Vol: 0.5}, {Step: 7, Pitch: 24, Dur: 0.59, Vol: 0.63},
		{Step: 10, Pitch: 24, Dur: 0.39, Vol: 0.6}, {Step: 12, Pitch: 24, Dur: 0.35, Vol: 0.63},
		{Step: 14, Pitch: 24, Dur: 0.39, Vol: 0.65}, {Step: 16, Pitch: 24, Dur: 1.48, Vol: 0.75},
		{Step: 22, Pitch: 23, Dur: 0.15, Vol: 0.72}, {Step: 23, Pitch: 23, Dur: 1.84, Vol: 0.68},
		{Step: 34, Pitch: 24, Dur: 0.2, Vol: 0.6}, {Step: 36, Pitch: 24, Dur: 0.2, Vol: 0.65},
		{Step: 37, Pitch: 24, Dur: 0.2, Vol: 0.6}, {Step: 38, Pitch: 24, Dur: 0.2, Vol: 0.63},
		{Step: 39, Pitch: 24, Dur: 0.2, Vol: 0.6}, {Step: 40, Pitch: 24, Dur: 0.15, Vol: 0.65},
		{Step: 41, Pitch: 24, Dur: 0.39, Vol: 0.65}, {Step: 43, Pitch: 24, Dur: 0.3, Vol: 0.6},
		{Step: 45, Pitch: 24, Dur: 0.3, Vol: 0.6}, {Step: 47, Pitch: 24, Dur: 0.9, Vol: 0.72},
		{Step: 51, Pitch: 24, Dur: 0.2, Vol: 0.6}, {Step: 52, Pitch: 24, Dur: 0.2, Vol: 0.65},
		{Step: 53, Pitch: 23, Dur: 0.51, Vol: 0.68},
	}
	// Backing dyads under hook phrase 2 (soft airy stack).
	backing := concat(
		chordAt(34, 1.5, 0.4, 4, 7),
		chordAt(40, 1.5, 0.4, 5, 9),
		chordAt(47, 0.8, 0.4, 4, 7),
		chordAt(53, 2.0, 0.4, 2, 7),
	)
	// The unbroken Porcaro/Castro loop (per bar).
	kickBar := []Hit{{Step: 0, Vol: 0.9}, {Step: 3, Vol: 0.96}, {Step: 4, Vol: 0.96}, {Step: 8, Vol: 0.9}, {Step: 11, Vol: 0.95}, {Step: 12, Vol: 0.96}}
	pedalBar := []Hit{
		{Step: 0, Vol: 0.61}, {Step: 1, Vol: 0.38}, {Step: 2, Vol: 0.49}, {Step: 3, Vol: 0.39},
		{Step: 4, Vol: 0.63}, {Step: 5, Vol: 0.38}, {Step: 6, Vol: 0.49}, {Step: 7, Vol: 0.39},
		{Step: 8, Vol: 0.61}, {Step: 9, Vol: 0.38}, {Step: 10, Vol: 0.49}, {Step: 11, Vol: 0.39},
		{Step: 12, Vol: 0.63}, {Step: 13, Vol: 0.38}, {Step: 14, Vol: 0.49}, {Step: 15, Vol: 0.39},
	}
	congaBar := []Hit{
		{Step: 0, Dur: 0.3, Vol: 0.76}, {Step: 4, Dur: 0.3, Vol: 0.82}, {Step: 8, Dur: 0.3, Vol: 0.76},
		{Step: 9, Dur: 0.25, Vol: 0.57}, {Step: 12, Dur: 0.3, Vol: 0.79},
	}
	agogoBar := []Hit{
		{Step: 2, Vol: 0.49}, {Step: 5, Vol: 0.24}, {Step: 7, Vol: 0.53},
		{Step: 11, Vol: 0.39}, {Step: 14, Vol: 0.53}, {Step: 15, Vol: 0.36},
	}
	woodblockBar := []Hit{
		{Step: 0, Vol: 0.58}, {Step: 1, Vol: 0.28}, {Step: 2, Vol: 0.31}, {Step: 3, Vol: 0.35},
		{Step: 4, Vol: 0.5}, {Step: 6, Vol: 0.13}, {Step: 7, Vol: 0.28}, {Step: 8, Vol: 0.31},
		{Step: 10, Vol: 0.63}, {Step: 12, Vol: 0.35}, {Step: 14, Vol: 0.35},
	}
	return Showcase{
		Stem: "toto-africa", BPM: 94, Subdiv: 16, Bars: 22,
		Insts: []InstSpec{
			{ID: "fm-pluck", Name: "Kalimba", Volume: 0.65, Pan: 0.15},
			{ID: "fm-pluck-1", Name: "Marimba", Volume: 0.5, Pan: -0.2},
			{ID: "fm-bell", Name: "Answer", Volume: 0.55, Pan: 0.25},
			{ID: "modular-pad", Name: "CS-80 Pad", Volume: 0.5, ReverbSend: 0.2},
			{ID: "viola-pad", Name: "Root Synth", Volume: 0.42, Pan: -0.15, ReverbSend: 0.18},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "sax", Name: "Verse Voc", Volume: 0.62, ReverbSend: 0.16},
			{ID: "ensemble-lead", Name: "Hook", Volume: 0.6, ReverbSend: 0.24},
			{ID: "voice-whisper", Name: "Backing", Volume: 0.45, Pan: 0.2, ReverbSend: 0.26},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.95},
			{ID: "tom", Name: "Tom", Volume: 0.72, ReverbSend: 0.14},
			{ID: "hihat-pedal", Name: "Pedal Hat", Volume: 0.5},
			{ID: "ride", Name: "Ride", Volume: 0.32, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
			{ID: "conga-open", Name: "Conga", Volume: 0.55, Pan: 0.3},
			{ID: "conga-tumba", Name: "Bongo Lo", Volume: 0.5, Pan: -0.3},
			{ID: "cowbell-1", Name: "Agogo", Volume: 0.38, Pan: 0.35},
			{ID: "sidestick", Name: "Woodblock", Volume: 0.45, Pan: -0.35},
			{ID: "conga", Name: "Bongo Hi", Volume: 0.5, Pan: 0.2},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Riff layers: intro ×2; the stab bar also closes each verse cycle
			// (encoded inside verseKeys/verseBass turnarounds).
			{Inst: "fm-pluck", Hits: concat(riffTop, at(32, riffTop))},
			{Inst: "fm-pluck-1", Hits: concat(marimba, at(32, marimba))},
			{Inst: "fm-bell", Hits: concat(answerRun, at(32, answerRun))},
			// Pads: verse cycles at bars 5 & 9, pre-chorus stabs, chorus cycle ×4.
			{Inst: "modular-pad", Hits: concat(
				at(64, verseKeys), at(128, verseKeys),
				chordAt(208, 0.4, 0.55, -5, 0, 4), chordAt(211, 0.4, 0.5, -6, -1, 2),
				chordAt(214, 1.2, 0.5, -1, 4, 7),
				at(224, chorusKeys), at(256, chorusKeys), at(288, chorusKeys), at(320, chorusKeys),
			)},
			{Inst: "viola-pad", Hits: concat(
				at(224, []Hit{{Step: 0, Pitch: -3, Dur: 2, Vol: 0.5}, {Step: 8, Pitch: -7, Dur: 2, Vol: 0.5}, {Step: 16, Pitch: 0, Dur: 2, Vol: 0.5}, {Step: 24, Pitch: -5, Dur: 2, Vol: 0.5}}),
				at(256, []Hit{{Step: 0, Pitch: -3, Dur: 2, Vol: 0.5}, {Step: 8, Pitch: -7, Dur: 2, Vol: 0.5}, {Step: 16, Pitch: 0, Dur: 2, Vol: 0.5}, {Step: 24, Pitch: -5, Dur: 2, Vol: 0.5}}),
				at(288, []Hit{{Step: 0, Pitch: -3, Dur: 2, Vol: 0.5}, {Step: 8, Pitch: -7, Dur: 2, Vol: 0.5}, {Step: 16, Pitch: 0, Dur: 2, Vol: 0.5}, {Step: 24, Pitch: -5, Dur: 2, Vol: 0.5}}),
				at(320, []Hit{{Step: 0, Pitch: -3, Dur: 2, Vol: 0.5}, {Step: 8, Pitch: -7, Dur: 2, Vol: 0.5}, {Step: 16, Pitch: 0, Dur: 2, Vol: 0.5}, {Step: 24, Pitch: -5, Dur: 2, Vol: 0.5}}),
			)},
			// Bass: intro riff double, verse cycles, pre-chorus walk, chorus.
			// (+12 octave policy throughout.)
			{Inst: "bass-guitar", Hits: concat(
				[]Hit{{Step: 0, Pitch: -12, Dur: 0.5, Vol: 0.85}, {Step: 3, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 5, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 7, Pitch: -12, Dur: 0.15, Vol: 0.8}, {Step: 8, Pitch: -12, Dur: 0.3, Vol: 0.82}, {Step: 10, Pitch: -13, Dur: 0.2, Vol: 0.8}, {Step: 12, Pitch: -8, Dur: 4.6, Vol: 0.85}},
				at(32, []Hit{{Step: 0, Pitch: -12, Dur: 0.5, Vol: 0.85}, {Step: 3, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 5, Pitch: -12, Dur: 0.2, Vol: 0.8}, {Step: 7, Pitch: -12, Dur: 0.15, Vol: 0.8}, {Step: 8, Pitch: -12, Dur: 0.3, Vol: 0.82}, {Step: 10, Pitch: -13, Dur: 0.2, Vol: 0.8}, {Step: 12, Pitch: -8, Dur: 4.6, Vol: 0.85}}),
				at(64, verseBass), at(128, verseBass),
				at(192, concat(
					bassCell(0, -13),
					[]Hit{{Step: 8, Pitch: -12, Dur: 0.3, Vol: 0.85}, {Step: 11, Pitch: -12, Dur: 0.2, Vol: 0.82}, {Step: 13, Pitch: -12, Dur: 0.2, Vol: 0.82}, {Step: 15, Pitch: -12, Dur: 0.2, Vol: 0.82}, {Step: 16, Pitch: -12, Dur: 0.4, Vol: 0.85}, {Step: 18, Pitch: -13, Dur: 0.3, Vol: 0.85}, {Step: 20, Pitch: -8, Dur: 3.0, Vol: 0.87}},
				)),
				at(224, chorusBass), at(256, chorusBass), at(288, chorusBass), at(320, chorusBass),
			)},
			{Inst: "sax", Hits: concat(at(64, verseVocal), at(128, verseVocal))},
			// Pre-chorus climb + the chorus hook block (phrase 1+2, twice).
			{Inst: "ensemble-lead", Hits: concat(
				at(192, preChorusHook),
				at(224, hookPhrase), at(288, hookPhrase),
				// End-of-chorus melisma tail into the loop wrap.
				at(224, []Hit{{Step: 125, Pitch: 21, Dur: 0.3, Vol: 0.6}, {Step: 126, Pitch: 19, Dur: 0.58, Vol: 0.6}}),
			)},
			{Inst: "voice-whisper", Hits: concat(at(224, backing), at(288, backing))},
			{Inst: "kick-acoustic", Hits: ostinato(22, kickBar)},
			// Tom backbeat + the chorus-entry mini-fill.
			{Inst: "tom", Hits: concat(
				ostinato(22, []Hit{{Step: 4, Pitch: 0, Dur: 0.3, Vol: 0.84}, {Step: 12, Pitch: 0, Dur: 0.3, Vol: 0.84}}),
				[]Hit{{Step: 218, Pitch: 0, Dur: 0.2, Vol: 0.84}, {Step: 222, Pitch: 0, Dur: 0.2, Vol: 0.84}, {Step: 223, Pitch: 0, Dur: 0.2, Vol: 0.87}},
			)},
			{Inst: "hihat-pedal", Hits: ostinato(22, pedalBar)},
			{Inst: "ride", Hits: ostinato(22, []Hit{{Step: 0, Vol: 0.35}, {Step: 4, Vol: 0.35}, {Step: 8, Vol: 0.35}, {Step: 12, Vol: 0.35}})},
			{Inst: "conga-open", Hits: ostinato(22, congaBar)},
			{Inst: "conga-tumba", Hits: ostinato(22, []Hit{{Step: 3, Dur: 0.25, Vol: 0.76}, {Step: 11, Dur: 0.25, Vol: 0.82}})},
			// Agogo lattice enters at the verse (silent in the intro).
			{Inst: "cowbell-1", Hits: at(64, ostinato(18, agogoBar))},
			// Woodblock chatter + bongo-hi accents: chorus only.
			{Inst: "sidestick", Hits: at(224, ostinato(8, woodblockBar))},
			{Inst: "conga", Hits: []Hit{{Step: 224, Dur: 0.3, Vol: 0.84}, {Step: 256, Dur: 0.3, Vol: 0.84}, {Step: 288, Dur: 0.3, Vol: 0.84}, {Step: 320, Dur: 0.3, Vol: 0.84}}},
			// Section-boundary accents (pre-chorus tail, chorus beat 2).
			{Inst: "crash", Hits: []Hit{{Step: 219, Vol: 0.45}, {Step: 223, Vol: 0.5}, {Step: 228, Vol: 0.55}}},
		},
	}
}
