package ui

import "time"

// These fields are runtime BEHAVIOUR, not design tokens, so they never move to DESIGN.md; RuntimeFlags is a
// zero-cost read-only view so new code can state "I am reading behaviour, not appearance".

// In seconds; 0 disables a statistic. WarnOnly never panics: Bridge (B) and EmitDrift (D) measure internal proxies
// the lookahead absorbs, while Stage C (overdue) is the authoritative "audio is late" signal and stays fatal.
type timingStageBounds struct {
	MeanMax, P99Max, StdDevMax, AbsMax float64
	WarnOnly                           bool
}

// Browser values are looser and log-only: JS event-loop and bridge jitter would false-positive the desktop envelope.
type timingBounds struct {
	Enabled      bool
	FatalDefault bool
	WatchDefault parityWatchMode
	MinSamples   int // don't gate a stage until its window has this many samples
	SeqFire      timingStageBounds
	Bridge       timingStageBounds
	Overdue      timingStageBounds // Stage C; MeanMax=AvgLag, P99Max=LagP99, AbsMax=MaxLag; StdDevMax unused (0)
	EmitDrift    timingStageBounds
	// An overdue breach is FATAL only if the real emission also drifted this late: a scheduling blip that still
	// emitted on time is audibly fine and only WARNs. 0 = overdue is fatal alone.
	OverdueEmitFatalSec float64
}

// Use instead of Profile() when the field controls behaviour, not appearance.
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

// The single source of browser/desktop divergence (LayoutProfile owns screen class). Never add a runtime.GOOS or
// GOARCH check in internal/ui: add a field here. Tests swap it with SetRuntimeProfileForTest or set fields directly.
type RuntimeProfile struct {
	// A larger pad rebuilds less during pans, for larger offscreen images.
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

	// Exists ONLY to survive the Go→JS crossing on WASM (lead is read at .start() after the bridge delay); 0 on desktop,
	// where scheduleSound's lookahead provides lead and groove rush may legitimately land before the grid.
	MinDispatchLeadSec float64

	// The audio lane commits up to AudioHorizonSec ahead and reconciles on edits, so a main-thread stall shorter
	// than the horizon is inaudible. LaneVoiceNearSec 0 = re-commit the whole horizon.
	AudioLaneEnabled        bool
	AudioHorizonSec         float64
	AudioOffsetSec          float64
	CancelGuardSec          float64
	LateToleranceSec        float64
	LaneFullReconcileSec    float64
	LaneVoiceRecommitMinSec float64
	LaneVoiceNearSec        float64

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

	// In seconds; 0 disables a statistic.
	Timing timingBounds

	// Identity. Snapshotted at profile construction from runtime.GOOS == "js".
	IsBrowser bool

	// In subdivisions; reads outside return false and the cold archive answers history. A knob in case WASM needs a
	// tighter ceiling under memory pressure.
	PredictorWindowCap int

	// Unread since the ribbon became a whole-song bar; a removal candidate.
	RibbonBeatsPerPixel float64
	// Still read by DrumView.TrackBeat for the pattern window's recentre column.
	RibbonPlayheadFrac float64

	// The analysis tabs need real height to be legible (Synth clips at 2×); the screen fraction caps it so the panel
	// never crowds the grid, and a drag-resize still overrides.
	AudioPanelHeightMultiplier int
	AudioPanelHeightScreenFrac float64

	// Wall-clock paced: under load the browser frame rate drops. Not load-bearing (automatic GC bounds the heap); the
	// browser uses it to trim the working set, since WASM linear memory only grows. Desktop's concurrent GC gains nothing.
	ForceGCInterval time.Duration

	// Beyond it the LRU evicts, never the renders a row plays or voices scheduled or playing, so warming and
	// auditions cannot hoard RAM.
	VoiceCacheBudgetBytes int64

	// In each control's resolution, so a one-tick edit lands warm; 0 warms exact values only.
	PredictiveWarmTicks int

	// The gated warm-up's wall-clock divisor. Browser 1: goroutines do not parallelise on js/wasm, and audio.js has
	// its own render workers.
	PrewarmWorkers int
}

var activeRuntimeProfile *RuntimeProfile

func RuntimeProf() *RuntimeProfile {
	if activeRuntimeProfile == nil {
		activeRuntimeProfile = newRuntimeProfile()
	}
	return activeRuntimeProfile
}

// defer restore() in the same test.
func SetRuntimeProfileForTest(p *RuntimeProfile) (restore func()) {
	prev := activeRuntimeProfile
	activeRuntimeProfile = p
	return func() { activeRuntimeProfile = prev }
}

