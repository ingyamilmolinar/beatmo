//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestCachedAnalyzerState_TTLHonored pins the WASM-layer analyzer.State
// cache contract: build() is invoked only when the cache entry is older
// than stateCacheTTL OR the key (activeID, rowsFP, sampleRate) differs.
// This caps per-Draw State construction at 30 Hz regardless of the
// 60-120 fps Draw rate — the dominant OOM-prevention mechanism for the
// EQ panel hot path on WASM.
func TestCachedAnalyzerState_TTLHonored(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	var builds int
	build := func() *analyzer.State { builds++; return probe }

	// Cold cache: first call must build.
	if got := cachedAnalyzerState("main", 1, 48000, build); got != probe {
		t.Fatalf("first call returned %p, want %p", got, probe)
	}
	if builds != 1 {
		t.Fatalf("expected 1 build after cold call, got %d", builds)
	}

	// Many calls within TTL: must hit cache.
	for i := 0; i < 200; i++ {
		_ = cachedAnalyzerState("main", 1, 48000, build)
	}
	if builds != 1 {
		t.Fatalf("expected build count to stay at 1 within TTL, got %d "+
			"— cache TTL is not gating the rebuild path", builds)
	}

	// Different rowsFP: must rebuild even within TTL.
	_ = cachedAnalyzerState("main", 2, 48000, build)
	if builds != 2 {
		t.Fatalf("expected rebuild on rowsFP change, got builds=%d", builds)
	}

	// Different activeID: must rebuild.
	_ = cachedAnalyzerState("kick", 2, 48000, build)
	if builds != 3 {
		t.Fatalf("expected rebuild on activeID change, got builds=%d", builds)
	}
}

// TestCachedAnalyzerState_HitAllocBudget pins the per-cached-hit
// allocation count to 0. The cache returns the stored pointer directly
// — no allocation, just a mutex acquire + pointer read.
func TestCachedAnalyzerState_HitAllocBudget(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	build := func() *analyzer.State { return probe }
	// Prime the cache.
	_ = cachedAnalyzerState("main", 7, 48000, build)

	const budget = 0.0
	allocs := testing.AllocsPerRun(1000, func() {
		_ = cachedAnalyzerState("main", 7, 48000, build)
	})
	if allocs > budget {
		t.Fatalf("cache-hit allocs/call = %.1f exceeds budget %.0f — "+
			"a code path is allocating on the cache-hit return; "+
			"check that build() is genuinely skipped on hit",
			allocs, budget)
	}
	t.Logf("cached analyzer state hit allocs/call = %.1f (budget %.0f)",
		allocs, budget)
}

// TestRowsFingerprint_StableForSameRows asserts the fingerprint stays
// the same when the underlying row contents are unchanged — even when
// the slice header is a new copy. This is the property that lets the
// cache key on row content rather than slice identity, so callers that
// recompute `dv.Rows` every Draw still hit the cache.
func TestRowsFingerprint_StableForSameRows(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Name: "Kick"},
		{Instrument: "snare", Name: "Snare"},
		{Instrument: "hihat", Name: "Hat"},
	}
	rowsCopy := make([]*DrumRow, len(rows))
	for i, r := range rows {
		dup := *r
		rowsCopy[i] = &dup
	}
	if a, b := rowsFingerprint(rows), rowsFingerprint(rowsCopy); a != b {
		t.Errorf("fingerprint changed across distinct slice instances with "+
			"identical content: a=%x b=%x", a, b)
	}
}

// TestRowsFingerprint_ChangesOnRename asserts the fingerprint shifts
// when any row's instrument or name changes. Critical so the cache
// doesn't serve stale state after an instrument swap mid-session.
func TestRowsFingerprint_ChangesOnRename(t *testing.T) {
	rows := []*DrumRow{{Instrument: "kick", Name: "Kick"}}
	base := rowsFingerprint(rows)
	rows[0].Name = "Bass Drum"
	if got := rowsFingerprint(rows); got == base {
		t.Errorf("fingerprint unchanged after rename: base=%x got=%x", base, got)
	}
	rows[0].Name = "Kick"
	rows[0].Instrument = "kick-1"
	if got := rowsFingerprint(rows); got == base {
		t.Errorf("fingerprint unchanged after instrument swap: base=%x got=%x", base, got)
	}
}

// TestCachedScopeState_TTLHonored pins the scope.State cache contract.
func TestCachedScopeState_TTLHonored(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &scope.State{}
	var builds int
	build := func() *scope.State { builds++; return probe }

	_ = cachedScopeState("kick", scope.StageSynth, scope.StageMaster, build)
	for i := 0; i < 100; i++ {
		_ = cachedScopeState("kick", scope.StageSynth, scope.StageMaster, build)
	}
	if builds != 1 {
		t.Fatalf("expected 1 build, got %d", builds)
	}
	// Different tap selection invalidates.
	_ = cachedScopeState("kick", scope.StageSends, scope.StageMaster, build)
	if builds != 2 {
		t.Fatalf("expected rebuild on tap change, got %d", builds)
	}
}

// TestCachedAnalyzerState_TTLExpiry verifies the cache rebuilds after
// the TTL elapses. Uses a small artificial sleep — the actual TTL is
// 33 ms which is safe for a unit test.
func TestCachedAnalyzerState_TTLExpiry(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	var builds int
	build := func() *analyzer.State { builds++; return probe }
	_ = cachedAnalyzerState("main", 9, 48000, build)
	if builds != 1 {
		t.Fatalf("setup: expected 1 build, got %d", builds)
	}
	time.Sleep(stateCacheTTL + 10*time.Millisecond)
	_ = cachedAnalyzerState("main", 9, 48000, build)
	if builds != 2 {
		t.Fatalf("expected rebuild after TTL elapsed, got %d", builds)
	}
}
