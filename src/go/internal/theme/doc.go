// Package theme is the runtime color-scheme document, plain data, so userprefs, the project file and ui can all carry
// it without an import cycle.
package theme

// Categories map a category id to a swatch NAME (never hex); Tokens map a DESIGN.md token to "#RRGGBB" (an escape hatch,
// applied after the categories). Both partial; an empty Doc is the shipped design.
type Doc struct {
	// Applied BEFORE Categories/Tokens, which may then customise it. Empty = the shipped design.
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
