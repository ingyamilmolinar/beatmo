//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestMobileTransportNoVolViewMenuCaptions guarantees the mobile transport
// bar never renders the "Vol", "View", or "Menu" text captions that used
// to overlap the corresponding icon buttons on the secondary row. The
// captions were deleted because they overlapped their icons in tight
// cells; this test prevents reintroduction.
//
// The test exploits the fact that text rendering in this package is
// routed through TextSprite(), which caches by exact string. Any text
// drawn during the transport render lands as a key in textSprites.
func TestMobileTransportNoVolViewMenuCaptions(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	ClearTextCacheForTest()

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 390, 96))

	// Render via the cache path (matches the production renderToolbar entry).
	dst := ebiten.NewImage(390, 96)
	z.renderToolbarDirect(dst)

	textCacheMu.RLock()
	defer textCacheMu.RUnlock()
	for _, banned := range []string{"Vol", "View", "Menu"} {
		if _, ok := textSprites[banned]; ok {
			t.Errorf("mobile transport rendered the banned caption %q — drawMobileRowLabelsOffset must stay deleted", banned)
		}
	}
}
