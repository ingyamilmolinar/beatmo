package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ensureActionMenu lazily builds the shared "⋯" overflow menu used by the
// mobile Synth/Sampler action clusters.
func (dv *DrumView) ensureActionMenu() *actionMenuComponent {
	if dv.actionMenu == nil {
		dv.actionMenu = newActionMenuComponent()
	}
	return dv.actionMenu
}

// openActionMenu opens the shared overflow menu anchored under `anchor` with the
// given items, through the same OverlayPortal path as the subdiv menu.
func (dv *DrumView) openActionMenu(id string, anchor image.Rectangle, items []actionMenuItem) {
	dv.openActionMenuEntry(id, anchor, items, false, nil)
}

// openQuestion opens the same menu as a must-answer question stacked over an
// open menu (the instrument menu): its items are the answers; closing it any
// other way (×, Esc, a tap outside) runs onCancel.
func (dv *DrumView) openQuestion(id string, anchor image.Rectangle, items []actionMenuItem, onCancel func()) {
	answered := false
	for i := range items {
		pick := items[i].OnSelect
		items[i].OnSelect = func() {
			answered = true
			if pick != nil {
				pick()
			}
		}
	}
	dv.openActionMenuEntry(id, anchor, items, true, func() {
		if !answered && onCancel != nil {
			onCancel()
		}
	})
}

func (dv *DrumView) openActionMenuEntry(id string, anchor image.Rectangle, items []actionMenuItem, mustAnswer bool, onClosed func()) {
	if dv.tree == nil {
		return
	}
	m := dv.ensureActionMenu()
	m.SetScreenBounds(dv.Bounds)
	m.SetProps(actionMenuProps{
		AnchorRect: anchor,
		RowHeight:  Profile().DensityValues().SynthHeaderButtonH,
		Items:      items,
		OnClose:    func() {},
	})
	m.Open()
	dv.portal().Open(PortalEntry{
		ID: id,
		// updateFn pumps the keyboard cursor every frame; without it
		// MenuKeyNav.Update never runs and the arrows are dead.
		Overlay: &compPortalOverlay{comp: m, tag: id, updateFn: m.UpdateKeyNav},
		Modal:   mustAnswer,
		Scrim:   true, // menu — backdrop dims so modality is visible
		Anchor:  anchor,
		OnClose: func() {
			m.Close()
			if onClosed != nil {
				onClosed()
			}
		},
	})
}

// openSynthActionMenu opens the Synth "⋯" overflow: Save As… and the portable
// Export/Import instrument (*.beatmo-inst.json).
func (dv *DrumView) openSynthActionMenu(anchor image.Rectangle) {
	dv.openActionMenu("synth-action-menu", anchor, dv.synthActionMenuItems())
}

// openSamplerActionMenu mirrors openSynthActionMenu for the Sampler tab.
func (dv *DrumView) openSamplerActionMenu(anchor image.Rectangle) {
	dv.openActionMenu("sampler-action-menu", anchor, dv.samplerActionMenuItems())
}

func (dv *DrumView) synthActionMenuItems() []actionMenuItem {
	return dv.instrumentActionMenuItems(dv.openSaveAsDialog)
}

func (dv *DrumView) samplerActionMenuItems() []actionMenuItem {
	return dv.instrumentActionMenuItems(dv.samplerSaveAs)
}

// instrumentActionMenuItems builds the shared "⋯" item set for both tabs: the
// tab's Save As plus the portable Export/Import actions.
func (dv *DrumView) instrumentActionMenuItems(saveAs func()) []actionMenuItem {
	return []actionMenuItem{
		{Label: i18n.T(i18n.KeySaveAs), OnSelect: saveAs},
		{Label: i18n.T(i18n.KeyMenuExport), OnSelect: func() { dv.exportActiveInstrument() }},
		{Label: i18n.T(i18n.KeyMenuImport), OnSelect: func() { dv.importInstrumentFromFile() }},
	}
}

// synthPanelMobileActions / samplerPanelMobileActions are the DrumView
// forwarders EQPanelZone uses to hand the sticky bar the active tab's trailing
// action buttons on mobile.
func (dv *DrumView) synthPanelMobileActions() []*Button {
	return dv.synp().MobileActionButtons()
}

func (dv *DrumView) samplerPanelMobileActions() []*Button {
	return dv.sp().MobileActionButtons()
}

// ActionMenuKeyNavForTest exposes the action menu's keyboard cursor.
func (dv *DrumView) ActionMenuKeyNavForTest() *MenuKeyNav {
	if dv.actionMenu == nil {
		return nil
	}
	return dv.actionMenu.KeyNav()
}
