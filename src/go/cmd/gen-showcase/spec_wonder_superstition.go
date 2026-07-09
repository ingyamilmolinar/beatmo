package main

// Stevie Wonder — "Superstition" (Talking Book, 1972). Eb minor, 101 BPM.
// All note data transcribed from BitMidi 97097 (research dossier
// wonder-superstition_dossier.md; prose: theconversation.com 50-year analysis,
// Roland Behind the Beat, ryangabbart horn-chart blog).
//
// 16-bar arc: bars 1–4 the solo drum-break intro (+ clav pickup), bars 5–8 the
// full three-clavinet stack + Moog bass instrumental hook, bars 9–12 verse 1
// (vocal substitute enters), bars 13–16 chorus (horn stabs answer the vocal).
//
// GROOVE: the MIDI encodes exact 2:1 triplet 16th swing — every off-16th in
// bass/clavs/hat pushes is delayed by 1/3 of a step. Encoded literally via
// swing16(…, 0.33); the 8th grid (kick/snare/hat 8ths) stays straight.
func superstitionShowcase() Showcase {
	clav1Main := []Hit{
		{Step: 57, Pitch: -13, Dur: 0.20, Vol: 0.81}, {Step: 58, Pitch: -11, Dur: 0.11, Vol: 0.94}, {Step: 60, Pitch: -8, Dur: 0.14, Vol: 1.00}, {Step: 62, Pitch: -6, Dur: 0.16, Vol: 0.99},
		{Step: 64, Pitch: -30, Dur: 0.41, Vol: 0.98}, {Step: 65, Pitch: -18, Dur: 0.11, Vol: 0.80}, {Step: 66, Pitch: -6, Dur: 0.11, Vol: 0.95}, {Step: 67, Pitch: -18, Dur: 0.10, Vol: 0.91},
		{Step: 68, Pitch: -8, Dur: 0.26, Vol: 0.93}, {Step: 69, Pitch: -6, Dur: 0.08, Vol: 0.82}, {Step: 70, Pitch: -30, Dur: 0.12, Vol: 0.91}, {Step: 72, Pitch: -3, Dur: 0.13, Vol: 1.00},
		{Step: 74, Pitch: -18, Dur: 0.04, Vol: 0.75}, {Step: 75, Pitch: -3, Dur: 0.62, Vol: 1.00}, {Step: 77, Pitch: -18, Dur: 0.07, Vol: 0.83}, {Step: 78, Pitch: -8, Dur: 0.11, Vol: 0.88},
		{Step: 80, Pitch: -30, Dur: 0.40, Vol: 0.95}, {Step: 82, Pitch: -6, Dur: 0.12, Vol: 1.00}, {Step: 83, Pitch: -18, Dur: 0.07, Vol: 0.77}, {Step: 84, Pitch: -8, Dur: 0.17, Vol: 0.99},
		{Step: 85, Pitch: -6, Dur: 0.09, Vol: 0.96}, {Step: 86, Pitch: -18, Dur: 0.05, Vol: 0.84}, {Step: 90, Pitch: -3, Dur: 0.10, Vol: 0.98}, {Step: 92, Pitch: -1, Dur: 0.35, Vol: 0.95},
		{Step: 94, Pitch: -3, Dur: 0.11, Vol: 0.90}, {Step: 95, Pitch: -3, Dur: 0.11, Vol: 1.00}, {Step: 96, Pitch: -30, Dur: 0.23, Vol: 0.87}, {Step: 97, Pitch: -18, Dur: 0.08, Vol: 0.76},
		{Step: 98, Pitch: -6, Dur: 0.11, Vol: 0.98}, {Step: 99, Pitch: -18, Dur: 0.05, Vol: 0.66}, {Step: 100, Pitch: -8, Dur: 0.15, Vol: 0.99}, {Step: 101, Pitch: -6, Dur: 0.12, Vol: 1.00},
		{Step: 104, Pitch: -3, Dur: 0.03, Vol: 0.97}, {Step: 107, Pitch: -3, Dur: 0.09, Vol: 0.98}, {Step: 109, Pitch: -18, Dur: 0.12, Vol: 0.81}, {Step: 110, Pitch: -3, Dur: 0.20, Vol: 0.95},
		{Step: 112, Pitch: -30, Dur: 0.24, Vol: 0.91}, {Step: 114, Pitch: 9, Dur: 0.08, Vol: 0.98}, {Step: 116, Pitch: -18, Dur: 0.09, Vol: 0.78}, {Step: 118, Pitch: -30, Dur: 0.18, Vol: 0.89},
		{Step: 119, Pitch: -18, Dur: 0.05, Vol: 0.72}, {Step: 120, Pitch: 9, Dur: 0.10, Vol: 1.00}, {Step: 122, Pitch: -18, Dur: 0.11, Vol: 0.85}, {Step: 123, Pitch: 9, Dur: 0.04, Vol: 0.83},
		{Step: 125, Pitch: -18, Dur: 0.13, Vol: 0.77}, {Step: 126, Pitch: -1, Dur: 0.20, Vol: 0.93}, {Step: 127, Pitch: 1, Dur: 0.10, Vol: 0.91}, {Step: 128, Pitch: -30, Dur: 0.20, Vol: 0.81},
		{Step: 130, Pitch: -3, Dur: 0.07, Vol: 0.83}, {Step: 134, Pitch: -3, Dur: 0.10, Vol: 0.95}, {Step: 139, Pitch: -6, Dur: 0.10, Vol: 1.00}, {Step: 141, Pitch: -18, Dur: 0.17, Vol: 0.85},
		{Step: 142, Pitch: -8, Dur: 0.20, Vol: 0.94}, {Step: 143, Pitch: -6, Dur: 0.12, Vol: 0.93}, {Step: 144, Pitch: -30, Dur: 0.59, Vol: 0.93}, {Step: 146, Pitch: -3, Dur: 0.09, Vol: 0.97},
		{Step: 148, Pitch: -3, Dur: 0.35, Vol: 1.00}, {Step: 150, Pitch: -18, Dur: 0.11, Vol: 0.82}, {Step: 153, Pitch: -18, Dur: 0.09, Vol: 0.64}, {Step: 155, Pitch: -18, Dur: 0.11, Vol: 0.87},
		{Step: 157, Pitch: -18, Dur: 0.06, Vol: 0.70}, {Step: 158, Pitch: -1, Dur: 0.23, Vol: 0.95}, {Step: 159, Pitch: -3, Dur: 0.08, Vol: 0.80}, {Step: 160, Pitch: -30, Dur: 0.26, Vol: 0.91},
		{Step: 161, Pitch: -18, Dur: 0.07, Vol: 0.73}, {Step: 162, Pitch: -3, Dur: 0.11, Vol: 0.95}, {Step: 164, Pitch: -8, Dur: 0.07, Vol: 0.76}, {Step: 166, Pitch: -3, Dur: 0.13, Vol: 1.00},
		{Step: 169, Pitch: -18, Dur: 0.07, Vol: 0.73}, {Step: 171, Pitch: -6, Dur: 0.12, Vol: 0.98}, {Step: 173, Pitch: -18, Dur: 0.11, Vol: 0.78}, {Step: 174, Pitch: -8, Dur: 0.15, Vol: 0.94},
		{Step: 175, Pitch: -6, Dur: 0.09, Vol: 0.87}, {Step: 176, Pitch: -30, Dur: 0.28, Vol: 0.93}, {Step: 178, Pitch: -3, Dur: 0.06, Vol: 0.97}, {Step: 180, Pitch: -3, Dur: 0.10, Vol: 1.00},
		{Step: 184, Pitch: -3, Dur: 0.09, Vol: 0.97}, {Step: 187, Pitch: -8, Dur: 0.03, Vol: 0.70}, {Step: 190, Pitch: -3, Dur: 0.09, Vol: 0.95}, {Step: 191, Pitch: -8, Dur: 0.07, Vol: 0.84},	}
	clav1Chorus := []Hit{ // 2-bar chorus comp cell (verified exact repeat)
		{Step: 0, Pitch: -30, Dur: 0.23, Vol: 0.90}, {Step: 6, Pitch: -3, Dur: 0.08, Vol: 0.98}, {Step: 8, Pitch: -3, Dur: 0.25, Vol: 1.00}, {Step: 10, Pitch: -18, Dur: 0.11, Vol: 0.82},
		{Step: 11, Pitch: -3, Dur: 0.10, Vol: 0.93}, {Step: 13, Pitch: -18, Dur: 0.19, Vol: 0.85}, {Step: 14, Pitch: -1, Dur: 0.21, Vol: 0.94}, {Step: 15, Pitch: -3, Dur: 0.10, Vol: 0.89},
		{Step: 16, Pitch: -30, Dur: 0.22, Vol: 0.88}, {Step: 18, Pitch: -3, Dur: 0.08, Vol: 0.80}, {Step: 20, Pitch: -18, Dur: 0.15, Vol: 0.80}, {Step: 21, Pitch: -3, Dur: 0.10, Vol: 0.86},
		{Step: 22, Pitch: -30, Dur: 0.17, Vol: 0.85}, {Step: 23, Pitch: -18, Dur: 0.08, Vol: 0.73}, {Step: 24, Pitch: -3, Dur: 0.12, Vol: 1.00}, {Step: 26, Pitch: -18, Dur: 0.12, Vol: 0.75},	}
	clav2Loop := []Hit{ // 4-bar layer loop (bars 7–10; 11–14 verified identical)
		{Step: 2, Pitch: -6, Dur: 0.13, Vol: 0.91}, {Step: 4, Pitch: -8, Dur: 0.29, Vol: 0.83}, {Step: 5, Pitch: -6, Dur: 0.10, Vol: 0.69}, {Step: 6, Pitch: -18, Dur: 0.07, Vol: 0.87},
		{Step: 8, Pitch: -3, Dur: 0.06, Vol: 0.88}, {Step: 10, Pitch: -18, Dur: 0.03, Vol: 0.69}, {Step: 11, Pitch: -6, Dur: 0.09, Vol: 0.91}, {Step: 13, Pitch: -18, Dur: 0.03, Vol: 0.74},
		{Step: 14, Pitch: -8, Dur: 0.29, Vol: 0.85}, {Step: 15, Pitch: -6, Dur: 0.09, Vol: 0.76}, {Step: 16, Pitch: -18, Dur: 0.16, Vol: 1.00}, {Step: 18, Pitch: -6, Dur: 0.12, Vol: 1.00},
		{Step: 20, Pitch: -8, Dur: 0.32, Vol: 0.91}, {Step: 21, Pitch: -6, Dur: 0.11, Vol: 0.74}, {Step: 22, Pitch: -18, Dur: 0.10, Vol: 0.85}, {Step: 24, Pitch: -11, Dur: 0.41, Vol: 0.92},
		{Step: 26, Pitch: -8, Dur: 0.21, Vol: 0.69}, {Step: 27, Pitch: -11, Dur: 0.12, Vol: 0.72}, {Step: 28, Pitch: -13, Dur: 0.27, Vol: 0.67}, {Step: 29, Pitch: -15, Dur: 0.12, Vol: 0.75},
		{Step: 30, Pitch: -13, Dur: 0.25, Vol: 0.75}, {Step: 31, Pitch: -15, Dur: 0.11, Vol: 0.69}, {Step: 32, Pitch: -18, Dur: 0.40, Vol: 1.00}, {Step: 34, Pitch: -6, Dur: 0.13, Vol: 0.92},
		{Step: 35, Pitch: -18, Dur: 0.03, Vol: 0.59}, {Step: 36, Pitch: -8, Dur: 0.21, Vol: 0.88}, {Step: 37, Pitch: -6, Dur: 0.08, Vol: 0.72}, {Step: 38, Pitch: -18, Dur: 0.09, Vol: 1.00},
		{Step: 40, Pitch: -3, Dur: 0.08, Vol: 0.85}, {Step: 43, Pitch: -3, Dur: 0.11, Vol: 0.85}, {Step: 46, Pitch: -3, Dur: 0.47, Vol: 0.88}, {Step: 48, Pitch: -18, Dur: 0.31, Vol: 0.87},
		{Step: 50, Pitch: -6, Dur: 0.14, Vol: 0.90}, {Step: 52, Pitch: -8, Dur: 0.36, Vol: 0.95}, {Step: 54, Pitch: -6, Dur: 0.15, Vol: 0.87}, {Step: 56, Pitch: -13, Dur: 0.15, Vol: 0.87},
		{Step: 59, Pitch: -11, Dur: 0.19, Vol: 0.77}, {Step: 61, Pitch: -8, Dur: 0.19, Vol: 0.87}, {Step: 63, Pitch: -6, Dur: 0.10, Vol: 0.92},	}
	clav3Loop := []Hit{ // high accent-jab layer, 4-bar loop
		{Step: 2, Pitch: -8, Dur: 0.08, Vol: 1.00}, {Step: 4, Pitch: -6, Dur: 0.06, Vol: 1.00}, {Step: 8, Pitch: -6, Dur: 0.07, Vol: 1.00}, {Step: 12, Pitch: -6, Dur: 0.11, Vol: 1.00},
		{Step: 14, Pitch: -8, Dur: 0.03, Vol: 0.82}, {Step: 15, Pitch: -6, Dur: 0.61, Vol: 1.00}, {Step: 18, Pitch: -8, Dur: 0.07, Vol: 1.00}, {Step: 20, Pitch: -6, Dur: 0.08, Vol: 0.99},
		{Step: 22, Pitch: -8, Dur: 0.03, Vol: 0.97}, {Step: 24, Pitch: -3, Dur: 0.12, Vol: 1.00}, {Step: 26, Pitch: -1, Dur: 0.13, Vol: 1.00}, {Step: 28, Pitch: -3, Dur: 0.12, Vol: 1.00},
		{Step: 30, Pitch: -8, Dur: 0.07, Vol: 1.00}, {Step: 32, Pitch: -6, Dur: 0.28, Vol: 1.00}, {Step: 34, Pitch: -8, Dur: 0.07, Vol: 1.00}, {Step: 36, Pitch: -6, Dur: 0.16, Vol: 1.00},
		{Step: 38, Pitch: -8, Dur: 0.06, Vol: 0.96}, {Step: 40, Pitch: -6, Dur: 0.11, Vol: 1.00}, {Step: 42, Pitch: -8, Dur: 0.04, Vol: 0.90}, {Step: 44, Pitch: -6, Dur: 0.08, Vol: 1.00},
		{Step: 47, Pitch: -1, Dur: 0.19, Vol: 1.00}, {Step: 49, Pitch: -1, Dur: 0.07, Vol: 0.69}, {Step: 50, Pitch: -1, Dur: 0.17, Vol: 1.00}, {Step: 52, Pitch: -3, Dur: 0.16, Vol: 1.00},
		{Step: 54, Pitch: -6, Dur: 0.11, Vol: 1.00}, {Step: 56, Pitch: -6, Dur: 0.12, Vol: 1.00}, {Step: 60, Pitch: -6, Dur: 0.11, Vol: 1.00}, {Step: 62, Pitch: -8, Dur: 0.03, Vol: 0.81},	}
	bassLoop := []Hit{ // Moog bass 4-bar ostinato (verse AND chorus, verified)
		{Step: 0, Pitch: -30, Dur: 0.39, Vol: 0.83}, {Step: 2, Pitch: -18, Dur: 0.17, Vol: 0.77}, {Step: 3, Pitch: -30, Dur: 0.09, Vol: 0.48}, {Step: 4, Pitch: -20, Dur: 0.26, Vol: 0.72},
		{Step: 5, Pitch: -18, Dur: 0.13, Vol: 0.71}, {Step: 6, Pitch: -30, Dur: 0.36, Vol: 0.76}, {Step: 8, Pitch: -27, Dur: 0.22, Vol: 0.82}, {Step: 9, Pitch: -25, Dur: 0.17, Vol: 0.58},
		{Step: 10, Pitch: -23, Dur: 0.16, Vol: 0.73}, {Step: 11, Pitch: -20, Dur: 0.46, Vol: 0.77}, {Step: 13, Pitch: -23, Dur: 0.13, Vol: 0.69}, {Step: 14, Pitch: -18, Dur: 0.21, Vol: 0.79},
		{Step: 15, Pitch: -18, Dur: 0.19, Vol: 0.76}, {Step: 16, Pitch: -30, Dur: 0.42, Vol: 0.82}, {Step: 18, Pitch: -18, Dur: 0.15, Vol: 0.82}, {Step: 19, Pitch: -30, Dur: 0.09, Vol: 0.62},
		{Step: 20, Pitch: -20, Dur: 0.34, Vol: 0.76}, {Step: 21, Pitch: -18, Dur: 0.19, Vol: 0.77}, {Step: 23, Pitch: -20, Dur: 0.10, Vol: 0.61}, {Step: 24, Pitch: -13, Dur: 0.24, Vol: 0.83},
		{Step: 26, Pitch: -15, Dur: 0.24, Vol: 0.84}, {Step: 28, Pitch: -15, Dur: 0.44, Vol: 0.79}, {Step: 30, Pitch: -18, Dur: 0.28, Vol: 0.73}, {Step: 31, Pitch: -20, Dur: 0.13, Vol: 0.70},
		{Step: 32, Pitch: -30, Dur: 0.52, Vol: 0.69}, {Step: 34, Pitch: -18, Dur: 0.19, Vol: 0.84}, {Step: 35, Pitch: -30, Dur: 0.05, Vol: 0.54}, {Step: 36, Pitch: -20, Dur: 0.27, Vol: 0.80},
		{Step: 37, Pitch: -18, Dur: 0.11, Vol: 0.71}, {Step: 38, Pitch: -30, Dur: 0.35, Vol: 0.77}, {Step: 40, Pitch: -27, Dur: 0.23, Vol: 0.93}, {Step: 41, Pitch: -25, Dur: 0.15, Vol: 0.62},
		{Step: 42, Pitch: -23, Dur: 0.19, Vol: 0.73}, {Step: 43, Pitch: -20, Dur: 0.43, Vol: 0.80}, {Step: 45, Pitch: -23, Dur: 0.13, Vol: 0.69}, {Step: 46, Pitch: -18, Dur: 0.22, Vol: 0.80},
		{Step: 47, Pitch: -18, Dur: 0.22, Vol: 0.69}, {Step: 48, Pitch: -30, Dur: 0.46, Vol: 0.84}, {Step: 50, Pitch: -18, Dur: 0.18, Vol: 0.84}, {Step: 51, Pitch: -30, Dur: 0.10, Vol: 0.57},
		{Step: 52, Pitch: -20, Dur: 0.28, Vol: 0.80}, {Step: 53, Pitch: -18, Dur: 0.15, Vol: 0.80}, {Step: 56, Pitch: -13, Dur: 0.24, Vol: 0.80}, {Step: 58, Pitch: -15, Dur: 0.28, Vol: 0.79},
		{Step: 60, Pitch: -15, Dur: 0.45, Vol: 0.86}, {Step: 62, Pitch: -18, Dur: 0.21, Vol: 0.76}, {Step: 63, Pitch: -20, Dur: 0.10, Vol: 0.71},	}
	hornCell := []Hit{ // 2-bar chorus stab cell (lower 8va = tenor sax)
		{Step: 2, Pitch: 6, Dur: 0.24, Vol: 0.91}, {Step: 5, Pitch: 6, Dur: 0.24, Vol: 0.91}, {Step: 8, Pitch: -3, Dur: 0.24, Vol: 0.91}, {Step: 9, Pitch: -1, Dur: 0.24, Vol: 0.91},
		{Step: 10, Pitch: 1, Dur: 0.24, Vol: 0.91}, {Step: 11, Pitch: 4, Dur: 0.49, Vol: 0.91}, {Step: 13, Pitch: 1, Dur: 0.11, Vol: 0.91}, {Step: 14, Pitch: 6, Dur: 0.24, Vol: 0.91},
		{Step: 15, Pitch: 6, Dur: 0.13, Vol: 0.91}, {Step: 18, Pitch: 6, Dur: 0.13, Vol: 0.91}, {Step: 20, Pitch: 4, Dur: 0.24, Vol: 0.91}, {Step: 21, Pitch: 6, Dur: 0.12, Vol: 0.91},
		{Step: 24, Pitch: 13, Dur: 0.15, Vol: 0.91}, {Step: 26, Pitch: 11, Dur: 0.13, Vol: 0.91}, {Step: 28, Pitch: 9, Dur: 0.16, Vol: 0.91},	}
	// Hat 8ths: strong on-beats / ghosted off-8ths, plus the swung 16th pushes
	// at 5, 11, 15 (velocity map from the MIDI averages).
	hatBar := []Hit{
		{Step: 0, Vol: 0.81}, {Step: 2, Vol: 0.55}, {Step: 4, Vol: 0.97}, {Step: 5, Vol: 0.83},
		{Step: 6, Vol: 0.54}, {Step: 8, Vol: 0.88}, {Step: 10, Vol: 0.57}, {Step: 11, Vol: 0.87},
		{Step: 12, Vol: 0.95}, {Step: 14, Vol: 0.6}, {Step: 15, Vol: 0.88},
	}
	sw := func(hits []Hit) []Hit { return swing16(hits, 0.33) }
	return Showcase{
		Stem: "wonder-superstition", BPM: 101, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			// Kick leads: the record opens with the solo drum break, and row 0
			// must produce visible steps immediately (import round-trip gate).
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.95},
			{ID: "fm-pluck", Name: "Clav 1", Volume: 0.72, Pan: -0.15},
			{ID: "fm-pluck-1", Name: "Clav 2", Volume: 0.5, Pan: 0.2},
			{ID: "fm-epiano", Name: "Clav 3", Volume: 0.42, Pan: 0.35},
			{ID: "bass-fm", Name: "Moog Bass", Volume: 0.88},
			{ID: "ensemble-lead", Name: "Vocal", Volume: 0.6, ReverbSend: 0.18},
			{ID: "trumpet", Name: "Trumpet", Volume: 0.58, Pan: 0.15, ReverbSend: 0.12},
			{ID: "sax", Name: "Tenor Sax", Volume: 0.52, Pan: -0.15, ReverbSend: 0.12},
			{ID: "snare", Name: "Snare", Volume: 0.8},
			{ID: "clap-tight", Name: "Clap", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "snare-ghost", Name: "Ghost", Volume: 0.45},
			{ID: "hihat", Name: "Hihat", Volume: 0.55},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.6, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Kick — four-on-floor; chorus bar 14 adds the 16th-pickup drive.
			{Inst: "kick-acoustic", Hits: concat(
				ostinato(16, []Hit{{Step: 0, Vol: 1.0}, {Step: 4, Vol: 0.95}, {Step: 8, Vol: 0.97}, {Step: 12, Vol: 0.95}}),
				[]Hit{{Step: 218, Vol: 0.9}, {Step: 222, Vol: 0.9}},
			)},
			// Clav 1 — pickup + hook + verse comp, then the 2-bar chorus cell ×2.
			{Inst: "fm-pluck", Hits: sw(concat(clav1Main, at(192, clav1Chorus), at(224, clav1Chorus)))},
			// Clav 2 — pickup + the 4-bar layer loop through verse and chorus.
			{Inst: "fm-pluck-1", Hits: sw(concat(
				[]Hit{{Step: 57, Pitch: -13, Vol: 0.9}, {Step: 58, Pitch: -11, Vol: 1.0}, {Step: 59, Pitch: -13, Vol: 0.76}, {Step: 60, Pitch: -8, Vol: 0.98}, {Step: 62, Pitch: -6, Vol: 1.0}},
				at(64, clav2Loop), at(128, clav2Loop), at(192, clav2Loop),
			))},
			// Clav 3 — flat-127 accent jabs (bars 5–16).
			{Inst: "fm-epiano", Hits: sw(concat(at(64, clav3Loop), at(128, clav3Loop), at(192, clav3Loop)))},
			// Moog bass — enters with the hook (bar 5) and never stops.
			// +12: bass-fm renders one octave down; re-encoded to true record pitch.
			{Inst: "bass-fm", Hits: sw(transposeHits(concat(at(64, bassLoop), at(128, bassLoop), at(192, bassLoop)), 12))},
			// Lead vocal substitute — verse phrases + chorus hook (belted register).
			{Inst: "ensemble-lead", Hits: sw([]Hit{
		{Step: 120, Pitch: 4, Dur: 0.38, Vol: 1.00}, {Step: 122, Pitch: 6, Dur: 0.09, Vol: 0.93}, {Step: 123, Pitch: 6, Dur: 0.33, Vol: 1.00}, {Step: 125, Pitch: 6, Dur: 0.18, Vol: 1.00},
		{Step: 127, Pitch: 9, Dur: 0.95, Vol: 1.00}, {Step: 132, Pitch: 6, Dur: 1.49, Vol: 1.00}, {Step: 138, Pitch: 4, Dur: 0.14, Vol: 0.74}, {Step: 139, Pitch: 1, Dur: 0.09, Vol: 0.70},
		{Step: 152, Pitch: 4, Dur: 0.08, Vol: 0.96}, {Step: 153, Pitch: 1, Dur: 0.56, Vol: 1.00}, {Step: 156, Pitch: -1, Dur: 0.42, Vol: 1.00}, {Step: 158, Pitch: -3, Dur: 0.07, Vol: 0.85},
		{Step: 159, Pitch: -1, Dur: 0.70, Vol: 1.00}, {Step: 162, Pitch: -3, Dur: 0.33, Vol: 0.93}, {Step: 196, Pitch: 6, Dur: 1.40, Vol: 1.00}, {Step: 202, Pitch: 4, Dur: 0.13, Vol: 0.85},
		{Step: 203, Pitch: 1, Dur: 0.06, Vol: 0.77}, {Step: 216, Pitch: 4, Dur: 0.08, Vol: 1.00}, {Step: 217, Pitch: -1, Dur: 0.58, Vol: 1.00}, {Step: 220, Pitch: -1, Dur: 0.38, Vol: 0.98},
		{Step: 222, Pitch: -3, Dur: 0.07, Vol: 0.75}, {Step: 223, Pitch: -1, Dur: 0.71, Vol: 1.00}, {Step: 226, Pitch: -3, Dur: 0.20, Vol: 0.88}, {Step: 248, Pitch: 11, Dur: 0.33, Vol: 1.00},
		{Step: 250, Pitch: 11, Dur: 0.26, Vol: 0.92}, {Step: 252, Pitch: 11, Dur: 0.09, Vol: 0.94}, {Step: 254, Pitch: 11, Dur: 0.11, Vol: 0.99}, {Step: 255, Pitch: 9, Dur: 0.62, Vol: 0.94},			})},
			// Horns — trumpet doubles the tenor an octave up (the record's octave
			// section writing), chorus only.
			{Inst: "trumpet", Hits: sw(concat(at(192, transposeHits(hornCell, 12)), at(224, transposeHits(hornCell, 12))))},
			{Inst: "sax", Hits: sw(concat(at(192, hornCell), at(224, hornCell)))},
			// Backbeat: snare + tight clap layered (the MIDI's snare-el+clap pair).
			{Inst: "snare", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.97}, {Step: 12, Vol: 1.0}})},
			{Inst: "clap-tight", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.45}, {Step: 12, Vol: 0.45}})},
			// Ghost snares on the swung "a of 2" / "e of 4" (prose-sourced,
			// labeled idiomatic in the dossier).
			{Inst: "snare-ghost", Hits: sw(ostinato(16, []Hit{{Step: 11, Vol: 0.28}, {Step: 13, Vol: 0.25}}))},
			{Inst: "hihat", Hits: sw(ostinato(16, hatBar))},
			// Open hat marks bar 3 of every 4-bar cycle (step 11, swung).
			{Inst: "hihat-1", Hits: sw([]Hit{{Step: 43, Vol: 0.72}, {Step: 107, Vol: 0.72}, {Step: 171, Vol: 0.72}, {Step: 235, Vol: 0.72}})},
			// Crash opens each 4-bar cycle.
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.55}, {Step: 64, Vol: 0.5}, {Step: 128, Vol: 0.45}, {Step: 192, Vol: 0.5}}},
		},
	}
}
