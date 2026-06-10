package ui

// PanelTab identifies one of the tabs in the EQ/waveform/spectrum/meters panel.
type PanelTab int

const (
	TabWave     PanelTab = iota
	TabSpectrum
	TabMeters
	TabEQ
	TabScope
	TabSynth   // Phase 4 redirect: synth-recipe parameter editor for the selected row
	TabSampler // sample editor: trim/pitch a WAV or synth-capture, save as an instrument
)

// AllPanelTabs returns all tabs in display order. Synth is appended at the
// end of the strip so existing scene catalogs / pixel tests that anchor on
// the legacy 5-tab layout stay valid; mobile uses an abbreviated label.
func AllPanelTabs() []PanelTab {
	return []PanelTab{TabEQ, TabWave, TabSpectrum, TabMeters, TabScope, TabSynth, TabSampler}
}

// PanelTabLabel returns the human-readable label for a tab.
//
// TabMeters renders as "Levels" — every consumer DAW (Logic, GarageBand,
// FL, Ableton) uses "Levels" for the peak/RMS meter view. "Mtr" was jargon.
//
// TabScope renders as "Chain" — communicates "your sound's journey through
// effects" instead of the engineer-jargon "Scope". The underlying 6 pipeline
// stages (Synth · Click guard · FX · EQ · Bus · Master) are exposed as
// thumbnails inside the tab; see chain_panel_zone.go.
func PanelTabLabel(tab PanelTab) string {
	switch tab {
	case TabWave:
		return "Wave"
	case TabSpectrum:
		return "Spectrum"
	case TabMeters:
		return "Levels"
	case TabEQ:
		return "EQ"
	case TabScope:
		return "Chain"
	case TabSynth:
		return "Synth"
	case TabSampler:
		return "Sampler"
	default:
		return "?"
	}
}

// PanelTabLabelForProfile returns the tab label sized for the active screen
// class. Mobile uses 3–4 char abbreviations so all five tabs fit in a 390px
// audio-view bar alongside the channel button + HPF/LPF/freeze controls.
func PanelTabLabelForProfile(tab PanelTab) string {
	if Profile().IsMobile() {
		switch tab {
		case TabWave:
			return "Wave"
		case TabSpectrum:
			return "Spec"
		case TabMeters:
			return "Lvl"
		case TabEQ:
			return "EQ"
		case TabScope:
			return "Chn"
		case TabSynth:
			return "Syn"
		case TabSampler:
			return "Smpl"
		default:
			return "?"
		}
	}
	return PanelTabLabel(tab)
}

// PanelTabSlug returns the canonical lowercase slug for a tab. Slugs are
// stable identifiers used by scene catalogs, screenshot subjects, and the
// SetActiveEQTab API. Both the new and legacy slugs route through
// SetActiveEQTab — see game_eq_tab.go.
func PanelTabSlug(tab PanelTab) string {
	switch tab {
	case TabWave:
		return "wave"
	case TabSpectrum:
		return "spectrum"
	case TabMeters:
		return "levels"
	case TabEQ:
		return "eq"
	case TabScope:
		return "chain"
	case TabSynth:
		return "synth"
	case TabSampler:
		return "sampler"
	default:
		return ""
	}
}

// PanelTabState holds the currently active tab and the expand/collapse state
// of the bottom panel. This is the data model that will eventually drive the
// tab bar UI (Task 15). For now it replaces the boolean waveformMode toggle.
type PanelTabState struct {
	activeTab PanelTab
	expanded  bool
}

// NewPanelTabState creates a PanelTabState with default values:
// TabEQ active, collapsed.
func NewPanelTabState() *PanelTabState {
	return &PanelTabState{activeTab: TabEQ}
}

// ActiveTab returns the currently selected tab.
func (p *PanelTabState) ActiveTab() PanelTab { return p.activeTab }

// SetActiveTab switches to the given tab.
func (p *PanelTabState) SetActiveTab(tab PanelTab) { p.activeTab = tab }

// Expanded returns true when the panel is in its expanded (2x) state.
func (p *PanelTabState) Expanded() bool { return p.expanded }

// ToggleExpanded flips the expand/collapse state.
func (p *PanelTabState) ToggleExpanded() { p.expanded = !p.expanded }

// PanelHeight returns the panel height in pixels.
//
// Sized through RuntimeProfile (AudioPanelHeightMultiplier × eqPanelHeight)
// so the Spectrum/Levels/Chain/Synth analysis surfaces have room for legible
// scales, channel strips, signal-flow cards, and synth knob cards. Default
// 3×; bound by AudioPanelHeightScreenFrac of the framebuffer height when a
// screenH > 0 is supplied so the panel never crowds the grid. User
// drag-resize (RowHeight(2) in drumview_geometry.go) still overrides.
//
// PanelHeightAt is the screen-aware variant; callers that don't know the
// framebuffer pass 0 to opt out of the cap (legacy behaviour).
func (p *PanelTabState) PanelHeight() int {
	return p.PanelHeightAt(0)
}

// PanelHeightAt returns the panel height clamped to a fraction of screenH
// (the framebuffer height in pixels). screenH ≤ 0 disables the cap.
func (p *PanelTabState) PanelHeightAt(screenH int) int {
	mult := 2
	frac := 0.0
	if rp := RuntimeProf(); rp != nil {
		if rp.AudioPanelHeightMultiplier > 0 {
			mult = rp.AudioPanelHeightMultiplier
		}
		if rp.AudioPanelHeightScreenFrac > 0 {
			frac = rp.AudioPanelHeightScreenFrac
		}
	}
	h := eqPanelHeight * mult
	if screenH > 0 && frac > 0 {
		cap := int(float64(screenH) * frac)
		if cap > 0 && h > cap {
			h = cap
		}
	}
	return h
}
