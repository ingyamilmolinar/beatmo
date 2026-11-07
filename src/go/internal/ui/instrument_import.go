package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

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

// Never overwrites a built-in (re-namespaced); a USER collision is resolved by the caller. With a valid row,
// the LAST instrument binds there. nil locators skip WAV resolution.
func (dv *DrumView) ApplyInstrumentBundle(
	b *audio.InstrumentBundle,
	resolve func(ImportCollision) CollisionChoice,
	applyToRow int,
	locators map[audio.WavSourceKind]audio.WavLocator,
) ([]ImportResult, error) {
	if err := audio.ValidateBundleHeader(b); err != nil {
		return nil, err
	}
	// Validate every document first: a bad second one must not leave the first registered.
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
			// The project's copy has the library entry as its Origin.
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

		// Through the real pick path so dependents follow.
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

// Built-ins never collide: they are re-namespaced.
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

// For a shipped id, a Keep-both collision, or no id at all.
func freshImportID(doc audio.BundleInstrument) instrument.ID {
	if doc.Synth != nil {
		return audio.NewUserInstrumentID(doc.Name, *doc.Synth)
	}
	return userSampleID(doc.Name, "bundle")
}

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
