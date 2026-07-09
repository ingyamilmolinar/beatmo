package main

// Marc Anthony — "Vivir Mi Vida" (2013). C minor, 105 BPM, Cm–Ab–Eb–Bb loop
// (Cifra Club). Hook pitches from recorder-pedagogy solfège (transposed +3 to
// concert Cm); every salsa section pattern (cáscara, conga marcha + ponche,
// bongó→campana, güiro, anticipated bass tumbao, all-offbeat piano guajeo,
// horn punches, abanico) sourced from the same artist's salsa-band MIDI
// ("Vivir lo Nuestro", bitmidi 71935) + rhythmnotes/scphillips pedagogy.
// Dossier: salsa-vivir_dossier.md.
//
// KEY FIX vs the old template: the C4↔Eb4 oscillation is the VERSE (sourced,
// flute lines 5-8); the true chorus hook RISES C-Eb-G-Ab. Both are now here.
//
// 16 bars: chorus (8: hook lines 1-4 + coro answers + horn punches + kick) →
// verse (4, thin: oscillation + the pre-chorus descent, no kick) → mambo (4:
// campana + timbale bell + horn moñas + abanico into the wrap).
func vivirShowcase() Showcase {
	// Hook lines (one per 2-bar cycle; rhythm labeled idiomatic — pitch order
	// sourced). L1 rises to Ab; L2 answers "la la la la"; L3 variant; L4=L2.
	l1 := []Hit{
		{Step: 0, Pitch: 3, Dur: 0.5, Vol: 0.79}, {Step: 2, Pitch: 6, Dur: 0.5, Vol: 0.76},
		{Step: 4, Pitch: 10, Dur: 0.75, Vol: 0.82}, {Step: 8, Pitch: 11, Dur: 1.5, Vol: 0.87},
		{Step: 16, Pitch: 3, Dur: 0.5, Vol: 0.79}, {Step: 18, Pitch: 6, Dur: 0.5, Vol: 0.76},
		{Step: 20, Pitch: 11, Dur: 0.75, Vol: 0.85}, {Step: 24, Pitch: 10, Dur: 1.75, Vol: 0.88},
	}
	l2 := []Hit{
		{Step: 0, Pitch: 3, Dur: 0.5, Vol: 0.77}, {Step: 2, Pitch: 6, Dur: 0.5, Vol: 0.76},
		{Step: 4, Pitch: 10, Dur: 1.0, Vol: 0.82}, {Step: 12, Pitch: 8, Dur: 0.5, Vol: 0.83},
		{Step: 16, Pitch: 8, Dur: 0.5, Vol: 0.85}, {Step: 20, Pitch: 8, Dur: 0.5, Vol: 0.83},
		{Step: 24, Pitch: 6, Dur: 0.5, Vol: 0.8}, {Step: 26, Pitch: 5, Dur: 0.5, Vol: 0.76},
		{Step: 28, Pitch: 3, Dur: 1.0, Vol: 0.79},
	}
	l3 := []Hit{
		{Step: 0, Pitch: 3, Dur: 0.5, Vol: 0.77}, {Step: 2, Pitch: 6, Dur: 0.5, Vol: 0.76},
		{Step: 4, Pitch: 10, Dur: 1.0, Vol: 0.82}, {Step: 12, Pitch: 11, Dur: 0.5, Vol: 0.84},
		{Step: 16, Pitch: 11, Dur: 0.5, Vol: 0.85}, {Step: 20, Pitch: 11, Dur: 0.5, Vol: 0.83},
		{Step: 24, Pitch: 8, Dur: 0.5, Vol: 0.81}, {Step: 28, Pitch: 10, Dur: 1.0, Vol: 0.83},
	}
	// Verse oscillation (sourced line 5) + pre-chorus descent (sourced line 7).
	verseOsc := []Hit{
		{Step: 0, Pitch: 6, Dur: 0.5, Vol: 0.6}, {Step: 2, Pitch: 6, Dur: 0.5, Vol: 0.58},
		{Step: 4, Pitch: 3, Dur: 0.5, Vol: 0.6}, {Step: 8, Pitch: 6, Dur: 0.5, Vol: 0.6},
		{Step: 10, Pitch: 6, Dur: 0.5, Vol: 0.58}, {Step: 12, Pitch: 3, Dur: 0.5, Vol: 0.6},
		{Step: 16, Pitch: 6, Dur: 0.5, Vol: 0.6}, {Step: 18, Pitch: 6, Dur: 0.75, Vol: 0.58},
	}
	descent := []Hit{
		{Step: 0, Pitch: 3, Dur: 0.75, Vol: 0.64}, {Step: 4, Pitch: 3, Dur: 0.75, Vol: 0.62},
		{Step: 8, Pitch: 5, Dur: 0.75, Vol: 0.64}, {Step: 12, Pitch: 6, Dur: 0.75, Vol: 0.66},
		{Step: 16, Pitch: 5, Dur: 0.75, Vol: 0.64}, {Step: 20, Pitch: 3, Dur: 0.75, Vol: 0.62},
		{Step: 24, Pitch: 1, Dur: 0.75, Vol: 0.6}, {Step: 28, Pitch: -1, Dur: 1.0, Vol: 0.62},
	}
	// All-offbeat piano guajeo (rhythm sourced; Cm-loop pitches adapted).
	montuno4 := concat(
		chordAt(2, 0.3, 0.72, 10, 15, 18), chordAt(6, 0.3, 0.7, 10, 15, 18), chordAt(10, 0.3, 0.72, 10, 15, 18),
		chordAt(14, 0.3, 0.78, 3, 15),
		chordAt(18, 0.3, 0.72, 11, 15, 18), chordAt(22, 0.3, 0.7, 11, 15, 18), chordAt(26, 0.3, 0.72, 11, 15, 18),
		chordAt(30, 0.3, 0.78, 18, 23),
		chordAt(34, 0.3, 0.72, 10, 13, 18), chordAt(38, 0.3, 0.7, 10, 13, 18), chordAt(42, 0.3, 0.72, 10, 13, 18),
		chordAt(46, 0.3, 0.78, 13, 25),
		chordAt(50, 0.3, 0.72, 8, 13, 17), chordAt(54, 0.3, 0.7, 8, 13, 17), chordAt(58, 0.3, 0.72, 8, 13, 17),
		chordAt(62, 0.3, 0.78, 15, 18),
	)
	// Anticipated bass tumbao (+12 octave policy): &-of-2 pickup, fifth on 3,
	// NEXT chord's root on 4 held over the barline; downbeats silent.
	bass4 := []Hit{
		{Step: 6, Pitch: -9, Dur: 0.5, Vol: 0.88}, {Step: 8, Pitch: -14, Dur: 0.75, Vol: 0.76},
		{Step: 12, Pitch: -13, Dur: 2.0, Vol: 0.79},
		{Step: 22, Pitch: -13, Dur: 0.5, Vol: 0.87}, {Step: 24, Pitch: -18, Dur: 0.75, Vol: 0.75},
		{Step: 28, Pitch: -18, Dur: 2.0, Vol: 0.77},
		{Step: 38, Pitch: -6, Dur: 0.5, Vol: 0.88}, {Step: 40, Pitch: -11, Dur: 0.75, Vol: 0.76},
		{Step: 44, Pitch: -11, Dur: 2.0, Vol: 0.79},
		{Step: 54, Pitch: -11, Dur: 0.5, Vol: 0.87}, {Step: 56, Pitch: -16, Dur: 0.75, Vol: 0.75},
		{Step: 60, Pitch: -9, Dur: 2.0, Vol: 0.8},
	}
	// Horn punches (rhythm/shape sourced; Cm adaptation labeled).
	tptPunch := concat(
		chordAt(0, 0.5, 0.95, 15, 22),
		chordAt(2, 0.4, 0.9, 18, 22),
		chordAt(10, 0.4, 0.9, 18, 22),
		chordAt(18, 0.4, 0.92, 17, 20),
		chordAt(26, 0.4, 0.92, 17, 20),
		[]Hit{{Step: 30, Pitch: 11, Dur: 0.25, Vol: 0.83}, {Step: 31, Pitch: 13, Dur: 0.25, Vol: 0.85}},
	)
	tbnPunch := concat(
		chordAt(0, 0.5, 0.85, -2, 3),
		chordAt(2, 0.4, 0.8, 6, 10),
		chordAt(10, 0.4, 0.8, 6, 10),
		chordAt(18, 0.4, 0.82, 5, 8),
		chordAt(26, 0.4, 0.82, 5, 8),
		[]Hit{{Step: 30, Pitch: -1, Dur: 0.25, Vol: 0.75}},
	)
	// Percussion cycles (2-bar / 32-step; sourced from the band MIDI).
	clave := []Hit{{Step: 4, Vol: 0.83}, {Step: 8, Vol: 0.83}, {Step: 16, Vol: 0.87}, {Step: 22, Vol: 0.83}, {Step: 28, Vol: 0.87}}
	cascara := []Hit{
		{Step: 0, Vol: 0.72}, {Step: 4, Vol: 0.58}, {Step: 6, Vol: 0.6}, {Step: 10, Vol: 0.58}, {Step: 14, Vol: 0.62},
		{Step: 16, Vol: 0.65}, {Step: 20, Vol: 0.58}, {Step: 24, Vol: 0.6}, {Step: 26, Vol: 0.71}, {Step: 30, Vol: 0.71},
	}
	guiro := []Hit{
		{Step: 0, Vol: 0.68, Dur: 0.7}, {Step: 4, Vol: 0.55}, {Step: 6, Vol: 0.52},
		{Step: 8, Vol: 0.6, Dur: 0.7}, {Step: 12, Vol: 0.55}, {Step: 14, Vol: 0.52},
		{Step: 16, Vol: 0.66, Dur: 0.7}, {Step: 20, Vol: 0.55}, {Step: 22, Vol: 0.52},
		{Step: 24, Vol: 0.6, Dur: 0.7}, {Step: 28, Vol: 0.55}, {Step: 30, Vol: 0.52},
	}
	marcha := []Hit{
		{Step: 2, Dur: 0.2, Vol: 0.3}, {Step: 4, Dur: 0.2, Vol: 0.7}, {Step: 6, Dur: 0.2, Vol: 0.3},
		{Step: 10, Dur: 0.2, Vol: 0.32}, {Step: 16, Dur: 0.2, Vol: 0.3}, {Step: 18, Dur: 0.2, Vol: 0.3},
		{Step: 20, Dur: 0.2, Vol: 0.7}, {Step: 22, Dur: 0.2, Vol: 0.3}, {Step: 26, Dur: 0.2, Vol: 0.32},
	}
	ponche := []Hit{
		{Step: 12, Dur: 0.35, Vol: 0.72}, {Step: 14, Dur: 0.35, Vol: 0.88},
		{Step: 28, Dur: 0.35, Vol: 0.75}, {Step: 30, Dur: 0.35, Vol: 0.9},
	}
	bongoHi := []Hit{{Step: 2, Pitch: 0, Dur: 0.2, Vol: 0.55}, {Step: 8, Pitch: 0, Dur: 0.2, Vol: 0.75}, {Step: 20, Pitch: 0, Dur: 0.2, Vol: 0.62}, {Step: 26, Pitch: 0, Dur: 0.2, Vol: 0.7}, {Step: 30, Pitch: 0, Dur: 0.2, Vol: 0.6}}
	bongoLo := []Hit{{Step: 12, Pitch: 0, Dur: 0.25, Vol: 0.68}, {Step: 28, Pitch: 0, Dur: 0.25, Vol: 0.72}}
	campana := []Hit{
		{Step: 0, Vol: 0.75}, {Step: 4, Vol: 0.68}, {Step: 8, Vol: 0.7}, {Step: 10, Vol: 0.66},
		{Step: 12, Vol: 0.7}, {Step: 14, Vol: 0.95}, {Step: 18, Vol: 0.68}, {Step: 20, Vol: 0.7},
		{Step: 22, Vol: 0.95}, {Step: 24, Vol: 0.7}, {Step: 28, Vol: 0.95}, {Step: 30, Vol: 0.68},
	}
	timbBell := []Hit{{Step: 0, Vol: 0.55, Dur: 0.9}, {Step: 8, Vol: 0.5, Dur: 0.9}, {Step: 16, Vol: 0.55, Dur: 0.9}, {Step: 24, Vol: 0.5, Dur: 0.9}}
	kickCyc := []Hit{{Step: 0, Vol: 0.82}, {Step: 8, Vol: 0.78}, {Step: 16, Vol: 0.82}, {Step: 24, Vol: 0.78}}
	return Showcase{
		Stem: "salsa-vivir", BPM: 105, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "sax", Name: "Voz", Volume: 0.68, ReverbSend: 0.16},
			{ID: "ensemble-lead", Name: "Coro", Volume: 0.55, Pan: 0.2, ReverbSend: 0.2},
			{ID: "piano-grand", Name: "Montuno", Volume: 0.62, Pan: -0.25, ReverbSend: 0.1},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.88},
			{ID: "trumpet", Name: "Trumpet", Volume: 0.6, Pan: 0.2, ReverbSend: 0.12},
			{ID: "french-horn-loud", Name: "Trombone", Volume: 0.55, Pan: 0.3, ReverbSend: 0.12},
			{ID: "sidestick", Name: "Clave", Volume: 0.85},
			{ID: "rimshot", Name: "Cáscara", Volume: 0.6, Pan: 0.25},
			{ID: "shaker", Name: "Güiro", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga", Name: "Marcha", Volume: 0.68, Pan: -0.25, SynthParams: map[string]float64{"decay": 0.5}},
			{ID: "conga-open", Name: "Ponche", Volume: 0.85, Pan: -0.25},
			{ID: "high-tom-organic", Name: "Bongó", Volume: 0.5, Pan: 0.35},
			{ID: "conga-tumba", Name: "Bongó Lo", Volume: 0.5, Pan: 0.35},
			{ID: "cowbell", Name: "Campana", Volume: 0.5, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "cowbell-1", Name: "Timb Bell", Volume: 0.45, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.85},
			{ID: "snare-1", Name: "Abanico", Volume: 0.6},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Lead: chorus hook L1-L4 → verse oscillation → pre-chorus descent.
			{Inst: "sax", Hits: concat(
				l1, at(32, l2), at(64, l3), at(96, l2),
				at(128, verseOsc), at(160, descent),
				at(192, l1), at(224, l3),
			)},
			// Coro answers the "la la la la" lines (3rd-below doubling labeled
			// idiomatic; here unison at coro weight).
			{Inst: "ensemble-lead", Hits: concat(
				at(32, []Hit{{Step: 12, Pitch: 8, Dur: 0.5, Vol: 0.6}, {Step: 16, Pitch: 8, Dur: 0.5, Vol: 0.62}, {Step: 20, Pitch: 8, Dur: 0.5, Vol: 0.6}, {Step: 24, Pitch: 6, Dur: 0.5, Vol: 0.58}, {Step: 26, Pitch: 5, Dur: 0.5, Vol: 0.55}, {Step: 28, Pitch: 3, Dur: 1.0, Vol: 0.58}}),
				at(96, []Hit{{Step: 12, Pitch: 8, Dur: 0.5, Vol: 0.6}, {Step: 16, Pitch: 8, Dur: 0.5, Vol: 0.62}, {Step: 20, Pitch: 8, Dur: 0.5, Vol: 0.6}, {Step: 24, Pitch: 6, Dur: 0.5, Vol: 0.58}, {Step: 26, Pitch: 5, Dur: 0.5, Vol: 0.55}, {Step: 28, Pitch: 3, Dur: 1.0, Vol: 0.58}}),
			)},
			{Inst: "piano-grand", Hits: concat(montuno4, at(64, montuno4), at(128, montuno4), at(192, montuno4))},
			{Inst: "bass-guitar", Hits: concat(bass4, at(64, bass4), at(128, bass4), at(192, bass4))},
			// Horns: chorus cycle 1 + the mambo moñas.
			{Inst: "trumpet", Hits: concat(tptPunch, at(192, tptPunch), at(224, tptPunch))},
			{Inst: "french-horn-loud", Hits: concat(tbnPunch, at(192, tbnPunch), at(224, tbnPunch))},
			{Inst: "sidestick", Hits: ostinato2(16, clave)},
			// Cáscara in chorus+verse; campana takes over in the mambo.
			{Inst: "rimshot", Hits: ostinato2(12, cascara)},
			{Inst: "shaker", Hits: ostinato2(16, guiro)},
			{Inst: "conga", Hits: ostinato2(16, marcha)},
			{Inst: "conga-open", Hits: ostinato2(16, ponche)},
			{Inst: "high-tom-organic", Hits: ostinato2(16, bongoHi)},
			{Inst: "conga-tumba", Hits: ostinato2(16, bongoLo)},
			{Inst: "cowbell", Hits: at(192, ostinato2(4, campana))},
			{Inst: "cowbell-1", Hits: at(192, ostinato2(4, timbBell))},
			// Kick: chorus + mambo (drops out for the thin verse).
			{Inst: "kick-acoustic", Hits: concat(ostinato2(8, kickCyc), at(192, ostinato2(4, kickCyc)))},
			// Abanico crescendo into the loop wrap.
			{Inst: "snare-1", Hits: []Hit{{Step: 248, Vol: 0.42}, {Step: 252, Vol: 0.55}, {Step: 254, Vol: 0.68}, {Step: 255, Vol: 0.8}}},
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.5}, {Step: 192, Vol: 0.5}}},
		},
	}
}
