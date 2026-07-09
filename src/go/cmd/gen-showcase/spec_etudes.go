package main

// Batch 4 — the seven small "etude" templates, expanded from 2-bar loops into
// 8-12-bar mini-arrangements. No specific recording is transcribed; every
// pattern is genre-idiomatic, assembled from fetched pedagogy sources
// (etudes_dossier.md cites each: Lemaire minor-blues form, jazznightschool
// swing kit, guitarwiz Travis picking, drumming.com train beat, Blackstar
// riff cells, MusicRadar clav grammar, drumlessons ghost placements,
// talkingbass funk-formula, scphillips son-montuno grids, salsadiary maracas).
// felt-prelude is derived from the real BWV 846 score (first 8 bars).

// trimBars drops hits at or beyond `bars` and clamps the showcase length.
func trimBars(sh Showcase, bars int) Showcase {
	limit := bars * 16
	for ri := range sh.Rows {
		var kept []Hit
		for _, h := range sh.Rows[ri].Hits {
			if h.Step < limit {
				kept = append(kept, h)
			}
		}
		sh.Rows[ri].Hits = kept
	}
	sh.Bars = bars
	return sh
}

// ── felt-prelude: BWV 846 mm.1-8 on the felt piano ──────────────────────────
func feltPreludeShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(preludeCXML, "bwv846-felt"), "felt-prelude", 16, []InstSpec{
		{ID: "piano-felt", Name: "Felt Piano", Volume: 0.82, ReverbSend: 0.26},
	})
	sh.BPM = 60
	sh = trimBars(sh, 8)
	sh = capDuplicateIDs(sh, map[string]string{"piano-felt": "piano-grand"})
	return sh
}

// ── sax-blues: A-minor 12-bar blues (Lemaire form 2), AAB head ───────────────
func saxBluesShowcase() Showcase {
	// A-phrase riff (2 bars) and the B answer with the Eb→D blue-note resolve.
	riffA := []Hit{
		{Step: 0, Pitch: 7, Dur: 0.5, Vol: 0.71}, {Step: 2, Pitch: 10, Dur: 0.5, Vol: 0.75},
		{Step: 4, Pitch: 12, Dur: 1.0, Vol: 0.91}, {Step: 8, Pitch: 15, Dur: 0.5, Vol: 0.79},
		{Step: 10, Pitch: 12, Dur: 0.5, Vol: 0.71}, {Step: 12, Pitch: 10, Dur: 1.0, Vol: 0.75},
		{Step: 16, Pitch: 12, Dur: 2.0, Vol: 0.87},
	}
	answerB := []Hit{
		{Step: 0, Pitch: 15, Dur: 0.5, Vol: 0.83}, {Step: 2, Pitch: 18, Dur: 0.5, Vol: 0.87},
		{Step: 4, Pitch: 17, Dur: 1.0, Vol: 0.79}, {Step: 8, Pitch: 15, Dur: 0.5, Vol: 0.75},
		{Step: 12, Pitch: 12, Dur: 1.0, Vol: 0.79}, {Step: 16, Pitch: 10, Dur: 1.0, Vol: 0.75},
		{Step: 20, Pitch: 7, Dur: 1.0, Vol: 0.71}, {Step: 24, Pitch: 12, Dur: 2.0, Vol: 0.83},
	}
	// Walking bass: quarter notes, chromatic approaches (rules cited).
	walk := []float64{
		-12, -9, -5, -2 /*Am*/, 0, -2, -5, -9, -12, -5, -2, 0, 0, -2, -5, -8,
		-7, -4, 0, -4 /*Dm*/, -7, -4, -7, -11, -12, -9, -5, -2, 0, -5, -9, -5,
		-4, 0, -9, -4 /*F7*/, -5, -1, -10, -11 /*E7*/, -12, -9, -5, -2, 0, -2, -4, -5,
	}
	var bass []Hit
	for i, p := range walk {
		bass = append(bass, Hit{Step: i * 4, Pitch: p, Dur: 1.0, Vol: 0.72})
	}
	// Spang-a-lang ride: skip notes pulled back to the triplet position.
	rideBar := []Hit{
		{Step: 0, Vol: 0.79}, {Step: 4, Vol: 0.79},
		{Step: 7, Vol: 0.63, Groove: "rush", GroovePct: 0.33},
		{Step: 8, Vol: 0.79}, {Step: 12, Vol: 0.79},
		{Step: 15, Vol: 0.63, Groove: "rush", GroovePct: 0.33},
	}
	return Showcase{
		Stem: "sax-blues", BPM: 96, Subdiv: 16, Bars: 12,
		Insts: []InstSpec{
			{ID: "sax", Name: "Sax", Volume: 0.75, ReverbSend: 0.2},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.8},
			{ID: "ride", Name: "Ride", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
			{ID: "hihat-pedal", Name: "Foot Hat", Volume: 0.5},
			{ID: "kick-acoustic", Name: "Feather", Volume: 0.35},
			{ID: "sidestick", Name: "Stick", Volume: 0.45},
		},
		Rows: []RowSpec{
			// AAB head: call (bars 1-2), call over iv (5-6), answer (9-10),
			// tag (11-12).
			{Inst: "sax", Hits: concat(riffA, at(64, riffA), at(128, answerB), at(160, riffA))},
			{Inst: "bass-guitar", Hits: bass},
			{Inst: "ride", Hits: ostinato(12, rideBar)},
			{Inst: "hihat-pedal", Hits: ostinato(12, []Hit{{Step: 4, Vol: 0.67}, {Step: 12, Vol: 0.67}})},
			{Inst: "kick-acoustic", Hits: ostinato(12, []Hit{{Step: 0, Vol: 0.22}, {Step: 4, Vol: 0.2}, {Step: 8, Vol: 0.22}, {Step: 12, Vol: 0.2}})},
			{Inst: "sidestick", Hits: ostinato(12, []Hit{{Step: 12, Vol: 0.55}})},
		},
	}
}

