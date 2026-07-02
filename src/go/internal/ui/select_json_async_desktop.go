//go:build !js && !test

package ui

import (
	"context"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// selectJSONAsync routes the desktop file picker through the bounded
// "ui.dialog" pool so user-initiated dialogs cannot spawn unbounded
// goroutines if the picker stalls or the user spams the button.
func selectJSONAsync(cb func([]byte, string, error)) {
	err := async.Go("ui.dialog", func(_ context.Context) {
		d, name, e := selectJSON()
		cb(d, name, e)
	})
	if err != nil {
		// Pool saturated or shutting down — surface the error so the
		// caller's UI gates (importing/uploading flags) clear instead
		// of staying stuck waiting for a callback that never arrives.
		cb(nil, "", err)
	}
}
