package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func (dv *DrumView) ensureActionMenu() *actionMenuComponent {
	if dv.actionMenu == nil {
		dv.actionMenu = newActionMenuComponent()
	}
	return dv.actionMenu
}

func (dv *DrumView) openActionMenu(id string, anchor image.Rectangle, items []actionMenuItem) {
	dv.openActionMenuEntry(id, anchor, items, false, nil)
}

// A must-answer question stacked over an open menu: closing it any other way (×, Esc, outside tap) runs onCancel.
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
		// Without updateFn MenuKeyNav.Update never runs and the arrows are dead.
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

func (dv *DrumView) instrumentActionMenuItems(saveAs func()) []actionMenuItem {
	return []actionMenuItem{
		{Label: i18n.T(i18n.KeySaveAs), OnSelect: saveAs},
		{Label: i18n.T(i18n.KeyMenuExport), OnSelect: func() { dv.exportActiveInstrument() }},
		{Label: i18n.T(i18n.KeyMenuImport), OnSelect: func() { dv.importInstrumentFromFile() }},
	}
}

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