// ── steel-folk: Travis picking over G-C-D-Em + brushed train beat ────────────
func steelFolkShowcase() Showcase {
	travisBar := func(base int, root, fifth, pinch1, pinch2, inner, mel1, mel2 float64) []Hit {
		return concat(
			chordAt(base, 0.8, 0.82, root, pinch1),
			[]Hit{
				{Step: base + 2, Pitch: inner, Dur: 0.5, Vol: 0.59},
				{Step: base + 4, Pitch: fifth, Dur: 0.8, Vol: 0.75},
				{Step: base + 6, Pitch: mel1, Dur: 0.5, Vol: 0.67},
			},
			chordAt(base+8, 0.8, 0.78, root, pinch2),
			[]Hit{
				{Step: base + 10, Pitch: mel2, Dur: 0.5, Vol: 0.67},
				{Step: base + 12, Pitch: fifth, Dur: 0.8, Vol: 0.75},
				{Step: base + 14, Pitch: inner, Dur: 0.5, Vol: 0.59},
			},
		)
	}
	cycle := concat(
		travisBar(0, -14, -7, 10, 2, 5, 2, 5),   // G
		travisBar(16, -9, -14, 7, 3, 10, 3, 7),  // C
		travisBar(32, -7, -12, 9, 5, 12, 5, 9),  // D
		travisBar(48, -17, -10, 10, 7, 2, 7, 2), // Em
	)
	return Showcase{
		Stem: "steel-folk", BPM: 92, Subdiv: 16, Bars: 8,
		Insts: []InstSpec{
			{ID: "guitar-steel", Name: "Travis", Volume: 0.75, ReverbSend: 0.16},
			{ID: "snare-ghost", Name: "Brush Bed", Volume: 0.4},
			{ID: "snare", Name: "Brush Accent", Volume: 0.55},
			{ID: "kick-acoustic", Name: "Kick", Volume: 0.6},
			{ID: "hihat-pedal", Name: "Foot Hat", Volume: 0.42},
		},
		Rows: []RowSpec{
			{Inst: "guitar-steel", Hits: concat(cycle, at(64, cycle))},
			// Train beat: 16th brush bed with "&" accents on the snare row.
			{Inst: "snare-ghost", Hits: ostinato(8, []Hit{
				{Step: 0, Vol: 0.32}, {Step: 1, Vol: 0.3}, {Step: 3, Vol: 0.3},
				{Step: 4, Vol: 0.32}, {Step: 5, Vol: 0.3}, {Step: 7, Vol: 0.3},
				{Step: 8, Vol: 0.32}, {Step: 9, Vol: 0.3}, {Step: 11, Vol: 0.3},
				{Step: 12, Vol: 0.32}, {Step: 13, Vol: 0.3}, {Step: 15, Vol: 0.3},
			})},
			{Inst: "snare", Hits: ostinato(8, []Hit{{Step: 2, Vol: 0.75}, {Step: 6, Vol: 0.75}, {Step: 10, Vol: 0.75}, {Step: 14, Vol: 0.75}})},
			{Inst: "kick-acoustic", Hits: ostinato(8, []Hit{{Step: 0, Vol: 0.67}, {Step: 4, Vol: 0.62}, {Step: 8, Vol: 0.67}, {Step: 12, Vol: 0.62}})},
			{Inst: "hihat-pedal", Hits: ostinato(8, []Hit{{Step: 2, Vol: 0.47}, {Step: 6, Vol: 0.47}, {Step: 10, Vol: 0.47}, {Step: 14, Vol: 0.47}})},
		},
	}
}

