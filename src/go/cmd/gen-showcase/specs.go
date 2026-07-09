package main

// Pitch offsets from A3 (220Hz). Songs transposed to fit the A-based modular
// tuning; intervals/contour preserved for recognizability.
//
// Colors are assigned automatically by build() from templates.InstrumentSequence
// (row N → sequence[N % len]), so no per-spec color literals are needed here.
//
// Step convention (Subdiv=16, 1 bar = 16 steps):
//
//	Beat 1=0, Beat 2=4, Beat 3=8, Beat 4=12.  "Ands" (offbeats) = 2,6,10,14.
//	Bar B (0-indexed) starts at step B*16; an 8-bar loop is steps 0..127.
//
// Pitch reference (semitones from A3=0):
//
//	C2=-21 C#2=-20 D2=-19 Eb2=-18 E2=-17 F2=-16 F#2=-15 G2=-14 G#2=-13 A2=-12 Bb2=-11 B2=-10
//	C3=-9  C#3=-8  D3=-7  Eb3=-6  E3=-5  F3=-4  F#3=-3  G3=-2  G#3=-1  A3=0   Bb3=1   B3=2
//	C4=3   C#4=4   D4=5   Eb4=6   E4=7   F4=8   F#4=9   G4=10  G#4=11  A4=12  Bb4=13  B4=14
//	C5=15  C#5=16  D5=17  Eb5=18  E5=19  F5=20  F#5=21  G5=22  G#5=23  A5=24  Bb5=25  B5=26
//
// Each template recreates the instrumental signature of one genre-defining
// masterpiece. First-pass renditions — ear-tune collaboratively after listening.
// See docs/superpowers/specs/2026-06-20-genre-masterpiece-templates-design.md.
func showcases() []Showcase {
	return []Showcase{

		// ── Bach — Toccata & Fugue BWV 565 (spec_classical.go, real score) ──
		toccataShowcase(),

		// ── Vivaldi — Spring RV 269 (spec_classical.go, real score) ──
		vivaldiShowcase(),

		// ── Mozart — Sonata K 545 (spec_classical.go, real score) ──
		k545Showcase(),

		// ── 4. Eagles — Hotel California (spec_eagles_hotel_california.go) ──────
		hotelCaliforniaShowcase(),

		// ── 5. Toto — Africa (spec_toto_africa.go) ──────────────────────────────
		africaShowcase(),

		// ── 6. Miles Davis — So What (spec_miles_so_what.go) ────────────────────
		soWhatShowcase(),

		// ── 7. Tito Puente / Santana — Oye Como Va (spec_puente_oye_como_va.go) ─
		oyeComoVaShowcase(),

		// ── 8. Stevie Wonder — Superstition (spec_wonder_superstition.go) ──────
		superstitionShowcase(),

		// ── 9. Black Box — Ride On Time (spec_blackbox_ride_on_time.go) ─────────
		rideOnTimeShowcase(),

		// ── 10. Dr. Dre — Nuthin' but a 'G' Thang (spec_dre_g_thang.go) ─────────
		gThangShowcase(),

		// ── 11. Gloria Gaynor — I Will Survive (spec_gaynor_survive.go) ─────────
		gaynorShowcase(),

		// ── 12. Bob Marley — Exodus (spec_marley_exodus.go) ─────────────────────
		exodusShowcase(),

		// ── 13. Chic — Good Times (spec_chic_good_times.go) ─────────────────────
		goodTimesShowcase(),

		// ── 14. Jobim — The Girl from Ipanema (spec_jobim_ipanema.go) ───────────
		ipanemaShowcase(),

		// ── 15. B.B. King — The Thrill Is Gone (spec_bbking_thrill.go) ──────────
		thrillShowcase(),

		// ── Bach — Prelude in C BWV 846 (spec_classical.go, real score) ──
		preludeCShowcase(),

		// ── Bach — Flute Allemande BWV 1013 (spec_classical.go, real score) ──
		fluteAllemandeShowcase(),

		// ── Bach — Cello Prelude BWV 1007 (spec_classical.go, real score) ──
		celloPreludeShowcase(),

		// ── Cielito Lindo — mariachi vals (spec_cielito_trumpet.go) ────────────
		cielitoShowcase(),

		// ── Handel — Alla Hornpipe HWV 349 (spec_classical.go, real score) ──
		handelShowcase(),

		// ── Pachelbel — Canon in D (spec_classical.go, real score) ──
		pachelbelShowcase(),

		// ── Marcello — Oboe Adagio (spec_classical.go, real score) ──
		marcelloShowcase(),

		// ── Bach BWV 846 on felt piano (spec_etudes.go, real score) ──
		feltPreludeShowcase(),

		// ── Sax blues — A-minor 12-bar (spec_etudes.go) ──
		saxBluesShowcase(),

		// ── Steel folk — Travis picking (spec_etudes.go) ──
		steelFolkShowcase(),

		// ── Electric riff — rock power riff (spec_etudes.go) ──
		electricRiffShowcase(),

		// ── Clav funk (spec_etudes.go) ──
		clavFunkShowcase(),

		// ── Bass groove — slap etude (spec_etudes.go) ──
		bassGrooveShowcase(),

		// ── Conga tumbao — son montuno family (spec_etudes.go) ──
		congaTumbaoShowcase(),

		// ── Marc Anthony — Vivir Mi Vida (spec_salsa_vivir.go) ─────────────────
		vivirShowcase(),

		// ── Aventura — Obsesión (spec_bachata_obsesion.go) ─────────────────────
		obsesionShowcase(),

		// ── Albéniz — Asturias (MusicXML-sourced, score_import.go) ──────────────
		asturiasShowcase(),
	}
}

