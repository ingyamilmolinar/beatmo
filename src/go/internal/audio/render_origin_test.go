//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// DispatchRenders gates invariant I3 (the dispatch goroutine never renders), so misattribution would make the benchmark lie.
func TestRenderOriginAttribution(t *testing.T) {
	const id = roster.OrganChurch
	const pitch = 3.0

	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Prewarm path → attributed to PrewarmRenders.
	PrewarmVoice(id, pitch)
	m := GetRenderLatencyMetrics()
	if m.PrewarmRenders != 1 {
		t.Fatalf("PrewarmVoice: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}
	if m.DispatchRenders != 0 {
		t.Fatalf("PrewarmVoice must not count as a dispatch render: DispatchRenders=%d want 0", m.DispatchRenders)
	}

	// Dispatch path on an ALREADY-WARM voice → a hit, no new render.
	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Dispatch path on a COLD voice → attributed to DispatchRenders.
	newRecipeAwareVoicePitched(id, SampleRate(), pitch)
	m = GetRenderLatencyMetrics()
	if m.DispatchRenders != 1 {
		t.Fatalf("dispatch cold render: DispatchRenders=%d want 1", m.DispatchRenders)
	}
	if m.PrewarmRenders != 0 {
		t.Fatalf("dispatch render must not count as prewarm: PrewarmRenders=%d want 0", m.PrewarmRenders)
	}

	// Totals stay consistent with the pre-existing miss counter.
	if got := m.DispatchRenders + m.PrewarmRenders; got != m.CacheMisses {
		t.Fatalf("origin split must partition CacheMisses: dispatch+prewarm=%d CacheMisses=%d", got, m.CacheMisses)
	}
}

// The DispatchRenders == 0 gate would false-positive on the knob-release audition the knob-storm scenario triggers.
func TestNonDispatchCallersDoNotInflateDispatchRenders(t *testing.T) {
	const id = roster.OrganChurch

	// Without a user param Play takes the legacy voice, which records no metrics, and the assertion would be vacuous.
	SetInstrumentParam(id, "drive", 0.42)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()
	Play(id) // Synth/Sampler audition — UI goroutine
	if m := GetRenderLatencyMetrics(); m.DispatchRenders != 0 {
		t.Errorf("audio.Play (UI audition) counted %d dispatch render(s), want 0", m.DispatchRenders)
	}
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders == 0 {
		t.Error("audio.Play rendered nothing — this assertion is vacuous. Play must " +
			"actually render (recipe path) for the DispatchRenders==0 check to prove anything.")
	}

	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()
	WarmInstrument(id, DefaultWarmPitches) // UI-goroutine warm
	if m := GetRenderLatencyMetrics(); m.DispatchRenders != 0 {
		t.Errorf("WarmInstrument (UI warm) counted %d dispatch render(s), want 0", m.DispatchRenders)
	}
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders == 0 {
		t.Error("WarmInstrument rendered nothing — test is vacuous, it must actually render to prove attribution")
	}
}

// THE LINCHPIN: if the two key computations drift, prewarm renders into a slot nobody reads and dispatch still
// cold-renders.
func TestPrewarmVoiceDocKeyMatchesDispatch(t *testing.T) {
	const id = roster.OrganChurch
	const pitch = -2.0

	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	p := churnableParam(t, string(id))
	SetInstrumentParam(id, p, 0.375)
	doc, _ := InstrumentDoc(id)

	ResetRenderLatencyMetrics()
	PrewarmVoiceDoc(id, pitch, doc)
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders != 1 {
		t.Fatalf("PrewarmVoiceDoc did not render off-thread: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}

	// Dispatch must now HIT the warmed key.
	ResetRenderLatencyMetrics()
	newRecipeAwareVoicePitched(id, SampleRate(), pitch)
	m := GetRenderLatencyMetrics()
	if m.CacheHits != 1 || m.DispatchRenders != 0 {
		t.Fatalf("warm key != read key: dispatch got hits=%d dispatchRenders=%d "+
			"(want hits=1 dispatchRenders=0). PrewarmVoiceDoc warmed a slot "+
			"dispatch does not read.", m.CacheHits, m.DispatchRenders)
	}
}
