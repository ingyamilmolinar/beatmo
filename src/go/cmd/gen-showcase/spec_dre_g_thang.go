package main

// Dr. Dre ft. Snoop Dogg — "Nuthin' but a 'G' Thang" (1992). B minor, 94 BPM.
// Note data from BitMidi 41193; bass corroborated by BigBassTabs (Wilton
// Felder's line from the Leon Haywood sample, replayed by Colin Wolfe); vamp
// identity Bm7↔C#m7 from ChordU/Chordify; Moog-whistle sound research from
// Syntorial + mudcake G-funk breakdowns. Dossier: dre-g-thang_dossier.md.
//
// 16-bar arc: bars 1–8 the full chorus groove (whistle + bass + EP + wah +
// drums), bars 9–12 verse (whistle rests; string-pad seam fills), bars 13–16
// the hook returns. Whistle dropped one octave from the MIDI's B6 register
// (resampler-safe) — same policy as the previous note-accuracy pass.
//
// GROOVE (measured): drums MPC-straight; backbeat +5ms lazy (delay 0.04);
// bass beat-1 turn slurs EARLY (A1 −11ms, F#1 −43ms → rush 0.08/0.33);
// whistle's resolving F# lands +21ms late (delay 0.17); pad swells in early.
func gThangShowcase() Showcase {
	whistle := []Hit{
		{Step: 0, Pitch: 26, Dur: 0.54, Vol: 0.55}, {Step: 2, Pitch: 24, Dur: 0.53, Vol: 0.63},
		{Step: 4, Pitch: 26, Dur: 0.56, Vol: 0.71}, {Step: 6, Pitch: 28, Dur: 0.56, Vol: 0.55},
		{Step: 8, Pitch: 26, Dur: 0.56, Vol: 0.71}, {Step: 10, Pitch: 24, Dur: 1.09, Vol: 0.71},
		{Step: 14, Pitch: 22, Dur: 0.52, Vol: 0.71}, {Step: 16, Pitch: 21, Dur: 1.03, Vol: 0.71},
		{Step: 20, Pitch: 19, Dur: 0.55, Vol: 0.71},
		// The resolving F# sits back +21ms every phrase (measured).
		{Step: 22, Pitch: 21, Dur: 2.43, Vol: 0.71, Groove: "delay", GroovePct: 0.17},
	}
	// Felder/Wolfe bass: descending turn B–A–F#–B (slurred EARLY into the
	// downbeat — measured rush) alternating with the on-grid B–D–E–F# run.
	bassLoop := []Hit{
		{Step: 0, Pitch: -22, Dur: 0.23, Vol: 0.79},
		{Step: 1, Pitch: -24, Dur: 0.23, Vol: 0.79, Groove: "rush", GroovePct: 0.08},
		{Step: 2, Pitch: -27, Dur: 0.29, Vol: 0.79, Groove: "rush", GroovePct: 0.33},
		{Step: 3, Pitch: -22, Dur: 0.26, Vol: 0.79, Groove: "rush", GroovePct: 0.08},
		{Step: 8, Pitch: -22, Dur: 0.26, Vol: 0.79}, {Step: 9, Pitch: -19, Dur: 0.29, Vol: 0.79},
		{Step: 10, Pitch: -17, Dur: 0.29, Vol: 0.71}, {Step: 11, Pitch: -15, Dur: 0.26, Vol: 0.79},
		{Step: 16, Pitch: -22, Dur: 0.23, Vol: 0.79}, {Step: 17, Pitch: -19, Dur: 0.29, Vol: 0.79},
		{Step: 18, Pitch: -17, Dur: 0.29, Vol: 0.71}, {Step: 19, Pitch: -15, Dur: 0.26, Vol: 0.79},
		{Step: 24, Pitch: -22, Dur: 0.23, Vol: 0.79},
		{Step: 25, Pitch: -24, Dur: 0.23, Vol: 0.79, Groove: "rush", GroovePct: 0.08},
		{Step: 26, Pitch: -27, Dur: 0.41, Vol: 0.79, Groove: "rush", GroovePct: 0.33},
		{Step: 27, Pitch: -22, Dur: 0.26, Vol: 0.79, Groove: "rush", GroovePct: 0.08},
	}
	// EP "plink" stabs answering the backbeat; Bm7/A-color dyad rolls under
	// the MIDI's single top notes (voicings labeled idiomatic in the dossier).
	epLoop := concat(
		chordAt(4, 0.12, 0.55, 12, 14),
		[]Hit{{Step: 6, Pitch: 14, Dur: 0.12, Vol: 0.55}},
		chordAt(20, 0.12, 0.55, 7, 12),
		[]Hit{{Step: 22, Pitch: 12, Dur: 0.12, Vol: 0.55}, {Step: 24, Pitch: 9, Dur: 0.12, Vol: 0.24}},
	)
	// Wah-guitar rocking figure F#→E (one-beat sustains).
	wahLoop := []Hit{
		{Step: 0, Pitch: 9, Dur: 0.99, Vol: 0.63}, {Step: 8, Pitch: 7, Dur: 0.99, Vol: 0.63},
		{Step: 24, Pitch: 9, Dur: 0.99, Vol: 0.63},
	}
	// String-pad seam fill (echoes the whistle an octave below it; swells in
	// early — measured −53ms → rush 0.33).
	padFill := pushed([]Hit{
		{Step: 0, Pitch: 14, Dur: 1.66, Vol: 0.5}, {Step: 6, Pitch: 12, Dur: 0.61, Vol: 0.5},
		{Step: 8, Pitch: 9, Dur: 0.98, Vol: 0.5}, {Step: 12, Pitch: 7, Dur: 0.53, Vol: 0.5},
		{Step: 14, Pitch: 9, Dur: 4.58, Vol: 0.5},
	}, 0.33)
	// MPC drums: rolling G-funk kick (1, &2, 3, &4); bar-B and-of-3 event =
	// open hat + layered acoustic double-kick at step 26.
	kickLoop := []Hit{
		{Step: 0, Vol: 1.0}, {Step: 6, Vol: 0.95}, {Step: 8, Vol: 0.97}, {Step: 14, Vol: 0.95},
		{Step: 16, Vol: 1.0}, {Step: 22, Vol: 0.95}, {Step: 24, Vol: 0.97}, {Step: 26, Vol: 0.87}, {Step: 30, Vol: 0.95},
	}
	hatLoop := []Hit{
		{Step: 0, Vol: 0.55}, {Step: 2, Vol: 0.5}, {Step: 4, Vol: 0.55}, {Step: 6, Vol: 0.5},
		{Step: 8, Vol: 0.55}, {Step: 10, Vol: 0.5}, {Step: 12, Vol: 0.55}, {Step: 14, Vol: 0.5},
		{Step: 16, Vol: 0.55}, {Step: 18, Vol: 0.5}, {Step: 20, Vol: 0.55}, {Step: 22, Vol: 0.5},
		{Step: 24, Vol: 0.55}, {Step: 28, Vol: 0.55}, {Step: 30, Vol: 0.5},
	}
	tambBar := []Hit{
		{Step: 0, Vol: 0.5}, {Step: 1, Vol: 0.32}, {Step: 2, Vol: 0.5}, {Step: 3, Vol: 0.32},
		{Step: 4, Vol: 0.5}, {Step: 5, Vol: 0.32}, {Step: 6, Vol: 0.5}, {Step: 7, Vol: 0.32},
		{Step: 8, Vol: 0.5}, {Step: 9, Vol: 0.32}, {Step: 10, Vol: 0.5}, {Step: 11, Vol: 0.32},
		{Step: 12, Vol: 0.5}, {Step: 13, Vol: 0.32}, {Step: 14, Vol: 0.5}, {Step: 15, Vol: 0.32},
	}
	return Showcase{
		Stem: "dre-g-thang", BPM: 94, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "scifi-lead", Name: "Whistle", Volume: 0.55, ReverbSend: 0.16},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.9},
			{ID: "bass-808", Name: "Sub", Volume: 0.45},
			{ID: "fm-epiano", Name: "EP Stab", Volume: 0.55, Pan: 0.2},
			{ID: "guitar-electric-neck", Name: "Wah Gtr", Volume: 0.48, Pan: -0.25},
			{ID: "viola-pad", Name: "Pad", Volume: 0.42, Pan: 0.15, ReverbSend: 0.2},
			{ID: "kick-deep", Name: "Kick", Volume: 1.0},
			{ID: "kick-acoustic", Name: "Kick 2", Volume: 0.75},
			{ID: "snare", Name: "Snare", Volume: 0.85},
			{ID: "sidestick", Name: "Stick", Volume: 0.55},
			{ID: "hihat", Name: "Hihat", Volume: 0.5},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.68, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "shaker", Name: "Tamb", Volume: 0.4, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "cowbell-1", Name: "Vibraslap", Volume: 0.3},
			{ID: "conga-open", Name: "Conga", Volume: 0.4, Pan: 0.3},
		},
		Rows: []RowSpec{
			// Whistle: chorus bars 1–8 + return bars 13–16 (rests in the verse).
			{Inst: "scifi-lead", Hits: concat(
				whistle, at(32, whistle), at(64, whistle), at(96, whistle),
				at(192, whistle), at(224, whistle),
			)},
			// +12: bass-guitar renders one octave down; re-encoded to true record pitch.
			{Inst: "bass-guitar", Hits: ostinato2(16, transposeHits(bassLoop, 12))},
			// Sub reinforces the B1 roots only (the "rolling" G-funk low end).
			{Inst: "bass-808", Hits: ostinato2(16, []Hit{
				{Step: 0, Pitch: -10, Dur: 0.4, Vol: 0.5}, {Step: 8, Pitch: -10, Dur: 0.4, Vol: 0.45},
				{Step: 16, Pitch: -10, Dur: 0.4, Vol: 0.5}, {Step: 24, Pitch: -10, Dur: 0.4, Vol: 0.45},
			})},
			{Inst: "fm-epiano", Hits: ostinato2(16, epLoop)},
			{Inst: "guitar-electric-neck", Hits: ostinato2(16, wahLoop)},
			// Verse seam fills (bars 9 and 11).
			{Inst: "viola-pad", Hits: concat(at(128, padFill), at(160, padFill))},
			{Inst: "kick-deep", Hits: ostinato2(16, kickLoop)},
			{Inst: "kick-acoustic", Hits: ostinato2(16, []Hit{{Step: 26, Vol: 0.85}})},
			// Backbeat sits +5ms behind the grid (measured — the lazy crack).
			{Inst: "snare", Hits: laidBack(ostinato(16, []Hit{{Step: 4, Vol: 0.85}, {Step: 12, Vol: 0.85}}), 0.04)},
			{Inst: "sidestick", Hits: laidBack(ostinato(16, []Hit{{Step: 4, Vol: 0.55}, {Step: 12, Vol: 0.55}}), 0.04)},
			{Inst: "hihat", Hits: ostinato2(16, hatLoop)},
			{Inst: "hihat-1", Hits: ostinato2(16, []Hit{{Step: 26, Vol: 0.72}})},
			{Inst: "shaker", Hits: ostinato(16, tambBar)},
			// Vibraslap section markers (bars 1 and 9).
			{Inst: "cowbell-1", Hits: []Hit{{Step: 0, Vol: 0.32}, {Step: 2, Vol: 0.2}, {Step: 128, Vol: 0.32}}},
			// Sparse conga answer on the and-of-4 (idiomatic, labeled).
			{Inst: "conga-open", Hits: ostinato2(16, []Hit{{Step: 30, Dur: 0.3, Vol: 0.42}})},
		},
	}
}
