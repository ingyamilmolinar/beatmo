package ui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// renameInstrumentTo sets a metadata-only display-name override for the
// instrument in the given row. The instrument ID is never changed — this is
// the entire rename operation. Empty/whitespace names are ignored.
func (dv *DrumView) renameInstrumentTo(row int, name string) {
	name = strings.TrimSpace(name)
	if name == "" || row < 0 || row >= len(dv.Rows) {
		return
	}
	id := dv.Rows[row].Instrument
	if id == "" {
		return
	}
	dv.applyInstrumentDisplayName(id, name)
	// The id is unchanged, so this is NOT an instrument swap — do NOT call
	// onRowInstrumentChanged. Emit the rename event (publishes
	// EventInstrumentRenamed + records undo via recordUndo). For a
	// metadata-only rename old==new id.
	emitInstrumentRenamed(id, id)
}

// applyInstrumentDisplayName sets id's display-name override and refreshes
// every row playing id (label, caches, open menu). Metadata only: the id
// never changes. Shared by rename and Save As (which names the instrument).
func (dv *DrumView) applyInstrumentDisplayName(id instrument.ID, name string) {
	audio.SetInstrumentDisplayName(id, name)
	dv.invalidateLabelCaches()
	labels := dv.rowLabels()
	for i, r := range dv.Rows {
		if r == nil || r.Instrument != id {
			continue
		}
		r.Name = dv.computeInstLabel(id)
		if i < len(labels) {
			labels[i].Text = r.Name
		}
	}
	if dv.IsInstMenuOpen() {
		dv.refreshInstMenuComponent()
	}
	dv.markRowControlsDirty()
	dv.bgDirty = true
}

func (dv *DrumView) refreshInstruments() {
	// Query available instruments (catalog + already-registered) lazily.
	if !runningUnderGoTest() {
		audio.InitDefaultCatalog()
	}
	catVer := audio.CatalogVersion()
	instVer := audio.InstrumentsVersion()
	if !dv.instRefreshDirty && dv.instCatalogVersion == catVer && dv.instRegistryVersion == instVer {
		return
	}
	dv.instCatalogVersion = catVer
	dv.instRegistryVersion = instVer
	dv.instRefreshDirty = false
	meta := audio.Catalog()
	dv.instMeta = map[instrument.ID]audio.SoundMeta{}
	for _, m := range meta {
		dv.instMeta[m.ID] = m
	}

	seen := map[instrument.ID]bool{}
	opts := []instrument.ID{}
	for _, id := range audio.Instruments() {
		if !seen[id] {
			opts = append(opts, id)
			seen[id] = true
		}
	}
	for _, m := range meta {
		if !seen[m.ID] {
			opts = append(opts, m.ID)
			seen[m.ID] = true
		}
	}

	// Canonical category per instrument, plus the ordered taxonomy filtered to
	// categories that actually have at least one instrument.
	dv.instCatByID = map[instrument.ID]audio.Category{}
	usedCats := map[audio.Category]bool{}
	for _, id := range opts {
		cat := audio.CategoryOf(id)
		dv.instCatByID[id] = cat
		usedCats[cat] = true
	}
	cats := []audio.Category{}
	for _, c := range audio.Categories() {
		if usedCats[c] {
			cats = append(cats, c)
		}
	}
	dv.instCategories = cats
	if len(dv.instCategories) > 0 && dv.instMenuActiveCat == audio.CatNone {
		dv.instMenuActiveCat = dv.instCategories[0]
	}

	prevSet := map[instrument.ID]bool{}
	for _, id := range dv.instOptions {
		prevSet[id] = true
	}

	dv.instMu.Lock()
	dv.instAvail = map[instrument.ID]bool{}
	for _, id := range opts {
		dv.instAvail[id] = true // known catalog or registered
	}
	for _, id := range audio.Instruments() {
		dv.instAvail[id] = true // registered playable
	}
	// Initialize missing map if needed
	if dv.missingInst == nil {
		dv.missingInst = map[instrument.ID]bool{}
	}
	// Remove from missing any IDs that just became available
	for id := range dv.missingInst {
		if dv.instAvail[id] {
			delete(dv.missingInst, id)
		}
	}
	// Build options: available first, then missing IDs (sorted for stability).
	combined := make([]instrument.ID, 0, len(opts)+len(dv.missingInst))
	combined = append(combined, opts...)
	if len(dv.missingInst) > 0 {
		var missing []instrument.ID
		for id := range dv.missingInst {
			if !dv.instAvail[id] {
				missing = append(missing, id)
			}
		}
		slices.Sort(missing)
		combined = append(combined, missing...)
	}
	changed := !slices.Equal(combined, dv.instOptions)
	if changed {
		// Bias only when a newly registered/playable instrument appears.
		for _, id := range combined {
			if !prevSet[id] {
				dv.instMenuLastAdded = id
				break
			}
		}
		if os.Getenv("BEATMO_DEBUG_INST") == "1" {
			fmt.Printf("[instMenu/refresh] changed=true lastAdded=%s opts=%d->%d\n", dv.instMenuLastAdded, len(dv.instOptions), len(combined))
		}
		// Invalidate label caches since instrument list or metadata changed.
		dv.invalidateLabelCaches()
	}
	dv.instOptions = combined
	dv.instMu.Unlock()
	// Update row label styles to reflect new availability.
	for i := range dv.Rows {
		if i < len(dv.rowLabels()) {
			id := dv.Rows[i].Instrument
			if dv.IsInstrumentAvailable(id) {
				dv.rowLabels()[i].Style = InstButtonStyle
			} else {
				dv.rowLabels()[i].Style = MissingInstStyle
			}
		}
	}
	if changed && dv.IsInstMenuOpen() {
		dv.refreshInstMenuComponent()
	}
}

