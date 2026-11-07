package ui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Antialiased vector primitives on a 24-unit grid, converted to pixels at draw time; the language is DESIGN.md's
// `icon:` block, mirrored in touch_sizes.go.

// Centred on a non-square rect so an icon never stretches.
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

// At least 1 px so thin strokes survive small sizes.
func iconStrokeWidth(r image.Rectangle) float32 {
	dim := float32(minI(r.Dx(), r.Dy()))
	w := IconStrokeWeight * dim / float32(IconGrid)
	if w < 1 {
		w = 1
	}
	return w
}

// The path API samples a texture: a 3×3 white image with a 1×1 sub-image keeps antialiased edges from bleeding.
// Lazy, so test builds (stubbed Ebiten) never allocate a GPU texture.
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

func makeStrokeOptions(width float32) *vector.StrokeOptions {
	return &vector.StrokeOptions{
		Width:    width,
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}
}

// ── Public icon-drawing helpers (logical coords) ──────────────────────

func iconStrokeLine(dst *ebiten.Image, c iconCanvas, x0, y0, x1, y1 float32, col color.Color) {
	px0, py0 := c.px(x0, y0)
	px1, py1 := c.px(x1, y1)
	w := IconStrokeWeight * c.scale
	if w < 1 {
		w = 1
	}
	// vector.StrokeLine has butt caps; a Path gives the round caps the other helpers use.
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

// The last vertex connects to the first.
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
	// Quadratic corners are close enough to an arc at these sizes and cheaper.
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

// omit is "top"/"bottom"/"left"/"right": the trash bin and the import/export trays are open-topped.
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
	// Corners next to the omitted edge are skipped too: an open edge has no rounding.
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
	// Each segment is its own subpath; round caps make them meet cleanly.
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

// Radians, 0 = +x, π/2 = +y (down).
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
