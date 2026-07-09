package main

// Gloria Gaynor — "I Will Survive" (1978). A minor, 117 BPM. Note data from
// BitMidi 59419 (multi-track: bass/piano/brass/strings/drums) + 48783 (karaoke:
// chorus vocal melody, claps, crash); prose from Wikipedia (Perren/Fekaris
// production, Gadson drums, Da Costa percussion, Scott Edwards bass). Dossier:
// gaynor-survive_dossier.md.
//
// 16 bars = the famous 8-chord cycle-of-fourths ×2
// (Am·Dm7·G·Cmaj7·Fmaj7·Bm7b5·Esus4·E). Cycle 1 is the instrumental groove
// (strings counter-melody answers the changes); cycle 2 adds the chorus vocal
// (ensemble-lead) and the brass stab doubling — the record's own build.
//
// GROOVE: hard-straight 16ths; the drive is the dotted-8th piano-stab chain
// (steps 2-5-8-11-14) against the four-on-floor kick. Bass = octave pairs,
// high octave dominant (bass-guitar) over a sub-bass low layer.
func gaynorShowcase() Showcase {
	bassHi := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.40, Vol: 0.85}, {Step: 2, Pitch: -12, Dur: 0.40, Vol: 0.85}, {Step: 5, Pitch: -12, Dur: 0.70, Vol: 0.85}, {Step: 8, Pitch: -12, Dur: 0.90, Vol: 0.85},
		{Step: 12, Pitch: -9, Dur: 0.40, Vol: 0.85}, {Step: 14, Pitch: -12, Dur: 0.40, Vol: 0.85}, {Step: 16, Pitch: -7, Dur: 0.40, Vol: 0.85}, {Step: 18, Pitch: -7, Dur: 0.40, Vol: 0.85},
		{Step: 21, Pitch: -7, Dur: 0.70, Vol: 0.85}, {Step: 24, Pitch: -7, Dur: 0.90, Vol: 0.85}, {Step: 28, Pitch: -5, Dur: 0.40, Vol: 0.85}, {Step: 30, Pitch: -4, Dur: 0.40, Vol: 0.85},
		{Step: 32, Pitch: -2, Dur: 0.40, Vol: 0.85}, {Step: 34, Pitch: -14, Dur: 0.40, Vol: 0.85}, {Step: 37, Pitch: -14, Dur: 0.70, Vol: 0.85}, {Step: 40, Pitch: -14, Dur: 0.90, Vol: 0.85},
		{Step: 44, Pitch: -12, Dur: 0.40, Vol: 0.85}, {Step: 46, Pitch: -10, Dur: 0.40, Vol: 0.85}, {Step: 48, Pitch: -9, Dur: 0.40, Vol: 0.85}, {Step: 50, Pitch: -9, Dur: 0.40, Vol: 0.85},
		{Step: 53, Pitch: -9, Dur: 0.70, Vol: 0.85}, {Step: 56, Pitch: -9, Dur: 0.90, Vol: 0.85}, {Step: 60, Pitch: -10, Dur: 0.40, Vol: 0.85}, {Step: 62, Pitch: -12, Dur: 0.40, Vol: 0.85},
		{Step: 63, Pitch: -14, Dur: 0.40, Vol: 0.85}, {Step: 64, Pitch: -16, Dur: 0.40, Vol: 0.85}, {Step: 66, Pitch: -16, Dur: 0.40, Vol: 0.85}, {Step: 69, Pitch: -16, Dur: 0.70, Vol: 0.85},
		{Step: 72, Pitch: -16, Dur: 0.90, Vol: 0.85}, {Step: 76, Pitch: -14, Dur: 0.40, Vol: 0.85}, {Step: 78, Pitch: -12, Dur: 0.40, Vol: 0.85}, {Step: 80, Pitch: -10, Dur: 0.40, Vol: 0.85},
		{Step: 82, Pitch: -10, Dur: 0.40, Vol: 0.85}, {Step: 85, Pitch: -10, Dur: 0.70, Vol: 0.85}, {Step: 88, Pitch: -10, Dur: 0.90, Vol: 0.85}, {Step: 92, Pitch: -7, Dur: 0.40, Vol: 0.85},
		{Step: 94, Pitch: -6, Dur: 0.40, Vol: 0.85}, {Step: 96, Pitch: -5, Dur: 0.40, Vol: 0.85}, {Step: 98, Pitch: -17, Dur: 0.40, Vol: 0.85}, {Step: 101, Pitch: -17, Dur: 0.70, Vol: 0.85},
		{Step: 104, Pitch: -17, Dur: 0.90, Vol: 0.85}, {Step: 108, Pitch: -17, Dur: 0.40, Vol: 0.85}, {Step: 110, Pitch: -7, Dur: 0.40, Vol: 0.85}, {Step: 111, Pitch: -6, Dur: 0.40, Vol: 0.85},
		{Step: 112, Pitch: -5, Dur: 0.40, Vol: 0.85}, {Step: 114, Pitch: -17, Dur: 0.40, Vol: 0.85}, {Step: 117, Pitch: -17, Dur: 0.70, Vol: 0.85}, {Step: 120, Pitch: -2, Dur: 0.90, Vol: 0.85},
		{Step: 122, Pitch: -14, Dur: 0.40, Vol: 0.85}, {Step: 124, Pitch: -1, Dur: 0.40, Vol: 0.85}, {Step: 126, Pitch: -13, Dur: 0.40, Vol: 0.85},	}
	bassLo := []Hit{
		{Step: 0, Pitch: -12, Dur: 0.40, Vol: 0.55}, {Step: 2, Pitch: -24, Dur: 0.40, Vol: 0.55}, {Step: 5, Pitch: -24, Dur: 0.70, Vol: 0.55}, {Step: 8, Pitch: -24, Dur: 0.90, Vol: 0.55},
		{Step: 12, Pitch: -21, Dur: 0.40, Vol: 0.55}, {Step: 14, Pitch: -24, Dur: 0.40, Vol: 0.55}, {Step: 16, Pitch: -19, Dur: 0.40, Vol: 0.55}, {Step: 18, Pitch: -19, Dur: 0.40, Vol: 0.55},
		{Step: 21, Pitch: -19, Dur: 0.70, Vol: 0.55}, {Step: 24, Pitch: -19, Dur: 0.90, Vol: 0.55}, {Step: 28, Pitch: -17, Dur: 0.40, Vol: 0.55}, {Step: 30, Pitch: -16, Dur: 0.40, Vol: 0.55},
		{Step: 32, Pitch: -14, Dur: 0.40, Vol: 0.55}, {Step: 34, Pitch: -26, Dur: 0.40, Vol: 0.55}, {Step: 37, Pitch: -26, Dur: 0.70, Vol: 0.55}, {Step: 40, Pitch: -26, Dur: 0.90, Vol: 0.55},
		{Step: 44, Pitch: -24, Dur: 0.40, Vol: 0.55}, {Step: 46, Pitch: -22, Dur: 0.40, Vol: 0.55}, {Step: 48, Pitch: -21, Dur: 0.40, Vol: 0.55}, {Step: 50, Pitch: -21, Dur: 0.40, Vol: 0.55},
		{Step: 53, Pitch: -21, Dur: 0.70, Vol: 0.55}, {Step: 56, Pitch: -21, Dur: 0.90, Vol: 0.55}, {Step: 60, Pitch: -22, Dur: 0.40, Vol: 0.55}, {Step: 62, Pitch: -24, Dur: 0.40, Vol: 0.55},
		{Step: 63, Pitch: -26, Dur: 0.40, Vol: 0.55}, {Step: 64, Pitch: -28, Dur: 0.40, Vol: 0.55}, {Step: 66, Pitch: -28, Dur: 0.40, Vol: 0.55}, {Step: 69, Pitch: -28, Dur: 0.70, Vol: 0.55},
		{Step: 72, Pitch: -28, Dur: 0.90, Vol: 0.55}, {Step: 76, Pitch: -26, Dur: 0.40, Vol: 0.55}, {Step: 78, Pitch: -24, Dur: 0.40, Vol: 0.55}, {Step: 80, Pitch: -22, Dur: 0.40, Vol: 0.55},
		{Step: 82, Pitch: -22, Dur: 0.40, Vol: 0.55}, {Step: 85, Pitch: -22, Dur: 0.70, Vol: 0.55}, {Step: 88, Pitch: -22, Dur: 0.90, Vol: 0.55}, {Step: 92, Pitch: -19, Dur: 0.40, Vol: 0.55},
		{Step: 94, Pitch: -18, Dur: 0.40, Vol: 0.55}, {Step: 96, Pitch: -17, Dur: 0.40, Vol: 0.55}, {Step: 98, Pitch: -29, Dur: 0.40, Vol: 0.55}, {Step: 101, Pitch: -29, Dur: 0.70, Vol: 0.55},
		{Step: 104, Pitch: -29, Dur: 0.90, Vol: 0.55}, {Step: 108, Pitch: -29, Dur: 0.40, Vol: 0.55}, {Step: 110, Pitch: -19, Dur: 0.40, Vol: 0.55}, {Step: 111, Pitch: -18, Dur: 0.40, Vol: 0.55},
		{Step: 112, Pitch: -17, Dur: 0.40, Vol: 0.55}, {Step: 114, Pitch: -29, Dur: 0.40, Vol: 0.55}, {Step: 117, Pitch: -29, Dur: 0.70, Vol: 0.55}, {Step: 120, Pitch: -14, Dur: 0.90, Vol: 0.55},
		{Step: 122, Pitch: -26, Dur: 0.40, Vol: 0.55}, {Step: 124, Pitch: -13, Dur: 0.40, Vol: 0.55}, {Step: 126, Pitch: -25, Dur: 0.40, Vol: 0.55},	}
	counterMelody := []Hit{
		{Step: 0, Pitch: 12, Dur: 3.00, Vol: 0.85}, {Step: 12, Pitch: 15, Dur: 0.50, Vol: 0.85}, {Step: 14, Pitch: 12, Dur: 0.50, Vol: 0.85}, {Step: 16, Pitch: 17, Dur: 2.50, Vol: 0.85},
		{Step: 26, Pitch: 15, Dur: 0.50, Vol: 0.85}, {Step: 28, Pitch: 14, Dur: 0.50, Vol: 0.85}, {Step: 30, Pitch: 12, Dur: 0.50, Vol: 0.85}, {Step: 32, Pitch: 10, Dur: 3.00, Vol: 0.85},
		{Step: 44, Pitch: 12, Dur: 0.50, Vol: 0.85}, {Step: 46, Pitch: 14, Dur: 0.50, Vol: 0.85}, {Step: 48, Pitch: 15, Dur: 3.00, Vol: 0.85}, {Step: 60, Pitch: 14, Dur: 0.50, Vol: 0.85},
		{Step: 62, Pitch: 12, Dur: 0.25, Vol: 0.85}, {Step: 63, Pitch: 10, Dur: 0.25, Vol: 0.85}, {Step: 64, Pitch: 12, Dur: 4.00, Vol: 0.85},	}
	// Dotted-8th piano stab chain: 3-tone rolls at 2,5,8,11,14 of every bar;
	// the step-11 stab is held + accented (the lean-in).
	pianoCycle := concat(
		pianoBar(0, 12, 15, 19), pianoBar(16, 12, 17, 20), pianoBar(32, 14, 17, 22),
		pianoBar(48, 14, 19, 22), pianoBar(64, 12, 15, 19), pianoBar(80, 12, 17, 20),
		pianoBar(96, 12, 14, 19), pianoBar(112, 11, 14, 19),
	)
	// String pads: whole-bar chords (upper-structure voicings from the MIDI).
	padCycle := concat(
		chordAt(0, 3.8, 0.4, -12, 0, 3, 7), chordAt(16, 3.8, 0.4, -19, 0, 5, 8),
		chordAt(32, 3.8, 0.4, -14, -2, 2, 5), chordAt(48, 3.8, 0.4, -21, -2, 2, 7),
		chordAt(64, 3.8, 0.4, -16, -5, 0, 3), chordAt(80, 3.8, 0.4, 8, 12, 17),
		chordAt(96, 7.8, 0.4, 7, 12, 19), chordAt(112, 3.8, 0.4, 11, 14),
	)
	// Brass doubles the piano stab top note (cycle 2 of the record).
	brassCycle := concat(
		brassBar(0, 19), brassBar(16, 20), brassBar(32, 22), brassBar(48, 22),
		brassBar(64, 19), brassBar(80, 20), brassBar(96, 19), brassBar(112, 19),
	)
	drumBar := []Hit{}
	_ = drumBar
	return Showcase{
		Stem: "gaynor-survive", BPM: 117, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "ghost-bass", Name: "Low Oct", Volume: 0.42},
			{ID: "piano-grand", Name: "Piano", Volume: 0.68, Pan: -0.15, ReverbSend: 0.12},
			{ID: "trumpet", Name: "Brass", Volume: 0.5, Pan: 0.2, ReverbSend: 0.12},
			{ID: "violin-ensemble", Name: "Strings", Volume: 0.6, Pan: -0.25, ReverbSend: 0.28},
			{ID: "viola-pad", Name: "Pads", Volume: 0.4, Pan: 0.25, ReverbSend: 0.3},
			{ID: "ensemble-lead", Name: "Vocal", Volume: 0.66, ReverbSend: 0.18},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.95},
			{ID: "snare", Name: "Snare", Volume: 0.78},
			{ID: "snare-ghost", Name: "Fill", Volume: 0.5},
			{ID: "clap", Name: "Clap", Volume: 0.68, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "hihat", Name: "Hihat", Volume: 0.55},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.62, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga-open", Name: "Conga", Volume: 0.5, Pan: 0.3},
			{ID: "conga-tumba", Name: "Tumba", Volume: 0.45, Pan: 0.35},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// +12: bass-guitar renders one octave down; re-encoded to true record pitch.
			{Inst: "bass-guitar", Hits: transposeHits(concat(bassHi, at(128, bassHi)), 12)},
			{Inst: "ghost-bass", Hits: transposeHits(concat(bassLo, at(128, bassLo)), 12)},
			{Inst: "piano-grand", Hits: concat(pianoCycle, at(128, pianoCycle))},
			// Brass joins on cycle 2 (bar 21+ in the source).
			{Inst: "trumpet", Hits: at(128, brassCycle)},
			{Inst: "violin-ensemble", Hits: concat(counterMelody, at(128, counterMelody))},
			{Inst: "viola-pad", Hits: concat(padCycle, at(128, padCycle))},
			// Chorus vocal rides cycle 2 only — the record's verse→chorus build.
			{Inst: "ensemble-lead", Hits: at(128, []Hit{
		{Step: 0, Pitch: 12, Dur: 0.27, Vol: 0.72}, {Step: 2, Pitch: 12, Dur: 0.55, Vol: 0.72}, {Step: 5, Pitch: 12, Dur: 0.62, Vol: 0.72}, {Step: 8, Pitch: 12, Dur: 0.35, Vol: 0.72},
		{Step: 10, Pitch: 12, Dur: 0.39, Vol: 0.72}, {Step: 12, Pitch: 14, Dur: 0.47, Vol: 0.72}, {Step: 14, Pitch: 15, Dur: 0.37, Vol: 0.72}, {Step: 16, Pitch: 14, Dur: 0.41, Vol: 0.72},
		{Step: 18, Pitch: 12, Dur: 0.55, Vol: 0.72}, {Step: 21, Pitch: 12, Dur: 0.55, Vol: 0.72}, {Step: 24, Pitch: 12, Dur: 1.30, Vol: 0.72}, {Step: 30, Pitch: 12, Dur: 0.27, Vol: 0.72},
		{Step: 32, Pitch: 12, Dur: 0.48, Vol: 0.72}, {Step: 34, Pitch: 10, Dur: 0.31, Vol: 0.72}, {Step: 36, Pitch: 10, Dur: 0.17, Vol: 0.72}, {Step: 37, Pitch: 10, Dur: 0.27, Vol: 0.72},
		{Step: 39, Pitch: 12, Dur: 0.78, Vol: 0.72}, {Step: 42, Pitch: 10, Dur: 0.30, Vol: 0.72}, {Step: 44, Pitch: 10, Dur: 0.49, Vol: 0.72}, {Step: 46, Pitch: 8, Dur: 0.25, Vol: 0.72},
		{Step: 48, Pitch: 8, Dur: 0.47, Vol: 0.72}, {Step: 50, Pitch: 7, Dur: 0.29, Vol: 0.72}, {Step: 52, Pitch: 7, Dur: 0.60, Vol: 0.72}, {Step: 55, Pitch: 7, Dur: 1.50, Vol: 0.72},
		{Step: 62, Pitch: 7, Dur: 0.29, Vol: 0.72}, {Step: 64, Pitch: 8, Dur: 0.46, Vol: 0.72}, {Step: 66, Pitch: 7, Dur: 0.26, Vol: 0.72}, {Step: 68, Pitch: 7, Dur: 0.18, Vol: 0.72},
		{Step: 69, Pitch: 7, Dur: 0.30, Vol: 0.72}, {Step: 71, Pitch: 8, Dur: 0.70, Vol: 0.72}, {Step: 74, Pitch: 7, Dur: 0.67, Vol: 0.72}, {Step: 77, Pitch: 7, Dur: 0.37, Vol: 0.72},
		{Step: 80, Pitch: 7, Dur: 0.52, Vol: 0.72}, {Step: 82, Pitch: 5, Dur: 0.49, Vol: 0.72}, {Step: 85, Pitch: 5, Dur: 0.52, Vol: 0.72}, {Step: 88, Pitch: 5, Dur: 0.91, Vol: 0.72},
		{Step: 91, Pitch: 8, Dur: 0.66, Vol: 0.72}, {Step: 94, Pitch: 5, Dur: 0.34, Vol: 0.72}, {Step: 96, Pitch: 7, Dur: 0.28, Vol: 0.72}, {Step: 98, Pitch: 7, Dur: 0.54, Vol: 0.72},
		{Step: 101, Pitch: 7, Dur: 1.39, Vol: 0.72}, {Step: 106, Pitch: 11, Dur: 0.65, Vol: 0.72}, {Step: 109, Pitch: 12, Dur: 0.45, Vol: 0.72}, {Step: 110, Pitch: 9, Dur: 0.42, Vol: 0.72},
		{Step: 112, Pitch: 11, Dur: 2.20, Vol: 0.72}, {Step: 122, Pitch: 11, Dur: 0.39, Vol: 0.72}, {Step: 124, Pitch: 11, Dur: 0.17, Vol: 0.72}, {Step: 125, Pitch: 11, Dur: 0.33, Vol: 0.72},
		{Step: 127, Pitch: 12, Dur: 0.70, Vol: 0.72},			})},
			{Inst: "kick-acoustic", Hits: ostinato(16, []Hit{{Step: 0, Vol: 1.0}, {Step: 4, Vol: 0.95}, {Step: 8, Vol: 0.97}, {Step: 12, Vol: 0.95}})},
			{Inst: "snare", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.78}, {Step: 12, Vol: 0.78}})},
			// Boundary fill (idiomatic, labeled UNSOURCED in the dossier):
			// snare-16th crescendo closing each 8-bar cycle.
			{Inst: "snare-ghost", Hits: []Hit{
				{Step: 125, Vol: 0.42}, {Step: 126, Vol: 0.52}, {Step: 127, Vol: 0.62},
				{Step: 253, Vol: 0.42}, {Step: 254, Vol: 0.52}, {Step: 255, Vol: 0.62},
			}},
			{Inst: "clap", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.68}, {Step: 12, Vol: 0.68}})},
			// Closed-hat 16th pairs on/after each beat; open barks on offbeats.
			{Inst: "hihat", Hits: ostinato(16, []Hit{
				{Step: 0, Vol: 0.66}, {Step: 1, Vol: 0.5}, {Step: 4, Vol: 0.66}, {Step: 5, Vol: 0.5},
				{Step: 8, Vol: 0.66}, {Step: 9, Vol: 0.5}, {Step: 12, Vol: 0.66}, {Step: 13, Vol: 0.5},
			})},
			{Inst: "hihat-1", Hits: ostinato(16, []Hit{{Step: 2, Vol: 0.77}, {Step: 6, Vol: 0.77}, {Step: 10, Vol: 0.77}, {Step: 14, Vol: 0.77}})},
			// Da Costa conga layer.
			{Inst: "conga-open", Hits: ostinato(16, []Hit{{Step: 2, Dur: 0.3, Vol: 0.6}, {Step: 8, Dur: 0.3, Vol: 0.6}})},
			{Inst: "conga-tumba", Hits: ostinato(16, []Hit{{Step: 5, Dur: 0.3, Vol: 0.55}, {Step: 12, Dur: 0.25, Vol: 0.45}, {Step: 14, Dur: 0.25, Vol: 0.48}})},
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.5}, {Step: 128, Vol: 0.55}}},
		},
	}
}

// pianoBar builds one bar of the I Will Survive dotted-8th stab chain: 3-tone
// rolls at local steps 2,5,8,11,14 with the step-11 stab held + accented.
func pianoBar(base int, tones ...float64) []Hit {
	return concat(
		chordAt(base+2, 0.12, 0.78, tones...),
		chordAt(base+5, 0.12, 0.76, tones...),
		chordAt(base+8, 0.12, 0.78, tones...),
		chordAt(base+11, 0.5, 0.84, tones...),
		chordAt(base+14, 0.12, 0.76, tones...),
	)
}

// brassBar doubles the stab chain's top note (single hits, same rhythm).
func brassBar(base int, top float64) []Hit {
	return []Hit{
		{Step: base + 2, Pitch: top, Dur: 0.12, Vol: 0.72},
		{Step: base + 5, Pitch: top, Dur: 0.12, Vol: 0.7},
		{Step: base + 8, Pitch: top, Dur: 0.12, Vol: 0.72},
		{Step: base + 11, Pitch: top, Dur: 0.5, Vol: 0.8},
		{Step: base + 14, Pitch: top, Dur: 0.12, Vol: 0.7},
	}
}
