package ui

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// parityTimingFatalEnabled gates whether a timing-gate breach panics.
// Default ON (desktop/test), forced OFF on browser unless PARITY_TIMING_FATAL
// arms it — mirrors parityFatalEnabled.
var parityTimingFatalEnabled atomic.Bool

// parityTimingWatchDefault is the initial watch mode for the timing gate.
var parityTimingWatchDefault parityWatchMode

func SetParityTimingFatal(enabled bool) { parityTimingFatalEnabled.Store(enabled) }

// resolveParityTimingActivation computes (watch, fatal) from the seed defaults,
// the PARITY_TIMING / PARITY_TIMING_FATAL env values, and whether this is a
// browser runtime. On browser, fatal is forced OFF (watch=log) unless
// PARITY_TIMING_FATAL explicitly arms it — mirroring game.go's PARITY_WASM_FATAL
// safety net so a leaked PARITY_TIMING=panic cannot crash WASM/Playwright runs.
func resolveParityTimingActivation(seedWatch parityWatchMode, seedFatal bool, isBrowser bool, timingEnv, timingFatalEnv string) (watch parityWatchMode, fatal bool) {
	watch, fatal = seedWatch, seedFatal
	switch strings.ToLower(strings.TrimSpace(timingEnv)) {
	case "off":
		watch, fatal = parityWatchOff, false
	case "log", "1", "true":
		watch, fatal = parityWatchLog, false
	case "panic", "fatal":
		watch, fatal = parityWatchPanic, true
	}
	if isBrowser {
		// Browser safety net: force log-only unless explicitly armed.
		armed := false
		switch strings.ToLower(strings.TrimSpace(timingFatalEnv)) {
		case "1", "true", "panic":
			armed = true
		}
		if armed {
			watch, fatal = parityWatchPanic, true
		} else {
			watch, fatal = parityWatchLog, false
		}
	}
	return watch, fatal
}

func init() {
	tb := RuntimeProf().Timing
	w, f := resolveParityTimingActivation(tb.WatchDefault, tb.FatalDefault, RuntimeProf().IsBrowser, os.Getenv("PARITY_TIMING"), os.Getenv("PARITY_TIMING_FATAL"))
	parityTimingWatchDefault = w
	parityTimingFatalEnabled.Store(f)
	// Dormant-under-test default: the timing gate reasons about wall-clock
	// deltas (time.Since(PlayStart), the settle guard) and live scheduler
	// timing. Many existing tests fast-forward simulated time by backdating
	// PlayStart (see driveSchedulerToAbs) to make the scheduler "catch up",
	// which produces huge SYNTHETIC Stage-A/B/D readings that have nothing to
	// do with real audio timing. No bound value can distinguish that from a
	// genuine breach, so under `go test` the gate stays fully OFF unless a
	// dedicated timing test explicitly arms it (see enableParityTimingForTest
	// in parity_test_helpers_test.go). Production desktop/browser runtimes
	// (real wall clock) are unaffected — this only fires inside test binaries.
	if runningUnderGoTest() {
		parityTimingFatalEnabled.Store(false)
		parityTimingWatchDefault = parityWatchOff
	}
}

