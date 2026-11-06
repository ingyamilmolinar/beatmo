package ui

import (
	"image/color"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// Runtime theme engine — rebuild-in-place.
//
// The generated genColor* variables ARE the live palette. ApplyTheme resolves
// a document to a full token map, overwrites the variables by name, re-runs
// the two derivation steps that otherwise happen once at init (component
// specs, then theme.go aliases/styles), and notifies listeners so caches that
// baked colors can invalidate (Game.OnThemeChanged). Draw sites keep reading
// the same variables: a theme costs nothing per frame.
//
// Always call ApplyTheme from the UI goroutine (it mutates package state read
// by Draw).
var (
	themeMu        sync.Mutex
	themeActive    theme.Doc
	themeGen       int
	themeListeners []*func()
	// themeWarnf receives non-fatal resolution warnings (unknown swatch,
	// fixed token…). Game rebinds it to its logger.
	themeWarnf = func(string, ...any) {}
)

// ApplyTheme makes doc the active theme. Unknown or fixed entries are skipped
// and logged through themeWarnf.
func ApplyTheme(doc theme.Doc) {
	tokens, warns := resolveThemeTokens(doc)
	for _, w := range warns {
		themeWarnf("[theme] %s", w)
	}
	themeMu.Lock()
	themeCaptureMu.Lock()
	recordCaptureValuesLocked() // remember what widgets may have captured so far
	themeCaptureMu.Unlock()
	for name, c := range tokens {
		if p, ok := genColorByName[name]; ok {
			*p = c
		}
	}
	rebuildComponentSpecs()
	rebuildThemeAliases()
	themeActive = doc.Clone()
	themeGen++
	listeners := append([]*func(){}, themeListeners...)
	themeMu.Unlock()
	for _, fn := range listeners {
		(*fn)()
	}
}

// ActiveTheme returns a copy of the last applied document.
func ActiveTheme() theme.Doc {
	themeMu.Lock()
	defer themeMu.Unlock()
	return themeActive.Clone()
}

// ThemeGeneration increments on every ApplyTheme (cache keys may include it).
func ThemeGeneration() int {
	themeMu.Lock()
	defer themeMu.Unlock()
	return themeGen
}

// OnThemeChange registers fn to run after every ApplyTheme; cancel removes it.
func OnThemeChange(fn func()) (cancel func()) {
	p := &fn
	themeMu.Lock()
	themeListeners = append(themeListeners, p)
	themeMu.Unlock()
	return func() {
		themeMu.Lock()
		defer themeMu.Unlock()
		for i, q := range themeListeners {
			if q == p {
				themeListeners = append(themeListeners[:i], themeListeners[i+1:]...)
				return
			}
		}
	}
}

// SetThemeForTest applies doc and returns a restore func that reapplies the
// previous document. Tests must call restore so the package stays on the
// default palette for the drift/discipline suites.
func SetThemeForTest(doc theme.Doc) (restore func()) {
	prev := ActiveTheme()
	ApplyTheme(doc)
	return func() { ApplyTheme(prev) }
}

// ── Captured token colors ────────────────────────────────────────────────
//
// Dozens of call sites copy a token into a widget at construction
// (`b.IconColor = colTextSecondary`, profile fields, per-frame record tints).
// When a theme is applied LATER (a circuit theme on import, an editor pick)
// those copies would keep the previous value. liveCaptured maps a captured
// opaque color back to the token it was copied from and returns that token's
// CURRENT value; colors that never were a chrome token pass through.
//
// Only chrome tokens are remapped, in priority order, so a hex shared by a
// text/accent token and a visualizer token (e.g. primary == viz-bar) resolves
// to the chrome one — every capture site in the codebase copies a chrome
// token (text, accent, semantic state, border, surface).
var themeCaptureTokens = []string{
	"on-surface", "on-surface-muted", "on-surface-disabled", "on-surface-accent",
	"primary", "primary-bright", "primary-dim",
	"record-idle", "record-active", "error", "error-text", "success", "mute", "destructive",
	"border", "surface-1", "surface-2", "surface-3", "background",
}

var (
	themeCaptureMu  sync.RWMutex
	themeCaptureMap map[color.RGBA]string // a value a token has HELD → token name
)

// recordCaptureValuesLocked snapshots the current value of every chrome
// token (called before tokens are overwritten, and lazily for the defaults).
// Earlier mappings win so a value keeps pointing at the token that first
// held it.
func recordCaptureValuesLocked() {
	if themeCaptureMap == nil {
		themeCaptureMap = map[color.RGBA]string{}
		for _, name := range themeCaptureTokens {
			if c, ok := genColorDefaultByName[name]; ok {
				if _, dup := themeCaptureMap[c]; !dup {
					themeCaptureMap[c] = name
				}
			}
		}
	}
	for _, name := range themeCaptureTokens {
		if p, ok := genColorByName[name]; ok {
			if _, dup := themeCaptureMap[*p]; !dup {
				themeCaptureMap[*p] = name
			}
		}
	}
}

// liveCaptured returns the current value of the token c was captured from,
// or c itself when it is nil, translucent, or not a chrome token value.
func liveCaptured(c color.Color) color.Color {
	if c == nil {
		return nil
	}
	themeCaptureMu.RLock()
	m := themeCaptureMap
	themeCaptureMu.RUnlock()
	if m == nil {
		return c
	}
	var key color.RGBA
	switch v := c.(type) {
	case color.RGBA:
		if v.A != 255 {
			return c
		}
		key = v
	case color.NRGBA:
		if v.A != 255 {
			return c
		}
		key = color.RGBAModel.Convert(v).(color.RGBA) // exact for opaque NRGBA
	default:
		return c
	}
	name, ok := m[key]
	if !ok {
		return c
	}
	if p, ok := genColorByName[name]; ok {
		return *p
	}
	return c
}
