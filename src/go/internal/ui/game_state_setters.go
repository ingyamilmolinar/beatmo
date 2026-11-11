//go:build !test

package ui

// game_state_setters.go exposes production-only state knobs the screenshot
// harness uses to boot the game into specific UI states (mobile profile,
// auto-size override, default-start opt-out). The companion
// game_state_setters_stub.go provides no-op stubs for the test build, so
// these knobs cannot accidentally fire in test runs.

// SetDefaultStart toggles whether a default start node is created at (0,0)
// when the game has no nodes after layout. Mirrors SetDefaultStartForTest.
func (g *Game) SetDefaultStart(b bool) { enableDefaultStart = b }

// SetForceMobileProfile forces the layout profile to mobile regardless of
// the detected viewport size. Triggers UpdateProfile so dependent layout
// flags refresh immediately.
func (g *Game) SetForceMobileProfile(b bool) {
	forceSmallScreenForTest = b
	UpdateProfile()
}

// SetForceAutoSize forces Game.Layout to apply its auto-sizing path even
// when running under tests, so screenshot scenes lay out as in production.
func (g *Game) SetForceAutoSize(b bool) { forceAutoSize = b }
