//go:build js

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// pushLandscapeNoticeText sends the localized rotate-to-portrait notice text to
// the DOM overlay (#landscape-block) defined in index.html. The overlay's
// VISIBILITY is pure CSS (orientation media query); Go only owns the text so it
// stays in the active language (single i18n source). Safe to call before the
// setter exists (no-op until index.html registers it).
func pushLandscapeNoticeText() {
	fn := js.Global().Get("__beatmoSetLandscapeText")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(i18n.T(i18n.KeyOrientationTitle), i18n.T(i18n.KeyOrientationBody))
}

// registerLandscapeNoticeLocaleSync pushes the notice text once now and re-pushes
// on every locale change so the overlay tracks the in-app language.
func registerLandscapeNoticeLocaleSync() {
	pushLandscapeNoticeText()
	i18n.OnChange(pushLandscapeNoticeText)
}