// ostinato repeats a 1-bar (16-step) hit pattern across `bars` bars.
func ostinato(bars int, pat []Hit) []Hit {
	return repeatPattern(bars, 16, pat)
}

// ostinato2 repeats a 2-bar (32-step) hit pattern across `bars` bars.
func ostinato2(bars int, pat []Hit) []Hit {
	return repeatPattern(bars, 32, pat)
}

// repeatPattern tiles `pat` (whose steps lie in [0,period)) every `period`
// steps for `bars` bars (16 steps/bar), offsetting each copy's Step.
func repeatPattern(bars, period int, pat []Hit) []Hit {
	total := bars * 16
	out := make([]Hit, 0, len(pat)*(total/period+1))
	for base := 0; base < total; base += period {
		for _, h := range pat {
			if base+h.Step >= total {
				continue
			}
			c := h
			c.Step = base + h.Step
			out = append(out, c)
		}
	}
	return out
}

// hatEighths returns 8th-note hi-hat hits across `bars` bars at volume `v`.
func hatEighths(bars int, v float64) []Hit {
	var out []Hit
	for s := 0; s < bars*16; s += 2 {
		out = append(out, Hit{Step: s, Vol: v})
	}
	return out
}

// hatSixteenths returns 16th-note hi-hat/shaker hits across `bars` bars at volume `v`.
func hatSixteenths(bars int, v float64) []Hit {
	var out []Hit
	for s := 0; s < bars*16; s++ {
		out = append(out, Hit{Step: s, Vol: v})
	}
	return out
}

// chordAt voices a chord under the one-node-per-step model: each tone is placed
// on a consecutive 16th step starting at `step`, all sharing `dur` so they ring
// together — a tight strum for short dur, a sustained pad for long dur. Keep the
// tone count ≤ the gap to the next event in the row (build() panics on a
// duplicate step). pitches are semitones from A3.
func chordAt(step int, dur, vol float64, pitches ...float64) []Hit {
	out := make([]Hit, len(pitches))
	for i, p := range pitches {
		out[i] = Hit{Step: step + i, Pitch: p, Dur: dur, Vol: vol}
	}
	return out
}

// concat flattens hit groups into one slice (lets a row compose chordAt() spreads
// alongside single-note hits).
func concat(groups ...[]Hit) []Hit {
	var out []Hit
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// swing8 applies a delay-groove to the offbeat 8th positions ("ands", steps
// ≡ 2 mod 4) of a hit list — the classic swung-8ths feel. pct is the delay
// percentage (0..1) of the groove window. Hits on other steps pass unchanged;
// hits that already carry a groove are left alone.
func swing8(hits []Hit, pct float64) []Hit {
	return grooveOn(hits, pct, func(s int) bool { return s%4 == 2 })
}

// swing16 applies a delay-groove to every odd 16th step — swung 16ths (funk
// hat feel). See swing8.
func swing16(hits []Hit, pct float64) []Hit {
	return grooveOn(hits, pct, func(s int) bool { return s%2 == 1 })
}

// grooveOn returns a copy of hits with Groove:"delay"/pct applied to every hit
// whose step satisfies pred (and that doesn't already carry a groove).
func grooveOn(hits []Hit, pct float64, pred func(int) bool) []Hit {
	out := make([]Hit, len(hits))
	for i, h := range hits {
		if h.Groove == "" && pred(h.Step) {
			h.Groove = "delay"
			h.GroovePct = pct
		}
		out[i] = h
	}
	return out
}

// laidBack marks every hit in the list as delayed by pct — a whole part
// sitting behind the beat (G-funk leads, reggae skanks).
func laidBack(hits []Hit, pct float64) []Hit {
	return grooveOn(hits, pct, func(int) bool { return true })
}

// pushed marks every hit as rushed by pct — a part playing on top of / ahead
// of the beat (latin montuno anticipations).
func pushed(hits []Hit, pct float64) []Hit {
	out := make([]Hit, len(hits))
	for i, h := range hits {
		if h.Groove == "" {
			h.Groove = "rush"
			h.GroovePct = pct
		}
		out[i] = h
	}
	return out
}

// at shifts every hit's Step by base — places a pattern at an absolute
// bar/step offset (e.g. a section that starts at bar 8 = at(128, ...)).
func at(base int, hits []Hit) []Hit {
	out := make([]Hit, len(hits))
	for i, h := range hits {
		h.Step += base
		out[i] = h
	}
	return out
}

// transposeHits returns a copy of hits with every pitch shifted by semis.
func transposeHits(hits []Hit, semis float64) []Hit {
	out := make([]Hit, len(hits))
	for i, h := range hits {
		h.Pitch += semis
		out[i] = h
	}
	return out
}
