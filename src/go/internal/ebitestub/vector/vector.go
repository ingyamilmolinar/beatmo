// Package vector stubs Ebiten's vector package for the -tags test build: no rendering, so its helpers are no-ops that
// let icon-dispatch logic be tested without a GL context. Signatures must follow the Ebiten pinned in go.mod.
package vector

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// ── Direction (used by Path.Arc) ───────────────────────────────────────

// Direction selects whether Path.Arc sweeps clockwise or counter-clockwise.
type Direction int

const (
	Clockwise Direction = iota
	CounterClockwise
)

// ── Line caps and joins (used by StrokeOptions) ────────────────────────

// LineCap selects the cap style at the end of a stroked line.
type LineCap int

const (
	LineCapButt LineCap = iota
	LineCapRound
	LineCapSquare
)

// LineJoin selects the join style at vertices of a stroked path.
type LineJoin int

const (
	LineJoinMiter LineJoin = iota
	LineJoinBevel
	LineJoinRound
)

// StrokeOptions configures Path.AppendVerticesAndIndicesForStroke.
type StrokeOptions struct {
	Width      float32
	LineCap    LineCap
	LineJoin   LineJoin
	MiterLimit float32
}

// ── Path (path builder) ────────────────────────────────────────────────

// Records nothing: appending returns the input slices, so callers see no triangles.
type Path struct{}

// MoveTo starts a new subpath at (x, y). No-op in the stub.
func (p *Path) MoveTo(x, y float32) {}

// LineTo appends a line segment to (x, y). No-op in the stub.
func (p *Path) LineTo(x, y float32) {}

// QuadTo appends a quadratic Bézier curve. No-op in the stub.
func (p *Path) QuadTo(x1, y1, x2, y2 float32) {}

// CubicTo appends a cubic Bézier curve. No-op in the stub.
func (p *Path) CubicTo(x1, y1, x2, y2, x3, y3 float32) {}

// Arc appends a circular arc centered at (x, y). No-op in the stub.
func (p *Path) Arc(x, y, radius, startAngle, endAngle float32, dir Direction) {}

// ArcTo appends a tangent arc. No-op in the stub.
func (p *Path) ArcTo(x1, y1, x2, y2, radius float32) {}

// Close closes the current subpath. No-op in the stub.
func (p *Path) Close() {}

// AppendVerticesAndIndicesForFilling returns the inputs unchanged in the
// stub (no triangles produced). Real Ebiten tessellates the path's interior.
func (p *Path) AppendVerticesAndIndicesForFilling(vs []ebiten.Vertex, is []uint16) ([]ebiten.Vertex, []uint16) {
	return vs, is
}

// AppendVerticesAndIndicesForStroke returns the inputs unchanged in the
// stub. Real Ebiten tessellates the stroked outline using StrokeOptions.
func (p *Path) AppendVerticesAndIndicesForStroke(vs []ebiten.Vertex, is []uint16, op *StrokeOptions) ([]ebiten.Vertex, []uint16) {
	return vs, is
}

// ── Convenience drawing helpers (high-level) ───────────────────────────

// DrawFilledRect draws a filled axis-aligned rectangle. No-op stub.
func DrawFilledRect(dst *ebiten.Image, x, y, width, height float32, clr color.Color, antialias bool) {
}

// StrokeRect strokes the outline of an axis-aligned rectangle. No-op stub.
func StrokeRect(dst *ebiten.Image, x, y, width, height, strokeWidth float32, clr color.Color, antialias bool) {
}

// StrokeLine strokes a line segment from (x0, y0) to (x1, y1). No-op stub.
func StrokeLine(dst *ebiten.Image, x0, y0, x1, y1, strokeWidth float32, clr color.Color, antialias bool) {
}

// DrawFilledCircle draws a filled circle centered at (cx, cy). No-op stub.
func DrawFilledCircle(dst *ebiten.Image, cx, cy, r float32, clr color.Color, antialias bool) {
}

// StrokeCircle strokes the outline of a circle centered at (cx, cy). No-op stub.
func StrokeCircle(dst *ebiten.Image, cx, cy, r, strokeWidth float32, clr color.Color, antialias bool) {
}
