package ui

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Save As, the only save: the project owns COPIES of library documents, and Save As files a copy (document +
// audio bundle) as a new VERSION, of its own instrument ("over", nothing overwritten) or a new one.

// classifySaveAsName has already refused another instrument's name; a save that cannot be stored notifies and
// changes nothing.
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

// Every circuit opened later plays this version instead of the document it carries. "" when nothing was saved.
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

func (dv *DrumView) SaveActiveRecipeAs(displayName string) instrument.ID {
	return dv.SaveActiveRecipeAsFor(dv.synthTabActiveInstrument(), displayName)
}

// A new content-addressed user instrument; the copy's Origin is repointed so the row keeps its id and channel,
// and the playing rows take the new name. One undo step reverts name and origin; the library entry stays.
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

// A new user sample on the same pristine PCM with the source's bundle; nothing is baked.
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
