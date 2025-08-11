package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name       string
	Instrument string
	Steps      []bool
	Color      color.Color
}

/* ───────────────────────────────────────────────────────────── */

type DrumView struct {
	Rows   []*DrumRow
	Bounds image.Rectangle
	Graph  *model.Graph
	logger *game_log.Logger

	cell      int // px per step
	labelW    int
	controlsW int // width reserved for control buttons

	bgDirty bool
	bgCache []*ebiten.Image

	instBtn     image.Rectangle
	uploadBtn   image.Rectangle
	instOptions []string

	// ui widgets (re-computed every frame)
	playBtn   image.Rectangle
	stopBtn   image.Rectangle
	bpmDecBtn image.Rectangle // Decrease BPM
	bpmBox    image.Rectangle
	bpmIncBtn image.Rectangle // Increase BPM
	lenDecBtn image.Rectangle // Decrease length
	lenIncBtn image.Rectangle // Increase length

	// internal ui state
	bpm           int
	focusBPM      bool
	playPressed   bool
	stopPressed   bool
	Length        int  // Length of the drum view, independent of graph
	bpmIncPressed bool // State for BPM increase button
	bpmDecPressed bool // State for BPM decrease button
	lenIncPressed bool // State for length increase button
	lenDecPressed bool // State for length decrease button

	// window scrolling
	Offset        int // index of first visible beat
	dragging      bool
	dragStartX    int
	startOffset   int
	offsetChanged bool
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
	opts := audio.Instruments()
	inst := "snare"
	name := "Snare"
	if len(opts) > 0 {
		inst = opts[0]
		name = strings.ToUpper(inst[:1]) + inst[1:]
	}
	dv := &DrumView{
		Bounds:      b,
		labelW:      80,
		bpm:         120,
		bgDirty:     true,
		Graph:       g,
		logger:      logger,
		Length:      8, // Default length
		Offset:      0,
		instOptions: opts,
	}
	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), Color: colStep}}
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
	dv.playBtn = image.Rect(10, dv.Bounds.Min.Y+10, 90, dv.Bounds.Min.Y+50)
	dv.stopBtn = image.Rect(100, dv.Bounds.Min.Y+10, 180, dv.Bounds.Min.Y+50)
	dv.bpmDecBtn = image.Rect(190, dv.Bounds.Min.Y+10, 230, dv.Bounds.Min.Y+50)
	dv.bpmBox = image.Rect(235, dv.Bounds.Min.Y+10, 275, dv.Bounds.Min.Y+50)
	dv.bpmIncBtn = image.Rect(280, dv.Bounds.Min.Y+10, 320, dv.Bounds.Min.Y+50)
	dv.lenDecBtn = image.Rect(325, dv.Bounds.Min.Y+10, 365, dv.Bounds.Min.Y+50)
	dv.lenIncBtn = image.Rect(370, dv.Bounds.Min.Y+10, 410, dv.Bounds.Min.Y+50)
	dv.instBtn = image.Rect(10, dv.Bounds.Min.Y+60, 150, dv.Bounds.Min.Y+100)
	dv.uploadBtn = image.Rect(160, dv.Bounds.Min.Y+60, 300, dv.Bounds.Min.Y+100)
	dv.controlsW = dv.lenIncBtn.Max.X
}

