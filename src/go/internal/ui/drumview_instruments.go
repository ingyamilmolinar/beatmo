package ui

import (
	"fmt"
	"os"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

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
	dv.instCatByID = map[string]string{}
	dv.instMeta = map[string]audio.SoundMeta{}
	for _, m := range meta {
		dv.instCatByID[m.ID] = m.Category
		dv.instMeta[m.ID] = m
	}
	cats := audio.CatalogCategories()
	fallbackCat := fallbackInstCategory

	seen := map[string]bool{}
	opts := []string{}
	// Registered (built-in or already loaded) instruments first to keep legacy ordering.
	for _, id := range audio.Instruments() {
		if !seen[id] {
			opts = append(opts, id)
			seen[id] = true
		}
	}
	// Catalog entries in their discovered order.
	for _, m := range meta {
		if !seen[m.ID] {
			opts = append(opts, m.ID)
			seen[m.ID] = true
		}
	}

	// Assign a fallback category to any instrument that lacks catalog metadata
	// so category mode never renders empty and built-ins remain reachable.
	uncat := 0
	for _, id := range opts {
		if _, ok := dv.instCatByID[id]; !ok {
			dv.instCatByID[id] = fallbackCat
			uncat++
		}
	}
	if uncat > 0 {
		hasFallback := false
		for _, c := range cats {
			if c == fallbackCat {
				hasFallback = true
				break
			}
		}
		if !hasFallback {
			cats = append(cats, fallbackCat)
		}
	}
	// Deterministic ordering keeps tests stable.
	slices.Sort(cats)
	dv.instCategories = cats
	if len(dv.instCategories) > 0 && dv.instMenuActiveCat == "" {
		dv.instMenuActiveCat = dv.instCategories[0]
	}

	prevSet := map[string]bool{}
	for _, id := range dv.instOptions {
		prevSet[id] = true
	}

	dv.instMu.Lock()
	dv.instAvail = map[string]bool{}
	for _, id := range opts {
		dv.instAvail[id] = true // known catalog or registered
	}
	for _, id := range audio.Instruments() {
		dv.instAvail[id] = true // registered playable
	}
	// Initialize missing map if needed
	if dv.missingInst == nil {
		dv.missingInst = map[string]bool{}
	}
	// Remove from missing any IDs that just became available
	for id := range dv.missingInst {
		if dv.instAvail[id] {
			delete(dv.missingInst, id)
		}
	}
	// Build options: available first, then missing IDs (sorted for stability).
	combined := make([]string, 0, len(opts)+len(dv.missingInst))
	combined = append(combined, opts...)
	if len(dv.missingInst) > 0 {
		var missing []string
		for id := range dv.missingInst {
			if !dv.instAvail[id] {
				missing = append(missing, id)
			}
		}
		slices.Sort(missing)
		combined = append(combined, missing...)
	}
	// Optional custom ordering override.
	if len(dv.instOrderIndex) > 0 {
		slices.SortStableFunc(combined, func(a, b string) int {
			ia, oka := dv.instOrderIndex[a]
			ib, okb := dv.instOrderIndex[b]
			switch {
			case oka && okb:
				return ia - ib
			case oka:
				return -1
			case okb:
				return 1
			default:
				return 0
			}
		})
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
		dv.buildInstMenu()
	}
}

// IsInstrumentAvailable reports whether an instrument id is currently available.
func (dv *DrumView) IsInstrumentAvailable(id string) bool {
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
func (dv *DrumView) EnsureInstrumentKnown(id string) {
	if id == "" {
		return
	}
	// If already available, nothing to do.
	if dv.IsInstrumentAvailable(id) {
		return
	}
	dv.instMu.Lock()
	if dv.missingInst == nil {
		dv.missingInst = map[string]bool{}
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
		dv.buildInstMenu()
	}
}

// SetInstrumentOrder allows callers to impose a custom ordering for instruments.
// IDs not present in the order slice keep their existing relative order.
func (dv *DrumView) SetInstrumentOrder(order []string) {
	dv.instOrderIndex = map[string]int{}
	for i, id := range order {
		dv.instOrderIndex[id] = i
	}
	dv.instRefreshDirty = true
	dv.refreshInstruments()
}
