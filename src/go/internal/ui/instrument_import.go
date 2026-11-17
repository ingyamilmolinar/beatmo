package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
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
	RecipeID instrument.ID
}

// ImportResult reports the outcome for one imported instrument.
type ImportResult struct {
	ID       instrument.ID
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
	locators map[audio.WavSourceKind]audio.WavLocator,
) ([]ImportResult, error) {
	if err := audio.ValidateBundleHeader(b); err != nil {
		return nil, err
	}
	// Validate EVERY document before registering any: a bad second doc must not
	// leave the first one registered, bound and persisted.
	for i, doc := range b.Instruments {
		if doc.Synth == nil {
			continue
		}
		d, err := audio.MigrateAndValidateInstrumentDoc(*doc.Synth)
		if err != nil {
			return nil, fmt.Errorf("instruments[%d] %q: %w", i, doc.Name, err)
		}
		b.Instruments[i].Synth = &d
	}
	var results []ImportResult
	for i, doc := range b.Instruments {
		newID := dv.resolveImportID(i, doc, resolve)

		if doc.Synth != nil {
			// The library gets the document under its (possibly re-namespaced)
			// id; the project gets a copy whose Origin is that entry.
			lib := doc.Synth.Clone()
			lib.ID, lib.Origin = newID, ""
			if err := audio.SaveLibraryDoc(lib); err != nil {
				return results, err
			}
			proj := lib.Clone()
			proj.Origin = newID
			if err := audio.SetInstrumentDoc(newID, proj); err != nil {
				return results, err
			}
			audio.RegisterLibraryInstrument(newID)
		}

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
					dv.wavRegistry().Register(string(newID), r.GotSha)
				} else {
					res.WavState = audio.WavResolveMissing // undecodable → treat as unresolved
				}
			}
		}

		// EQ + row binding when a target row is given (last instrument wins),
		// through the real pick path so dependents (EQ channel …) follow.
		if applyToRow >= 0 && applyToRow < len(dv.Rows) {
			if doc.EQ != nil {
				applyInstrumentEQToRow(dv.Rows[applyToRow], doc.EQ)
			}
			dv.selRow = applyToRow
			dv.SetInstrument(newID)
		}

		results = append(results, res)
	}
	dv.refreshInstruments()
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
		want := bundleWantID(doc)
		if want == "" {
			continue
		}
		if _, shipped := audio.ShippedDoc(want); shipped {
			continue
		}
		if _, saved := audio.UserLibraryDoc(want); saved {
			out = append(out, ImportCollision{DocIndex: i, Name: doc.Name, RecipeID: want})
		}
	}
	return out
}

// bundleWantID is the id a bundled instrument asks for (its document's id).
func bundleWantID(doc audio.BundleInstrument) instrument.ID {
	if doc.Synth != nil {
		return doc.Synth.ID
	}
	return ""
}

// freshImportID mints a user id for a bundled instrument that cannot keep the
// id it carries (a shipped id, a Keep-both collision, or none at all).
func freshImportID(doc audio.BundleInstrument) instrument.ID {
	if doc.Synth != nil {
		return audio.NewUserInstrumentID(doc.Name, *doc.Synth)
	}
	return userSampleID(doc.Name, "bundle")
}

// resolveImportID picks the id a bundled instrument registers under, protecting
// shipped instruments and honoring the collision resolver for user dups.
func (dv *DrumView) resolveImportID(idx int, doc audio.BundleInstrument, resolve func(ImportCollision) CollisionChoice) instrument.ID {
	want := bundleWantID(doc)
	if want == "" {
		return freshImportID(doc)
	}
	if _, shipped := audio.ShippedDoc(want); shipped {
		// Never clobber a shipped instrument — mint a fresh user id.
		return freshImportID(doc)
	}
	if _, saved := audio.UserLibraryDoc(want); saved {
		choice := CollisionReplace
		if resolve != nil {
			choice = resolve(ImportCollision{DocIndex: idx, Name: doc.Name, RecipeID: want})
		}
		if choice == CollisionKeepBoth {
			return freshImportID(doc)
		}
	}
	return want
}

// applyInstrumentEQToRow copies a bundle's inline EQ onto the row's
// instrument.
func applyInstrumentEQToRow(row *DrumRow, eq *audio.InstrumentEQ) {
	if eq == nil {
		return
	}
	dst := rowEQ(row)
	copy(dst.GainsDB, eq.GainsDB)
	copy(dst.BandMuted, eq.BandMuted)
	dst.HPF, dst.HPFHz = eq.HPFEnabled, eq.HPFCutoffHz
	dst.LPF, dst.LPFHz = eq.LPFEnabled, eq.LPFCutoffHz
}
