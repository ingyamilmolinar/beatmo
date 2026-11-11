package ui

import "sync/atomic"

// devtools.go — the ONE switch between the developer UI and the production UI
// beatmo.io ships (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md).
//
// Developer-only surfaces — the Chain tab, the Synth tab's low-level controls
// (pipeline strip, "+ Add", stage and category drills, "More", stage pills) —
// consult devToolsOn() and nothing else. They stay COMPILED into every build
// (user decision 2026-10-24: hidden, not removed), so devToolsBuild is a var
// set by a build-tagged file (devtools_build_dev.go / devtools_build_release.go),
// never a const the linker could fold away.
//
// devToolsForcedOff shows the production UI in a developer build: the
// `-devtools=false` flag (desktop: screenshots, the tour), `?devtools=0` (dev
// WASM), and tests (forceProductionUIForTest). Set it BEFORE ui.New: the tab
// strip and the phone's bottom nav are built from the available lists at
// construction.

var devToolsForcedOff atomic.Bool

// devToolsOn reports whether the developer-only surfaces are available.
func devToolsOn() bool { return devToolsBuild && !devToolsForcedOff.Load() }

// SetDevToolsForcedOff hides (true) or restores (false) the developer-only
// surfaces in a developer build. A release build has none to restore.
func SetDevToolsForcedOff(off bool) { devToolsForcedOff.Store(off) }

// DevToolsOn is devToolsOn for callers outside the package (cmd, tests).
func DevToolsOn() bool { return devToolsOn() }
