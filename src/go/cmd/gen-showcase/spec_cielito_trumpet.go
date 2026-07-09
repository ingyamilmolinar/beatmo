package main

// "Cielito Lindo" (Quirino Mendoza y Cortés, 1882) — mariachi vals in D major,
// 160 BPM (3/4: 12 steps/bar). Melody verbatim from the John Chambers ABC
// (trillian.mit.edu, transposed +2 to the canonical mariachi D); cross-checked
// against BitMidi 24204 which is natively in D at 160. Ensemble texture from
// mariachi pedagogy (Tucson glossary: mánico/picado/adorno; TeacherVision +
// guitarrón sources: bass on 1, strums on 2-3, octave-doubled guitarrón "on or
// a bit ahead of the beat"; felicitymazurpark: trumpets AND violins in 3rds,
// call-and-response). Dossier: cielito-trumpet_dossier.md.
//
// 32 musical bars (= Bars 24 on the 16-step grid): the "Ay, ay, ay, ay"
// refrain (16) then the "De la Sierra Morena" verse (16). No drum kit —
// the armonía IS the percussion.
func cielitoShowcase() Showcase {
	refrain := []Hit{
		{Step: 0, Pitch: 21, Dur: 3.0, Vol: 0.88}, {Step: 12, Pitch: 19, Dur: 2.0, Vol: 0.85},
		{Step: 20, Pitch: 17, Dur: 1.0, Vol: 0.82}, {Step: 24, Pitch: 14, Dur: 6.0, Vol: 0.87},
		{Step: 48, Pitch: 19, Dur: 3.0, Vol: 0.85}, {Step: 60, Pitch: 19, Dur: 2.0, Vol: 0.82},
		{Step: 68, Pitch: 17, Dur: 0.5, Vol: 0.76}, {Step: 70, Pitch: 17, Dur: 0.5, Vol: 0.76},
		{Step: 72, Pitch: 21, Dur: 1.0, Vol: 0.87}, {Step: 76, Pitch: 17, Dur: 4.0, Vol: 0.83},
		{Step: 92, Pitch: 12, Dur: 1.0, Vol: 0.72}, {Step: 96, Pitch: 14, Dur: 2.0, Vol: 0.82},
		{Step: 104, Pitch: 12, Dur: 1.0, Vol: 0.77}, {Step: 108, Pitch: 14, Dur: 1.0, Vol: 0.82},
		{Step: 112, Pitch: 14, Dur: 1.0, Vol: 0.8}, {Step: 116, Pitch: 12, Dur: 0.5, Vol: 0.74},
		{Step: 118, Pitch: 12, Dur: 0.5, Vol: 0.74}, {Step: 120, Pitch: 22, Dur: 1.0, Vol: 0.88},
		{Step: 124, Pitch: 22, Dur: 1.0, Vol: 0.87}, {Step: 128, Pitch: 19, Dur: 2.0, Vol: 0.83},
		{Step: 136, Pitch: 16, Dur: 1.0, Vol: 0.79}, {Step: 140, Pitch: 12, Dur: 1.0, Vol: 0.76},
		{Step: 144, Pitch: 14, Dur: 1.0, Vol: 0.8}, {Step: 148, Pitch: 14, Dur: 1.0, Vol: 0.8},
		{Step: 152, Pitch: 12, Dur: 2.0, Vol: 0.79}, {Step: 160, Pitch: 10, Dur: 1.0, Vol: 0.76},
		{Step: 164, Pitch: 12, Dur: 1.0, Vol: 0.77}, {Step: 168, Pitch: 9, Dur: 0.5, Vol: 0.74},
		{Step: 170, Pitch: 7, Dur: 0.5, Vol: 0.72}, {Step: 172, Pitch: 5, Dur: 5.0, Vol: 0.82},
	}
	// Trumpet 2 — diatonic 3rd below (rule sourced; note-level derivation
	// labeled, with the bars-9-10 chord-tone fix: F#4 under every B4 over D).
	refrainHarm := []float64{17, 16, 14, 10, 16, 16, 14, 14, 17, 14, 9, 9, 9, 9, 9, 9, 9, 19, 19, 16, 12, 9, 10, 10, 9, 7, 9, 5, 4, 2}
	harm := make([]Hit, len(refrain))
	for i, h := range refrain {
		h.Pitch = refrainHarm[i]
		h.Vol -= 0.1
		harm[i] = h
	}
	verse := []Hit{
		{Step: 0, Pitch: 17, Dur: 1.0, Vol: 0.83}, {Step: 4, Pitch: 17, Dur: 1.0, Vol: 0.82},
		{Step: 8, Pitch: 14, Dur: 2.0, Vol: 0.8}, {Step: 16, Pitch: 16, Dur: 1.0, Vol: 0.79},
		{Step: 20, Pitch: 12, Dur: 1.0, Vol: 0.76}, {Step: 24, Pitch: 17, Dur: 1.0, Vol: 0.83},
		{Step: 28, Pitch: 17, Dur: 1.0, Vol: 0.82}, {Step: 32, Pitch: 14, Dur: 2.0, Vol: 0.8},
		{Step: 40, Pitch: 16, Dur: 1.0, Vol: 0.79}, {Step: 44, Pitch: 12, Dur: 1.0, Vol: 0.76},
		{Step: 48, Pitch: 17, Dur: 1.0, Vol: 0.83}, {Step: 52, Pitch: 17, Dur: 1.0, Vol: 0.82},
		{Step: 56, Pitch: 14, Dur: 2.0, Vol: 0.8}, {Step: 64, Pitch: 16, Dur: 1.0, Vol: 0.79},
		{Step: 68, Pitch: 12, Dur: 1.0, Vol: 0.76}, {Step: 72, Pitch: 10, Dur: 1.0, Vol: 0.79},
		{Step: 76, Pitch: 7, Dur: 5.0, Vol: 0.8}, {Step: 96, Pitch: 16, Dur: 1.0, Vol: 0.82},
		{Step: 100, Pitch: 16, Dur: 1.0, Vol: 0.8}, {Step: 104, Pitch: 16, Dur: 1.0, Vol: 0.8},
		{Step: 108, Pitch: 16, Dur: 1.0, Vol: 0.8}, {Step: 112, Pitch: 14, Dur: 1.0, Vol: 0.79},
		{Step: 116, Pitch: 12, Dur: 1.0, Vol: 0.76}, {Step: 120, Pitch: 10, Dur: 1.0, Vol: 0.77},
		{Step: 124, Pitch: 7, Dur: 1.0, Vol: 0.74}, {Step: 128, Pitch: 7, Dur: 2.0, Vol: 0.74},
		{Step: 136, Pitch: 9, Dur: 1.0, Vol: 0.76}, {Step: 140, Pitch: 10, Dur: 1.0, Vol: 0.77},
		{Step: 144, Pitch: 12, Dur: 1.0, Vol: 0.79}, {Step: 148, Pitch: 12, Dur: 1.0, Vol: 0.79},
		{Step: 152, Pitch: 12, Dur: 2.0, Vol: 0.79}, {Step: 160, Pitch: 12, Dur: 1.5, Vol: 0.77},
		{Step: 166, Pitch: 10, Dur: 0.5, Vol: 0.74}, {Step: 168, Pitch: 9, Dur: 0.5, Vol: 0.74},
		{Step: 170, Pitch: 7, Dur: 0.5, Vol: 0.72}, {Step: 172, Pitch: 5, Dur: 5.0, Vol: 0.8},
	}
	// Guitarrón: one strong note on beat 1 of every 12-step bar (+12 octave
	// policy); slightly ahead on section downbeats (sourced tendency).
	guitarronBars := func(base int, roots []float64) []Hit {
		out := make([]Hit, len(roots))
		for i, r := range roots {
			h := Hit{Step: base + i*12, Pitch: r, Dur: 2.5, Vol: 0.85}
			if i == 0 {
				h.Groove = "rush"
				h.GroovePct = 0.5
			}
			out[i] = h
		}
		return out
	}
	refrainRoots := []float64{-7, -12, -14, -7, -7, -12, -7, -12, -7, -12, -14, -12, -12, -5, -7, -12}
	verseRoots := []float64{-7, -12, -7, -12, -7, -12, -5, -5, -12, -12, -12, -5, -12, -5, -7, -12}
	// Vihuela mánico parejo: chord rolls on beats 2 & 3 (steps 4 & 8).
	dMaj := []float64{0, 5, 9}
	a7 := []float64{4, 7, 10}
	gMaj := []float64{-2, 2, 5}
	em := []float64{-2, 2, 7}
	aMaj := []float64{4, 7, 12}
	strumBars := func(base int, chords [][]float64) []Hit {
		var out []Hit
		for i, c := range chords {
			out = append(out, chordAt(base+i*12+4, 0.6, 0.62, c...)...)
			out = append(out, chordAt(base+i*12+8, 0.6, 0.55, c...)...)
		}
		return out
	}
	refrainChords := [][]float64{dMaj, dMaj, gMaj, gMaj, dMaj, a7, dMaj, dMaj, dMaj, dMaj, gMaj, a7, a7, a7, dMaj, dMaj}
	verseChords := [][]float64{dMaj, a7, dMaj, a7, dMaj, a7, em, em, aMaj, aMaj, a7, a7, a7, a7, dMaj, dMaj}
	// Violin adornos in the held-note gaps (role sourced; pitches labeled
	// idiomatic) + soft unison doubling of the verse.
	violinFills := concat(
		chordAt(36, 1.0, 0.62, 14, 17), chordAt(40, 1.0, 0.6, 12, 16), chordAt(44, 1.0, 0.6, 10, 14),
		chordAt(84, 1.0, 0.58, 9, 12), chordAt(88, 1.0, 0.6, 12, 16),
		[]Hit{{Step: 180, Pitch: 21, Dur: 0.5, Vol: 0.63}, {Step: 182, Pitch: 19, Dur: 0.5, Vol: 0.63}, {Step: 184, Pitch: 17, Dur: 2.0, Vol: 0.65}},
	)
	verseDouble := make([]Hit, len(verse))
	for i, h := range verse {
		h.Vol -= 0.28
		verseDouble[i] = h
	}
	return Showcase{
		Stem: "cielito-trumpet", BPM: 160, Subdiv: 16, Bars: 24,
		Insts: []InstSpec{
			{ID: "trumpet", Name: "Trumpet 1", Volume: 0.75, Pan: 0.15, ReverbSend: 0.16},
			{ID: "trumpet-mellow", Name: "Trumpet 2", Volume: 0.6, Pan: -0.15, ReverbSend: 0.16},
			{ID: "bass-guitar", Name: "Guitarrón", Volume: 0.85},
			{ID: "guitar-nylon-bright", Name: "Vihuela", Volume: 0.6, Pan: 0.25, ReverbSend: 0.12},
			{ID: "guitar-nylon", Name: "Guitarra", Volume: 0.42, Pan: -0.25, ReverbSend: 0.12},
			{ID: "violin-ensemble", Name: "Violines", Volume: 0.55, Pan: -0.1, ReverbSend: 0.22},
			{ID: "shaker", Name: "Color", Volume: 0.4, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "crash", Name: "Crash", Volume: 0.45, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			{Inst: "trumpet", Hits: concat(refrain, at(192, verse))},
			// Trumpet 2 duets the refrain in 3rds; tacet for the verse.
			{Inst: "trumpet-mellow", Hits: harm},
			{Inst: "bass-guitar", Hits: concat(guitarronBars(0, refrainRoots), guitarronBars(192, verseRoots))},
			{Inst: "guitar-nylon-bright", Hits: concat(strumBars(0, refrainChords), strumBars(192, verseChords))},
			// Guitar doubles the strums an octave down, softer.
			{Inst: "guitar-nylon", Hits: transposeHits(concat(strumBars(0, refrainChords), strumBars(192, verseChords)), -12)},
			// Violins: refrain adornos, verse unison doubling.
			{Inst: "violin-ensemble", Hits: concat(violinFills, at(192, verseDouble))},
			{Inst: "shaker", Hits: repeatPattern(24, 12, []Hit{{Step: 4, Vol: 0.5}, {Step: 8, Vol: 0.42}})},
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.5}}},
		},
	}
}
