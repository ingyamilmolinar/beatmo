//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFileOpsIconStrokeThicknessAtTypicalSize verifies that file ops icons
// (upload, import, export) render strokes thick enough to be visually
// distinguishable at typical desktop button sizes. At 28-36px buttons,
// a 2px stroke is nearly invisible; strokes should be >= 3px.
func TestFileOpsIconStrokeThicknessAtTypicalSize(t *testing.T) {
	assertDefaultParityState(t)

	// Typical desktop file ops button: ~30x28 after transport layout padding.
	// The Button.Draw applies 18% inset, then icons apply another /5 inset.
	// We test the icon functions directly with the box they'd receive.
	dim := 30
	pad := dim * 18 / 100 // 5
	box := image.Rect(pad, pad, dim-pad, dim-pad) // 5,5 to 25,25 = 20x20

	icons := map[string]func(*ebiten.Image, image.Rectangle, color.Color){
		"upload": drawUploadIcon,
		"import": drawImportIcon,
		"export": drawExportIcon,
	}

	for name, drawFn := range icons {
		t.Run(name, func(t *testing.T) {
			dst := ebiten.NewImage(dim, dim)
			var minStrokeW, minStrokeH int
			minStrokeW = 999
			minStrokeH = 999

			orig := drawRect
			drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
				if !filled {
					orig(d, r, c, filled)
					return
				}
				w := r.Dx()
				h := r.Dy()
				// Track minimum stroke dimensions (exclude full-width base lines).
				if w > 0 && w < box.Dx() && w < minStrokeW {
					minStrokeW = w
				}
				if h > 0 && h < box.Dy() && h < minStrokeH {
					minStrokeH = h
				}
				orig(d, r, c, filled)
			}
			defer func() { drawRect = orig }()

			drawFn(dst, box, color.White)

			// Strokes should be at least 3px in both dimensions for visibility.
			const minThick = 3
			if minStrokeW < minThick {
				t.Errorf("icon %q min stroke width %d < %d at %dx%d box", name, minStrokeW, minThick, box.Dx(), box.Dy())
			}
			if minStrokeH < minThick {
				t.Errorf("icon %q min stroke height %d < %d at %dx%d box", name, minStrokeH, minThick, box.Dx(), box.Dy())
			}
		})
	}
}

// TestChevronIconStrokeThicknessAtTypicalSize verifies BPM chevron icons
// have sufficient stroke thickness at typical button sizes.
func TestChevronIconStrokeThicknessAtTypicalSize(t *testing.T) {
	assertDefaultParityState(t)

	dim := 28
	pad := dim * 18 / 100
	box := image.Rect(pad, pad, dim-pad, dim-pad)

	icons := map[string]func(*ebiten.Image, image.Rectangle, color.Color){
		"chevron-up":   drawChevronUpIcon,
		"chevron-down": drawChevronDownIcon,
	}

	for name, drawFn := range icons {
		t.Run(name, func(t *testing.T) {
			dst := ebiten.NewImage(dim, dim)
			var minStroke int
			minStroke = 999

			orig := drawRect
			drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
				if filled {
					w := r.Dx()
					h := r.Dy()
					s := w
					if h < s {
						s = h
					}
					if s > 0 && s < minStroke {
						minStroke = s
					}
				}
				orig(d, r, c, filled)
			}
			defer func() { drawRect = orig }()

			drawFn(dst, box, color.White)

			const minThick = 3
			if minStroke < minThick {
				t.Errorf("icon %q min stroke %d < %d at %dx%d box", name, minStroke, minThick, box.Dx(), box.Dy())
			}
		})
	}
}
