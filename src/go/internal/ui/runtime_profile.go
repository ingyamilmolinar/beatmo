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

// timingStageBounds is the per-stage tolerance envelope for the timing-parity
// gate (see parityTimingCheck, added by a later task). All values are in
// seconds; a bound of 0 disables that statistic for the stage.
// timingStageBounds holds one stage's tolerances. WarnOnly demotes the whole
// stage to log-only: a breach still emits a WARN dump but never panics, even
// when the global gate is fatal. Bridge (Stage B) and EmitDrift (Stage D) are
// WarnOnly on every platform because they measure internal scheduling proxies
// (enqueue→dispatch queue-wait behind the per-beat synth render; device-clock
// emission jitter) that the audio lookahead cushion absorbs — Stage C
// (overdue) is the authoritative "audio is actually late" signal and stays
// fatal. See the 2026-07 timing-gate calibration investigation: bridge
// legitimately runs ~13-20ms (per-beat newRecipeAwareVoice render on the
// dispatch goroutine) while overdue stays ~0, so a fatal bridge/emit bound
// panics normal desktop playback on latency that never reaches the speakers.
type timingStageBounds struct {
	MeanMax, P99Max, StdDevMax, AbsMax float64
	WarnOnly                           bool
}

// timingBounds is the full set of timing-gate tolerances for one platform
// (desktop or browser). Desktop values are tight and enforced by default;
// browser values are looser and log-only by default (WASM scheduling jitter
// from the JS event loop + Go→JS bridge crossing means the same envelope
// would false-positive constantly).
type timingBounds struct {
	Enabled      bool
	FatalDefault bool
	WatchDefault parityWatchMode
	MinSamples   int // don't gate a stage until its window has this many samples
	SeqFire      timingStageBounds
	Bridge       timingStageBounds
	Overdue      timingStageBounds // Stage C; MeanMax=AvgLag, P99Max=LagP99, AbsMax=MaxLag; StdDevMax unused (0)
	EmitDrift    timingStageBounds
	// OverdueEmitFatalSec cross-checks Stage C against Stage D: an overdue
	// breach (a scheduling-LEAD metric: dispatched with `when` in the past) is
	// only FATAL when the ACTUAL sample emission also drifted at least this far
	// late (Stage D emit.absMax). A transient scheduling blip that still emitted
	// on-time (e.g. a UI action briefly stalling the audioLoop) is audibly fine,
	// so it downgrades to a WARN instead of panicking. 0 disables the cross-check
	// (overdue stays fatal on its own). See parityTimingCheck.
	OverdueEmitFatalSec float64
}

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

	// MinDispatchLeadSec is the dispatch-time floor applied to
	// sequencer-scheduled events (row >= 0) in audioLoop: req.when is bumped
	// up to audioNow+MinDispatchLeadSec when it would otherwise slip into the
	// past or the small-lead danger zone. It exists ONLY to survive the
	// Go→JS PlayBatch crossing on WASM (the JS observer reads lead at
	// .start() after the bridge delay, so the floor must exceed the bridge
	// max to keep audioMetrics.minLead positive). 0 disables the clamp
	// entirely — on desktop audio.Now() is the engine clock with no bridge,
	// so scheduleSound's lookahead already provides lead and groove "rush"
	// can legitimately schedule before the grid boundary.
	MinDispatchLeadSec float64

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

	// Timing-parity gate tolerances. See parityTimingCheck. All values in
	// seconds; a bound of 0 disables that statistic. Desktop values are tight
	// (dev/test enforcement); browser values looser + log-only by default.
	Timing timingBounds

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

	// AudioPanelHeightMultiplier scales the base eqPanelHeight to size
	// the audio analysis panel. Spectrum / Levels / Chain / Synth all
	// need substantial vertical real estate to be legible (the Synth
	// tab clips knobs at 2×; spectrum bars need ~480 px for dB ticks
	// to render). 3× gives ~360 px on a 720 px viewport (~50% of the
	// screen) and lets every audio tab claim the same headroom — a
	// user-draggable handle on the row strip still overrides.
	// AudioPanelHeightScreenFrac is the hard upper bound expressed as
	// a fraction of the framebuffer height; PanelHeight clamps to
	// floor(screenH * frac) so the panel never crowds the grid.
	AudioPanelHeightMultiplier int
	AudioPanelHeightScreenFrac float64

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

	// VoiceCacheBudgetBytes is the hard ceiling on retained rendered-voice
	// buffers in audio's globalVoiceCache (LRU eviction beyond it). Bounds RAM
	// so predictive neighborhood warming can mint many keys without OOM/hoarding.
	// Wired to audio.SetVoiceCacheBudgetBytes at startup.
	VoiceCacheBudgetBytes int64

	// PredictiveWarmTicks is the ±N radius (in each control's resolution) of the
	// predictive-neighborhood warm around the current value, so an immediate
	// one-tick edit lands on a warm cache. 1 = warm the immediate neighbors;
	// 0 disables neighborhood warming (exact values only).
	PredictiveWarmTicks int
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
		TimelineInfoThrottleMS:             0,     // unified with desktop after Phase F sweep
		AudioLookaheadSec:                  0.06,  // 60ms cushion; 2026-06 re-sweep showed overdue=0/smallLead=0 at 0.06 over 60s incl. forced-GC cycles, with adaptive +0.05 headroom and the dispatch floor absorbing Stage-B tails (bench-results/audio_lookahead_resweep_2026-06.md)
		MinDispatchLeadSec:                 0.012, // 8ms Go-side clamp + ~4ms Go→JS PlayBatch crossing — keeps JS-observed minLead positive
		AudioBatchMax:                      32,    // Phase F: 256 regressed drawAvg 25% — keep
		SequencerTickMS:                    4,     // Phase F: 1ms tick regressed fps 19% — keep
		ParityScanMinPeriod:                8,
		AdaptivePanPad:                     true, // Phase F: disabling regressed drawAvg 23%
		FastPanDetect:                      true, // Phase F: same — load-bearing
		SkipCursorHover:                    true, // platform-API necessity: Ebiten cursor APIs are no-ops on WASM
		PredictorBackoffOnSlowDraw:         true, // Phase F: disabling regressed drawMax 35%
		ForceInfoLog:                       true,
		DefaultPerfFastPath:                true,
		EnableTouchSmallScreen:             true,  // platform-API: small-screen detection only meaningful in browser viewport
		YieldInDrawHelpers:                 true,  // platform-API: runtime.Gosched() semantics differ
		ParityFatalDefault:                 false, // browser: log-only by default (PARITY_WASM_FATAL=1 to opt in)
		ParityWatchDefault:                 parityWatchLog,
		IsBrowser:                          true,
		PredictorWindowCap:                 4096,
		RibbonBeatsPerPixel:                0.20, // mobile/touch: slightly wider beats (5 px/beat) for finger scrub precision
		RibbonPlayheadFrac:                 0.70,
		AudioPanelHeightMultiplier:         3,
		AudioPanelHeightScreenFrac:         0.60,
		ForceGCInterval:                    30 * time.Second, // counters single-threaded WASM GC starvation (see field doc)
		VoiceCacheBudgetBytes:              24 << 20,         // 24 MB: tighter on browser (single 2 GB WASM heap ceiling)
		PredictiveWarmTicks:                1,
		// Timing-gate tolerances; see plan Task 1.2. Looser than desktop and
		// log-only by default: JS event-loop scheduling + Go→JS bridge
		// crossing add jitter native desktop doesn't have. Starting values
		// pending bench tuning (bench-results/).
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: false, // log-only on browser unless PARITY_TIMING_FATAL arms it
			WatchDefault: parityWatchLog,
			MinSamples:   128,
			// SeqFire WarnOnly for the same reason as desktop: internal
			// scheduling proxy; Stage C owns audible lateness.
			SeqFire:   timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060, WarnOnly: true},
			Bridge:    timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060, WarnOnly: true},
			Overdue:   timingStageBounds{MeanMax: 0.002, P99Max: 0.012, StdDevMax: 0, AbsMax: 0.040},
			EmitDrift: timingStageBounds{MeanMax: 0.008, P99Max: 0.025, StdDevMax: 0.010, AbsMax: 0.080, WarnOnly: true},
			// Browser is log-only anyway; keep the cross-check off (0).
			OverdueEmitFatalSec: 0,
		},
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
		// 60ms cushion, unified with the browser value (same bench basis:
		// audio_lookahead_resweep_2026-06.md showed overdue=0/smallLead=0 at
		// 0.06). The prior desktop 0.02 base (max 0.05 with the adaptive +0.03)
		// sat BELOW real desktop scheduling jitter: 2026-07-13 fatal-gate bench
		// dumps at 200/240/300 BPM recorded single 34-55ms overdue transients
		// (sequencer goroutine wakeup delay under full-core load) with
		// confirmed 44-62ms-late emissions — each one a real audible glitch the
		// 20ms base could not absorb. 120 BPM was already clean at 0.02; 0.06
		// absorbs the measured transient envelope at every benched tempo.
		AudioLookaheadSec:          0.06,
		MinDispatchLeadSec:         0,   // no Go→JS bridge on desktop; audio.Now() is the engine clock, so no dispatch-time floor is needed
		AudioBatchMax:              256, // desktop default
		SequencerTickMS:            1,   // dedicated 1ms goroutine
		ParityScanMinPeriod:        1,
		AdaptivePanPad:             false,
		FastPanDetect:              false,
		SkipCursorHover:            false,
		PredictorBackoffOnSlowDraw: false,
		ForceInfoLog:               false,
		DefaultPerfFastPath:        false,
		EnableTouchSmallScreen:     false,
		YieldInDrawHelpers:         false,
		ParityFatalDefault:         true,
		ParityWatchDefault:         parityWatchOff,
		IsBrowser:                  false,
		PredictorWindowCap:         4096,
		RibbonBeatsPerPixel:        0.25, // desktop: 4 px/beat — major (16), medium (4), minor (1) all readable
		RibbonPlayheadFrac:         0.70,
		AudioPanelHeightMultiplier: 3,
		AudioPanelHeightScreenFrac: 0.60,
		ForceGCInterval:            0,        // off: desktop GC has its own scheduler thread
		VoiceCacheBudgetBytes:      48 << 20, // 48 MB retained-voice ceiling (see field doc)
		PredictiveWarmTicks:        1,
		// Timing-gate tolerances; see plan Task 1.2. Tight + enforced: no
		// Go→JS bridge, dedicated 1ms sequencer tick. Starting values pending
		// bench tuning (bench-results/).
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: true,
			WatchDefault: parityWatchPanic,
			MinSamples:   64,
			// Stage A: scheduler tick is 1ms desktop; fire-late should be tiny.
			// WarnOnly: Stage A measures an internal scheduling proxy. Per-beat
			// ideal baseNow (game_sequencer_schedule.go) means fire-lateness does
			// NOT propagate into audio lateness — it only reduces Stage C lead —
			// and a 300 BPM bench (2026-07-13) showed its p99 tripping on a single
			// ~19ms Go-scheduler wakeup delay (execution trace: selectnbsend on
			// hlCh readies the UI receiver and the sequencer goroutine waits for a
			// P under draw-spike contention) while Stage C stayed at zero. Stage C
			// (+ emit corroboration) is the authoritative fatal gate.
			SeqFire: timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.030, WarnOnly: true},
			// Stage B: enqAt→dispatch queue-wait. The historical ~13-20ms average
			// was the per-trigger oto ctx.Resume() serializing dispatch behind the
			// ALSA writer's blocking snd_pcm_writei (fixed 2026-07-13; see
			// engine_play.go). Post-fix steady state is ~1-3ms; spikes surface in
			// the WARN dump. WarnOnly: a health signal, not a fatal gate — the
			// lookahead cushion absorbs queue-wait (Stage C catches real lateness).
			Bridge: timingStageBounds{MeanMax: 0.002, P99Max: 0.008, StdDevMax: 0.003, AbsMax: 0.030, WarnOnly: true},
			// Stage C: overdue (lag) — the authoritative audible-lateness gate,
			// fatal. Any sustained overdue is bad; dropout at AbsMax.
			Overdue: timingStageBounds{MeanMax: 0.0005, P99Max: 0.006, StdDevMax: 0, AbsMax: 0.020},
			// Stage D: realized emission vs the scheduler-intended 'when'.
			// Desktop wall-vs-device-clock skew is a diagnostic proxy, not an
			// audible defect on its own → WarnOnly (Stage C catches real lateness).
			EmitDrift: timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.040, WarnOnly: true},
			// Cross-check: an overdue breach panics only if the actual emission
			// also drifted at least this late (real audible lateness). A
			// transient scheduling-lead blip that emitted on-time -> WARN, not
			// crash. MUST be >= EmitDrift.AbsMax: emission drift below that
			// bound is calibrated as "not an audible defect on its own"
			// (device-period quantization puts the NATURAL healthy emission
			// tail at ~25ms — a 300 BPM bench showed emit.p99=25.5ms in an
			// otherwise clean window), so a smaller threshold turns device
			// noise into false corroboration. The confirmed-real crash this
			// gate exists for measured emit.absMax=106ms.
			OverdueEmitFatalSec: 0.040,
		},
	}
}