// parityTimingCheck evaluates the four-stage timing gate against the live
// schedule-metrics window. It runs on the UI thread at parity-scan cadence.
// Suppressed while stopped/paused, during the parity grace window (so live
// circuit/rule edits never false-trigger), and until each stage's window has
// RuntimeProf().Timing.MinSamples samples.
//
// IMPORTANT: only Stage C (schedMetrics.Observe) applies the internal
// schedWarmupSec warmup. Stages A/B/D (ObserveSeqFireLate/ObserveBridge/
// ObserveEmitDrift) do NOT — see schedule_metrics.go. Cold-start spikes on
// those stages during the first instants of playback could otherwise
// false-panic, so this function adds its own explicit settle guard: it
// suppresses the whole gate until at least schedWarmupSec of wall-clock time
// has elapsed since PlayStart. PlayStart (gamestate.State.playStart) is the
// existing wall-clock play-start timestamp already tracked for beat-display
// sync (see syncUIToTime); reusing it avoids inventing a parallel clock.
// Under `go test`, syncUIToTime deliberately never sets PlayStart (see its
// runningUnderGoTest() guard), so PlayStart stays zero for tests that flip
// Playing via SetPlaying/SetPlayingForTest without driving a real Update()
// loop — the zero-value check below treats "no recorded play-start" as
// "settle not applicable" so those tests exercise the gate immediately and
// deterministically, independent of wall-clock. Tests that want to verify
// settle suppression itself should call SetPlayStartForTest with a recent
// timestamp.
//
// On breach it routes through the same watch/fatal policy as the logical
// checker.
func (g *Game) parityTimingCheck() {
	tb := RuntimeProf().Timing
	if !tb.Enabled {
		return
	}
	if g.parityTimingWatch == parityWatchOff && !parityTimingFatalEnabled.Load() {
		return
	}
	if !g.Playing() || g.Paused() {
		return
	}
	if g.parityInGrace() {
		return
	}
	if ps := g.PlayStart(); !ps.IsZero() && time.Since(ps).Seconds() < schedWarmupSec {
		// Cold-start sibling of the C1 fix (see parity_timing_liveedit_test.go).
		// The settle guard suppresses the CHECK during warmup, but Stages A/B/D
		// record OBSERVATIONS unconditionally. A genuine cold-start spike —
		// audio-device warmup, first synth renders (50-85ms), GC, goroutine
		// contention — would otherwise outlive the guard and trip the gate the
		// instant it lifts. A FULL Reset (not just ResetPeaks) is required:
		// clearing only the four monotonic MAX fields leaves the warmup spikes
		// in the percentile reservoirs, where they polluted mean/p99/stddev for
		// the rest of the metrics window (observed as emit.p99=83ms with
		// emit.absMax=11.5ms — the p99 outlasting a cleared max — and ~10s of
		// stale WarnOnly dumps after every play start). When the guard lifts,
		// every statistic reflects only post-warmup observations. See design
		// §3.4 ("universal suppression across all stages").
		g.schedMetrics.Reset()
		return
	}
	snap := g.schedMetrics.Snapshot()

	// emit reports a breach and returns true when the scan should STOP. A
	// WarnOnly stage (bridge, emitdrift) logs its dump and falls through so a
	// later fatal stage (overdue) in the same scan is still evaluated — a warn
	// proxy must never mask a genuine audible-lateness panic. A fatal-capable
	// stage stops the scan (it either panics inside emitTimingFatal, or — in
	// global log-only mode — logs once, preserving first-breach-wins).
	emit := func(br *timingBreach) bool {
		if br == nil {
			return false
		}
		g.emitTimingFatal(*br, snap)
		return !br.WarnOnly
	}

	if emit(evalTimingStage("seqfire", tb.SeqFire, snap.SeqFireCount, snap.SeqFireAvg, snap.SeqFireP99, snap.SeqFireStdDev, snap.SeqFireMax, tb.MinSamples)) {
		return
	}
	if emit(evalTimingStage("bridge", tb.Bridge, snap.BridgeCount, snap.BridgeAvg, snap.BridgeP99, snap.BridgeStdDev, snap.BridgeMax, tb.MinSamples)) {
		return
	}
	// Stage C — overdue/lag. mean=AvgLag, p99=LagP99, absmax=MaxLag; no stddev.
	if br := evalTimingStage("overdue", tb.Overdue, snap.Count, snap.AvgLag, snap.LagP99, 0, snap.MaxLag, tb.MinSamples); br != nil {
		// Cross-check Stage C (a scheduling-LEAD metric: dispatched with `when`
		// in the past) against Stage D (the ACTUAL sample emission). An overdue
		// breach panics ONLY on POSITIVE evidence of audible lateness — the
		// emission also drifted at least OverdueEmitFatalSec late. A transient
		// scheduling blip (a UI action briefly stalling the audioLoop, a GC
		// pause) that still emitted on-time downgrades to a WARN. EmitAbsMax is 0
		// when no emission drift has been observed (or was just ResetPeaks'd by a
		// mutation), so absence of evidence -> WARN, matching the intent "don't
		// crash unless the audio actually came out late." See the 3rd
		// overdue-crash investigation (solo/un-solo: overdue 41ms, emit 5ms).
		if tb.OverdueEmitFatalSec > 0 && snap.EmitAbsMax < tb.OverdueEmitFatalSec {
			br.WarnOnly = true
		}
		if emit(br) {
			return
		}
	}
	if emit(evalTimingStage("emitdrift", tb.EmitDrift, snap.EmitCount, snap.EmitAvg, snap.EmitP99, snap.EmitStdDev, snap.EmitAbsMax, tb.MinSamples)) {
		return
	}
}

