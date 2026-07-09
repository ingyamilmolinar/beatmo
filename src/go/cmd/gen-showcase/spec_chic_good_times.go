package main

// Chic — "Good Times" (1979). 16-bar arc in the A-centric key of the source
// MIDI (bitmidi 23412, 117 BPM; canonical record is E minor ~110 — the MIDI is
// transposed +P4 with halved note values; kept, as all note data lives in its
// grid). Research dossier: scratchpad chic-good-times_dossier.md (talkingbass
// bassline analysis, Wikipedia personnel, djrobblog breakdown timeline).
//
// Arc: bars 1–8 full-groove chorus (bass + both guitars + string/brass stabs +
// Figueroa percussion bed) → bars 9–12 the "clap your hands" chant turnaround
// (guitars tacet, rolled e-piano chords, group-chant substitute) → bars 13–16
// the famous 3:12 breakdown (bass + drums + claps carry; sustained e-piano
// chords, trem-string pulses, rising pad; string fill cues the loop re-entry).
//
// Groove: the MIDI is fully quantized straight — the pocket is articulation,
// not timing (staccato gates, ghost-chuck velocity contrast). No groove fields.
func goodTimesShowcase() Showcase {
	// Bernard Edwards' 2-bar figure, verbatim (Am7 bar + D bar): the E–G–G#→A
	// chromatic walk closes bar A; the C#→D anticipation + G pickup close bar B.
	bassLoop := []Hit{
		{Step: 0, Pitch: -24, Dur: 0.62, Vol: 0.92}, {Step: 4, Pitch: -24, Dur: 0.5, Vol: 0.9},
		{Step: 7, Pitch: -26, Dur: 0.45, Vol: 0.9}, {Step: 9, Pitch: -29, Dur: 0.2, Vol: 0.85},
		{Step: 10, Pitch: -26, Dur: 0.45, Vol: 0.9}, {Step: 12, Pitch: -25, Dur: 0.45, Vol: 0.9},
		{Step: 14, Pitch: -24, Dur: 0.45, Vol: 0.92}, {Step: 16, Pitch: -19, Dur: 0.58, Vol: 0.92},
		{Step: 20, Pitch: -19, Dur: 0.45, Vol: 0.9}, {Step: 22, Pitch: -24, Dur: 0.2, Vol: 0.85},
		{Step: 23, Pitch: -21, Dur: 0.45, Vol: 0.9}, {Step: 25, Pitch: -21, Dur: 0.2, Vol: 0.85},
		{Step: 26, Pitch: -24, Dur: 0.45, Vol: 0.9}, {Step: 28, Pitch: -20, Dur: 0.2, Vol: 0.88},
		{Step: 29, Pitch: -19, Dur: 0.45, Vol: 0.92}, {Step: 31, Pitch: -26, Dur: 0.2, Vol: 0.72},
	}
	// Nile Rodgers chucks — dyad on accents, dead ghost scratches between
	// (vel contrast IS the chuck; ghosts stay tiny). Am = C4/E4, sus = B3/D4,
	// Dm = F4/A4 (top two of the MIDI triple-stops; the roll uses 2 tones so
	// dense 16th neighbors never collide).
	chuckLoop := concat(
		chordAt(0, 0.35, 0.62, 3, 7),
		[]Hit{{Step: 2, Pitch: 7, Dur: 0.12, Vol: 0.58}, {Step: 3, Pitch: 7, Dur: 0.05, Vol: 0.24}, {Step: 5, Pitch: 7, Dur: 0.05, Vol: 0.24}},
		chordAt(6, 0.2, 0.62, 3, 7),
		chordAt(10, 0.35, 0.58, 2, 5),
		[]Hit{{Step: 14, Pitch: 5, Dur: 0.2, Vol: 0.4}},
		chordAt(16, 0.3, 0.62, 8, 12),
		[]Hit{{Step: 18, Pitch: 12, Dur: 0.12, Vol: 0.58}, {Step: 19, Pitch: 12, Dur: 0.05, Vol: 0.24}, {Step: 21, Pitch: 12, Dur: 0.08, Vol: 0.26}},
		chordAt(22, 0.2, 0.62, 8, 12),
		[]Hit{{Step: 25, Pitch: 12, Dur: 0.2, Vol: 0.4}},
		chordAt(26, 0.3, 0.62, 3, 7),
		chordAt(29, 0.3, 0.58, 3, 7),
	)
	// Funk single-note counter line (neck pickup), verbatim from track 7.
	funkLoop := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.17, Vol: 0.55}, {Step: 1, Pitch: -7, Dur: 0.25, Vol: 0.32},
		{Step: 3, Pitch: 0, Dur: 0.17, Vol: 0.55}, {Step: 4, Pitch: -7, Dur: 0.25, Vol: 0.32},
		{Step: 6, Pitch: 0, Dur: 0.17, Vol: 0.48}, {Step: 7, Pitch: 0, Dur: 0.17, Vol: 0.3},
		{Step: 9, Pitch: -7, Dur: 0.17, Vol: 0.5}, {Step: 10, Pitch: 0, Dur: 0.29, Vol: 0.55},
		{Step: 13, Pitch: -2, Dur: 0.33, Vol: 0.55}, {Step: 15, Pitch: -2, Dur: 0.17, Vol: 0.38},
		{Step: 16, Pitch: 0, Dur: 0.17, Vol: 0.55}, {Step: 17, Pitch: -7, Dur: 0.25, Vol: 0.33},
		{Step: 19, Pitch: 0, Dur: 0.17, Vol: 0.55}, {Step: 20, Pitch: -7, Dur: 0.25, Vol: 0.33},
		{Step: 22, Pitch: 0, Dur: 0.17, Vol: 0.49}, {Step: 23, Pitch: -7, Dur: 0.17, Vol: 0.31},
		{Step: 25, Pitch: -7, Dur: 0.17, Vol: 0.48}, {Step: 26, Pitch: -7, Dur: 0.17, Vol: 0.23},
		{Step: 27, Pitch: -7, Dur: 0.17, Vol: 0.43}, {Step: 28, Pitch: 0, Dur: 0.25, Vol: 0.52},
		{Step: 30, Pitch: -2, Dur: 0.45, Vol: 0.55},
	}
	// Chic Strings Charleston stabs (and-of-1 area, steps 4 & 7 of each bar).
	stringStabs := concat(
		chordAt(4, 0.55, 0.6, 3, 7, 19),
		chordAt(7, 0.2, 0.55, 2, 5, 17),
		chordAt(20, 0.55, 0.6, 8, 12, 20),
		chordAt(23, 0.2, 0.55, 3, 7, 19),
	)
	// Brass doubles only the stab top line.
	brassLoop := []Hit{
		{Step: 4, Pitch: 19, Dur: 0.55, Vol: 0.72}, {Step: 7, Pitch: 17, Dur: 0.2, Vol: 0.78},
		{Step: 20, Pitch: 20, Dur: 0.55, Vol: 0.72}, {Step: 23, Pitch: 19, Dur: 0.2, Vol: 0.78},
	}
	// Tambourine-proxy 16th pulse: loud offbeat accents at 2 & 10, soft odd 16ths.
	tambBar := []Hit{
		{Step: 0, Vol: 0.5}, {Step: 1, Vol: 0.32}, {Step: 2, Vol: 0.78}, {Step: 3, Vol: 0.32},
		{Step: 4, Vol: 0.5}, {Step: 5, Vol: 0.32}, {Step: 6, Vol: 0.5}, {Step: 7, Vol: 0.32},
		{Step: 8, Vol: 0.5}, {Step: 9, Vol: 0.32}, {Step: 10, Vol: 0.78}, {Step: 11, Vol: 0.32},
		{Step: 12, Vol: 0.5}, {Step: 13, Vol: 0.32}, {Step: 14, Vol: 0.5}, {Step: 15, Vol: 0.32},
	}
	// Sammy Figueroa's conga bed (velocities from the MIDI, same both bars).
	congaBar := []Hit{
		{Step: 0, Dur: 0.3, Vol: 0.5}, {Step: 2, Dur: 0.3, Vol: 0.5}, {Step: 4, Dur: 0.25, Vol: 0.3},
		{Step: 6, Dur: 0.25, Vol: 0.4}, {Step: 7, Dur: 0.3, Vol: 0.47}, {Step: 9, Dur: 0.25, Vol: 0.35},
		{Step: 10, Dur: 0.2, Vol: 0.22}, {Step: 12, Dur: 0.3, Vol: 0.45}, {Step: 14, Dur: 0.25, Vol: 0.38},
	}
	return Showcase{
		Stem: "chic-good-times", BPM: 117, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "bass-guitar", Name: "Bass", Volume: 0.88},
			{ID: "guitar-electric", Name: "Chuck", Volume: 0.58, Pan: 0.25, ReverbSend: 0.06},
			{ID: "guitar-electric-neck", Name: "Funk Gtr", Volume: 0.5, Pan: -0.3, ReverbSend: 0.06},
			{ID: "violin-ensemble", Name: "Strings", Volume: 0.52, Pan: -0.2, ReverbSend: 0.24},
			{ID: "trumpet", Name: "Brass", Volume: 0.55, Pan: 0.2, ReverbSend: 0.14},
			{ID: "fm-epiano", Name: "E-Piano", Volume: 0.55, ReverbSend: 0.12},
			{ID: "modular-pad", Name: "Pad", Volume: 0.38, Pan: 0.15, ReverbSend: 0.28},
			{ID: "ensemble-lead", Name: "Chant", Volume: 0.52, ReverbSend: 0.2},
			{ID: "kick", Name: "Kick", Volume: 0.92},
			{ID: "snare", Name: "Snare", Volume: 0.75},
			{ID: "snare-ghost", Name: "Ghost", Volume: 0.5},
			{ID: "clap", Name: "Clap", Volume: 0.7, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "hihat", Name: "Tamb", Volume: 0.55},
			{ID: "shaker", Name: "Shaker", Volume: 0.38, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga-open", Name: "Conga", Volume: 0.5, Pan: 0.3},
		},
		Rows: []RowSpec{
			// Bass plays the verbatim loop through all 16 bars (it carries the
			// chant AND the breakdown — that's the record's arrangement).
			// +12: bass-guitar renders one octave down (110Hz at pitch 0), so the
			// MIDI octave is re-encoded up to render at the record's true pitch.
			{Inst: "bass-guitar", Hits: ostinato2(16, transposeHits(bassLoop, 12))},
			// Both guitars: full-groove bars 1–8 only, tacet after (chant +
			// breakdown strip them out).
			{Inst: "guitar-electric", Hits: ostinato2(8, chuckLoop)},
			{Inst: "guitar-electric-neck", Hits: ostinato2(8, funkLoop)},
			// Strings: chorus stabs bars 1–8; trem pulses + the descending
			// re-entry cue fill in the breakdown (bars 13–16).
			{Inst: "violin-ensemble", Hits: concat(
				ostinato2(8, stringStabs),
				// Trem pulses at 0,4,7,10 of each breakdown bar (top voice).
				at(192, []Hit{{Step: 0, Pitch: 12, Dur: 0.29, Vol: 0.45}, {Step: 4, Pitch: 12, Dur: 0.29, Vol: 0.42}, {Step: 7, Pitch: 12, Dur: 0.29, Vol: 0.42}, {Step: 10, Pitch: 12, Dur: 0.29, Vol: 0.42}}),
				at(208, []Hit{{Step: 0, Pitch: 12, Dur: 0.29, Vol: 0.45}, {Step: 4, Pitch: 12, Dur: 0.29, Vol: 0.42}, {Step: 7, Pitch: 12, Dur: 0.29, Vol: 0.42}, {Step: 10, Pitch: 12, Dur: 0.29, Vol: 0.42}}),
				at(224, []Hit{{Step: 0, Pitch: 8, Dur: 0.29, Vol: 0.45}, {Step: 4, Pitch: 8, Dur: 0.29, Vol: 0.42}, {Step: 7, Pitch: 8, Dur: 0.29, Vol: 0.42}, {Step: 10, Pitch: 8, Dur: 0.29, Vol: 0.42}}),
				at(240, []Hit{{Step: 0, Pitch: 7, Dur: 0.29, Vol: 0.45}, {Step: 4, Pitch: 7, Dur: 0.29, Vol: 0.42}, {Step: 7, Pitch: 7, Dur: 0.29, Vol: 0.42}}),
				// "Sharp descending notes" cue the re-entry (djrobblog).
				at(252, []Hit{{Step: 0, Pitch: 5, Dur: 0.2, Vol: 0.55}, {Step: 1, Pitch: 2, Dur: 0.2, Vol: 0.55}, {Step: 2, Pitch: -1, Dur: 0.2, Vol: 0.58}}),
			)},
			{Inst: "trumpet", Hits: ostinato2(8, brassLoop)},
			// E-piano: rolled chant-turnaround chords (bars 9–12: Fmaj7 Dm7
			// Bm7b5/A E), then sustained breakdown chords (bars 13–16: Fmaj7
			// Dm7 Bø7 Esus→E — resolving into the loop wrap to Am).
			{Inst: "fm-epiano", Hits: concat(
				chordAt(128, 2.3, 0.6, -4, 0, 3, 7),
				chordAt(144, 2.3, 0.6, -7, -4, 0, 3),
				chordAt(160, 2.3, 0.6, 0, 2, 5, 8),
				chordAt(176, 2.3, 0.6, -5, -1, 2, 7),
				chordAt(192, 3.9, 0.62, 0, 3, 7, 8),
				chordAt(208, 3.9, 0.62, 0, 3, 5, 8),
				chordAt(224, 3.9, 0.62, 0, 2, 5, 8),
				chordAt(240, 1.9, 0.62, 0, 2, 7),
				chordAt(248, 1.9, 0.62, -1, 2, 7),
			)},
			// Pad: chant top notes, then the rising breakdown line A4→C5→D5→E5.
			{Inst: "modular-pad", Hits: []Hit{
				{Step: 128, Pitch: 0, Dur: 3.8, Vol: 0.4}, {Step: 144, Pitch: 3, Dur: 3.8, Vol: 0.4},
				{Step: 160, Pitch: 5, Dur: 3.8, Vol: 0.4}, {Step: 176, Pitch: 7, Dur: 3.8, Vol: 0.4},
				{Step: 192, Pitch: 12, Dur: 3.8, Vol: 0.42}, {Step: 208, Pitch: 15, Dur: 3.8, Vol: 0.44},
				{Step: 224, Pitch: 17, Dur: 3.8, Vol: 0.46}, {Step: 240, Pitch: 19, Dur: 3.8, Vol: 0.5},
			}},
			// Group chant (vocal substitute; labeled idiomatic in the dossier):
			// monotone root quarter-pairs following the turnaround, G# on the E bar.
			{Inst: "ensemble-lead", Hits: []Hit{
				{Step: 128, Pitch: 0, Dur: 0.8, Vol: 0.52}, {Step: 132, Pitch: 0, Dur: 0.8, Vol: 0.5},
				{Step: 144, Pitch: 0, Dur: 0.8, Vol: 0.52}, {Step: 148, Pitch: 0, Dur: 0.8, Vol: 0.5},
				{Step: 160, Pitch: 0, Dur: 0.8, Vol: 0.52}, {Step: 164, Pitch: 0, Dur: 0.8, Vol: 0.5},
				{Step: 176, Pitch: -1, Dur: 0.8, Vol: 0.52}, {Step: 180, Pitch: -1, Dur: 0.8, Vol: 0.5},
			}},
			// Tony Thompson: strict four-on-the-floor, backbeat snare.
			{Inst: "kick", Hits: ostinato(16, []Hit{{Step: 0, Vol: 0.92}, {Step: 4, Vol: 0.9}, {Step: 8, Vol: 0.92}, {Step: 12, Vol: 0.9}})},
			{Inst: "snare", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.75}, {Step: 12, Vol: 0.75}})},
			// Press-fill ghosts closing every 2-bar loop (bar B steps 25–27).
			{Inst: "snare-ghost", Hits: ostinato2(16, []Hit{
				{Step: 25, Vol: 0.3}, {Step: 26, Vol: 0.36}, {Step: 27, Vol: 0.36},
			})},
			// Claps join for the chant + breakdown (the record's identity).
			{Inst: "clap", Hits: at(128, ostinato(8, []Hit{{Step: 4, Vol: 0.85}, {Step: 12, Vol: 0.85}}))},
			{Inst: "hihat", Hits: ostinato(16, tambBar)},
			// Cabasa/shaker bed + conga: full-groove bars only (their dropout
			// is what makes the chant/breakdown feel stripped).
			{Inst: "shaker", Hits: hatSixteenths(8, 0.38)},
			{Inst: "conga-open", Hits: ostinato(8, congaBar)},
		},
	}
}
