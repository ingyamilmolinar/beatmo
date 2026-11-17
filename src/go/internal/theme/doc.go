// Package theme holds the runtime UI color-scheme document. It is a plain
// data package (no Ebiten, no ui import) so userprefs, the project file, and
// the ui package can all carry a Doc without an import cycle.
package theme

// Doc is a config-only UI color scheme.
//
//   - Categories: category id (see ui/theme_categories.go) → swatch NAME from
//     the Vice City grid (e.g. "sunset-gold-300"). Names, never hex.
//   - Tokens: DESIGN.md color token name → "#RRGGBB". Escape hatch for
//     hand-authored config; applied after category derivation.
//
// Both maps are partial and omitempty. An empty Doc is the shipped design.
type Doc struct {
	// Preset is the id of a first-party theme (ui/theme_presets.go); its
	// category picks are applied BEFORE Categories/Tokens, which may then
	// customize it. Empty = the shipped design.
	Preset     string            `json:"preset,omitempty"`
	Categories map[string]string `json:"categories,omitempty"`
	Tokens     map[string]string `json:"tokens,omitempty"`
}

// UserDoc is the per-user profile theme plus the override flag.
type UserDoc struct {
	Doc
	// OverrideCircuits, when true, makes the user's categories win over a
	// circuit's own theme for every category the user has set.
	OverrideCircuits bool `json:"override_circuits,omitempty"`
}

// IsEmpty reports whether the document sets nothing.
func (d Doc) IsEmpty() bool {
	return d.Preset == "" && len(d.Categories) == 0 && len(d.Tokens) == 0
}

// Clone deep-copies both maps (nil stays nil).
func (d Doc) Clone() Doc {
	out := Doc{Preset: d.Preset}
	if d.Categories != nil {
		out.Categories = make(map[string]string, len(d.Categories))
		for k, v := range d.Categories {
			out.Categories[k] = v
		}
	}
	if d.Tokens != nil {
		out.Tokens = make(map[string]string, len(d.Tokens))
		for k, v := range d.Tokens {
			out.Tokens[k] = v
		}
	}
	return out
}

// Equal compares map contents (nil and empty are equal).
func (d Doc) Equal(o Doc) bool {
	return d.Preset == o.Preset && eqMap(d.Categories, o.Categories) && eqMap(d.Tokens, o.Tokens)
}

// Clone deep-copies the user document.
func (u UserDoc) Clone() UserDoc {
	return UserDoc{Doc: u.Doc.Clone(), OverrideCircuits: u.OverrideCircuits}
}

func eqMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
