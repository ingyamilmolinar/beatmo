package ui

import "github.com/ingyamilmolinar/beatmo/internal/i18n"

// PanelTab identifies one of the tabs in the EQ/waveform/spectrum/meters panel.
type PanelTab int

const (
	TabWave PanelTab = iota
	TabSpectrum
	TabMeters
	TabEQ
	TabScope
	TabSynth   // synth parameter editor for the selected row
	TabSampler // sample editor: trim/pitch a WAV or synth-capture, save as an instrument
)

// Chain is a developer tool and is omitted in production. Package-level slices so per-frame callers
// allocate nothing; read-only.
func AllPanelTabs() []PanelTab {
	if devToolsOn() {
		return allPanelTabsDev
	}
	return allPanelTabsProduction
}

var (
	allPanelTabsDev        = []PanelTab{TabEQ, TabWave, TabSpectrum, TabMeters, TabScope, TabSynth, TabSampler}
	allPanelTabsProduction = []PanelTab{TabEQ, TabWave, TabSpectrum, TabMeters, TabSynth, TabSampler}
)

func panelTabAvailable(tab PanelTab) bool { return tab != TabScope || devToolsOn() }

// "Levels" and "Chain" are the words consumer DAWs use; "Mtr" and "Scope" were jargon.
func PanelTabLabel(tab PanelTab) string {
	switch tab {
	case TabWave:
		return i18n.T(i18n.KeyTabWave)
	case TabSpectrum:
		return i18n.T(i18n.KeyTabSpectrum)
	case TabMeters:
		return i18n.T(i18n.KeyTabLevels)
	case TabEQ:
		return i18n.T(i18n.KeyTabEQ)
	case TabScope:
		return i18n.T(i18n.KeyTabChain)
	case TabSynth:
		return i18n.T(i18n.KeyTabSynth)
	case TabSampler:
		return i18n.T(i18n.KeyTabSampler)
	default:
		return "?"
	}
}

// Mobile abbreviations let every tab fit a 390 px bar beside the channel and filter controls.
func PanelTabLabelForProfile(tab PanelTab) string {
	if Profile().IsMobile() {
		switch tab {
		case TabWave:
			return i18n.T(i18n.KeyTabWaveShort)
		case TabSpectrum:
			return i18n.T(i18n.KeyTabSpectrumShort)
		case TabMeters:
			return i18n.T(i18n.KeyTabLevelsShort)
		case TabEQ:
			return i18n.T(i18n.KeyTabEQShort)
		case TabScope:
			return i18n.T(i18n.KeyTabChainShort)
		case TabSynth:
			return i18n.T(i18n.KeyTabSynthShort)
		case TabSampler:
			return i18n.T(i18n.KeyTabSamplerShort)
		default:
			return "?"
		}
	}
	return PanelTabLabel(tab)
}

// Slugs are stable ids for scenes, screenshot subjects and the SetActiveEQTab API.
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

type PanelTabState struct {
	activeTab PanelTab
	expanded  bool
	// onLeave fires on every real tab change: every switch path ends in SetActiveTab, so this is the one place
	// to flush state that must not survive a tab change (the synth wheel-nudge commit).
	onLeave func(prev PanelTab)
}

func NewPanelTabState() *PanelTabState {
	return &PanelTabState{activeTab: TabEQ}
}

// ActiveTab returns the currently selected tab.
func (p *PanelTabState) ActiveTab() PanelTab { return p.activeTab }

func (p *PanelTabState) SetOnLeave(fn func(prev PanelTab)) { p.onLeave = fn }

// A tab the build does not offer (Chain in production) is refused here, the chokepoint every route reaches.
func (p *PanelTabState) SetActiveTab(tab PanelTab) {
	if !panelTabAvailable(tab) {
		return
	}
	prev := p.activeTab
	p.activeTab = tab
	if prev != tab && p.onLeave != nil {
		p.onLeave(prev)
	}
}

// Expanded returns true when the panel is in its expanded (2x) state.
func (p *PanelTabState) Expanded() bool { return p.expanded }

// ToggleExpanded flips the expand/collapse state.
func (p *PanelTabState) ToggleExpanded() { p.expanded = !p.expanded }

// Scaled from eqPanelHeight by RuntimeProfile so the analysis tabs have room for legible scales; a user
// drag-resize still overrides. PanelHeightAt adds the screen cap.
func (p *PanelTabState) PanelHeight() int {
	return p.PanelHeightAt(0)
}

// screenH ≤ 0 disables the cap.
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
	// Synth knob cards need more vertical room than the other tabs, so a taller floor.
	if p.activeTab == TabSynth {
		h += eqPanelHeight
	}
	if screenH > 0 && frac > 0 {
		cap := int(float64(screenH) * frac)
		if cap > 0 && h > cap {
			h = cap
		}
	}
	return h
}
