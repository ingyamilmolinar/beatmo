package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Render boxes spec.Fill and the resolved border into color.Color (+2 allocs), negligible at hundreds of surfaces per
// frame. Skipped on WASM: the stub has no off-screen images.
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

// adjustColor runs twice, fill and border.
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

// The baseline to compare BenchmarkRender against.
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
