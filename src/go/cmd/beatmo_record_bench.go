//go:build !js

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

// startRecordBench opens the bench dir, a CPU profile and a recording-lifecycle logger; stopFn writes the final heap
// profile. Env: BEATMO_BLOCK_PROFILE / BEATMO_MUTEX_PROFILE (rate), BEATMO_RECORD_BENCH_HEAP_SNAPSHOTS=1 (heap at 5/30/60 s),
// BEATMO_RECORD_BENCH_SYNTH_CHURN_HZ (param churn; pair with -scene crop_synth_tab_drum).
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
	logger.Infof("[record-bench] outDir=%s cpuProfile=%s", outDir, cpuPath)

	blockRate := bench.EnvInt("BEATMO_BLOCK_PROFILE", 0)
	mutexFrac := bench.EnvInt("BEATMO_MUTEX_PROFILE", 0)
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
		logger.Infof("[record-bench] block profiling enabled rate=%d", blockRate)
	}
	if mutexFrac > 0 {
		runtime.SetMutexProfileFraction(mutexFrac)
		logger.Infof("[record-bench] mutex profiling enabled fraction=%d", mutexFrac)
	}

	subUnsubs := []func(){
		hooks.Subscribe(hooks.EventRecordStart, func(e hooks.Event) {
			logger.Infof("[record-bench/hook] record.start payload=%v", e.Payload)
		}),
		hooks.Subscribe(hooks.EventRecordStop, func(e hooks.Event) {
			logger.Infof("[record-bench/hook] record.stop payload=%v", e.Payload)
		}),
		hooks.Subscribe(hooks.EventRecordDropped, func(e hooks.Event) {
			logger.Errorf("[record-bench/hook] record.dropped count=%v — pipeline backpressure!", e.Payload)
		}),
	}

	// Both env-gated so -record-bench's default behaviour is unchanged.
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
		// Before the final heap profile, so the churn goroutine is not still editing params during it.
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
		logger.Infof("[record-bench] artifacts in %s", outDir)
	}
	return outDir, stop, nil
}

// runHeapSnapshotSchedule writes heap profiles at 5, 30 and 60 s (allocs at 60 s), each after a forced GC so it shows
// the live retained set. Stable filenames, so analyze_synth_profile.sh finds them.
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

// writeNamedHeapProfile is writeHeapProfile with a caller-chosen basename, so the schedule never clobbers heap.pprof.
func writeNamedHeapProfile(outDir, name string, logger *game_log.Logger) {
	runtime.GC()
	path := filepath.Join(outDir, name+".pprof")
	f, err := os.Create(path)
	if err != nil {
		logger.Errorf("[record-bench] create %s profile: %v", name, err)
		return
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		logger.Errorf("[record-bench] write %s profile: %v", name, err)
		return
	}
	logger.Infof("[record-bench] %s profile → %s", name, path)
}

// runSynthParamChurn sweeps pitch and amp_decay on every instrument at hz, the rate a dragged knob produces; it needs no
// open Synth tab, but pair it with -scene crop_synth_tab_drum to exercise the draw stack too.
func runSynthParamChurn(hz int, logger *game_log.Logger, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	if hz <= 0 {
		return
	}
	period := time.Second / time.Duration(hz)
	if period <= 0 {
		period = time.Millisecond
	}
	logger.Infof("[record-bench] synth param churn enabled hz=%d period=%s", hz, period)
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	start := time.Now()
	var ticks int
	for {
		select {
		case <-stop:
			logger.Infof("[record-bench] synth param churn stopped ticks=%d", ticks)
			return
		case <-ticker.C:
			ticks++
			elapsed := time.Since(start).Seconds()
			pitch := -12 + 24*math.Sin(elapsed*2.0)
			decay := 0.25 + 0.75*math.Sin(elapsed*1.7)
			for _, id := range audio.Instruments() {
				audio.SetInstrumentParam(id, "pitch", pitch)
				audio.SetInstrumentParam(id, "amp_decay", decay)
			}
		}
	}
}

// writeNamedProfile dumps a runtime/pprof named profile (e.g. "block",
// "mutex", "goroutine", "allocs") to <outDir>/<name>.pprof.
func writeNamedProfile(outDir, name string, logger *game_log.Logger) {
	prof := pprof.Lookup(name)
	if prof == nil {
		logger.Errorf("[record-bench] no profile named %q", name)
		return
	}
	path := filepath.Join(outDir, name+".pprof")
	f, err := os.Create(path)
	if err != nil {
		logger.Errorf("[record-bench] create %s profile: %v", name, err)
		return
	}
	defer f.Close()
	if err := prof.WriteTo(f, 0); err != nil {
		logger.Errorf("[record-bench] write %s profile: %v", name, err)
		return
	}
	logger.Infof("[record-bench] %s profile → %s", name, path)
}

func writeHeapProfile(outDir string, logger *game_log.Logger) {
	runtime.GC()
	heapPath := filepath.Join(outDir, "heap.pprof")
	f, err := os.Create(heapPath)
	if err != nil {
		logger.Errorf("[record-bench] create heap profile: %v", err)
		return
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		logger.Errorf("[record-bench] write heap profile: %v", err)
		return
	}
	logger.Infof("[record-bench] heap profile → %s", heapPath)
}

// audioCountersAdapter keeps internal/bench free of an internal/audio import.
type audioCountersAdapter struct{}

func (audioCountersAdapter) RecordingDrops() int64 { return audio.RecordingDrops() }
func (audioCountersAdapter) IsRecording() bool     { return audio.IsRecording() }

func writeRuntimeStats(outDir string, logger *game_log.Logger) {
	stats := bench.RuntimeStatsSnapshot(audioCountersAdapter{})
	b, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		logger.Errorf("[record-bench] marshal runtime stats: %v", err)
		return
	}
	statsPath := filepath.Join(outDir, "runtime_stats.json")
	if err := os.WriteFile(statsPath, b, 0o644); err != nil {
		logger.Errorf("[record-bench] write runtime stats: %v", err)
	}
}

// The storage lives in internal/bench so the counter is testable alone.
var hookActionCount = &bench.ActionCounter{}

// loadHooksConfig registers subscribers for the built-in actions (log, exit, dump_pprof); the cleanup unsubscribes.
// Recording stays tied to -record-bench on purpose.
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
		logger.Errorf("[hooks] unknown action: %s", action)
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
				logger.Errorf("[hooks] dump_pprof: %v", err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				logger.Errorf("[hooks] dump_pprof write: %v", err)
				return
			}
			logger.Infof("[hooks] dump_pprof → %s", path)
		}()
	}
}
