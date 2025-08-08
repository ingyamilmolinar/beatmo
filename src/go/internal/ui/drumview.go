package ui

import (
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
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
	playBtn   image.Rectangle
	stopBtn   image.Rectangle
	bpmBox    image.Rectangle
	lenIncBtn image.Rectangle // New button for increasing length
	lenDecBtn image.Rectangle // New button for decreasing length

	// internal ui state
	bpm           int
	focusBPM      bool
	playPressed   bool
	stopPressed   bool
	Length        int  // Length of the drum view, independent of graph
	lenIncPressed bool // New state for length increase button
	lenDecPressed bool // New state for length decrease button
}

/* ─── geometry helpers ─────────────────────────────────────── */

func (dv *DrumView) rowHeight() int {
	rows := len(dv.Rows)
	if rows == 0 {
		return 0
	}
	return dv.Bounds.Dy() / rows
}

// SetBeatLength sets the beat length in the underlying graph.
func (dv *DrumView) SetBeatLength(length int) {
	if dv.Graph != nil {
		dv.Graph.SetBeatLength(length)
		dv.logger.Debugf("[DRUMVIEW] Graph beat length set to: %d", length)
	}
}

/* ─── ctor ─────────────────────────────────────────────────── */

func NewDrumView(b image.Rectangle, g *model.Graph, logger *game_log.Logger) *DrumView {
	dv := &DrumView{
		Bounds:        b,
		labelW:        40,
		bpm:           120,
		bgDirty:       true,
		Graph:         g,
		logger:        logger,
		Length:        8, // Default length
		lenIncPressed: false,
		lenDecPressed: false,
	}
	dv.Rows = []*DrumRow{{Name: "H", Steps: make([]bool, dv.Length)}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	return dv
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

func (dv *DrumView) recalcButtons() {
	// Implementation of recalcButtons
	dv.playBtn = image.Rect(10, dv.Bounds.Min.Y+10, 90, dv.Bounds.Min.Y+50)     // Increased size
	dv.stopBtn = image.Rect(100, dv.Bounds.Min.Y+10, 180, dv.Bounds.Min.Y+50)   // Increased size
	dv.bpmBox = image.Rect(190, dv.Bounds.Min.Y+10, 270, dv.Bounds.Min.Y+50)    // Increased size
	dv.lenDecBtn = image.Rect(280, dv.Bounds.Min.Y+10, 320, dv.Bounds.Min.Y+50) // Increased size
	dv.lenIncBtn = image.Rect(330, dv.Bounds.Min.Y+10, 370, dv.Bounds.Min.Y+50) // Increased size
}

func (dv *DrumView) calcLayout() {
	// Implementation of calcLayout
	if len(dv.Rows) > 0 {
		dv.cell = (dv.Bounds.Dx() - dv.labelW - 380) / len(dv.Rows[0].Steps) // Leave space for buttons
	}
}

func (dv *DrumView) PlayPressed() bool {
	if dv.playPressed {
		dv.playPressed = false
		return true
	}
	return false
}

func (dv *DrumView) StopPressed() bool {
	if dv.stopPressed {
		dv.stopPressed = false
		return true
	}
	return false
}

func (dv *DrumView) BPM() int {
	return dv.bpm
}

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
			dv.playPressed = true
			dv.logger.Infof("[DRUMVIEW] Play button pressed.")
		case pt(mx, my, dv.stopBtn):
			dv.stopPressed = true
			dv.logger.Infof("[DRUMVIEW] Stop button pressed.")
		case pt(mx, my, dv.bpmBox):
			if !dv.focusBPM {
				dv.focusBPM = true
				dv.logger.Debugf("[DRUMVIEW] BPM box clicked. focusingBPM: %t", dv.focusBPM)
			}
		case pt(mx, my, dv.lenIncBtn):
			dv.lenIncPressed = true
			dv.logger.Infof("[DRUMVIEW] Length increase button pressed.")
		case pt(mx, my, dv.lenDecBtn):
			dv.lenDecPressed = true
			dv.logger.Infof("[DRUMVIEW] Length decrease button pressed.")
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

	/* ——— Length editing ——— */
	if dv.lenIncPressed {
		if dv.Length < 64 { // Set a reasonable max length
			dv.Length++
			dv.logger.Infof("[DRUMVIEW] Length increased to: %d", dv.Length)
			dv.Rows[0].Steps = make([]bool, dv.Length) // Update the steps slice
			dv.SetBeatLength(dv.Length)                // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenIncPressed = false
	}
	if dv.lenDecPressed {
		if dv.Length > 1 { // Prevent length from going below 1
			dv.Length--
			dv.logger.Infof("[DRUMVIEW] Length decreased to: %d", dv.Length)
			dv.Rows[0].Steps = make([]bool, dv.Length) // Update the steps slice
			dv.SetBeatLength(dv.Length)                // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenDecPressed = false
	}
}

func (dv *DrumView) Draw(dst *ebiten.Image, highlightedBeats map[int]int64, frame int64, beatInfos []model.BeatInfo) {
	dv.logger.Debugf("[DRUMVIEW] Draw called. beatInfos: %v, highlightedBeats: %v", beatInfos, highlightedBeats)
	// draw background
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)

	// draw buttons
	drawRect(dst, dv.playBtn, color.Gray{100}, false)
	drawRect(dst, dv.stopBtn, color.Gray{100}, false)
	drawRect(dst, dv.bpmBox, color.Gray{100}, false)
	drawRect(dst, dv.lenDecBtn, color.Gray{100}, false)
	drawRect(dst, dv.lenIncBtn, color.Gray{100}, false)

	// draw steps
	for i, r := range dv.Rows {
		y := dv.Bounds.Min.Y + i*dv.rowHeight()
		for j, step := range r.Steps {
			x := dv.Bounds.Min.X + dv.labelW + 380 + j*dv.cell // Adjusted for buttons
			rect := image.Rect(x, y, x+dv.cell, y+dv.rowHeight())

			// Highlighting logic
			highlighted := false
			isRegularNode := len(beatInfos) > j && beatInfos[j].NodeType == model.NodeTypeRegular

			if _, ok := highlightedBeats[j]; ok {
				highlighted = true
				if isRegularNode {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting regular node at index %d, NodeID: %v", j, beatInfos[j].NodeID)
				} else {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting empty beat at index %d", j)
				}
			}

			if highlighted {
				drawRect(dst, rect, color.RGBA{255, 255, 0, 255}, true) // Yellow highlight
			} else if step {
				drawRect(dst, rect, color.White, true)
			} else {
				drawRect(dst, rect, color.Gray{50}, false)
			}
		}
	}
}

func (dv *DrumView) bg(w, h int) *ebiten.Image {
	if dv.bgDirty || len(dv.bgCache) == 0 || !dv.bgCache[0].Bounds().Eq(image.Rect(0, 0, w, h)) {
		dv.bgCache = make([]*ebiten.Image, 1)
		img := ebiten.NewImage(w, h)
		img.Fill(color.Black)
		dv.bgCache[0] = img
		dv.bgDirty = false
	}
	return dv.bgCache[0]
}
