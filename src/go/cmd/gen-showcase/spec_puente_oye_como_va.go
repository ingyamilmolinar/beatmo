package main

// Tito Puente / Santana — "Oye Como Va" (Abraxas, 1970). A dorian, 126 BPM,
// straight 4/4 cha-cha-chá. Note data from BitMidi 91454 (organ voicings match
// the canonical Rolie Am7 7-3-5-1 / rootless D9 exactly — ultimatesantana);
// genre conventions from Ethan Hein's analysis + Liberty Park Music cha-cha
// drumming pedagogy. Dossier: puente-oye-como-va_dossier.md.
//
// IMPORTANT GENRE FIX vs the old template: cha-cha-chá has NO clave and NO
// cáscara — the timekeeper is the quarter-note cha-cha bell with ghosted 8th
// upbeats, güiro long-short-short, and the conga open tones on 4 & 4-and. The
// old son-clave row was a salsa-ism and is gone.
//
// 16 bars: organ+bass intro (2) → full groove + the doubled-guitar hook (8) →
// Am unison stab break (1) → vocal phrase in 3rds (4) → stab break + timbale
// abanico into the wrap (1).
func oyeComoVaShowcase() Showcase {
	// Organ guajeo 2-bar loop (Hein's 2-3/2-4/2-3 eighth grouping); only the
	// bottom voice moves G→F#; top A6 doubling dropped (register).
	am7 := []float64{10, 15, 19, 24}
	d9 := []float64{9, 15, 19, 24}
	organLoop := concat(
		chordAt(0, 0.57, 0.62, am7...),
		chordAt(4, 0.72, 0.6, am7...),
		chordAt(10, 0.57, 0.65, am7...),
		chordAt(14, 0.74, 0.66, d9...),
		chordAt(22, 0.53, 0.6, d9...),
		chordAt(26, 0.57, 0.6, d9...),
	)
	// The syncopated unison Am stab break (2-tone rolls; 16th neighbors).
	stabBar := concat(
		chordAt(0, 0.45, 0.78, 7, 12), chordAt(4, 0.45, 0.75, 7, 12),
		chordAt(6, 0.45, 0.75, 7, 12), chordAt(10, 0.45, 0.75, 7, 12),
		chordAt(12, 0.45, 0.78, 7, 12), chordAt(14, 0.45, 0.8, 7, 12),
	)
	// Bass tumbao (+12 octave policy): root 1, root &-of-3, anticipated D on
	// the &-of-4 ringing over the barline, D3→C3 walk home.
	bassLoop := []Hit{
		{Step: 0, Pitch: 0, Dur: 1.27, Vol: 0.82}, {Step: 10, Pitch: 0, Dur: 0.7, Vol: 0.9},
		{Step: 14, Pitch: -7, Dur: 1.6, Vol: 0.82}, {Step: 22, Pitch: 0, Dur: 0.31, Vol: 0.74},
		{Step: 24, Pitch: 5, Dur: 0.85, Vol: 0.86}, {Step: 28, Pitch: 3, Dur: 0.9, Vol: 0.76},
	}
	bassStabs := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.4, Vol: 0.85}, {Step: 4, Pitch: 0, Dur: 0.4, Vol: 0.82},
		{Step: 6, Pitch: 0, Dur: 0.4, Vol: 0.82}, {Step: 10, Pitch: 0, Dur: 0.4, Vol: 0.82},
		{Step: 12, Pitch: 0, Dur: 0.4, Vol: 0.85}, {Step: 14, Pitch: 0, Dur: 0.4, Vol: 0.87},
	}
	// The hook ("the Lick" variant A-A-B-C-D-B-(G)-A + descending tail).
	phrase1 := []Hit{
		{Step: 0, Pitch: 12, Dur: 0.46, Vol: 0.88}, {Step: 4, Pitch: 12, Dur: 0.42, Vol: 0.82},
		{Step: 6, Pitch: 14, Dur: 0.43, Vol: 0.77}, {Step: 8, Pitch: 15, Dur: 0.43, Vol: 0.81},
		{Step: 10, Pitch: 17, Dur: 0.55, Vol: 0.85}, {Step: 14, Pitch: 14, Dur: 0.7, Vol: 0.78},
		{Step: 17, Pitch: 12, Dur: 1.99, Vol: 0.8},
	}
	phrase2 := []Hit{
		{Step: 32, Pitch: 12, Dur: 0.51, Vol: 0.88}, {Step: 36, Pitch: 12, Dur: 0.42, Vol: 0.81},
		{Step: 38, Pitch: 14, Dur: 0.44, Vol: 0.77}, {Step: 40, Pitch: 15, Dur: 0.43, Vol: 0.81},
		{Step: 42, Pitch: 17, Dur: 0.73, Vol: 0.88}, {Step: 46, Pitch: 14, Dur: 0.87, Vol: 0.79},
		{Step: 50, Pitch: 10, Dur: 0.52, Vol: 0.78}, {Step: 52, Pitch: 12, Dur: 0.57, Vol: 0.46},
		{Step: 54, Pitch: 5, Dur: 0.9, Vol: 0.81}, {Step: 57, Pitch: 3, Dur: 0.33, Vol: 0.59},
		{Step: 59, Pitch: -2, Dur: 0.58, Vol: 0.4},
	}
	hook8 := concat(phrase1, phrase2)
	// Vocal phrase (sax carries the chant; trumpet in parallel 3rds).
	vocal := []Hit{
		{Step: 0, Pitch: 0, Dur: 0.47, Vol: 0.88}, {Step: 2, Pitch: 5, Dur: 0.49, Vol: 0.9},
		{Step: 4, Pitch: 3, Dur: 0.49, Vol: 0.88}, {Step: 6, Pitch: 5, Dur: 0.49, Vol: 0.83},
		{Step: 8, Pitch: 0, Dur: 0.83, Vol: 0.88}, {Step: 22, Pitch: 0, Dur: 0.51, Vol: 0.83},
		{Step: 24, Pitch: 7, Dur: 1.0, Vol: 0.91}, {Step: 28, Pitch: 5, Dur: 0.77, Vol: 0.85},
		{Step: 32, Pitch: 0, Dur: 0.47, Vol: 0.88}, {Step: 34, Pitch: 5, Dur: 0.49, Vol: 0.9},
		{Step: 36, Pitch: 3, Dur: 0.49, Vol: 0.88}, {Step: 38, Pitch: 5, Dur: 0.49, Vol: 0.83},
		{Step: 40, Pitch: 0, Dur: 0.83, Vol: 0.88}, {Step: 54, Pitch: -2, Dur: 0.49, Vol: 0.93},
		{Step: 56, Pitch: 0, Dur: 0.98, Vol: 0.8}, {Step: 60, Pitch: -5, Dur: 0.52, Vol: 0.61},
	}
	harmony := []Hit{
		{Step: 0, Pitch: 3, Dur: 0.47, Vol: 0.78}, {Step: 2, Pitch: 9, Dur: 0.49, Vol: 0.8},
		{Step: 4, Pitch: 7, Dur: 0.49, Vol: 0.78}, {Step: 6, Pitch: 9, Dur: 0.49, Vol: 0.74},
		{Step: 8, Pitch: 3, Dur: 0.83, Vol: 0.78}, {Step: 22, Pitch: 7, Dur: 0.51, Vol: 0.74},
		{Step: 24, Pitch: 10, Dur: 1.0, Vol: 0.8}, {Step: 28, Pitch: 9, Dur: 0.77, Vol: 0.76},
		{Step: 32, Pitch: 3, Dur: 0.47, Vol: 0.78}, {Step: 34, Pitch: 9, Dur: 0.49, Vol: 0.8},
		{Step: 36, Pitch: 7, Dur: 0.49, Vol: 0.78}, {Step: 38, Pitch: 9, Dur: 0.49, Vol: 0.74},
		{Step: 40, Pitch: 3, Dur: 0.83, Vol: 0.78}, {Step: 54, Pitch: 2, Dur: 0.49, Vol: 0.8},
		{Step: 56, Pitch: 3, Dur: 0.98, Vol: 0.71}, {Step: 60, Pitch: -2, Dur: 0.52, Vol: 0.55},
	}
	// Percussion bars (1-bar patterns; percussion tacet during stab breaks).
	bellQ := []Hit{{Step: 0, Vol: 0.85}, {Step: 4, Vol: 0.8}, {Step: 8, Vol: 0.82}, {Step: 12, Vol: 0.8}}
	bellUp := []Hit{{Step: 2, Vol: 0.34}, {Step: 6, Vol: 0.32}, {Step: 10, Vol: 0.34}, {Step: 14, Vol: 0.32}}
	guiroBar := []Hit{
		{Step: 0, Vol: 0.68, Dur: 0.9}, {Step: 4, Vol: 0.55}, {Step: 6, Vol: 0.53},
		{Step: 8, Vol: 0.58, Dur: 0.9}, {Step: 12, Vol: 0.55}, {Step: 14, Vol: 0.5},
	}
	slapBar := []Hit{{Step: 2, Dur: 0.2, Vol: 0.85}, {Step: 4, Dur: 0.2, Vol: 0.9}, {Step: 10, Dur: 0.2, Vol: 0.85}}
	openBar := []Hit{{Step: 6, Dur: 0.35, Vol: 0.85}, {Step: 12, Dur: 0.35, Vol: 0.92}, {Step: 14, Dur: 0.35, Vol: 0.92}}
	tumbaBar := []Hit{{Step: 12, Dur: 0.3, Vol: 0.42}, {Step: 14, Dur: 0.3, Vol: 0.45}}
	pailaBar := []Hit{{Step: 0, Vol: 0.8}, {Step: 4, Vol: 0.8}, {Step: 8, Vol: 0.85}, {Step: 12, Vol: 0.88}}
	kickBar := []Hit{{Step: 0, Vol: 0.75}, {Step: 8, Vol: 0.72}, {Step: 14, Vol: 0.52}}
	hatBar := []Hit{{Step: 4, Vol: 0.72}, {Step: 12, Vol: 0.72}}
	// grooveSpan places a 1-bar percussion pattern over bars 3-10 and 12-15.
	grooveSpan := func(bar []Hit) []Hit {
		return concat(at(32, ostinato(8, bar)), at(176, ostinato(4, bar)))
	}
	// Timbale (tom substitute): pickup roll into the groove + the closing
	// abanico into the loop wrap (both sourced fill shapes).
	timbale := concat(
		[]Hit{{Step: 26, Pitch: 3, Dur: 0.2, Vol: 0.55}, {Step: 28, Pitch: 3, Dur: 0.2, Vol: 0.65}, {Step: 30, Pitch: 3, Dur: 0.2, Vol: 0.78}},
		at(240, []Hit{
			{Step: 0, Pitch: 3, Dur: 0.2, Vol: 0.6}, {Step: 1, Pitch: 3, Dur: 0.2, Vol: 0.65},
			{Step: 2, Pitch: 3, Dur: 0.2, Vol: 0.7}, {Step: 4, Pitch: 3, Dur: 0.2, Vol: 0.75},
			{Step: 5, Pitch: 3, Dur: 0.2, Vol: 0.8}, {Step: 6, Pitch: 0, Dur: 0.2, Vol: 0.7},
			{Step: 9, Pitch: 0, Dur: 0.2, Vol: 0.6}, {Step: 11, Pitch: 0, Dur: 0.2, Vol: 0.65},
			{Step: 13, Pitch: 3, Dur: 0.2, Vol: 0.85}, {Step: 15, Pitch: 0, Dur: 0.25, Vol: 0.9},
		}),
	)
	return Showcase{
		Stem: "puente-oye-como-va", BPM: 126, Subdiv: 16, Bars: 16,
		Insts: []InstSpec{
			{ID: "organ", Name: "Hammond", Volume: 0.66, Pan: -0.25, ReverbSend: 0.1},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "guitar-electric", Name: "Santana", Volume: 0.75, Pan: 0.2, ReverbSend: 0.18},
			{ID: "guitar-electric-neck", Name: "Double", Volume: 0.4, Pan: 0.35, ReverbSend: 0.18},
			{ID: "sax", Name: "Voz", Volume: 0.62, ReverbSend: 0.14},
			{ID: "trumpet-mellow", Name: "Coro 3rds", Volume: 0.48, Pan: 0.15, ReverbSend: 0.14},
			{ID: "cowbell", Name: "Cha Bell", Volume: 0.5, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "cowbell-1", Name: "Bell Up", Volume: 0.4, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "shaker", Name: "Güiro", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "conga", Name: "Slap", Volume: 0.7, SynthParams: map[string]float64{"decay": 0.5}},
			{ID: "conga-open", Name: "Open", Volume: 0.8},
			{ID: "conga-tumba", Name: "Tumba", Volume: 0.5, Pan: -0.2},
			{ID: "sidestick", Name: "Paila", Volume: 0.7, Pan: 0.25},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.85},
			{ID: "hihat", Name: "Hihat", Volume: 0.45},
			{ID: "tom-1", Name: "Timbale", Volume: 0.6, Pan: -0.3},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Organ: guajeo everywhere except the unison stab bars (11 & 16).
			{Inst: "organ", Hits: concat(
				ostinato2(10, organLoop), at(160, stabBar),
				at(176, ostinato2(4, organLoop)), at(240, stabBar),
			)},
			{Inst: "bass-guitar", Hits: concat(
				ostinato2(10, bassLoop), at(160, bassStabs),
				at(176, ostinato2(4, bassLoop)), at(240, bassStabs),
			)},
			// The doubled hook (bars 3-10) + guitars join the stab breaks.
			{Inst: "guitar-electric", Hits: concat(at(32, hook8), at(96, hook8), at(160, transposeHits(stabBar, 0)))},
			{Inst: "guitar-electric-neck", Hits: concat(at(32, hook8), at(96, hook8))},
			{Inst: "sax", Hits: at(176, vocal)},
			{Inst: "trumpet-mellow", Hits: at(176, harmony)},
			{Inst: "cowbell", Hits: grooveSpan(bellQ)},
			{Inst: "cowbell-1", Hits: grooveSpan(bellUp)},
			{Inst: "shaker", Hits: grooveSpan(guiroBar)},
			{Inst: "conga", Hits: grooveSpan(slapBar)},
			{Inst: "conga-open", Hits: grooveSpan(openBar)},
			{Inst: "conga-tumba", Hits: grooveSpan(tumbaBar)},
			{Inst: "sidestick", Hits: grooveSpan(pailaBar)},
			{Inst: "kick-acoustic", Hits: grooveSpan(kickBar)},
			{Inst: "hihat", Hits: grooveSpan(hatBar)},
			{Inst: "tom-1", Hits: timbale},
			{Inst: "crash", Hits: []Hit{{Step: 32, Vol: 0.5}, {Step: 160, Vol: 0.45}, {Step: 176, Vol: 0.45}, {Step: 240, Vol: 0.45}}},
		},
	}
}
