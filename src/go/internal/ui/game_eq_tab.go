package ui

import (
	"fmt"
	"strings"
)

// SetActiveEQTab switches the EQ panel tab. Accepts canonical slugs
// ("eq", "wave", "spectrum", "levels", "chain", "synth", "sampler") and legacy
// aliases ("meters" → levels, "scope" → chain). Mirrors the setEQTab JS export.
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

// ActiveEQTab returns the canonical lowercase slug for the current EQ
// panel tab. Slugs: eq · wave · spectrum · levels · chain · synth · sampler.
func (g *Game) ActiveEQTab() string {
	if g.drum == nil || g.drum.eqPanelZone == nil {
		return ""
	}
	return PanelTabSlug(g.drum.eqPanelZone.tabState.ActiveTab())
}