// IsInstrumentAvailable reports whether an instrument id is currently available.
func (dv *DrumView) IsInstrumentAvailable(id instrument.ID) bool {
	if id == "" {
		return false
	}
	dv.instMu.RLock()
	ok := dv.instAvail != nil && dv.instAvail[id]
	dv.instMu.RUnlock()
	return ok
}

// EnsureInstrumentKnown records an instrument id so it appears in menus even
// when not currently available (e.g., imported from another machine). Once the
// instrument becomes available, it will be removed from the missing list.
func (dv *DrumView) EnsureInstrumentKnown(id instrument.ID) {
	if id == "" {
		return
	}
	// If already available, nothing to do.
	if dv.IsInstrumentAvailable(id) {
		return
	}
	dv.instMu.Lock()
	if dv.missingInst == nil {
		dv.missingInst = map[instrument.ID]bool{}
	}
	dv.missingInst[id] = true
	dv.instMu.Unlock()
	dv.instRefreshDirty = true
	dv.refreshInstruments()
}

// SetInstrumentSearch sets a case-insensitive substring filter applied to the
// instrument list (IDs, names, or relative paths). It rebuilds the menu if open.
func (dv *DrumView) SetInstrumentSearch(q string) {
	dv.instSearch = q
	if dv.IsInstMenuOpen() {
		dv.refreshInstMenuComponent()
	}
}

// refreshInstMenuComponent rebuilds the open instrument menu after a
// catalog mutation (rename, registration, search filter, refresh).
// No-op when the menu is closed or the component isn't constructed.
func (dv *DrumView) refreshInstMenuComponent() {
	if dv == nil || dv.instMenuComp == nil {
		return
	}
	if !dv.instMenuComp.IsOpen() {
		return
	}
	var instOpts []InstrumentOption
	for _, id := range dv.instOptions {
		label := string(id)
		if dv.instLabelCache != nil {
			if l, ok := dv.instLabelCache[id]; ok {
				label = l
			}
		}
		cat := audio.CatNone
		if dv.instCatByID != nil {
			cat = dv.instCatByID[id]
		}
		instOpts = append(instOpts, InstrumentOption{
			ID:       id,
			Label:    label,
			Category: cat,
			Color:    dv.instrumentRowColor(id),
		})
	}
	props := dv.instMenuComp.Props()
	props.Instruments = instOpts
	props.Categories = dv.instCategories
	dv.instMenuComp.SetProps(props)
	dv.instMenuComp.Refresh()
	dv.syncInstMenuScrollFromComp()
}
