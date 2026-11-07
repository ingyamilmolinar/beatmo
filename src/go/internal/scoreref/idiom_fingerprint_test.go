package scoreref

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// Beatmo writes its groove scores, so the score is not an outside oracle: each idiom's DEFINING instrument must strike
// exactly the textbook pattern the catalogue cites, so a clave on the wrong sixteenth fails here. Positions on a
// perQuarter grid from the first downbeat; only full-strength hits count (a ghost note is decoration).
type fingerprint struct {
	score      string
	gmKey      int
	perQuarter int
	want       []int
	why        string
}

var idiomFingerprints = []fingerprint{
	// ── Traditional idioms ──
	{"groove-salsa", 75, 4, []int{2, 4, 8, 11, 14},
		"son clave 2-3: the two side, then the three side (cut-time eighths written as sixteenths)"},
	{"groove-reggaeton", 40, 4, []int{3, 6, 11, 14},
		"tresillo/habanera on the snare: the 3+3+2 cell's second and third strokes in each half bar"},
	{"groove-reggaeton", 36, 4, []int{0, 4, 8, 12}, "kick on every beat"},
	{"groove-techno", 36, 4, []int{0, 4, 8, 12}, "four on the floor"},
	{"groove-jazz-swing", 51, 4, []int{0, 4, 6, 8, 12, 14, 16, 20, 22, 24, 28, 30},
		"spang-a-lang: 1, 2, 2&, 3, 4, 4& in swung eighths"},
	{"groove-jazz-swing", 44, 4, []int{4, 12, 20, 28}, "hi-hat foot on two and four"},
	{"groove-merengue", 73, 4, []int{0, 4, 8, 12}, "güira accents on the beats"},
	{"groove-cumbia", 64, 4, []int{2, 6, 10, 14}, "llamador on the offbeats"},
	{"groove-vallenato", 73, 4, []int{0, 4, 8, 12}, "guacharaca accents on the beats"},
	{"groove-trap", 38, 8, []int{16, 48}, "half-time snare on beat three"},

	// ── Famous breaks, typed from the drum-patterns.com step grids the
	// catalogue cites (1-based steps there, 0-based here) ──
	{"groove-funky-drummer", 36, 4, []int{0, 2, 10, 13, 16, 18, 26, 29}, "Funky Drummer kick: 1, 1&, 3&, 4e"},
	{"groove-funky-drummer", 38, 4, []int{4, 11, 12, 15, 20, 27, 28, 31},
		"Funky Drummer snare at full strength (the a-of-2 and e-of-3 strokes are ghosts)"},
	{"groove-impeach-the-president", 36, 4, []int{0, 7, 8, 14}, "Impeach the President kick: 1, 2a, 3, 4&"},
	{"groove-impeach-the-president", 38, 4, []int{4, 12}, "backbeat"},
	{"groove-superstition", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60}, "Superstition: kick on every beat"},
	{"groove-amen-break", 36, 4, []int{0, 2, 10, 11, 16, 18, 26, 27, 32, 34, 42, 50, 51, 58},
		"Amen break kick over its four bars"},
	{"groove-amen-break", 38, 4, []int{4, 7, 9, 12, 15, 20, 23, 25, 28, 31, 36, 39, 41, 46, 49, 52, 55, 57, 62},
		"Amen break snare: backbeat with the a-of-2 / e-of-3 strokes, displaced in bars 3-4"},
	{"groove-planet-rock", 36, 4, []int{0, 6}, "Planet Rock kick: 1 and 2&"},
	{"groove-planet-rock", 56, 4, []int{0, 2, 4, 6, 7, 9, 11, 12, 14}, "Planet Rock cowbell"},
	{"groove-blue-monday", 36, 4, []int{0, 4, 8, 9, 10, 11, 12, 13, 14, 15, 16, 20, 24, 28},
		"Blue Monday: the sixteenth-note kick run, then four on the floor"},
	{"groove-da-funk", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44}, "Da Funk: four on the floor, dropping out in bar 4"},
	{"groove-uk-garage", 36, 4, []int{0, 10, 16, 26}, "2-step kick: 1 and 3& — no kick on beat three"},
	{"groove-we-will-rock-you", 36, 4, []int{0, 2, 8, 10}, "We Will Rock You: stomp-stomp"},
	{"groove-we-will-rock-you", 70, 4, []int{4, 12}, "We Will Rock You: clap"},
	{"groove-rosanna-shuffle", 36, 4, []int{0, 6, 12, 18, 24, 30}, "Rosanna half-time shuffle kick"},
	{"groove-in-the-air-tonight", 36, 4, []int{10, 12, 16, 20, 24, 26, 28, 30}, "In the Air Tonight kick"},
	{"groove-levee-breaks", 36, 4, []int{0, 1, 7, 10, 11}, "When the Levee Breaks kick: 1, 1e, 2a, 3&, 3a"},
	{"groove-levee-breaks", 38, 4, []int{4, 12}, "backbeat"},
	{"groove-baile-funk", 36, 4, []int{0, 10}, "baile funk kick"},
	{"groove-baile-funk", 39, 4, []int{3, 6, 12}, "baile funk claps"},
	{"groove-dancehall", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28}, "dancehall: kick on every beat"},
	{"groove-dancehall", 39, 4, []int{6, 22, 26}, "dancehall clap pushes"},

	// Kick patterns of the canon beats, from the drum-patterns.com grids the catalogue cites.
	{"groove-hook-and-sling", 36, 4, []int{0, 2, 9, 13, 14, 29, 32, 33, 44, 45, 47, 48, 50, 53, 59, 61, 62}, "Hook and Sling kick (Eddie Bo)"},
	{"groove-look-ka-py-py", 36, 4, []int{0, 3, 5, 10, 13, 14, 16, 19, 21, 23, 24, 26, 29, 30}, "Look-Ka Py Py kick (The Meters)"},
	{"groove-synthetic-substitution", 36, 4, []int{0, 2, 7, 9, 10, 11, 15, 16, 18, 23, 25, 26, 27, 31}, "Synthetic Substitution kick (Melvin Bliss)"},
	{"groove-haitian-divorce", 36, 4, []int{2, 4, 10, 12, 18, 20, 26, 28}, "Haitian Divorce kick (Steely Dan)"},
	{"groove-palm-grease", 36, 4, []int{0, 8, 15, 18}, "Palm Grease kick (Herbie Hancock)"},
	{"groove-expensive-shit", 36, 4, []int{3, 6, 14, 19, 22, 30}, "Expensive Shit kick (Fela Kuti)"},
	{"groove-immigrant-song", 36, 4, []int{0, 2, 3, 5, 8, 10, 11, 13, 16, 18, 19, 21, 24, 26, 27, 29, 32, 35, 37, 40, 42, 43, 45, 48, 50, 51, 53, 56, 58, 59, 61, 64, 66, 67, 72, 80, 82, 83, 85, 88, 95, 96, 98, 99, 101, 104, 106, 107, 109, 112, 114, 115, 120}, "Immigrant Song kick (Led Zeppelin)"},
	{"groove-hit-me-with-your-rhythm-stick", 36, 4, []int{7, 8, 16, 23, 24, 32, 39, 40, 48, 55, 56, 64, 66, 72, 74, 79, 88, 96, 104, 112, 119, 120}, "Hit Me with Your Rhythm Stick kick (Ian Dury & the Blockheads)"},
	{"groove-dirty-love", 36, 4, []int{0, 2, 7, 8, 10, 16, 18, 23, 24, 26, 32, 34, 39, 40, 42, 48, 50, 55, 56, 58, 64, 66, 71, 72, 74, 80, 82, 87, 88, 90, 96, 98, 103, 104, 106, 112, 114, 119, 120, 122}, "Dirty Love kick (Frank Zappa)"},
	{"groove-rock-lobster", 36, 4, []int{0, 8, 14, 16, 22, 26, 32, 38, 42, 48, 54, 58}, "Rock Lobster kick (The B-52's)"},
	{"groove-peek-a-boo", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96, 100, 104, 108}, "Peek-a-Boo! kick (Devo)"},
	{"groove-scentless-apprentice", 36, 4, []int{0, 2, 5, 8, 9, 10, 11, 13, 15, 16, 18, 21, 24, 25, 26, 27, 32, 34, 37, 40, 41, 42, 43, 45, 48, 50, 53, 56, 57, 58, 59, 61}, "Scentless Apprentice kick (Nirvana)"},
	{"groove-iron-man", 36, 4, []int{0, 8, 10, 16, 18, 19, 21, 24, 26, 32, 34, 35, 37, 40, 42, 48, 50, 51, 53, 57, 59, 61, 64, 66, 72, 75, 80, 82, 88, 90, 92, 94, 96, 98, 112, 114, 120, 123, 125}, "Iron Man kick (Black Sabbath)"},
	{"groove-dig-it", 36, 4, []int{0, 10, 11, 13, 16, 23, 25, 26}, "Dig It kick (Skinny Puppy)"},
	{"groove-achtung", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 32, 36, 40, 44, 64, 68, 72, 88, 89, 90, 91, 92, 93, 94, 95, 96, 100, 104, 108, 112, 116, 120, 124}, "Achtung kick (Laibach)"},
	{"groove-down-in-it", 36, 4, []int{0, 1, 2, 6, 7, 9, 10, 16, 17, 18, 22, 23, 24, 25, 26, 32, 35, 39, 42, 48, 51, 55, 57, 58, 61}, "Down in It kick (Nine Inch Nails)"},
	{"groove-being-boiled", 36, 4, []int{0, 7, 10}, "Being Boiled kick (The Human League)"},
	{"groove-1000-knives", 36, 4, []int{0, 1, 8, 9}, "1000 Knives kick (Yellow Magic Orchestra)"},
	{"groove-raised-by-snakes", 36, 4, []int{0, 4, 8, 10, 12, 16, 20, 24, 26, 28, 32, 36, 40, 42, 44, 48, 52, 56, 58, 60, 64, 68, 72, 74, 76, 80, 84, 88, 90, 92, 96, 100, 104, 106, 108, 112, 116, 120, 122, 124}, "Raised by Snakes kick (Telex)"},
	{"groove-problemes-damour", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96, 100, 104, 108, 112, 116, 120, 124}, "Problèmes d'amour kick (Alexander Robotnick)"},
	{"groove-e-mc2", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96, 100, 104, 108, 112, 116, 120, 124}, "E=MC² kick (Giorgio Moroder)"},
	{"groove-needle-to-the-groove", 36, 4, []int{0, 3, 6, 8}, "Needle to the Groove kick (Mantronix)"},
	{"groove-egypt-egypt", 36, 4, []int{0, 6}, "Egypt, Egypt kick (Egyptian Lover)"},
	{"groove-buffalo-gals", 36, 4, []int{0, 1, 6, 7, 15, 16, 17, 22, 23, 31, 32, 40, 46, 47, 54, 55, 58, 63, 64, 71, 72, 74, 79, 80, 81, 86, 87, 90, 95, 96, 97, 102, 103, 111, 112, 119, 120, 122, 127}, "Buffalo Gals kick (Malcolm McLaren)"},
	{"groove-clear", 36, 4, []int{0, 3, 6, 10, 16, 22, 26, 32, 35, 38, 42, 48, 54, 58, 64, 67, 70, 74, 80, 86, 90, 96, 99, 102, 106, 112, 118, 122}, "Clear kick (Cybotron)"},
	{"groove-you-got-me", 36, 4, []int{0, 9, 12, 13, 14, 15, 16, 32, 41, 44, 45, 46, 48, 49, 54, 57, 65, 69, 70, 74, 75, 77, 80, 89, 92, 96, 105, 109, 111, 121, 124, 125, 126, 127}, "You Got Me kick (The Roots)"},
	{"groove-on-and-on", 36, 4, []int{18, 34, 38, 42, 46, 50, 54, 58, 62, 66, 70, 74, 78, 82, 86, 90, 94, 98, 102, 106, 110, 114}, "On and On kick (Jesse Saunders)"},
	{"groove-can-you-feel-it", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 31, 32, 36, 40, 44, 48, 52, 56, 60, 63, 64, 68, 72, 76, 80, 84, 88, 92, 95, 96, 100, 104, 108, 112, 116, 120, 124, 127}, "Can You Feel It kick (Mr. Fingers)"},
	{"groove-no-way-back", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96, 100, 104, 108, 112, 116, 120, 124}, "No Way Back kick (Adonis)"},
	{"groove-french-kiss", 36, 4, []int{0, 4, 8, 12}, "French Kiss kick (Lil Louis)"},
	{"groove-dominatrix-sleeps-tonight", 36, 4, []int{0, 3, 6, 7, 16, 19, 22, 23, 32, 34, 35, 37, 38, 39, 40, 41, 48, 50, 51, 53, 55, 57, 64, 67, 70, 71, 80, 82, 83, 85, 86, 87, 88, 89, 90, 91, 96, 99, 103, 104, 105, 112, 115, 119}, "The Dominatrix Sleeps Tonight kick (The Dominatrix)"},
	{"groove-voodoo-ray", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 49, 50, 51, 52, 55, 56, 57, 60, 61, 62, 63}, "Voodoo Ray kick (A Guy Called Gerald)"},
	{"groove-north-american-scum", 36, 4, []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 40, 48, 52, 56, 60}, "North American Scum kick (LCD Soundsystem)"},
	{"groove-flim", 36, 4, []int{0, 3, 6, 10, 11, 13, 15, 16, 18, 22, 26, 27, 29, 31, 32, 34, 35, 38, 42, 43, 47, 48, 51, 58, 59, 61, 63, 64, 70, 75, 77, 80, 82, 83, 86, 96, 98, 102, 109, 111, 112, 114, 115, 118, 122, 123, 125, 127}, "Flim kick (Aphex Twin)"},
	{"groove-xtal", 36, 4, []int{0, 3, 6, 11, 14, 16, 19, 22, 27, 30}, "Xtal kick (Aphex Twin)"},
	{"groove-idioteque", 36, 4, []int{0, 10, 14, 16, 21, 24, 34, 38, 40, 45}, "Idioteque kick (Radiohead)"},
	{"groove-when-i-rule-the-world", 36, 4, []int{0, 3, 4, 6, 8, 10, 12, 32, 33, 35, 36, 38, 40}, "When I Rule the World kick (LIZ (prod. SOPHIE))"},
}

