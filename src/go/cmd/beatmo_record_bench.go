package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/bench"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// startRecordBench creates the bench output directory, opens a CPU profile
// file scoped to the run, and registers a hooks subscriber that logs
// recording lifecycle transitions. Returns (outputDir, stopFn, err) where
// stopFn writes the final heap profile + flushes CPU profile.
//
// Honors BEATMO_BLOCK_PROFILE / BEATMO_MUTEX_PROFILE env vars: when set
// to a positive integer, enables runtime block / mutex profiling at that
// rate (1 = every event). Profiles are written next to cpu.pprof so we
// can ask "where is audio actually blocked?" instead of guessing.
//
// Additional optional env vars for memory-footprint investigations:
//
//	BEATMO_RECORD_BENCH_HEAP_SNAPSHOTS=1
//	    Captures heap profiles at fixed wall-clock offsets from the
//	    record-bench process-start moment: 5s ("warmup"), 30s ("mid"), 60s
//	    ("end"), plus an allocs profile at 60s. Lands as
//	    heap_{warmup,mid,end}.pprof and allocs.pprof in outDir.
//	BEATMO_RECORD_BENCH_SYNTH_CHURN_HZ=<int>
//	    When > 0, spawns a goroutine that cycles audio.SetInstrumentParam
//	    on every available instrument at this rate (Hz). Mirrors the soak
//	    test's toggleSoakSynthParamChurn pattern. Use together with -scene
//	    crop_synth_tab_drum so the Synth tab Draw stack is exercised.
func startRecordBench(logger *game_log.Logger) (string, func(), error) {
	timestamp := time.Now().Format("2006-01-02_150405")
	outDir := filepath.Join("bench-results", "record-"+timestamp)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	cpuPath := filepath.Join(outDir, "cpu.pprof")
	cpuFile, err := os.Create(cpuPath)
	if err != nil {
		return "", nil, fmt.Errorf("create cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		cpuFile.Close()
		return "", nil, fmt.Errorf("start cpu profile: %w", err)
	}
	logger.Infof("[RECORD-BENCH] outDir=%s cpuProfile=%s", outDir, cpuPath)

	blockRate := bench.EnvInt("BEATMO_BLOCK_PROFILE", 0)
	mutexFrac := bench.EnvInt("BEATMO_MUTEX_PROFILE", 0)
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
		logger.Infof("[RECORD-BENCH] block profiling enabled rate=%d", blockRate)
	}
	if mutexFrac > 0 {
		runtime.SetMutexProfileFraction(mutexFrac)
		logger.Infof("[RECORD-BENCH] mutex profiling enabled fraction=%d", mutexFrac)
	}

	subUnsubs := []func(){
		hooks.Subscribe(hooks.EventRecordStart, func(e hooks.Event) {
			logger.Infof("[RECORD-BENCH/hook] record.start payload=%v", e.Payload)
		}),
		hooks.Subscribe(hooks.EventRecordStop, func(e hooks.Event) {
			logger.Infof("[RECORD-BENCH/hook] record.stop payload=%v", e.Payload)
		}),
		hooks.Subscribe(hooks.EventRecordDropped, func(e hooks.Event) {
			logger.Errorf("[RECORD-BENCH/hook] record.dropped count=%v — pipeline backpressure!", e.Payload)
		}),
	}

	// Optional: timed heap snapshots + synth-param churn for memory-footprint
	// investigations. Both gated behind env vars so the default behavior of
	// -record-bench is unchanged.
	profilerStop := make(chan struct{})
	var profilerWG sync.WaitGroup
	if os.Getenv("BEATMO_RECORD_BENCH_HEAP_SNAPSHOTS") == "1" {
		profilerWG.Add(1)
		go runHeapSnapshotSchedule(outDir, logger, profilerStop, &profilerWG)
	}
	if hz := bench.EnvInt("BEATMO_RECORD_BENCH_SYNTH_CHURN_HZ", 0); hz > 0 {
		profilerWG.Add(1)
		go runSynthParamChurn(hz, logger, profilerStop, &profilerWG)
	}

	stop := func() {
		pprof.StopCPUProfile()
		cpuFile.Close()
		// Stop the auxiliary profilers BEFORE the final heap profile so the
		// churn goroutine isn't still mutating the params manager during the
		// final WriteHeapProfile.
		close(profilerStop)
		profilerWG.Wait()
		writeHeapProfile(outDir, logger)
		if blockRate > 0 {
			writeNamedProfile(outDir, "block", logger)
			runtime.SetBlockProfileRate(0)
		}
		if mutexFrac > 0 {
			writeNamedProfile(outDir, "mutex", logger)
			runtime.SetMutexProfileFraction(0)
		}
		writeRuntimeStats(outDir, logger)
		for _, u := range subUnsubs {
			u()
		}
		logger.Infof("[RECORD-BENCH] artifacts in %s", outDir)
	}
	return outDir, stop, nil
}

