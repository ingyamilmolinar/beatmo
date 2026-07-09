package ui

import (
	"strings"
	"testing"
)

// newTestGameForParity builds a fully-initialized Game suitable for exercising
// parityTimingCheck / parity dump helpers. Thin wrapper over buildTestGame
// (subdiv_safety_test.go) kept as its own name so timing-gate tests read
// self-descriptively and can diverge from the generic UI-test setup later
// without touching unrelated suites.
func newTestGameForParity(t *testing.T) *Game {
	t.Helper()
	return buildTestGame(t)
}

// enableParityTimingForTest arms the timing gate for the duration of a
// dedicated timing test: it forces the global fatal switch on and sets this
// Game's watch mode to panic, then restores BOTH to their PRIOR values on
// cleanup. The gate is dormant-by-default under `go test` (see init() in
// game_parity_timing.go) precisely so that fast-forwarded/synthetic-time
// tests elsewhere in the suite never see it; a hardcoded restore-to-true
// here would re-arm the global for every test that runs afterward and
// reintroduce that failure mode. Only dedicated timing tests should call
// this.
func enableParityTimingForTest(t *testing.T, g *Game) {
	t.Helper()
	prevFatal := parityTimingFatalEnabled.Load()
	prevWatch := g.parityTimingWatch
	parityTimingFatalEnabled.Store(true)
	g.parityTimingWatch = parityWatchPanic
	t.Cleanup(func() {
		parityTimingFatalEnabled.Store(prevFatal)
		g.parityTimingWatch = prevWatch
	})
}

// assertPanics runs fn and asserts it panics with a message containing
// wantSubstr. Fails the test if fn does not panic, or panics with a message
// that doesn't contain wantSubstr.
func assertPanics(t *testing.T, wantSubstr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got no panic", wantSubstr)
			return
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected panic containing %q, got non-string panic: %v", wantSubstr, r)
			return
		}
		if !strings.Contains(msg, wantSubstr) {
			t.Fatalf("expected panic message to contain %q, got: %s", wantSubstr, msg)
		}
	}()
	fn()
}