// ── electric-riff: E5/G5/A5 power riff + rock kit with a real fill ───────────
func electricRiffShowcase() Showcase {
	// Power chords as root+fifth rolls; dense 16th pairs use single strokes.
	riff2 := concat(
		chordAt(0, 1.5, 0.91, -17, -10),
		[]Hit{{Step: 6, Pitch: -14, Dur: 0.25, Vol: 0.79}, {Step: 7, Pitch: -7, Dur: 0.25, Vol: 0.79}},
		chordAt(8, 1.0, 0.91, -12, -5),
		chordAt(12, 0.5, 0.75, -14, -7),
		[]Hit{{Step: 14, Pitch: -17, Dur: 0.5, Vol: 0.75}},
		chordAt(16, 0.5, 0.87, -17, -10),
		[]Hit{
			{Step: 18, Pitch: -17, Dur: 0.3, Vol: 0.71}, {Step: 20, Pitch: -17, Dur: 0.3, Vol: 0.71},
			{Step: 22, Pitch: -12, Dur: 0.25, Vol: 0.79}, {Step: 23, Pitch: -5, Dur: 0.25, Vol: 0.79},
		},
		chordAt(24, 1.0, 0.87, -14, -7),
		chordAt(28, 2.0, 0.91, -17, -10),
	)
	// Final pass drops the ring-out so the drum fill owns beats 3-4 of bar 8.
	var riffTail []Hit
	for _, h := range riff2 {
		if h.Step < 28 {
			riffTail = append(riffTail, h)
		}
	}
	return Showcase{
		Stem: "electric-riff", BPM: 120, Subdiv: 16, Bars: 8,
		Insts: []InstSpec{
			{ID: "guitar-electric", Name: "Riff", Volume: 0.78, ReverbSend: 0.1},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.8},
			{ID: "kick", Name: "Kick", Volume: 0.95},
			{ID: "snare", Name: "Snare", Volume: 0.9},
			{ID: "hihat", Name: "Hihat", Volume: 0.6},
			{ID: "tom", Name: "Tom Hi", Volume: 0.7},
			{ID: "tom-1", Name: "Tom Lo", Volume: 0.72},
			{ID: "crash", Name: "Crash", Volume: 0.55, Effects: []EffectSpec{{Mode: 0, Cutoff: 12000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			// Riff ×4 minus the ring-out beats where the bar-8 fill takes over.
			{Inst: "guitar-electric", Hits: concat(riff2, at(32, riff2), at(64, riff2), at(96, riffTail)),
			},
			// Root support: E on 1 & 3 (renders E1 through the octave-down bass).
			{Inst: "bass-guitar", Hits: ostinato(8, []Hit{{Step: 0, Pitch: -17, Dur: 0.8, Vol: 0.8}, {Step: 8, Pitch: -17, Dur: 0.8, Vol: 0.75}})},
			{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 0.87}, {Step: 8, Vol: 0.87}, {Step: 9, Vol: 0.6}})},
			// Backbeat + the sourced bar-8 fill (snare 16ths on beat 3).
			{Inst: "snare", Hits: concat(
				ostinato(8, []Hit{{Step: 4, Vol: 0.9}, {Step: 12, Vol: 0.9}}),
				[]Hit{{Step: 120, Vol: 0.79}, {Step: 121, Vol: 0.83}, {Step: 122, Vol: 0.87}, {Step: 123, Vol: 0.87}},
			)},
			{Inst: "hihat", Hits: ostinato(8, []Hit{
				{Step: 0, Vol: 0.75}, {Step: 2, Vol: 0.67}, {Step: 4, Vol: 0.75}, {Step: 6, Vol: 0.67},
				{Step: 8, Vol: 0.75}, {Step: 10, Vol: 0.67}, {Step: 12, Vol: 0.75}, {Step: 14, Vol: 0.67},
			})},
			{Inst: "tom", Hits: []Hit{{Step: 124, Pitch: 0, Dur: 0.25, Vol: 0.87}, {Step: 125, Pitch: 0, Dur: 0.25, Vol: 0.87}}},
			{Inst: "tom-1", Hits: []Hit{{Step: 126, Pitch: 0, Dur: 0.25, Vol: 0.91}, {Step: 127, Pitch: 0, Dur: 0.3, Vol: 0.93}}},
			{Inst: "crash", Hits: []Hit{{Step: 0, Vol: 0.94}, {Step: 64, Vol: 0.94}}},
		},
	}
}

