//go:build !test

package ui

// Production-only knobs for the screenshot harness; the stub keeps them off in test builds.

func (g *Game) SetDefaultStart(b bool) { enableDefaultStart = b }

func (g *Game) SetForceMobileProfile(b bool) {
	forceSmallScreenForTest = b
	UpdateProfile()
}

// So screenshot scenes lay out as in production.
func (g *Game) SetForceAutoSize(b bool) { forceAutoSize = b }
