//go:build !test && !js

package ui

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// What a live synth/sampler edit costs the audio dispatch goroutine during playback: an edit rotates the voice
// cache key, and a miss rendered synchronously made beats late. Untagged (the -tags test synth is a stub), and
// never SetPlayFunc, which stubs the render it measures.

// A control dragged at ~60 Hz while the sequencer runs.
type churnFn func(t *testing.T, g *Game, id string, step int)

type paramEditScenario struct {
	name  string
	churn churnFn // nil = baseline (no edits)
}

// From the document's declared fields, so schema changes do not break it; "pitch" is special-cased into the key.
func churnParamName(t *testing.T, id string) string {
	t.Helper()
	merged := audio.MergedInstrumentParamsRO(instrument.ID(id))
	keys := make([]string, 0, len(merged))
	for k := range merged {
		if k == "pitch" || !audio.InstrumentDeclaresField(instrument.ID(id), k) {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		t.Fatalf("instrument %q has no draggable params", id)
	}
	sort.Strings(keys)
	return keys[0]
}

func TestParamEditPlaybackCostTable(t *testing.T) {
	const instID = roster.OrganChurch // a pricey melodic render → worst case
	const steps = 240                 // playback steps per scenario

	knobParam := churnParamName(t, string(instID))
	t.Logf("dragging synth param %q on %s", knobParam, instID)

	scenarios := []paramEditScenario{
		{name: "baseline", churn: nil},
		{
			name: "synth-control-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// Sweep a real param, new value every step → rotates the key.
				v := float64(step%100) / 100.0
				audio.SetInstrumentParam(instrument.ID(id), knobParam, v)
			},
		},
		{
			name: "sampler-edit-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// GainDB is float32, the fracs float64; an identity edit becomes a Clear, which this exercises on purpose.
				audio.SetSampleEdit(instrument.ID(id), audio.SampleEdit{
					StartFrac: 0,
					EndFrac:   1.0 - float64(step%20)*0.01,
					GainDB:    -float32(step % 6),
				})
			},
		},
		{
			// The control: insert-FX params are live per block and not in the voice binding, so this must look like the
			// baseline; renders or misses here mean FX params leaked into the binding.
			name: "fx-param-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				if step == 1 {
					audio.AddInsertEffect(instrument.ID(id), audio.EffectDistortion,
						map[string]float64{"drive": 1.4, "tone": 8000, "mix": 1})
				}
				audio.SetInsertEffectParam(instrument.ID(id), 0, "mix", float64(step%100)/100.0)
			},
		},
	}

	// The table is evidence, not output: t.Log shows it with -v or on failure.
	var table strings.Builder
	t.Cleanup(func() { t.Log(table.String()) })
	fmt.Fprintf(&table, "\n=== param-edit during playback: audio-dispatch cost (%s, %d steps) ===\n", instID, steps)
	fmt.Fprintf(&table, "%-20s %-10s %-10s %-10s %-12s %-12s %-12s\n",
		"scenario", "triggers", "dispatchR", "prewarmR", "missRatio", "renderAvgMS", "renderMaxMS")

	for _, sc := range scenarios {
		// Or the previous scenario's edits leak in.
		audio.ResetInstrumentParams(instID)
		audio.ClearSampleEdit(instID)
		audio.ResetAllInsertEffects()
		// The voice cache is process-global; without a reset each scenario's numbers depend on what ran before.
		audio.ClearVoiceCacheForTest()

		g := newPitchedRowCircuitGame(t, string(instID), 0, 5, -2, 7, -5, 12, 3, -7)
		g.SetPlaying(true)
		advanceFrames(g, 5) // let playback settle before measuring

		start := g.elapsedBeats
		if start < 0 {
			start = 0
		}

		// Warm, as a user plays the demo before touching a knob, so first-play cost is not counted as edit cost.
		const warmupSteps = 60
		for i := 1; i <= warmupSteps; i++ {
			setPlayStartForAbs(g, start+i)
			_ = g.Update()
		}
		start += warmupSteps

		// A warm cache is the realistic start.
		audio.ResetRenderLatencyMetrics()

		for i := 1; i <= steps; i++ {
			setPlayStartForAbs(g, start+i)
			_ = g.Update()
			if sc.churn != nil {
				sc.churn(t, g, string(instID), i)
			}
		}

		m := audio.GetRenderLatencyMetrics()
		triggers := m.CacheHits + m.CacheMisses
		missRatio := 0.0
		if triggers > 0 {
			missRatio = float64(m.CacheMisses) / float64(triggers)
		}

		fmt.Fprintf(&table, "%-20s %-10d %-10d %-10d %-12.3f %-12.3f %-12.3f\n",
			sc.name, triggers, m.DispatchRenders, m.PrewarmRenders, missRatio,
			float64(m.RenderAvgNS)/1e6, float64(m.RenderMaxNS)/1e6)

		// An edit storm must not scale dispatch renders with edit rate; the ceiling catches a gross regression without
		// flaking on cold-start renders.
		if m.DispatchRenders > 15 {
			t.Errorf("%s: %d dispatch renders — edit storm is leaking onto the "+
				"audio dispatch goroutine (best-available fallback not absorbing "+
				"edit-induced misses)", sc.name, m.DispatchRenders)
		}

		stopPlaybackForTest(g)
	}

	// missRatio and triggers are evidence, never a gate: audioLoop races this tight Update loop, so the same no-edit
	// baseline varied 0.67/0.18/0.09 across runs. DispatchRenders is the gate.
}
