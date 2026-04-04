package ui

// PanelTab identifies one of the tabs in the EQ/waveform/spectrum/meters panel.
type PanelTab int

const (
	TabWave     PanelTab = iota
	TabSpectrum
	TabMeters
	TabEQ
)

// AllPanelTabs returns all tabs in display order.
func AllPanelTabs() []PanelTab {
	return []PanelTab{TabWave, TabSpectrum, TabMeters, TabEQ}
}

// PanelTabLabel returns the human-readable label for a tab.
func PanelTabLabel(tab PanelTab) string {
	switch tab {
	case TabWave:
		return "Wave"
	case TabSpectrum:
		return "Spectrum"
	case TabMeters:
		return "Meters"
	case TabEQ:
		return "EQ"
	default:
		return "?"
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
// Collapsed: eqPanelHeight. Expanded: 2 * eqPanelHeight.
func (p *PanelTabState) PanelHeight() int {
	if p.expanded {
		return eqPanelHeight * 2
	}
	return eqPanelHeight
}
