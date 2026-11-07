package ui

import (
	"fmt"
	"strings"
)

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
	case "levels", "meters":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)
	case "chain", "scope":
		if !panelTabAvailable(TabScope) {
			return fmt.Errorf("EQ tab %q is not available in this build", name)
		}
		g.drum.eqPanelZone.tabState.SetActiveTab(TabScope)
		g.drum.bgDirty = true
	case "synth":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabSynth)
	case "sampler":
		g.drum.eqPanelZone.tabState.SetActiveTab(TabSampler)
	default:
		return fmt.Errorf("unknown EQ tab: %q", name)
	}
	return nil
}

func (g *Game) ActiveEQTab() string {
	if g.drum == nil || g.drum.eqPanelZone == nil {
		return ""
	}
	return PanelTabSlug(g.drum.eqPanelZone.tabState.ActiveTab())
}
