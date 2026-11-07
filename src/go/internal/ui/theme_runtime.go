package ui

import (
	"image/color"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// The genColor* vars ARE the live palette: ApplyTheme overwrites them, re-runs the spec and alias derivations
// and notifies caches, so a theme costs nothing per frame. UI goroutine only: Draw reads the state.
var (
	themeMu        sync.Mutex
	themeActive    theme.Doc
	themeGen       int
	themeListeners []*func()
	// Game rebinds it to its logger.
	themeWarnf = func(string, ...any) {}
)

// Unknown or fixed entries are skipped and logged.
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

// Tests must restore, so the drift/discipline suites see the default palette.
func SetThemeForTest(doc theme.Doc) (restore func()) {
	prev := ActiveTheme()
	ApplyTheme(doc)
	return func() { ApplyTheme(prev) }
}

// Call sites copy tokens at construction, which a later theme would leave stale; liveCaptured maps such a copy back
// to its token's CURRENT value. Chrome tokens only, in priority order, so a hex shared with a viz token resolves to chrome.
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

// Earlier mappings win, so a value keeps pointing at the token that first held it.
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

// c itself when nil, translucent or not a chrome token value.
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
