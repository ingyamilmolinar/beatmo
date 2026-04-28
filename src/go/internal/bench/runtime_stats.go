package bench

import "runtime"

// AudioCounters is the minimal audio surface RuntimeStatsSnapshot needs.
// internal/audio adapts to this interface so internal/bench stays
// dependency-free of internal/audio (avoids an import cycle and lets
// tests substitute a recorder).
type AudioCounters interface {
	RecordingDrops() int64
	IsRecording() bool
}

// RuntimeStatsSnapshot collects runtime counters into a marshal-ready
// map. Pure: no I/O, no logger. The cmd-side wrapper handles the
// JSON write + error logging.
func RuntimeStatsSnapshot(audio AudioCounters) map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	stats := map[string]any{
		"goroutines":   runtime.NumGoroutine(),
		"heap_alloc_kb": ms.Alloc / 1024,
		"heap_sys_kb":   ms.HeapSys / 1024,
		"heap_objects":  ms.HeapObjects,
	}
	if audio != nil {
		stats["recording_drops"] = audio.RecordingDrops()
		stats["recording_active"] = audio.IsRecording()
	}
	return stats
}