func (dv *DrumView) calcLayout() {
	if len(dv.Rows) > 0 {
		dv.cell = (dv.Bounds.Dx() - dv.labelW - dv.controlsW) / len(dv.Rows[0].Steps) // Leave space for buttons
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

func (dv *DrumView) OffsetChanged() bool {
	if dv.offsetChanged {
		dv.offsetChanged = false
		return true
	}
	return false
}

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.recalcButtons()
	dv.calcLayout()

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	stepsRect := image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// wheel zoom for length adjustment
	if _, whY := wheel(); whY != 0 {
		if pt(mx, my, stepsRect) {
			if whY > 0 && dv.Length < 64 {
				dv.Length++
				dv.Rows[0].Steps = make([]bool, dv.Length)
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length increased to: %d via wheel", dv.Length)
			}
			if whY < 0 && dv.Length > 1 {
				dv.Length--
				dv.Rows[0].Steps = make([]bool, dv.Length)
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length decreased to: %d via wheel", dv.Length)
			}
		}
	}

	/* ——— widget clicks & dragging ——— */
	if left {
		switch {
		case pt(mx, my, dv.playBtn):
			dv.playPressed = true
			dv.logger.Infof("[DRUMVIEW] Play button pressed.")
		case pt(mx, my, dv.stopBtn):
			dv.stopPressed = true
			dv.logger.Infof("[DRUMVIEW] Stop button pressed.")
		case pt(mx, my, dv.bpmDecBtn):
			dv.bpmDecPressed = true
			dv.logger.Infof("[DRUMVIEW] BPM decrease button pressed.")
		case pt(mx, my, dv.bpmIncBtn):
			dv.bpmIncPressed = true
			dv.logger.Infof("[DRUMVIEW] BPM increase button pressed.")
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
		case pt(mx, my, dv.instBtn):
			if len(dv.instOptions) > 0 {
				cur := dv.Rows[0].Instrument
				for i, id := range dv.instOptions {
					if id == cur {
						next := dv.instOptions[(i+1)%len(dv.instOptions)]
						dv.Rows[0].Instrument = next
						dv.Rows[0].Name = strings.ToUpper(next[:1]) + next[1:]
						break
					}
				}
			}
		case pt(mx, my, dv.uploadBtn):
			if id, err := audio.RegisterWAVDialog(); err == nil {
				dv.instOptions = audio.Instruments()
				dv.Rows[0].Instrument = id
				dv.Rows[0].Name = strings.ToUpper(id[:1]) + id[1:]
				dv.logger.Infof("[DRUMVIEW] Loaded user WAV %s", id)
			} else {
				dv.logger.Infof("[DRUMVIEW] Failed to load WAV: %v", err)
			}
		default:
			if pt(mx, my, stepsRect) {
				if !dv.dragging {
					dv.dragging = true
					dv.dragStartX = mx
					dv.startOffset = dv.Offset
				}
			} else if dv.focusBPM {
				dv.focusBPM = false
				dv.logger.Debugf("[DRUMVIEW] Clicked outside BPM box. focusingBPM: %t", dv.focusBPM)
			}
		}
	} else {
		dv.dragging = false
	}

	if dv.dragging {
		delta := (dv.dragStartX - mx) / dv.cell
		newOffset := dv.startOffset + delta
		if newOffset < 0 {
			newOffset = 0
		}
		if newOffset != dv.Offset {
			dv.Offset = newOffset
			dv.offsetChanged = true
			dv.logger.Debugf("[DRUMVIEW] Dragging: offset=%d", dv.Offset)
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

	/* ——— BPM editing via buttons ——— */
	if dv.bpmIncPressed {
		if dv.bpm < 300 {
			dv.bpm++
			dv.logger.Infof("[DRUMVIEW] BPM increased to: %d", dv.bpm)
		}
		dv.bpmIncPressed = false
	}
	if dv.bpmDecPressed {
		if dv.bpm > 1 {
			dv.bpm--
			dv.logger.Infof("[DRUMVIEW] BPM decreased to: %d", dv.bpm)
		}
		dv.bpmDecPressed = false
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

	drawButton(dst, dv.playBtn, colPlayButton, colButtonBorder, dv.playPressed)
	drawButton(dst, dv.stopBtn, colStopButton, colButtonBorder, dv.stopPressed)
	drawButton(dst, dv.bpmDecBtn, colLenDec, colButtonBorder, dv.bpmDecPressed)
	drawButton(dst, dv.bpmBox, colBPMBox, colButtonBorder, dv.focusBPM)
	drawButton(dst, dv.bpmIncBtn, colLenInc, colButtonBorder, dv.bpmIncPressed)
	drawButton(dst, dv.lenDecBtn, colLenDec, colButtonBorder, dv.lenDecPressed)
	drawButton(dst, dv.lenIncBtn, colLenInc, colButtonBorder, dv.lenIncPressed)
	drawButton(dst, dv.instBtn, colBPMBox, colButtonBorder, false)
	drawButton(dst, dv.uploadBtn, colBPMBox, colButtonBorder, false)
	ebitenutil.DebugPrintAt(dst, "▶", dv.playBtn.Min.X+30, dv.playBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "■", dv.stopBtn.Min.X+30, dv.stopBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "-", dv.bpmDecBtn.Min.X+15, dv.bpmDecBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, strconv.Itoa(dv.bpm), dv.bpmBox.Min.X+8, dv.bpmBox.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "+", dv.bpmIncBtn.Min.X+15, dv.bpmIncBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "-", dv.lenDecBtn.Min.X+15, dv.lenDecBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "+", dv.lenIncBtn.Min.X+15, dv.lenIncBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, dv.Rows[0].Name+" ▼", dv.instBtn.Min.X+5, dv.instBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "Upload", dv.uploadBtn.Min.X+5, dv.uploadBtn.Min.Y+18)

	// draw steps
	for i, r := range dv.Rows {
		y := dv.Bounds.Min.Y + i*dv.rowHeight()
		for j, step := range r.Steps {
			x := dv.Bounds.Min.X + dv.labelW + dv.controlsW + j*dv.cell // Adjusted for buttons
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

			var fill color.Color = colStepOff
			if step {
				fill = r.Color
			}
			if highlighted {
				fill = colHighlight
			}
			drawRect(dst, rect, fill, true)
			drawRect(dst, rect, colStepBorder, false)
		}
	}
}

func (dv *DrumView) bg(w, h int) *ebiten.Image {
	if dv.bgDirty || len(dv.bgCache) == 0 || !dv.bgCache[0].Bounds().Eq(image.Rect(0, 0, w, h)) {
		dv.bgCache = make([]*ebiten.Image, 1)
		img := ebiten.NewImage(w, h)
		img.Fill(colBGBottom)
		dv.bgCache[0] = img
		dv.bgDirty = false
	}
	return dv.bgCache[0]
}
