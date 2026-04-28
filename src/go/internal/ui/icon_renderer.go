package ui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Icon renderer kernel — antialiased vector primitives in a 24-unit logical
// grid. Every IconID body composes itself from these helpers in
// logical-grid coordinates; the helpers convert to pixel space at draw
// time using the bounding rect's dimensions.
//
// Visual language: Lucide-style minimalism — 1.75-unit stroke, round
// caps/joins, generous interior padding, antialias on. Defined in
// DESIGN.md `icon:` block; mirrored in touch_sizes.go (IconGrid /
// IconStrokeWeight / IconPadding / IconCornerRadius).

// iconCanvas maps the IconGrid×IconGrid logical coordinate space onto a
// pixel-space rectangle. Logical (0,0) is the top-left of the canvas;
// logical (IconGrid, IconGrid) is the bottom-right. The canvas is
// centered when the rect is non-square so the icon never stretches.
type iconCanvas struct {
	ox, oy float32 // pixel-space origin of the logical (0,0) point
	scale  float32 // pixels per logical unit
}

func newIconCanvas(r image.Rectangle) iconCanvas {
	dim := minI(r.Dx(), r.Dy())
	scale := float32(dim) / float32(IconGrid)
	cx := float32(r.Min.X) + float32(r.Dx())/2
	cy := float32(r.Min.Y) + float32(r.Dy())/2
	half := float32(IconGrid) * scale / 2
	return iconCanvas{ox: cx - half, oy: cy - half, scale: scale}
}

// px converts a logical-grid coordinate to pixel space.
func (c iconCanvas) px(x, y float32) (float32, float32) {
	return c.ox + x*c.scale, c.oy + y*c.scale
}

// iconStrokeWidth returns the antialiased stroke width in pixels for the
// rect r. Always ≥ 1 px so sub-pixel strokes don't disappear at small
// icon sizes (the floor is the only branch the size-floor test covers).
func iconStrokeWidth(r image.Rectangle) float32 {
	dim := float32(minI(r.Dx(), r.Dy()))
	w := IconStrokeWeight * dim / float32(IconGrid)
	if w < 1 {
		w = 1
	}
	return w
}

// ── Internal: white texture for path rasterization ─────────────────────
//
// Ebiten's path API (AppendVerticesAndIndicesForFilling/ForStroke) needs
// a sample texture for DrawTriangles. The example pattern is a 3×3 white
// image with a 1×1 sub-image so antialiased edges sample interior
// pixels and don't bleed outside. Lazy-initialized so test builds (which
// stub Ebiten) never attempt to allocate a real GPU texture.
var (
	iconWhiteOnce sync.Once
	iconWhiteSub  *ebiten.Image
)

func iconWhiteSubImage() *ebiten.Image {
	iconWhiteOnce.Do(func() {
		base := ebiten.NewImage(3, 3)
		base.Fill(color.White)
		iconWhiteSub = base.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
	})
	return iconWhiteSub
}

// rgbaParts splits col into 0..1 float32 components for vertex tinting.
func rgbaParts(col color.Color) (cr, cg, cb, ca float32) {
	r16, g16, b16, a16 := col.RGBA()
	return float32(r16) / 65535, float32(g16) / 65535, float32(b16) / 65535, float32(a16) / 65535
}

// drawPath rasterizes a built vector.Path either as a stroke (when op is
// non-nil) or as a fill, tinting all generated vertices to col. Uses the
// shared whiteSubImage texture per Ebiten's vector example.
func drawPath(dst *ebiten.Image, path *vector.Path, op *vector.StrokeOptions, col color.Color) {
	var vs []ebiten.Vertex
	var is []uint16
	if op != nil {
		vs, is = path.AppendVerticesAndIndicesForStroke(vs, is, op)
	} else {
		vs, is = path.AppendVerticesAndIndicesForFilling(vs, is)
	}
	cr, cg, cb, ca := rgbaParts(col)
	for i := range vs {
		vs[i].SrcX = 1
		vs[i].SrcY = 1
		vs[i].ColorR = cr
		vs[i].ColorG = cg
		vs[i].ColorB = cb
		vs[i].ColorA = ca
	}
	dop := &ebiten.DrawTrianglesOptions{
		AntiAlias: true,
		FillRule:  ebiten.FillRuleNonZero,
	}
	dst.DrawTriangles(vs, is, iconWhiteSubImage(), dop)
}

