package ui

import (
	"fmt"
	"strings"
)

// SetActiveEQTab switches the EQ panel tab. Accepts "eq", "wave",
// "spectrum", "meters", "scope". Mirrors the setEQTab JS export.
func (g *Game) SetActiveEQTab(name string) error {
	if g.drum == nil || g.drum.eqPanelZone == nil {
		return fmt.Errorf("EQ panel not initialized")
	}
	switch strings.ToLower(name) {
	case "eq":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabEQ)
	case "wave":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabWave)
	case "spectrum":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabSpectrum)
	case "meters":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	case "scope":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabScope)
		g.drum.bgDirty = true
	default:
		return fmt.Errorf("unknown EQ tab: %q", name)
	}
	return nil
}

// ActiveEQTab returns the current EQ panel tab name.
func (g *Game) ActiveEQTab() string {
	if g.drum == nil || g.drum.eqPanelZone == nil {
		return ""
	}
	switch g.drum.eqPanelZone.tabState.ActiveTab() {
	case TabEQ:
		return "eq"
	case TabWave:
		return "wave"
	case TabSpectrum:
		return "spectrum"
	case TabMeters:
		return "meters"
	case TabScope:
		return "scope"
	}
	return ""
}