// Each divergence from desktop carries the measurement that justifies it.
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
		AudioLookaheadSec:                  0.06,  // overdue 0 / small-lead 0 over 60 s including forced-GC cycles
		MinDispatchLeadSec:                 0.012, // 8ms Go-side clamp + ~4ms Go→JS PlayBatch crossing — keeps JS-observed minLead positive
		AudioLaneEnabled:                   true,  // A/B: no note arrived late vs 17–69 on legacy; IOI p95 5–9.5 ms vs 76–287 ms
		AudioHorizonSec:                    1.0,   // headless software-GL frames 0.4-0.6 s (1.1 s under load) + an injected 300 ms stall (bench-results/audio_lane_ab_before_2026-10-02.md); edit latency does not depend on H
		AudioOffsetSec:                     0.06,  // = AudioLookaheadSec: keeps today's AV relationship in steady state
		CancelGuardSec:                     0.008, // > one 128-frame render quantum (2.7 ms) + start() delivery: a source closer than this may already be rendering
		LateToleranceSec:                   0.025,
		LaneFullReconcileSec:               0.25,
		LaneVoiceRecommitMinSec:            0.03,
		LaneVoiceNearSec:                   0.25, // = the desktop horizon; a whole-horizon re-commit per drag tick was ~3000 voice starts/s
		AudioBatchMax:                      32,   // Phase F: 256 regressed drawAvg 25% — keep
		SequencerTickMS:                    4,    // Phase F: 1ms tick regressed fps 19% — keep
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
		ForceGCInterval:                    10 * time.Second, // working-set trim (33 vs 43 MB), sub-2ms pauses, overdue 0 — see field doc + bench-results/gc_tuning_2026-07.md
		VoiceCacheBudgetBytes:              24 << 20,         // 24 MB: tighter on browser (single 2 GB WASM heap ceiling)
		PredictiveWarmTicks:                1,
		PrewarmWorkers:                     1, // js/wasm is single-threaded; parallelism lives in audio.js workers
		// Looser and log-only: JS event-loop and bridge jitter that desktop lacks.
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: false, // log-only on browser unless PARITY_TIMING_FATAL arms it
			WatchDefault: parityWatchLog,
			MinSamples:   128,
			// WarnOnly as on desktop: an internal proxy; Stage C owns audible lateness.
			SeqFire:   timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060, WarnOnly: true},
			Bridge:    timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060, WarnOnly: true},
			Overdue:   timingStageBounds{MeanMax: 0.002, P99Max: 0.012, StdDevMax: 0, AbsMax: 0.040},
			EmitDrift: timingStageBounds{MeanMax: 0.008, P99Max: 0.025, StdDevMax: 0.010, AbsMax: 0.080, WarnOnly: true},
			// Browser is log-only anyway; keep the cross-check off (0).
			OverdueEmitFatalSec: 0,
		},
	}
}

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
		// Unified with the browser: a 20 ms base sat below real desktop jitter (34–55 ms overdue transients at 200–300 BPM,
		// each an audible glitch); 60 ms absorbs the measured envelope at every benched tempo.
		AudioLookaheadSec:          0.06,
		MinDispatchLeadSec:         0,    // no Go→JS bridge on desktop; audio.Now() is the engine clock, so no dispatch-time floor is needed
		AudioLaneEnabled:           true, // same code path as the browser (one lane, both platforms); browser A/B: bench-results/audio_lane_ab_after_2026-10-02.md
		AudioHorizonSec:            0.25, // desktop draws on its own goroutine; 0.25 s covers the measured 34-55 ms full-core goroutine wake-up transients with margin
		AudioOffsetSec:             0.06,
		CancelGuardSec:             0.003, // one 64-sample mixer block (1.45 ms) + the 10 ms oto buffer is drained per Read, so 3 ms keeps a margin over a block
		LateToleranceSec:           0.025,
		LaneFullReconcileSec:       0.25,
		LaneVoiceRecommitMinSec:    0.03,
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
		PrewarmWorkers:             4, // 96-pair warm set: 1.42 s serial → ~0.4 s
		// Tight and enforced: no bridge, a 1 ms sequencer tick.
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: true,
			WatchDefault: parityWatchPanic,
			MinSamples:   64,
			// WarnOnly: fire-lateness reduces Stage C lead but never makes audio late, and a single ~19 ms Go-scheduler
			// wakeup tripped its p99 while Stage C stayed zero.
			SeqFire: timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.030, WarnOnly: true},
			// WarnOnly: queue-wait is absorbed by the lookahead; a health signal, not a gate.
			Bridge: timingStageBounds{MeanMax: 0.002, P99Max: 0.008, StdDevMax: 0.003, AbsMax: 0.030, WarnOnly: true},
			// The authoritative audible-lateness gate; fatal.
			Overdue: timingStageBounds{MeanMax: 0.0005, P99Max: 0.006, StdDevMax: 0, AbsMax: 0.020},
			// Wall-vs-device skew is a diagnostic proxy, not audible alone.
			EmitDrift: timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.040, WarnOnly: true},
			// MUST be ≥ EmitDrift.AbsMax: device-period quantisation puts the healthy emission tail at ~25 ms, so a smaller
			// threshold turns device noise into false corroboration.
			OverdueEmitFatalSec: 0.040,
		},
	}
}