// makeStrokeOptions builds the canonical stroke options used by every icon:
// round caps + round joins, fixed width, no miter spikes.
func makeStrokeOptions(width float32) *vector.StrokeOptions {
	return &vector.StrokeOptions{
		Width:    width,
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}
}

// ── Public icon-drawing helpers (logical coords) ──────────────────────

// iconStrokeLine strokes a line from (x0,y0) to (x1,y1) in logical-grid
// units, using col and the canvas-derived stroke width.
func iconStrokeLine(dst *ebiten.Image, c iconCanvas, x0, y0, x1, y1 float32, col color.Color) {
	px0, py0 := c.px(x0, y0)
	px1, py1 := c.px(x1, y1)
	w := IconStrokeWeight * c.scale
	if w < 1 {
		w = 1
	}
	// vector.StrokeLine has butt caps. To get round caps that match the
	// other helpers, build a minimal Path + StrokeOptions instead.
	var p vector.Path
	p.MoveTo(px0, py0)
	p.LineTo(px1, py1)
	drawPath(dst, &p, makeStrokeOptions(w), col)
}

// iconStrokeCircle strokes a circle outline. cx,cy and r are logical units.
func iconStrokeCircle(dst *ebiten.Image, c iconCanvas, cx, cy, r float32, col color.Color) {
	pcx, pcy := c.px(cx, cy)
	pr := r * c.scale
	w := IconStrokeWeight * c.scale
	if w < 1 {
		w = 1
	}
	vector.StrokeCircle(dst, pcx, pcy, pr, w, col, true)
}

// iconFillCircle fills a circle.
func iconFillCircle(dst *ebiten.Image, c iconCanvas, cx, cy, r float32, col color.Color) {
	pcx, pcy := c.px(cx, cy)
	pr := r * c.scale
	vector.DrawFilledCircle(dst, pcx, pcy, pr, col, true)
}

// iconStrokePolyline strokes a connected sequence of line segments
// (logical coords) with the canonical stroke options.
func iconStrokePolyline(dst *ebiten.Image, c iconCanvas, pts [][2]float32, col color.Color) {
	if len(pts) < 2 {
		return
	}
	var p vector.Path
	x, y := c.px(pts[0][0], pts[0][1])
	p.MoveTo(x, y)
	for i := 1; i < len(pts); i++ {
		px, py := c.px(pts[i][0], pts[i][1])
		p.LineTo(px, py)
	}
	w := IconStrokeWeight * c.scale
	if w < 1 {
		w = 1
	}
	drawPath(dst, &p, makeStrokeOptions(w), col)
}

// iconFillPath fills a closed polygon defined by pts (logical coords).
// Last vertex is connected to the first; no need to repeat the start.
func iconFillPath(dst *ebiten.Image, c iconCanvas, pts [][2]float32, col color.Color) {
	if len(pts) < 3 {
		return
	}
	var p vector.Path
	x, y := c.px(pts[0][0], pts[0][1])
	p.MoveTo(x, y)
	for i := 1; i < len(pts); i++ {
		px, py := c.px(pts[i][0], pts[i][1])
		p.LineTo(px, py)
	}
	p.Close()
	drawPath(dst, &p, nil, col)
}

// iconFillRoundedRect fills a rounded rectangle. x,y is the top-left,
// w,h are dimensions, radius is the corner radius — all in logical units.
func iconFillRoundedRect(dst *ebiten.Image, c iconCanvas, x, y, w, h, radius float32, col color.Color) {
	if radius <= 0 {
		px, py := c.px(x, y)
		vector.DrawFilledRect(dst, px, py, w*c.scale, h*c.scale, col, true)
		return
	}
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	var p vector.Path
	// Corners as quadratic curves — close enough to a circular arc at
	// these scales, faster than tessellating a true circular arc.
	x0, y0 := c.px(x+radius, y)
	x1, y1 := c.px(x+w-radius, y)
	x2, y2 := c.px(x+w, y+radius)
	x3, y3 := c.px(x+w, y+h-radius)
	x4, y4 := c.px(x+w-radius, y+h)
	x5, y5 := c.px(x+radius, y+h)
	x6, y6 := c.px(x, y+h-radius)
	x7, y7 := c.px(x, y+radius)
	cx_tr, cy_tr := c.px(x+w, y)
	cx_br, cy_br := c.px(x+w, y+h)
	cx_bl, cy_bl := c.px(x, y+h)
	cx_tl, cy_tl := c.px(x, y)
	p.MoveTo(x0, y0)
	p.LineTo(x1, y1)
	p.QuadTo(cx_tr, cy_tr, x2, y2)
	p.LineTo(x3, y3)
	p.QuadTo(cx_br, cy_br, x4, y4)
	p.LineTo(x5, y5)
	p.QuadTo(cx_bl, cy_bl, x6, y6)
	p.LineTo(x7, y7)
	p.QuadTo(cx_tl, cy_tl, x0, y0)
	p.Close()
	drawPath(dst, &p, nil, col)
}

