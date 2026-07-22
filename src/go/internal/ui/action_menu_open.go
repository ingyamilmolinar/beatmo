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
		ID:      id,
		Overlay: &compPortalOverlay{comp: m, tag: id},
		Modal:   false,
		Scrim:   true, // menu — backdrop dims so modality is visible
		Anchor:  anchor,
		OnClose: func() { m.Close() },
	})
}

// openSynthActionMenu opens the Synth "⋯" overflow: Save As…, Export/Import
// instrument (portable *.beatmo-inst.json), and Reset (destructive).
func (dv *DrumView) openSynthActionMenu(anchor image.Rectangle) {
	dv.openActionMenu("synth-action-menu", anchor, dv.instrumentActionMenuItems(
		func() { dv.ResetActiveRecipe() }, dv.openSaveAsDialog))
}

// openSamplerActionMenu mirrors openSynthActionMenu for the Sampler tab.
func (dv *DrumView) openSamplerActionMenu(anchor image.Rectangle) {
	dv.openActionMenu("sampler-action-menu", anchor, dv.instrumentActionMenuItems(
		func() { dv.samplerReset() }, dv.samplerSaveAs))
}

// instrumentActionMenuItems builds the shared "⋯" item set for both tabs: the
// tab-specific Save As + Reset plus the portable Export/Import actions.
func (dv *DrumView) instrumentActionMenuItems(reset, saveAs func()) []actionMenuItem {
	return []actionMenuItem{
		{Label: i18n.T(i18n.KeySaveAs), OnSelect: saveAs},
		{Label: i18n.T(i18n.KeyMenuExport), OnSelect: func() { dv.exportActiveInstrument() }},
		{Label: i18n.T(i18n.KeyMenuImport), OnSelect: func() { dv.importInstrumentFromFile() }},
		{Label: i18n.T(i18n.KeyReset), Destructive: true, OnSelect: reset},
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