// fullStrength separates the idiom from its ornaments: notes at or above the
// prevailing mezzo-forte. Ghost notes are written at 40-55% of MIDI 90.
const fullStrength = 0.6

func TestGrooveScoresMatchTheirIdiom(t *testing.T) {
	covered := map[string]bool{}
	for _, fp := range idiomFingerprints {
		covered[fp.score] = true
		sc, err := ParseScore(fp.score)
		if err != nil {
			t.Errorf("%s: %v", fp.score, err)
			continue
		}
		var got []int
		for _, p := range sc.Parts {
			for _, n := range p.Notes {
				if !n.Unpitched || n.PercKey != fp.gmKey || n.Velocity < fullStrength {
					continue
				}
				pos := float64(n.OnsetDiv) / float64(p.Divisions) * float64(fp.perQuarter)
				if pos != math.Trunc(pos) {
					t.Errorf("%s key %d: onset %v is off the grid", fp.score, fp.gmKey, pos)
					continue
				}
				got = append(got, int(pos))
			}
		}
		sort.Ints(got)
		if fmt.Sprint(got) != fmt.Sprint(fp.want) {
			t.Errorf("%s GM key %d: strikes %v, the idiom is %v (%s)", fp.score, fp.gmKey, got, fp.want, fp.why)
		}
	}
	for _, s := range cat(t).Scores {
		if s.IsGroove() && !covered[s.ID] {
			t.Errorf("groove %q has no idiom fingerprint — add the pattern it claims to encode", s.ID)
		}
	}
}

