package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// SetComponentRegistry wires a component registry into the drum view so
// overlays and controls render via pluggable components.
func (dv *DrumView) SetComponentRegistry(reg *ComponentRegistry) {
	dv.components = reg
}

func (dv *DrumView) renderComponents(phase RenderPhase, dst *ebiten.Image) {
	if dst == nil {
		return
	}
	if dv.components != nil {
		dv.components.Render(phase, dst)
		return
	}
	dv.renderBuiltinPhase(phase, dst)
}

func (dv *DrumView) renderBuiltinPhase(phase RenderPhase, dst *ebiten.Image) {
	switch phase {
	case RenderPhaseToolbar:
		if dv.simpleDraw {
			return
		}
		dv.renderToolbarControls(dst)
		dv.drawNotifications(dst)
	case RenderPhaseRowControls:
		dv.renderRowControlOverlay(dst)
	}
}
