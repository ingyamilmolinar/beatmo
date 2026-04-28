//go:build test

package ui

// Stubbed companions of game_state_setters.go — keep production-only knobs
// off in test builds. Tests that need to flip these still use the
// *ForTest helpers (SetDefaultStartForTest, SetForceAutoSizeForTest).

func (g *Game) SetDefaultStart(bool)        {}
func (g *Game) SetForceMobileProfile(bool)  {}
func (g *Game) SetForceAutoSize(bool)       {}
