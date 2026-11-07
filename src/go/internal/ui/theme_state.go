package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// Per category and per token: override && user → user; else circuit; else user; else the default.
func effectiveTheme(user theme.UserDoc, circuit theme.Doc) theme.Doc {
	pick := func(u, c map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range u {
			out[k] = v
		}
		for k, v := range c {
			if uv, ok := u[k]; ok && user.OverrideCircuits {
				out[k] = uv
				continue
			}
			out[k] = v
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return theme.Doc{
		Preset:     effectivePresetID(user, circuit),
		Categories: pick(user.Categories, circuit.Categories),
		Tokens:     pick(user.Tokens, circuit.Tokens),
	}
}

// effectivePresetID applies the category precedence rule to the preset id.
func effectivePresetID(user theme.UserDoc, circuit theme.Doc) string {
	switch {
	case user.OverrideCircuits && user.Preset != "":
		return user.Preset
	case circuit.Preset != "":
		return circuit.Preset
	default:
		return user.Preset
	}
}

// Customisation is dropped so the preset name stays truthful.
func (g *Game) setUserThemePreset(id string) {
	g.userTheme.Doc = theme.Doc{Preset: id}
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged(hooks.ThemeScopeUser, "preset", id)
}

// setCircuitThemePreset makes id the circuit theme (undoable document edit).
func (g *Game) setCircuitThemePreset(id string) {
	g.circuitTheme = theme.Doc{Preset: id}
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged("preset", id)
}

// So a pick is never shadowed by a stale hand-authored hex.
func dropTokensOfCategory(d *theme.Doc, id string) {
	for tok := range d.Tokens {
		if owner, ok := themeCategoryOfToken(tok); ok && owner == id {
			delete(d.Tokens, tok)
		}
	}
}

// Applies only on a real change: importing an unthemed circuit must not cost a cache-rebuild burst.
func (g *Game) recomputeEffectiveTheme() {
	eff := effectiveTheme(g.userTheme, g.circuitTheme)
	if eff.Equal(ActiveTheme()) {
		return
	}
	ApplyTheme(eff)
}

// UserTheme returns a copy of the profile theme + override flag.
func (g *Game) UserTheme() theme.UserDoc { return g.userTheme.Clone() }

// CircuitTheme returns a copy of the open circuit's theme.
func (g *Game) CircuitTheme() theme.Doc { return g.circuitTheme.Clone() }

// SetCircuitTheme replaces the circuit's theme (import / undo restore path).
func (g *Game) SetCircuitTheme(d theme.Doc) {
	g.circuitTheme = d.Clone()
	g.recomputeEffectiveTheme()
}

func (g *Game) setUserThemeCategory(id, swatch string) {
	if g.userTheme.Categories == nil {
		g.userTheme.Categories = map[string]string{}
	}
	g.userTheme.Categories[id] = swatch
	dropTokensOfCategory(&g.userTheme.Doc, id)
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged(hooks.ThemeScopeUser, id, swatch)
}

func (g *Game) clearUserThemeCategory(id string) {
	delete(g.userTheme.Categories, id)
	dropTokensOfCategory(&g.userTheme.Doc, id)
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged(hooks.ThemeScopeUser, id, "")
}

func (g *Game) resetUserTheme() {
	g.userTheme.Doc = theme.Doc{}
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged(hooks.ThemeScopeUser, "", "")
}

func (g *Game) setThemeOverride(on bool) {
	g.userTheme.OverrideCircuits = on
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeOverrideChanged(on)
}

func (g *Game) setCircuitThemeCategory(id, swatch string) {
	if g.circuitTheme.Categories == nil {
		g.circuitTheme.Categories = map[string]string{}
	}
	g.circuitTheme.Categories[id] = swatch
	dropTokensOfCategory(&g.circuitTheme, id)
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged(id, swatch)
}

func (g *Game) clearCircuitThemeCategory(id string) {
	delete(g.circuitTheme.Categories, id)
	dropTokensOfCategory(&g.circuitTheme, id)
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged(id, "")
}

func (g *Game) resetCircuitTheme() {
	g.circuitTheme = theme.Doc{}
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged("", "")
}
