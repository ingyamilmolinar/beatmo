package main

// B.B. King — "The Thrill Is Gone" (Completely Well, 1969). B minor 12-bar
// blues (Bm×4 | Em×2 Bm×2 | Gmaj7 F#7 Bm×2), 88 BPM exact. Note data from
// BitMidi 102720 (incl. 1388 pitch-bend events annotating BB's actual bends);
// personnel/strings history from Wikipedia + mixonline/guitarworld snippets
// (Szymczyk production; Lovelle/Jemmott/Harris/McCracken; DeCoteaux 12-piece
// strings overdub). Dossier: bbking-thrill-is-gone_dossier.md.
//
// 12 bars = the iconic intro chorus: Lucille's opening solo over the full
// form, e-piano answering in the gaps, McCracken's ringing comp, Jemmott's
// chromatic walk, the DeCoteaux string pad + celli ostinato (from the
// phase-aligned chorus 3), and the sourced turnaround fill that hands the
// loop back to bar 1.
//
// GROOVE: straight 16ths (measured — zero shuffle); the simmer is the pushed
// 16th kick before beat 3 (encoded rush 0.09) and 16th-level fills.
func thrillShowcase() Showcase {
	lead := []Hit{
		{Step: 0, Pitch: 14, Dur: 2.94, Vol: 0.8}, {Step: 12, Pitch: 14, Dur: 0.37, Vol: 0.79},
		{Step: 16, Pitch: 17, Dur: 0.4, Vol: 0.79}, {Step: 18, Pitch: 14, Dur: 0.29, Vol: 0.7},
		{Step: 20, Pitch: 12, Dur: 0.3, Vol: 0.76}, {Step: 22, Pitch: 8, Dur: 0.42, Vol: 0.79},
		{Step: 24, Pitch: 5, Dur: 0.38, Vol: 0.68}, {Step: 26, Pitch: 5, Dur: 0.4, Vol: 0.65},
		{Step: 28, Pitch: 7, Dur: 0.48, Vol: 0.78}, {Step: 30, Pitch: 2, Dur: 0.2, Vol: 0.6},
		{Step: 32, Pitch: 7, Dur: 0.38, Vol: 0.74}, {Step: 34, Pitch: 2, Dur: 0.25, Vol: 0.63},
		{Step: 36, Pitch: 2, Dur: 0.21, Vol: 0.67}, {Step: 38, Pitch: 2, Dur: 1.37, Vol: 0.74},
		{Step: 44, Pitch: 5, Dur: 0.29, Vol: 0.76}, {Step: 46, Pitch: 7, Dur: 1.46, Vol: 0.81},
		{Step: 52, Pitch: 9, Dur: 0.28, Vol: 0.72}, {Step: 54, Pitch: 14, Dur: 0.27, Vol: 0.82},
		{Step: 64, Pitch: 17, Dur: 1.38, Vol: 0.8}, {Step: 70, Pitch: 17, Dur: 0.33, Vol: 0.73},
		{Step: 72, Pitch: 14, Dur: 1.97, Vol: 0.72}, {Step: 84, Pitch: 14, Dur: 0.25, Vol: 0.62},
		{Step: 86, Pitch: 17, Dur: 0.27, Vol: 0.69}, {Step: 88, Pitch: 18, Dur: 0.24, Vol: 0.73},
		{Step: 90, Pitch: 19, Dur: 0.3, Vol: 0.76}, {Step: 92, Pitch: 19, Dur: 0.37, Vol: 0.78},
		{Step: 94, Pitch: 19, Dur: 0.31, Vol: 0.76}, {Step: 96, Pitch: 17, Dur: 1.85, Vol: 0.74},
		{Step: 104, Pitch: 14, Dur: 0.99, Vol: 0.74}, {Step: 108, Pitch: 14, Dur: 0.1, Vol: 0.56},
		{Step: 110, Pitch: 17, Dur: 2.34, Vol: 0.72}, {Step: 120, Pitch: 14, Dur: 0.18, Vol: 0.63},
		{Step: 122, Pitch: 17, Dur: 0.17, Vol: 0.42}, {Step: 124, Pitch: 14, Dur: 0.94, Vol: 0.75},
		{Step: 128, Pitch: 17, Dur: 0.47, Vol: 0.8}, {Step: 130, Pitch: 19, Dur: 0.22, Vol: 0.8},
		{Step: 132, Pitch: 19, Dur: 0.47, Vol: 0.8}, {Step: 134, Pitch: 17, Dur: 0.22, Vol: 0.73},
		{Step: 136, Pitch: 14, Dur: 1.02, Vol: 0.8}, {Step: 140, Pitch: 14, Dur: 0.28, Vol: 0.81},
		{Step: 148, Pitch: 19, Dur: 0.25, Vol: 0.78}, {Step: 150, Pitch: 20, Dur: 0.42, Vol: 0.8},
		{Step: 152, Pitch: 17, Dur: 0.26, Vol: 0.79}, {Step: 156, Pitch: 19, Dur: 0.43, Vol: 0.68},
		{Step: 158, Pitch: 14, Dur: 0.14, Vol: 0.69}, {Step: 160, Pitch: 17, Dur: 0.47, Vol: 0.8},
		{Step: 162, Pitch: 14, Dur: 0.15, Vol: 0.55}, {Step: 166, Pitch: 17, Dur: 0.43, Vol: 0.73},
		{Step: 168, Pitch: 14, Dur: 0.98, Vol: 0.72}, {Step: 172, Pitch: 17, Dur: 0.94, Vol: 0.74},
		{Step: 176, Pitch: 14, Dur: 0.26, Vol: 0.72}, {Step: 188, Pitch: 14, Dur: 0.16, Vol: 0.72},
	}
	// Jemmott's walk (+12 octave policy): Bm cell = B B | F(chromatic) F# F# | A#.
	bmCell := func(b int) []Hit {
		return []Hit{
			{Step: b, Pitch: -10, Dur: 0.44, Vol: 0.8}, {Step: b + 2, Pitch: -10, Dur: 0.42, Vol: 0.83},
			{Step: b + 6, Pitch: -16, Dur: 0.5, Vol: 0.82}, {Step: b + 8, Pitch: -15, Dur: 0.45, Vol: 0.8},
			{Step: b + 10, Pitch: -15, Dur: 0.45, Vol: 0.78}, {Step: b + 14, Pitch: -11, Dur: 0.51, Vol: 0.81},
		}
	}
	bass := concat(
		bmCell(0), bmCell(16), bmCell(32),
		[]Hit{ // bar 4: walk-up into Em
			{Step: 48, Pitch: -10, Dur: 0.44, Vol: 0.8}, {Step: 50, Pitch: -10, Dur: 0.42, Vol: 0.82},
			{Step: 54, Pitch: -16, Dur: 0.5, Vol: 0.82}, {Step: 56, Pitch: -15, Dur: 0.45, Vol: 0.8},
			{Step: 58, Pitch: -15, Dur: 0.45, Vol: 0.78}, {Step: 60, Pitch: -10, Dur: 0.45, Vol: 0.85},
			{Step: 62, Pitch: -6, Dur: 0.45, Vol: 0.82},
		},
		[]Hit{ // bars 5–6: Em
			{Step: 64, Pitch: -5, Dur: 0.45, Vol: 0.83}, {Step: 66, Pitch: -5, Dur: 0.45, Vol: 0.84},
			{Step: 70, Pitch: -11, Dur: 0.45, Vol: 0.78}, {Step: 72, Pitch: -10, Dur: 0.45, Vol: 0.79},
			{Step: 74, Pitch: -10, Dur: 0.45, Vol: 0.79}, {Step: 78, Pitch: -6, Dur: 0.45, Vol: 0.8},
			{Step: 80, Pitch: -5, Dur: 0.45, Vol: 0.83}, {Step: 82, Pitch: -5, Dur: 0.45, Vol: 0.85},
			{Step: 86, Pitch: -11, Dur: 0.45, Vol: 0.79}, {Step: 88, Pitch: -10, Dur: 0.45, Vol: 0.8},
			{Step: 90, Pitch: -10, Dur: 0.45, Vol: 0.83}, {Step: 92, Pitch: -7, Dur: 0.45, Vol: 0.83},
			{Step: 94, Pitch: -8, Dur: 0.45, Vol: 0.83},
		},
		bmCell(96),
		[]Hit{ // bar 8: chromatic drop into G
			{Step: 112, Pitch: -10, Dur: 0.44, Vol: 0.8}, {Step: 114, Pitch: -10, Dur: 0.42, Vol: 0.82},
			{Step: 118, Pitch: -16, Dur: 0.5, Vol: 0.82}, {Step: 120, Pitch: -15, Dur: 0.45, Vol: 0.8},
			{Step: 122, Pitch: -15, Dur: 0.45, Vol: 0.78}, {Step: 124, Pitch: -10, Dur: 0.45, Vol: 0.85},
			{Step: 126, Pitch: -12, Dur: 0.45, Vol: 0.85},
		},
		[]Hit{ // bar 9 Gmaj7: root–5–octave
			{Step: 128, Pitch: -14, Dur: 0.45, Vol: 0.81}, {Step: 130, Pitch: -14, Dur: 0.45, Vol: 0.82},
			{Step: 134, Pitch: -7, Dur: 0.45, Vol: 0.83}, {Step: 136, Pitch: -2, Dur: 0.45, Vol: 0.83},
			{Step: 138, Pitch: -2, Dur: 0.45, Vol: 0.83}, {Step: 142, Pitch: -7, Dur: 0.45, Vol: 0.83},
		},
		[]Hit{ // bar 10 F#7 + chromatic climb G#–A#→B
			{Step: 144, Pitch: -3, Dur: 0.45, Vol: 0.82}, {Step: 146, Pitch: -3, Dur: 0.45, Vol: 0.83},
			{Step: 150, Pitch: -8, Dur: 0.45, Vol: 0.82}, {Step: 152, Pitch: -15, Dur: 0.45, Vol: 0.82},
			{Step: 154, Pitch: -15, Dur: 0.45, Vol: 0.81}, {Step: 156, Pitch: -13, Dur: 0.45, Vol: 0.81},
			{Step: 158, Pitch: -11, Dur: 0.45, Vol: 0.85},
		},
		bmCell(160),
		[]Hit{ // bar 12 turnaround: the long G#–A# climb into the wrap
			{Step: 176, Pitch: -10, Dur: 0.44, Vol: 0.82}, {Step: 178, Pitch: -10, Dur: 0.42, Vol: 0.83},
			{Step: 180, Pitch: -15, Dur: 0.45, Vol: 0.82}, {Step: 182, Pitch: -15, Dur: 0.45, Vol: 0.81},
			{Step: 184, Pitch: -13, Dur: 0.84, Vol: 0.84}, {Step: 188, Pitch: -11, Dur: 0.84, Vol: 0.82},
		},
	)
	// Paul Harris e-piano: pads + answer figures + the B4→F#5 climb into G.
	epiano := concat(
		chordAt(0, 3.6, 0.7, 2, 5, 9, 14),
		chordAt(16, 3.4, 0.45, -10, -3),
		[]Hit{{Step: 18, Pitch: 14, Dur: 0.35, Vol: 0.55}},
		chordAt(20, 1.0, 0.7, 9, 14),
		[]Hit{
			{Step: 26, Pitch: 7, Dur: 0.13, Vol: 0.69}, {Step: 27, Pitch: 9, Dur: 0.27, Vol: 0.73},
			{Step: 28, Pitch: 12, Dur: 0.25, Vol: 0.69}, {Step: 29, Pitch: 14, Dur: 0.16, Vol: 0.83},
		},
		chordAt(30, 0.3, 0.8, 6, 7),
		chordAt(32, 2.2, 0.65, -3, 0, 5),
		[]Hit{{Step: 42, Pitch: 0, Dur: 0.12, Vol: 0.79}, {Step: 44, Pitch: 2, Dur: 0.12, Vol: 0.96}},
		chordAt(46, 1.9, 0.75, -3, 5),
		[]Hit{
			{Step: 50, Pitch: 0, Dur: 0.69, Vol: 0.48}, {Step: 56, Pitch: 9, Dur: 0.5, Vol: 1.0},
			{Step: 58, Pitch: 12, Dur: 0.17, Vol: 0.8},
		},
		chordAt(60, 0.9, 0.75, 6, 7),
		chordAt(62, 0.44, 0.69, 0, 5),
		chordAt(64, 3.9, 0.75, -5, -2, 2),
		chordAt(88, 1.3, 0.65, -5, 2),
		[]Hit{
			{Step: 90, Pitch: 10, Dur: 0.34, Vol: 0.87}, {Step: 92, Pitch: 9, Dur: 0.26, Vol: 0.97},
			{Step: 94, Pitch: 7, Dur: 0.48, Vol: 0.94},
		},
		chordAt(96, 3.3, 0.78, -3, 2, 5),
		[]Hit{
			{Step: 112, Pitch: 2, Dur: 0.57, Vol: 0.72}, {Step: 114, Pitch: 7, Dur: 0.17, Vol: 0.75},
			{Step: 116, Pitch: 12, Dur: 0.93, Vol: 0.73}, {Step: 117, Pitch: 7, Dur: 0.2, Vol: 0.74},
			{Step: 118, Pitch: 5, Dur: 0.41, Vol: 0.75}, {Step: 120, Pitch: 14, Dur: 1.2, Vol: 0.91},
			{Step: 122, Pitch: 17, Dur: 0.14, Vol: 0.76}, {Step: 124, Pitch: 19, Dur: 0.33, Vol: 0.84},
			{Step: 126, Pitch: 21, Dur: 0.43, Vol: 0.84},
		},
		chordAt(128, 3.5, 0.75, 5, 9, 14, 19),
		chordAt(144, 2.5, 0.7, 7, 9, 13),
		[]Hit{{Step: 148, Pitch: 21, Dur: 0.12, Vol: 0.91}},
		chordAt(160, 2.8, 0.6, 2, 5, 9, 14),
		[]Hit{
			{Step: 172, Pitch: 19, Dur: 0.23, Vol: 0.97}, {Step: 173, Pitch: 17, Dur: 0.23, Vol: 0.69},
			{Step: 174, Pitch: 14, Dur: 0.34, Vol: 0.69}, {Step: 176, Pitch: 17, Dur: 0.48, Vol: 0.81},
			{Step: 178, Pitch: 14, Dur: 0.27, Vol: 0.79}, {Step: 184, Pitch: 7, Dur: 0.15, Vol: 0.82},
			{Step: 185, Pitch: 8, Dur: 0.15, Vol: 0.86}, {Step: 186, Pitch: 9, Dur: 0.15, Vol: 0.94},
			{Step: 188, Pitch: 7, Dur: 0.99, Vol: 0.82},
		},
		chordAt(190, 0.5, 0.72, 9, 12),
	)
	// McCracken comp: ringing Bm, low runs, the Gmaj7/F#7 fingerpick figures.
	comp := concat(
		chordAt(0, 7.3, 0.6, 5, 9, 14),
		[]Hit{
			{Step: 56, Pitch: -7, Dur: 0.3, Vol: 0.5}, {Step: 57, Pitch: -5, Dur: 0.3, Vol: 0.48},
			{Step: 58, Pitch: -3, Dur: 0.3, Vol: 0.52}, {Step: 60, Pitch: -3, Dur: 0.3, Vol: 0.55},
			{Step: 61, Pitch: -5, Dur: 0.3, Vol: 0.48}, {Step: 62, Pitch: -7, Dur: 0.3, Vol: 0.45},
			{Step: 64, Pitch: -5, Dur: 5.9, Vol: 0.55},
			{Step: 116, Pitch: -5, Dur: 0.25, Vol: 0.45}, {Step: 117, Pitch: -3, Dur: 0.25, Vol: 0.55},
			{Step: 118, Pitch: 0, Dur: 0.3, Vol: 0.68},
		},
		chordAt(120, 0.35, 0.58, -3, 2),
		[]Hit{
			{Step: 122, Pitch: 0, Dur: 0.77, Vol: 0.59}, {Step: 124, Pitch: -3, Dur: 0.77, Vol: 0.48},
			{Step: 130, Pitch: -14, Dur: 0.5, Vol: 0.7}, {Step: 132, Pitch: -3, Dur: 0.5, Vol: 0.72},
			{Step: 134, Pitch: 2, Dur: 0.5, Vol: 0.75}, {Step: 136, Pitch: -14, Dur: 0.5, Vol: 0.7},
			{Step: 138, Pitch: -3, Dur: 0.5, Vol: 0.72}, {Step: 140, Pitch: 2, Dur: 0.5, Vol: 0.75},
			{Step: 142, Pitch: -17, Dur: 0.5, Vol: 0.68},
			{Step: 144, Pitch: -15, Dur: 0.5, Vol: 0.65}, {Step: 146, Pitch: -3, Dur: 0.5, Vol: 0.68},
			{Step: 148, Pitch: 2, Dur: 0.6, Vol: 0.72}, {Step: 152, Pitch: -3, Dur: 0.6, Vol: 0.68},
			{Step: 154, Pitch: 1, Dur: 1.0, Vol: 0.72}, {Step: 158, Pitch: -3, Dur: 0.5, Vol: 0.62},
			{Step: 160, Pitch: -10, Dur: 2.9, Vol: 0.55},
			{Step: 176, Pitch: -7, Dur: 0.5, Vol: 0.6}, {Step: 179, Pitch: -10, Dur: 0.4, Vol: 0.55},
			{Step: 180, Pitch: -7, Dur: 0.5, Vol: 0.58}, {Step: 181, Pitch: -5, Dur: 0.6, Vol: 0.62},
			{Step: 184, Pitch: -5, Dur: 0.15, Vol: 0.5}, {Step: 185, Pitch: -4, Dur: 0.15, Vol: 0.55},
			{Step: 186, Pitch: -3, Dur: 0.6, Vol: 0.65}, {Step: 188, Pitch: 0, Dur: 0.8, Vol: 0.72},
			{Step: 190, Pitch: -3, Dur: 0.5, Vol: 0.55},
		},
	)
	// DeCoteaux string pad (dyads, far back) + celli ostinato (+12: cello
	// renders one octave down).
	pad := concat(
		chordAt(0, 15.8, 0.4, 14, 17),
		chordAt(64, 7.9, 0.4, 10, 14),
		chordAt(96, 7.8, 0.38, 14, 17),
		chordAt(128, 4.0, 0.38, 10, 14),
		chordAt(144, 4.0, 0.36, 13, 16),
		chordAt(160, 7.7, 0.38, 14, 17),
	)
	celliBmOdd := []Hit{
		{Step: 0, Pitch: -10, Dur: 0.2, Vol: 0.55}, {Step: 1, Pitch: -10, Dur: 0.2, Vol: 0.5},
		{Step: 3, Pitch: -10, Dur: 0.2, Vol: 0.5}, {Step: 5, Pitch: -10, Dur: 0.2, Vol: 0.5},
		{Step: 7, Pitch: -12, Dur: 0.2, Vol: 0.52}, {Step: 8, Pitch: -10, Dur: 0.2, Vol: 0.55},
	}
	celliBmEven := concat(celliBmOdd, []Hit{{Step: 10, Pitch: -7, Dur: 0.2, Vol: 0.52}, {Step: 12, Pitch: -10, Dur: 0.2, Vol: 0.52}})
	celli := concat(
		celliBmOdd, at(16, celliBmEven), at(32, celliBmOdd), at(48, celliBmEven),
		[]Hit{
			{Step: 64, Pitch: -5, Dur: 0.2, Vol: 0.55}, {Step: 67, Pitch: -7, Dur: 0.2, Vol: 0.5},
			{Step: 68, Pitch: -5, Dur: 0.2, Vol: 0.52}, {Step: 71, Pitch: -7, Dur: 0.2, Vol: 0.5},
			{Step: 72, Pitch: -5, Dur: 0.2, Vol: 0.52}, {Step: 75, Pitch: -7, Dur: 0.2, Vol: 0.5},
			{Step: 76, Pitch: -5, Dur: 0.2, Vol: 0.52}, {Step: 79, Pitch: -7, Dur: 0.2, Vol: 0.5},
			{Step: 80, Pitch: -5, Dur: 0.2, Vol: 0.55}, {Step: 81, Pitch: -7, Dur: 0.2, Vol: 0.5},
			{Step: 83, Pitch: -10, Dur: 0.2, Vol: 0.5}, {Step: 85, Pitch: -12, Dur: 0.2, Vol: 0.5},
			{Step: 87, Pitch: -14, Dur: 0.2, Vol: 0.5}, {Step: 88, Pitch: -17, Dur: 0.78, Vol: 0.55},
		},
		at(96, celliBmOdd), at(112, celliBmEven),
		[]Hit{{Step: 128, Pitch: -14, Dur: 4.0, Vol: 0.6}, {Step: 144, Pitch: -15, Dur: 3.9, Vol: 0.65}},
		at(160, celliBmOdd), at(176, celliBmEven),
	)
	return Showcase{
		Stem: "bbking-thrill-is-gone", BPM: 88, Subdiv: 16, Bars: 12,
		Insts: []InstSpec{
			{ID: "guitar-electric-neck", Name: "Lucille", Volume: 0.8, ReverbSend: 0.22},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "fm-epiano", Name: "E-Piano", Volume: 0.52, Pan: -0.2, ReverbSend: 0.14},
			{ID: "guitar-electric", Name: "Comp", Volume: 0.45, Pan: 0.25, ReverbSend: 0.16},
			{ID: "violin-ensemble", Name: "Strings", Volume: 0.45, Pan: -0.3, ReverbSend: 0.3},
			{ID: "cello-warm", Name: "Celli", Volume: 0.5, Pan: 0.3, ReverbSend: 0.26},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.85},
			{ID: "snare", Name: "Snare", Volume: 0.85},
			{ID: "hihat", Name: "Hihat", Volume: 0.45},
			{ID: "ride", Name: "Ride", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
			{ID: "tom", Name: "Toms", Volume: 0.68},
			{ID: "tom-1", Name: "Low Tom", Volume: 0.68},
		},
		Rows: []RowSpec{
			{Inst: "guitar-electric-neck", Hits: lead},
			{Inst: "bass-guitar", Hits: bass},
			{Inst: "fm-epiano", Hits: epiano},
			{Inst: "guitar-electric", Hits: comp},
			{Inst: "violin-ensemble", Hits: pad},
			{Inst: "cello-warm", Hits: transposeHits(celli, 12)},
			// Lovelle: main groove bars 1–4 & 9–12, Em variation bars 5–8.
			// The step-7 kick is the pushed 16th (measured ~9% early → rush).
			{Inst: "kick-acoustic", Hits: concat(
				ostinato(4, []Hit{{Step: 0, Vol: 0.75}, {Step: 7, Vol: 0.62, Groove: "rush", GroovePct: 0.09}, {Step: 8, Vol: 0.82}, {Step: 10, Vol: 0.65}}),
				at(64, ostinato(4, []Hit{{Step: 0, Vol: 0.75}, {Step: 2, Vol: 0.62}, {Step: 15, Vol: 0.62, Groove: "rush", GroovePct: 0.09}})),
				at(128, ostinato(4, []Hit{{Step: 0, Vol: 0.75}, {Step: 7, Vol: 0.62, Groove: "rush", GroovePct: 0.09}, {Step: 8, Vol: 0.82}, {Step: 10, Vol: 0.65}})),
			)},
			// Backbeat + the sourced ruff (bar 8) and turnaround (bar 12) fills.
			{Inst: "snare", Hits: concat(
				ostinato(12, []Hit{{Step: 4, Vol: 0.88}, {Step: 12, Vol: 0.9}}),
				[]Hit{
					{Step: 123, Vol: 0.76}, {Step: 125, Vol: 0.8}, {Step: 127, Vol: 0.86},
					{Step: 183, Vol: 0.7}, {Step: 184, Vol: 0.74},
				},
			)},
			{Inst: "hihat", Hits: ostinato(12, []Hit{
				{Step: 0, Vol: 0.42}, {Step: 2, Vol: 0.38}, {Step: 4, Vol: 0.5}, {Step: 6, Vol: 0.38},
				{Step: 8, Vol: 0.42}, {Step: 10, Vol: 0.38}, {Step: 12, Vol: 0.5}, {Step: 14, Vol: 0.38},
			})},
			// Ride marks the form pillars (bar 1, Em, Gmaj7).
			{Inst: "ride", Hits: []Hit{{Step: 0, Vol: 0.6}, {Step: 64, Vol: 0.62}, {Step: 128, Vol: 0.6}}},
			// Sourced tom fills: into Em (bar 4) and the bar-12 turnaround.
			{Inst: "tom", Hits: []Hit{
				{Step: 61, Pitch: 0, Dur: 0.25, Vol: 0.57}, {Step: 62, Pitch: -3, Dur: 0.25, Vol: 0.6},
				{Step: 185, Pitch: 0, Dur: 0.25, Vol: 0.7}, {Step: 186, Pitch: -3, Dur: 0.25, Vol: 0.72},
			}},
			{Inst: "tom-1", Hits: []Hit{{Step: 63, Pitch: 0, Dur: 0.3, Vol: 0.68}, {Step: 190, Pitch: 0, Dur: 0.3, Vol: 0.76}}},
		},
	}
}