// runHeapSnapshotSchedule writes runtime/pprof heap profiles at three fixed
// wall-clock offsets relative to the goroutine's start moment: 5s, 30s, 60s.
// Each capture is preceded by a forced GC so the profile reflects the live
// retained set, not allocator slack. Allocs profile is captured at the 60s
// mark (final). The schedule exits cleanly when stop is closed; partial
// snapshots are skipped.
//
// Filenames are stable so scripts/analyze_synth_profile.sh can find them
// without parsing timestamps.
func runHeapSnapshotSchedule(outDir string, logger *game_log.Logger, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	type tick struct {
		after time.Duration
		name  string
	}
	schedule := []tick{
		{5 * time.Second, "heap_warmup"},
		{30 * time.Second, "heap_mid"},
		{60 * time.Second, "heap_end"},
	}
	start := time.Now()
	for _, t := range schedule {
		remaining := t.after - time.Since(start)
		if remaining > 0 {
			select {
			case <-stop:
				return
			case <-time.After(remaining):
			}
		}
		writeNamedHeapProfile(outDir, t.name, logger)
		if t.name == "heap_end" {
			writeNamedProfile(outDir, "allocs", logger)
		}
	}
}

// writeNamedHeapProfile forces a GC and writes a runtime/pprof heap profile
// to <outDir>/<name>.pprof. Mirrors writeHeapProfile but lets the caller
// pick the basename so the snapshot-schedule files don't clobber the final
// heap.pprof.
func writeNamedHeapProfile(outDir, name string, logger *game_log.Logger) {
	runtime.GC()
	path := filepath.Join(outDir, name+".pprof")
	f, err := os.Create(path)
	if err != nil {
		logger.Errorf("[RECORD-BENCH] create %s profile: %v", name, err)
		return
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		logger.Errorf("[RECORD-BENCH] write %s profile: %v", name, err)
		return
	}
	logger.Infof("[RECORD-BENCH] %s profile → %s", name, path)
}

// runSynthParamChurn drives audio.SetInstrumentParam at hz on every
// available instrument, cycling through the wired synth recipe params
// ("pitch" and "decay") with a sin-based sweep. Mirrors the soak test's
// toggleSoakSynthParamChurn so the production OOM stack
// (EQPanelZone.Draw → drawSynthTab → drawSynthSectionCard → Knob.Draw →
// drawArc + hashRecipeParams keying) is exercised at the same rate a user
// dragging a knob would produce.
//
// The churn is platform-agnostic — it does not require the Synth tab to
// be open in the UI for SetInstrumentParam to take effect, but it is
// intended to be used alongside -scene crop_synth_tab_drum so the Draw
// stack is also exercised.
func runSynthParamChurn(hz int, logger *game_log.Logger, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	if hz <= 0 {
		return
	}
	period := time.Second / time.Duration(hz)
	if period <= 0 {
		period = time.Millisecond
	}
	logger.Infof("[RECORD-BENCH] synth param churn enabled hz=%d period=%s", hz, period)
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	start := time.Now()
	var ticks int
	for {
		select {
		case <-stop:
			logger.Infof("[RECORD-BENCH] synth param churn stopped ticks=%d", ticks)
			return
		case <-ticker.C:
			ticks++
			elapsed := time.Since(start).Seconds()
			pitch := -12 + 24*math.Sin(elapsed*2.0)
			decay := 0.25 + 0.75*math.Sin(elapsed*1.7)
			for _, id := range audio.Instruments() {
				audio.SetInstrumentParam(id, "pitch", pitch)
				audio.SetInstrumentParam(id, "decay", decay)
			}
		}
	}
}