// Swing is part of the idiom, and a lost <swing> would straighten a template with no other test noticing.
func TestGrooveSwingIsNotated(t *testing.T) {
	want := map[string][2]float64{ // ratio, unit in quarters
		"groove-jazz-swing":             {2, 0.5},
		"groove-funky-drummer":          {13.0 / 11, 0.25},
		"groove-superstition":           {5.0 / 3, 0.25},
		"groove-blue-monday":            {13.0 / 11, 0.25},
		"groove-rosanna-shuffle":        {2, 0.25},
		"groove-hook-and-sling":         {13.0 / 11, 0.25},
		"groove-look-ka-py-py":          {13.0 / 11, 0.25},
		"groove-synthetic-substitution": {13.0 / 11, 0.25},
		"groove-expensive-shit":         {13.0 / 11, 0.25},
		"groove-rock-lobster":           {2.0 / 1, 0.25},
		"groove-peek-a-boo":             {13.0 / 11, 0.25},
		"groove-achtung":                {13.0 / 11, 0.25},
		"groove-raised-by-snakes":       {13.0 / 11, 0.25},
		"groove-problemes-damour":       {7.0 / 5, 0.25},
		"groove-clear":                  {7.0 / 5, 0.25},
		"groove-on-and-on":              {13.0 / 11, 0.25},
		"groove-can-you-feel-it":        {13.0 / 11, 0.25},
		"groove-voodoo-ray":             {13.0 / 11, 0.25},
	}
	for _, s := range cat(t).Scores {
		if !s.IsGroove() {
			continue
		}
		sc, err := ParseScore(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		sw, ok := sc.Parts[0].SwingAt(0)
		w, wantSwing := want[s.ID]
		switch {
		case wantSwing && !ok:
			t.Errorf("%s: no swing marked", s.ID)
		case !wantSwing && ok:
			t.Errorf("%s: swing %+v marked on a straight idiom", s.ID, sw)
		case wantSwing && (sw.SwingRatio != w[0] || sw.SwingUnit != w[1]):
			t.Errorf("%s: swing %v:%v, want ratio %v on unit %v", s.ID, sw.SwingRatio, sw.SwingUnit, w[0], w[1])
		}
	}
}

// The one place a percussion onset falls between grid steps, which the generator must turn into rush/delay.
func TestTrapRollIsATuplet(t *testing.T) {
	sc, err := ParseScore("groove-trap")
	if err != nil {
		t.Fatal(err)
	}
	off := 0
	for _, n := range sc.Parts[0].Notes {
		pos := float64(n.OnsetDiv) / float64(sc.Parts[0].Divisions) * 8
		if n.PercKey == 42 && pos != math.Trunc(pos) {
			off++
		}
	}
	if off != 4 {
		t.Errorf("trap hi-hat has %d off-grid onsets, want 4: a six-note sixteenth-triplet roll puts four strokes between the 32nd grid lines", off)
	}
}

// The one TRANSCRIBED melody: its pitches, typed from the cited public-domain sheet (Commons, Korobeiniki.jpg, C minor,
// 2/4, both strains), must be the score's, in order.
func TestKorobeinikiMatchesTheSheet(t *testing.T) {
	sheet := []string{
		"G5", "D5", "Eb5", "F5", "Eb5", "D5", "C5", "C5", "Eb5", "G5", "F5", "Eb5",
		"D5", "Eb5", "F5", "G5", "Eb5", "C5", "C5",
		"F5", "Ab5", "C6", "Bb5", "Ab5", "G5", "Eb5", "G5", "F5", "Eb5",
		"D5", "D5", "Eb5", "F5", "G5", "Eb5", "C5", "C5",
	}
	sc, err := ParseScore("trad-korobeiniki")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	names := []string{"A", "Bb", "B", "C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab"}
	for _, n := range sc.Parts[0].Notes {
		if n.Rest {
			continue
		}
		midi := n.Semitone + 57
		got = append(got, names[((n.Semitone%12)+12)%12]+fmt.Sprint(midi/12-1))
	}
	if fmt.Sprint(got) != fmt.Sprint(sheet) {
		t.Errorf("Korobeiniki pitches\n got %v\nwant %v", got, sheet)
	}
}
