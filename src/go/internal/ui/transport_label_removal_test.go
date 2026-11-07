//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The captions overlapped their icons in tight cells. Every string drawn goes through TextSprite's cache, so its
// keys show what was rendered.
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
		if _, ok := textSprites[textKey{s: banned, gen: i18n.FontGeneration()}]; ok {
			t.Errorf("mobile transport rendered the banned caption %q — drawMobileRowLabelsOffset must stay deleted", banned)
		}
	}
}
