package ui

import (
	"image"
)

// "" means no crop. A new subject needs a const, a SubjectRect case and a public bounds getter on its owner: never
// reach into a private field from here.
type Subject string

const (
	SubjectFullScreen     Subject = ""
	SubjectMainGrid       Subject = "main_grid"
	SubjectDrumView       Subject = "drum_view" // entire drum pane (header + rows + EQ panel)
	SubjectDrumRows       Subject = "drum_rows" // just the row scroll area, excludes header + EQ
	SubjectEQPanel        Subject = "eq_panel"
	SubjectEQTabEQ        Subject = "eq_tab_eq"
	SubjectEQTabWave      Subject = "eq_tab_wave"
	SubjectEQTabSpectrum  Subject = "eq_tab_spectrum"
	SubjectEQTabLevels    Subject = "eq_tab_levels"
	SubjectSynthPanel     Subject = "synth_panel"   // audio panel content rect when TabSynth active
	SubjectSampler        Subject = "sampler_panel" // audio panel content rect when TabSampler active
	SubjectChain          Subject = "chain"
	SubjectAudioStickyBar Subject = "audio_sticky_bar"
	SubjectToolbar        Subject = "toolbar" // full top header strip (transport+BPM+vol+overflow)
	SubjectFXPanel        Subject = "fx_panel"
	SubjectContextMenu    Subject = "context_menu"
	SubjectOverflowMenu   Subject = "overflow_menu"
	SubjectInstrumentMenu Subject = "instrument_menu"
	SubjectGroupMenu      Subject = "group_menu"
	SubjectThemeEditor    Subject = "theme_editor" // the Theme editor overlay panel (grid palette button)
	SubjectNavMenu        Subject = "nav_menu"     // the navigation menu card (beat counter) incl. the go-to wheel
	SubjectWarmupToast    Subject = "warmup_toast" // the circuit-loading progress toast (logsToast)
)

// Excludes SubjectFullScreen, the absence of a crop.
func AllSubjects() []Subject {
	return []Subject{
		SubjectMainGrid,
		SubjectDrumView,
		SubjectDrumRows,
		SubjectEQPanel,
		SubjectEQTabEQ,
		SubjectEQTabWave,
		SubjectEQTabSpectrum,
		SubjectEQTabLevels,
		SubjectSynthPanel,
		SubjectSampler,
		SubjectChain,
		SubjectAudioStickyBar,
		SubjectToolbar,
		SubjectFXPanel,
		SubjectContextMenu,
		SubjectOverflowMenu,
		SubjectInstrumentMenu,
		SubjectGroupMenu,
	}
}

// !ok when the subject is not visible (the caller reports it). Clipped to the framebuffer. Valid only after a
// layout + draw: the rects are zero on a new Game.
func (g *Game) SubjectRect(s Subject) (image.Rectangle, bool) {
	if s == SubjectFullScreen {
		return image.Rect(0, 0, g.winW, g.winH), true
	}
	dv := g.drum
	full := image.Rect(0, 0, g.winW, g.winH)
	clamp := func(r image.Rectangle) (image.Rectangle, bool) {
		if r.Empty() {
			return image.Rectangle{}, false
		}
		c := r.Intersect(full)
		if c.Empty() {
			return image.Rectangle{}, false
		}
		return c, true
	}
	switch s {
	case SubjectMainGrid:
		if g.split == nil || g.winW == 0 || g.winH == 0 {
			return image.Rectangle{}, false
		}
		return clamp(g.split.GridRect(g.winW, g.winH))
	case SubjectDrumView:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.Bounds)
	case SubjectDrumRows:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.RowsRect())
	case SubjectEQPanel:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.eqRect)
	case SubjectEQTabEQ, SubjectEQTabWave, SubjectEQTabSpectrum, SubjectEQTabLevels, SubjectSynthPanel, SubjectSampler:
		if dv == nil || dv.eqPanelZone == nil {
			return image.Rectangle{}, false
		}
		// The whole panel, tab strip included, so the PNG shows the active-tab affordance; Synth and Sampler share it.
		return clamp(dv.eqRect)
	case SubjectChain:
		if dv == nil || dv.eqPanelZone == nil {
			return image.Rectangle{}, false
		}
		sz := dv.eqPanelZone.ChainZone()
		if sz == nil {
			return image.Rectangle{}, false
		}
		return clamp(sz.Rect())
	case SubjectAudioStickyBar:
		if dv == nil || dv.eqPanelZone == nil || dv.eqPanelZone.stickyBar == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.eqPanelZone.stickyBar.Rect())
	case SubjectToolbar:
		if dv == nil {
			return image.Rectangle{}, false
		}
		// The full header strip: WidgetTransport alone is only the play/stop/record cluster.
		return clamp(dv.HeaderRect())
	case SubjectFXPanel:
		if dv == nil {
			return image.Rectangle{}, false
		}
		r, ok := dv.FXPanelRect()
		if !ok {
			return image.Rectangle{}, false
		}
		return clamp(r)
	case SubjectContextMenu:
		if dv == nil || !dv.IsContextMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.contextMenuRect)
	case SubjectOverflowMenu:
		if dv == nil || !dv.IsOverflowMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.OverflowPopupRect())
	case SubjectInstrumentMenu:
		if dv == nil || !dv.IsInstMenuOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.InstrumentMenuRect())
	case SubjectGroupMenu:
		if g.groupMenu == nil || !g.groupMenu.IsOpen() {
			return image.Rectangle{}, false
		}
		return clamp(g.groupMenu.InputBounds())
	case SubjectNavMenu:
		if dv == nil || dv.navMenuComp == nil || !dv.navMenuComp.IsOpen() {
			return image.Rectangle{}, false
		}
		return clamp(dv.navMenuComp.Bounds())
	case SubjectThemeEditor:
		if dv == nil || dv.portal() == nil {
			return image.Rectangle{}, false
		}
		if ov, ok := dv.portal().TopOverlay().(*ThemeOverlay); ok && ov != nil {
			return clamp(ov.Rect())
		}
		return image.Rectangle{}, false
	case SubjectWarmupToast:
		if dv == nil {
			return image.Rectangle{}, false
		}
		return clamp(dv.logsToast.rectIn(g.toastHostRect()))
	}
	return image.Rectangle{}, false
}

// "" is SubjectFullScreen; unknown names return !ok.
func SubjectByName(name string) (Subject, bool) {
	if name == "" {
		return SubjectFullScreen, true
	}
	for _, s := range AllSubjects() {
		if string(s) == name {
			return s, true
		}
	}
	return "", false
}
