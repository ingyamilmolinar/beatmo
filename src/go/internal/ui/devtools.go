package ui

import "sync/atomic"

// The ONE switch between the developer and production UI. Developer surfaces stay COMPILED in (user decision:
// hidden, not removed), so devToolsBuild is a build-tagged var, never a const the linker could fold away.
// devToolsForcedOff must be set BEFORE ui.New: the tab strip and bottom nav are built at construction.

var devToolsForcedOff atomic.Bool

// devToolsOn reports whether the developer-only surfaces are available.
func devToolsOn() bool { return devToolsBuild && !devToolsForcedOff.Load() }

// A release build has nothing to restore.
func SetDevToolsForcedOff(off bool) { devToolsForcedOff.Store(off) }

// DevToolsOn is devToolsOn for callers outside the package (cmd, tests).
func DevToolsOn() bool { return devToolsOn() }
