package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name   string
	Steps  []bool
	Offset int
	Color  color.Color
}

/* ───────────────────────────────────────────────────────────── */

type DrumView struct {
	Rows   []*DrumRow
	Bounds image.Rectangle

	cell   int // px per step
	labelW int

	bgDirty bool
	bgCache []*ebiten.Image

	// ui widgets (re-computed every frame)
	minusBtn image.Rectangle
	plusBtn  image.Rectangle
	playBtn  image.Rectangle
	stopBtn  image.Rectangle
	bpmBox   image.Rectangle

	playing  bool
	bpm      int
	focusBPM bool

	// drag-rotate
	dragRow    int
	prevMouseX int
}

/* ─── geometry helpers ─────────────────────────────────────── */

func (dv *DrumView) rowHeight() int {
	rows := len(dv.Rows)
	if rows == 0 {
		return 0
	}
	return dv.Bounds.Dy() / rows
}

/* ─── ctor ─────────────────────────────────────────────────── */

func NewDrumView(b image.Rectangle) *DrumView {
	return &DrumView{
		Bounds:  b,
		labelW:  40,
		bpm:     120,
		bgDirty: true,
	}
}

// SetBounds is called from Game whenever the splitter moves or the window
// resizes; it invalidates the cached background so dimensions update next draw.
func (dv *DrumView) SetBounds(b image.Rectangle) {
	if dv.Bounds != b {
		dv.Bounds = b
		dv.bgDirty = true
	}
}

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.recalcButtons()
	dv.calcLayout()

	mx, my := cursorPosition()

	/* ——— widget clicks ——— */
	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		switch {
		case pt(mx, my, dv.minusBtn):
			dv.resizeSteps(-1)
			return
		case pt(mx, my, dv.plusBtn):
			dv.resizeSteps(+1)
			return
		case pt(mx, my, dv.playBtn):
			dv.playing = true
		case pt(mx, my, dv.stopBtn):
			dv.playing = false
		case pt(mx, my, dv.bpmBox):
			dv.focusBPM = true
		default:
			dv.focusBPM = false
		}
	}

	/* ——— BPM editing ——— */
	if dv.focusBPM {
		for _, r := range inputChars() {
			if r >= '0' && r <= '9' {
				val, _ := strconv.Atoi(string(r))
				dv.bpm = dv.bpm*10 + val
				if dv.bpm > 300 {
					dv.bpm = 300
				}
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			dv.bpm /= 10
			if dv.bpm == 0 {
				dv.bpm = 1
			}
		}
	}

	/* ——— row rotate (click-drag horizontally) ——— */
	if !pt(mx, my, dv.Bounds) {
		dv.dragRow = -1
		return
	}
	rowIdx := (my - dv.Bounds.Min.Y) / dv.rowHeight()
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		dv.dragRow = -1
		return
	}

	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		if dv.dragRow == -1 {
			dv.dragRow = rowIdx
			dv.prevMouseX = mx
		} else if dv.dragRow == rowIdx {
			dx := mx - dv.prevMouseX
			stepDelta := dx / dv.cell
			if stepDelta != 0 {
				r := dv.Rows[rowIdx]
				r.Offset = (r.Offset + stepDelta) % len(r.Steps)
				if r.Offset < 0 {
					r.Offset += len(r.Steps)
				}
				dv.prevMouseX = mx
			}
		}
	} else {
		dv.dragRow = -1
	}
}

/* ─── layout & background cache ────────────────────────────── */

func (dv *DrumView) resizeSteps(dir int) {
	row := dv.Rows[0] // all rows share length; only mutate first then copy
	switch dir {
	case -1:
		if len(row.Steps) > 4 {
			row.Steps = row.Steps[:len(row.Steps)/2]
		}
	case +1:
		if len(row.Steps) < 64 {
			row.Steps = append(row.Steps, row.Steps...)
		}
	}
	// ensure every row’s slice length matches
	for i := range dv.Rows {
		if len(dv.Rows[i].Steps) != len(row.Steps) {
			dv.Rows[i].Steps = make([]bool, len(row.Steps))
		}
	}
	dv.bgDirty = true
}

