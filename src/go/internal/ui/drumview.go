package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name  string
	Steps []bool
	Color color.Color
}

/* ───────────────────────────────────────────────────────────── */

type DrumView struct {
	Rows   []*DrumRow
	Bounds image.Rectangle
	Graph  *model.Graph
	logger *game_log.Logger

	cell   int // px per step
	labelW int

	bgDirty bool
	bgCache []*ebiten.Image

	// ui widgets (re-computed every frame)
	
	playBtn  image.Rectangle
	stopBtn  image.Rectangle
	bpmBox   image.Rectangle

	playing  bool
	bpm      int
	focusBPM bool
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

func NewDrumView(b image.Rectangle, g *model.Graph, logger *game_log.Logger) *DrumView {
	return &DrumView{
		Bounds:  b,
		labelW:  40,
		bpm:     120,
		bgDirty: true,
		Graph:   g,
		logger:  logger,
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
		case pt(mx, my, dv.playBtn):
			if !dv.playing {
				dv.playing = true
				dv.logger.Infof("[DRUMVIEW] Play button pressed. playing: %t", dv.playing)
			}
		case pt(mx, my, dv.stopBtn):
			if dv.playing {
				dv.playing = false
				dv.logger.Infof("[DRUMVIEW] Stop button pressed. playing: %t", dv.playing)
			}
		case pt(mx, my, dv.bpmBox):
			if !dv.focusBPM {
				dv.focusBPM = true
				dv.logger.Debugf("[DRUMVIEW] BPM box clicked. focusingBPM: %t", dv.focusBPM)
			}
		default:
			if dv.focusBPM {
				dv.focusBPM = false
				dv.logger.Debugf("[DRUMVIEW] Clicked outside BPM box. focusingBPM: %t", dv.focusBPM)
			}
		}
	}

	/* ——— BPM editing ——— */
	if dv.focusBPM {
		for _, r := range inputChars() {
			if r >= '0' && r <= '9' {
				val, _ := strconv.Atoi(string(r))
				newBPM := dv.bpm*10 + val
				if newBPM > 300 {
					newBPM = 300
				}
				if newBPM != dv.bpm {
					dv.bpm = newBPM
					dv.logger.Debugf("[DRUMVIEW] BPM changed to: %d", dv.bpm)
				}
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			newBPM := dv.bpm / 10
			if newBPM == 0 {
				newBPM = 1
			}
			if newBPM != dv.bpm {
				dv.bpm = newBPM
				dv.logger.Debugf("[DRUMVIEW] BPM changed to: %d", dv.bpm)
			}
		}
	}
}

/* ─── layout & background cache ────────────────────────────── */

func (dv *DrumView) SetLength(newLen int) {
	for i := range dv.Rows {
		if len(dv.Rows[i].Steps) != newLen {
			dv.Rows[i].Steps = make([]bool, newLen)
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
	dv.playBtn = image.Rect(4, top, 28, top+20)
	dv.stopBtn = image.Rect(34, top, 58, top+20)
	dv.bpmBox = image.Rect(70, top, 130, top+20)
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
			if !r.Steps[i] {
				continue
			}
			x := dv.Bounds.Min.X + dv.labelW + i*dv.cell
			// Draw a filled rectangle for the active step
			stepRect := image.Rect(x+2, y+2, x+dv.cell-2, y+rh-2)
			drawRect(dst, stepRect, color.RGBA{0, 200, 0, 255}) // Bright green
		}
	}

	// widgets (draw last so they sit on top)
	drawRect(dst, dv.playBtn, color.White)
	ebitenutil.DebugPrintAt(dst, ">", dv.playBtn.Min.X+4, dv.playBtn.Min.Y+2)

	drawRect(dst, dv.stopBtn, color.White)
	ebitenutil.DebugPrintAt(dst, "X", dv.stopBtn.Min.X+4, dv.stopBtn.Min.Y+2)

	

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

