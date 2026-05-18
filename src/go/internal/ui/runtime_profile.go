package ui

import "time"

// runtime_profile.go — Phase 5b of the design-system refactor.
//
// LayoutProfile (layout_profile.go) historically mixed two distinct
// concerns: (a) profile-conditional design data — sizing, spacing, visual
// style — and (b) profile-conditional runtime behavior — feature flags,
// rendering modes, debug overlays. Phase 5a moved (a)'s dimensional
// portion into DESIGN.md `profileOverrides:` (consumed via
// design_profile.gen.go); Phase 5b documents the boundary for (b) and
// gives those fields a dedicated accessor so future PRs can extract them
// from LayoutProfile without breaking existing consumers.
//
// The fields below are NOT design tokens. They control behavior, not
// appearance:
//   - ShowLayoutGuides, ShowCursorLabel, ShowEscHint — debug overlays
//   - EnableLayoutResize — interactivity toggle (touch can't resize)
//   - ShowRackSurface — visibility toggle
//   - DirectDrawRows — rendering optimization mode (mobile bypass)
//   - UseBottomSheet — UX behavior (sheet vs floating panel)
//   - DefaultTimelineBeats — initial state value
//   - ReserveAddRowSpace — layout behavior
//
// They will never move to DESIGN.md — designers shouldn't be tweaking
// "should we direct-draw rows" or "is the resize splitter active". They
// belong in the Go runtime where they're tested with build tags + env
// vars + integration tests.
//
// RuntimeFlags is the conceptual counterpart to the generated profile
// values. For now it is a read-only view onto the same LayoutProfile
// (zero-cost; no copies). A future PR may extract these fields into
// their own struct, removing them from LayoutProfile entirely; until
// then, the Runtime() accessor exists so new code can already document
// intent ("I'm reading runtime behavior, not design").

// RuntimeFlags exposes the feature/behavior toggle subset of the active
// profile. Use this in new code instead of Profile() when the field
// you're reading controls behavior, not appearance.
type RuntimeFlags struct {
	p *LayoutProfile
}

// Runtime returns the active runtime-flags view. Cheap — no allocation.
func Runtime() RuntimeFlags { return RuntimeFlags{p: Profile()} }

func (r RuntimeFlags) ShowLayoutGuides() bool    { return r.p.ShowLayoutGuides }
func (r RuntimeFlags) ShowCursorLabel() bool     { return r.p.ShowCursorLabel }
func (r RuntimeFlags) ShowEscHint() bool         { return r.p.ShowEscHint }
func (r RuntimeFlags) EnableLayoutResize() bool  { return r.p.EnableLayoutResize }
func (r RuntimeFlags) ShowRackSurface() bool     { return r.p.ShowRackSurface }
func (r RuntimeFlags) DirectDrawRows() bool      { return r.p.DirectDrawRows }
func (r RuntimeFlags) UseBottomSheet() bool      { return r.p.UseBottomSheet }
func (r RuntimeFlags) DefaultTimelineBeats() int { return r.p.DefaultTimelineBeats }
func (r RuntimeFlags) ReserveAddRowSpace() bool  { return r.p.ReserveAddRowSpace }

