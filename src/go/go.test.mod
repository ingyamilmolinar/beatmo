module github.com/ingyamilmolinar/tunkul

go 1.23

toolchain go1.23.8

require (
	github.com/hajimehoshi/ebiten/v2 v2.8.6
	github.com/hajimehoshi/ebiten/v2/ebitenutil v0.0.0-00010101000000-000000000000
)

replace github.com/hajimehoshi/ebiten/v2 => ./internal/ebitestub/ebiten

replace github.com/hajimehoshi/ebiten/v2/ebitenutil => ./internal/ebitestub/ebitenutil
