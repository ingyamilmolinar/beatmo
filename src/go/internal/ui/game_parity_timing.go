package ui

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// On by default; off in the browser unless PARITY_TIMING_FATAL arms it.
var parityTimingFatalEnabled atomic.Bool

// parityTimingWatchDefault is the initial watch mode for the timing gate.
var parityTimingWatchDefault parityWatchMode

func SetParityTimingFatal(enabled bool) { parityTimingFatalEnabled.Store(enabled) }

// In the browser fatal is off unless PARITY_TIMING_FATAL arms it, so a leaked PARITY_TIMING=panic
// cannot crash WASM runs.
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
	// Armed under go test like the logical checker: every test Game panics on a timing breach.
	if runningUnderGoTest() {
		parityTimingFatalEnabled.Store(true)
		parityTimingWatchDefault = parityWatchPanic
	}
}

// Suppressed while stopped, during grace, and until each stage has MinSamples. Only Stage C has a
// built-in warmup, so the whole gate also waits schedWarmupSec after PlayStart; a zero PlayStart (tests
// that never ran Update) means no settle, so those tests check immediately.
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
		// A FULL Reset after the settle: stages A/B/D observe during warmup, and resetting only the peaks
		// leaves cold-start spikes in the percentile reservoirs for the rest of the window.
		g.schedMetrics.Reset()
		return
	}
	snap := g.schedMetrics.Snapshot()

	// true stops the scan. A WarnOnly stage logs and falls through, so a warn proxy never masks a
	// later fatal stage in the same scan.
	emit := func(br *timingBreach) bool {
		if br == nil {
			return false
		}
		g.emitTimingFatal(*br, snap)
		return !br.WarnOnly
	}

	// With the lane on, notes were committed ahead, so a late sequencer pass moves a highlight, not a
	// sound; the lane's audio timing is stages C and D.
	if !g.audioLaneOn() && emit(evalTimingStage("seqfire", tb.SeqFire, snap.SeqFireCount, snap.SeqFireAvg, snap.SeqFireP99, snap.SeqFireStdDev, snap.SeqFireMax, tb.MinSamples)) {
		return
	}
	if emit(evalTimingStage("bridge", tb.Bridge, snap.BridgeCount, snap.BridgeAvg, snap.BridgeP99, snap.BridgeStdDev, snap.BridgeMax, tb.MinSamples)) {
		return
	}
	// Stage C — overdue/lag. mean=AvgLag, p99=LagP99, absmax=MaxLag; no stddev.
	if br := evalTimingStage("overdue", tb.Overdue, snap.Count, snap.AvgLag, snap.LagP99, 0, snap.MaxLag, tb.MinSamples); br != nil {
		// Overdue panics only on POSITIVE evidence the emission itself came out late (Stage D); a
		// scheduling blip that still emitted on time is a WARN. EmitAbsMax 0 = no evidence = WARN.
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

// Long enough for MinSamples at normal tempos, short enough to reflect recent health.
const schedMetricsWindow = 10 * time.Second

// At frame end, never tied to the PERF_LOG cadence, which kept the window from ever reaching
// MinSamples (or never reset it at all).
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
	// Desktop only; a no-op elsewhere.
	if dp, dw, ok := deviceClockSnapshot(); ok {
		fmtTimingI(b, "device.pos", dp)
		fmtTimingF(b, "device.wallSec", dw)
	}
	return b.String()
}

func (g *Game) emitTimingFatal(br timingBreach, snap ScheduleMetricsSnapshot) {
	// A WarnOnly stage never panics, even when the gate is fatal.
	fatal := parityTimingFatalEnabled.Load() && !br.WarnOnly
	if g.parityTimingWatch == parityWatchOff && !fatal {
		return
	}
	if !fatal {
		// One dump per (stage, stat) per window: the check runs every frame and a latched breach would
		// re-dump at 60 lines/s. UI goroutine only.
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

// A no-op on test and js builds, so this is safe under every build tag.
var deviceClockSnapshot = func() (int64, float64, bool) { return 0, 0, false }

func init() { deviceClockSnapshot = audio.DeviceClockSnapshot }

// abs goes through uint32, so a negative abs can never sign-extend into the row bits.
func timingTraceKey(row, abs int) int64 { return int64(row)<<32 | int64(uint32(abs)) }

// gen/parityGen let closeTimingTrace drop a report that lands after a live edit.
func (g *Game) openTimingTrace(row, abs int, idealWhen float64, gen, parityGen uint64) {
	if row < 0 {
		return
	}
	if !RuntimeProf().Timing.Enabled {
		return
	}
	g.parityMu.Lock()
	// Hand-built test Games leave the map nil, and writing to it would panic.
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

// Stale-generation reports are dropped, like the audio-parity filter.
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
