package audio

import (
	"path"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Keyed by the immutable ID: a rename writes here and nowhere else, so no other map needs re-keying.
var (
	instNameMu        sync.RWMutex
	instNameOverrides = map[instrument.ID]string{}
)

// An empty name clears the override.
func SetInstrumentDisplayName(id instrument.ID, name string) {
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

func InstrumentDisplayNameOverride(id instrument.ID) string {
	instNameMu.RLock()
	defer instNameMu.RUnlock()
	return instNameOverrides[id]
}

// ClearInstrumentDisplayName removes any override for the instrument ID.
func ClearInstrumentDisplayName(id instrument.ID) {
	instNameMu.Lock()
	delete(instNameOverrides, id)
	instNameMu.Unlock()
}

// So the process-global override store cannot leak a rename across tests.
func clearInstrumentDisplayNames() {
	instNameMu.Lock()
	instNameOverrides = map[instrument.ID]string{}
	instNameMu.Unlock()
}

// The single source of instrument labels: override → catalog name → catalog file basename → PrettyName(id).
func InstrumentDisplayName(id instrument.ID) string {
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
	return PrettyName(string(id))
}