// schedMetricsWindow is the rolling-window length for the timing gate's
// schedule metrics. Long enough that the statistical stages (MinSamples=64)
// go live at normal tempos (~20 observations/s on the startup demo → ~3s to
// 64 samples), short enough that the gate reflects recent health rather than
// a session-lifetime average.
const schedMetricsWindow = 10 * time.Second

// maybeResetSchedMetricsWindow resets schedMetrics when the rolling window
// expires. Runs unconditionally at frame end — NOT inside the PERF_LOG block.
// The old placement coupled the gate's window to the 2s perf-log cadence:
// under run-debug the window never reached MinSamples=64 at normal tempos
// (silently reducing the 4-stage gate to its single-sample absmax checks),
// and without PERF_LOG the metrics were never reset at all.
func (g *Game) maybeResetSchedMetricsWindow(now time.Time) {
	if g.schedMetricsNextReset.IsZero() {
		g.schedMetricsNextReset = now.Add(schedMetricsWindow)
		return
	}
	if now.After(g.schedMetricsNextReset) {
		g.schedMetrics.Reset()
		g.schedMetricsNextReset = now.Add(schedMetricsWindow)
		// New window, fresh warn budget (see emitTimingFatal dedup).
		clear(g.timingWarnedThisWindow)
	}
}

// buildTimingFatalDump reuses the logical parity dump and appends a timing
// section: the offending stage/statistic, the full three-stage snapshot, and
// (desktop) the device-clock anchor.
func (g *Game) buildTimingFatalDump(br timingBreach, snap ScheduleMetricsSnapshot) string {
	base := g.buildParityFatalDump(mismatchEntry{
		Row: -1, Abs: -1, Source: "timing", Kind: "timing_" + br.Stage,
		Detail: br.Stat, When: br.Value,
	})
	b := &strings.Builder{}
	b.WriteString(base)
	b.WriteString("[PARITY][timing] gate breach\n")
	fmtTimingLine(b, "stage", br.Stage)
	fmtTimingLine(b, "stat", br.Stat)
	fmtTimingF(b, "value", br.Value)
	fmtTimingF(b, "limit", br.Limit)
	fmtTimingI(b, "windowSamples", br.Count)
	fmtTimingF(b, "seqFire.avg", snap.SeqFireAvg)
	fmtTimingF(b, "seqFire.p99", snap.SeqFireP99)
	fmtTimingF(b, "seqFire.max", snap.SeqFireMax)
	fmtTimingF(b, "bridge.avg", snap.BridgeAvg)
	fmtTimingF(b, "bridge.p99", snap.BridgeP99)
	fmtTimingF(b, "bridge.max", snap.BridgeMax)
	fmtTimingF(b, "overdue.avgLag", snap.AvgLag)
	fmtTimingF(b, "overdue.p99Lag", snap.LagP99)
	fmtTimingF(b, "overdue.maxLag", snap.MaxLag)
	fmtTimingI(b, "overdueCount", snap.Overdue)
	fmtTimingF(b, "emit.avg", snap.EmitAvg)
	fmtTimingF(b, "emit.p99", snap.EmitP99)
	fmtTimingF(b, "emit.absMax", snap.EmitAbsMax)
	fmtTimingF(b, "emit.stdDev", snap.EmitStdDev)
	fmtTimingF(b, "e2eP99", snap.E2EP99)
	// Desktop device-clock anchor (see engine_clock_probe.go). No-op elsewhere.
	// deviceClockSnapshot is a package var (below) reassigned to
	// audio.DeviceClockSnapshot by the desktop build in Task 3.1.
	if dp, dw, ok := deviceClockSnapshot(); ok {
		fmtTimingI(b, "device.pos", dp)
		fmtTimingF(b, "device.wallSec", dw)
	}
	return b.String()
}

