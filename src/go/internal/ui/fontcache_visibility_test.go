//go:build !test

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fontTestGame is a minimal Ebiten game that runs font rendering assertions
// inside the game loop where GPU context is available for ReadPixels.
type fontTestGame struct {
	fn  func() map[string][]string // returns test name → error messages
	ran bool
	out map[string][]string
}

func (g *fontTestGame) Update() error {
	if g.ran {
		return ebiten.Termination
	}
	g.ran = true
	g.out = g.fn()
	return ebiten.Termination
}
func (g *fontTestGame) Draw(*ebiten.Image)         {}
func (g *fontTestGame) Layout(int, int) (int, int) { return 320, 240 }

// TestRenderTextSpriteVisibility verifies that renderTextSprite produces
// visible text correctly positioned in the sprite image.
//
// This test catches a historical bug where an extra GeoM.Translate by
// face.Metrics().HAscent pushed glyphs to the bottom of the sprite,
// leaving only ~2px of anti-aliased edges visible. Ebiten's text/v2
// already handles the ascent offset internally, so any additional
// translate double-counts it.
//
// The test runs in a subprocess because ebiten.RunGame can only be called
// once per process and poisons all subsequent NewImage calls. Running in
// a subprocess prevents this from breaking other tests in the package.
func TestRenderTextSpriteVisibility(t *testing.T) {
	if os.Getenv("FONTCACHE_SUBPROCESS") != "1" {
		// Re-exec this test in a subprocess so RunGame doesn't poison
		// the parent process's Ebiten state.
		cmd := exec.Command(os.Args[0], "-test.run=^TestRenderTextSpriteVisibility$", "-test.v")
		cmd.Env = append(os.Environ(), "FONTCACHE_SUBPROCESS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("subprocess failed:\n%s", out)
		}
		t.Log(string(out))
		return
	}

	// --- subprocess execution below ---
	game := &fontTestGame{fn: func() map[string][]string {
		results := map[string][]string{}

		// Test 1: Text starts above the vertical midpoint for all font sizes.
		sizes := []struct {
			name string
			size float64
		}{
			{"Caption", FontSizeCaption},
			{"Body", FontSizeBody},
			{"Label", FontSizeLabel},
			{"Title", FontSizeTitle},
			{"Heading", FontSizeHeading},
		}
		for _, tc := range sizes {
			key := "AboveMidpoint/" + tc.name
			errs := checkTextAboveMidpoint(tc.size)
			if len(errs) > 0 {
				results[key] = errs
			}
		}

		// Test 2: Text occupies a reasonable portion of sprite height.
		{
			key := "CoversReasonableHeight"
			errs := checkTextCoversHeight(FontSizeBody)
			if len(errs) > 0 {
				results[key] = errs
			}
		}

		return results
	}}

	if err := ebiten.RunGame(game); err != nil {
		t.Fatalf("RunGame failed: %v", err)
	}

	if len(game.out) == 0 {
		t.Log("all font rendering checks passed")
		return
	}
	for name, errs := range game.out {
		t.Errorf("[%s] %s", name, strings.Join(errs, "; "))
	}
}

// checkTextAboveMidpoint verifies that text rendered at the given size
// has its topmost visible pixel in the upper half of the sprite.
func checkTextAboveMidpoint(size float64) []string {
	// Use "Ag" to exercise both ascender (A) and descender (g).
	img := renderTextSprite("Ag", size, true)
	if img == nil {
		return []string{fmt.Sprintf("renderTextSprite returned nil for size %.0f", size)}
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return []string{fmt.Sprintf("sprite has zero dimensions: %dx%d", w, h)}
	}

	pix := make([]byte, w*h*4)
	img.ReadPixels(pix)

	topRow := -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pix[(y*w+x)*4+3] > 0 {
				topRow = y
				goto found
			}
		}
	}
found:
	if topRow < 0 {
		return []string{"no visible pixels in text sprite"}
	}

	midY := h / 2
	if topRow >= midY {
		return []string{fmt.Sprintf(
			"text starts at row %d, below sprite midpoint %d (height=%d); "+
				"likely a redundant ascent offset in renderTextSprite",
			topRow, midY, h)}
	}
	return nil
}

// checkTextCoversHeight verifies that rendered text occupies a meaningful
// portion of its sprite height, not just a thin sliver at the edge.
func checkTextCoversHeight(size float64) []string {
	img := renderTextSprite("Hg", size, true)
	if img == nil {
		return []string{"renderTextSprite returned nil"}
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	pix := make([]byte, w*h*4)
	img.ReadPixels(pix)

	topRow, botRow := -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pix[(y*w+x)*4+3] > 0 {
				if topRow < 0 {
					topRow = y
				}
				botRow = y
				break
			}
		}
	}
	if topRow < 0 {
		return []string{"no visible pixels in text sprite"}
	}

	textH := botRow - topRow + 1
	pct := float64(textH) / float64(h)
	// Text should occupy at least 40% of the sprite height.
	// With correct layout, "Hg" (ascender + descender) typically
	// covers 70-90% of the allocated height.
	if pct < 0.4 {
		return []string{fmt.Sprintf(
			"text occupies %d/%d rows (%.0f%%); expected >= 40%% of sprite height",
			textH, h, pct*100)}
	}
	return nil
}
