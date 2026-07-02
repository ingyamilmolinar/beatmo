package audio

import (
	"strings"
	"sync"
)

// CategoryID is the stable identifier for an instrument category. The
// built-in taxonomy is hardcoded; custom categories may be added at runtime
// via RegisterCategory (persistence is deferred).
type CategoryID string

// Category is a displayable taxonomy entry.
type Category struct {
	ID   CategoryID
	Name string
}

// Built-in category taxonomy. Names are display labels.
const (
	CatKick       CategoryID = "kick"
	CatSnare      CategoryID = "snare"
	CatHiHat      CategoryID = "hihat"
	CatCymbal     CategoryID = "cymbal"
	CatTom        CategoryID = "tom"
	CatPercussion CategoryID = "percussion"
	CatBass       CategoryID = "bass"
	CatKeys       CategoryID = "keys"
	CatGuitar     CategoryID = "guitar"
	CatStrings    CategoryID = "strings"
	CatWoodwind   CategoryID = "woodwind"
	CatBrass      CategoryID = "brass"
	CatLead       CategoryID = "lead"
	CatOther      CategoryID = "other"
)

var (
	categoryMu sync.RWMutex
	// builtinCategoryOrder is the canonical display order.
	builtinCategoryOrder = []Category{
		{CatKick, "Kick"}, {CatSnare, "Snare"}, {CatHiHat, "Hi-Hat"},
		{CatCymbal, "Cymbal"}, {CatTom, "Tom"}, {CatPercussion, "Percussion"},
		{CatBass, "Bass"}, {CatKeys, "Keys"}, {CatGuitar, "Guitar"},
		{CatStrings, "Strings"}, {CatWoodwind, "Woodwind"}, {CatBrass, "Brass"},
		{CatLead, "Lead/Synth"}, {CatOther, "Other"},
	}
	// extraCategories holds custom categories registered at runtime.
	extraCategories []Category
	// categoryAssignments holds explicit per-instrument overrides (custom or
	// corrective) that take precedence over the derived category.
	categoryAssignments = map[string]CategoryID{}
)

// Categories returns the taxonomy in display order (built-ins, then customs).
func Categories() []Category {
	categoryMu.RLock()
	defer categoryMu.RUnlock()
	out := make([]Category, 0, len(builtinCategoryOrder)+len(extraCategories))
	out = append(out, builtinCategoryOrder...)
	out = append(out, extraCategories...)
	return out
}

// RegisterCategory adds a custom category to the taxonomy (idempotent by ID).
// Persistence is deferred; this only mutates the in-memory registry.
func RegisterCategory(c Category) {
	if c.ID == "" {
		return
	}
	categoryMu.Lock()
	defer categoryMu.Unlock()
	for _, e := range builtinCategoryOrder {
		if e.ID == c.ID {
			return
		}
	}
	for i, e := range extraCategories {
		if e.ID == c.ID {
			extraCategories[i] = c
			return
		}
	}
	extraCategories = append(extraCategories, c)
}

// AssignCategory pins an instrument ID to a category, overriding derivation.
func AssignCategory(instID string, cat CategoryID) {
	if instID == "" {
		return
	}
	categoryMu.Lock()
	categoryAssignments[instID] = cat
	categoryMu.Unlock()
}

// CategoryOf returns the category for an instrument ID. Order: explicit
// assignment → recipe-derived → role-heuristic-derived → CatOther.
func CategoryOf(id string) CategoryID {
	if id == "" {
		return CatOther
	}
	categoryMu.RLock()
	assigned, ok := categoryAssignments[id]
	categoryMu.RUnlock()
	if ok {
		return assigned
	}
	if c, ok := categoryByRecipe(factoryRecipeForInstrument(id)); ok {
		return c
	}
	if c, ok := categoryByRole(roleHeuristic(id)); ok {
		return c
	}
	// Last resort: try the id text itself (covers WAV-only catalog ids with
	// no recipe binding, e.g. "kick-acoustic").
	if c, ok := categoryByText(id); ok {
		return c
	}
	return CatOther
}

// InstrumentsByCategory returns every known instrument ID in a category, in
// registration/catalog order.
func InstrumentsByCategory(cat CategoryID) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
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

// categoryByRecipe maps a SynthRecipe id to a category via substring rules.
func categoryByRecipe(recipe string) (CategoryID, bool) {
	if recipe == "" {
		return "", false
	}
	return categoryByText(recipe)
}

// categoryByText classifies an id/recipe string by substring. Ordered most-
// specific first so "open-hihat" hits HiHat before any generic rule.
func categoryByText(s string) (CategoryID, bool) {
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
	case strings.Contains(lc, "bass"), strings.Contains(lc, "808"), strings.Contains(lc, "sub"):
		return CatBass, true
	case strings.Contains(lc, "guitar"):
		return CatGuitar, true
	case strings.Contains(lc, "violin"), strings.Contains(lc, "cello"), strings.Contains(lc, "harp"), strings.Contains(lc, "string"):
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
	return "", false
}

// categoryByRole maps a kit role (from roleHeuristic) to a category.
func categoryByRole(role string) (CategoryID, bool) {
	switch role {
	case "kick":
		return CatKick, true
	case "snare":
		return CatSnare, true
	case "hat":
		return CatHiHat, true
	case "tom":
		return CatTom, true
	case "ride", "crash":
		return CatCymbal, true
	case "clap", "cowbell", "shaker":
		return CatPercussion, true
	case "bass":
		return CatBass, true
	}
	return "", false
}