func (dv *DrumView) calcLayout() {
	steps := len(dv.Rows[0].Steps)
	if steps == 0 {
		steps = 4
	}
	dv.cell = (dv.Bounds.Dx() - dv.labelW) / steps
	if dv.cell < 20 {
		dv.cell = 20
	}
	if dv.bgDirty {
		dv.rebuildBG()
		dv.bgDirty = false
	}
}

func (dv *DrumView) rebuildBG() {
	dv.bgCache = make([]*ebiten.Image, len(dv.Rows))
	rh := dv.rowHeight()
	for idx := range dv.Rows {
		img := ebiten.NewImage(dv.Bounds.Dx(), rh)
		img.Fill(color.RGBA{30, 30, 30, 255})

		// vertical guides
		for i := 0; i <= len(dv.Rows[idx].Steps); i++ {
			x := dv.labelW + i*dv.cell
			drawLine(img, float64(x), 0, float64(x), float64(rh),
				color.RGBA{60, 60, 60, 255})
		}
		ebitenutil.DebugPrintAt(img, dv.Rows[idx].Name, 4, rh/2-4)
		dv.bgCache[idx] = img
	}
}

func (dv *DrumView) recalcButtons() {
	top := dv.Bounds.Min.Y + 4
	dv.minusBtn = image.Rect(4, top, 32, top+20)
	dv.plusBtn = image.Rect(36, top, 64, top+20)
	dv.playBtn = image.Rect(80, top, 104, top+20)
	dv.stopBtn = image.Rect(110, top, 134, top+20)
	dv.bpmBox = image.Rect(150, top, 210, top+20)
}

/* ─── draw ───────────────────────────────────────────────── */

var idM ebiten.GeoM

func (dv *DrumView) Draw(dst *ebiten.Image) {
	if len(dv.Rows) == 0 {
		return
	}

	rh := dv.rowHeight()

	// rows
	for idx, r := range dv.Rows {
		y := dv.Bounds.Min.Y + idx*rh

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(y))
		dst.DrawImage(dv.bgCache[idx], op)

		for i := 0; i < len(r.Steps); i++ {
			if !r.Steps[(i+r.Offset)%len(r.Steps)] {
				continue
			}
			x := dv.Bounds.Min.X + dv.labelW + i*dv.cell
			scale := float64(rh-4) / float64(NodeAnim.Bounds().Dx())
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(scale, scale)
			op.GeoM.Translate(float64(x+2), float64(y+2))
			dst.DrawImage(NodeFrames[0], op)
		}
	}

	// widgets (draw last so they sit on top)
	drawRect(dst, dv.playBtn, color.White)
	ebitenutil.DebugPrintAt(dst, ">", dv.playBtn.Min.X+4, dv.playBtn.Min.Y+2)

	drawRect(dst, dv.stopBtn, color.White)
	ebitenutil.DebugPrintAt(dst, "X", dv.stopBtn.Min.X+4, dv.stopBtn.Min.Y+2)

	drawRect(dst, dv.minusBtn, color.White)
	ebitenutil.DebugPrintAt(dst, "-", dv.minusBtn.Min.X+8, dv.minusBtn.Min.Y+2)

	drawRect(dst, dv.plusBtn, color.White)
	ebitenutil.DebugPrintAt(dst, "+", dv.plusBtn.Min.X+8, dv.plusBtn.Min.Y+2)

	drawRect(dst, dv.bpmBox, color.White)
	ebitenutil.DebugPrintAt(dst, fmt.Sprintf("%d", dv.bpm),
		dv.bpmBox.Min.X+4, dv.bpmBox.Min.Y+2)
}

/* ─── utility ───────────────────────────────────────────── */

func pt(x, y int, r image.Rectangle) bool {
	return x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y
}

func drawLine(dst *ebiten.Image, x1, y1, x2, y2 float64, col color.Color) {
	DrawLineCam(dst, x1, y1, x2, y2, &idM, col, 1)
}

func drawRect(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	drawLine(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Min.Y), col)
	drawLine(dst, float64(r.Max.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Max.Y), col)
	drawLine(dst, float64(r.Max.X), float64(r.Max.Y), float64(r.Min.X), float64(r.Max.Y), col)
	drawLine(dst, float64(r.Min.X), float64(r.Max.Y), float64(r.Min.X), float64(r.Min.Y), col)
}

