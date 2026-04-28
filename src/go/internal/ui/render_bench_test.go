package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// BenchmarkRender measures per-call allocation of the stateless Render
// primitive. Phase 4 baseline (BenchmarkButtonStyleDraw): ~2 allocs/op,
// ~8 B/op — those come from inside the drawButton closure's body.
// Render adds 2 more (~16 B/op) because spec.Fill (color.RGBA) and
// spec.Border.Resolve() (color.NRGBA) box to color.Color when passed to
// drawButton, whereas ButtonStyle.Fill / Border are already color.Color.
// At realistic UI render rates (~hundreds of chrome surfaces/frame at
// 60 FPS) this is single-digit kilobytes/sec — well below GC noise.
// The fast-path branch in Render avoids the slow-path local vars when
// no interaction delta applies.
//
// Skipped on WASM (no off-screen image support in the stub).
func BenchmarkRender(b *testing.B) {
	dst := ebiten.NewImage(64, 32)
	r := image.Rect(0, 0, 64, 32)
	spec := Spec(ComponentButtonSecondary)
	state := ComponentState{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(dst, r, spec, state)
	}
}

// BenchmarkRenderHovered exercises the hover-delta path (where adjustColor
// is invoked twice — fill + border).
func BenchmarkRenderHovered(b *testing.B) {
	dst := ebiten.NewImage(64, 32)
	r := image.Rect(0, 0, 64, 32)
	spec := Spec(ComponentButtonSecondary)
	state := ComponentState{Hovered: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(dst, r, spec, state)
	}
}

// BenchmarkRenderFocused exercises the focus path (delta + accent ring).
func BenchmarkRenderFocused(b *testing.B) {
	dst := ebiten.NewImage(64, 32)
	r := image.Rect(0, 0, 64, 32)
	spec := Spec(ComponentInputField)
	state := ComponentState{Focused: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(dst, r, spec, state)
	}
}

// BenchmarkButtonStyleDraw is the pre-refactor baseline. Compare against
// BenchmarkRender to verify Phase 4 doesn't regress per-frame allocation.
func BenchmarkButtonStyleDraw(b *testing.B) {
	dst := ebiten.NewImage(64, 32)
	r := image.Rect(0, 0, 64, 32)
	style := TransportPlayStyle
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		style.Draw(dst, r, false, false)
	}
}

