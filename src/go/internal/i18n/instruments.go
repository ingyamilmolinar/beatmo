package i18n

type builtinInstrumentDefault struct {
	canonicalEN string
	key         Key
}

var builtinInstruments = map[string]builtinInstrumentDefault{
	"kick":  {"Kick", KeyInstKick},
	"snare": {"Snare", KeyInstSnare},
	"hihat": {"Hi-Hat", KeyInstHiHat},
	"clap":  {"Clap", KeyInstClap},
	"tom":   {"Tom", KeyInstTom},
	"bass":  {"Bass", KeyInstBass},
}

// InstrumentDisplayName returns the name to show for an instrument. When stored
// equals the builtin's canonical English default (or is empty), the localized
// default is returned; otherwise stored is returned verbatim (user rename).
func InstrumentDisplayName(id, stored string) string {
	def, ok := builtinInstruments[id]
	if !ok {
		return stored
	}
	if stored == "" || stored == def.canonicalEN {
		return T(def.key)
	}
	return stored
}