// ── clav-funk: MusicRadar clav grammar + drumlessons ghost kit ───────────────
func clavFunkShowcase() Showcase {
	clavBar := concat(
		chordAt(0, 0.2, 0.94, 3, 10), // THE ONE
		[]Hit{
			{Step: 2, Pitch: 10, Dur: 0.1, Vol: 0.35}, {Step: 3, Pitch: 10, Dur: 0.1, Vol: 0.35},
			{Step: 4, Pitch: -12, Dur: 0.15, Vol: 0.75}, // LH octave, clipped
		},
		chordAt(6, 0.15, 0.87, 3, 7), // & of 2 accent
		[]Hit{
			{Step: 8, Pitch: 10, Dur: 0.1, Vol: 0.35}, {Step: 9, Pitch: 10, Dur: 0.1, Vol: 0.35},
			{Step: 10, Pitch: 10, Dur: 0.1, Vol: 0.35}, {Step: 11, Pitch: 10, Dur: 0.1, Vol: 0.35},
		},
		chordAt(12, 0.15, 0.79, 3, 7),
		[]Hit{{Step: 14, Pitch: 10, Dur: 0.1, Vol: 0.35}, {Step: 15, Pitch: 10, Dur: 0.1, Vol: 0.43}},
	)
	bassBar := []Hit{
		{Step: 0, Pitch: -12, Dur: 0.4, Vol: 0.91}, {Step: 3, Pitch: -12, Dur: 0.1, Vol: 0.31},
		{Step: 6, Pitch: 0, Dur: 0.25, Vol: 0.79}, {Step: 8, Pitch: -2, Dur: 0.25, Vol: 0.71},
		{Step: 10, Pitch: -12, Dur: 0.4, Vol: 0.87}, {Step: 13, Pitch: -12, Dur: 0.1, Vol: 0.31},
		{Step: 14, Pitch: -5, Dur: 0.4, Vol: 0.71},
	}
	sw := func(h []Hit) []Hit { return swing16(h, 0.12) } // light funk 16th swing
	return Showcase{
		Stem: "clav-funk", BPM: 104, Subdiv: 16, Bars: 8,
		Insts: []InstSpec{
			{ID: "fm-pluck", Name: "Clav", Volume: 0.7, Pan: 0.15},
			{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
			{ID: "kick", Name: "Kick", Volume: 0.95},
			{ID: "snare", Name: "Snare", Volume: 0.9},
			{ID: "snare-ghost", Name: "Ghosts", Volume: 0.45},
			{ID: "hihat", Name: "Hihat", Volume: 0.6},
			{ID: "hihat-1", Name: "Open Hat", Volume: 0.68, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
		},
		Rows: []RowSpec{
			{Inst: "fm-pluck", Hits: sw(ostinato(8, clavBar))},
			// Bass +12: renders at the written A1/A2 slap register.
			{Inst: "bass-guitar", Hits: sw(ostinato(8, transposeHits(bassBar, 12)))},
			{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 0.91}, {Step: 10, Vol: 0.91}})},
			{Inst: "snare", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.91}, {Step: 12, Vol: 0.91}})},
			{Inst: "snare-ghost", Hits: sw(ostinato(8, []Hit{
				{Step: 5, Vol: 0.3}, {Step: 6, Vol: 0.3}, {Step: 9, Vol: 0.3}, {Step: 13, Vol: 0.3}, {Step: 14, Vol: 0.3},
			}))},
			{Inst: "hihat", Hits: sw(ostinato(8, []Hit{
				{Step: 0, Vol: 0.67}, {Step: 2, Vol: 0.6}, {Step: 4, Vol: 0.67}, {Step: 6, Vol: 0.6},
				{Step: 8, Vol: 0.67}, {Step: 10, Vol: 0.6}, {Step: 12, Vol: 0.67},
			}))},
			{Inst: "hihat-1", Hits: ostinato(8, []Hit{{Step: 14, Vol: 0.75}})},
		},
	}
}

