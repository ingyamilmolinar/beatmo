//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A miss records a render; the identical second trigger hits. Makes the param-storm cost falsifiable.
func TestRenderLatencyMetrics(t *testing.T) {
	const (
		id = roster.KickAcoustic
		sr = 44100
	)
	// A live per-instrument param override forces the recipe render path (the
	// real param-storm scenario); without it an as-shipped instrument takes the
	// legacy fast path and never renders through the cache.
	SetInstrumentParam(id, "gen1_kick_reverb", 0.9)
	defer ResetInstrumentParams(id)

	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	if v := newRecipeAwareVoice(id, sr); v == nil {
		t.Fatal("newRecipeAwareVoice returned nil")
	}
	m := GetRenderLatencyMetrics()
	if m.CacheMisses < 1 {
		t.Fatalf("first trigger should be a cache miss (a render); got %+v", m)
	}
	if m.RenderAvgNS <= 0 || m.RenderMaxNS <= 0 {
		t.Fatalf("a cache-miss render should record a positive duration; got %+v", m)
	}

	missesBefore := m.CacheMisses
	if v := newRecipeAwareVoice(id, sr); v == nil {
		t.Fatal("second newRecipeAwareVoice returned nil")
	}
	m2 := GetRenderLatencyMetrics()
	if m2.CacheHits < 1 {
		t.Fatalf("identical second trigger should be a cache hit; got %+v", m2)
	}
	if m2.CacheMisses != missesBefore {
		t.Fatalf("a cache hit must not re-render: misses %d -> %d", missesBefore, m2.CacheMisses)
	}

	ResetRenderLatencyMetrics()
	if got := GetRenderLatencyMetrics(); got != (RenderLatencyMetrics{}) {
		t.Fatalf("reset did not clear counters: %+v", got)
	}
}