// writeNamedProfile dumps a runtime/pprof named profile (e.g. "block",
// "mutex", "goroutine", "allocs") to <outDir>/<name>.pprof.
func writeNamedProfile(outDir, name string, logger *game_log.Logger) {
	prof := pprof.Lookup(name)
	if prof == nil {
		logger.Errorf("[RECORD-BENCH] no profile named %q", name)
		return
	}
	path := filepath.Join(outDir, name+".pprof")
	f, err := os.Create(path)
	if err != nil {
		logger.Errorf("[RECORD-BENCH] create %s profile: %v", name, err)
		return
	}
	defer f.Close()
	if err := prof.WriteTo(f, 0); err != nil {
		logger.Errorf("[RECORD-BENCH] write %s profile: %v", name, err)
		return
	}
	logger.Infof("[RECORD-BENCH] %s profile → %s", name, path)
}

func writeHeapProfile(outDir string, logger *game_log.Logger) {
	runtime.GC()
	heapPath := filepath.Join(outDir, "heap.pprof")
	f, err := os.Create(heapPath)
	if err != nil {
		logger.Errorf("[RECORD-BENCH] create heap profile: %v", err)
		return
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		logger.Errorf("[RECORD-BENCH] write heap profile: %v", err)
		return
	}
	logger.Infof("[RECORD-BENCH] heap profile → %s", heapPath)
}

// audioCountersAdapter bridges the internal/audio package-level helpers
// to the AudioCounters interface that internal/bench expects. The
// adapter exists so internal/bench stays free of an internal/audio
// import.
type audioCountersAdapter struct{}

func (audioCountersAdapter) RecordingDrops() int64 { return audio.RecordingDrops() }
func (audioCountersAdapter) IsRecording() bool     { return audio.IsRecording() }

func writeRuntimeStats(outDir string, logger *game_log.Logger) {
	stats := bench.RuntimeStatsSnapshot(audioCountersAdapter{})
	b, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		logger.Errorf("[RECORD-BENCH] marshal runtime stats: %v", err)
		return
	}
	statsPath := filepath.Join(outDir, "runtime_stats.json")
	if err := os.WriteFile(statsPath, b, 0o644); err != nil {
		logger.Errorf("[RECORD-BENCH] write runtime stats: %v", err)
	}
}

// hookActionCount tracks how many times each action has fired (handy for
// CI assertions and debugging). Storage lives in internal/bench so the
// counter can be tested in isolation.
var hookActionCount = &bench.ActionCounter{}

// loadHooksConfig parses path and registers subscribers that invoke
// built-in actions. Currently supported actions: log, exit, dump_pprof.
// Recording actions are intentionally tied to -record-bench so the CLI
// stays opinionated; hook-driven recording can be layered on later.
//
// Returns a cleanup function alongside the error so callers can
// unsubscribe at shutdown if desired.
func loadHooksConfig(path string, logger *game_log.Logger) (func(), error) {
	cfg, err := bench.LoadHooksConfig(path)
	if err != nil {
		return func() {}, err
	}
	cleanup := bench.SubscribeRules(cfg.Rules, logger, hookActionCount, func(action string, e hooks.Event) {
		runHookAction(action, e, logger)
	})
	return cleanup, nil
}

func runHookAction(action string, _ hooks.Event, logger *game_log.Logger) {
	if !bench.IsKnownAction(action) {
		logger.Errorf("[HOOKS] unknown action: %s", action)
		return
	}
	switch bench.HookAction(action) {
	case bench.ActionLog:
		// already logged; no further action
	case bench.ActionExit:
		// Defer slightly so the calling event handler can return cleanly.
		go func() { time.Sleep(50 * time.Millisecond); os.Exit(0) }()
	case bench.ActionDumpPProf:
		go func() {
			ts := time.Now().Format("2006-01-02_150405")
			path := filepath.Join("bench-results", "hook-heap-"+ts+".pprof")
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			f, err := os.Create(path)
			if err != nil {
				logger.Errorf("[HOOKS] dump_pprof: %v", err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				logger.Errorf("[HOOKS] dump_pprof write: %v", err)
				return
			}
			logger.Infof("[HOOKS] dump_pprof → %s", path)
		}()
	}
}

