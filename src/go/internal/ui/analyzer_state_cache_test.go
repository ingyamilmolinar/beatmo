//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Rebuilt only past stateCacheTTL or on a new key (activeID, rowsFP, sampleRate), capping State construction at 30 Hz
// whatever the draw rate: the main OOM guard for the EQ panel on WASM.
func TestCachedAnalyzerState_TTLHonored(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	var builds int
	build := func() *analyzer.State { builds++; return probe }

	// Cold cache: first call must build.
	if got := cachedAnalyzerState(channel.Master, 1, 48000, build); got != probe {
		t.Fatalf("first call returned %p, want %p", got, probe)
	}
	if builds != 1 {
		t.Fatalf("expected 1 build after cold call, got %d", builds)
	}

	// Many calls within TTL: must hit cache.
	for i := 0; i < 200; i++ {
		_ = cachedAnalyzerState(channel.Master, 1, 48000, build)
	}
	if builds != 1 {
		t.Fatalf("expected build count to stay at 1 within TTL, got %d "+
			"— cache TTL is not gating the rebuild path", builds)
	}

	// Different rowsFP: must rebuild even within TTL.
	_ = cachedAnalyzerState(channel.Master, 2, 48000, build)
	if builds != 2 {
		t.Fatalf("expected rebuild on rowsFP change, got builds=%d", builds)
	}

	// Different activeID: must rebuild.
	_ = cachedAnalyzerState(channel.Of(roster.Kick), 2, 48000, build)
	if builds != 3 {
		t.Fatalf("expected rebuild on activeID change, got builds=%d", builds)
	}
}

// A hit returns the stored pointer: zero allocations.
func TestCachedAnalyzerState_HitAllocBudget(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	build := func() *analyzer.State { return probe }
	// Prime the cache.
	_ = cachedAnalyzerState(channel.Master, 7, 48000, build)

	const budget = 0.0
	allocs := testing.AllocsPerRun(1000, func() {
		_ = cachedAnalyzerState(channel.Master, 7, 48000, build)
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

// Keyed on row content, not slice identity, so a fresh copy of the same rows still hits.
func TestRowsFingerprint_StableForSameRows(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: roster.Kick, Name: "Kick"},
		{Instrument: roster.Snare, Name: "Snare"},
		{Instrument: roster.Hihat, Name: "Hat"},
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

// Or an instrument swap serves stale state.
func TestRowsFingerprint_ChangesOnRename(t *testing.T) {
	rows := []*DrumRow{{Instrument: roster.Kick, Name: "Kick"}}
	base := rowsFingerprint(rows)
	rows[0].Name = "Bass Drum"
	if got := rowsFingerprint(rows); got == base {
		t.Errorf("fingerprint unchanged after rename: base=%x got=%x", base, got)
	}
	rows[0].Name = "Kick"
	rows[0].Instrument = roster.KickSnap
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

	_ = cachedScopeState(channel.Of(roster.Kick), scope.StageSynth, scope.StageMaster, build)
	for i := 0; i < 100; i++ {
		_ = cachedScopeState(channel.Of(roster.Kick), scope.StageSynth, scope.StageMaster, build)
	}
	if builds != 1 {
		t.Fatalf("expected 1 build, got %d", builds)
	}
	// Different tap selection invalidates.
	_ = cachedScopeState(channel.Of(roster.Kick), scope.StageEQ, scope.StageMaster, build)
	if builds != 2 {
		t.Fatalf("expected rebuild on tap change, got %d", builds)
	}
}

// The TTL is 33 ms, short enough to sleep through.
func TestCachedAnalyzerState_TTLExpiry(t *testing.T) {
	t.Cleanup(InvalidateAnalyzerStateCacheForTest)
	InvalidateAnalyzerStateCacheForTest()

	probe := &analyzer.State{}
	var builds int
	build := func() *analyzer.State { builds++; return probe }
	_ = cachedAnalyzerState(channel.Master, 9, 48000, build)
	if builds != 1 {
		t.Fatalf("setup: expected 1 build, got %d", builds)
	}
	time.Sleep(stateCacheTTL + 10*time.Millisecond)
	_ = cachedAnalyzerState(channel.Master, 9, 48000, build)
	if builds != 2 {
		t.Fatalf("expected rebuild after TTL elapsed, got %d", builds)
	}
}