func (g *Game) emitTimingFatal(br timingBreach, snap ScheduleMetricsSnapshot) {
	// A WarnOnly stage never panics, even when the global gate is fatal: its
	// breach is downgraded to a WARN dump. See timingStageBounds.WarnOnly.
	fatal := parityTimingFatalEnabled.Load() && !br.WarnOnly
	if g.parityTimingWatch == parityWatchOff && !fatal {
		return
	}
	if !fatal {
		// Dedup non-fatal dumps to one per (stage, stat) per metrics window:
		// parityTimingCheck runs every frame and a latched breach otherwise
		// re-dumps at ~60 lines/s until the window resets (4788 dumps in a 90s
		// bench). Cleared in maybeResetSchedMetricsWindow; UI-goroutine only.
		key := br.Stage + "/" + br.Stat
		if _, dup := g.timingWarnedThisWindow[key]; dup {
			return
		}
		if g.timingWarnedThisWindow == nil {
			g.timingWarnedThisWindow = make(map[string]struct{})
		}
		g.timingWarnedThisWindow[key] = struct{}{}
	}
	msg := g.buildTimingFatalDump(br, snap)
	if g.logger != nil {
		if fatal {
			g.logger.Errorf(msg)
		} else {
			g.logger.Warnf(msg)
		}
	}
	if os.Getenv("PARITY_DUMP_STDERR") == "1" {
		fmt.Fprint(os.Stderr, msg)
	}
	if fatal {
		panic("audio timing parity breach: " + br.Stage + "/" + br.Stat)
	}
}

func fmtTimingLine(b *strings.Builder, k, v string)      { fmt.Fprintf(b, " %s=%s\n", k, v) }
func fmtTimingF(b *strings.Builder, k string, v float64) { fmt.Fprintf(b, " %s=%.6f\n", k, v) }
func fmtTimingI(b *strings.Builder, k string, v int64)   { fmt.Fprintf(b, " %s=%d\n", k, v) }

// deviceClockSnapshot reports (devPos, wallSec, ok). Phase 2 stub returned
// unavailable; Task 3.1 reassigns it to audio.DeviceClockSnapshot below —
// the stubs added to stub.go (test build) and engine_wasm.go (js build) make
// audio.DeviceClockSnapshot resolve to an always-unavailable no-op on those
// platforms, so this init is safe under every build tag.
var deviceClockSnapshot = func() (int64, float64, bool) { return 0, 0, false }

func init() { deviceClockSnapshot = audio.DeviceClockSnapshot }

// timingTraceKey packs (row, abs) into a single map key. abs is masked through
// uint32 (NOT int32) so its bits stay in the low 32 and can never sign-extend
// into and clobber the row bits — a negative abs would otherwise collide row=r
// with row=-1. Traces are only opened for row>=0 with non-negative abs today,
// but this keeps the encoding collision-free for any (row, abs) pair.
func timingTraceKey(row, abs int) int64 { return int64(row)<<32 | int64(uint32(abs)) }

// openTimingTrace records the ideal-when for a beat at schedule time. gen/
// parityGen pin the generation so closeTimingTrace can drop a stale report
// (one that lands after a live circuit/rule edit) rather than feed it into
// Stage-D drift.
func (g *Game) openTimingTrace(row, abs int, idealWhen float64, gen, parityGen uint64) {
	if row < 0 {
		return
	}
	if !RuntimeProf().Timing.Enabled {
		return
	}
	g.parityMu.Lock()
	// Lazy-init: NewGame allocates this map, but hand-built *Game test stubs
	// (and any future construction path that skips the constructor) leave it
	// nil; writing to a nil map panics. Guard here so openTimingTrace is safe
	// from every scheduleSound caller regardless of how the Game was built.
	if g.timingTraces == nil {
		g.timingTraces = make(map[int64]timingTrace)
	}
	g.timingTraces[timingTraceKey(row, abs)] = timingTrace{
		row: row, abs: abs, gen: gen, parityGen: parityGen, idealWhen: idealWhen, openAt: time.Now(),
	}
	// Bound: drop far-behind traces (never closed, e.g. dropped audio).
	if len(g.timingTraces) > 2048 {
		for k, tr := range g.timingTraces {
			if time.Since(tr.openAt) > 2*time.Second {
				delete(g.timingTraces, k)
			}
		}
	}
	g.parityMu.Unlock()
}

// closeTimingTrace matches an emission to its open trace, feeds Stage-D drift,
// and removes the trace. Stale-generation reports are dropped (mirrors the
// audio-parity gen filter) so a report that lands after a live edit does not
// pollute the drift window.
func (g *Game) closeTimingTrace(row, abs int, emittedWallSec float64) {
	g.parityMu.Lock()
	key := timingTraceKey(row, abs)
	tr, ok := g.timingTraces[key]
	if ok {
		delete(g.timingTraces, key)
	}
	curGen := g.audioGen.Load()
	curParityGen := g.parityGen.Load()
	g.parityMu.Unlock()
	if !ok {
		return
	}
	if tr.gen != curGen || tr.parityGen != curParityGen {
		return // stale — dropped by generation filter
	}
	g.schedMetrics.ObserveEmitDrift(emittedWallSec - tr.idealWhen)
}
