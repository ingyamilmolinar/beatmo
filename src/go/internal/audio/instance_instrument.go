package audio

// instanceBaseID strips a trailing "-1" / "-2" instance suffix used by the
// gen-showcase duplicate-id migration (e.g. "organ-2" → "organ"). Returns the
// id unchanged when it carries no such suffix. Tag-neutral: it is the single
// source of truth for the strip shared by factoryRecipeForInstrument (this
// file's package, all build tags) and EnsureInstanceInstrument. The "-1"/"-2"
// suffix mirrors variant ids; other variant suffixes are not instance variants
// and are left alone.
func instanceBaseID(id string) string {
	if len(id) > 2 && (id[len(id)-2:] == "-1" || id[len(id)-2:] == "-2") {
		return id[:len(id)-2]
	}
	return id
}

// EnsureInstanceInstrument promotes an instance variant id (e.g. "organ-2") to
// a first-class, available, recipe-bound, voiced instrument that renders
// IDENTICALLY to its base ("organ"). The gen-showcase migration auto-suffixes
// repeated instrument ids in templates so each voice can carry its own display
// name; without this promotion the variant id is in none of the playable
// registry, the catalog, or BuiltinInstrumentIDs, so it would be unavailable
// (import fails) and silent (no voice, no recipe).
//
// Behavior:
//   - idempotent: already-registered id → return.
//   - base == id (no instance suffix) → return.
//   - base not a registered instrument → return (nothing to alias).
//   - otherwise: alias the variant onto the base's playable Instrument and bind
//     it to the base's factory recipe, then seed its defaults so it takes the
//     same recipe-vs-legacy render path as the base. The recipe binding +
//     seeding is what guarantees render equivalence: tryRecipeVoice keys on
//     RecipeForInstrument(id) + the seeded recipe defaults, both of which now
//     match the base exactly.
//
// Mutates the PROCESS-GLOBAL instrument registry; tests must t.Cleanup with
// ResetInstruments.
func EnsureInstanceInstrument(id string) {
	if instanceAlreadyRegistered(id) {
		return
	}
	base := instanceBaseID(id)
	if base == id {
		return
	}
	if !instanceAlreadyRegistered(base) {
		return
	}
	registerInstanceAlias(id, base)
	if r := factoryRecipeForInstrument(id); r != "" {
		BindInstrumentToRecipe(id, r)
		// Seed the variant's recipe defaults to the platform layer so an
		// unedited variant renders through the same recipe path as its base
		// (otherwise a migrated/seeded recipe would fall to the generic tone).
		platformInstrumentDefaultsPush(id, RecipeDefaultParams(r))
	}
}