// ── bass-groove: slap/pop etude (octave pops, hammer-ons, chromatic walks) ───
func bassGrooveShowcase() Showcase {
	// Written pitches from the dossier, +12 so the slap register renders
	// as written (A1 root, A2 pops).
	line := transposeHits([]Hit{
		{Step: 0, Pitch: -24, Dur: 0.4, Vol: 0.93}, {Step: 2, Pitch: -12, Dur: 0.25, Vol: 0.83},
		{Step: 5, Pitch: -24, Dur: 0.1, Vol: 0.31}, {Step: 6, Pitch: -24, Dur: 0.4, Vol: 0.75},
		{Step: 8, Pitch: -21, Dur: 0.22, Vol: 0.71}, {Step: 9, Pitch: -19, Dur: 0.25, Vol: 0.67},
		{Step: 10, Pitch: -17, Dur: 0.5, Vol: 0.79}, {Step: 12, Pitch: -12, Dur: 0.25, Vol: 0.79},
		{Step: 14, Pitch: -26, Dur: 0.25, Vol: 0.67}, {Step: 15, Pitch: -25, Dur: 0.25, Vol: 0.71},
		{Step: 16, Pitch: -24, Dur: 0.4, Vol: 0.93}, {Step: 18, Pitch: -12, Dur: 0.25, Vol: 0.83},
		{Step: 21, Pitch: -24, Dur: 0.1, Vol: 0.31}, {Step: 22, Pitch: -14, Dur: 0.25, Vol: 0.75},
		{Step: 24, Pitch: -17, Dur: 0.4, Vol: 0.75}, {Step: 26, Pitch: -19, Dur: 0.25, Vol: 0.71},
		{Step: 27, Pitch: -18, Dur: 0.25, Vol: 0.67}, {Step: 28, Pitch: -17, Dur: 0.5, Vol: 0.83},
		{Step: 31, Pitch: -24, Dur: 0.1, Vol: 0.31},
	}, 12)
	return Showcase{
		Stem: "bass-groove", BPM: 110, Subdiv: 16, Bars: 8,
		Insts: []InstSpec{
			{ID: "bass-guitar", Name: "Slap Bass", Volume: 0.9},
			{ID: "guitar-electric-neck", Name: "Skank", Volume: 0.5, Pan: 0.25},
			{ID: "kick-tight", Name: "Kick", Volume: 0.85},
			{ID: "sidestick", Name: "Stick", Volume: 0.65},
			{ID: "hihat", Name: "Hihat", Volume: 0.5},
		},
		Rows: []RowSpec{
			{Inst: "bass-guitar", Hits: ostinato2(8, line)},
			{Inst: "guitar-electric-neck", Hits: ostinato(8, concat(
				chordAt(4, 0.15, 0.75, 0, 3, 7),
				chordAt(12, 0.15, 0.75, 0, 3, 7),
			))},
			{Inst: "kick-tight", Hits: ostinato(8, []Hit{{Step: 0, Vol: 0.87}, {Step: 10, Vol: 0.87}})},
			{Inst: "sidestick", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.75}, {Step: 12, Vol: 0.75}})},
			{Inst: "hihat", Hits: hatEighths(8, 0.55)},
		},
	}
}

