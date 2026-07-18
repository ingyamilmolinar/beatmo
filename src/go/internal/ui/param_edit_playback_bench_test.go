//go:build !test && !js

package ui

import (
	"fmt"
	"sort"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// param_edit_playback_bench_test.go — measures what a live synth-knob / sampler
// edit costs the AUDIO DISPATCH GOROUTINE during real playback.
//
// Scenario (field report): play the startup demo, drag a synth knob or change a
// sampler setting → audio cracks. The scheduler is innocent; the cost is that a
// param edit rotates the voice-cache key (hashRecipeParams covers the FULL
// merged param set), so the next trigger is a guaranteed cache MISS, and the
// miss renders SYNCHRONOUSLY on the audioLoop dispatch goroutine
// (batch_desktop.go's PlayBatch is a plain serial loop). recipe.Render is a
// ~24ms CGo call for organ-church. That does NOT underrun the oto device (a
// separate goroutine) — it makes beats LATE, which is the audible crack.
//
// MUST be untagged: under -tags test the synth is stubbed and no real render
// happens. MUST NOT call g.SetPlayFunc: that stubs the trigger and the render
// never happens, which would make this benchmark silently measure nothing.
//
// Run:
//   cd src/go && xvfb-run -a ../../.tools/go/bin/go test -count=1 \
//     -run TestParamEditPlaybackCostTable -v -timeout 900s ./internal/ui/

// churnFn mutates instrument state once per playback step, mimicking a user
// dragging a control at ~60Hz while the sequencer runs.
type churnFn func(t *testing.T, g *Game, id string, step int)

type paramEditScenario struct {
	name  string
	churn churnFn // nil = baseline (no edits)
}

// churnParamName picks a deterministic, real param of the instrument's recipe
// to drag. Derived from the recipe defaults rather than hardcoded, so this
// keeps working when the instrument's param schema changes. "pitch" is excluded
// because it is special-cased into the cache key.
func churnParamName(t *testing.T, id string) string {
	t.Helper()
	merged := audio.MergeRecipeDefaults(audio.RecipeForInstrument(id), nil)
	keys := make([]string, 0, len(merged))
	for k := range merged {
		if k == "pitch" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		t.Fatalf("instrument %q has no draggable recipe params", id)
	}
	sort.Strings(keys)
	return keys[0]
}

func TestParamEditPlaybackCostTable(t *testing.T) {
	const instID = "organ-church" // the priciest render in the demo → worst case
	const steps = 240             // playback steps per scenario

	knobParam := churnParamName(t, instID)
	t.Logf("dragging synth param %q on %s", knobParam, instID)

	scenarios := []paramEditScenario{
		{name: "baseline", churn: nil},
		{
			name: "synth-knob-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// Sweep a real param, new value every step → rotates the key.
				v := float64(step%100) / 100.0
				audio.SetInstrumentParam(id, knobParam, v)
			},
		},
		{
			name: "sampler-edit-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// NOTE: SampleEdit.GainDB is float32 (sample_edit.go:22) while
				// StartFrac/EndFrac are float64 — do not unify the casts.
				// Also note SetSampleEdit treats an identity edit as a Clear, so
				// the (step%20==0 && step%6==0) case exercises ClearSampleEdit
				// too. That is intentional coverage, not a bug.
				audio.SetSampleEdit(id, audio.SampleEdit{
					StartFrac: 0,
					EndFrac:   1.0 - float64(step%20)*0.01,
					GainDB:    -float32(step % 6),
				})
			},
		},
		{
			// CONTROL scenario. Insert-FX params are LIVE per-block (they mutate
			// the running C instance via SetParam) and are NOT part of the voice
			// binding — so an FX drag must NOT rotate the cache key and must look
			// exactly like `baseline` here. If this scenario ever shows renders or
			// a raised missRatio, something has wrongly coupled FX params into the
			// voice binding.
			//
			// NOTE: this scenario does NOT assert anything about clicks. FX
			// discontinuity (14 of 24 C effects are unsmoothed; SetChannelEQ
			// zeroes biquad state) is the subject of a separate spec — see §6 of
			// the design doc. Gating it here would add a gate with no remediation
			// behind it.
			name: "fx-param-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				if step == 1 {
					audio.AddInsertEffect(id, audio.EffectDistortion,
						map[string]float64{"drive": 1.4, "tone": 8000, "mix": 1})
				}
				audio.SetInsertEffectParam(id, 0, "mix", float64(step%100)/100.0)
			},
		},
	}

	fmt.Printf("\n=== param-edit during playback: audio-dispatch cost (%s, %d steps) ===\n", instID, steps)
	fmt.Printf("%-20s %-10s %-10s %-10s %-12s %-12s %-12s\n",
		"scenario", "triggers", "dispatchR", "prewarmR", "missRatio", "renderAvgMS", "renderMaxMS")

	for _, sc := range scenarios {
		// Each scenario starts from a clean instrument state, or the previous
		// scenario's edits leak in and every binding is already rotated.
		audio.ResetInstrumentParams(instID)
		audio.ClearSampleEdit(instID)
		audio.ResetAllInsertEffects()
		// Cold, identical starting cache: the voice cache is process-global, so
		// without this reset a scenario's numbers depend on whatever ran before
		// it. Measured: the SAME baseline scenario reported 2 / 60 / 51 triggers
		// across three runs, and a fully-warm run reported ZERO renders
		// (renderAvgMS=0.000) because globalVoiceCache was still warm from a
		// prior scenario/run.
		audio.ClearVoiceCacheForTest()

		g := buildTestGame(t)
		if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
			t.Fatalf("%s: import startup demo: %v", sc.name, err)
		}
		g.updateBeatInfos()
		g.SetPlaying(true)
		advanceFrames(g, 5) // let playback settle before measuring

		start := g.elapsedBeats
		if start < 0 {
			start = 0
		}

		// WARMUP (no churn): bring the voice cache + prewarmer to a realistic
		// steady state before measuring. This mirrors the real user, who plays
		// the demo (warming the cache) BEFORE touching a knob — measuring from a
		// stone-cold cache would conflate "first play" render cost with the
		// param-edit cost this benchmark exists to isolate. Combined with the
		// cold-cache reset above, this makes every scenario start from the SAME
		// realistic state instead of an ad hoc leftover from whatever ran before
		// it (see the 2 / 60 / 51 instability cited above).
		const warmupSteps = 60
		for i := 1; i <= warmupSteps; i++ {
			setPlayStartForAbs(g, start+i)
			_ = g.Update()
		}
		start += warmupSteps

		// Do NOT clear the cache here — a warm cache is the realistic starting
		// state for the measured section below.
		audio.ResetRenderLatencyMetrics()

		for i := 1; i <= steps; i++ {
			setPlayStartForAbs(g, start+i)
			_ = g.Update()
			if sc.churn != nil {
				sc.churn(t, g, instID, i)
			}
		}

		m := audio.GetRenderLatencyMetrics()
		triggers := m.CacheHits + m.CacheMisses
		missRatio := 0.0
		if triggers > 0 {
			missRatio = float64(m.CacheMisses) / float64(triggers)
		}

		fmt.Printf("%-20s %-10d %-10d %-10d %-12.3f %-12.3f %-12.3f\n",
			sc.name, triggers, m.DispatchRenders, m.PrewarmRenders, missRatio,
			float64(m.RenderAvgNS)/1e6, float64(m.RenderMaxNS)/1e6)

		// GATE (measure-first, tightened in the eviction-pinning task): with the
		// non-blocking dispatch + best-available fallback, an edit storm must not
		// SCALE dispatch renders with edit rate. Pre-fix, the storms produced
		// 4-44 dispatch renders that grew with editing; now edit-induced misses
		// are absorbed by best-available, leaving only cold-start/eviction
		// renders. A generous ceiling catches a gross regression without flaking
		// on cold-start noise; the eviction-pinning task tightens it from
		// measurement. dispatchR is printed above as the real evidence.
		if m.DispatchRenders > 15 {
			t.Errorf("%s: %d dispatch renders — edit storm is leaking onto the "+
				"audio dispatch goroutine (best-available fallback not absorbing "+
				"edit-induced misses)", sc.name, m.DispatchRenders)
		}

		stopPlaybackForTest(g)
	}

	// missRatio / triggers are RECORDED AS EVIDENCE ONLY and must never become a
	// gate. This was measured, not assumed: the SAME baseline scenario (churn=nil,
	// no edits at all) produced missRatio 0.667 / 0.179 / 0.090 and triggers
	// 9 / 67 / 78 across three consecutive runs. A cold-cache reset plus the
	// 60-step warmup above did NOT stabilise it — the cause is structural, not
	// cache warmth: audioLoop is a real goroutine racing this test's tight
	// Update() loop, so how many beats it drains before the run ends is
	// timing-dependent by construction. Any threshold over `triggers` is a coin
	// flip. Do not re-attempt one.
	//
	// DispatchRenders (asserted above against a generous ceiling) is the sole
	// gate, and is the better one: once dispatch is non-blocking, edit-induced
	// misses are STRUCTURALLY absorbed by best-available regardless of
	// goroutine interleaving — only cold-start/eviction renders remain.
}
