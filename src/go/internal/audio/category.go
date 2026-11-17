package audio

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/enum"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Category is an instrument-menu category: a closed enum (the menu's tabs, in
// display order). CatNone is "no category" — the zero value, never a tab.
type Category int

// The categories, in display order.
const (
	CatNone Category = iota
	CatKick
	CatSnare
	CatHiHat
	CatCymbal
	CatTom
	CatPercussion
	CatBass
	CatKeys
	CatGuitar
	CatStrings
	CatWoodwind
	CatBrass
	CatVoice
	CatLead
	CatOther
)

var categoryNames = enum.Names[Category]{Kind: "instrument category", Names: []string{
	"", "kick", "snare", "hihat", "cymbal", "tom", "percussion", "bass", "keys", "guitar",
	"strings", "woodwind", "brass", "voice", "lead", "other",
}}

// categoryLabels are the menu's display labels, index = Category.
var categoryLabels = [...]string{
	"", "Kick", "Snare", "Hi-Hat", "Cymbal", "Tom", "Percussion", "Bass", "Keys", "Guitar",
	"Strings", "Woodwind", "Brass", "Voice", "Lead/Synth", "Other",
}

// String is the category's stable spelling ("hihat").
func (c Category) String() string { return enum.String(categoryNames, c) }

// Label is the category's display label ("Hi-Hat").
func (c Category) Label() string {
	if !enum.Valid(categoryNames, c) {
		return c.String()
	}
	return categoryLabels[c]
}

// Categories returns every category in display order (CatNone excluded).
func Categories() []Category { return enum.All(categoryNames)[1:] }

// CategoryOf returns an instrument's category: a shipped instrument's own
// (builtinProperties; an instance "x--N" has x's), else -- for an instrument
// the user named, a WAV sample or a Save-As instrument -- the category its
// display name suggests, else CatOther.
func CategoryOf(id instrument.ID) Category {
	if id == "" {
		return CatOther
	}
	if p, ok := builtinPropertyOf(id); ok {
		return p.Category
	}
	return categoryFromName(InstrumentDisplayName(id))
}

// categoryFromName classifies an instrument the user named. A voice/choir
// word wins over the role rules: roleFromName has a generic "bass" rule
// with no voice awareness, so a voice-flavored name that also says "bass"
// would otherwise land in CatBass.
func categoryFromName(name string) Category {
	if c, ok := categoryByText(name); ok && c == CatVoice {
		return c
	}
	if c, ok := categoryByRole(roleFromName(name)); ok {
		return c
	}
	if c, ok := categoryByText(name); ok {
		return c
	}
	return CatOther
}

// InstrumentsByCategory returns every known instrument ID in a category, in
// registration/catalog order.
func InstrumentsByCategory(cat Category) []instrument.ID {
	seen := map[instrument.ID]bool{}
	var out []instrument.ID
	add := func(id instrument.ID) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		if CategoryOf(id) == cat {
			out = append(out, id)
		}
	}
	for _, id := range Instruments() {
		add(id)
	}
	for _, m := range Catalog() {
		add(m.ID)
	}
	return out
}

// categoryByText classifies an instrument name by substring. Ordered most-
// specific first so "open-hihat" hits HiHat before any generic rule.
func categoryByText(s string) (Category, bool) {
	lc := strings.ToLower(s)
	switch {
	// "hat" only as a suffix ("open-hat", "closed-hat") — a bare Contains would
	// misclassify unrelated ids like "ratchet" or "whatchamacallit" as HiHat.
	case strings.Contains(lc, "hihat"), strings.Contains(lc, "hi-hat"), strings.HasSuffix(lc, "hat"):
		return CatHiHat, true
	case strings.Contains(lc, "ride"), strings.Contains(lc, "crash"), strings.Contains(lc, "cymbal"):
		return CatCymbal, true
	case strings.Contains(lc, "kick"):
		return CatKick, true
	case strings.Contains(lc, "snare"), strings.Contains(lc, "rimshot"), strings.Contains(lc, "sidestick"):
		return CatSnare, true
	case strings.Contains(lc, "tom"):
		return CatTom, true
	case strings.Contains(lc, "clap"), strings.Contains(lc, "cowbell"), strings.Contains(lc, "shaker"), strings.Contains(lc, "conga"), strings.Contains(lc, "perc"):
		return CatPercussion, true
	// Voice/choir before "bass"/"string"/"lead": a voice-flavored id must hit
	// Voice, not the bass substring rule below (ghost-bass/viola-pad/
	// ensemble-lead(-dark) are NOT voice-flavored ids, so they correctly fall
	// through to the bass/strings/lead rules per the 2026-07-08 re-category).
	case strings.Contains(lc, "choir"), strings.Contains(lc, "vocal"), strings.Contains(lc, "voice"),
		strings.Contains(lc, "soprano"), strings.Contains(lc, "whisper"):
		return CatVoice, true
	case strings.Contains(lc, "bass"), strings.Contains(lc, "808"), strings.Contains(lc, "sub"):
		return CatBass, true
	case strings.Contains(lc, "guitar"):
		return CatGuitar, true
	case strings.Contains(lc, "violin"), strings.Contains(lc, "viola"), strings.Contains(lc, "cello"), strings.Contains(lc, "harp"), strings.Contains(lc, "string"):
		return CatStrings, true
	case strings.Contains(lc, "piano"), strings.Contains(lc, "organ"), strings.Contains(lc, "epiano"), strings.Contains(lc, "keys"):
		return CatKeys, true
	case strings.Contains(lc, "flute"), strings.Contains(lc, "oboe"), strings.Contains(lc, "sax"), strings.Contains(lc, "clarinet"):
		return CatWoodwind, true
	case strings.Contains(lc, "trumpet"), strings.Contains(lc, "horn"), strings.Contains(lc, "brass"), strings.Contains(lc, "trombone"):
		return CatBrass, true
	case strings.Contains(lc, "lead"), strings.Contains(lc, "pluck"), strings.Contains(lc, "bell"):
		return CatLead, true
	}
	return CatNone, false
}

// categoryByRole maps a kit role to a category.
func categoryByRole(role instrument.Role) (Category, bool) {
	switch role {
	case instrument.RoleKick:
		return CatKick, true
	case instrument.RoleSnare:
		return CatSnare, true
	case instrument.RoleHat:
		return CatHiHat, true
	case instrument.RoleTom:
		return CatTom, true
	case instrument.RoleRide, instrument.RoleCrash:
		return CatCymbal, true
	case instrument.RoleClap, instrument.RoleCowbell, instrument.RoleShaker:
		return CatPercussion, true
	case instrument.RoleBass:
		return CatBass, true
	}
	return CatNone, false
}

// synthCatalogCategory is the catalog group of a built-in synth: its menu
// category, marked as synthesized (WAV assets group under "... (WAV)").
func synthCatalogCategory(id instrument.ID) string {
	return CategoryOf(id).Label() + " (Synth)"
}
