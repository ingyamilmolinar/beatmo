package ui

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// recipe_save_sink.go — Save As, the only save (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §2.7). The project owns COPIES of library instrument documents; Save As
// moves a copy — document and audio bundle (Sampler edit, EQ, insert effects)
// — into the library as a new VERSION: of the instrument it comes from ("save
// over", nothing overwritten) or of a new instrument ("save as new"). The
// library persists through audio.SetInstrumentLibrarySink (instruments.json /
// localStorage), wired by the bootstraps.

// saveAsInstrument runs the Save As the dialog confirmed: the name of the
// library instrument instID comes from saves over it, any other name makes a
// new one (classifySaveAsName already refused another instrument's name). A
// save that cannot be stored tells the user and changes nothing.
func (dv *DrumView) saveAsInstrument(instID instrument.ID, displayName string) instrument.ID {
	var id instrument.ID
	if classifySaveAsName(displayName, instID).kind == saveAsOver {
		id = saveRecipeForInstrument(instID)
	} else {
		id = dv.SaveActiveRecipeAsFor(instID, displayName)
	}
	if id == "" {
		dv.notifyErrorKey(i18n.KeySaveFailed, displayName)
	}
	return id
}

// saveRecipeForInstrument saves instID's project copy over the library
// instrument it comes from (audio.InstrumentLibraryIdentity: its Origin, else
// its instance base): a new version of it,
// carrying the instrument's current audio bundle. Every circuit opened later
// plays it instead of the document the circuit carries for the same
// instrument (audio.OpenCircuitInstrumentDocs). Returns the library id saved,
// or "" when nothing could be saved.
func saveRecipeForInstrument(instID instrument.ID) instrument.ID {
	if instID == "" {
		return ""
	}
	b := audio.CurrentAudioBundle(instID)
	doc, ok := audio.InstrumentDoc(instID)
	if !ok {
		if audio.SaveSampleVersion(instID, b) != nil {
			return ""
		}
		return instID
	}
	// The identity the dialog names and the circuit-open rule resolves.
	libID := audio.InstrumentLibraryIdentity(instID, doc)
	lib := doc.Clone()
	lib.ID, lib.Origin = libID, ""
	if err := audio.SaveInstrumentVersion(libID, lib, b); err != nil {
		return ""
	}
	hooks.PublishKind(hooks.EventRecipeSaved, hooks.RecipePayload{
		RecipeID:     libID,
		InstrumentID: instID,
	})
	return libID
}

// SaveActiveRecipeAs saves the Synth tab's instrument as a new one (see
// SaveActiveRecipeAsFor).
func (dv *DrumView) SaveActiveRecipeAs(displayName string) instrument.ID {
	return dv.SaveActiveRecipeAsFor(dv.synthTabActiveInstrument(), displayName)
}

// SaveActiveRecipeAsFor writes instID's project copy and audio bundle into the
// library as the first version of a NEW user instrument (id =
// audio.NewUserInstrumentID, content addressed), makes it playable and listed,
// and repoints the project copy's Origin at it — so the row keeps its id (and
// channel) while the dirty dot and the next Save As now refer to the new
// instrument. The rows playing instID take the new name (the same
// metadata-only override a rename writes, exported as the row name). One undo
// step (the rename) reverts the copy's name and origin; the library entry
// stays. A sample instrument's new instrument is a new user sample on the same
// PCM. displayName "" keeps the "<name> (saved)" suggestion. Returns the new
// library id, or "" when nothing was saved.
func (dv *DrumView) SaveActiveRecipeAsFor(instID instrument.ID, displayName string) instrument.ID {
	if instID == "" {
		return ""
	}
	b := audio.CurrentAudioBundle(instID)
	doc, ok := audio.InstrumentDoc(instID)
	if !ok {
		return dv.saveSampleAsNew(instID, displayName, b)
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = fmt.Sprintf("%s (saved)", doc.Name)
	}
	base := doc.Origin
	lib := doc.Clone()
	lib.Name = displayName
	lib.ID = audio.NewUserInstrumentID(displayName, lib)
	lib.Origin = ""
	if err := audio.SaveInstrumentVersion(lib.ID, lib, b); err != nil {
		return ""
	}
	audio.RegisterLibraryInstrument(lib.ID)
	// The copy now IS the new library entry (name included), so it reads clean.
	doc.Origin, doc.Name = lib.ID, lib.Name
	if err := audio.SetInstrumentDoc(instID, doc); err != nil {
		return ""
	}
	dv.refreshInstruments()
	dv.applyInstrumentDisplayName(instID, displayName)
	emitInstrumentRenamed(instID, instID)
	hooks.PublishKind(hooks.EventRecipeCreated, hooks.RecipePayload{
		RecipeID:     lib.ID,
		BaseRecipe:   base,
		InstrumentID: instID,
		DisplayName:  displayName,
	})
	return lib.ID
}

// saveSampleAsNew makes a sample instrument's Save As new: a new user sample
// on the same (pristine) PCM, with the source's audio bundle, as its first
// version; the rows that played the source play it. Nothing is baked.
func (dv *DrumView) saveSampleAsNew(src instrument.ID, displayName string, b audio.AudioBundle) instrument.ID {
	rec, ok := audio.UserSamplePCM(src)
	if !ok {
		return ""
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = audio.InstrumentDisplayName(src)
	}
	id := userSampleID(displayName, string(src))
	audio.SaveUserSample(id, rec.PCM, rec.SampleRate)
	audio.ApplyAudioBundle(id, b)
	if audio.SaveSampleVersion(id, b) != nil {
		return ""
	}
	audio.SetInstrumentDisplayName(id, displayName)
	for i, r := range dv.Rows {
		if r != nil && r.Instrument == src {
			dv.selRow = i
			dv.SetInstrument(id)
			dv.Rows[i].Name = displayName
		}
	}
	dv.refreshInstruments()
	dv.markAllRowsDirty()
	hooks.PublishKind(hooks.EventSampleCreated, hooks.SamplePayload{
		SampleID:    id,
		SourceID:    src,
		DisplayName: displayName,
		Frames:      len(rec.PCM),
	})
	return id
}
