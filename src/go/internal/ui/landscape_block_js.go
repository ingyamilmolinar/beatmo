//go:build js

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Visibility is pure CSS (an orientation query); Go owns only the text, so it stays in the active language. Safe
// before index.html registers the setter.
func pushLandscapeNoticeText() {
	fn := js.Global().Get("__beatmoSetLandscapeText")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(i18n.T(i18n.KeyOrientationTitle), i18n.T(i18n.KeyOrientationBody))
}

func registerLandscapeNoticeLocaleSync() {
	pushLandscapeNoticeText()
	i18n.OnChange(pushLandscapeNoticeText)
}
