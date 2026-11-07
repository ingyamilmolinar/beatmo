package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Empty on desktop, where the side pane carries the graph.
func (p *synthPanel) synthMobileFocusRect() image.Rectangle {
	dv := p.dv

	return dv.instEditorMobileFocusRect
}

// After the preview pane and mobile band are carved out.
func (p *synthPanel) synthSectionsRectForTest() image.Rectangle {
	dv := p.dv

	return dv.instEditorSectionsRect
}

// Sticky on press, never hover (that would thrash the graph as the pointer crosses dials); falls back to the
// stage's main knob so the graph is never empty.

func (p *synthPanel) synthSelectedControlIdx(instID instrument.ID, sec *synthSection) int {
	dv := p.dv

	if sec == nil || len(sec.knobIdxs) == 0 {
		return -1
	}
	if dv.instEditorSelectedControl != nil {
		if idx, ok := dv.instEditorSelectedControl[instID]; ok {
			for _, k := range sec.knobIdxs {
				if k == idx {
					return idx
				}
			}
		}
	}
	return sec.knobIdxs[0]
}

// setSynthSelectedControl records the focus-graph selection for instID.
func (p *synthPanel) setSynthSelectedControl(instID instrument.ID, kIdx int) {
	dv := p.dv

	if instID == "" {
		return
	}
	if dv.instEditorSelectedControl == nil {
		dv.instEditorSelectedControl = map[instrument.ID]int{}
	}
	dv.instEditorSelectedControl[instID] = kIdx
}

// --- DrumView forwarding shims ---

func (dv *DrumView) synthMobileFocusRect() image.Rectangle {
	return dv.synp().synthMobileFocusRect()
}

func (dv *DrumView) synthSectionsRectForTest() image.Rectangle {
	return dv.synp().synthSectionsRectForTest()
}

func (dv *DrumView) synthSelectedControlIdx(instID instrument.ID, sec *synthSection) int {
	return dv.synp().synthSelectedControlIdx(instID, sec)
}

func (dv *DrumView) setSynthSelectedControl(instID instrument.ID, kIdx int) {
	dv.synp().setSynthSelectedControl(instID, kIdx)
}
