package ui

import (
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A group row or a name override, never a silent fallback, which once drew volume pictures for non-volume knobs.
func TestEveryVisibleParamHasAnExplicitView(t *testing.T) {
	missing := map[string][]string{}
	for _, id := range roster.All {
		merged := audio.MergedInstrumentParamsRO(id)
		for _, d := range audio.InstrumentParamDefs(id) {
			if d.Group == audio.SynthHiddenGroup || isSynthEnableParam(d.Name) {
				continue
			}
			if _, ok := synthParamView(d, merged); !ok {
				missing[d.Group] = append(missing[d.Group], d.Name)
			}
		}
	}
	var groups []string
	for g := range missing {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		t.Errorf("group %q has no graph view (params %v): add it to synthParamViewGroups", g, missing[g][:min(4, len(missing[g]))])
	}
}
