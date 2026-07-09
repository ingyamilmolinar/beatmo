package main

// Black Box — "Ride On Time" (1989). A minor, 120 BPM (exact MIDI tempo).
// Note data from BitMidi 88846 (13-track arrangement); production research:
// MusicRadar (Korg M1 "Piano 8" riff, TR-909 kit, dark square/saw bass),
// Wikipedia (Groove Groove Melody; Loleatta Holloway "Love Sensation" vocal,
// "Love's Theme" strings). Dossier: blackbox-ride-on-time_dossier.md.
//
// 16-bar arc: bars 1–4 the held diva hook over the bass/909/organ-stab groove;
// bars 5–8 the M1 piano riff + string stabs (groove voicing Am–C–F–G);
// bars 9–12 the ICONIC intro voicing (true Am–G–F–G change) + sparse vocal
// re-trigger chops (labeled idiomatic); bars 13–16 riff again, closed by the
// rising 16th vocal run that lands on the loop wrap.
//
// GROOVE: 909 drums are machine-straight; the italo "eagerness" is the
// written-in and-of-4 chord anticipations plus a slight rush on the piano
// stabs (measured −5 ticks ≈ 7ms ahead → rush groove 0.06).
func rideOnTimeShowcase() Showcase {
	// Vocal hook (bars 1–4): the held "diva" phrase, verbatim.
	hook := []Hit{
		{Step: 0, Pitch: 12, Dur: 3.23, Vol: 0.99}, {Step: 13, Pitch: 14, Dur: 0.16, Vol: 0.86},
		{Step: 14, Pitch: 15, Dur: 0.17, Vol: 0.88}, {Step: 15, Pitch: 17, Dur: 0.18, Vol: 0.85},
		{Step: 16, Pitch: 19, Dur: 2.71, Vol: 0.96}, {Step: 27, Pitch: 24, Dur: 1.22, Vol: 0.98},
		{Step: 32, Pitch: 22, Dur: 3.51, Vol: 1.0}, {Step: 46, Pitch: 19, Dur: 0.18, Vol: 0.88},
		{Step: 47, Pitch: 17, Dur: 0.17, Vol: 0.77}, {Step: 48, Pitch: 19, Dur: 0.45, Vol: 0.99},
		{Step: 50, Pitch: 17, Dur: 0.2, Vol: 0.81}, {Step: 51, Pitch: 15, Dur: 0.18, Vol: 0.68},
		{Step: 52, Pitch: 17, Dur: 0.52, Vol: 0.98}, {Step: 54, Pitch: 15, Dur: 0.16, Vol: 0.82},
		{Step: 55, Pitch: 14, Dur: 0.17, Vol: 0.75}, {Step: 56, Pitch: 15, Dur: 0.48, Vol: 0.99},
		{Step: 58, Pitch: 10, Dur: 0.21, Vol: 0.76}, {Step: 59, Pitch: 7, Dur: 0.25, Vol: 0.65},
		{Step: 60, Pitch: 10, Dur: 0.73, Vol: 0.96}, {Step: 63, Pitch: 7, Dur: 0.21, Vol: 0.67},
	}
	// The famous rising 16th run (intro bars 2–3 of the MIDI) — used here as
	// the bar-16 fill landing on the loop wrap.
	risingRun := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.24, Vol: 0.7}, {Step: 1, Pitch: 3, Dur: 0.24, Vol: 0.72},
		{Step: 2, Pitch: 5, Dur: 0.24, Vol: 0.74}, {Step: 3, Pitch: 7, Dur: 0.24, Vol: 0.76},
		{Step: 4, Pitch: 8, Dur: 0.24, Vol: 0.78}, {Step: 5, Pitch: 10, Dur: 0.24, Vol: 0.8},
		{Step: 6, Pitch: 12, Dur: 0.24, Vol: 0.83}, {Step: 7, Pitch: 14, Dur: 0.24, Vol: 0.86},
		{Step: 8, Pitch: 15, Dur: 0.24, Vol: 0.89}, {Step: 9, Pitch: 17, Dur: 0.24, Vol: 0.92},
		{Step: 10, Pitch: 20, Dur: 0.24, Vol: 0.95}, {Step: 11, Pitch: 23, Dur: 0.3, Vol: 0.97},
		{Step: 12, Pitch: 24, Dur: 1.0, Vol: 1.0},
	}
	// M1 piano groove cell (2 bars, Am–C | F–C/G–G), stab rolls + LH bass
	// notes; and-of-4 pushes written in.
	grooveCell := concat(
		chordAt(0, 0.3, 0.9, 0, 7),
		[]Hit{{Step: 2, Pitch: -12, Dur: 0.25, Vol: 0.55}},
		chordAt(4, 0.3, 0.82, 0, 7),
		[]Hit{{Step: 7, Pitch: -12, Dur: 0.25, Vol: 0.48}},
		chordAt(8, 0.3, 0.72, 3, 7, 10),
		chordAt(11, 0.3, 0.72, 7, 10),
		[]Hit{{Step: 13, Pitch: -14, Dur: 0.25, Vol: 0.38}},
		chordAt(14, 0.8, 0.82, 3, 8),
		[]Hit{{Step: 16, Pitch: -16, Dur: 0.4, Vol: 0.8}},
		chordAt(18, 0.3, 0.68, 3, 8),
		chordAt(20, 0.3, 0.82, 3, 10),
		chordAt(22, 0.3, 0.62, 3, 8),
		chordAt(24, 0.3, 0.78, 3, 7),
		chordAt(26, 0.3, 0.62, 2, 5),
		[]Hit{{Step: 28, Pitch: 3, Dur: 0.25, Vol: 0.72}},
		chordAt(29, 0.3, 0.76, 2, 5),
		[]Hit{{Step: 31, Pitch: -14, Dur: 0.25, Vol: 0.54}},
	)
	// Intro cell — the "true" Am–G–F–G change (the record's iconic opening).
	introCell := concat(
		chordAt(0, 0.35, 0.88, 7, 12),
		[]Hit{{Step: 2, Pitch: -12, Dur: 0.25, Vol: 0.55}},
		chordAt(4, 0.35, 0.85, 3, 7, 12),
		[]Hit{{Step: 7, Pitch: -14, Dur: 0.25, Vol: 0.5}},
		chordAt(8, 0.35, 0.82, 2, 5, 10),
		chordAt(11, 0.3, 0.78, 5, 10),
		[]Hit{{Step: 13, Pitch: -16, Dur: 0.25, Vol: 0.5}},
		chordAt(14, 0.8, 0.82, 3, 8),
		[]Hit{{Step: 16, Pitch: -16, Dur: 0.4, Vol: 0.79}},
		chordAt(18, 0.3, 0.72, 0, 3),
		chordAt(20, 0.3, 0.78, 3, 10),
		chordAt(22, 0.3, 0.7, 0, 8),
		chordAt(24, 0.3, 0.75, 3, 7),
		chordAt(26, 0.3, 0.66, 2, 5),
		[]Hit{{Step: 28, Pitch: 3, Dur: 0.25, Vol: 0.7}},
		chordAt(29, 0.3, 0.74, 2, 5),
		[]Hit{{Step: 31, Pitch: -14, Dur: 0.25, Vol: 0.52}},
	)
	// Dark synth bass, 2-bar loop, verbatim.
	bassCell := []Hit{
		{Step: 0, Pitch: -24, Dur: 0.57, Vol: 0.9}, {Step: 3, Pitch: -24, Dur: 0.2, Vol: 0.52},
		{Step: 4, Pitch: -24, Dur: 0.23, Vol: 0.86}, {Step: 5, Pitch: -24, Dur: 0.19, Vol: 0.75},
		{Step: 7, Pitch: -24, Dur: 0.24, Vol: 0.64}, {Step: 8, Pitch: -21, Dur: 0.5, Vol: 0.77},
		{Step: 10, Pitch: -24, Dur: 0.5, Vol: 0.71}, {Step: 12, Pitch: -21, Dur: 0.49, Vol: 0.72},
		{Step: 14, Pitch: -29, Dur: 0.46, Vol: 0.62}, {Step: 16, Pitch: -28, Dur: 0.6, Vol: 0.83},
		{Step: 19, Pitch: -28, Dur: 0.21, Vol: 0.69}, {Step: 21, Pitch: -28, Dur: 0.21, Vol: 0.62},
		{Step: 23, Pitch: -28, Dur: 0.2, Vol: 0.65}, {Step: 24, Pitch: -26, Dur: 0.48, Vol: 0.8},
		{Step: 26, Pitch: -29, Dur: 0.48, Vol: 0.63}, {Step: 28, Pitch: -26, Dur: 0.46, Vol: 0.82},
		{Step: 30, Pitch: -29, Dur: 0.25, Vol: 0.78}, {Step: 31, Pitch: -26, Dur: 0.25, Vol: 0.63},
	}
	// String-stab counterline (top notes of the "Love's Theme"-style hits).
	stabCell := []Hit{
		{Step: 1, Pitch: 19, Dur: 0.25, Vol: 0.54}, {Step: 3, Pitch: 19, Dur: 0.25, Vol: 0.81},
		{Step: 4, Pitch: 19, Dur: 0.67, Vol: 0.9}, {Step: 7, Pitch: 19, Dur: 0.25, Vol: 0.8},
		{Step: 8, Pitch: 22, Dur: 0.25, Vol: 0.87}, {Step: 9, Pitch: 22, Dur: 0.25, Vol: 0.66},
		{Step: 11, Pitch: 19, Dur: 0.25, Vol: 0.84}, {Step: 12, Pitch: 22, Dur: 0.25, Vol: 0.87},
		{Step: 14, Pitch: 20, Dur: 0.25, Vol: 0.91}, {Step: 17, Pitch: 20, Dur: 0.25, Vol: 0.99},
		{Step: 18, Pitch: 15, Dur: 0.25, Vol: 0.87}, {Step: 21, Pitch: 15, Dur: 0.25, Vol: 0.72},
		{Step: 23, Pitch: 22, Dur: 0.25, Vol: 0.69}, {Step: 25, Pitch: 22, Dur: 0.25, Vol: 0.79},
		{Step: 27, Pitch: 22, Dur: 0.25, Vol: 0.67}, {Step: 29, Pitch: 10, Dur: 0.25, Vol: 0.91},
		{Step: 30, Pitch: 22, Dur: 0.53, Vol: 0.72},
	}
	return Showcase{
		Stem: "blackbox-ride-on-time", BPM: 120, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "voice-soprano", Name: "Diva", Volume: 0.68, ReverbSend: 0.22},
			{ID: "piano-grand", Name: "M1 Piano", Volume: 0.72, Pan: 0.1, ReverbSend: 0.14},
			{ID: "bass-fm", Name: "Synth Bass", Volume: 0.85},
			{ID: "violin-ensemble", Name: "Stabs", Volume: 0.52, Pan: -0.2, ReverbSend: 0.2},
			{ID: "organ", Name: "Stab Pad", Volume: 0.4, Pan: 0.25},
			{ID: "kick-electro", Name: "Kick", Volume: 1.0},
			{ID: "hihat-pedal", Name: "Pedal Hat", Volume: 0.45},
			{ID: "hihat", Name: "Hihat", Volume: 0.55},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.7, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "clap", Name: "909 Clap", Volume: 0.75, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "snare-1", Name: "Push", Volume: 0.6},
			{ID: "shaker", Name: "Maracas", Volume: 0.4, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "rimshot", Name: "Zap", Volume: 0.35},
			{ID: "crash", Name: "Crash", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Diva: hook (bars 1–4) → re-trigger chops (bars 9–10, idiomatic,
			// see dossier UNSOURCED) → the rising run landing the loop wrap.
			{Inst: "voice-soprano", Hits: concat(
				hook,
				[]Hit{{Step: 128, Pitch: 12, Dur: 0.2, Vol: 0.6}, {Step: 130, Pitch: 12, Dur: 0.2, Vol: 0.55}, {Step: 132, Pitch: 12, Dur: 0.2, Vol: 0.6}, {Step: 134, Pitch: 12, Dur: 0.2, Vol: 0.55},
					{Step: 144, Pitch: 12, Dur: 0.2, Vol: 0.6}, {Step: 146, Pitch: 12, Dur: 0.2, Vol: 0.55}, {Step: 148, Pitch: 12, Dur: 0.4, Vol: 0.62}},
				at(240, risingRun),
			)},
			// Piano: groove riff bars 5–8, intro change bars 9–12, riff 13–16.
			// Pushed slightly ahead (measured −7ms) — the italo eagerness.
			{Inst: "piano-grand", Hits: pushed(concat(
				at(64, grooveCell), at(96, grooveCell),
				at(128, introCell), at(160, introCell),
				at(192, grooveCell), at(224, grooveCell),
			), 0.06)},
			// +12: bass-fm renders one octave down; re-encoded to true record pitch.
			{Inst: "bass-fm", Hits: ostinato2(16, transposeHits(bassCell, 12))},
			{Inst: "violin-ensemble", Hits: concat(at(64, stabCell), at(96, stabCell), at(192, stabCell), at(224, stabCell))},
			// Gated organ-stab 16ths tracking the chords (root/5th alternation,
			// strong beats / weak off-16ths).
			{Inst: "organ", Hits: concat(
				stabPadBars(0, 4, 0, 7),
				stabPadBars(64, 1, 0, 7), stabPadBars(80, 1, 3, 8), stabPadBars(96, 1, 0, 7), stabPadBars(112, 1, 3, 8),
				stabPadBars(128, 1, 0, 7), stabPadBars(144, 1, 2, 5), stabPadBars(160, 1, 3, 8), stabPadBars(176, 1, 2, 5),
				stabPadBars(192, 1, 0, 7), stabPadBars(208, 1, 3, 8), stabPadBars(224, 1, 0, 7), stabPadBars(240, 1, 3, 8),
			)},
			{Inst: "kick-electro", Hits: ostinato(16, []Hit{{Step: 0, Vol: 1.0}, {Step: 4, Vol: 1.0}, {Step: 8, Vol: 1.0}, {Step: 12, Vol: 1.0}})},
			{Inst: "hihat-pedal", Hits: ostinato(16, []Hit{{Step: 0, Vol: 0.45}, {Step: 4, Vol: 0.45}, {Step: 8, Vol: 0.45}, {Step: 12, Vol: 0.45}})},
			{Inst: "hihat", Hits: ostinato(16, []Hit{{Step: 1, Vol: 0.6}, {Step: 5, Vol: 0.6}, {Step: 9, Vol: 0.6}, {Step: 13, Vol: 0.6}})},
			{Inst: "hihat-1", Hits: ostinato(16, []Hit{{Step: 2, Vol: 0.85}, {Step: 6, Vol: 0.8}, {Step: 10, Vol: 0.85}, {Step: 14, Vol: 0.8}})},
			{Inst: "clap", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.75}, {Step: 12, Vol: 0.75}})},
			// Snare mini-fill pushes into bars 2 & 4 of each 4-bar phrase.
			{Inst: "snare-1", Hits: repeatPattern(16, 64, []Hit{{Step: 14, Vol: 0.68}, {Step: 15, Vol: 0.75}, {Step: 46, Vol: 0.68}, {Step: 47, Vol: 0.75}})},
			// Maracas: every 16th except the open-hat offbeats, beat accents.
			{Inst: "shaker", Hits: ostinato(16, []Hit{
				{Step: 0, Vol: 0.7}, {Step: 1, Vol: 0.4}, {Step: 3, Vol: 0.4}, {Step: 4, Vol: 0.7},
				{Step: 5, Vol: 0.4}, {Step: 7, Vol: 0.4}, {Step: 8, Vol: 0.7}, {Step: 9, Vol: 0.4},
				{Step: 11, Vol: 0.4}, {Step: 12, Vol: 0.7}, {Step: 13, Vol: 0.4}, {Step: 15, Vol: 0.45},
			})},
			{Inst: "rimshot", Hits: ostinato(16, []Hit{{Step: 4, Vol: 0.35}, {Step: 12, Vol: 0.35}})},
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.55}, {Step: 64, Vol: 0.5}, {Step: 128, Vol: 0.5}, {Step: 192, Vol: 0.5}}},
		},
	}
}

// stabPadBars emits `bars` bars of the gated 16th stab-pad texture from
// `base`: root tone on even 16ths (strong on beats), fifth tone on odd 16ths.
func stabPadBars(base, bars int, root, fifth float64) []Hit {
	var out []Hit
	for b := 0; b < bars; b++ {
		for s := 0; s < 16; s++ {
			h := Hit{Step: base + b*16 + s, Dur: 0.2}
			if s%2 == 0 {
				h.Pitch = root
				h.Vol = 0.55
				if s%4 == 0 {
					h.Vol = 0.62
					h.Dur = 0.44
				}
			} else {
				h.Pitch = fifth
				h.Vol = 0.32
			}
			out = append(out, h)
		}
	}
	return out
}