// iconStrokeOpenRoundedRect strokes 3 of 4 sides of a rounded rectangle.
// `omit` controls which edge is skipped: "top", "bottom", "left", "right".
// Used for the trash bin (no top edge — lid sits over the body) and the
// open-top tray of import/export icons.
func iconStrokeOpenRoundedRect(dst *ebiten.Image, c iconCanvas, x, y, w, h, radius float32, omit string, col color.Color) {
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	type seg struct {
		x0, y0, x1, y1 float32
		cx, cy         float32
		curve          bool
	}
	// Build the four edges + four corners as a list, then skip whichever
	// the caller asked to omit. Corners adjacent to the omitted edge are
	// also skipped (an "open" edge has no rounding).
	top := seg{x + radius, y, x + w - radius, y, 0, 0, false}
	right := seg{x + w, y + radius, x + w, y + h - radius, 0, 0, false}
	bottom := seg{x + w - radius, y + h, x + radius, y + h, 0, 0, false}
	left := seg{x, y + h - radius, x, y + radius, 0, 0, false}
	tr := seg{x + w - radius, y, x + w, y + radius, x + w, y, true}
	br := seg{x + w, y + h - radius, x + w - radius, y + h, x + w, y + h, true}
	bl := seg{x + radius, y + h, x, y + h - radius, x, y + h, true}
	tl := seg{x, y + radius, x + radius, y, x, y, true}
	type edge struct {
		side string
		segs []seg
	}
	edges := []edge{
		{"top", []seg{tl, top, tr}},
		{"right", []seg{tr, right, br}},
		{"bottom", []seg{br, bottom, bl}},
		{"left", []seg{bl, left, tl}},
	}
	// Strokes draw each remaining segment as its own subpath; the
	// stroke options apply round caps so adjacent segments meet cleanly.
	w_px := IconStrokeWeight * c.scale
	if w_px < 1 {
		w_px = 1
	}
	op := makeStrokeOptions(w_px)
	var p vector.Path
	have := false
	for _, e := range edges {
		if e.side == omit {
			continue
		}
		for _, s := range e.segs {
			x0, y0 := c.px(s.x0, s.y0)
			x1, y1 := c.px(s.x1, s.y1)
			if !have {
				p.MoveTo(x0, y0)
				have = true
			} else {
				p.MoveTo(x0, y0)
			}
			if s.curve {
				cx, cy := c.px(s.cx, s.cy)
				p.QuadTo(cx, cy, x1, y1)
			} else {
				p.LineTo(x1, y1)
			}
		}
	}
	if have {
		drawPath(dst, &p, op, col)
	}
}

// iconStrokeArc strokes a circular arc from startAngle to endAngle (radians,
// 0 = +x, π/2 = +y down). cx,cy and r are logical units.
func iconStrokeArc(dst *ebiten.Image, c iconCanvas, cx, cy, r, startAngle, endAngle float32, col color.Color) {
	pcx, pcy := c.px(cx, cy)
	pr := r * c.scale
	var p vector.Path
	const steps = 24
	a0 := float64(startAngle)
	a1 := float64(endAngle)
	step := (a1 - a0) / steps
	for i := 0; i <= steps; i++ {
		a := a0 + step*float64(i)
		x := pcx + float32(math.Cos(a))*pr
		y := pcy + float32(math.Sin(a))*pr
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	w := IconStrokeWeight * c.scale
	if w < 1 {
		w = 1
	}
	drawPath(dst, &p, makeStrokeOptions(w), col)
}