// ────────────────────────────────────────────────────────────────────────────
// RuntimeProfile — single source of truth for browser/desktop divergence.
// ────────────────────────────────────────────────────────────────────────────
//
// Sibling to LayoutProfile (which owns dimensional/visual divergence between
// mobile/desktop screen classes). RuntimeProfile owns runtime-behavior
// divergence between the two BUILD targets: WASM/browser vs native desktop.
//
// Migrated from defaults_js.go / defaults_notjs.go (Phase B of the
// WASM↔Desktop unification refactor). The build-tagged constants those
// files used left a hole at GOOS=js && -tags test — RuntimeProfile closes it
// because runtime_profile_js.go is //go:build js (no test exclusion).
//
// Rule: no new runtime.GOOS == "js" or runtime.GOARCH == "wasm" checks in
// internal/ui/. Add a field here and read from RuntimeProf() instead.
//
// Tests may swap the entire profile via SetRuntimeProfileForTest, or write
// individual fields directly on the value returned by RuntimeProf().
type RuntimeProfile struct {
	// Cache pads (pixels). Larger pad → fewer rebuilds during pans, at the
	// cost of slightly larger offscreen images.
	EdgeCachePad int
	GridCachePad int

	// Visual quality knobs.
	DisableNodeGlow                    bool
	DisableEdgeArrows                  bool
	SimpleDrawDefault                  bool
	SimpleDrawAutoDisableFramesDefault int
	ScreenEdgesDefault                 bool

	// Throttles (milliseconds; 0 disables).
	TimelineInfoThrottleMS int
	AudioLookaheadSec      float64 // base audio lookahead at profile init

	// Loop tuning.
	AudioBatchMax       int // max events per audio loop iteration
	SequencerTickMS     int // sequencer goroutine cooperative tick
	ParityScanMinPeriod int // floor for parity scan frame period

	// Runtime gates (bool flags that turn behavior on/off).
	AdaptivePanPad             bool // doubles cache pads on fast pan
	FastPanDetect              bool // tracks panFastFrames to gate ensures
	SkipCursorHover            bool // skip Ebiten cursor-hover probe (no-op on WASM)
	PredictorBackoffOnSlowDraw bool // back off predictor lookahead under draw pressure
	ForceInfoLog               bool // force LevelInfo logging at construction
	DefaultPerfFastPath        bool // start with PerfMode fast-path enabled
	EnableTouchSmallScreen     bool // detect small-screen via touch dimensions
	YieldInDrawHelpers         bool // call runtime.Gosched() in maybeYield()

	// Parity defaults.
	ParityFatalDefault bool
	ParityWatchDefault parityWatchMode

	// Identity. Snapshotted at profile construction from runtime.GOOS == "js".
	IsBrowser bool

	// PredictorWindowCap bounds the predictor's per-row sliding-window size
	// (in subdivisions). Reads at idx outside the window return false; the
	// timeline cold archive answers historical queries beyond it. Default
	// 4096 (≈8 visible windows of 512). Identical for browser and desktop;
	// reserved as a knob in case WASM later needs a tighter ceiling under
	// memory pressure.
	PredictorWindowCap int

	// RibbonBeatsPerPixel fixes the timeline ribbon's horizontal scale so
	// long sessions no longer compress the ticks into a forest. The visible
	// window is `barRect.Dx() * RibbonBeatsPerPixel` beats wide, slid so
	// the playhead sits at RibbonPlayheadFrac of the bar width. Visual-
	// readability target (DESIGN.md ribbon spec); not bench-derived. Lower
	// values = denser bars; 0.25 = 4 px/beat default.
	RibbonBeatsPerPixel float64
	// RibbonPlayheadFrac is the playhead's horizontal position inside the
	// visible ribbon window, expressed as a fraction of bar width. 0.70 =
	// 70% from the left, leaving ~30% of the bar for lookahead context.
	RibbonPlayheadFrac float64

	// ForceGCInterval triggers runtime.GC() at most once per interval in
	// heapProbeTick (wall-clock based, NOT frame-count based — under
	// sustained load the browser frame rate drops to ~7-13 fps so a
	// frame-count interval would fire too rarely; the 2026-05-17 soak
	// reached only frame=310 after 36 s wall-clock). Browser default
	// 30s: single-threaded WASM Go GC starves under sustained audio +
	// parameter-dispatch load — the 75 s synth-tab soak grew HeapAlloc
	// 26.7 MB → 493 MB with gcCount=0 across 300 s. Forcing GC at the
	// heap-probe yield point reclaims the cycle every interval;
	// HeapAlloc oscillates around the live-data baseline (~50 MB)
	// instead of growing linearly to the 2 GB WASM ceiling. Desktop
	// default 0 (off): the desktop Go runtime has a real GC scheduler
	// thread and does not need cooperative help.
	ForceGCInterval time.Duration
}

var activeRuntimeProfile *RuntimeProfile

// RuntimeProf returns the active runtime profile, lazily constructing it on
// first access. Mirrors the LayoutProfile singleton pattern.
func RuntimeProf() *RuntimeProfile {
	if activeRuntimeProfile == nil {
		activeRuntimeProfile = newRuntimeProfile()
	}
	return activeRuntimeProfile
}

