package main

// Bob Marley & The Wailers — "Exodus" (1977). A minor one-chord vamp, 132 BPM
// (MIDI tempo meta + songbpm.com; the old template's 98 was unsupported by any
// fetched source). All note data from BitMidi 18775 (dossier
// marley-exodus_dossier.md; corroborated by nikku.ca + bigbasstabs bass tabs,
// Wikipedia personnel, bestmusicsheet one-drop pedagogy).
//
// 16 bars = the full chorus section ("Exodus! … movement of Jah people") ×2:
// Familyman's 4-bar bass cycle, the double-chop skank, the clavinet bubble,
// dotted-quarter guitar triads (3-against-4), piano octave walk-up, the Zap Pow
// three-part horn answer, lead-vocal hook (sax substitute, dropped one octave
// from the MIDI's register into Bob's chest voice), and the I-Threes chant.
//
// GROOVE (measured from the MIDI): bass/tambourine dead-center; kick ~+6ms,
// skank ~+5ms, clav bubble the laziest at ~+15ms — encoded as laidBack delay
// fractions of a 16th (113ms at 132 BPM).
func exodusShowcase() Showcase {
	// Familyman's 4-bar cycle: bar 2 answers with C2 on the "&" of 1 (ghosted
	// beat 1); bar 4 lands the C2 ON beat 1. G–G#–A chromatic walk-up closes
	// each half.
	bassCycle := []Hit{
		{Step: 0, Pitch: -24, Dur: 0.54, Vol: 0.8}, {Step: 4, Pitch: -26, Dur: 0.37, Vol: 0.6},
		{Step: 6, Pitch: -24, Dur: 0.64, Vol: 0.86}, {Step: 10, Pitch: -26, Dur: 0.29, Vol: 0.75},
		{Step: 12, Pitch: -24, Dur: 0.47, Vol: 0.86}, {Step: 18, Pitch: -21, Dur: 0.37, Vol: 0.84},
		{Step: 20, Pitch: -24, Dur: 0.55, Vol: 0.8}, {Step: 24, Pitch: -26, Dur: 0.42, Vol: 0.77},
		{Step: 26, Pitch: -25, Dur: 0.4, Vol: 0.74}, {Step: 28, Pitch: -24, Dur: 0.59, Vol: 0.75},
		{Step: 32, Pitch: -24, Dur: 0.51, Vol: 0.87}, {Step: 36, Pitch: -26, Dur: 0.31, Vol: 0.74},
		{Step: 38, Pitch: -24, Dur: 0.48, Vol: 0.83}, {Step: 42, Pitch: -26, Dur: 0.23, Vol: 0.8},
		{Step: 44, Pitch: -24, Dur: 0.54, Vol: 0.79}, {Step: 48, Pitch: -21, Dur: 0.63, Vol: 0.87},
		{Step: 52, Pitch: -24, Dur: 0.55, Vol: 0.81}, {Step: 56, Pitch: -26, Dur: 0.43, Vol: 0.81},
		{Step: 58, Pitch: -25, Dur: 0.44, Vol: 0.83}, {Step: 60, Pitch: -24, Dur: 0.5, Vol: 0.75},
	}
	// Double-chop skank (A3+C4 dyad, staccato), velocity crescendo through the
	// 2-bar cell.
	skankCell := concat(
		chordAt(2, 0.06, 0.6, 0, 3), chordAt(4, 0.06, 0.72, 0, 3),
		chordAt(8, 0.06, 0.61, 0, 3), chordAt(10, 0.06, 0.75, 0, 3),
		chordAt(14, 0.06, 0.65, 0, 3),
		chordAt(16, 0.06, 0.84, 0, 3), chordAt(20, 0.06, 0.67, 0, 3),
		chordAt(22, 0.06, 0.81, 0, 3), chordAt(26, 0.06, 0.86, 0, 3),
		chordAt(28, 0.06, 0.91, 0, 3), chordAt(30, 0.06, 0.94, 0, 3),
	)
	// Dotted-quarter Am triads — 3-against-4 cross-rhythm, resets every 2 bars.
	triadCell := concat(
		chordAt(0, 0.2, 0.55, 3, 7, 12), chordAt(6, 0.2, 0.43, 3, 7, 12),
		chordAt(12, 0.2, 0.51, 3, 7, 12), chordAt(18, 0.2, 0.54, 3, 7, 12),
		chordAt(24, 0.2, 0.64, 3, 7, 12),
	)
	// Clavinet bubble — RH Am/C stabs on the off-8ths (top-two voicings; the
	// LH bass mirror lives in the bass row).
	bubbleCell := concat(
		chordAt(0, 0.3, 0.41, 3, 7),
		chordAt(2, 0.2, 0.44, 0, 3),
		[]Hit{{Step: 4, Pitch: -14, Dur: 0.28, Vol: 0.54}},
		chordAt(6, 0.15, 0.57, 3, 7),
		chordAt(10, 0.16, 0.39, -14, 5),
		chordAt(12, 0.2, 0.55, 3, 7),
		[]Hit{{Step: 14, Pitch: -2, Dur: 0.14, Vol: 0.44}},
		chordAt(16, 0.25, 0.52, 3, 7),
		[]Hit{{Step: 18, Pitch: -2, Dur: 0.14, Vol: 0.34}},
		chordAt(20, 0.2, 0.43, 0, 3),
		chordAt(24, 0.25, 0.55, -14, 5),
		chordAt(26, 0.2, 0.42, 0, 3),
		chordAt(28, 0.25, 0.54, 3, 7),
		[]Hit{{Step: 30, Pitch: -2, Dur: 0.14, Vol: 0.39}},
	)
	// Piano: A octave drone + the B–C–D … B–G–A walk-up under "movement of
	// Jah people".
	pianoSection := concat(
		chordAt(0, 8.0, 0.7, -24, 0),
		chordAt(46, 0.5, 0.75, -10, 2),
		chordAt(48, 0.38, 0.8, -9, 3),
		chordAt(50, 1.5, 0.76, -7, 5),
		[]Hit{{Step: 56, Pitch: 2, Dur: 0.78, Vol: 0.87}, {Step: 60, Pitch: -2, Dur: 0.45, Vol: 0.84}},
		chordAt(62, 8.0, 0.76, -12, 0),
	)
	// Lead vocal hook (dropped one octave from the MIDI's A5 register).
	vocalSection := []Hit{
		{Step: 0, Pitch: 12, Dur: 0.54, Vol: 0.83}, {Step: 3, Pitch: 10, Dur: 0.85, Vol: 0.76},
		{Step: 6, Pitch: 7, Dur: 0.71, Vol: 0.72}, {Step: 9, Pitch: 5, Dur: 0.17, Vol: 0.69},
		{Step: 10, Pitch: 3, Dur: 0.18, Vol: 0.66}, {Step: 11, Pitch: 0, Dur: 0.23, Vol: 0.65},
		{Step: 12, Pitch: 5, Dur: 0.72, Vol: 0.77}, {Step: 15, Pitch: 3, Dur: 1.24, Vol: 0.65},
		{Step: 48, Pitch: 7, Dur: 0.79, Vol: 0.83}, {Step: 52, Pitch: 7, Dur: 0.74, Vol: 0.78},
		{Step: 56, Pitch: 5, Dur: 0.83, Vol: 0.8}, {Step: 60, Pitch: 3, Dur: 0.49, Vol: 0.76},
		{Step: 62, Pitch: 5, Dur: 0.8, Vol: 0.8}, {Step: 66, Pitch: 3, Dur: 0.91, Vol: 0.74},
		{Step: 70, Pitch: 12, Dur: 0.93, Vol: 0.81}, {Step: 74, Pitch: 3, Dur: 0.93, Vol: 0.69},
		{Step: 114, Pitch: 0, Dur: 0.81, Vol: 0.72}, {Step: 118, Pitch: 3, Dur: 0.81, Vol: 0.82},
		{Step: 122, Pitch: 5, Dur: 0.8, Vol: 0.82}, {Step: 126, Pitch: 7, Dur: 1.24, Vol: 0.83},
	}
	// I-Threes chant triads (C-major-over-A = Am7 color), rolled.
	chantSection := concat(
		chordAt(0, 0.95, 0.6, 7, 10, 15),
		chordAt(4, 0.5, 0.59, 10, 15),
		chordAt(6, 1.2, 0.58, 7, 10, 15),
		chordAt(48, 0.95, 0.59, 7, 10, 15),
		chordAt(52, 0.93, 0.6, 10, 15),
		chordAt(56, 0.9, 0.56, 5, 8, 12),
		chordAt(60, 0.5, 0.55, 7, 10),
		chordAt(62, 0.8, 0.54, 5, 8, 12),
		chordAt(66, 2.0, 0.55, 3, 7, 10),
	)
	// Zap Pow horns (tpt / alto / tbn top-to-bottom): the bar-2/3 stab and the
	// full "Exodus!" answer theme (bars 6–7 of the section).
	tptSection := []Hit{
		{Step: 28, Pitch: 7, Dur: 1.0, Vol: 0.71}, {Step: 32, Pitch: 12, Dur: 2.5, Vol: 0.69},
		{Step: 84, Pitch: 12, Dur: 0.4, Vol: 0.76}, {Step: 86, Pitch: 10, Dur: 0.4, Vol: 0.58},
		{Step: 90, Pitch: 12, Dur: 0.45, Vol: 0.76}, {Step: 92, Pitch: 10, Dur: 0.35, Vol: 0.58},
		{Step: 94, Pitch: 7, Dur: 0.2, Vol: 0.66}, {Step: 96, Pitch: 10, Dur: 1.0, Vol: 0.72},
		{Step: 100, Pitch: 12, Dur: 0.5, Vol: 0.68},
	}
	altoSection := []Hit{
		{Step: 28, Pitch: 0, Dur: 1.0, Vol: 0.73}, {Step: 32, Pitch: 3, Dur: 2.5, Vol: 0.71},
		{Step: 84, Pitch: 3, Dur: 0.4, Vol: 0.87}, {Step: 86, Pitch: 2, Dur: 0.4, Vol: 0.71},
		{Step: 90, Pitch: 3, Dur: 0.45, Vol: 0.9}, {Step: 92, Pitch: 2, Dur: 0.35, Vol: 0.71},
		{Step: 94, Pitch: -2, Dur: 0.2, Vol: 0.71}, {Step: 96, Pitch: 2, Dur: 1.0, Vol: 0.8},
		{Step: 100, Pitch: 3, Dur: 0.5, Vol: 0.85},
	}
	tbnSection := []Hit{
		{Step: 28, Pitch: 3, Dur: 1.0, Vol: 0.84}, {Step: 32, Pitch: 7, Dur: 2.5, Vol: 0.79},
		{Step: 84, Pitch: 7, Dur: 0.4, Vol: 0.97}, {Step: 86, Pitch: 5, Dur: 0.4, Vol: 0.73},
		{Step: 90, Pitch: 7, Dur: 0.45, Vol: 0.95}, {Step: 92, Pitch: 5, Dur: 0.35, Vol: 0.74},
		{Step: 94, Pitch: 2, Dur: 0.2, Vol: 0.73}, {Step: 96, Pitch: 5, Dur: 1.0, Vol: 0.74},
		{Step: 100, Pitch: 7, Dur: 0.5, Vol: 0.82},
	}
	// Carlton Barrett: kick on 1 & 3 (this MIDI's funkier reading) + the
	// one-drop cross-stick on 3 (prose-sourced convention, labeled idiomatic).
	hatBar := []Hit{
		{Step: 0, Vol: 0.62}, {Step: 2, Vol: 0.52}, {Step: 4, Vol: 0.81}, {Step: 6, Vol: 0.45},
		{Step: 8, Vol: 0.61}, {Step: 10, Vol: 0.4}, {Step: 12, Vol: 0.81}, {Step: 14, Vol: 0.68},
	}
	tambBar := []Hit{
		{Step: 0, Vol: 0.46}, {Step: 2, Vol: 0.25}, {Step: 4, Vol: 0.44}, {Step: 6, Vol: 0.28},
		{Step: 8, Vol: 0.47}, {Step: 10, Vol: 0.25}, {Step: 12, Vol: 0.45}, {Step: 14, Vol: 0.28},
	}
	// Seeco's bongo chatter (densest at 2, 6–7, 14) + sparse conga answers.
	bongoBar := []Hit{
		{Step: 2, Pitch: 5, Dur: 0.2, Vol: 0.55}, {Step: 6, Pitch: 5, Dur: 0.2, Vol: 0.7},
		{Step: 7, Pitch: 3, Dur: 0.2, Vol: 0.5}, {Step: 14, Pitch: 5, Dur: 0.2, Vol: 0.65},
	}
	// Timbale-style crescendo fill closing each 8-bar section.
	fill := []Hit{
		{Step: 8, Pitch: 5, Dur: 0.2, Vol: 0.35}, {Step: 9, Pitch: 5, Dur: 0.2, Vol: 0.44},
		{Step: 10, Pitch: 5, Dur: 0.2, Vol: 0.49}, {Step: 11, Pitch: 5, Dur: 0.2, Vol: 0.58},
		{Step: 12, Pitch: 5, Dur: 0.25, Vol: 0.87},
	}
	return Showcase{
		Stem: "marley-exodus", BPM: 132, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "bass-guitar", Name: "Bass", Volume: 0.9},
			{ID: "guitar-electric", Name: "Skank", Volume: 0.55, Pan: 0.25, ReverbSend: 0.08},
			{ID: "guitar-electric-neck", Name: "Triads", Volume: 0.42, Pan: -0.3, ReverbSend: 0.1},
			{ID: "fm-pluck", Name: "Bubble", Volume: 0.5, Pan: 0.15},
			{ID: "piano-grand", Name: "Piano", Volume: 0.55, Pan: -0.15, ReverbSend: 0.1},
			{ID: "sax", Name: "Lead Voc", Volume: 0.68, ReverbSend: 0.16},
			{ID: "ensemble-lead", Name: "I-Threes", Volume: 0.5, Pan: 0.2, ReverbSend: 0.22},
			{ID: "trumpet", Name: "Trumpet", Volume: 0.55, Pan: 0.1, ReverbSend: 0.12},
			{ID: "trumpet-mellow", Name: "Alto", Volume: 0.45, Pan: 0.25, ReverbSend: 0.12},
			{ID: "french-horn", Name: "Trombone", Volume: 0.45, Pan: -0.2, ReverbSend: 0.12},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.9},
			{ID: "sidestick", Name: "Rim", Volume: 0.7},
			{ID: "hihat", Name: "Hihat", Volume: 0.5},
			{ID: "shaker", Name: "Tamb", Volume: 0.42, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga-open", Name: "Bongo", Volume: 0.5, Pan: 0.35},
			{ID: "tom-1", Name: "Timbale", Volume: 0.6, Pan: -0.1},
		},
		Rows: []RowSpec{
			// +12: bass-guitar renders one octave down; re-encoded to true record pitch.
			{Inst: "bass-guitar", Hits: transposeHits(concat(bassCycle, at(64, bassCycle), at(128, bassCycle), at(192, bassCycle)), 12)},
			{Inst: "guitar-electric", Hits: laidBack(ostinato2(16, skankCell), 0.05)},
			{Inst: "guitar-electric-neck", Hits: ostinato2(16, triadCell)},
			{Inst: "fm-pluck", Hits: laidBack(ostinato2(16, bubbleCell), 0.13)},
			{Inst: "piano-grand", Hits: concat(pianoSection, at(128, pianoSection))},
			{Inst: "sax", Hits: concat(vocalSection, at(128, vocalSection))},
			{Inst: "ensemble-lead", Hits: concat(chantSection, at(128, chantSection))},
			{Inst: "trumpet", Hits: concat(tptSection, at(128, tptSection))},
			{Inst: "trumpet-mellow", Hits: concat(altoSection, at(128, altoSection))},
			{Inst: "french-horn", Hits: concat(tbnSection, at(128, tbnSection))},
			{Inst: "kick-acoustic", Hits: laidBack(ostinato(16, []Hit{{Step: 0, Vol: 0.85}, {Step: 8, Vol: 0.85}}), 0.05)},
			{Inst: "sidestick", Hits: ostinato(16, []Hit{{Step: 8, Vol: 0.75}})},
			{Inst: "hihat", Hits: ostinato(16, hatBar)},
			{Inst: "shaker", Hits: ostinato(16, tambBar)},
			{Inst: "conga-open", Hits: ostinato(16, bongoBar)},
			{Inst: "tom-1", Hits: concat(at(112, fill), at(240, fill))},
		},
	}
}