// ── conga-tumbao: the full son-montuno percussion family (scphillips grids) ──
func congaTumbaoShowcase() Showcase {
	marcha := []Hit{ // high drum: heel/tip ghost bed + THE slap on 2
		{Step: 0, Dur: 0.2, Vol: 0.35}, {Step: 2, Dur: 0.2, Vol: 0.31},
		{Step: 4, Dur: 0.25, Vol: 0.91}, {Step: 6, Dur: 0.2, Vol: 0.31},
		{Step: 8, Dur: 0.2, Vol: 0.59}, {Step: 10, Dur: 0.2, Vol: 0.31},
		{Step: 16, Dur: 0.2, Vol: 0.35}, {Step: 18, Dur: 0.2, Vol: 0.31},
		{Step: 20, Dur: 0.25, Vol: 0.91}, {Step: 24, Dur: 0.2, Vol: 0.35}, {Step: 26, Dur: 0.2, Vol: 0.31},
	}
	return Showcase{
		Stem: "conga-tumbao", BPM: 100, Subdiv: 16, Bars: 8,
		Insts: []InstSpec{
			{ID: "conga", Name: "Marcha", Volume: 0.72, SynthParams: map[string]float64{"decay": 0.5}},
			{ID: "conga-open", Name: "Open", Volume: 0.85},
			{ID: "conga-tumba", Name: "Tumba", Volume: 0.85, Pan: -0.2},
			{ID: "sidestick", Name: "Clave", Volume: 0.85},
			{ID: "rimshot", Name: "Cáscara", Volume: 0.55, Pan: 0.3},
			{ID: "cowbell", Name: "Bell Mouth", Volume: 0.5, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "cowbell-1", Name: "Bell Body", Volume: 0.38, SynthParams: map[string]float64{"cym_tune": 0.8}, Effects: []EffectSpec{{Mode: 0, Cutoff: 7500, Q: 0.707}}},
			{ID: "shaker", Name: "Güiro", Volume: 0.5, Effects: []EffectSpec{{Mode: 0, Cutoff: 9000, Q: 0.707}}},
			{ID: "hihat", Name: "Maracas", Volume: 0.4},
			{ID: "bass-guitar", Name: "Tumbao", Volume: 0.7},
		},
		Rows: []RowSpec{
			{Inst: "conga", Hits: ostinato2(8, marcha)},
			// High-drum open tones close the 2-side bar (steps 12, 14).
			{Inst: "conga-open", Hits: ostinato2(8, []Hit{{Step: 12, Dur: 0.35, Vol: 0.83}, {Step: 14, Dur: 0.35, Vol: 0.83}})},
			// Tumba sounds the bombo + the 3-side "boom-boom".
			{Inst: "conga-tumba", Hits: ostinato2(8, []Hit{{Step: 22, Dur: 0.35, Vol: 0.79}, {Step: 28, Dur: 0.35, Vol: 0.87}, {Step: 30, Dur: 0.35, Vol: 0.87}})},
			// Son clave 2-3.
			{Inst: "sidestick", Hits: ostinato2(8, []Hit{
				{Step: 4, Vol: 0.87}, {Step: 8, Vol: 0.87},
				{Step: 16, Vol: 0.87}, {Step: 22, Vol: 0.87}, {Step: 28, Vol: 0.87},
			})},
			// Cáscara with clave-aligned accents.
			{Inst: "rimshot", Hits: ostinato2(8, []Hit{
				{Step: 0, Vol: 0.55}, {Step: 4, Vol: 0.87}, {Step: 8, Vol: 0.87}, {Step: 10, Vol: 0.55}, {Step: 14, Vol: 0.55},
				{Step: 16, Vol: 0.87}, {Step: 20, Vol: 0.55}, {Step: 22, Vol: 0.87}, {Step: 26, Vol: 0.63}, {Step: 30, Vol: 0.63},
			})},
			{Inst: "cowbell", Hits: ostinato2(8, []Hit{{Step: 0, Vol: 0.91}, {Step: 8, Vol: 0.91}, {Step: 16, Vol: 0.91}, {Step: 24, Vol: 0.91}})},
			{Inst: "cowbell-1", Hits: ostinato2(8, []Hit{
				{Step: 4, Vol: 0.51}, {Step: 12, Vol: 0.59}, {Step: 14, Vol: 0.59},
				{Step: 20, Vol: 0.59}, {Step: 22, Vol: 0.59}, {Step: 28, Vol: 0.59}, {Step: 30, Vol: 0.59},
			})},
			{Inst: "shaker", Hits: ostinato(8, []Hit{
				{Step: 0, Vol: 0.79, Dur: 0.5}, {Step: 4, Vol: 0.55, Dur: 0.15}, {Step: 6, Vol: 0.55, Dur: 0.15},
				{Step: 8, Vol: 0.79, Dur: 0.5}, {Step: 12, Vol: 0.55, Dur: 0.15}, {Step: 14, Vol: 0.55, Dur: 0.15},
			})},
			{Inst: "hihat", Hits: ostinato(8, []Hit{
				{Step: 0, Vol: 0.45}, {Step: 4, Vol: 0.5}, {Step: 6, Vol: 0.37},
				{Step: 8, Vol: 0.45}, {Step: 12, Vol: 0.5}, {Step: 14, Vol: 0.37},
			})},
			// Tresillo bass vamp (labeled idiomatic; +12 octave policy).
			{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
				{Step: 6, Pitch: -12, Dur: 1.5, Vol: 0.72}, {Step: 12, Pitch: -5, Dur: 1.0, Vol: 0.66},
				{Step: 22, Pitch: -12, Dur: 1.5, Vol: 0.72}, {Step: 28, Pitch: -5, Dur: 1.0, Vol: 0.66},
			})},
		},
	}
}
