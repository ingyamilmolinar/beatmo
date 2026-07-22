package ui

import (
	"encoding/json"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// CollisionChoice is how the caller resolves an id/name collision with an
// existing USER instrument on import.
type CollisionChoice int

const (
	// CollisionReplace overwrites the existing user instrument in place.
	CollisionReplace CollisionChoice = iota
	// CollisionKeepBoth imports as a fresh, re-namespaced user instrument.
	CollisionKeepBoth
)

// ImportCollision describes a colliding instrument the resolver must decide on.
type ImportCollision struct {
	DocIndex int
	Name     string
	RecipeID string
}

// ImportResult reports the outcome for one imported instrument.
type ImportResult struct {
	ID       string
	Name     string
	WavRef   *audio.WavRef
	WavState audio.WavResolveStatus // meaningful only when WavRef != nil
}

// ApplyInstrumentBundle registers every instrument in a portable bundle into the
// running app (config-only — recipe, effects, EQ, sample-edit, and a WAV
// reference resolved via locators). It never overwrites a built-in (a colliding
// recipe id is re-namespaced); a colliding USER instrument is resolved by the
// caller (Replace / Keep both). When applyToRow is a valid row index, the LAST
// imported instrument is bound to that row and its EQ applied there.
//
// locators supply the platform byte-fetchers per WAV source kind (file /
// localstorage / url); pass nil to skip WAV resolution (config still imports).
func (dv *DrumView) ApplyInstrumentBundle(
	b *audio.InstrumentBundle,
	resolve func(ImportCollision) CollisionChoice,
	applyToRow int,
	locators map[string]audio.WavLocator,
) ([]ImportResult, error) {
	if err := audio.ValidateBundleHeader(b); err != nil {
		return nil, err
	}
	sink := activeRecipeSink()
	var results []ImportResult
	for i, doc := range b.Instruments {
		newID := dv.resolveImportID(i, doc, resolve)

		if _, err := audio.RegisterUserRecipeFromBase(newID, doc.Name, doc.Recipe.BaseRecipe, doc.Recipe.ParamSeed); err != nil {
			return results, err
		}
		// An imported instrument's instrument-id equals its recipe-id.
		audio.BindInstrumentToRecipe(newID, newID)

		// Effects: denormalize the shared library + this instrument's ordered chain.
		audio.SetInsertEffects(newID, audio.DenormalizeEffects(b.Effects, doc.EffectChain))

		// Sample-edit descriptor (post-render transform).
		if len(doc.SampleEdit) > 0 {
			audio.SetSampleEdit(newID, audio.SampleEditFromFields(doc.SampleEdit))
		}

		res := ImportResult{ID: newID, Name: doc.Name, WavRef: doc.WavRef}
		if doc.WavRef != nil && locators != nil {
			r := audio.ResolveWavRef(doc.WavRef, locators)
			res.WavState = r.Status
			if r.Status == audio.WavResolveMatched {
				// Hash matched the pinned version — decode the bytes and bind.
				if pcm, sr, derr := wavgen.DecodeWAV(r.Data); derr == nil {
					audio.PutUserSample(newID, pcm, sr)
					dv.wavRegistry().Register(newID, r.GotSha)
				} else {
					res.WavState = audio.WavResolveMissing // undecodable → treat as unresolved
				}
			}
		}

		// EQ + row binding when a target row is given (last instrument wins).
		if applyToRow >= 0 && applyToRow < len(dv.Rows) {
			if doc.EQ != nil {
				applyInstrumentEQToRow(dv.Rows[applyToRow], doc.EQ)
			}
			dv.Rows[applyToRow].Instrument = newID
		}

		// Persist the reconstructed recipe to the (decoupled) store, best-effort.
		if sink != nil {
			if raw, err := json.Marshal(recipeDocForStore(newID, doc)); err == nil {
				_ = sink.SaveUserRecipe(newID, raw)
			}
		}

		results = append(results, res)
	}
	return results, nil
}

// bundleUserCollisions returns the docs whose recipe id collides with an
// existing USER instrument (built-ins never collide — they are re-namespaced).
// Used to decide whether to prompt Replace/Keep-both before applying.
func bundleUserCollisions(b *audio.InstrumentBundle) []ImportCollision {
	if b == nil {
		return nil
	}
	var out []ImportCollision
	for i, doc := range b.Instruments {
		want := doc.Recipe.ID
		if want == "" || audio.RecipeIsBuiltin(want) {
			continue
		}
		if audio.RecipeExists(want) {
			out = append(out, ImportCollision{DocIndex: i, Name: doc.Name, RecipeID: want})
		}
	}
	return out
}

// resolveImportID picks the id a bundled instrument registers under, protecting
// built-ins and honoring the collision resolver for user dups.
func (dv *DrumView) resolveImportID(idx int, doc audio.InstrumentDoc, resolve func(ImportCollision) CollisionChoice) string {
	want := doc.Recipe.ID
	if want == "" || audio.RecipeIsBuiltin(want) {
		// Never clobber a builtin (or a doc with no id) — mint a fresh user id.
		return generateUserRecipeID(doc.Recipe.BaseRecipe, doc.Recipe.ParamSeed)
	}
	if audio.RecipeExists(want) {
		choice := CollisionReplace
		if resolve != nil {
			choice = resolve(ImportCollision{DocIndex: idx, Name: doc.Name, RecipeID: want})
		}
		if choice == CollisionKeepBoth {
			return generateUserRecipeID(doc.Recipe.BaseRecipe, doc.Recipe.ParamSeed)
		}
	}
	return want
}

// recipeDocForStore builds the RecipeDoc persisted for an imported instrument,
// with its id set to the (possibly re-namespaced) newID.
func recipeDocForStore(newID string, doc audio.InstrumentDoc) audio.RecipeDoc {
	rd := doc.Recipe
	rd.ID = newID
	return rd
}

// applyInstrumentEQToRow copies a bundle's inline EQ onto a row.
func applyInstrumentEQToRow(row *DrumRow, eq *audio.InstrumentEQ) {
	if eq == nil {
		return
	}
	if len(eq.GainsDB) > 0 {
		row.EQGainsDB = append([]float64(nil), eq.GainsDB...)
	}
	if len(eq.BandMuted) > 0 {
		row.EQBandMuted = append([]bool(nil), eq.BandMuted...)
	}
	row.HPFEnabled = eq.HPFEnabled
	row.HPFCutoffHz = eq.HPFCutoffHz
	row.LPFEnabled = eq.LPFEnabled
	row.LPFCutoffHz = eq.LPFCutoffHz
}
