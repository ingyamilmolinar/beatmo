package audio

import (
	"path"
	"strings"
	"sync"
)

// instNameOverrides holds user-set display-name overrides keyed by the
// immutable instrument ID. Renaming an instrument writes here and nowhere
// else — the ID never moves, so no other map needs re-keying.
var (
	instNameMu        sync.RWMutex
	instNameOverrides = map[string]string{}
)

// SetInstrumentDisplayName records a display-name override for an instrument
// ID. An empty name clears the override (the instrument falls back to its
// catalog/pretty default). The ID is unchanged — this is pure metadata.
func SetInstrumentDisplayName(id, name string) {
	if id == "" {
		return
	}
	instNameMu.Lock()
	if name == "" {
		delete(instNameOverrides, id)
	} else {
		instNameOverrides[id] = name
	}
	instNameMu.Unlock()
}

// ClearInstrumentDisplayName removes any override for the instrument ID.
func ClearInstrumentDisplayName(id string) {
	instNameMu.Lock()
	delete(instNameOverrides, id)
	instNameMu.Unlock()
}

// clearInstrumentDisplayNames drops every display-name override. ResetInstruments
// calls it so a registry reset also restores default instrument names — and so
// the process-global override store cannot leak across tests (a rename/import in
// one test would otherwise change a builtin's label for every later test).
func clearInstrumentDisplayNames() {
	instNameMu.Lock()
	instNameOverrides = map[string]string{}
	instNameMu.Unlock()
}

// InstrumentDisplayName resolves the user-facing label for an instrument ID.
// Resolution order: user override → catalog SoundMeta.Name → catalog RelPath
// basename (pretty-cased) → PrettyName(id). This is the single source of
// truth for instrument labels across UI and non-UI callers.
func InstrumentDisplayName(id string) string {
	if id == "" {
		return ""
	}
	instNameMu.RLock()
	override, ok := instNameOverrides[id]
	instNameMu.RUnlock()
	if ok && override != "" {
		return override
	}
	if meta, ok := CatalogLookup(id); ok {
		if meta.Name != "" {
			return meta.Name
		}
		if meta.RelPath != "" {
			base := path.Base(meta.RelPath)
			base = strings.TrimSuffix(base, path.Ext(base))
			if base != "" {
				return PrettyName(base)
			}
		}
	}
	return PrettyName(id)
}
