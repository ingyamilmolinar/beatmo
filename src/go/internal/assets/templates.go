package assets

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed templates/*.json
var templateFS embed.FS

// Generated from a committed public-domain score by cmd/gen-showcase (the notes ARE the score), so never hand-edit
// templates/*.json: regenerate with go run ./cmd/gen-showcase. Display and BPM come from each file; only the menu order
// is kept in Go.
type Template struct {
	Genre   string
	Display string
	BPM     int
	Group   string // menu group label (from templateGroups) — drives the section headers
	Bytes   []byte
}

// The ONLY hand-maintained template metadata (menu grouping and order, by stem); Display and BPM come from the embedded
// JSON so they cannot drift. Labels render as section headers when the search is empty.
var templateGroups = []struct {
	Label string
	Stems []string
}{
	{"Funk · Soul · Jazz", []string{
		"funky-drummer", "impeach-the-president", "superstition-groove", "hook-and-sling",
		"look-ka-py-py", "synthetic-substitution", "haitian-divorce", "palm-grease", "expensive-shit",
		"jazz-swing",
	}},
	{"Rock · Post-punk", []string{
		"we-will-rock-you-stomp", "rosanna-shuffle", "in-the-air-tonight", "levee-breaks-rock",
		"immigrant-song", "hit-me-with-your-rhythm-stick", "dirty-love", "rock-lobster", "peek-a-boo",
		"scentless-apprentice", "iron-man",
	}},
	{"Industrial · Synth", []string{
		"dig-it", "achtung", "down-in-it", "being-boiled", "1000-knives", "raised-by-snakes",
		"problemes-damour", "e-mc2", "blue-monday-synthpop",
	}},
	{"Hip-hop · Electro", []string{
		"planet-rock-electro", "needle-to-the-groove", "egypt-egypt", "buffalo-gals", "clear",
		"you-got-me", "trap-808",
	}},
	{"House · Techno", []string{
		"on-and-on", "can-you-feel-it", "no-way-back", "french-kiss", "dominatrix-sleeps-tonight",
		"voodoo-ray", "north-american-scum", "da-funk-house", "uk-garage-2step", "tetris-techno",
	}},
	{"Breaks · IDM", []string{
		"amen-break-jungle", "flim", "xtal", "idioteque", "when-i-rule-the-world",
	}},
	{"Latin · Global", []string{
		"reggaeton-dembow", "baile-funk", "dancehall-riddim", "salsa-clave", "cumbia", "merengue",
		"vallenato",
	}},
	{"Baroque", []string{
		"bach-brandenburg-3", "bach-brandenburg-4", "bach-kyrie-bwv232",
	}},
	{"Classical", []string{
		"beethoven-fifth", "beethoven-ode-to-joy", "haydn-london", "schubert-fifth",
		"haydn-quartet-op76", "mozart-quartet-k387", "beethoven-quartet-op18",
	}},
	{"Romantic", []string{
		"brahms-first-finale", "dvorak-american", "borodin-quartet-2",
		"beach-gaelic", "boulanger-printemps",
	}},
	{"Lied · Song", []string{
		"schubert-erlkonig", "schubert-ave-maria", "schubert-standchen",
		"clara-schumann-op13-1", "hensel-italien",
	}},
	{"French · Americas", []string{
		"faure-apres-un-reve", "satie-air-du-rat",
		"liliuokalani-aloha-oe", "gonzaga-espanha-e-brasil",
	}},
}

// Menu order, tests and the search index key off the flat series.
var templateOrder = flattenTemplateGroups()

func flattenTemplateGroups() []string {
	var out []string
	for _, g := range templateGroups {
		out = append(out, g.Stems...)
	}
	return out
}

// Parsed once and cached: the template menu calls this every frame while open, and re-parsing the embedded JSON per
// call froze WASM mobile. Read-only to callers; panics at init only on a missing or malformed embed.
func Templates() []Template {
	templatesOnce.Do(func() { templatesCache = buildTemplates() })
	return templatesCache
}

var (
	templatesOnce  sync.Once
	templatesCache []Template
)

func buildTemplates() []Template {
	out := make([]Template, 0, len(templateOrder))
	for _, g := range templateGroups {
		for _, genre := range g.Stems {
			b, err := templateFS.ReadFile("templates/" + genre + ".json")
			if err != nil {
				panic("assets: missing embedded template " + genre + ".json: " + err.Error())
			}
			var meta struct {
				BPM int `json:"bpm"`
			}
			if err := json.Unmarshal(b, &meta); err != nil {
				panic("assets: malformed template " + genre + ".json: " + err.Error())
			}
			out = append(out, Template{
				Genre:   genre,
				Display: displayName(genre),
				BPM:     meta.BPM,
				Group:   g.Label,
				Bytes:   b,
			})
		}
	}
	return out
}

// The real "Artist — Song" title, since a template is A/B'd against the named original; every stem needs one.
var displayTitles = map[string]string{
	"funky-drummer":                 "James Brown — Funky Drummer",
	"impeach-the-president":         "The Honey Drippers — Impeach the President",
	"superstition-groove":           "Stevie Wonder — Superstition",
	"amen-break-jungle":             "The Winstons — Amen, Brother",
	"planet-rock-electro":           "Afrika Bambaataa — Planet Rock",
	"blue-monday-synthpop":          "New Order — Blue Monday",
	"da-funk-house":                 "Daft Punk — Da Funk",
	"uk-garage-2step":               "UK garage — 2-step beat",
	"trap-808":                      "Trap — 808 hats & rolls",
	"tetris-techno":                 "Tetris — Korobeiniki",
	"we-will-rock-you-stomp":        "Queen — We Will Rock You",
	"rosanna-shuffle":               "Toto — Rosanna",
	"in-the-air-tonight":            "Phil Collins — In the Air Tonight",
	"levee-breaks-rock":             "Led Zeppelin — When the Levee Breaks",
	"jazz-swing":                    "Jazz swing — ride & walking bass",
	"reggaeton-dembow":              "Reggaeton — dembow beat",
	"baile-funk":                    "Baile funk — tamborzão",
	"dancehall-riddim":              "Dancehall — riddim",
	"salsa-clave":                   "Salsa — son clave 2-3",
	"cumbia":                        "Cumbia — guacharaca & llamador",
	"merengue":                      "Merengue — güira & tambora",
	"vallenato":                     "Vallenato — caja & guacharaca",
	"hook-and-sling":                "Eddie Bo — Hook and Sling",
	"look-ka-py-py":                 "The Meters — Look-Ka Py Py",
	"synthetic-substitution":        "Melvin Bliss — Synthetic Substitution",
	"haitian-divorce":               "Steely Dan — Haitian Divorce",
	"palm-grease":                   "Herbie Hancock — Palm Grease",
	"expensive-shit":                "Fela Kuti — Expensive Shit",
	"immigrant-song":                "Led Zeppelin — Immigrant Song",
	"hit-me-with-your-rhythm-stick": "Ian Dury & the Blockheads — Hit Me with Your Rhythm Stick",
	"dirty-love":                    "Frank Zappa — Dirty Love",
	"rock-lobster":                  "The B-52's — Rock Lobster",
	"peek-a-boo":                    "Devo — Peek-a-Boo!",
	"scentless-apprentice":          "Nirvana — Scentless Apprentice",
	"iron-man":                      "Black Sabbath — Iron Man",
	"dig-it":                        "Skinny Puppy — Dig It",
	"achtung":                       "Laibach — Achtung",
	"down-in-it":                    "Nine Inch Nails — Down in It",
	"being-boiled":                  "The Human League — Being Boiled",
	"1000-knives":                   "Yellow Magic Orchestra — 1000 Knives",
	"raised-by-snakes":              "Telex — Raised by Snakes",
	"problemes-damour":              "Alexander Robotnick — Problèmes d'amour",
	"e-mc2":                         "Giorgio Moroder — E=MC²",
	"needle-to-the-groove":          "Mantronix — Needle to the Groove",
	"egypt-egypt":                   "Egyptian Lover — Egypt, Egypt",
	"buffalo-gals":                  "Malcolm McLaren — Buffalo Gals",
	"clear":                         "Cybotron — Clear",
	"you-got-me":                    "The Roots — You Got Me",
	"on-and-on":                     "Jesse Saunders — On and On",
	"can-you-feel-it":               "Mr. Fingers — Can You Feel It",
	"no-way-back":                   "Adonis — No Way Back",
	"french-kiss":                   "Lil Louis — French Kiss",
	"dominatrix-sleeps-tonight":     "The Dominatrix — The Dominatrix Sleeps Tonight",
	"voodoo-ray":                    "A Guy Called Gerald — Voodoo Ray",
	"north-american-scum":           "LCD Soundsystem — North American Scum",
	"flim":                          "Aphex Twin — Flim",
	"xtal":                          "Aphex Twin — Xtal",
	"idioteque":                     "Radiohead — Idioteque",
	"when-i-rule-the-world":         "LIZ (prod. SOPHIE) — When I Rule the World",
	"bach-brandenburg-3":            "Bach — Brandenburg Concerto No. 3",
	"bach-brandenburg-4":            "Bach — Brandenburg Concerto No. 4",
	"bach-kyrie-bwv232":             "Bach — Mass in B minor, Kyrie",
	"beethoven-fifth":               "Beethoven — Symphony No. 5",
	"beethoven-ode-to-joy":          "Beethoven — Ode to Joy",
	"haydn-london":                  "Haydn — Symphony No. 104 (London)",
	"schubert-fifth":                "Schubert — Symphony No. 5",
	"haydn-quartet-op76":            "Haydn — String Quartet Op. 76 No. 1",
	"mozart-quartet-k387":           "Mozart — String Quartet K. 387",
	"beethoven-quartet-op18":        "Beethoven — String Quartet Op. 18 No. 1",
	"brahms-first-finale":           "Brahms — Symphony No. 1, Finale",
	"dvorak-american":               "Dvořák — American Quartet",
	"borodin-quartet-2":             "Borodin — String Quartet No. 2",
	"beach-gaelic":                  "Amy Beach — Gaelic Symphony",
	"boulanger-printemps":           "Lili Boulanger — D'un matin de printemps",
	"schubert-erlkonig":             "Schubert — Erlkönig",
	"schubert-ave-maria":            "Schubert — Ave Maria",
	"schubert-standchen":            "Schubert — Ständchen",
	"clara-schumann-op13-1":         "Clara Schumann — Ich stand in dunklen Träumen",
	"hensel-italien":                "Fanny Hensel — Italien",
	"faure-apres-un-reve":           "Fauré — Après un rêve",
	"satie-air-du-rat":              "Satie — Air du rat",
	"liliuokalani-aloha-oe":         "Liliʻuokalani — Aloha ʻOe",
	"gonzaga-espanha-e-brasil":      "Chiquinha Gonzaga — Espanha e Brasil",
}

// displayName returns the menu title for a stem: the curated song title when
// known, otherwise a title-cased fallback ("hip-hop" -> "Hip-Hop") so an
// unmapped stem still renders a sane label rather than panicking.
func displayName(genre string) string {
	if title, ok := displayTitles[genre]; ok {
		return title
	}
	parts := strings.Split(genre, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "-")
}
