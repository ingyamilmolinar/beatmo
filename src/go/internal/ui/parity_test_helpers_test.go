package ui

import (
	"strings"
	"testing"
)

// Its own name so timing-gate tests read clearly and can diverge from the generic setup.
func newTestGameForParity(t *testing.T) *Game {
	t.Helper()
	return buildTestGame(t)
}

// Restores both switches to their PRIOR values: the gate is dormant under go test so synthetic-time tests never
// see it, and a hardcoded restore-to-true would re-arm it for every later test. Only dedicated timing tests call it.
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
