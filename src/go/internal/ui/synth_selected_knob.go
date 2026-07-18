package ui

import "image"

// synthMobileFocusRect is the right column beside the knob grid (empty on
// desktop) reserved on mobile for "Your sound" + the focus graph (on desktop the
// side preview pane carries them instead).
func (p *synthPanel) synthMobileFocusRect() image.Rectangle {
	dv := p.dv

	return dv.instEditorMobileFocusRect
}

// synthSectionsRectForTest returns the post-shrink knob-grid rect that
// layoutSynthSections actually consumes — i.e. after the desktop preview pane
// and/or mobile focus band have been carved out. Test-only introspection.
func (p *synthPanel) synthSectionsRectForTest() image.Rectangle {
	dv := p.dv

	return dv.instEditorSectionsRect
}

// synth_selected_knob.go — which knob the focus graph explains.
//
// Selection is per-instrument, set on knob press (click-to-select, sticky —
// never hover, which would thrash the graph as the pointer crosses dials). When
// the stored knob isn't part of the currently open stage, or nothing is
// selected yet, the focus graph defaults to the stage's MAIN knob (its first
// knob), so the graph is never empty.

// synthSelectedKnobIdx returns the knob index the focus graph should explain for
// instID within the open section: the stored selection when it belongs to this
// section, else the section's first (main) knob.
func (p *synthPanel) synthSelectedKnobIdx(instID string, sec *synthSection) int {
	dv := p.dv

	if sec == nil || len(sec.knobIdxs) == 0 {
		return -1
	}
	if dv.instEditorSelectedKnob != nil {
		if idx, ok := dv.instEditorSelectedKnob[instID]; ok {
			for _, k := range sec.knobIdxs {
				if k == idx {
					return idx
				}
			}
		}
	}
	return sec.knobIdxs[0]
}

// setSynthSelectedKnob records the focus-graph selection for instID.
func (p *synthPanel) setSynthSelectedKnob(instID string, kIdx int) {
	dv := p.dv

	if instID == "" {
		return
	}
	if dv.instEditorSelectedKnob == nil {
		dv.instEditorSelectedKnob = map[string]int{}
	}
	dv.instEditorSelectedKnob[instID] = kIdx
}

// --- DrumView forwarding shims ---

func (dv *DrumView) synthMobileFocusRect() image.Rectangle {
	return dv.synp().synthMobileFocusRect()
}

func (dv *DrumView) synthSectionsRectForTest() image.Rectangle {
	return dv.synp().synthSectionsRectForTest()
}

func (dv *DrumView) synthSelectedKnobIdx(instID string, sec *synthSection) int {
	return dv.synp().synthSelectedKnobIdx(instID, sec)
}

func (dv *DrumView) setSynthSelectedKnob(instID string, kIdx int) {
	dv.synp().setSynthSelectedKnob(instID, kIdx)
}
