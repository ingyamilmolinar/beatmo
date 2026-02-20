module github.com/ingyamilmolinar/beatmo

go 1.23

toolchain go1.23.8

require (
	github.com/grafana/pyroscope-go v1.2.7
	github.com/hajimehoshi/ebiten/v2 v2.8.6
	github.com/hajimehoshi/ebiten/v2/ebitenutil v0.0.0-00010101000000-000000000000
	github.com/hajimehoshi/ebiten/v2/vector v0.0.0
)

require (
	github.com/grafana/pyroscope-go/godeltaprof v0.1.9 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
)

replace github.com/hajimehoshi/ebiten/v2 => ./internal/ebitestub/ebiten

replace github.com/hajimehoshi/ebiten/v2/ebitenutil => ./internal/ebitestub/ebitenutil

replace github.com/hajimehoshi/ebiten/v2/vector => ./internal/ebitestub/vector
