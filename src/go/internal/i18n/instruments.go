package i18n

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type builtinInstrumentDefault struct {
	canonicalEN string
	key         Key
}

var builtinInstruments = map[instrument.ID]builtinInstrumentDefault{
	roster.Kick:  {"Kick", KeyInstKick},
	roster.Snare: {"Snare", KeyInstSnare},
	roster.Hihat: {"Hi-Hat", KeyInstHiHat},
	roster.Clap:  {"Clap", KeyInstClap},
	roster.Tom:   {"Tom", KeyInstTom},
}

// InstrumentDisplayName returns the name to show for an instrument. When stored
// equals the builtin's canonical English default (or is empty), the localized
// default is returned; otherwise stored is returned verbatim (user rename).
func InstrumentDisplayName(id instrument.ID, stored string) string {
	def, ok := builtinInstruments[id]
	if !ok {
		return stored
	}
	if stored == "" || stored == def.canonicalEN {
		return T(def.key)
	}
	return stored
}

// InstrumentNames lists every name a shipped instrument goes by: its English
// default and its name in each locale (nothing for other instruments).
func InstrumentNames(id instrument.ID) []string {
	def, ok := builtinInstruments[id]
	if !ok {
		return nil
	}
	out := []string{def.canonicalEN}
	for _, l := range AllLocales() {
		if v, ok := catalogFor(l)[def.key]; ok {
			out = append(out, v)
		}
	}
	return out
}
