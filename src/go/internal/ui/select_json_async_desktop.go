//go:build !js && !test

package ui

import (
	"context"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// The bounded ui.dialog pool, so a stalled picker or a spammed button cannot spawn unbounded goroutines.
func selectJSONAsync(cb func([]byte, string, error)) {
	err := async.Go("ui.dialog", func(_ context.Context) {
		d, name, e := selectJSON()
		cb(d, name, e)
	})
	if err != nil {
		// Report the error so the caller's importing/uploading gates clear instead of waiting forever.
		cb(nil, "", err)
	}
}
