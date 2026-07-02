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

		// ── 1. Bach — Toccata & Fugue in D minor, BWV 565 (organ) ──────────────
		// Dm. The iconic opening: mordent flourish on A, descending turn, repeated
		// an octave lower; a descending D-minor run into a diminished arpeggio and
		// the big chord; rolling toccata figuration; rising run turnaround.
		{
			Stem: "bach-toccata", BPM: 70, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "organ", Name: "Organ", Volume: 0.8, ReverbSend: 0.4},
				{ID: "organ", Name: "Pedal", Volume: 0.78, ReverbSend: 0.32},
				{ID: "violin-ensemble", Name: "Strings", Volume: 0.5, ReverbSend: 0.4},
			},
			Rows: []RowSpec{
				// Organ manual (lead).
				{Inst: "organ", Hits: []Hit{
					// Bar 1: mordent A-G-A, held A, then descending turn G-F-E-D-C#-D.
					{Step: 0, Pitch: 12, Dur: 0.5, Vol: 1.0}, {Step: 1, Pitch: 10, Dur: 0.25}, {Step: 2, Pitch: 12, Dur: 1.6, Vol: 1.0},
					{Step: 8, Pitch: 10, Dur: 0.4}, {Step: 9, Pitch: 8, Dur: 0.4}, {Step: 10, Pitch: 7, Dur: 0.4},
					{Step: 11, Pitch: 5, Dur: 0.4}, {Step: 12, Pitch: 4, Dur: 0.4}, {Step: 13, Pitch: 5, Dur: 1.2, Vol: 0.95},
					// Bar 2: same gesture an octave lower.
					{Step: 16, Pitch: 0, Dur: 0.5, Vol: 1.0}, {Step: 17, Pitch: -2, Dur: 0.25}, {Step: 18, Pitch: 0, Dur: 1.6},
					{Step: 24, Pitch: -2, Dur: 0.4}, {Step: 25, Pitch: -4, Dur: 0.4}, {Step: 26, Pitch: -5, Dur: 0.4},
					{Step: 27, Pitch: -7, Dur: 0.4}, {Step: 28, Pitch: -8, Dur: 0.4}, {Step: 29, Pitch: -7, Dur: 1.2, Vol: 0.95},
					// Bar 3: descending D-minor run (16ths).
					{Step: 32, Pitch: 12}, {Step: 33, Pitch: 10}, {Step: 34, Pitch: 8}, {Step: 35, Pitch: 7},
					{Step: 36, Pitch: 5}, {Step: 37, Pitch: 3}, {Step: 38, Pitch: 1}, {Step: 39, Pitch: 0},
					{Step: 40, Pitch: -2}, {Step: 41, Pitch: -4}, {Step: 42, Pitch: -5}, {Step: 43, Pitch: -7},
					{Step: 44, Pitch: -9}, {Step: 45, Pitch: -11}, {Step: 46, Pitch: -12, Dur: 0.6},
					// Bar 4: C# dim7 arpeggio up → big D-minor chord arpeggio, held.
					{Step: 48, Pitch: 4, Dur: 0.4}, {Step: 49, Pitch: 7, Dur: 0.4}, {Step: 50, Pitch: 10, Dur: 0.4}, {Step: 51, Pitch: 13, Dur: 0.4},
					{Step: 52, Pitch: 12, Dur: 0.6, Vol: 1.0},
					{Step: 56, Pitch: 5, Dur: 0.4}, {Step: 57, Pitch: 8, Dur: 0.4}, {Step: 58, Pitch: 12, Dur: 0.4}, {Step: 59, Pitch: 17, Dur: 0.5},
					{Step: 60, Pitch: 12, Dur: 2.0, Vol: 1.0},
					// Bars 5-6: rolling toccata figuration around A-D-F.
					{Step: 64, Pitch: 0}, {Step: 65, Pitch: 5}, {Step: 66, Pitch: 8}, {Step: 67, Pitch: 12},
					{Step: 68, Pitch: 8}, {Step: 69, Pitch: 5}, {Step: 70, Pitch: 0}, {Step: 71, Pitch: 5},
					{Step: 72, Pitch: 0}, {Step: 73, Pitch: 5}, {Step: 74, Pitch: 8}, {Step: 75, Pitch: 12},
					{Step: 76, Pitch: 8}, {Step: 77, Pitch: 5}, {Step: 78, Pitch: 0}, {Step: 79, Pitch: 5},
					{Step: 80, Pitch: -2}, {Step: 81, Pitch: 1}, {Step: 82, Pitch: 5}, {Step: 83, Pitch: 10},
					{Step: 84, Pitch: 5}, {Step: 85, Pitch: 1}, {Step: 86, Pitch: -2}, {Step: 87, Pitch: 1},
					{Step: 88, Pitch: -2}, {Step: 89, Pitch: 1}, {Step: 90, Pitch: 5}, {Step: 91, Pitch: 10},
					{Step: 92, Pitch: 5}, {Step: 93, Pitch: 1}, {Step: 94, Pitch: -2}, {Step: 95, Pitch: 1},
					// Bars 7-8: rising run back up, mordent turnaround on high A.
					{Step: 96, Pitch: -12}, {Step: 97, Pitch: -9}, {Step: 98, Pitch: -7}, {Step: 99, Pitch: -5},
					{Step: 100, Pitch: -4}, {Step: 101, Pitch: -2}, {Step: 102, Pitch: 0}, {Step: 103, Pitch: 3},
					{Step: 104, Pitch: 5}, {Step: 105, Pitch: 7}, {Step: 106, Pitch: 8}, {Step: 107, Pitch: 10},
					{Step: 108, Pitch: 12}, {Step: 109, Pitch: 15}, {Step: 110, Pitch: 17}, {Step: 111, Pitch: 19},
					{Step: 112, Pitch: 12, Dur: 0.5, Vol: 1.0}, {Step: 113, Pitch: 10, Dur: 0.25}, {Step: 114, Pitch: 12, Dur: 2.0, Vol: 1.0},
				}},
				// Organ pedal (bass).
				{Inst: "organ", Hits: []Hit{
					{Step: 0, Pitch: -19, Dur: 4.0, Vol: 0.85}, {Step: 16, Pitch: -19, Dur: 4.0, Vol: 0.85},
					{Step: 32, Pitch: -19, Dur: 4.0, Vol: 0.85}, {Step: 48, Pitch: -20, Dur: 2.0, Vol: 0.8}, {Step: 56, Pitch: -19, Dur: 2.0, Vol: 0.9},
					{Step: 64, Pitch: -19, Dur: 4.0, Vol: 0.8}, {Step: 80, Pitch: -14, Dur: 4.0, Vol: 0.8},
					{Step: 96, Pitch: -19, Dur: 4.0, Vol: 0.8}, {Step: 112, Pitch: -12, Dur: 2.0, Vol: 0.85}, {Step: 120, Pitch: -19, Dur: 2.0, Vol: 0.9},
				}},
				// Strings — soft Dm swells doubling the big chords (bars 3-8).
				{Inst: "violin-ensemble", Hits: []Hit{
					{Step: 48, Pitch: 0, Dur: 2.0, Vol: 0.55}, {Step: 56, Pitch: 5, Dur: 2.0, Vol: 0.6},
					{Step: 64, Pitch: -4, Dur: 4.0, Vol: 0.5}, {Step: 80, Pitch: 1, Dur: 4.0, Vol: 0.5},
					{Step: 96, Pitch: 0, Dur: 4.0, Vol: 0.55}, {Step: 112, Pitch: 5, Dur: 4.0, Vol: 0.6},
				}},
			},
		},

		// ── 2. Vivaldi — "Spring" (La Primavera) RV 269, Mvt I ─────────────────
		// E major. Ritornello (forte) → echo (piano) → birdsong trills → ritornello.
		{
			Stem: "vivaldi-spring", BPM: 110, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "violin-ensemble", Name: "Strings", Volume: 0.78, ReverbSend: 0.28},
				{ID: "violin", Name: "Solo Violin", Volume: 0.8, ReverbSend: 0.24},
				{ID: "cello", Name: "Cello", Volume: 0.7, ReverbSend: 0.22},
				{ID: "organ", Name: "Continuo", Volume: 0.4, ReverbSend: 0.2},
			},
			Rows: []RowSpec{
				// Ritornello theme (E maj arpeggio up, stepwise answer down).
				{Inst: "violin-ensemble", Hits: []Hit{
					// Bar 1 (forte): E-G#-B-(E-G#-B).
					{Step: 0, Pitch: 7, Dur: 0.45, Vol: 0.9}, {Step: 2, Pitch: 11, Dur: 0.45}, {Step: 4, Pitch: 14, Dur: 0.9, Vol: 1.0},
					{Step: 8, Pitch: 7, Dur: 0.45}, {Step: 10, Pitch: 11, Dur: 0.45}, {Step: 12, Pitch: 14, Dur: 0.9, Vol: 1.0},
					// Bar 2: B-A-G#-A-G#-F#-E (descending answer).
					{Step: 16, Pitch: 14, Dur: 0.45}, {Step: 18, Pitch: 12, Dur: 0.3}, {Step: 20, Pitch: 11, Dur: 0.3},
					{Step: 22, Pitch: 12, Dur: 0.3}, {Step: 24, Pitch: 11, Dur: 0.3}, {Step: 26, Pitch: 9, Dur: 0.3}, {Step: 28, Pitch: 7, Dur: 0.9, Vol: 1.0},
					// Bars 3-4: echo (piano dynamic).
					{Step: 32, Pitch: 7, Dur: 0.45, Vol: 0.55}, {Step: 34, Pitch: 11, Dur: 0.45, Vol: 0.55}, {Step: 36, Pitch: 14, Dur: 0.9, Vol: 0.6},
					{Step: 40, Pitch: 7, Dur: 0.45, Vol: 0.55}, {Step: 42, Pitch: 11, Dur: 0.45, Vol: 0.55}, {Step: 44, Pitch: 14, Dur: 0.9, Vol: 0.6},
					{Step: 48, Pitch: 14, Dur: 0.45, Vol: 0.55}, {Step: 50, Pitch: 12, Dur: 0.3}, {Step: 52, Pitch: 11, Dur: 0.3},
					{Step: 54, Pitch: 12, Dur: 0.3}, {Step: 56, Pitch: 11, Dur: 0.3}, {Step: 58, Pitch: 9, Dur: 0.3}, {Step: 60, Pitch: 7, Dur: 0.9, Vol: 0.6},
					// Bars 7-8: ritornello returns (forte).
					{Step: 96, Pitch: 7, Dur: 0.45, Vol: 0.9}, {Step: 98, Pitch: 11, Dur: 0.45}, {Step: 100, Pitch: 14, Dur: 0.9, Vol: 1.0},
					{Step: 104, Pitch: 7, Dur: 0.45}, {Step: 106, Pitch: 11, Dur: 0.45}, {Step: 108, Pitch: 14, Dur: 0.9, Vol: 1.0},
					{Step: 112, Pitch: 14, Dur: 0.45}, {Step: 114, Pitch: 12, Dur: 0.3}, {Step: 116, Pitch: 11, Dur: 0.3},
					{Step: 118, Pitch: 12, Dur: 0.3}, {Step: 120, Pitch: 11, Dur: 0.3}, {Step: 122, Pitch: 9, Dur: 0.3}, {Step: 124, Pitch: 7, Dur: 1.4, Vol: 1.0},
				}},
				// Solo violin — birdsong trills (bars 5-6).
				{Inst: "violin", Hits: []Hit{
					{Step: 64, Pitch: 19, Dur: 0.2}, {Step: 65, Pitch: 21, Dur: 0.2}, {Step: 66, Pitch: 19, Dur: 0.2}, {Step: 67, Pitch: 21, Dur: 0.2},
					{Step: 68, Pitch: 19, Dur: 0.4, Vol: 0.85}, {Step: 70, Pitch: 14, Dur: 0.4},
					{Step: 72, Pitch: 21, Dur: 0.2}, {Step: 73, Pitch: 23, Dur: 0.2}, {Step: 74, Pitch: 21, Dur: 0.2}, {Step: 75, Pitch: 23, Dur: 0.2},
					{Step: 76, Pitch: 19, Dur: 0.4, Vol: 0.85}, {Step: 78, Pitch: 16, Dur: 0.4},
					{Step: 80, Pitch: 14, Dur: 0.2}, {Step: 81, Pitch: 16, Dur: 0.2}, {Step: 82, Pitch: 14, Dur: 0.2}, {Step: 83, Pitch: 16, Dur: 0.2},
					{Step: 84, Pitch: 19, Dur: 0.2}, {Step: 85, Pitch: 21, Dur: 0.2}, {Step: 86, Pitch: 19, Dur: 0.2}, {Step: 87, Pitch: 16, Dur: 0.2},
					{Step: 88, Pitch: 19, Dur: 0.6, Vol: 0.9}, {Step: 92, Pitch: 14, Dur: 0.6},
				}},
				// Cello — tonic/dominant roots.
				{Inst: "cello", Hits: []Hit{
					{Step: 0, Pitch: -17, Dur: 1.8, Vol: 0.8}, {Step: 8, Pitch: -17, Dur: 1.8},
					{Step: 16, Pitch: -10, Dur: 1.8}, {Step: 24, Pitch: -17, Dur: 1.8},
					{Step: 32, Pitch: -17, Dur: 1.8, Vol: 0.65}, {Step: 40, Pitch: -17, Dur: 1.8},
					{Step: 48, Pitch: -10, Dur: 1.8}, {Step: 56, Pitch: -17, Dur: 1.8},
					{Step: 64, Pitch: -17, Dur: 4.0, Vol: 0.7}, {Step: 80, Pitch: -17, Dur: 4.0},
					{Step: 96, Pitch: -17, Dur: 1.8, Vol: 0.8}, {Step: 104, Pitch: -17, Dur: 1.8},
					{Step: 112, Pitch: -10, Dur: 1.8}, {Step: 120, Pitch: -17, Dur: 1.8},
				}},
				// Continuo organ — soft pulsing E/B chords on the beat.
				{Inst: "organ", Hits: []Hit{
					{Step: 0, Pitch: 7, Dur: 0.5, Vol: 0.4}, {Step: 4, Pitch: 7, Dur: 0.5}, {Step: 8, Pitch: 7, Dur: 0.5}, {Step: 12, Pitch: 7, Dur: 0.5},
					{Step: 16, Pitch: 2, Dur: 0.5}, {Step: 20, Pitch: 2, Dur: 0.5}, {Step: 24, Pitch: 7, Dur: 0.5}, {Step: 28, Pitch: 7, Dur: 0.5},
					{Step: 96, Pitch: 7, Dur: 0.5, Vol: 0.4}, {Step: 100, Pitch: 7, Dur: 0.5}, {Step: 104, Pitch: 7, Dur: 0.5}, {Step: 108, Pitch: 7, Dur: 0.5},
					{Step: 112, Pitch: 2, Dur: 0.5}, {Step: 116, Pitch: 2, Dur: 0.5}, {Step: 120, Pitch: 7, Dur: 0.5}, {Step: 124, Pitch: 7, Dur: 0.5},
				}},
			},
		},

		// ── 3. Mozart — Piano Sonata No.16 in C, K545, Mvt I ───────────────────
		// C major. RH singing theme over a continuous LH Alberti bass (perfect
		// 16th-grid fit). 8-bar opening phrase.
		{
			Stem: "mozart-k545", BPM: 120, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "piano-grand", Name: "Piano RH", Volume: 0.72, ReverbSend: 0.12},
				{ID: "piano-felt", Name: "Piano LH", Volume: 0.6, ReverbSend: 0.1},
			},
			Rows: []RowSpec{
				// RH melody — the famous K545 theme.
				{Inst: "piano-grand", Hits: []Hit{
					// Bar 1: C5 (half) · E5-G5.
					{Step: 0, Pitch: 15, Dur: 1.8, Vol: 0.9}, {Step: 8, Pitch: 19, Dur: 0.9}, {Step: 12, Pitch: 22, Dur: 0.9},
					// Bar 2: turn B4-C5-D5-C5 · (rest) then run.
					{Step: 16, Pitch: 22, Dur: 0.45}, {Step: 18, Pitch: 20, Dur: 0.45}, {Step: 20, Pitch: 19, Dur: 0.9},
					{Step: 24, Pitch: 17, Dur: 0.45}, {Step: 26, Pitch: 15, Dur: 0.45}, {Step: 28, Pitch: 14, Dur: 0.9},
					// Bar 3: scalar run up G4-A4-B4-C5-D5-E5-F5-G5.
					{Step: 32, Pitch: 10, Dur: 0.4}, {Step: 34, Pitch: 12, Dur: 0.4}, {Step: 36, Pitch: 14, Dur: 0.4}, {Step: 38, Pitch: 15, Dur: 0.4},
					{Step: 40, Pitch: 17, Dur: 0.4}, {Step: 42, Pitch: 19, Dur: 0.4}, {Step: 44, Pitch: 20, Dur: 0.4}, {Step: 46, Pitch: 22, Dur: 0.4},
					// Bar 4: cadence C5-(B4)-C5.
					{Step: 48, Pitch: 19, Dur: 0.9, Vol: 0.95}, {Step: 52, Pitch: 14, Dur: 0.45}, {Step: 54, Pitch: 15, Dur: 1.4},
					// Bars 5-6: theme restated with embellishment.
					{Step: 64, Pitch: 15, Dur: 1.8, Vol: 0.9}, {Step: 72, Pitch: 19, Dur: 0.9}, {Step: 76, Pitch: 22, Dur: 0.9},
					{Step: 80, Pitch: 22, Dur: 0.45}, {Step: 82, Pitch: 20, Dur: 0.45}, {Step: 84, Pitch: 19, Dur: 0.9},
					{Step: 88, Pitch: 24, Dur: 0.45}, {Step: 90, Pitch: 22, Dur: 0.45}, {Step: 92, Pitch: 20, Dur: 0.9},
					// Bars 7-8: descending close to C5.
					{Step: 96, Pitch: 19, Dur: 0.4}, {Step: 98, Pitch: 17, Dur: 0.4}, {Step: 100, Pitch: 15, Dur: 0.4}, {Step: 102, Pitch: 14, Dur: 0.4},
					{Step: 104, Pitch: 12, Dur: 0.4}, {Step: 106, Pitch: 14, Dur: 0.4}, {Step: 108, Pitch: 15, Dur: 0.9},
					{Step: 112, Pitch: 19, Dur: 0.9, Vol: 0.95}, {Step: 116, Pitch: 14, Dur: 0.45}, {Step: 118, Pitch: 15, Dur: 1.8, Vol: 1.0},
				}},
				// LH Alberti bass — continuous broken-chord 16ths (C: C-G-E-G; G7: B-G-D-G).
				{Inst: "piano-felt", Hits: []Hit{
					// Bars 1-2: C (C3-G3-E3-G3) ×.
					{Step: 0, Pitch: -9, Vol: 0.6}, {Step: 2, Pitch: -2}, {Step: 4, Pitch: -5}, {Step: 6, Pitch: -2},
					{Step: 8, Pitch: -9}, {Step: 10, Pitch: -2}, {Step: 12, Pitch: -5}, {Step: 14, Pitch: -2},
					// G7 (B2-G3-D3-G3).
					{Step: 16, Pitch: -10}, {Step: 18, Pitch: -2}, {Step: 20, Pitch: -7}, {Step: 22, Pitch: -2},
					{Step: 24, Pitch: -10}, {Step: 26, Pitch: -2}, {Step: 28, Pitch: -7}, {Step: 30, Pitch: -2},
					// Bar 3: C then F (C-G-E-G / A2-F3-C3-F3).
					{Step: 32, Pitch: -9}, {Step: 34, Pitch: -2}, {Step: 36, Pitch: -5}, {Step: 38, Pitch: -2},
					{Step: 40, Pitch: -12}, {Step: 42, Pitch: -4}, {Step: 44, Pitch: -9}, {Step: 46, Pitch: -4},
					// Bar 4: G7 → C cadence.
					{Step: 48, Pitch: -10}, {Step: 50, Pitch: -2}, {Step: 52, Pitch: -7}, {Step: 54, Pitch: -2},
					{Step: 56, Pitch: -9}, {Step: 58, Pitch: -2}, {Step: 60, Pitch: -5}, {Step: 62, Pitch: -2},
					// Bars 5-6: C / G7 again.
					{Step: 64, Pitch: -9}, {Step: 66, Pitch: -2}, {Step: 68, Pitch: -5}, {Step: 70, Pitch: -2},
					{Step: 72, Pitch: -9}, {Step: 74, Pitch: -2}, {Step: 76, Pitch: -5}, {Step: 78, Pitch: -2},
					{Step: 80, Pitch: -10}, {Step: 82, Pitch: -2}, {Step: 84, Pitch: -7}, {Step: 86, Pitch: -2},
					{Step: 88, Pitch: -10}, {Step: 90, Pitch: -2}, {Step: 92, Pitch: -7}, {Step: 94, Pitch: -2},
					// Bars 7-8: C / G7 → C.
					{Step: 96, Pitch: -9}, {Step: 98, Pitch: -2}, {Step: 100, Pitch: -5}, {Step: 102, Pitch: -2},
					{Step: 104, Pitch: -10}, {Step: 106, Pitch: -2}, {Step: 108, Pitch: -7}, {Step: 110, Pitch: -2},
					{Step: 112, Pitch: -10}, {Step: 114, Pitch: -2}, {Step: 116, Pitch: -7}, {Step: 118, Pitch: -2},
					{Step: 120, Pitch: -9}, {Step: 122, Pitch: -2}, {Step: 124, Pitch: -5, Dur: 1.0},
				}},
			},
		},

		// ── 4. Eagles — Hotel California ───────────────────────────────────────
		// Bm. Progression Bm-F#7-A-E-G-D-Em-F#7 (one bar each), arpeggiated 12-string
		// + walking bass + harmonized twin-lead outro (bars 7-8) + relaxed rock kit.
		{
			Stem: "eagles-hotel-california", BPM: 75, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "guitar-steel", Name: "12-String", Volume: 0.7, ReverbSend: 0.18},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "guitar-electric", Name: "Lead Hi", Volume: 0.72, ReverbSend: 0.22},
				{ID: "guitar-electric", Name: "Lead Lo", Volume: 0.62, ReverbSend: 0.22},
				{ID: "kick", Name: "Kick", Volume: 0.9},
				{ID: "snare", Name: "Snare", Volume: 0.82},
				{ID: "hihat", Name: "Hihat", Volume: 0.5},
			},
			Rows: []RowSpec{
				// 12-string arpeggios (triad up-down per bar).
				{Inst: "guitar-steel", Hits: []Hit{
					{Step: 0, Pitch: -10, Dur: 0.50, Vol: 0.7}, {Step: 2, Pitch: -3, Dur: 0.50}, {Step: 4, Pitch: 2, Dur: 0.50}, {Step: 6, Pitch: 5, Dur: 0.50}, {Step: 8, Pitch: 9, Dur: 0.50}, {Step: 10, Pitch: 5, Dur: 0.50}, {Step: 12, Pitch: 2, Dur: 0.50}, {Step: 14, Pitch: -3, Dur: 0.50}, {Step: 16, Pitch: -15, Dur: 0.50}, {Step: 18, Pitch: -8, Dur: 0.50}, {Step: 20, Pitch: -5, Dur: 0.50}, {Step: 22, Pitch: 1, Dur: 0.50}, {Step: 24, Pitch: 4, Dur: 0.50}, {Step: 26, Pitch: 1, Dur: 0.50}, {Step: 28, Pitch: -5, Dur: 0.50}, {Step: 30, Pitch: -8, Dur: 0.50}, {Step: 32, Pitch: -12, Dur: 0.50}, {Step: 34, Pitch: -5, Dur: 0.50}, {Step: 36, Pitch: 0, Dur: 0.50}, {Step: 38, Pitch: 4, Dur: 0.50}, {Step: 40, Pitch: 7, Dur: 0.50}, {Step: 42, Pitch: 4, Dur: 0.50}, {Step: 44, Pitch: 0, Dur: 0.50}, {Step: 46, Pitch: -5, Dur: 0.50}, {Step: 48, Pitch: -17, Dur: 0.50}, {Step: 50, Pitch: -10, Dur: 0.50}, {Step: 52, Pitch: -5, Dur: 0.50}, {Step: 54, Pitch: -1, Dur: 0.50}, {Step: 56, Pitch: 2, Dur: 0.50}, {Step: 58, Pitch: -1, Dur: 0.50}, {Step: 60, Pitch: -5, Dur: 0.50}, {Step: 62, Pitch: -10, Dur: 0.50}, {Step: 64, Pitch: -14, Dur: 0.50}, {Step: 66, Pitch: -7, Dur: 0.50}, {Step: 68, Pitch: -2, Dur: 0.50}, {Step: 70, Pitch: 2, Dur: 0.50}, {Step: 72, Pitch: 5, Dur: 0.50}, {Step: 74, Pitch: 2, Dur: 0.50}, {Step: 76, Pitch: -2, Dur: 0.50}, {Step: 78, Pitch: -7, Dur: 0.50}, {Step: 80, Pitch: -19, Dur: 0.50}, {Step: 82, Pitch: -12, Dur: 0.50}, {Step: 84, Pitch: -7, Dur: 0.50}, {Step: 86, Pitch: -3, Dur: 0.50}, {Step: 88, Pitch: 0, Dur: 0.50}, {Step: 90, Pitch: -3, Dur: 0.50}, {Step: 92, Pitch: -7, Dur: 0.50}, {Step: 94, Pitch: -12, Dur: 0.50}, {Step: 96, Pitch: -17, Dur: 0.50}, {Step: 98, Pitch: -10, Dur: 0.50}, {Step: 100, Pitch: -5, Dur: 0.50}, {Step: 102, Pitch: -2, Dur: 0.50}, {Step: 104, Pitch: 2, Dur: 0.50}, {Step: 106, Pitch: -2, Dur: 0.50}, {Step: 108, Pitch: -5, Dur: 0.50}, {Step: 110, Pitch: -10, Dur: 0.50}, {Step: 112, Pitch: -15, Dur: 0.50}, {Step: 114, Pitch: -8, Dur: 0.50}, {Step: 116, Pitch: -5, Dur: 0.50}, {Step: 118, Pitch: 1, Dur: 0.50}, {Step: 120, Pitch: 4, Dur: 0.50}, {Step: 122, Pitch: 1, Dur: 0.50}, {Step: 124, Pitch: -5, Dur: 0.50}, {Step: 126, Pitch: -8, Dur: 0.50},
				}},
				// Bass — root + fifth per bar.
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -10, Dur: 3.50, Vol: 0.85}, {Step: 16, Pitch: -15, Dur: 3.50}, {Step: 32, Pitch: -12, Dur: 3.50}, {Step: 48, Pitch: -17, Dur: 3.50}, {Step: 64, Pitch: -14, Dur: 3.50}, {Step: 80, Pitch: -19, Dur: 3.50}, {Step: 96, Pitch: -17, Dur: 3.50}, {Step: 112, Pitch: -15, Dur: 3.50},
				}},
				// Twin-lead high voice (outro, bars 7-8).
				{Inst: "guitar-electric", Hits: []Hit{
					{Step: 0, Pitch: 9, Dur: 1.00, Vol: 0.72}, {Step: 4, Pitch: 9, Dur: 1.00}, {Step: 8, Pitch: 9, Dur: 0.50}, {Step: 10, Pitch: 7, Dur: 0.50}, {Step: 12, Pitch: 7, Dur: 1.00}, {Step: 16, Pitch: 9, Dur: 1.00}, {Step: 20, Pitch: 5, Dur: 2.00},
				}},
				// Twin-lead low voice — harmonized a 3rd below.
				{Inst: "guitar-electric", Hits: []Hit{
					{Step: 96, Pitch: 11, Dur: 0.5, Vol: 0.7}, {Step: 100, Pitch: 14, Dur: 0.5}, {Step: 104, Pitch: 17, Dur: 0.5}, {Step: 108, Pitch: 16, Dur: 0.5},
					{Step: 112, Pitch: 17, Dur: 0.5}, {Step: 116, Pitch: 14, Dur: 0.5}, {Step: 120, Pitch: 11, Dur: 1.0, Vol: 0.75},
				}},
				// Kick 1 & 3.
				{Inst: "kick", Hits: []Hit{
					{Step: 0, Vol: 0.9}, {Step: 8, Vol: 0.85}, {Step: 16, Vol: 0.9}, {Step: 24, Vol: 0.85},
					{Step: 32, Vol: 0.9}, {Step: 40, Vol: 0.85}, {Step: 48, Vol: 0.9}, {Step: 56, Vol: 0.85},
					{Step: 64, Vol: 0.9}, {Step: 72, Vol: 0.85}, {Step: 80, Vol: 0.9}, {Step: 88, Vol: 0.85},
					{Step: 96, Vol: 0.9}, {Step: 104, Vol: 0.85}, {Step: 112, Vol: 0.9}, {Step: 120, Vol: 0.85},
				}},
				// Snare 2 & 4.
				{Inst: "snare", Hits: []Hit{
					{Step: 4, Vol: 0.82}, {Step: 12, Vol: 0.82}, {Step: 20, Vol: 0.82}, {Step: 28, Vol: 0.82},
					{Step: 36, Vol: 0.82}, {Step: 44, Vol: 0.82}, {Step: 52, Vol: 0.82}, {Step: 60, Vol: 0.82},
					{Step: 68, Vol: 0.82}, {Step: 76, Vol: 0.82}, {Step: 84, Vol: 0.82}, {Step: 92, Vol: 0.82},
					{Step: 100, Vol: 0.82}, {Step: 108, Vol: 0.82}, {Step: 116, Vol: 0.82}, {Step: 124, Vol: 0.9},
				}},
				// Hihat 8ths.
				{Inst: "hihat", Hits: hatEighths(8, 0.5)},
			},
		},

		// ── 5. Toto — Africa ───────────────────────────────────────────────────
		// B major area. The iconic bell ostinato + synth bass + pad chords + the
		// layered half-time groove. 2-bar pattern across 8 bars.
		{
			Stem: "toto-africa", BPM: 92, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "fm-bell", Name: "Kalimba", Volume: 0.62, ReverbSend: 0.2},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "piano-grand", Name: "Chords", Volume: 0.6, ReverbSend: 0.16},
				{ID: "kick", Name: "Kick", Volume: 0.92},
				{ID: "snare", Name: "Snare", Volume: 0.8},
				{ID: "hihat", Name: "Hihat", Volume: 0.5},
				{ID: "shaker", Name: "Shaker", Volume: 0.45},
			},
			Rows: []RowSpec{
				// Bell ostinato — syncopated F#-G#-C#-B figure (repeats each bar).
				{Inst: "fm-bell", Hits: []Hit{
					{Step: 0, Pitch: 12, Dur: 0.50, Vol: 0.62}, {Step: 2, Pitch: 12, Dur: 0.50}, {Step: 4, Pitch: 12, Dur: 0.50}, {Step: 6, Pitch: 12, Dur: 0.50}, {Step: 8, Pitch: 12, Dur: 0.50}, {Step: 10, Pitch: 11, Dur: 0.50}, {Step: 12, Pitch: 16, Dur: 0.50}, {Step: 14, Pitch: 12, Dur: 0.50}, {Step: 16, Pitch: 12, Dur: 0.50}, {Step: 18, Pitch: 12, Dur: 0.50}, {Step: 20, Pitch: 12, Dur: 0.50}, {Step: 22, Pitch: 12, Dur: 0.50}, {Step: 24, Pitch: 11, Dur: 0.50}, {Step: 26, Pitch: 16, Dur: 0.50}, {Step: 28, Pitch: 12, Dur: 0.50}, {Step: 30, Pitch: 12, Dur: 0.50}, {Step: 32, Pitch: 12, Dur: 0.50}, {Step: 34, Pitch: 12, Dur: 0.50}, {Step: 36, Pitch: 12, Dur: 0.50}, {Step: 38, Pitch: 11, Dur: 0.50}, {Step: 40, Pitch: 16, Dur: 0.50}, {Step: 42, Pitch: 12, Dur: 0.50}, {Step: 44, Pitch: 12, Dur: 0.50}, {Step: 46, Pitch: 12, Dur: 0.50}, {Step: 48, Pitch: 12, Dur: 0.50}, {Step: 50, Pitch: 12, Dur: 0.50}, {Step: 52, Pitch: 11, Dur: 0.50}, {Step: 54, Pitch: 16, Dur: 0.50}, {Step: 56, Pitch: 12, Dur: 0.50}, {Step: 58, Pitch: 12, Dur: 0.50}, {Step: 60, Pitch: 12, Dur: 0.50}, {Step: 62, Pitch: 12, Dur: 0.50}, {Step: 64, Pitch: 12, Dur: 0.50}, {Step: 66, Pitch: 11, Dur: 0.50}, {Step: 68, Pitch: 16, Dur: 0.50}, {Step: 70, Pitch: 12, Dur: 0.50}, {Step: 72, Pitch: 12, Dur: 0.50}, {Step: 74, Pitch: 12, Dur: 0.50}, {Step: 76, Pitch: 12, Dur: 0.50}, {Step: 78, Pitch: 12, Dur: 0.50}, {Step: 80, Pitch: 11, Dur: 0.50}, {Step: 82, Pitch: 16, Dur: 0.50}, {Step: 84, Pitch: 12, Dur: 0.50}, {Step: 86, Pitch: 12, Dur: 0.50}, {Step: 88, Pitch: 12, Dur: 0.50}, {Step: 90, Pitch: 12, Dur: 0.50}, {Step: 92, Pitch: 12, Dur: 0.50}, {Step: 94, Pitch: 11, Dur: 0.50}, {Step: 96, Pitch: 16, Dur: 0.50}, {Step: 98, Pitch: 12, Dur: 0.50}, {Step: 100, Pitch: 12, Dur: 0.50}, {Step: 102, Pitch: 12, Dur: 0.50}, {Step: 104, Pitch: 12, Dur: 0.50}, {Step: 106, Pitch: 12, Dur: 0.50}, {Step: 108, Pitch: 11, Dur: 0.50}, {Step: 110, Pitch: 16, Dur: 0.50}, {Step: 112, Pitch: 12, Dur: 0.50}, {Step: 114, Pitch: 12, Dur: 0.50}, {Step: 116, Pitch: 12, Dur: 0.50}, {Step: 118, Pitch: 12, Dur: 0.50}, {Step: 120, Pitch: 12, Dur: 0.50}, {Step: 122, Pitch: 11, Dur: 0.50}, {Step: 124, Pitch: 16, Dur: 0.50}, {Step: 126, Pitch: 12, Dur: 0.50},
				}},
				// Synth bass — roots tracking the iconic Africa chorus loop A-G#m-C#m-B
				// (the "gonna take a lot to drag me away" turnaround; A is the bVII).
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -12, Dur: 1.0, Vol: 0.9}, {Step: 6, Pitch: -12, Dur: 0.5}, {Step: 8, Pitch: -12, Dur: 0.8},
					{Step: 16, Pitch: -13, Dur: 1.0}, {Step: 22, Pitch: -13, Dur: 0.5}, {Step: 24, Pitch: -13, Dur: 0.8},
					{Step: 32, Pitch: -8, Dur: 1.0}, {Step: 38, Pitch: -8, Dur: 0.5}, {Step: 40, Pitch: -8, Dur: 0.8},
					{Step: 48, Pitch: -10, Dur: 1.0}, {Step: 54, Pitch: -10, Dur: 0.5}, {Step: 56, Pitch: -10, Dur: 0.8},
					{Step: 64, Pitch: -12, Dur: 1.0}, {Step: 70, Pitch: -12, Dur: 0.5}, {Step: 72, Pitch: -12, Dur: 0.8},
					{Step: 80, Pitch: -13, Dur: 1.0}, {Step: 86, Pitch: -13, Dur: 0.5}, {Step: 88, Pitch: -13, Dur: 0.8},
					{Step: 96, Pitch: -8, Dur: 1.0}, {Step: 102, Pitch: -8, Dur: 0.5}, {Step: 104, Pitch: -8, Dur: 0.8},
					{Step: 112, Pitch: -10, Dur: 1.0}, {Step: 118, Pitch: -10, Dur: 0.5}, {Step: 120, Pitch: -10, Dur: 0.8},
				}},
				// Pad chords — sustained per bar (A, G#m, C#m, B triads). chordAt spreads
				// the tones over consecutive 16ths with long Dur so they ring as a chord.
				{Inst: "piano-grand", Hits: concat(
					chordAt(0, 3.5, 0.55, 0, 4, 7),
					chordAt(16, 3.5, 0.55, 11, 14, 18),
					chordAt(32, 3.5, 0.55, 4, 7, 11),
					chordAt(48, 3.5, 0.55, 2, 6, 9),
					chordAt(64, 3.5, 0.55, 0, 4, 7),
					chordAt(80, 3.5, 0.55, 11, 14, 18),
					chordAt(96, 3.5, 0.55, 4, 7, 11),
					chordAt(112, 3.5, 0.55, 2, 6, 9),
				)},
				// Kick — the Africa half-time accent (1, & of 2, 3).
				{Inst: "kick", Hits: ostinato(8, []Hit{
					{Step: 0, Vol: 0.92}, {Step: 6, Vol: 0.8}, {Step: 8, Vol: 0.88},
				})},
				// Snare — backbeat 2 & 4 with a 16th grace.
				{Inst: "snare", Hits: ostinato(8, []Hit{
					{Step: 4, Vol: 0.8}, {Step: 12, Vol: 0.8}, {Step: 15, Vol: 0.4},
				})},
				// Hihat 8ths.
				{Inst: "hihat", Hits: hatEighths(8, 0.5)},
				// Shaker 16ths (soft).
				{Inst: "shaker", Hits: hatSixteenths(8, 0.4)},
			},
		},

		// ── 6. Miles Davis — So What ───────────────────────────────────────────
		// D dorian (→ Eb dorian B-section). Bass riff + the quartal "So What chord"
		// piano answer + trumpet/sax head + swing ride. AABA condensed to 8 bars.
		{
			Stem: "miles-so-what", BPM: 136, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "piano-felt", Name: "Piano", Volume: 0.6, ReverbSend: 0.16},
				{ID: "trumpet", Name: "Trumpet", Volume: 0.78, ReverbSend: 0.18},
				{ID: "sax", Name: "Sax", Volume: 0.7, ReverbSend: 0.18},
				{ID: "ride", Name: "Ride", Volume: 0.55},
				{ID: "hihat", Name: "Hat", Volume: 0.4},
			},
			Rows: []RowSpec{
				// Bass — the "So What" 2-bar riff (D dorian), up a semitone for Eb (bars 5-6).
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 2, Pitch: -7, Dur: 0.50, Vol: 0.85}, {Step: 4, Pitch: 2, Dur: 0.50}, {Step: 6, Pitch: 3, Dur: 0.50}, {Step: 8, Pitch: 5, Dur: 0.50}, {Step: 10, Pitch: 7, Dur: 0.50}, {Step: 12, Pitch: 5, Dur: 4.00}, {Step: 34, Pitch: -7, Dur: 0.50}, {Step: 36, Pitch: 2, Dur: 0.50}, {Step: 38, Pitch: 3, Dur: 0.50}, {Step: 40, Pitch: 5, Dur: 0.50}, {Step: 42, Pitch: 7, Dur: 0.50}, {Step: 44, Pitch: 5, Dur: 4.00}, {Step: 66, Pitch: -7, Dur: 0.50}, {Step: 68, Pitch: 2, Dur: 0.50}, {Step: 70, Pitch: 3, Dur: 0.50}, {Step: 72, Pitch: 5, Dur: 0.50}, {Step: 74, Pitch: 7, Dur: 0.50}, {Step: 76, Pitch: 5, Dur: 4.00}, {Step: 98, Pitch: -7, Dur: 0.50}, {Step: 100, Pitch: 2, Dur: 0.50}, {Step: 102, Pitch: 3, Dur: 0.50}, {Step: 104, Pitch: 5, Dur: 0.50}, {Step: 106, Pitch: 7, Dur: 0.50}, {Step: 108, Pitch: 5, Dur: 4.00},
				}},
				// Piano — the "So What chord" quartal answer (D-G-C-F-A) on the riff response.
				{Inst: "piano-felt", Hits: []Hit{
					{Step: 10, Pitch: -5, Dur: 1.00}, {Step: 11, Pitch: 0, Dur: 1.00}, {Step: 12, Pitch: 5, Dur: 1.00}, {Step: 13, Pitch: 10, Dur: 1.00}, {Step: 14, Pitch: 14, Dur: 1.00}, {Step: 16, Pitch: -7, Dur: 1.00}, {Step: 17, Pitch: -2, Dur: 1.00}, {Step: 18, Pitch: 3, Dur: 1.00}, {Step: 19, Pitch: 8, Dur: 1.00}, {Step: 20, Pitch: 12, Dur: 1.00}, {Step: 42, Pitch: -5, Dur: 1.00}, {Step: 43, Pitch: 0, Dur: 1.00}, {Step: 44, Pitch: 5, Dur: 1.00}, {Step: 45, Pitch: 10, Dur: 1.00}, {Step: 46, Pitch: 14, Dur: 1.00}, {Step: 48, Pitch: -7, Dur: 1.00}, {Step: 49, Pitch: -2, Dur: 1.00}, {Step: 50, Pitch: 3, Dur: 1.00}, {Step: 51, Pitch: 8, Dur: 1.00}, {Step: 52, Pitch: 12, Dur: 1.00}, {Step: 74, Pitch: -5, Dur: 1.00}, {Step: 75, Pitch: 0, Dur: 1.00}, {Step: 76, Pitch: 5, Dur: 1.00}, {Step: 77, Pitch: 10, Dur: 1.00}, {Step: 78, Pitch: 14, Dur: 1.00}, {Step: 80, Pitch: -7, Dur: 1.00}, {Step: 81, Pitch: -2, Dur: 1.00}, {Step: 82, Pitch: 3, Dur: 1.00}, {Step: 83, Pitch: 8, Dur: 1.00}, {Step: 84, Pitch: 12, Dur: 1.00}, {Step: 106, Pitch: -5, Dur: 1.00}, {Step: 107, Pitch: 0, Dur: 1.00}, {Step: 108, Pitch: 5, Dur: 1.00}, {Step: 109, Pitch: 10, Dur: 1.00}, {Step: 110, Pitch: 14, Dur: 1.00}, {Step: 112, Pitch: -7, Dur: 1.00}, {Step: 113, Pitch: -2, Dur: 1.00}, {Step: 114, Pitch: 3, Dur: 1.00}, {Step: 115, Pitch: 8, Dur: 1.00}, {Step: 116, Pitch: 12, Dur: 1.00},
				}},
				// Trumpet — head fragment (bars 3-4 and 7-8).
				{Inst: "trumpet", Hits: []Hit{
					{Step: 32, Pitch: 12, Dur: 0.6, Vol: 0.8}, {Step: 36, Pitch: 15, Dur: 0.6}, {Step: 40, Pitch: 17, Dur: 0.5}, {Step: 44, Pitch: 15, Dur: 0.5},
					{Step: 48, Pitch: 12, Dur: 1.2, Vol: 0.85},
					{Step: 96, Pitch: 12, Dur: 0.6, Vol: 0.8}, {Step: 100, Pitch: 15, Dur: 0.6}, {Step: 104, Pitch: 17, Dur: 0.5}, {Step: 108, Pitch: 19, Dur: 0.5},
					{Step: 112, Pitch: 17, Dur: 1.2, Vol: 0.85},
				}},
				// Sax — answers/doubles (bars 5-6 and tail).
				{Inst: "sax", Hits: []Hit{
					{Step: 64, Pitch: 13, Dur: 0.6, Vol: 0.7}, {Step: 68, Pitch: 16, Dur: 0.6}, {Step: 72, Pitch: 18, Dur: 0.5}, {Step: 76, Pitch: 16, Dur: 0.5},
					{Step: 80, Pitch: 13, Dur: 1.2, Vol: 0.75},
					{Step: 116, Pitch: 12, Dur: 0.5, Vol: 0.7}, {Step: 120, Pitch: 10, Dur: 0.5}, {Step: 124, Pitch: 7, Dur: 1.0},
				}},
				// Ride — swing spang-a-lang (beat + swung "and" of 2 & 4).
				{Inst: "ride", Hits: ostinato(8, []Hit{
					{Step: 0, Vol: 0.6}, {Step: 4, Vol: 0.5}, {Step: 6, Vol: 0.4}, {Step: 8, Vol: 0.6}, {Step: 12, Vol: 0.5}, {Step: 14, Vol: 0.4},
				})},
				// Hihat foot 2 & 4.
				{Inst: "hihat", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.4}, {Step: 12, Vol: 0.4}})},
			},
		},

		// ── 7. Tito Puente — Oye Como Va ───────────────────────────────────────
		// A-dorian Latin (Am7 <-> D9 vamp). Organ montuno + horn line (trumpet+sax)
		// + Am-D montuno bass + son clave + cowbell cáscara + conga open/slap.
		{
			Stem: "puente-oye-como-va", BPM: 126, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "piano-grand", Name: "Montuno", Volume: 0.7, ReverbSend: 0.1},
				{ID: "trumpet", Name: "Trumpet", Volume: 0.82, ReverbSend: 0.12},
				{ID: "sax", Name: "Sax", Volume: 0.72, ReverbSend: 0.12},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "sidestick", Name: "Clave", Volume: 0.95},
				{ID: "cowbell", Name: "Cowbell", Volume: 0.7},
				{ID: "conga", Name: "Conga Open", Volume: 0.88},
				{ID: "conga", Name: "Conga Slap", Volume: 0.88},
			},
			Rows: []RowSpec{
				// Montuno — the Oye Como Va organ guajeo, Am7 <-> D9 (A dorian),
				// transcribed from a MIDI of the Santana arrangement (bitmidi 91454).
				{Inst: "piano-grand", Hits: ostinato2(8, concat(
					chordAt(2, 0.3, 0.7, 0, 3, 7),
					chordAt(6, 0.3, 0.7, 3, 7, 10),
					chordAt(10, 0.3, 0.7, 0, 3, 7),
					chordAt(14, 0.3, 0.8, 3, 7, 9),
					chordAt(18, 0.3, 0.7, 5, 9, 12),
					chordAt(22, 0.3, 0.7, 2, 5, 9),
					chordAt(26, 0.3, 0.7, 5, 9, 12),
				))},
				// Trumpet — the horn melody (A dorian), stabs bars 3-4, 7-8.
				{Inst: "trumpet", Hits: []Hit{
					{Step: 0, Pitch: 12, Dur: 0.50, Vol: 0.85}, {Step: 2, Pitch: 14, Dur: 0.50}, {Step: 4, Pitch: 15, Dur: 0.50}, {Step: 6, Pitch: 17, Dur: 0.50}, {Step: 8, Pitch: 14, Dur: 0.50}, {Step: 10, Pitch: 10, Dur: 0.50}, {Step: 12, Pitch: 12, Dur: 1.50}, {Step: 16, Pitch: 12, Dur: 0.50}, {Step: 18, Pitch: 14, Dur: 0.50}, {Step: 20, Pitch: 15, Dur: 0.50}, {Step: 22, Pitch: 17, Dur: 0.50}, {Step: 24, Pitch: 14, Dur: 0.50}, {Step: 26, Pitch: 10, Dur: 0.50}, {Step: 28, Pitch: 12, Dur: 1.50}, {Step: 32, Pitch: 12, Dur: 0.50}, {Step: 34, Pitch: 14, Dur: 0.50}, {Step: 36, Pitch: 15, Dur: 0.50}, {Step: 38, Pitch: 17, Dur: 0.50}, {Step: 40, Pitch: 14, Dur: 0.50}, {Step: 42, Pitch: 10, Dur: 0.50}, {Step: 44, Pitch: 12, Dur: 1.50}, {Step: 48, Pitch: 12, Dur: 0.50}, {Step: 50, Pitch: 14, Dur: 0.50}, {Step: 52, Pitch: 15, Dur: 0.50}, {Step: 54, Pitch: 17, Dur: 0.50}, {Step: 56, Pitch: 14, Dur: 0.50}, {Step: 58, Pitch: 10, Dur: 0.50}, {Step: 60, Pitch: 12, Dur: 1.50}, {Step: 64, Pitch: 12, Dur: 0.50}, {Step: 66, Pitch: 14, Dur: 0.50}, {Step: 68, Pitch: 15, Dur: 0.50}, {Step: 70, Pitch: 17, Dur: 0.50}, {Step: 72, Pitch: 14, Dur: 0.50}, {Step: 74, Pitch: 10, Dur: 0.50}, {Step: 76, Pitch: 12, Dur: 1.50}, {Step: 80, Pitch: 12, Dur: 0.50}, {Step: 82, Pitch: 14, Dur: 0.50}, {Step: 84, Pitch: 15, Dur: 0.50}, {Step: 86, Pitch: 17, Dur: 0.50}, {Step: 88, Pitch: 14, Dur: 0.50}, {Step: 90, Pitch: 10, Dur: 0.50}, {Step: 92, Pitch: 12, Dur: 1.50}, {Step: 96, Pitch: 12, Dur: 0.50}, {Step: 98, Pitch: 14, Dur: 0.50}, {Step: 100, Pitch: 15, Dur: 0.50}, {Step: 102, Pitch: 17, Dur: 0.50}, {Step: 104, Pitch: 14, Dur: 0.50}, {Step: 106, Pitch: 10, Dur: 0.50}, {Step: 108, Pitch: 12, Dur: 1.50}, {Step: 112, Pitch: 12, Dur: 0.50}, {Step: 114, Pitch: 14, Dur: 0.50}, {Step: 116, Pitch: 15, Dur: 0.50}, {Step: 118, Pitch: 17, Dur: 0.50}, {Step: 120, Pitch: 14, Dur: 0.50}, {Step: 122, Pitch: 10, Dur: 0.50}, {Step: 124, Pitch: 12, Dur: 1.50},
				}},
				// Sax — harmonized a 3rd under (A dorian).
				{Inst: "sax", Hits: []Hit{
					{Step: 44, Pitch: 7, Dur: 0.3, Vol: 0.72}, {Step: 46, Pitch: 10, Dur: 0.3}, {Step: 48, Pitch: 12, Dur: 0.5, Vol: 0.8},
					{Step: 56, Pitch: 14, Dur: 0.3}, {Step: 58, Pitch: 12, Dur: 0.5, Vol: 0.8},
					{Step: 108, Pitch: 7, Dur: 0.3, Vol: 0.72}, {Step: 110, Pitch: 10, Dur: 0.3}, {Step: 112, Pitch: 12, Dur: 0.5, Vol: 0.8},
					{Step: 120, Pitch: 14, Dur: 0.3}, {Step: 122, Pitch: 19, Dur: 0.6, Vol: 0.85},
				}},
				// Montuno bass — the Am-D vamp bass, transcribed from the same MIDI.
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					// Oye Como Va — Am7|D9 montuno bass (A2/E2 then D2/A2). [repaired]
					{Step: 0, Pitch: -12, Dur: 0.8, Vol: 0.9}, {Step: 6, Pitch: -12, Dur: 0.4}, {Step: 8, Pitch: -17, Dur: 0.6}, {Step: 12, Pitch: -12, Dur: 0.4},
					{Step: 16, Pitch: -19, Dur: 0.8}, {Step: 22, Pitch: -19, Dur: 0.4}, {Step: 24, Pitch: -12, Dur: 0.6}, {Step: 28, Pitch: -19, Dur: 0.4},
				})},
				// Son clave 2-3 — kept (genre-correct Latin percussion): 2-side beats 2 & 3
				// (steps 4, 8); 3-side 1, "& of 2", 4 (steps 16, 22, 28).
				{Inst: "sidestick", Hits: ostinato2(8, []Hit{
					{Step: 4, Pitch: 6, Vol: 1.0}, {Step: 8, Pitch: 6, Vol: 1.0},
					{Step: 16, Pitch: 6, Vol: 1.1}, {Step: 22, Pitch: 6, Vol: 1.0}, {Step: 28, Pitch: 6, Vol: 1.1},
				})},
				// Cowbell cáscara — quarter pulse.
				{Inst: "cowbell", Hits: ostinato(8, []Hit{
					{Step: 0, Vol: 0.85}, {Step: 4, Vol: 0.7}, {Step: 8, Vol: 0.8}, {Step: 12, Vol: 0.7},
				})},
				// Conga open tone — tumbao accents.
				{Inst: "conga", Hits: ostinato(8, []Hit{
					{Step: 0, Pitch: 0, Dur: 0.4, Vol: 1.0}, {Step: 6, Pitch: 0, Dur: 0.35, Vol: 0.85}, {Step: 10, Pitch: 0, Dur: 0.3, Vol: 0.75},
				})},
				// Conga slap — high crack on "and" of 3/4.
				{Inst: "conga", Hits: ostinato(8, []Hit{
					{Step: 14, Pitch: 9, Dur: 0.22, Vol: 1.0}, {Step: 11, Pitch: 9, Dur: 0.22, Vol: 0.85},
				})},
			},
		},

		// ── 8. Stevie Wonder — Superstition ────────────────────────────────────
		// Eb minor. The multi-tracked riff (2-bar) + octave layer + bass +
		// the iconic hi-hat groove + horn stabs.
		{
			Stem: "wonder-superstition", BPM: 100, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "guitar-electric", Name: "Guitar", Volume: 0.78, ReverbSend: 0.06},
				{ID: "guitar-electric", Name: "Guitar Oct", Volume: 0.5, ReverbSend: 0.06},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.88},
				{ID: "kick", Name: "Kick", Volume: 1.0},
				{ID: "snare", Name: "Snare", Volume: 0.88},
				{ID: "hihat", Name: "Hihat", Volume: 0.55},
				{ID: "trumpet", Name: "Horns", Volume: 0.7, ReverbSend: 0.1},
				{ID: "sax", Name: "Sax", Volume: 0.6, ReverbSend: 0.1},
			},
			Rows: []RowSpec{
				// Riff — the real Superstition hook (Eb minor), transcribed
				// note-by-note from a MIDI of the recording (bitmidi id 97097, ~101 BPM).
				// Same-onset double-stops reduced to the melodic top note.
				{Inst: "guitar-electric", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -6, Dur: 0.45, Vol: 0.78}, {Step: 2, Pitch: 6, Dur: 0.45}, {Step: 4, Pitch: 4, Dur: 0.45}, {Step: 6, Pitch: 6, Dur: 0.45}, {Step: 8, Pitch: -3, Dur: 0.45}, {Step: 10, Pitch: -6, Dur: 0.45}, {Step: 11, Pitch: -8, Dur: 0.45}, {Step: 12, Pitch: -6, Dur: 0.45}, {Step: 16, Pitch: -1, Dur: 0.45}, {Step: 18, Pitch: 1, Dur: 0.45}, {Step: 20, Pitch: 4, Dur: 0.45}, {Step: 22, Pitch: 6, Dur: 0.45}, {Step: 24, Pitch: -3, Dur: 0.45}, {Step: 26, Pitch: -6, Dur: 0.45}, {Step: 28, Pitch: -8, Dur: 0.45}, {Step: 30, Pitch: -6, Dur: 0.45},
				})},
				// Octave layer — light +12 shadow of the riff's melody accents.
				{Inst: "guitar-electric", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -18, Dur: 0.45, Vol: 0.50}, {Step: 2, Pitch: -6, Dur: 0.45}, {Step: 4, Pitch: -8, Dur: 0.45}, {Step: 6, Pitch: -6, Dur: 0.45}, {Step: 8, Pitch: -15, Dur: 0.45}, {Step: 10, Pitch: -18, Dur: 0.45}, {Step: 11, Pitch: -20, Dur: 0.45}, {Step: 12, Pitch: -18, Dur: 0.45}, {Step: 16, Pitch: -13, Dur: 0.45}, {Step: 18, Pitch: -11, Dur: 0.45}, {Step: 20, Pitch: -8, Dur: 0.45}, {Step: 22, Pitch: -6, Dur: 0.45}, {Step: 24, Pitch: -15, Dur: 0.45}, {Step: 26, Pitch: -18, Dur: 0.45}, {Step: 28, Pitch: -20, Dur: 0.45}, {Step: 30, Pitch: -18, Dur: 0.45},
				})},
				// Bass — the real Superstition bassline (doubles the riff an octave+ below),
				// transcribed from the same MIDI. Verbatim octave (D#1 root pops).
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -18, Dur: 0.40, Vol: 0.88}, {Step: 6, Pitch: -18, Dur: 0.40}, {Step: 8, Pitch: -15, Dur: 0.40}, {Step: 10, Pitch: -18, Dur: 0.40}, {Step: 11, Pitch: -20, Dur: 0.40}, {Step: 12, Pitch: -18, Dur: 0.40}, {Step: 16, Pitch: -18, Dur: 0.40}, {Step: 22, Pitch: -18, Dur: 0.40}, {Step: 24, Pitch: -15, Dur: 0.40}, {Step: 26, Pitch: -18, Dur: 0.40}, {Step: 28, Pitch: -20, Dur: 0.40}, {Step: 30, Pitch: -18, Dur: 0.40},
				})},
				// Kick — syncopated funk.
				{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 1.0}, {Step: 7, Vol: 0.85}, {Step: 10, Vol: 0.9}})},
				// Snare 2 & 4.
				{Inst: "snare", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.88}, {Step: 12, Vol: 0.88}, {Step: 15, Vol: 0.35}})},
				// Hihat 16ths (the busy Superstition intro feel).
				{Inst: "hihat", Hits: hatSixteenths(8, 0.5)},
				// Horn stabs (bars 3-4, 7-8).
				{Inst: "trumpet", Hits: []Hit{
					{Step: 40, Pitch: 6, Dur: 0.3, Vol: 0.75}, {Step: 44, Pitch: 9, Dur: 0.3}, {Step: 46, Pitch: 11, Dur: 0.4, Vol: 0.85},
					{Step: 104, Pitch: 6, Dur: 0.3, Vol: 0.75}, {Step: 108, Pitch: 9, Dur: 0.3}, {Step: 110, Pitch: 13, Dur: 0.4, Vol: 0.85},
				}},
				{Inst: "sax", Hits: []Hit{
					{Step: 40, Pitch: -1, Dur: 0.3, Vol: 0.6}, {Step: 44, Pitch: 1, Dur: 0.3}, {Step: 46, Pitch: 4, Dur: 0.4, Vol: 0.7},
					{Step: 104, Pitch: -1, Dur: 0.3, Vol: 0.6}, {Step: 108, Pitch: 1, Dur: 0.3}, {Step: 110, Pitch: 6, Dur: 0.4, Vol: 0.7},
				}},
			},
		},

		// ── 9. Black Box — Ride On Time ────────────────────────────────────────
		// Piano/organ house (A minor, Am-G-F-G). Four-on-floor + the real Ride On
		// Time bassline + two organ pad rows (chord stack).
		{
			Stem: "blackbox-ride-on-time", BPM: 120, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "kick", Name: "Kick", Volume: 1.0},
				{ID: "clap", Name: "Clap", Volume: 0.78},
				{ID: "hihat", Name: "Hihat", Volume: 0.55},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.9},
				{ID: "organ", Name: "Pad Lo", Volume: 0.6, ReverbSend: 0.3},
				// Deep-house warmth: lowpass the upper pad so it sits behind the kick.
				{ID: "organ", Name: "Pad Hi", Volume: 0.5, ReverbSend: 0.3, Effects: []EffectSpec{{Mode: 0, Cutoff: 1800, Q: 0.8}}},
			},
			Rows: []RowSpec{
				// Four-on-floor.
				{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 1.0}, {Step: 4, Vol: 1.0}, {Step: 8, Vol: 1.0}, {Step: 12, Vol: 1.0}})},
				// Clap 2 & 4.
				{Inst: "clap", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.78}, {Step: 12, Vol: 0.78}})},
				// Hihat offbeats.
				{Inst: "hihat", Hits: ostinato(8, []Hit{{Step: 2, Vol: 0.55}, {Step: 6, Vol: 0.55}, {Step: 10, Vol: 0.55}, {Step: 14, Vol: 0.6}})},
				// Bass — the real "Ride On Time" bassline (A minor: an Am bar then the
				// F-G-Em walk), transcribed from a MIDI of the recording (bitmidi 88846).
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -24, Dur: 0.53, Vol: 0.9}, {Step: 3, Pitch: -24, Dur: 0.17}, {Step: 5, Pitch: -24, Dur: 0.17}, {Step: 7, Pitch: -24, Dur: 0.24}, {Step: 8, Pitch: -21, Dur: 0.49}, {Step: 10, Pitch: -24, Dur: 0.49}, {Step: 12, Pitch: -21, Dur: 0.49}, {Step: 14, Pitch: -17, Dur: 0.49}, {Step: 16, Pitch: -28, Dur: 0.53}, {Step: 19, Pitch: -28, Dur: 0.2}, {Step: 21, Pitch: -28, Dur: 0.23}, {Step: 23, Pitch: -28, Dur: 0.23}, {Step: 24, Pitch: -26, Dur: 0.51}, {Step: 26, Pitch: -29, Dur: 0.45}, {Step: 28, Pitch: -26, Dur: 0.47}, {Step: 30, Pitch: -29, Dur: 0.25},
				})},
				// Organ pad low (root+5th) — the Ride On Time progression Am-G-F-G.
				{Inst: "organ", Hits: concat(
					chordAt(0, 7.5, 0.6, 0, 7),
					chordAt(32, 7.5, 0.6, -2, 5),
					chordAt(64, 7.5, 0.6, -4, 3),
					chordAt(96, 7.5, 0.6, -2, 5),
				)},
				// Organ pad high (upper triads) — Am, G, F, G.
				{Inst: "organ", Hits: concat(
					chordAt(0, 7.5, 0.45, 12, 15, 19),
					chordAt(32, 7.5, 0.45, 10, 14, 17),
					chordAt(64, 7.5, 0.45, 8, 12, 15),
					chordAt(96, 7.5, 0.45, 10, 14, 17),
				)},
			},
		},

		// ── 10. Dr. Dre — Nuthin' but a 'G' Thang ──────────────────────────────
		// B-minor funk vamp (built on the Leon Haywood sample); minor-pentatonic
		// G-funk whistle lead + bassline + chord comp + laid-back groove. ~95 BPM,
		// 4/4. (Pitches transposed to the A-based tuning; minor-pentatonic mode
		// preserved — the original centers on B minor, not D.)
		{
			Stem: "dre-g-thang", BPM: 94, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "fm-lead", Name: "Whistle", Volume: 0.6, ReverbSend: 0.2},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.9},
				{ID: "piano-grand", Name: "Keys", Volume: 0.55, ReverbSend: 0.12},
				{ID: "kick", Name: "Kick", Volume: 1.0},
				{ID: "snare", Name: "Snare", Volume: 0.88},
				{ID: "hihat", Name: "Hihat", Volume: 0.5},
			},
			Rows: []RowSpec{
				// G-funk lead — the Moog "whistle" hook (B-minor pentatonic), transcribed
				// from a MIDI of the recording (bitmidi id 41193, 94 BPM); dropped one
				// octave from the recording's B6 register into a usable lead range.
				{Inst: "fm-lead", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: 26, Dur: 0.54, Vol: 0.6}, {Step: 2, Pitch: 24, Dur: 0.53}, {Step: 4, Pitch: 26, Dur: 0.56}, {Step: 6, Pitch: 28, Dur: 0.56}, {Step: 8, Pitch: 26, Dur: 0.56}, {Step: 10, Pitch: 24, Dur: 1.09}, {Step: 14, Pitch: 22, Dur: 0.52}, {Step: 16, Pitch: 21, Dur: 1.02}, {Step: 20, Pitch: 19, Dur: 0.55}, {Step: 22, Pitch: 21, Dur: 2.42},
				})},
				// Bass — the real laid-back B-minor bassline (B1-A1-F#1 / B1-D2-E2-F#2),
				// transcribed from the same MIDI; verbatim octave.
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -22, Dur: 0.23, Vol: 0.9}, {Step: 1, Pitch: -24, Dur: 0.23}, {Step: 2, Pitch: -27, Dur: 0.29}, {Step: 3, Pitch: -22, Dur: 0.26}, {Step: 8, Pitch: -22, Dur: 0.26}, {Step: 9, Pitch: -19, Dur: 0.29}, {Step: 10, Pitch: -17, Dur: 0.29}, {Step: 11, Pitch: -15, Dur: 0.26}, {Step: 16, Pitch: -22, Dur: 0.23}, {Step: 17, Pitch: -19, Dur: 0.29}, {Step: 18, Pitch: -17, Dur: 0.29}, {Step: 19, Pitch: -15, Dur: 0.26}, {Step: 24, Pitch: -22, Dur: 0.23}, {Step: 25, Pitch: -24, Dur: 0.23}, {Step: 26, Pitch: -27, Dur: 0.41}, {Step: 27, Pitch: -22, Dur: 0.26},
				})},
				// Keys — Bm comp stabs (the track's one-chord B-minor vamp).
				{Inst: "piano-grand", Hits: ostinato2(8, concat(
					chordAt(4, 0.4, 0.55, 2, 5, 9),
					chordAt(20, 0.4, 0.5, 2, 5, 9),
				))},
				// Kick — boom-bap-meets-G-funk.
				{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 1.0}, {Step: 6, Vol: 0.8}, {Step: 10, Vol: 0.85}})},
				// Snare 2 & 4.
				{Inst: "snare", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.88}, {Step: 12, Vol: 0.88}})},
				// Hihat swung 8ths.
				{Inst: "hihat", Hits: ostinato(8, []Hit{
					{Step: 0, Vol: 0.5}, {Step: 3, Vol: 0.45}, {Step: 4, Vol: 0.5}, {Step: 7, Vol: 0.45},
					{Step: 8, Vol: 0.5}, {Step: 11, Vol: 0.45}, {Step: 12, Vol: 0.5}, {Step: 15, Vol: 0.45},
				})},
			},
		},

		// ── 11. Gloria Gaynor — I Will Survive (disco) ──────────────────────────
		// The signature cycle-of-4ths piano intro Am-Dm7-G7-Cmaj7-Fmaj7-Bm7b5-E7-Am
		// with the descending top line (A4-A4-F4-E4-E4-D4-G#4-A4, the G#->A leading-tone
		// resolution) + walking disco octave bass + lush string pads + 4-on-floor with
		// claps on 2&4. Transcribed from sheet-music research (A minor, 117 BPM).
		{
			Stem: "gaynor-survive", BPM: 117, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "piano-grand", Name: "Piano", Volume: 0.74, ReverbSend: 0.16},
				{ID: "violin-ensemble", Name: "Strings", Volume: 0.55, ReverbSend: 0.26},
				{ID: "organ", Name: "Held", Volume: 0.4, ReverbSend: 0.2},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "kick", Name: "Kick", Volume: 1.0},
				{ID: "clap", Name: "Clap", Volume: 0.8},
				{ID: "hihat", Name: "Hihat", Volume: 0.5},
			},
			Rows: []RowSpec{
				{Inst: "piano-grand", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 1.00, Vol: 0.74}, {Step: 4, Pitch: 3, Dur: 1.00}, {Step: 8, Pitch: 7, Dur: 1.00}, {Step: 12, Pitch: 12, Dur: 1.00}, {Step: 16, Pitch: -7, Dur: 1.00}, {Step: 20, Pitch: -4, Dur: 1.00}, {Step: 24, Pitch: 0, Dur: 1.00}, {Step: 28, Pitch: 12, Dur: 1.00}, {Step: 32, Pitch: -14, Dur: 1.00}, {Step: 36, Pitch: -10, Dur: 1.00}, {Step: 40, Pitch: -7, Dur: 1.00}, {Step: 44, Pitch: 8, Dur: 1.00}, {Step: 48, Pitch: -9, Dur: 1.00}, {Step: 52, Pitch: -5, Dur: 1.00}, {Step: 56, Pitch: -2, Dur: 1.00}, {Step: 60, Pitch: 7, Dur: 1.00}, {Step: 64, Pitch: -16, Dur: 1.00}, {Step: 68, Pitch: -12, Dur: 1.00}, {Step: 72, Pitch: -9, Dur: 1.00}, {Step: 76, Pitch: 7, Dur: 1.00}, {Step: 80, Pitch: -10, Dur: 1.00}, {Step: 84, Pitch: -7, Dur: 1.00}, {Step: 88, Pitch: -4, Dur: 1.00}, {Step: 92, Pitch: 5, Dur: 1.00}, {Step: 96, Pitch: -17, Dur: 1.00}, {Step: 100, Pitch: -13, Dur: 1.00}, {Step: 104, Pitch: -10, Dur: 1.00}, {Step: 108, Pitch: 11, Dur: 1.00}, {Step: 112, Pitch: -12, Dur: 1.00}, {Step: 116, Pitch: -9, Dur: 1.00}, {Step: 120, Pitch: -5, Dur: 1.00}, {Step: 124, Pitch: 12, Dur: 1.00},
				}},
				{Inst: "violin-ensemble", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 3.50}, {Step: 1, Pitch: 3, Dur: 3.50}, {Step: 2, Pitch: 7, Dur: 3.50}, {Step: 16, Pitch: -7, Dur: 3.50}, {Step: 17, Pitch: -4, Dur: 3.50}, {Step: 18, Pitch: 0, Dur: 3.50}, {Step: 32, Pitch: -14, Dur: 3.50}, {Step: 33, Pitch: -10, Dur: 3.50}, {Step: 34, Pitch: -7, Dur: 3.50}, {Step: 48, Pitch: -9, Dur: 3.50}, {Step: 49, Pitch: -5, Dur: 3.50}, {Step: 50, Pitch: -2, Dur: 3.50}, {Step: 64, Pitch: -16, Dur: 3.50}, {Step: 65, Pitch: -12, Dur: 3.50}, {Step: 66, Pitch: -9, Dur: 3.50}, {Step: 80, Pitch: -10, Dur: 3.50}, {Step: 81, Pitch: -7, Dur: 3.50}, {Step: 82, Pitch: -4, Dur: 3.50}, {Step: 96, Pitch: -17, Dur: 3.50}, {Step: 97, Pitch: -13, Dur: 3.50}, {Step: 98, Pitch: -10, Dur: 3.50}, {Step: 112, Pitch: -12, Dur: 3.50}, {Step: 113, Pitch: -9, Dur: 3.50}, {Step: 114, Pitch: -5, Dur: 3.50},
				}},
				{Inst: "organ", Hits: []Hit{
					{Step: 0, Pitch: -12, Dur: 3.80}, {Step: 16, Pitch: -7, Dur: 3.80}, {Step: 32, Pitch: -14, Dur: 3.80}, {Step: 48, Pitch: -9, Dur: 3.80}, {Step: 64, Pitch: -16, Dur: 3.80}, {Step: 80, Pitch: -10, Dur: 3.80}, {Step: 96, Pitch: -17, Dur: 3.80}, {Step: 112, Pitch: -12, Dur: 3.80},
				}},
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -12, Dur: 0.40, Vol: 0.85}, {Step: 2, Pitch: 0, Dur: 0.40}, {Step: 4, Pitch: -12, Dur: 0.40}, {Step: 6, Pitch: 0, Dur: 0.40}, {Step: 8, Pitch: -12, Dur: 0.40}, {Step: 10, Pitch: 0, Dur: 0.40}, {Step: 12, Pitch: -12, Dur: 0.40}, {Step: 14, Pitch: 0, Dur: 0.40}, {Step: 16, Pitch: -7, Dur: 0.40}, {Step: 18, Pitch: 5, Dur: 0.40}, {Step: 20, Pitch: -7, Dur: 0.40}, {Step: 22, Pitch: 5, Dur: 0.40}, {Step: 24, Pitch: -7, Dur: 0.40}, {Step: 26, Pitch: 5, Dur: 0.40}, {Step: 28, Pitch: -7, Dur: 0.40}, {Step: 30, Pitch: 5, Dur: 0.40}, {Step: 32, Pitch: -14, Dur: 0.40}, {Step: 34, Pitch: -2, Dur: 0.40}, {Step: 36, Pitch: -14, Dur: 0.40}, {Step: 38, Pitch: -2, Dur: 0.40}, {Step: 40, Pitch: -14, Dur: 0.40}, {Step: 42, Pitch: -2, Dur: 0.40}, {Step: 44, Pitch: -14, Dur: 0.40}, {Step: 46, Pitch: -2, Dur: 0.40}, {Step: 48, Pitch: -9, Dur: 0.40}, {Step: 50, Pitch: 3, Dur: 0.40}, {Step: 52, Pitch: -9, Dur: 0.40}, {Step: 54, Pitch: 3, Dur: 0.40}, {Step: 56, Pitch: -9, Dur: 0.40}, {Step: 58, Pitch: 3, Dur: 0.40}, {Step: 60, Pitch: -9, Dur: 0.40}, {Step: 62, Pitch: 3, Dur: 0.40}, {Step: 64, Pitch: -16, Dur: 0.40}, {Step: 66, Pitch: -4, Dur: 0.40}, {Step: 68, Pitch: -16, Dur: 0.40}, {Step: 70, Pitch: -4, Dur: 0.40}, {Step: 72, Pitch: -16, Dur: 0.40}, {Step: 74, Pitch: -4, Dur: 0.40}, {Step: 76, Pitch: -16, Dur: 0.40}, {Step: 78, Pitch: -4, Dur: 0.40}, {Step: 80, Pitch: -10, Dur: 0.40}, {Step: 82, Pitch: 2, Dur: 0.40}, {Step: 84, Pitch: -10, Dur: 0.40}, {Step: 86, Pitch: 2, Dur: 0.40}, {Step: 88, Pitch: -10, Dur: 0.40}, {Step: 90, Pitch: 2, Dur: 0.40}, {Step: 92, Pitch: -10, Dur: 0.40}, {Step: 94, Pitch: 2, Dur: 0.40}, {Step: 96, Pitch: -17, Dur: 0.40}, {Step: 98, Pitch: -5, Dur: 0.40}, {Step: 100, Pitch: -17, Dur: 0.40}, {Step: 102, Pitch: -5, Dur: 0.40}, {Step: 104, Pitch: -17, Dur: 0.40}, {Step: 106, Pitch: -5, Dur: 0.40}, {Step: 108, Pitch: -17, Dur: 0.40}, {Step: 110, Pitch: -5, Dur: 0.40}, {Step: 112, Pitch: -12, Dur: 0.40}, {Step: 114, Pitch: 0, Dur: 0.40}, {Step: 116, Pitch: -12, Dur: 0.40}, {Step: 118, Pitch: 0, Dur: 0.40}, {Step: 120, Pitch: -12, Dur: 0.40}, {Step: 122, Pitch: 0, Dur: 0.40}, {Step: 124, Pitch: -12, Dur: 0.40}, {Step: 126, Pitch: 0, Dur: 0.40},
				}},
				{Inst: "kick", Hits: []Hit{{Step: 0}, {Step: 4}, {Step: 8}, {Step: 12}, {Step: 16}, {Step: 20}, {Step: 24}, {Step: 28}, {Step: 32}, {Step: 36}, {Step: 40}, {Step: 44}, {Step: 48}, {Step: 52}, {Step: 56}, {Step: 60}, {Step: 64}, {Step: 68}, {Step: 72}, {Step: 76}, {Step: 80}, {Step: 84}, {Step: 88}, {Step: 92}, {Step: 96}, {Step: 100}, {Step: 104}, {Step: 108}, {Step: 112}, {Step: 116}, {Step: 120}, {Step: 124}}},
				{Inst: "clap", Hits: []Hit{{Step: 4}, {Step: 12}, {Step: 20}, {Step: 28}, {Step: 36}, {Step: 44}, {Step: 52}, {Step: 60}, {Step: 68}, {Step: 76}, {Step: 84}, {Step: 92}, {Step: 100}, {Step: 108}, {Step: 116}, {Step: 124}}},
				{Inst: "hihat", Hits: hatSixteenths(8, 0.5)},
			},
		},

		// ── 12. Bob Marley — Exodus (organ bubble) ─────────────────────────────
		// Am. The relentless bassline riff + offbeat guitar skank + organ bubble +
		// one-drop drums + horn stabs.
		{
			Stem: "marley-exodus", BPM: 98, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "bass-guitar", Name: "Bass", Volume: 0.92},
				{ID: "guitar-electric", Name: "Skank", Volume: 0.6, ReverbSend: 0.1},
				{ID: "organ", Name: "Bubble", Volume: 0.5, ReverbSend: 0.12},
				{ID: "kick", Name: "Kick", Volume: 0.95},
				{ID: "snare", Name: "Snare", Volume: 0.85},
				{ID: "hihat", Name: "Hihat", Volume: 0.5},
				{ID: "trumpet", Name: "Horns", Volume: 0.7, ReverbSend: 0.12},
				{ID: "sax", Name: "Sax", Volume: 0.6, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				// The real Exodus bassline (Aston Barrett) — A minor with the chromatic
				// G-G#-A walk-up, transcribed note-by-note from a MIDI of the recording
				// (bitmidi id 18775); verbatim low octave (A1 root).
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					{Step: 3, Pitch: -12, Dur: 0.40, Vol: 0.92}, {Step: 5, Pitch: -12, Dur: 0.40}, {Step: 6, Pitch: -14, Dur: 0.40}, {Step: 7, Pitch: -12, Dur: 0.40}, {Step: 9, Pitch: -9, Dur: 0.40}, {Step: 11, Pitch: -12, Dur: 0.40}, {Step: 13, Pitch: -14, Dur: 0.40}, {Step: 14, Pitch: -13, Dur: 0.40}, {Step: 15, Pitch: -12, Dur: 0.40}, {Step: 19, Pitch: -12, Dur: 0.40}, {Step: 21, Pitch: -12, Dur: 0.40}, {Step: 22, Pitch: -14, Dur: 0.40}, {Step: 23, Pitch: -12, Dur: 0.40}, {Step: 25, Pitch: -9, Dur: 0.40}, {Step: 27, Pitch: -12, Dur: 0.40}, {Step: 29, Pitch: -14, Dur: 0.40}, {Step: 30, Pitch: -13, Dur: 0.40}, {Step: 31, Pitch: -12, Dur: 0.40},
				})},
				// Offbeat skank — Am chord chops on the "and"s (tight strum via chordAt).
				{Inst: "guitar-electric", Hits: ostinato(8, concat(
					chordAt(2, 0.2, 0.6, 0, 3, 7),
					chordAt(6, 0.2, 0.6, 0, 3, 7),
					chordAt(10, 0.2, 0.6, 0, 3, 7),
					chordAt(14, 0.2, 0.6, 0, 3, 7),
				))},
				// Organ bubble — staccato offbeat double-hits between skanks.
				{Inst: "organ", Hits: ostinato(8, []Hit{
					{Step: 3, Pitch: 7, Dur: 0.15, Vol: 0.5}, {Step: 4, Pitch: 12, Dur: 0.15, Vol: 0.4},
					{Step: 7, Pitch: 7, Dur: 0.15, Vol: 0.5}, {Step: 8, Pitch: 12, Dur: 0.15, Vol: 0.4},
					{Step: 11, Pitch: 7, Dur: 0.15, Vol: 0.5}, {Step: 12, Pitch: 12, Dur: 0.15, Vol: 0.4},
					{Step: 15, Pitch: 7, Dur: 0.15, Vol: 0.5},
				})},
				// One-drop: kick + snare together on beat 3.
				{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 8, Vol: 0.95}})},
				{Inst: "snare", Hits: ostinato(8, []Hit{{Step: 8, Vol: 0.85}, {Step: 14, Vol: 0.3}})},
				// Hihat 8ths.
				{Inst: "hihat", Hits: hatEighths(8, 0.5)},
				// Horn stabs (bars 4 and 8 — the "Exodus!" punches).
				{Inst: "trumpet", Hits: []Hit{
					{Step: 48, Pitch: 0, Dur: 0.4, Vol: 0.75}, {Step: 52, Pitch: 3, Dur: 0.4}, {Step: 56, Pitch: 7, Dur: 0.6, Vol: 0.85},
					{Step: 112, Pitch: 0, Dur: 0.4, Vol: 0.75}, {Step: 116, Pitch: 3, Dur: 0.4}, {Step: 120, Pitch: 7, Dur: 0.6, Vol: 0.85},
				}},
				{Inst: "sax", Hits: []Hit{
					{Step: 48, Pitch: -5, Dur: 0.4, Vol: 0.6}, {Step: 52, Pitch: 0, Dur: 0.4}, {Step: 56, Pitch: 3, Dur: 0.6, Vol: 0.7},
					{Step: 112, Pitch: -5, Dur: 0.4, Vol: 0.6}, {Step: 116, Pitch: 0, Dur: 0.4}, {Step: 120, Pitch: 3, Dur: 0.6, Vol: 0.7},
				}},
			},
		},

		// ── 13. Chic — Good Times ──────────────────────────────────────────────
		// E minor (E Dorian) — the Em7–A7 two-chord vamp (the C# in A7 gives the
		// Dorian color); ~112 BPM, 4/4 four-on-the-floor. The Bernard Edwards
		// bassline (later sampled in "Rapper's Delight") + Nile Rodgers' bright
		// 16th E9 guitar chucks (authentic funk dominant color over the minor
		// vamp) + soaring string line + Rhodes comp.
		{
			Stem: "chic-good-times", BPM: 117, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "bass-guitar", Name: "Bass", Volume: 0.92},
				{ID: "guitar-electric", Name: "Chuck", Volume: 0.6, ReverbSend: 0.06},
				{ID: "violin-ensemble", Name: "Strings", Volume: 0.55, ReverbSend: 0.24},
				{ID: "fm-epiano", Name: "Rhodes", Volume: 0.5, ReverbSend: 0.12},
				{ID: "kick", Name: "Kick", Volume: 0.95},
				{ID: "hihat", Name: "Hihat", Volume: 0.55},
				{ID: "clap", Name: "Clap", Volume: 0.75},
			},
			Rows: []RowSpec{
				// The iconic Bernard Edwards "Good Times" bassline (A-centric walking riff
				// with chromatic G#-A / C#-D approaches), transcribed note-by-note from a
				// MIDI of the recording (bitmidi 23412, 117 BPM); verbatim octave.
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: -24, Dur: 0.62, Vol: 0.92}, {Step: 4, Pitch: -24, Dur: 0.5}, {Step: 7, Pitch: -26, Dur: 0.46}, {Step: 9, Pitch: -29, Dur: 0.21}, {Step: 10, Pitch: -26, Dur: 0.46}, {Step: 12, Pitch: -25, Dur: 0.46}, {Step: 14, Pitch: -24, Dur: 0.46}, {Step: 16, Pitch: -19, Dur: 0.58}, {Step: 20, Pitch: -19, Dur: 0.46}, {Step: 22, Pitch: -24, Dur: 0.21}, {Step: 23, Pitch: -21, Dur: 0.46}, {Step: 25, Pitch: -21, Dur: 0.21}, {Step: 26, Pitch: -24, Dur: 0.46}, {Step: 28, Pitch: -20, Dur: 0.21}, {Step: 29, Pitch: -19, Dur: 0.46}, {Step: 31, Pitch: -26, Dur: 0.21},
				})},
				// Nile Rodgers 16th chucks — tight muted Am dyad (A-E).
				{Inst: "guitar-electric", Hits: ostinato(8, concat(
					chordAt(1, 0.12, 0.5, 0, 7),
					chordAt(3, 0.12, 0.6, 0, 7),
					chordAt(6, 0.12, 0.55, 0, 7),
					chordAt(9, 0.12, 0.5, 0, 7),
					chordAt(11, 0.12, 0.6, 0, 7),
					chordAt(14, 0.12, 0.55, 0, 7),
				))},
				// String line — the soaring Good Times strings (A minor).
				{Inst: "violin-ensemble", Hits: []Hit{
					{Step: 0, Pitch: 19, Dur: 1.4, Vol: 0.55}, {Step: 8, Pitch: 20, Dur: 1.4}, {Step: 12, Pitch: 19, Dur: 0.8},
					{Step: 16, Pitch: 22, Dur: 1.4}, {Step: 24, Pitch: 19, Dur: 1.4},
					{Step: 64, Pitch: 19, Dur: 1.4, Vol: 0.55}, {Step: 72, Pitch: 20, Dur: 1.4}, {Step: 76, Pitch: 24, Dur: 0.8},
					{Step: 80, Pitch: 22, Dur: 1.4}, {Step: 88, Pitch: 19, Dur: 1.4},
				}},
				// Rhodes comp — Am chord stabs on the "and".
				{Inst: "fm-epiano", Hits: ostinato(8, concat(
					chordAt(2, 0.3, 0.5, 0, 3, 7),
					chordAt(10, 0.3, 0.5, 0, 3, 7),
				))},
				// Four-on-floor.
				{Inst: "kick", Hits: ostinato(8, []Hit{{Step: 0, Vol: 0.95}, {Step: 4, Vol: 0.95}, {Step: 8, Vol: 0.95}, {Step: 12, Vol: 0.95}})},
				// Open-hat offbeats.
				{Inst: "hihat", Hits: ostinato(8, []Hit{{Step: 2, Vol: 0.55}, {Step: 6, Vol: 0.6}, {Step: 10, Vol: 0.55}, {Step: 14, Vol: 0.6}})},
				// Clap 2 & 4.
				{Inst: "clap", Hits: ostinato(8, []Hit{{Step: 4, Vol: 0.75}, {Step: 12, Vol: 0.75}})},
			},
		},

		// ── 14. Jobim — The Girl from Ipanema (sax) ────────────────────────────
		// F major. Nylon-guitar bossa comp + the Stan Getz tenor melody (sax) +
		// flute answer + soft bossa bass + shaker/rim percussion.
		{
			Stem: "jobim-ipanema", BPM: 130, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{
				{ID: "guitar-nylon", Name: "Violão", Volume: 0.62, ReverbSend: 0.14},
				{ID: "sax", Name: "Sax", Volume: 0.75, ReverbSend: 0.2},
				{ID: "flute", Name: "Flute", Volume: 0.55, ReverbSend: 0.2},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.8},
				{ID: "shaker", Name: "Shaker", Volume: 0.4},
				{ID: "sidestick", Name: "Rim", Volume: 0.55},
				{ID: "piano-felt", Name: "Comp", Volume: 0.4, ReverbSend: 0.14},
			},
			Rows: []RowSpec{
				// Bossa nylon comp — syncopated Fmaj7 / G7 chords (2-bar), strummed via chordAt.
				{Inst: "guitar-nylon", Hits: ostinato2(8, concat(
					chordAt(0, 0.4, 0.6, 8, 12, 15, 19),
					chordAt(6, 0.4, 0.55, 8, 12, 15, 19),
					chordAt(11, 0.4, 0.55, 8, 12, 15),
					chordAt(16, 0.4, 0.6, 10, 14, 17, 20),
					chordAt(22, 0.4, 0.55, 10, 14, 17),
					chordAt(27, 0.4, 0.55, 10, 14, 17),
				))},
				// Sax melody — the real "Girl from Ipanema" head (the repeated-E/G figure),
				// transcribed from a MIDI of the recording (bitmidi 102483); F major over
				// Fmaj7/G7. One chromatic D# passing note snapped to D to stay in key.
				{Inst: "sax", Hits: []Hit{
					// "Girl from Ipanema" A-section — transcribed verbatim from the
					// trillian.mit.edu ABC (F major): repeated-G motif G4 G4 E4 E4 E4 D4,
					// sequenced down a step each ii-V bar, bossa anticipation across bars.
					{Step: 0, Pitch: 10, Dur: 1.00, Vol: 0.78}, {Step: 4, Pitch: 10, Dur: 0.50}, {Step: 6, Pitch: 7, Dur: 0.50}, {Step: 8, Pitch: 7, Dur: 1.00}, {Step: 12, Pitch: 7, Dur: 0.50}, {Step: 14, Pitch: 5, Dur: 0.50}, {Step: 16, Pitch: 10, Dur: 1.00}, {Step: 20, Pitch: 10, Dur: 0.50}, {Step: 22, Pitch: 7, Dur: 0.50}, {Step: 24, Pitch: 7, Dur: 0.50}, {Step: 26, Pitch: 7, Dur: 0.50}, {Step: 28, Pitch: 5, Dur: 0.50}, {Step: 30, Pitch: 10, Dur: 1.50}, {Step: 36, Pitch: 10, Dur: 0.50}, {Step: 38, Pitch: 7, Dur: 0.50}, {Step: 40, Pitch: 7, Dur: 0.50}, {Step: 42, Pitch: 7, Dur: 0.50}, {Step: 44, Pitch: 5, Dur: 0.50}, {Step: 46, Pitch: 10, Dur: 1.50}, {Step: 52, Pitch: 10, Dur: 0.50}, {Step: 54, Pitch: 7, Dur: 0.50}, {Step: 56, Pitch: 7, Dur: 0.50}, {Step: 58, Pitch: 7, Dur: 0.50}, {Step: 60, Pitch: 5, Dur: 0.50}, {Step: 62, Pitch: 8, Dur: 1.50}, {Step: 68, Pitch: 8, Dur: 0.50}, {Step: 70, Pitch: 5, Dur: 0.50}, {Step: 72, Pitch: 5, Dur: 0.50}, {Step: 74, Pitch: 5, Dur: 0.50}, {Step: 76, Pitch: 3, Dur: 0.50}, {Step: 78, Pitch: 7, Dur: 1.50}, {Step: 84, Pitch: 7, Dur: 0.50}, {Step: 86, Pitch: 3, Dur: 0.50}, {Step: 88, Pitch: 3, Dur: 0.50}, {Step: 90, Pitch: 3, Dur: 0.50}, {Step: 92, Pitch: 2, Dur: 0.50}, {Step: 94, Pitch: 3, Dur: 4.50},
				}},
				// Flute — answer phrase (bars 3-4, 7-8).
				{Inst: "flute", Hits: []Hit{
					{Step: 36, Pitch: 17, Dur: 0.6, Vol: 0.5}, {Step: 40, Pitch: 15, Dur: 0.6}, {Step: 44, Pitch: 12, Dur: 1.0},
					{Step: 100, Pitch: 17, Dur: 0.6, Vol: 0.5}, {Step: 104, Pitch: 19, Dur: 0.6}, {Step: 108, Pitch: 20, Dur: 1.2},
				}},
				// Soft bossa bass — root/fifth on 1 & 3 (Fmaj7 / G7).
				{Inst: "bass-guitar", Hits: ostinato2(8, []Hit{
					// Soft bossa bass — root/fifth on 1 & 3 (Fmaj7 / G7).
					{Step: 0, Pitch: -16, Dur: 0.8, Vol: 0.8}, {Step: 8, Pitch: -9, Dur: 0.8},
					{Step: 16, Pitch: -14, Dur: 0.8}, {Step: 24, Pitch: -7, Dur: 0.8},
				})},
				// Shaker 16ths.
				{Inst: "shaker", Hits: hatSixteenths(8, 0.4)},
				// Rim — soft bossa clave.
				{Inst: "sidestick", Hits: ostinato2(8, []Hit{
					{Step: 0, Pitch: 0, Vol: 0.55}, {Step: 6, Pitch: 0, Vol: 0.5}, {Step: 10, Pitch: 0, Vol: 0.5},
					{Step: 20, Pitch: 0, Vol: 0.55}, {Step: 26, Pitch: 0, Vol: 0.5},
				})},
				// Felt-piano light comp on beat 1.
				{Inst: "piano-felt", Hits: ostinato2(8, concat(
					chordAt(0, 0.5, 0.4, 8, 12, 15),
					chordAt(16, 0.5, 0.4, 10, 14, 17),
				))},
			},
		},

		// ── 15. B.B. King — The Thrill Is Gone ─────────────────────────────────
		// Bm minor-blues 12-bar form (Bm·Bm·Bm·Bm Em·Em·Bm·Bm Gmaj7·F#7·Bm·F#7).
		// ~98 BPM. NOTE: the 1969 Completely Well master is STRAIGHT-eighth R&B
		// feel — producer Bill Szymczyk + Jemmott/Lovelle deliberately converted
		// King's usual 12/8 shuffle to straight subdivisions — so this rides
		// straight hats on a 4/4 grid, NOT a swung/triplet blues.
		// Iconic lead + walking bass + lush strings + organ comp + R&B kit.
		{
			Stem: "bbking-thrill-is-gone", BPM: 88, Subdiv: 16, Bars: 12,
			Insts: []InstSpec{
				{ID: "guitar-electric", Name: "Lead", Volume: 0.78, ReverbSend: 0.22},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "violin-ensemble", Name: "Strings", Volume: 0.5, ReverbSend: 0.28},
				{ID: "organ", Name: "Organ", Volume: 0.45, ReverbSend: 0.16},
				{ID: "kick", Name: "Kick", Volume: 0.9},
				{ID: "snare", Name: "Snare", Volume: 0.8},
				{ID: "hihat", Name: "Hihat", Volume: 0.45},
			},
			Rows: []RowSpec{
				// Lead — minor-blues licks (B blues scale: B-D-E-F-F#-A), call per section.
				{Inst: "guitar-electric", Hits: []Hit{
					{Step: 0, Pitch: 14, Dur: 0.50, Vol: 0.78}, {Step: 2, Pitch: 14, Dur: 0.50}, {Step: 4, Pitch: 12, Dur: 1.00}, {Step: 8, Pitch: 7, Dur: 0.50}, {Step: 10, Pitch: 5, Dur: 2.00}, {Step: 96, Pitch: 14, Dur: 0.50}, {Step: 98, Pitch: 14, Dur: 0.50}, {Step: 100, Pitch: 12, Dur: 1.00}, {Step: 104, Pitch: 7, Dur: 0.50}, {Step: 106, Pitch: 5, Dur: 2.00}, {Step: 160, Pitch: 14, Dur: 0.50}, {Step: 162, Pitch: 14, Dur: 0.50}, {Step: 164, Pitch: 12, Dur: 1.00}, {Step: 168, Pitch: 7, Dur: 0.50}, {Step: 170, Pitch: 5, Dur: 2.00},
				}},
				// Walking bass — roots through the 12-bar minor form.
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -10, Dur: 1.50, Vol: 0.85}, {Step: 8, Pitch: -15, Dur: 1.50}, {Step: 16, Pitch: -10, Dur: 1.50}, {Step: 24, Pitch: -15, Dur: 1.50}, {Step: 32, Pitch: -10, Dur: 1.50}, {Step: 40, Pitch: -15, Dur: 1.50}, {Step: 48, Pitch: -10, Dur: 1.50}, {Step: 56, Pitch: -15, Dur: 1.50}, {Step: 64, Pitch: -17, Dur: 1.50}, {Step: 72, Pitch: -10, Dur: 1.50}, {Step: 80, Pitch: -17, Dur: 1.50}, {Step: 88, Pitch: -10, Dur: 1.50}, {Step: 96, Pitch: -10, Dur: 1.50}, {Step: 104, Pitch: -15, Dur: 1.50}, {Step: 112, Pitch: -10, Dur: 1.50}, {Step: 120, Pitch: -15, Dur: 1.50}, {Step: 128, Pitch: -14, Dur: 1.50}, {Step: 136, Pitch: -7, Dur: 1.50}, {Step: 144, Pitch: -15, Dur: 1.50}, {Step: 152, Pitch: -8, Dur: 1.50}, {Step: 160, Pitch: -10, Dur: 1.50}, {Step: 168, Pitch: -15, Dur: 1.50}, {Step: 176, Pitch: -10, Dur: 1.50}, {Step: 184, Pitch: -15, Dur: 1.50},
				}},
				// Strings — lush sustained chord per section (root+5th spread).
				{Inst: "violin-ensemble", Hits: concat(
					chordAt(0, 3.5, 0.5, 2, 9),
					chordAt(64, 3.5, 0.5, 7, 14),
					chordAt(96, 3.5, 0.5, 2, 9),
					chordAt(128, 3.5, 0.5, 10, 14),
					chordAt(144, 3.5, 0.5, 9, 13),
					chordAt(160, 3.5, 0.5, 2, 9),
				)},
				// Organ comp — soft sustained chords mirroring the strings.
				{Inst: "organ", Hits: concat(
					chordAt(0, 7.5, 0.45, 2, 5, 9),
					chordAt(64, 7.5, 0.45, 7, 10, 14),
					chordAt(96, 7.5, 0.45, 2, 5, 9),
					chordAt(128, 3.5, 0.45, 10, 14),
					chordAt(144, 3.5, 0.45, 9, 13),
				)},
				// R&B kit — kick 1 & 3, snare 2 & 4, STRAIGHT eighth-note hats
				// (the 1969 record is straight-eighth, not a swung/triplet shuffle).
				{Inst: "kick", Hits: ostinato(12, []Hit{{Step: 0, Vol: 0.9}, {Step: 8, Vol: 0.85}})},
				{Inst: "snare", Hits: ostinato(12, []Hit{{Step: 4, Vol: 0.8}, {Step: 12, Vol: 0.8}})},
				{Inst: "hihat", Hits: ostinato(12, []Hit{
					{Step: 0, Vol: 0.45}, {Step: 2, Vol: 0.4}, {Step: 4, Vol: 0.45}, {Step: 6, Vol: 0.4},
					{Step: 8, Vol: 0.45}, {Step: 10, Vol: 0.4}, {Step: 12, Vol: 0.45}, {Step: 14, Vol: 0.4},
				})},
			},
		},

		// ── Bach — Prelude No. 1 in C major, BWV 846 (WTC I), opening 4 bars ──────
		// Precise transcription (music21 corpus bach/bwv846) of the moving 16th-note
		// arpeggio voice: CM, Dm7/C, G7/B, CM. Solo keyboard; one piano row.
		{
			Stem: "bach-prelude-c", BPM: 66, Subdiv: 16, Bars: 4,
			Insts: []InstSpec{
				{ID: "piano-grand", Name: "Piano", Volume: 0.8, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "piano-grand", Hits: []Hit{
					// Bar 1 (C maj) — continuous 16th-note broken chord.
					{Step: 0, Pitch: 3, Dur: 0.45}, {Step: 1, Pitch: 7, Dur: 0.45}, {Step: 2, Pitch: 10, Dur: 0.45}, {Step: 3, Pitch: 15, Dur: 0.45}, {Step: 4, Pitch: 19, Dur: 0.45}, {Step: 5, Pitch: 10, Dur: 0.45}, {Step: 6, Pitch: 15, Dur: 0.45}, {Step: 7, Pitch: 19, Dur: 0.45}, {Step: 8, Pitch: 3, Dur: 0.45}, {Step: 9, Pitch: 7, Dur: 0.45}, {Step: 10, Pitch: 10, Dur: 0.45}, {Step: 11, Pitch: 15, Dur: 0.45}, {Step: 12, Pitch: 19, Dur: 0.45}, {Step: 13, Pitch: 10, Dur: 0.45}, {Step: 14, Pitch: 15, Dur: 0.45}, {Step: 15, Pitch: 19, Dur: 0.45},
					// Bar 2 (Dm7/C) — continuous 16th-note broken chord.
					{Step: 16, Pitch: 3, Dur: 0.45}, {Step: 17, Pitch: 5, Dur: 0.45}, {Step: 18, Pitch: 12, Dur: 0.45}, {Step: 19, Pitch: 17, Dur: 0.45}, {Step: 20, Pitch: 20, Dur: 0.45}, {Step: 21, Pitch: 12, Dur: 0.45}, {Step: 22, Pitch: 17, Dur: 0.45}, {Step: 23, Pitch: 20, Dur: 0.45}, {Step: 24, Pitch: 3, Dur: 0.45}, {Step: 25, Pitch: 5, Dur: 0.45}, {Step: 26, Pitch: 12, Dur: 0.45}, {Step: 27, Pitch: 17, Dur: 0.45}, {Step: 28, Pitch: 20, Dur: 0.45}, {Step: 29, Pitch: 12, Dur: 0.45}, {Step: 30, Pitch: 17, Dur: 0.45}, {Step: 31, Pitch: 20, Dur: 0.45},
					// Bar 3 (G7/B) — continuous 16th-note broken chord.
					{Step: 32, Pitch: 2, Dur: 0.45}, {Step: 33, Pitch: 5, Dur: 0.45}, {Step: 34, Pitch: 10, Dur: 0.45}, {Step: 35, Pitch: 17, Dur: 0.45}, {Step: 36, Pitch: 20, Dur: 0.45}, {Step: 37, Pitch: 10, Dur: 0.45}, {Step: 38, Pitch: 17, Dur: 0.45}, {Step: 39, Pitch: 20, Dur: 0.45}, {Step: 40, Pitch: 2, Dur: 0.45}, {Step: 41, Pitch: 5, Dur: 0.45}, {Step: 42, Pitch: 10, Dur: 0.45}, {Step: 43, Pitch: 17, Dur: 0.45}, {Step: 44, Pitch: 20, Dur: 0.45}, {Step: 45, Pitch: 10, Dur: 0.45}, {Step: 46, Pitch: 17, Dur: 0.45}, {Step: 47, Pitch: 20, Dur: 0.45},
					// Bar 4 (C maj) — continuous 16th-note broken chord.
					{Step: 48, Pitch: 3, Dur: 0.45}, {Step: 49, Pitch: 7, Dur: 0.45}, {Step: 50, Pitch: 10, Dur: 0.45}, {Step: 51, Pitch: 15, Dur: 0.45}, {Step: 52, Pitch: 19, Dur: 0.45}, {Step: 53, Pitch: 10, Dur: 0.45}, {Step: 54, Pitch: 15, Dur: 0.45}, {Step: 55, Pitch: 19, Dur: 0.45}, {Step: 56, Pitch: 3, Dur: 0.45}, {Step: 57, Pitch: 7, Dur: 0.45}, {Step: 58, Pitch: 10, Dur: 0.45}, {Step: 59, Pitch: 15, Dur: 0.45}, {Step: 60, Pitch: 19, Dur: 0.45}, {Step: 61, Pitch: 10, Dur: 0.45}, {Step: 62, Pitch: 15, Dur: 0.45}, {Step: 63, Pitch: 19, Dur: 0.45},
				}},
			},
		},

		// ── Bach — Partita in A minor for solo flute, BWV 1013, Allemande opening ──
		// Precise transcription (Mutopia bwv1013.mid via music21) of the perpetual
		// 16th-note A-minor line. Solo flute; one monophonic row.
		{
			Stem: "bach-flute-allemande", BPM: 88, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "flute", Name: "Flute", Volume: 0.85, ReverbSend: 0.14},
			},
			Rows: []RowSpec{
				{Inst: "flute", Hits: []Hit{
					{Step: 1, Pitch: 19, Dur: 0.3}, {Step: 2, Pitch: 24, Dur: 0.3}, {Step: 3, Pitch: 23, Dur: 0.3}, {Step: 4, Pitch: 24, Dur: 0.3}, {Step: 5, Pitch: 27, Dur: 0.3}, {Step: 6, Pitch: 24, Dur: 0.3}, {Step: 7, Pitch: 19, Dur: 0.3}, {Step: 8, Pitch: 12, Dur: 0.3}, {Step: 9, Pitch: 19, Dur: 0.3}, {Step: 10, Pitch: 24, Dur: 0.3}, {Step: 11, Pitch: 23, Dur: 0.3}, {Step: 12, Pitch: 24, Dur: 0.3}, {Step: 13, Pitch: 27, Dur: 0.3}, {Step: 14, Pitch: 24, Dur: 0.3}, {Step: 15, Pitch: 19, Dur: 0.3}, {Step: 16, Pitch: 12, Dur: 0.3}, {Step: 17, Pitch: 15, Dur: 0.3}, {Step: 18, Pitch: 19, Dur: 0.3}, {Step: 19, Pitch: 20, Dur: 0.3}, {Step: 20, Pitch: 11, Dur: 0.3}, {Step: 21, Pitch: 20, Dur: 0.3}, {Step: 22, Pitch: 19, Dur: 0.3}, {Step: 23, Pitch: 17, Dur: 0.3}, {Step: 24, Pitch: 15, Dur: 0.3}, {Step: 25, Pitch: 19, Dur: 0.3}, {Step: 26, Pitch: 23, Dur: 0.3}, {Step: 27, Pitch: 24, Dur: 0.3}, {Step: 28, Pitch: 7, Dur: 0.3}, {Step: 29, Pitch: 17, Dur: 0.3}, {Step: 30, Pitch: 15, Dur: 0.3}, {Step: 31, Pitch: 14, Dur: 0.3},
				}},
			},
		},

		// ── Bach — Cello Suite 1 Prelude (BWV 1007), solo cello ──
		{
			Stem: "bach-cello-prelude", BPM: 68, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "cello", Name: "Cello", Volume: 0.85, ReverbSend: 0.14},
			},
			Rows: []RowSpec{
				{Inst: "cello", Hits: []Hit{
					{Step: 0, Pitch: -14, Dur: 0.4}, {Step: 1, Pitch: -7, Dur: 0.4}, {Step: 2, Pitch: 2, Dur: 0.4}, {Step: 3, Pitch: 0, Dur: 0.4}, {Step: 4, Pitch: 2, Dur: 0.4}, {Step: 5, Pitch: -7, Dur: 0.4}, {Step: 6, Pitch: 2, Dur: 0.4}, {Step: 7, Pitch: -7, Dur: 0.4}, {Step: 8, Pitch: -14, Dur: 0.4}, {Step: 9, Pitch: -7, Dur: 0.4}, {Step: 10, Pitch: 2, Dur: 0.4}, {Step: 11, Pitch: 0, Dur: 0.4}, {Step: 12, Pitch: 2, Dur: 0.4}, {Step: 13, Pitch: -7, Dur: 0.4}, {Step: 14, Pitch: 2, Dur: 0.4}, {Step: 15, Pitch: -7, Dur: 0.4}, {Step: 16, Pitch: -14, Dur: 0.4}, {Step: 17, Pitch: -5, Dur: 0.4}, {Step: 18, Pitch: 3, Dur: 0.4}, {Step: 19, Pitch: 2, Dur: 0.4}, {Step: 20, Pitch: 3, Dur: 0.4}, {Step: 21, Pitch: -5, Dur: 0.4}, {Step: 22, Pitch: 3, Dur: 0.4}, {Step: 23, Pitch: -5, Dur: 0.4}, {Step: 24, Pitch: -14, Dur: 0.4}, {Step: 25, Pitch: -5, Dur: 0.4}, {Step: 26, Pitch: 3, Dur: 0.4}, {Step: 27, Pitch: 2, Dur: 0.4}, {Step: 28, Pitch: 3, Dur: 0.4}, {Step: 29, Pitch: -5, Dur: 0.4}, {Step: 30, Pitch: 3, Dur: 0.4}, {Step: 31, Pitch: -5, Dur: 0.4},
				}},
			},
		},

		// ── Cielito Lindo (Quirino Mendoza y Cortes, 1882) — mariachi trumpet ──
		// Transcribed VERBATIM from the John Chambers ABC score (abcnotation.com,
		// C major, 3/4), cross-checked against 4 sources, transposed +2 to the
		// canonical mariachi key of D major. "Ay, ay, ay, ay" = descending
		// F#5-E5-D5-B4 (the C-major E-D-C-A). Verse + refrain.
		{
			Stem: "cielito-trumpet", BPM: 165, Subdiv: 16, Bars: 24,
			Insts: []InstSpec{{ID: "trumpet", Name: "Trumpet", Volume: 0.85, ReverbSend: 0.14}},
			Rows: []RowSpec{{Inst: "trumpet", Hits: []Hit{
				{Step: 0, Pitch: 17, Dur: 1.00}, {Step: 4, Pitch: 17, Dur: 1.00}, {Step: 8, Pitch: 14, Dur: 2.00}, {Step: 16, Pitch: 16, Dur: 1.00}, {Step: 20, Pitch: 12, Dur: 1.00}, {Step: 24, Pitch: 17, Dur: 1.00}, {Step: 28, Pitch: 17, Dur: 1.00}, {Step: 32, Pitch: 14, Dur: 2.00}, {Step: 40, Pitch: 16, Dur: 1.00}, {Step: 44, Pitch: 12, Dur: 1.00}, {Step: 48, Pitch: 17, Dur: 1.00}, {Step: 52, Pitch: 17, Dur: 1.00}, {Step: 56, Pitch: 14, Dur: 2.00}, {Step: 64, Pitch: 16, Dur: 1.00}, {Step: 68, Pitch: 12, Dur: 1.00}, {Step: 72, Pitch: 10, Dur: 1.00}, {Step: 76, Pitch: 7, Dur: 5.00}, {Step: 96, Pitch: 16, Dur: 1.00}, {Step: 100, Pitch: 16, Dur: 1.00}, {Step: 104, Pitch: 16, Dur: 1.00}, {Step: 108, Pitch: 16, Dur: 1.00}, {Step: 112, Pitch: 14, Dur: 1.00}, {Step: 116, Pitch: 12, Dur: 1.00}, {Step: 120, Pitch: 10, Dur: 1.00}, {Step: 124, Pitch: 7, Dur: 1.00}, {Step: 128, Pitch: 7, Dur: 2.00}, {Step: 136, Pitch: 9, Dur: 1.00}, {Step: 140, Pitch: 10, Dur: 1.00}, {Step: 144, Pitch: 12, Dur: 1.00}, {Step: 148, Pitch: 12, Dur: 1.00}, {Step: 152, Pitch: 12, Dur: 2.00}, {Step: 160, Pitch: 12, Dur: 1.50}, {Step: 166, Pitch: 10, Dur: 0.50}, {Step: 168, Pitch: 9, Dur: 0.50}, {Step: 170, Pitch: 7, Dur: 0.50}, {Step: 172, Pitch: 5, Dur: 5.00}, {Step: 192, Pitch: 21, Dur: 3.00}, {Step: 204, Pitch: 19, Dur: 2.00}, {Step: 212, Pitch: 17, Dur: 1.00}, {Step: 216, Pitch: 14, Dur: 6.00}, {Step: 240, Pitch: 19, Dur: 3.00}, {Step: 252, Pitch: 19, Dur: 2.00}, {Step: 260, Pitch: 17, Dur: 0.50}, {Step: 262, Pitch: 17, Dur: 0.50}, {Step: 264, Pitch: 21, Dur: 1.00}, {Step: 268, Pitch: 17, Dur: 4.00}, {Step: 284, Pitch: 12, Dur: 1.00}, {Step: 288, Pitch: 14, Dur: 2.00}, {Step: 296, Pitch: 12, Dur: 1.00}, {Step: 300, Pitch: 14, Dur: 1.00}, {Step: 304, Pitch: 14, Dur: 1.00}, {Step: 308, Pitch: 12, Dur: 0.50}, {Step: 310, Pitch: 12, Dur: 0.50}, {Step: 312, Pitch: 22, Dur: 1.00}, {Step: 316, Pitch: 22, Dur: 1.00}, {Step: 320, Pitch: 19, Dur: 2.00}, {Step: 328, Pitch: 16, Dur: 1.00}, {Step: 332, Pitch: 12, Dur: 1.00}, {Step: 336, Pitch: 14, Dur: 1.00}, {Step: 340, Pitch: 14, Dur: 1.00}, {Step: 344, Pitch: 12, Dur: 2.00}, {Step: 352, Pitch: 10, Dur: 1.00}, {Step: 356, Pitch: 12, Dur: 1.00}, {Step: 360, Pitch: 9, Dur: 0.50}, {Step: 362, Pitch: 7, Dur: 0.50}, {Step: 364, Pitch: 5, Dur: 5.00},
			}}},
		},

		// ── Handel — Water Music, Bourrée (HWV 349) — french horn ──
		{
			Stem: "handel-water-horn", BPM: 120, Subdiv: 16, Bars: 12,
			Insts: []InstSpec{{ID: "french-horn", Name: "Horn", Volume: 0.85, ReverbSend: 0.14}},
			Rows: []RowSpec{{Inst: "french-horn", Hits: []Hit{
				{Step: 0, Pitch: 0, Dur: 1.00}, {Step: 4, Pitch: 5, Dur: 1.00}, {Step: 8, Pitch: 9, Dur: 1.00}, {Step: 12, Pitch: 7, Dur: 1.00}, {Step: 16, Pitch: 5, Dur: 1.00}, {Step: 20, Pitch: 12, Dur: 2.00}, {Step: 28, Pitch: 9, Dur: 2.00}, {Step: 36, Pitch: 14, Dur: 1.00}, {Step: 40, Pitch: 12, Dur: 1.00}, {Step: 44, Pitch: 10, Dur: 1.00}, {Step: 48, Pitch: 9, Dur: 1.00}, {Step: 52, Pitch: 10, Dur: 1.00}, {Step: 56, Pitch: 9, Dur: 2.00}, {Step: 64, Pitch: 12, Dur: 1.00}, {Step: 68, Pitch: 14, Dur: 1.00}, {Step: 72, Pitch: 10, Dur: 1.00}, {Step: 76, Pitch: 7, Dur: 1.00}, {Step: 80, Pitch: 9, Dur: 0.50}, {Step: 82, Pitch: 10, Dur: 0.50}, {Step: 84, Pitch: 12, Dur: 1.00}, {Step: 88, Pitch: 9, Dur: 1.00}, {Step: 92, Pitch: 5, Dur: 1.00}, {Step: 96, Pitch: 7, Dur: 0.50}, {Step: 98, Pitch: 9, Dur: 0.50}, {Step: 100, Pitch: 10, Dur: 1.00}, {Step: 104, Pitch: 9, Dur: 1.00}, {Step: 108, Pitch: 7, Dur: 1.00}, {Step: 112, Pitch: 5, Dur: 1.00}, {Step: 116, Pitch: 7, Dur: 0.50}, {Step: 118, Pitch: 5, Dur: 0.50}, {Step: 120, Pitch: 7, Dur: 0.50}, {Step: 122, Pitch: 9, Dur: 0.50}, {Step: 124, Pitch: 7, Dur: 1.00}, {Step: 128, Pitch: 9, Dur: 0.50}, {Step: 130, Pitch: 10, Dur: 0.50}, {Step: 132, Pitch: 12, Dur: 1.00}, {Step: 136, Pitch: 9, Dur: 0.50}, {Step: 138, Pitch: 10, Dur: 0.50}, {Step: 140, Pitch: 12, Dur: 1.00}, {Step: 144, Pitch: 10, Dur: 0.50}, {Step: 146, Pitch: 9, Dur: 0.50}, {Step: 148, Pitch: 10, Dur: 1.00}, {Step: 152, Pitch: 7, Dur: 0.50}, {Step: 154, Pitch: 9, Dur: 0.50}, {Step: 156, Pitch: 10, Dur: 1.00}, {Step: 160, Pitch: 9, Dur: 0.50}, {Step: 162, Pitch: 7, Dur: 0.50}, {Step: 164, Pitch: 9, Dur: 1.00}, {Step: 168, Pitch: 5, Dur: 1.00}, {Step: 172, Pitch: 7, Dur: 1.00}, {Step: 176, Pitch: 5, Dur: 1.00}, {Step: 180, Pitch: 5, Dur: 3.00},
			}}},
		},

		// ── Pachelbel — Canon in D — violin ──
		{
			Stem: "pachelbel-violin", BPM: 64, Subdiv: 16, Bars: 8,
			Insts: []InstSpec{{ID: "violin", Name: "Violin", Volume: 0.85, ReverbSend: 0.14}},
			Rows: []RowSpec{{Inst: "violin", Hits: []Hit{
				{Step: 0, Pitch: 21, Dur: 2.00}, {Step: 8, Pitch: 19, Dur: 2.00}, {Step: 16, Pitch: 17, Dur: 2.00}, {Step: 24, Pitch: 16, Dur: 2.00}, {Step: 32, Pitch: 14, Dur: 2.00}, {Step: 40, Pitch: 12, Dur: 2.00}, {Step: 48, Pitch: 14, Dur: 2.00}, {Step: 56, Pitch: 16, Dur: 2.00}, {Step: 64, Pitch: 17, Dur: 2.00}, {Step: 72, Pitch: 21, Dur: 2.00}, {Step: 80, Pitch: 24, Dur: 2.00}, {Step: 88, Pitch: 22, Dur: 2.00}, {Step: 96, Pitch: 21, Dur: 2.00}, {Step: 104, Pitch: 17, Dur: 2.00}, {Step: 112, Pitch: 21, Dur: 2.00}, {Step: 120, Pitch: 19, Dur: 2.00},
			}}},
		},

		// ── Marcello — Oboe Concerto in D minor, Adagio ──
		{
			Stem: "marcello-oboe", BPM: 54, Subdiv: 16, Bars: 6,
			Insts: []InstSpec{{ID: "oboe", Name: "Oboe", Volume: 0.85, ReverbSend: 0.14}},
			Rows: []RowSpec{{Inst: "oboe", Hits: []Hit{
				{Step: 0, Pitch: 12, Dur: 1.00}, {Step: 4, Pitch: 17, Dur: 2.00}, {Step: 12, Pitch: 15, Dur: 1.00}, {Step: 16, Pitch: 13, Dur: 1.00}, {Step: 20, Pitch: 12, Dur: 2.00}, {Step: 28, Pitch: 10, Dur: 1.00}, {Step: 32, Pitch: 8, Dur: 1.00}, {Step: 36, Pitch: 7, Dur: 2.00}, {Step: 44, Pitch: 5, Dur: 1.00}, {Step: 48, Pitch: 7, Dur: 1.00}, {Step: 52, Pitch: 8, Dur: 2.00}, {Step: 60, Pitch: 7, Dur: 1.00}, {Step: 64, Pitch: 5, Dur: 1.00}, {Step: 68, Pitch: 3, Dur: 2.00}, {Step: 76, Pitch: 5, Dur: 3.00},
			}}},
		},

		// ── Felt Piano — Bach prelude figure (soft) ──
		{
			Stem: "felt-prelude", BPM: 60, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "piano-felt", Name: "Felt Piano", Volume: 0.85, ReverbSend: 0.16},
			},
			Rows: []RowSpec{
				{Inst: "piano-felt", Hits: []Hit{
					{Step: 0, Pitch: 3, Dur: 0.55}, {Step: 1, Pitch: 7, Dur: 0.55}, {Step: 2, Pitch: 10, Dur: 0.55}, {Step: 3, Pitch: 15, Dur: 0.55}, {Step: 4, Pitch: 19, Dur: 0.55}, {Step: 5, Pitch: 10, Dur: 0.55}, {Step: 6, Pitch: 15, Dur: 0.55}, {Step: 7, Pitch: 19, Dur: 0.55}, {Step: 8, Pitch: 3, Dur: 0.55}, {Step: 9, Pitch: 7, Dur: 0.55}, {Step: 10, Pitch: 10, Dur: 0.55}, {Step: 11, Pitch: 15, Dur: 0.55}, {Step: 12, Pitch: 19, Dur: 0.55}, {Step: 13, Pitch: 10, Dur: 0.55}, {Step: 14, Pitch: 15, Dur: 0.55}, {Step: 15, Pitch: 19, Dur: 0.55}, {Step: 16, Pitch: 3, Dur: 0.55}, {Step: 17, Pitch: 5, Dur: 0.55}, {Step: 18, Pitch: 12, Dur: 0.55}, {Step: 19, Pitch: 17, Dur: 0.55}, {Step: 20, Pitch: 20, Dur: 0.55}, {Step: 21, Pitch: 12, Dur: 0.55}, {Step: 22, Pitch: 17, Dur: 0.55}, {Step: 23, Pitch: 20, Dur: 0.55}, {Step: 24, Pitch: 3, Dur: 0.55}, {Step: 25, Pitch: 5, Dur: 0.55}, {Step: 26, Pitch: 12, Dur: 0.55}, {Step: 27, Pitch: 17, Dur: 0.55}, {Step: 28, Pitch: 20, Dur: 0.55}, {Step: 29, Pitch: 12, Dur: 0.55}, {Step: 30, Pitch: 17, Dur: 0.55}, {Step: 31, Pitch: 20, Dur: 0.55},
				}},
			},
		},

		// ── Blues line — solo alto sax ──
		{
			Stem: "sax-blues", BPM: 96, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "sax", Name: "Sax", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "sax", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 0.5}, {Step: 2, Pitch: 3, Dur: 0.5}, {Step: 4, Pitch: 5, Dur: 0.5}, {Step: 6, Pitch: 6, Dur: 0.5}, {Step: 8, Pitch: 7, Dur: 0.5}, {Step: 10, Pitch: 5, Dur: 0.5}, {Step: 12, Pitch: 3, Dur: 0.5}, {Step: 14, Pitch: 0, Dur: 0.5}, {Step: 16, Pitch: 0, Dur: 0.5}, {Step: 18, Pitch: 3, Dur: 0.5}, {Step: 20, Pitch: 7, Dur: 0.5}, {Step: 22, Pitch: 10, Dur: 0.5}, {Step: 24, Pitch: 12, Dur: 0.5}, {Step: 26, Pitch: 7, Dur: 0.5}, {Step: 28, Pitch: 5, Dur: 0.5}, {Step: 30, Pitch: 3, Dur: 0.5},
				}},
			},
		},

		// ── Folk fingerpick — steel-string guitar ──
		{
			Stem: "steel-folk", BPM: 92, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "guitar-steel", Name: "Steel Guitar", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "guitar-steel", Hits: []Hit{
					{Step: 0, Pitch: -2, Dur: 0.6}, {Step: 2, Pitch: 5, Dur: 0.6}, {Step: 4, Pitch: 2, Dur: 0.6}, {Step: 6, Pitch: 5, Dur: 0.6}, {Step: 8, Pitch: -2, Dur: 0.6}, {Step: 10, Pitch: 5, Dur: 0.6}, {Step: 12, Pitch: 2, Dur: 0.6}, {Step: 14, Pitch: 5, Dur: 0.6}, {Step: 16, Pitch: 0, Dur: 0.6}, {Step: 18, Pitch: 7, Dur: 0.6}, {Step: 20, Pitch: 5, Dur: 0.6}, {Step: 22, Pitch: 7, Dur: 0.6}, {Step: 24, Pitch: 0, Dur: 0.6}, {Step: 26, Pitch: 7, Dur: 0.6}, {Step: 28, Pitch: 5, Dur: 0.6}, {Step: 30, Pitch: 7, Dur: 0.6},
				}},
			},
		},

		// ── Pentatonic riff — electric guitar ──
		{
			Stem: "electric-riff", BPM: 120, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "guitar-electric", Name: "Electric Guitar", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "guitar-electric", Hits: []Hit{
					{Step: 0, Pitch: -5, Dur: 0.4}, {Step: 2, Pitch: -2, Dur: 0.4}, {Step: 4, Pitch: 0, Dur: 0.4}, {Step: 6, Pitch: 2, Dur: 0.4}, {Step: 8, Pitch: 0, Dur: 0.4}, {Step: 10, Pitch: -2, Dur: 0.4}, {Step: 12, Pitch: -5, Dur: 0.4}, {Step: 14, Pitch: -2, Dur: 0.4}, {Step: 16, Pitch: -5, Dur: 0.4}, {Step: 18, Pitch: -2, Dur: 0.4}, {Step: 20, Pitch: 0, Dur: 0.4}, {Step: 22, Pitch: 3, Dur: 0.4}, {Step: 24, Pitch: 2, Dur: 0.4}, {Step: 26, Pitch: 0, Dur: 0.4}, {Step: 28, Pitch: -2, Dur: 0.4}, {Step: 30, Pitch: -5, Dur: 0.4},
				}},
			},
		},

		// ── Funk riff — guitar ──
		{
			Stem: "clav-funk", BPM: 104, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "guitar-electric", Name: "Guitar", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "guitar-electric", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 0.18}, {Step: 3, Pitch: 0, Dur: 0.18}, {Step: 4, Pitch: 3, Dur: 0.18}, {Step: 6, Pitch: 0, Dur: 0.18}, {Step: 10, Pitch: 5, Dur: 0.18}, {Step: 11, Pitch: 0, Dur: 0.18}, {Step: 14, Pitch: 3, Dur: 0.18}, {Step: 16, Pitch: 0, Dur: 0.18}, {Step: 19, Pitch: 0, Dur: 0.18}, {Step: 20, Pitch: 7, Dur: 0.18}, {Step: 22, Pitch: 5, Dur: 0.18}, {Step: 26, Pitch: 3, Dur: 0.18}, {Step: 27, Pitch: 0, Dur: 0.18}, {Step: 30, Pitch: 7, Dur: 0.18},
				}},
			},
		},

		// ── Bass groove — synth bass ──
		{
			Stem: "bass-groove", BPM: 110, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "bass-guitar", Name: "Synth Bass", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -12, Dur: 0.4}, {Step: 4, Pitch: -12, Dur: 0.4}, {Step: 6, Pitch: -12, Dur: 0.25}, {Step: 8, Pitch: -17, Dur: 0.4}, {Step: 12, Pitch: -14, Dur: 0.25}, {Step: 14, Pitch: -12, Dur: 0.25}, {Step: 16, Pitch: -12, Dur: 0.4}, {Step: 20, Pitch: -12, Dur: 0.4}, {Step: 22, Pitch: -10, Dur: 0.25}, {Step: 24, Pitch: -17, Dur: 0.4}, {Step: 28, Pitch: -14, Dur: 0.4}, {Step: 30, Pitch: -12, Dur: 0.25},
				}},
			},
		},

		// ── Tumbao pattern — congas ──
		{
			Stem: "conga-tumbao", BPM: 100, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "conga", Name: "Congas", Volume: 0.85, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				{Inst: "conga", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 0.3}, {Step: 3, Pitch: 0, Dur: 0.3}, {Step: 4, Pitch: 5, Dur: 0.3}, {Step: 6, Pitch: 5, Dur: 0.3}, {Step: 8, Pitch: 0, Dur: 0.3}, {Step: 11, Pitch: 0, Dur: 0.3}, {Step: 12, Pitch: 5, Dur: 0.3}, {Step: 14, Pitch: 0, Dur: 0.3}, {Step: 16, Pitch: 0, Dur: 0.3}, {Step: 19, Pitch: 0, Dur: 0.3}, {Step: 20, Pitch: 5, Dur: 0.3}, {Step: 22, Pitch: 5, Dur: 0.3}, {Step: 24, Pitch: 0, Dur: 0.3}, {Step: 27, Pitch: 0, Dur: 0.3}, {Step: 28, Pitch: 5, Dur: 0.3}, {Step: 30, Pitch: 0, Dur: 0.3},
				}},
			},
		},
		// ── Salsa — Vivir Mi Vida (Marc Anthony) ──
		// C minor, 4-bar montuno loop Cm–Ab–Eb–Bb. Piano montuno stabs +
		// anticipated tumbao bass + conga open/slap + mambo trumpet stabs.
		{
			Stem: "salsa-vivir", BPM: 105, Subdiv: 16, Bars: 4,
			Insts: []InstSpec{
				{ID: "piano-grand", Name: "Montuno", Volume: 0.8, ReverbSend: 0.1},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "conga-open", Name: "Conga Open", Volume: 0.8, ReverbSend: 0.1},
				{ID: "conga", Name: "Conga Slap", Volume: 0.8, ReverbSend: 0.1},
				{ID: "trumpet", Name: "Trumpet", Volume: 0.8, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				// Piano montuno — short stabs (Cm–Ab–Eb–Bb).
				{Inst: "piano-grand", Hits: []Hit{
					{Step: 2, Pitch: 10, Dur: 0.4}, {Step: 3, Pitch: 6, Dur: 0.4}, {Step: 6, Pitch: 3, Dur: 0.4}, {Step: 7, Pitch: 6, Dur: 0.4}, {Step: 10, Pitch: 10, Dur: 0.4}, {Step: 11, Pitch: 15, Dur: 0.4}, {Step: 14, Pitch: 10, Dur: 0.4}, {Step: 16, Pitch: 6, Dur: 0.4}, {Step: 19, Pitch: 3, Dur: 0.4}, {Step: 22, Pitch: 11, Dur: 0.4}, {Step: 23, Pitch: 15, Dur: 0.4}, {Step: 26, Pitch: 18, Dur: 0.4}, {Step: 27, Pitch: 15, Dur: 0.4}, {Step: 30, Pitch: 11, Dur: 0.4}, {Step: 34, Pitch: 13, Dur: 0.4}, {Step: 35, Pitch: 10, Dur: 0.4}, {Step: 38, Pitch: 6, Dur: 0.4}, {Step: 39, Pitch: 10, Dur: 0.4}, {Step: 42, Pitch: 13, Dur: 0.4}, {Step: 43, Pitch: 18, Dur: 0.4}, {Step: 46, Pitch: 13, Dur: 0.4}, {Step: 48, Pitch: 8, Dur: 0.4}, {Step: 51, Pitch: 5, Dur: 0.4}, {Step: 54, Pitch: 13, Dur: 0.4}, {Step: 55, Pitch: 17, Dur: 0.4}, {Step: 58, Pitch: 20, Dur: 0.4}, {Step: 59, Pitch: 17, Dur: 0.4}, {Step: 62, Pitch: 13, Dur: 0.4},
				}},
				// Bass — anticipated tumbao (5th, root, root per bar).
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 6, Pitch: -14, Dur: 1.0}, {Step: 12, Pitch: -21, Dur: 1.0}, {Step: 14, Pitch: -21, Dur: 1.0}, {Step: 22, Pitch: -18, Dur: 1.0}, {Step: 28, Pitch: -25, Dur: 1.0}, {Step: 30, Pitch: -25, Dur: 1.0}, {Step: 38, Pitch: -11, Dur: 1.0}, {Step: 44, Pitch: -18, Dur: 1.0}, {Step: 46, Pitch: -18, Dur: 1.0}, {Step: 54, Pitch: -16, Dur: 1.0}, {Step: 60, Pitch: -23, Dur: 1.0}, {Step: 62, Pitch: -23, Dur: 1.0},
				}},
				// Conga open tones (tumbao accents).
				{Inst: "conga-open", Hits: []Hit{
					{Step: 12, Pitch: 0, Dur: 0.4}, {Step: 14, Pitch: 0, Dur: 0.4}, {Step: 28, Pitch: 0, Dur: 0.4}, {Step: 30, Pitch: 0, Dur: 0.4}, {Step: 44, Pitch: 0, Dur: 0.4}, {Step: 46, Pitch: 0, Dur: 0.4}, {Step: 60, Pitch: 0, Dur: 0.4}, {Step: 62, Pitch: 0, Dur: 0.4},
				}},
				// Conga slap — beat-2 crack.
				{Inst: "conga", Hits: []Hit{
					{Step: 4, Pitch: 0, Dur: 0.22}, {Step: 20, Pitch: 0, Dur: 0.22}, {Step: 36, Pitch: 0, Dur: 0.22}, {Step: 52, Pitch: 0, Dur: 0.22},
				}},
				// Trumpet — mambo stabs (chord 5th up high).
				{Inst: "trumpet", Hits: []Hit{
					// The recognizable sung HOOK "Voy a reir, voy a bailar / voy a gozar,
					// vivir mi vida la la la la" (Marc Anthony), on the lead trumpet — the
					// C4<->Eb4 "la la la la" oscillation + the C4->Bb3->Ab3 "gozar" descent.
					// Cm; contour sourced from recorder-tutorial solfege (notaspianoflauta
					// + docentestic agree), placed on the salsa grid with anticipation.
					{Step: 0, Pitch: 3, Dur: 0.50, Vol: 0.85}, {Step: 2, Pitch: 5, Dur: 0.50}, {Step: 4, Pitch: 6, Dur: 1.00}, {Step: 6, Pitch: 6, Dur: 0.50}, {Step: 7, Pitch: 3, Dur: 0.50}, {Step: 8, Pitch: 6, Dur: 0.50}, {Step: 10, Pitch: 6, Dur: 1.00}, {Step: 16, Pitch: 6, Dur: 0.50}, {Step: 18, Pitch: 6, Dur: 0.50}, {Step: 20, Pitch: 3, Dur: 0.50}, {Step: 22, Pitch: 6, Dur: 0.50}, {Step: 24, Pitch: 6, Dur: 0.50}, {Step: 26, Pitch: 3, Dur: 0.50}, {Step: 28, Pitch: 6, Dur: 0.50}, {Step: 30, Pitch: 5, Dur: 1.00}, {Step: 32, Pitch: 3, Dur: 0.50}, {Step: 34, Pitch: 5, Dur: 0.50}, {Step: 36, Pitch: 6, Dur: 1.00}, {Step: 38, Pitch: 5, Dur: 0.50}, {Step: 39, Pitch: 3, Dur: 0.50}, {Step: 40, Pitch: 3, Dur: 0.50}, {Step: 42, Pitch: 1, Dur: 0.50}, {Step: 44, Pitch: -1, Dur: 1.00}, {Step: 48, Pitch: 6, Dur: 0.50}, {Step: 50, Pitch: 6, Dur: 0.50}, {Step: 52, Pitch: 3, Dur: 0.50}, {Step: 54, Pitch: 6, Dur: 0.50}, {Step: 56, Pitch: 6, Dur: 0.50}, {Step: 58, Pitch: 3, Dur: 0.50}, {Step: 60, Pitch: 6, Dur: 0.50}, {Step: 62, Pitch: 8, Dur: 1.00},
				}},
			},
		},

		// ── Bachata — Obsesión (Aventura) ──
		// C# minor, 2-bar loop C#m–G#m. Signature requinto arpeggio +
		// bass + steady güira 16ths + bongó hi/hembra + vocal-hook lead.
		{
			Stem: "bachata-obsesion", BPM: 134, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "guitar-nylon", Name: "Requinto", Volume: 0.8, ReverbSend: 0.12},
				{ID: "bass-guitar", Name: "Bass", Volume: 0.85},
				{ID: "hihat", Name: "Güira", Volume: 0.5},
				{ID: "conga", Name: "Bongó Hi", Volume: 0.8, ReverbSend: 0.1},
				{ID: "conga-tumba", Name: "Bongó Hembra", Volume: 0.8, ReverbSend: 0.1},
				{ID: "sax", Name: "Vocal Hook", Volume: 0.75, ReverbSend: 0.12},
			},
			Rows: []RowSpec{
				// Requinto — the signature lead arpeggio (C#m → G#m).
				{Inst: "guitar-nylon", Hits: []Hit{
					{Step: 0, Pitch: 4, Dur: 0.25}, {Step: 1, Pitch: 7, Dur: 0.25}, {Step: 2, Pitch: 11, Dur: 0.25}, {Step: 3, Pitch: 16, Dur: 0.25}, {Step: 4, Pitch: 11, Dur: 0.25}, {Step: 5, Pitch: 7, Dur: 0.25}, {Step: 6, Pitch: 4, Dur: 0.5}, {Step: 8, Pitch: 7, Dur: 0.25}, {Step: 9, Pitch: 11, Dur: 0.25}, {Step: 10, Pitch: 16, Dur: 0.5}, {Step: 12, Pitch: 11, Dur: 0.25}, {Step: 13, Pitch: 7, Dur: 0.25}, {Step: 14, Pitch: 4, Dur: 0.5}, {Step: 16, Pitch: -1, Dur: 0.25}, {Step: 17, Pitch: 6, Dur: 0.25}, {Step: 18, Pitch: 11, Dur: 0.25}, {Step: 19, Pitch: 14, Dur: 0.25}, {Step: 20, Pitch: 11, Dur: 0.25}, {Step: 21, Pitch: 6, Dur: 0.25}, {Step: 22, Pitch: -1, Dur: 0.5}, {Step: 24, Pitch: 6, Dur: 0.25}, {Step: 25, Pitch: 11, Dur: 0.25}, {Step: 26, Pitch: 14, Dur: 0.5}, {Step: 28, Pitch: 9, Dur: 0.25}, {Step: 29, Pitch: 6, Dur: 0.25}, {Step: 30, Pitch: 2, Dur: 0.5},
				}},
				// Bass — root/5th (C#m → G#m).
				{Inst: "bass-guitar", Hits: []Hit{
					{Step: 0, Pitch: -20, Dur: 0.5}, {Step: 4, Pitch: -20, Dur: 0.9}, {Step: 6, Pitch: -13, Dur: 0.9}, {Step: 8, Pitch: -20, Dur: 0.5}, {Step: 11, Pitch: -13, Dur: 0.9}, {Step: 14, Pitch: -10, Dur: 0.5}, {Step: 16, Pitch: -13, Dur: 0.5}, {Step: 20, Pitch: -13, Dur: 0.9}, {Step: 22, Pitch: -18, Dur: 0.9}, {Step: 24, Pitch: -13, Dur: 0.5}, {Step: 27, Pitch: -18, Dur: 0.9}, {Step: 30, Pitch: -12, Dur: 0.5},
				}},
				// Güira — steady 16ths (light).
				{Inst: "hihat", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 0.2}, {Step: 1, Pitch: 0, Dur: 0.2}, {Step: 2, Pitch: 0, Dur: 0.2}, {Step: 3, Pitch: 0, Dur: 0.2}, {Step: 4, Pitch: 0, Dur: 0.2}, {Step: 5, Pitch: 0, Dur: 0.2}, {Step: 6, Pitch: 0, Dur: 0.2}, {Step: 7, Pitch: 0, Dur: 0.2}, {Step: 8, Pitch: 0, Dur: 0.2}, {Step: 9, Pitch: 0, Dur: 0.2}, {Step: 10, Pitch: 0, Dur: 0.2}, {Step: 11, Pitch: 0, Dur: 0.2}, {Step: 12, Pitch: 0, Dur: 0.2}, {Step: 13, Pitch: 0, Dur: 0.2}, {Step: 14, Pitch: 0, Dur: 0.2}, {Step: 15, Pitch: 0, Dur: 0.2}, {Step: 16, Pitch: 0, Dur: 0.2}, {Step: 17, Pitch: 0, Dur: 0.2}, {Step: 18, Pitch: 0, Dur: 0.2}, {Step: 19, Pitch: 0, Dur: 0.2}, {Step: 20, Pitch: 0, Dur: 0.2}, {Step: 21, Pitch: 0, Dur: 0.2}, {Step: 22, Pitch: 0, Dur: 0.2}, {Step: 23, Pitch: 0, Dur: 0.2}, {Step: 24, Pitch: 0, Dur: 0.2}, {Step: 25, Pitch: 0, Dur: 0.2}, {Step: 26, Pitch: 0, Dur: 0.2}, {Step: 27, Pitch: 0, Dur: 0.2}, {Step: 28, Pitch: 0, Dur: 0.2}, {Step: 29, Pitch: 0, Dur: 0.2}, {Step: 30, Pitch: 0, Dur: 0.2}, {Step: 31, Pitch: 0, Dur: 0.2},
				}},
				// Bongó hi (macho).
				{Inst: "conga", Hits: []Hit{
					{Step: 0, Pitch: 0, Dur: 0.22}, {Step: 2, Pitch: 0, Dur: 0.22}, {Step: 4, Pitch: 0, Dur: 0.22}, {Step: 6, Pitch: 0, Dur: 0.22}, {Step: 8, Pitch: 0, Dur: 0.22}, {Step: 10, Pitch: 0, Dur: 0.22}, {Step: 16, Pitch: 0, Dur: 0.22}, {Step: 18, Pitch: 0, Dur: 0.22}, {Step: 20, Pitch: 0, Dur: 0.22}, {Step: 22, Pitch: 0, Dur: 0.22}, {Step: 24, Pitch: 0, Dur: 0.22}, {Step: 26, Pitch: 0, Dur: 0.22},
				}},
				// Bongó hembra — low on beat 4 (the "macho" answer).
				{Inst: "conga-tumba", Hits: []Hit{
					{Step: 12, Pitch: 0, Dur: 0.4}, {Step: 28, Pitch: 0, Dur: 0.4},
				}},
				// Vocal hook — lead line (sax stand-in).
				{Inst: "sax", Hits: []Hit{
					{Step: 0, Pitch: 11, Dur: 0.5}, {Step: 4, Pitch: 12, Dur: 0.5}, {Step: 8, Pitch: 11, Dur: 0.5}, {Step: 12, Pitch: 7, Dur: 1.0}, {Step: 16, Pitch: 9, Dur: 0.5}, {Step: 20, Pitch: 11, Dur: 0.5}, {Step: 24, Pitch: 9, Dur: 0.5}, {Step: 28, Pitch: 6, Dur: 1.0},
				}},
			},
		},
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