// SetRuntimeProfileForTest replaces the active profile and returns a restore
// function. Use in tests that need to exercise the opposite platform's
// defaults; defer restore() in the same test.
func SetRuntimeProfileForTest(p *RuntimeProfile) (restore func()) {
	prev := activeRuntimeProfile
	activeRuntimeProfile = p
	return func() { activeRuntimeProfile = prev }
}

// browserRuntimeProfile holds the browser/WASM defaults. Each divergence from
// desktop carries a one-line bench citation justifying the divergence
// (Phase F). Knobs that previously diverged but were unified/deleted by the
// Phase F sweep now match desktop and are not listed in this comment block.
func browserRuntimeProfile() *RuntimeProfile {
	return &RuntimeProfile{
		EdgeCachePad:                       64, // unified with desktop after Phase F sweep
		GridCachePad:                       64,
		DisableNodeGlow:                    false, // Phase F: enabling glow improved drawAvg by 24% at BPM=120
		DisableEdgeArrows:                  false, // Phase F: same
		SimpleDrawDefault:                  true,  // Phase F: keep — auto-disables on first input
		SimpleDrawAutoDisableFramesDefault: 0,
		ScreenEdgesDefault:                 false,
		TimelineInfoThrottleMS:             0,    // unified with desktop after Phase F sweep
		AudioLookaheadSec:                  0.04, // Go↔JS audio jitter — keep until audio-jitter bench available
		AudioBatchMax:                      32,   // Phase F: 256 regressed drawAvg 25% — keep
		SequencerTickMS:                    4,    // Phase F: 1ms tick regressed fps 19% — keep
		ParityScanMinPeriod:                8,
		AdaptivePanPad:                     true, // Phase F: disabling regressed drawAvg 23%
		FastPanDetect:                      true, // Phase F: same — load-bearing
		SkipCursorHover:                    true, // platform-API necessity: Ebiten cursor APIs are no-ops on WASM
		PredictorBackoffOnSlowDraw:         true, // Phase F: disabling regressed drawMax 35%
		ForceInfoLog:                       true,
		DefaultPerfFastPath:                true,
		EnableTouchSmallScreen:             true, // platform-API: small-screen detection only meaningful in browser viewport
		YieldInDrawHelpers:                 true, // platform-API: runtime.Gosched() semantics differ
		ParityFatalDefault:                 false, // browser: log-only by default (PARITY_WASM_FATAL=1 to opt in)
		ParityWatchDefault:                 parityWatchLog,
		IsBrowser:                          true,
		PredictorWindowCap:                 4096,
		RibbonBeatsPerPixel:                0.20, // mobile/touch: slightly wider beats (5 px/beat) for finger scrub precision
		RibbonPlayheadFrac:                 0.70,
		ForceGCInterval:                    30 * time.Second, // counters single-threaded WASM GC starvation (see field doc)
	}
}

// desktopRuntimeProfile mirrors the defaults previously in defaults_notjs.go,
// plus the desktop branch of every runtime.GOOS / runtime.GOARCH check.
func desktopRuntimeProfile() *RuntimeProfile {
	return &RuntimeProfile{
		EdgeCachePad:                       64,
		GridCachePad:                       64,
		DisableNodeGlow:                    false,
		DisableEdgeArrows:                  false,
		SimpleDrawDefault:                  false,
		SimpleDrawAutoDisableFramesDefault: 0,
		ScreenEdgesDefault:                 false,
		TimelineInfoThrottleMS:             0,
		AudioLookaheadSec:                  0.02,
		AudioBatchMax:                      256, // desktop default
		SequencerTickMS:                    1,   // dedicated 1ms goroutine
		ParityScanMinPeriod:                1,
		AdaptivePanPad:                     false,
		FastPanDetect:                      false,
		SkipCursorHover:                    false,
		PredictorBackoffOnSlowDraw:         false,
		ForceInfoLog:                       false,
		DefaultPerfFastPath:                false,
		EnableTouchSmallScreen:             false,
		YieldInDrawHelpers:                 false,
		ParityFatalDefault:                 true,
		ParityWatchDefault:                 parityWatchOff,
		IsBrowser:                          false,
		PredictorWindowCap:                 4096,
		RibbonBeatsPerPixel:                0.25, // desktop: 4 px/beat — major (16), medium (4), minor (1) all readable
		RibbonPlayheadFrac:                 0.70,
		ForceGCInterval:                    0, // off: desktop GC has its own scheduler thread
	}
}
